package config

import (
	"encoding/json"
	"errors"
	"os"
)

type Settings struct {
	DiscordBotToken string   `json:"discord_bot_token"`
	EnableDebug     bool     `json:"enable_debug"`     // optional, default: false
	HistoryMaxSize  *int     `json:"history_max_size"` // optional, default: 8192
	SystemPrompt    string   `json:"system_prompt"`    // optional, default: "You are a helpful AI assistant."
	Temperature     *float64 `json:"temperature"`      // optional, default: 0.7
	Models          struct {
		OpenAI  *openai  `json:"openai"`
		Google  *google  `json:"google"`
		Mistral *mistral `json:"mistral"`
		Groq    *groq    `json:"groq"`
		Bedrock *bedrock `json:"aws_bedrock"`
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
		*s.HistoryMaxSize = 8192
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

func (s Settings) HasVision() bool {
	return s.IsGoogleEnabled() || s.IsBedrockEnabled()
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
