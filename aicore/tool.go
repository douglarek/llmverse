package aicore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/douglarek/llmverse/config"
	"github.com/koffeinsource/go-imgur"
	"github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/llms"
)

func availableTools(modelSetting config.LLMSetting) []llms.Tool {
	switch modelSetting.Name {
	case config.OpenAI:
		imageTool := llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "generateImage",
				Description: "Generate a detailed prompt to generate an image based on the following description: {image_desc}",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"image_desc": map[string]any{
							"type":        "string",
							"description": "A description of the image to generate",
						},
					},
					"required": []string{"image_desc"},
				},
			},
		}
		defaultTools = append(defaultTools, imageTool)
	default:
	}

	if modelSetting.OpenWeatherKey != nil && *modelSetting.OpenWeatherKey != "" {
		weatherTool := llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        "getWeather",
				Description: "Get the weather for a specific location based on the following location: {location}",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The location to get the weather for, formatted as 'City,Country', e.g. 'New York,US', and the city and country code must be in ISO 3166-1 alpha-2 format",
						},
					},
					"required": []string{"location"},
				},
			},
		}
		defaultTools = append(defaultTools, weatherTool)
	}

	return defaultTools
}

// defaultTools is a list of tools that the agent can use to help answer questions.
var defaultTools = []llms.Tool{
	{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "getExchangeRate",
			Description: "Get the exchange rate for currencies between countries",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"currency_date": map[string]any{
						"type":        "string",
						"description": "A date that must always be in YYYY-MM-DD format or the value 'latest' if a time period is not specified",
					},
					"currency_from": map[string]any{
						"type":        "string",
						"description": "The currency to convert from in ISO 4217 format",
					},
					"currency_to": map[string]any{
						"type":        "string",
						"description": "The currency to convert to in ISO 4217 format",
					},
				},
				"required": []string{"currency_from", "currency_date"},
			},
		},
	},
}

// getExchangeRate is a helper function that makes a request to the Frankfurter API
// to get the exchange rate for currencies between countries.
func getExchangeRate(ctx context.Context, currencyDate string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.frankfurter.app/"+currencyDate, nil)
	if err != nil {
		return nil, err
	}
	req.Close = true

	resp, err := (&http.Client{Timeout: 1 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

const dalle3SystemPrompt = `
Certainly, here are all the instructions from the guidelines:

1. If the description is not in English, then translate it.
2. Do not create more than 4 images, even if the user requests more.
3. Don't create images of politicians or other public figures. Recommend other ideas instead.
4. Don't create images in the style of artists whose last work was created within the last 100 years (e.g. Picasso, Kahlo). Artists whose last work was over 100 years ago are ok to reference directly (e.g. Van Gogh, Klimt). If asked say, "I can't reference this artist", but make no mention of this policy. Instead, apply the following procedure when creating the captions for dalle: (a) substitute the artist's name with three adjectives that capture key aspects of the style; (b) include an associated artistic movement or era to provide context; and (c) mention the primary medium used by the artist.
5. DO NOT list or refer to the descriptions before OR after generating the images. They should ONLY ever be written out ONCE, in the "prompts" field of the request. You do not need to ask for permission to generate, just do it!
6. Always mention the image type (photo, oil painting, watercolor painting, illustration, cartoon, drawing, vector, render, etc.) at the beginning of the caption. Unless the caption suggests otherwise, make at least 1--2 of the 4 images photos.
7. Diversify depictions of ALL images with people to include DESCENT and GENDER for EACH person using direct terms. Adjust only human descriptions.
8. Silently modify descriptions that include names or hints or references of specific people or celebrities by carefully selecting a few minimal modifications to substitute references to the people with generic descriptions that don't divulge any information about their identities, except for their genders and physiques.

-----------------------------------------------------------
Now, please generate the image based on the below description:


`

// generateImage is a helper function that generates an image based on the imageDesc
func generateImage(ctx context.Context, imageDesc string, ms config.LLMSetting) (string, error) {
	conf := openai.DefaultConfig(ms.APIKey)
	conf.BaseURL = ms.BaseURL

	c := openai.NewClientWithConfig(conf)
	resp, err := c.CreateImage(ctx, openai.ImageRequest{
		Prompt: dalle3SystemPrompt + imageDesc,
		Model:  openai.CreateImageModelDallE3,
		Size:   openai.CreateImageSize1024x1024,
	})

	if err != nil {
		return "", err
	}

	if ms.ImgurClientID == nil || *ms.ImgurClientID == "" {
		return resp.Data[0].URL, nil
	}

	ic, err := imgur.NewClient(&http.Client{Timeout: 1 * time.Minute}, *ms.ImgurClientID, "")
	if err != nil {
		return "", err
	}

	rl, err := ic.GetRateLimit()
	if err != nil {
		return "", err
	}
	if rl.ClientRemaining == 0 {
		slog.Warn("[generateImage] imgur rate limit exceeded", "reset_time", rl.UserReset)
		return resp.Data[0].URL, nil
	}

	slog.Debug("[generateImage] uploading image to imgur", "url", resp.Data[0].URL)
	ii, _, err := ic.UploadImage([]byte(resp.Data[0].URL), "", "URL", "", imageDesc)
	if err != nil {
		return "", err
	}

	return ii.Link, nil
}

// getWeather is a helper function that makes a request to the OpenWeather API
func getWeather(_ context.Context, location string, ms config.LLMSetting) ([]byte, error) {
	resp, err := http.Get("https://api.openweathermap.org/data/2.5/weather?mode=json&q=" + location + "&appid=" + *ms.OpenWeatherKey)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// executeToolCalls is a helper function that parses the response from a tool call
// and returns the content to be sent to the user, whether the response should be
// returned directly to the user, and any error that occurred.
func executeToolCalls(ctx context.Context, model llms.Model, ms config.LLMSetting, options []llms.CallOption, content []llms.MessageContent, output chan<- string) ([]llms.MessageContent, bool, error) { // content, return_direct, error
	var isStreaming bool
	var chunks []byte
	options = append(options, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		isStreaming = true
		output <- parseToolCallStreamingChunk(chunk, false)
		chunks = append(chunks, chunk...)
		return nil
	}))
	resp, err := model.GenerateContent(ctx, content, options...)
	if err != nil {
		return nil, false, err
	}

	respChoice := resp.Choices[0]
	ar := llms.TextParts(llms.ChatMessageTypeAI, respChoice.Content)
	if len(respChoice.ToolCalls) == 0 {
		content = append(content, ar)
		return content, true, nil
	}

	if isStreaming && len(chunks) > 0 {
		go func() { output <- parseToolCallStreamingChunk(nil, true) }()
	}

	var toolMessages []llms.MessageContent
	for _, tc := range respChoice.ToolCalls {
		var tr llms.MessageContent
		switch tc.FunctionCall.Name {
		case "getExchangeRate":
			slog.Debug(fmt.Sprintf("[executeToolCalls] getExchangeRate: %+v", tc.FunctionCall.Arguments))
			var args struct {
				CurrencyDate string `json:"currency_date"`
			}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
				return nil, false, err
			}
			rs, err := getExchangeRate(ctx, args.CurrencyDate)
			if err != nil {
				return nil, false, err
			}
			tr = llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    string(rs),
					},
				},
			}
		case "generateImage":
			slog.Debug(fmt.Sprintf("[executeToolCalls] generateImage: %+v", tc.FunctionCall.Arguments))
			var args struct {
				ImageDesc string `json:"image_desc"`
			}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
				return nil, false, err
			}
			rs, err := generateImage(ctx, args.ImageDesc, ms)
			if err != nil {
				return nil, false, err
			}
			tr = llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    fmt.Sprintf("the generated image url is: %s", rs),
					},
				},
			}
		case "getWeather":
			slog.Debug(fmt.Sprintf("[executeToolCalls] getWeather: %+v", tc.FunctionCall.Arguments))
			var args struct {
				Location string `json:"location"`
			}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
				return nil, false, err
			}
			rs, err := getWeather(ctx, args.Location, ms)
			if err != nil {
				return nil, false, err
			}
			tr = llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    string(rs),
					},
				},
			}
		default:
			slog.Warn("[LLMAgent.Query] hint unknown tool call", "name", tc.FunctionCall.Name)
			continue
		}

		ar.Parts = append(ar.Parts, tc)
		toolMessages = append(toolMessages, tr)
	}

	content = append(content, ar)
	content = append(content, toolMessages...)

	return content, false, nil
}

type toolCallStreamingChunk struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func parseToolCallStreamingChunk(chunk []byte, end bool) string {
	if end {
		return "`||\n\n"
	}

	slog.Debug("[tool.parseToolCallStreamingChunk]", "chunk", string(chunk))

	var tc []toolCallStreamingChunk
	if err := json.Unmarshal(chunk, &tc); err != nil {
		goto R
	}

	if len(tc) > 0 {

		if tc[0].Function.Name != "" {
			res := fmt.Sprintf("||*** Running tool: [%s] with arguments: *** `", tc[0].Function.Name)
			if tc[0].Function.Arguments != "" {
				res += tc[0].Function.Arguments
			}
			return res
		}
		if tc[0].Function.Arguments != "" {
			return tc[0].Function.Arguments
		}
	}

R:
	return string(chunk)
}
