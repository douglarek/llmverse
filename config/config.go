package config

import (
	"github.com/caarlos0/env/v11"
)

type Settings struct {
	DiscordBotToken string  `env:"DISCORD_BOT_TOKEN,required"`
	EnableDebug     bool    `env:"ENABLE_DEBUG"`
	SystemPrompt    string  `env:"SYSTEM_PROMPT" envDefault:"You are a helpful AI assistant. 记住：如无特别要求使用中文回复。"`
	Temperature     float64 `env:"TEMPERATURE" envDefault:"0.7"`
	HistoryMaxSize  int     `env:"HISTORY_MAX_SIZE" envDefault:"8192"`
	GoogleAPIKey    string  `env:"GOOGLE_API_KEY"`
	GoogleAPIModel  string  `env:"GOOGLE_API_MODEL" envDefault:"gemini-1.5-pro-latest"`
	MistralAPIKey   string  `env:"MISTRAL_API_KEY"`
	MistralModel    string  `env:"MISTRAL_MODEL" envDefault:"mistral-medium-latest"`
	GroqAPIKey      string  `env:"GROQ_API_KEY"`
	GroqModel       string  `env:"GROQ_MODEL" envDefault:"llama3-70b-8192"`
	GroqEndpoint    string  `env:"GROQ_ENDPOINT" envDefault:"https://api.groq.com/openai/v1"`
}

func (s Settings) IsGoogleEnabled() bool {
	return s.GoogleAPIKey != ""
}

func (s Settings) IsMistralEnabled() bool {
	return s.MistralAPIKey != ""
}

func (s Settings) IsGroqEnabled() bool {
	return s.GroqAPIKey != ""
}

func (s Settings) HasVision() bool {
	return s.IsGoogleEnabled() // todo: others
}
func Load() Settings {
	var settings Settings
	if err := env.Parse(&settings); err != nil {
		panic(err)
	}
	return settings
}
