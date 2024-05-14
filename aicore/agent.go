package aicore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/douglarek/llmverse/config"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/mistral"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/memory"
)

func buildModelsFromConfig(settings config.Settings) map[string]llms.Model {
	var model llms.Model
	var err error
	models := make(map[string]llms.Model)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, v := range settings.Models {
		if !v.Enabled {
			continue
		}

		switch v.Name {
		case config.OpenAI:
			model, err = openai.New(
				openai.WithToken(v.APIKey),
				openai.WithModel(v.Model),
				openai.WithBaseURL(v.BaseURL),
			)
		case config.Google:
			model, err = googleai.New(ctx,
				googleai.WithAPIKey(v.APIKey),
				googleai.WithDefaultModel(v.Model),
				googleai.WithHarmThreshold(googleai.HarmBlockNone),
			)
		case config.Mistral:
			model, err = mistral.New(
				mistral.WithAPIKey(v.APIKey),
				mistral.WithModel(v.Model),
			)
		case config.Bedrock:
			options := bedrockruntime.New(bedrockruntime.Options{
				Region: v.RegionName,
				Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
					return aws.Credentials{
						AccessKeyID:     v.AccessKeyID,
						SecretAccessKey: v.SecretAccessKey,
					}, nil
				}),
			})
			model, err = bedrock.New(
				bedrock.WithModel(v.ModelID),
				bedrock.WithClient(options),
			)
		case config.Azure:
			model, err = openai.New(
				openai.WithToken(v.APIKey),
				openai.WithModel(v.Model),
				openai.WithBaseURL(v.BaseURL),
				openai.WithAPIVersion(v.APIVersion),
				openai.WithAPIType(openai.APITypeAzure),
			)
		case config.Deepseek:
			model, err = openai.New(
				openai.WithToken(v.APIKey),
				openai.WithBaseURL(v.BaseURL),
				openai.WithModel(v.Model),
			)
		case config.ChatGLM:
			model, err = openai.New(
				openai.WithToken(v.APIKey),
				openai.WithModel(v.Model),
				openai.WithBaseURL(v.BaseURL),
			)
		}

		if err != nil {
			panic(err)
		}

		models[v.Name] = model
	}

	return models
}

type LLMAgent struct {
	models   map[string]llms.Model
	history  sync.Map
	settings config.Settings
}

func (a *LLMAgent) loadHistory(_ context.Context, model llms.Model, user string) *memory.ConversationTokenBuffer {
	v, _ := a.history.LoadOrStore(user, memory.NewConversationTokenBuffer(model, *a.settings.HistoryMaxSize))
	return v.(*memory.ConversationTokenBuffer)
}

func (a *LLMAgent) ClearHistory(_ context.Context, user string) {
	a.history.Delete(user)
	slog.Debug("history cleared", "user", user)
}

func (a *LLMAgent) saveHistory(ctx context.Context, model llms.Model, user, input, output string) error {
	ch := a.loadHistory(ctx, model, user).ChatHistory
	if err := ch.AddUserMessage(ctx, input); err != nil {
		return err
	}
	return ch.AddAIMessage(ctx, output)
}

func downloadImage(_ context.Context, url string) ([]byte, error) {
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func parseImageParts(modelName string, imageURLs []string) (parts []llms.ContentPart, err error) {
	for _, url := range imageURLs {
		if modelName == config.OpenAI || modelName == config.Azure {
			parts = append(parts, llms.ImageURLPart(url))
		} else {
			b, err := downloadImage(context.Background(), url)
			if err != nil {
				return nil, err
			}
			parts = append(parts, llms.BinaryPart("image/png", b))
		}
	}

	return
}

func (a *LLMAgent) Query(ctx context.Context, user string, input string, imageURLs []string) (<-chan string, error) {
	slog.Info("[LLMAgent.Query] query", "user", user, "input", input, "imageURLs", imageURLs)

	modelName := a.settings.GetModelName(input)
	model := a.models[modelName]
	if model == nil {
		return nil, fmt.Errorf("available models: %s. question should start with `[model]:`", a.settings.GetAvailableModels())
	}

	output := make(chan string)
	var err error

	if len(imageURLs) > 0 && !a.settings.GetVisionSupport(modelName) {
		close(output)
		return output, errors.New("vision of current model not enabled")
	}

	var content []llms.MessageContent

	{ // system prompt
		parts := []llms.ContentPart{llms.TextPart(a.settings.SystemPrompt)}
		content = append(content, llms.MessageContent{
			Role:  llms.ChatMessageTypeSystem,
			Parts: parts,
		})
	}

	chatHistory := a.loadHistory(ctx, model, user+"_"+modelName).ChatHistory
	{ // chat history
		cm, _ := chatHistory.Messages(ctx)
		for _, m := range cm {
			switch m.GetType() {
			case llms.ChatMessageTypeHuman:
				parts := []llms.ContentPart{llms.TextPart(m.GetContent())}
				content = append(content, llms.MessageContent{
					Role:  llms.ChatMessageTypeHuman,
					Parts: parts,
				})
			case llms.ChatMessageTypeAI:
				parts := []llms.ContentPart{llms.TextPart(m.GetContent())}
				content = append(content, llms.MessageContent{
					Role:  llms.ChatMessageTypeAI,
					Parts: parts,
				})
			}
		}
	}

	{ // user input
		var parts []llms.ContentPart

		parts = append(parts, llms.TextPart(input))

		ps, err := parseImageParts(modelName, imageURLs)
		if err != nil {
			close(output)
			return output, err
		}
		parts = append(parts, ps...)

		content = append(content, llms.MessageContent{
			Role:  llms.ChatMessageTypeHuman,
			Parts: parts,
		})
	}

	// parseTools
	options := []llms.CallOption{llms.WithTemperature(*a.settings.Temperature), llms.WithMaxTokens(*a.settings.OutputMaxSize)}

	go func() {
		defer close(output)

		output <- modelName + ": "

		// function tools
		if a.settings.GetToolSupport(modelName) {
			options = append(options, llms.WithTools(availableTools))
			var return_direct bool
			content, return_direct, err = executeToolCalls(ctx, model, options, content, output)
			if err != nil {
				output <- err.Error()
				return
			}
			if return_direct { // return directly, since stream response has been sent to output
				// save chat history
				if err = a.saveHistory(ctx, model, user+"_"+modelName, input, content[len(content)-1].Parts[0].(llms.TextContent).Text); err != nil {
					slog.Error("[LLMAgent.Query] failed to save history", "error", err)
				}
				return
			}
			slog.Debug("[LLMAgent.Query] parsed tools", "content", content[len(content)-1])
		}

		// streaming
		var isStreaming bool
		options = append(options, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			isStreaming = true
			output <- string(chunk)
			return nil
		}))
		resp, err := model.GenerateContent(ctx, content, options...)
		if err != nil {
			output <- err.Error()
			return
		}

		if !isStreaming {
			slog.Warn("[LLMAgent.Query] current model does not support streaming")
			if v := resp.Choices[0].Content; v != "" {
				output <- resp.Choices[0].Content
			} else {
				return
			}
		}

		// save chat history
		if err = a.saveHistory(ctx, model, user, input, resp.Choices[0].Content); err != nil {
			slog.Error("[LLMAgent.Query] failed to save history", "error", err)
		}
	}()

	return output, err
}

func NewLLMAgent(settings config.Settings) *LLMAgent {
	return &LLMAgent{
		models:   buildModelsFromConfig(settings),
		settings: settings,
	}
}
