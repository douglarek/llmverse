package aicore

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/douglarek/llmverse/config"
	"github.com/tmc/langchaingo/llms"
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

	if settings.IsGoogleEnabled() {
		model, err = googleai.New(ctx, googleai.WithAPIKey(settings.GoogleAPIKey), googleai.WithDefaultModel(settings.GoogleAPIModel), googleai.WithHarmThreshold(googleai.HarmBlockNone))
	} else if settings.IsGroqEnabled() {
		model, err = openai.New(openai.WithBaseURL(settings.GroqEndpoint), openai.WithToken(settings.GroqAPIKey), openai.WithModel(settings.GroqModel))
	} else if settings.IsMistralEnabled() {
		model, err = mistral.New(mistral.WithAPIKey(settings.MistralAPIKey), mistral.WithModel(settings.MistralModel))
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
	v, _ := a.history.LoadOrStore(user, memory.NewConversationTokenBuffer(a.model, a.settings.HistoryMaxSize))
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

func (a *LLMAgent) Query(ctx context.Context, user string, input string, imageURLs []string) (string, error) {
	slog.Info("[LLMAgent.Query] query", "user", user, "input", input, "imageURLs", imageURLs)
	if len(imageURLs) > 0 && !a.settings.HasVision() {
		return "", errors.New("vision of current model not enabled")
	}

	var content []llms.MessageContent

	{ // system prompt
		if !a.settings.IsGoogleEnabled() { // langchaingo does not support gemini system prompt
			parts := []llms.ContentPart{llms.TextPart(a.settings.SystemPrompt)}
			content = append(content, llms.MessageContent{
				Role:  llms.ChatMessageTypeSystem,
				Parts: parts,
			})
		}
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
		for _, url := range imageURLs {
			b, err := downloadImage(ctx, url)
			if err != nil {
				return "", err
			}
			parts = append(parts, llms.BinaryPart("image/png", b))
		}
		parts = append(parts, llms.TextPart(input))

		content = append(content, llms.MessageContent{
			Role:  llms.ChatMessageTypeHuman,
			Parts: parts,
		})
	}

	slog.Debug("[LLMAgent.Query]", "content", content)

	resp, err := a.model.GenerateContent(ctx, content)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no response")
	}

	// save chat history
	if err := chatHistory.AddUserMessage(ctx, input); err != nil {
		slog.Error("[LLMAgent.Query] failed to save user message", "error", err)
	}
	if err := chatHistory.AddAIMessage(ctx, resp.Choices[0].Content); err != nil {
		slog.Error("[LLMAgent.Query] failed to save ai message", "error", err)
	}

	return resp.Choices[0].Content, nil
}

func NewLLMAgent(settings config.Settings) *LLMAgent {
	return &LLMAgent{
		model:    buildModelFromConfig(settings),
		settings: settings,
	}
}
