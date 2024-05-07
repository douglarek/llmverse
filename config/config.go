package config

import (
	"encoding/json"
	"errors"
	"os"
)

type Settings struct {
	DiscordBotToken string   `json:"discord_bot_token"`
	EnableDebug     bool     `json:"enable_debug"`     // optional, default: false
	HistoryMaxSize  *int     `json:"history_max_size"` // optional, default: 2048
	OutputMaxSize   *int     `json:"output_max_size"`  // optional, default: 8192. note: context_windows >= history_max_size + output_max_size + discord_message_limit(about 2048 tokens)
	SystemPrompt    string   `json:"system_prompt"`    // optional, default: "You are a helpful AI assistant."
	Temperature     *float64 `json:"temperature"`      // optional, default: 0.7
	Models          struct {
		OpenAI   *openai   `json:"openai"`
		Google   *google   `json:"google"`
		Mistral  *mistral  `json:"mistral"`
		Groq     *groq     `json:"groq"`
		Bedrock  *bedrock  `json:"aws_bedrock"`
		Azure    *azure    `json:"azure"`
		Deepseek *deepseek `json:"deepseek"`
	} `json:"models"`
}

var _ json.Unmarshaler = (*Settings)(nil)

func (s *Settings) UnmarshalJSON(data []byte) error {
	type Alias Settings
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if s.DiscordBotToken == "" {
		return errors.New("discord_bot_token is required")
	}

	if s.HistoryMaxSize == nil {
		s.HistoryMaxSize = new(int)
		*s.HistoryMaxSize = 2048
	}

	if s.OutputMaxSize == nil {
		s.OutputMaxSize = new(int)
		*s.OutputMaxSize = 4096
	}

	if s.SystemPrompt == "" {
		s.SystemPrompt = "You are a helpful AI assistant."
	}

	if s.Temperature == nil {
		s.Temperature = new(float64)
		*s.Temperature = 0.7
	}

	// at most one model can be enabled and at least one model must be enabled
	var enabledModels int

	if s.IsOpenAIEnabled() {
		enabledModels++
	}
	if s.IsGoogleEnabled() {
		enabledModels++
	}
	if s.IsMistralEnabled() {
		enabledModels++
	}
	if s.IsGroqEnabled() {
		enabledModels++
	}
	if s.IsBedrockEnabled() {
		enabledModels++
	}
	if s.IsAzureEnabled() {
		enabledModels++
	}
	if s.IsDeepseekEnabled() {
		enabledModels++
	} // added if statement when new model is added

	if enabledModels == 0 {
		return errors.New("at least one model must be enabled")
	}

	if enabledModels > 1 {
		return errors.New("only one model can be enabled")
	}

	return nil
}

func (s Settings) IsOpenAIEnabled() bool {
	return s.Models.OpenAI != nil && s.Models.OpenAI.Enabled
}

func (s Settings) IsGoogleEnabled() bool {
	return s.Models.Google != nil && s.Models.Google.Enabled
}

func (s Settings) IsMistralEnabled() bool {
	return s.Models.Mistral != nil && s.Models.Mistral.Enabled
}

func (s Settings) IsGroqEnabled() bool {
	return s.Models.Groq != nil && s.Models.Groq.Enabled
}

func (s Settings) IsBedrockEnabled() bool {
	return s.Models.Bedrock != nil && s.Models.Bedrock.Enabled
}

func (s Settings) IsAzureEnabled() bool {
	return s.Models.Azure != nil && s.Models.Azure.Enabled
}

func (s Settings) IsDeepseekEnabled() bool {
	return s.Models.Deepseek != nil && s.Models.Deepseek.Enabled
}

func (s Settings) IsVisionSupported() bool {
	return s.IsOpenAIEnabled() || s.IsGoogleEnabled() || s.IsBedrockEnabled()
}

func (s Settings) IsToolSupported() bool {
	return s.IsGoogleEnabled()
}

func LoadSettings(filePath string) (Settings, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Settings{}, err
	}

	var config Settings
	if err := json.Unmarshal(data, &config); err != nil {
		return Settings{}, err
	}
	return config, nil
}
