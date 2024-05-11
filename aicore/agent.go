package aicore

import (
	"context"
	"errors"
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

func buildModelFromConfig(settings config.Settings) llms.Model {
	var model llms.Model
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if settings.IsOpenAIEnabled() {
		model, err = openai.New(
			openai.WithToken(settings.Models.OpenAI.APIKey),
			openai.WithModel(settings.Models.OpenAI.Model),
			openai.WithBaseURL(settings.Models.OpenAI.BaseURL),
		)
	} else if settings.IsGoogleEnabled() {
		model, err = googleai.New(ctx,
			googleai.WithAPIKey(settings.Models.Google.APIKey),
			googleai.WithDefaultModel(settings.Models.Google.Model),
			googleai.WithHarmThreshold(googleai.HarmBlockNone),
		)
	} else if settings.IsGroqEnabled() {
		model, err = openai.New(
			openai.WithBaseURL(settings.Models.Groq.BaseURL),
			openai.WithToken(settings.Models.Groq.APIKey),
			openai.WithModel(settings.Models.Groq.Model),
		)
	} else if settings.IsMistralEnabled() {
		model, err = mistral.New(
			mistral.WithAPIKey(settings.Models.Mistral.APIKey),
			mistral.WithModel(settings.Models.Mistral.Model),
		)
	} else if settings.IsBedrockEnabled() {
		options := bedrockruntime.New(bedrockruntime.Options{
			Region: settings.Models.Bedrock.RegionName,
			Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     settings.Models.Bedrock.AccessKeyID,
					SecretAccessKey: settings.Models.Bedrock.SecretAccessKey,
				}, nil
			}),
		})
		model, err = bedrock.New(
			bedrock.WithModel(settings.Models.Bedrock.ModelID),
			bedrock.WithClient(options),
		)
	} else if settings.IsAzureEnabled() {
		model, err = openai.New(
			openai.WithToken(settings.Models.Azure.APIKey),
			openai.WithModel(settings.Models.Azure.Model),
			openai.WithBaseURL(settings.Models.Azure.BaseURL),
			openai.WithAPIVersion(settings.Models.Azure.APIVersion),
			openai.WithAPIType(openai.APITypeAzure),
		)
	} else if settings.IsDeepseekEnabled() {
		model, err = openai.New(
			openai.WithToken(settings.Models.Deepseek.APIKey),
			openai.WithBaseURL(settings.Models.Deepseek.BaseURL),
			openai.WithModel(settings.Models.Deepseek.Model),
		)
	} else if settings.IsQwenEnabled() {
		model, err = openai.New(
			openai.WithToken(settings.Models.Qwen.APIKey),
			openai.WithModel(settings.Models.Qwen.Model),
			openai.WithBaseURL(settings.Models.Qwen.BaseURL),
		)
	} else {
		panic("no model available")
	}

	if err != nil {
		panic(err)
	}

	return model
}

type LLMAgent struct {
	model    llms.Model
	history  sync.Map
	settings config.Settings
}

func (a *LLMAgent) loadHistory(_ context.Context, user string) *memory.ConversationTokenBuffer {
	v, _ := a.history.LoadOrStore(user, memory.NewConversationTokenBuffer(a.model, *a.settings.HistoryMaxSize))
	return v.(*memory.ConversationTokenBuffer)
}

func (a *LLMAgent) ClearHistory(_ context.Context, user string) {
	a.history.Delete(user)
	slog.Debug("history cleared", "user", user)
}

func (a *LLMAgent) saveHistory(ctx context.Context, user, input, output string) error {
	ch := a.loadHistory(ctx, user).ChatHistory
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

func parseImageParts(s config.Settings, imageURLs []string) (parts []llms.ContentPart, err error) {
	for _, url := range imageURLs {
		if s.IsOpenAIEnabled() {
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

	output := make(chan string)
	var err error

	if len(imageURLs) > 0 && !a.settings.IsVisionSupported() {
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

	chatHistory := a.loadHistory(ctx, user).ChatHistory
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

		ps, err := parseImageParts(a.settings, imageURLs)
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

		// function tools
		if a.settings.IsToolSupported() {
			options = append(options, llms.WithTools(availableTools))
			var return_direct bool
			content, return_direct, err = executeToolCalls(ctx, a.model, options, content, output)
			if err != nil {
				output <- err.Error()
				return
			}
			if return_direct { // return directly, since stream response has been sent to output
				// save chat history
				if err = a.saveHistory(ctx, user, input, content[len(content)-1].Parts[0].(llms.TextContent).Text); err != nil {
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
		resp, err := a.model.GenerateContent(ctx, content, options...)
		if err != nil {
			output <- err.Error()
			return
		}

		if !isStreaming {
			slog.Warn("[LLMAgent.Query] current model does not support streaming")
			output <- resp.Choices[0].Content
		}

		// save chat history
		if err = a.saveHistory(ctx, user, input, resp.Choices[0].Content); err != nil {
			slog.Error("[LLMAgent.Query] failed to save history", "error", err)
		}
	}()

	return output, err
}

func NewLLMAgent(settings config.Settings) *LLMAgent {
	return &LLMAgent{
		model:    buildModelFromConfig(settings),
		settings: settings,
	}
}
