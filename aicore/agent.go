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
	"github.com/tmc/langchaingo/prompts"
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
	model          llms.Model
	history        sync.Map
	promptTemplate prompts.ChatPromptTemplate
	settings       config.Settings
}

func (a *LLMAgent) loadHistory(_ context.Context, user string) *memory.ConversationTokenBuffer {
	v, _ := a.history.LoadOrStore(user, memory.NewConversationTokenBuffer(a.model, a.settings.HistoryMaxSize))
	return v.(*memory.ConversationTokenBuffer)
}

func (a *LLMAgent) ClearHistory(_ context.Context, user string) {
	a.history.Delete(user)
	slog.Debug("history cleared", "user", user)
}

func (a *LLMAgent) Query(ctx context.Context, user string, input string) (string, error) {
	slog.Info("query", "user", user, "input", input)

	historyMessages, _ := a.loadHistory(ctx, user).LoadMemoryVariables(ctx, map[string]any{})
	prompt, _ := a.promptTemplate.Format(map[string]any{
		"historyMessages": historyMessages,
		"question":        input,
	})
	slog.Debug("prompt", "user", user, "prompt", prompt)

	resp, err := llms.GenerateFromSinglePrompt(ctx, a.model, prompt, llms.WithTemperature(a.settings.Temperature))
	if err == nil {
		ctb := a.loadHistory(ctx, user)
		if err := ctb.SaveContext(ctx, map[string]any{"input": input}, map[string]any{"output": resp}); err != nil {
			slog.Error("failed to save context", "error", err)
		}
		slog.Info("response", "user", "ai", "response", resp)
	}
	return resp, err
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

func (a *LLMAgent) QueryVision(ctx context.Context, user string, input string, imageURLs []string) (string, error) {
	slog.Info("query", "user", user, "input", input, "imageURLs", imageURLs)
	if !a.settings.HasVision() {
		return "", errors.New("vision not enabled")
	}

	var parts []llms.ContentPart
	for _, url := range imageURLs {
		b, err := downloadImage(ctx, url)
		if err != nil {
			return "", err
		}
		parts = append(parts, llms.BinaryPart("image/png", b))
	}
	parts = append(parts, llms.TextPart(input))

	content := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: parts,
		},
	}
	resp, err := a.model.GenerateContent(ctx, content)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no response")
	}

	return resp.Choices[0].Content, nil
}

func NewLLMAgent(settings config.Settings) *LLMAgent {
	pt := prompts.NewChatPromptTemplate([]prompts.MessageFormatter{
		prompts.NewSystemMessagePromptTemplate(
			"You are a helpful AI assistant. 记住：你回复用的语言需要和问题的语言一致。比如用户使用中文问你，你应该使用中文回复，其他的类同，总之你需要保持一致。",
			nil,
		),
		// Insert history
		prompts.NewGenericMessagePromptTemplate(
			"history",
			"\n{{index .historyMessages \"history\"}}\n",
			[]string{"history"},
		),
		prompts.NewHumanMessagePromptTemplate(
			`{{.question}}`,
			[]string{"question"},
		),
	})
	return &LLMAgent{
		model:          buildModelFromConfig(settings),
		promptTemplate: pt,
		settings:       settings,
	}
}
