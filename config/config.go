package config

import (
	"github.com/caarlos0/env/v11"
)

type Settings struct {
	DiscordBotToken      string  `env:"DISCORD_BOT_TOKEN,required"`
	EnableDebug          bool    `env:"ENABLE_DEBUG"`
	SystemPrompt         string  `env:"SYSTEM_PROMPT" envDefault:"You are a helpful AI assistant."`
	Temperature          float64 `env:"TEMPERATURE" envDefault:"0.7"`
	HistoryMaxSize       int     `env:"HISTORY_MAX_SIZE" envDefault:"8192"`
	OpenAIAPIKey         string  `env:"OPENAI_API_KEY"`
	OpenAIBaseURL        string  `env:"OPENAI_BASE_URL" envDefault:"https://api.openai.com/v1"`
	OpenAIModel          string  `env:"OPENAI_MODEL" envDefault:"gpt-4"`
	GoogleAPIKey         string  `env:"GOOGLE_API_KEY"`
	GoogleAPIModel       string  `env:"GOOGLE_API_MODEL" envDefault:"gemini-1.5-pro-latest"`
	MistralAPIKey        string  `env:"MISTRAL_API_KEY"`
	MistralModel         string  `env:"MISTRAL_MODEL" envDefault:"mistral-medium-latest"`
	GroqAPIKey           string  `env:"GROQ_API_KEY"`
	GroqModel            string  `env:"GROQ_MODEL" envDefault:"llama3-70b-8192"`
	GroqEndpoint         string  `env:"GROQ_ENDPOINT" envDefault:"https://api.groq.com/openai/v1"`
	AWSBedrockRegionName string  `env:"AWS_BEDROCK_REGION_NAME" envDefault:"us-west-2"`
	AWSBedrockModelID    string  `env:"AWS_BEDROCK_MODEL_ID" envDefault:"anthropic.claude-3-sonnet-20240229-v1:0"`
	AWSAccessKeyID       string  `env:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey   string  `env:"AWS_SECRET_ACCESS_KEY"`
}

func (s Settings) IsOpenAIEnabled() bool {
	return s.OpenAIAPIKey != "" && s.OpenAIModel != "" && s.OpenAIBaseURL != ""
}

func (s Settings) IsGoogleEnabled() bool {
	return s.GoogleAPIKey != "" && s.GoogleAPIModel != ""
}

func (s Settings) IsMistralEnabled() bool {
	return s.MistralAPIKey != "" && s.MistralModel != ""
}

func (s Settings) IsGroqEnabled() bool {
	return s.GroqAPIKey != "" && s.GroqModel != ""
}

func (s Settings) IsBedrockEnabled() bool {
	return s.AWSAccessKeyID != "" && s.AWSSecretAccessKey != "" && s.AWSBedrockModelID != "" && s.AWSBedrockRegionName != ""
}

func (s Settings) HasVision() bool {
	return s.IsGoogleEnabled() || s.IsBedrockEnabled()
}
func Load() Settings {
	var settings Settings
	if err := env.Parse(&settings); err != nil {
		panic(err)
	}
	return settings
}
