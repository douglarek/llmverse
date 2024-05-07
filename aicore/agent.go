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

	if len(imageURLs) > 0 && !a.settings.HasVision() {
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
		slog.Debug("[LLMAgent.Query]", "parts", parts)

		ps, err := parseImageParts(a.settings, imageURLs)
		if err != nil {
			return nil, err
		}
		parts = append(parts, ps...)

		content = append(content, llms.MessageContent{
			Role:  llms.ChatMessageTypeHuman,
			Parts: parts,
		})
	}

	go func() {
		defer close(output)
		var isStreaming bool
		var options []llms.CallOption
		options = append(options, llms.WithTemperature(*a.settings.Temperature), llms.WithMaxTokens(*a.settings.OutputMaxSize))
		options = append(options, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			output <- string(chunk)
			isStreaming = true
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
		if err := chatHistory.AddUserMessage(ctx, input); err != nil {
			slog.Error("[LLMAgent.Query] failed to save user message", "error", err)
		}
		if err := chatHistory.AddAIMessage(ctx, resp.Choices[0].Content); err != nil {
			slog.Error("[LLMAgent.Query] failed to save ai message", "error", err)
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
