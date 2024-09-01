package aicore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
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
		case config.OpenAI, config.Groq, config.Deepseek, config.Qwen, config.ChatGLM, config.Lingyiwanwu:
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

func (a *LLMAgent) loadHistory(_ context.Context, model llms.Model, key string) *memory.ConversationTokenBuffer {
	v, _ := a.history.LoadOrStore(key, memory.NewConversationTokenBuffer(model, *a.settings.HistoryMaxSize))
	return v.(*memory.ConversationTokenBuffer)
}

func (a *LLMAgent) ClearHistory(_ context.Context, user string) {
	a.history.Range(func(k, v interface{}) bool {
		slog.Debug("clearing history", "key", k, "user", user)
		if strings.HasPrefix(k.(string), user) {
			a.history.Delete(k)
		}
		return true
	})
	slog.Debug("history cleared", "user", user)
}

func (a *LLMAgent) saveHistory(ctx context.Context, model llms.Model, key string, content ...llms.MessageContent) error {
	ch := a.loadHistory(ctx, model, key).ChatHistory
	for _, c := range content {
		var err error
		switch c.Role {
		case llms.ChatMessageTypeHuman:
			err = ch.AddUserMessage(ctx, c.Parts[0].(llms.TextContent).Text)
		case llms.ChatMessageTypeAI:
			err = ch.AddAIMessage(ctx, c.Parts[0].(llms.TextContent).Text)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *LLMAgent) historyToContent(ctx context.Context, model llms.Model, key string) []llms.MessageContent {
	var content []llms.MessageContent

	chatHistory := a.loadHistory(ctx, model, key).ChatHistory
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
			// for tool message, temporarily not known how to handle or is it necessary to handle
		}
	}

	return content
}

func downloadImage(_ context.Context, url string) ([]byte, error) {
	c := &http.Client{Timeout: 1 * time.Minute}
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
			// since ChatGLM's server is located in China, it is not possible to use the image URL directly,
			// so we need to convert the image to base64 format
			if modelName == config.ChatGLM {
				// I really can't understand this implementation of ChatGLM.
				// You said it's compatible with OpenAI, but they even removed the 'data:image/jpeg;base64,' prefix.
				// Let it be, it's very amateurish.
				parts = append(parts, llms.ImageURLPart(base64.StdEncoding.EncodeToString(b)))
			} else if modelName == config.Qwen {
				parts = append(parts, llms.ImageURLPart(fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(b))))
			} else {
				parts = append(parts, llms.BinaryPart("image/png", b))
			}
		}
	}

	return
}

func (a *LLMAgent) AvailableModelNames() string {
	var models []string
	for k := range a.models {
		models = append(models, k)
	}
	slices.Sort(models)

	var b bytes.Buffer
	for _, m := range models {
		b.WriteString("`")
		b.WriteString(m)
		b.WriteString("`")
		b.WriteString(", ")
	}
	b.Truncate(b.Len() - 2)

	return b.String()
}

func (a *LLMAgent) ParseModelName(input string) string {
	index := strings.Index(input, ":")
	if index == -1 {
		return ""
	}

	modelName := input[:index]
	for k := range a.models {
		if k == modelName {
			return modelName
		}
	}

	return ""
}

func (a *LLMAgent) Query(ctx context.Context, modelName, user, input string, imageURLs []string) (<-chan string, error) {
	slog.Info("[LLMAgent.Query] query", "user", user, "input", input, "imageURLs", imageURLs)

	model := a.models[modelName]
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

	historyKey := user + "_" + modelName
	{ // chat history
		content = append(content, a.historyToContent(ctx, model, historyKey)...)
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

	slog.Debug("[LLMAgent.Query] content", "content", content)

	// parseTools
	options := []llms.CallOption{llms.WithTemperature(*a.settings.Temperature), llms.WithMaxTokens(*a.settings.OutputMaxSize)}

	go func() {
		defer close(output)

		// function tools
		if a.settings.GetToolSupport(modelName) {
			ms := a.settings.GetLLMModelSetting(modelName)
			options = append(options, llms.WithTools(availableTools(ms)))

			var return_direct bool
			content, return_direct, err = executeToolCalls(ctx, model, ms, options, content, output)
			if err != nil {
				output <- err.Error()
				return
			}

			if return_direct { // return directly, since stream response has been sent to output
				slog.Debug("[LLMAgent.Query] return_direct", "content", content[len(content)-1])
				// save chat history
				if err = a.saveHistory(ctx, model, historyKey, content[len(content)-1]); err != nil {
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
		if err = a.saveHistory(ctx, model, historyKey, llms.TextParts(llms.ChatMessageTypeHuman, input), llms.TextParts(llms.ChatMessageTypeAI, resp.Choices[0].Content)); err != nil {
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
