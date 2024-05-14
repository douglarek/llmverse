package config

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

type model = string

var (
	OpenAI   model = "openai"
	Google   model = "google"
	Mistral  model = "mistral"
	Groq     model = "groq"
	Bedrock  model = "bedrock"
	Azure    model = "azure"
	Deepseek model = "deepseek"
	ChatGLM  model = "chatglm"
)

type Settings struct {
	DiscordBotToken string   `json:"discord_bot_token"`
	EnableDebug     bool     `json:"enable_debug"`
	HistoryMaxSize  *int     `json:"history_max_size"`
	OutputMaxSize   *int     `json:"output_max_size"`
	SystemPrompt    string   `json:"system_prompt"`
	Temperature     *float64 `json:"temperature"`
	Models          []struct {
		Name             string `json:"name,omitempty"`
		APIKey           string `json:"api_key,omitempty"`
		APIVersion       string `json:"api_version,omitempty"`
		Enabled          bool   `json:"enabled"`
		Model            string `json:"model,omitempty"`
		BaseURL          string `json:"base_url,omitempty"`
		AccessKeyID      string `json:"access_key_id,omitempty"`
		ModelID          string `json:"model_id,omitempty"`
		RegionName       string `json:"region_name,omitempty"`
		SecretAccessKey  string `json:"secret_access_key,omitempty"`
		HasVisionSupport bool   `json:"has_vision_support,omitempty"`
		HasToolSupport   bool   `json:"has_tool_support,omitempty"`
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

	for i, v := range s.Models {
		if v.Enabled {
			switch v.Name {
			case OpenAI:
				if v.APIKey == "" {
					return errors.New("openai api_key is required")
				}
				if v.BaseURL == "" {
					s.Models[i].BaseURL = "https://api.openai.com/v1"
				}
				if v.Model == "" {
					s.Models[i].Model = "gpt-4"
				}
			case Google:
				if v.APIKey == "" {
					return errors.New("google api_key is required")
				}
				if v.Model == "" {
					s.Models[i].Model = "gemini-1.5-pro-latest"
				}
			case Mistral:
				if v.APIKey == "" {
					return errors.New("mistral api_key is required")
				}
				if v.Model == "" {
					s.Models[i].Model = "mistral-large-latest"
				}
			case Groq:
				if v.APIKey == "" {
					return errors.New("groq api_key is required")
				}
				if v.BaseURL == "" {
					s.Models[i].BaseURL = "https://api.groq.com/openai/v1"
				}
				if v.Model == "" {
					s.Models[i].Model = "llama3-70b-8192"
				}
			case Bedrock:
				if v.AccessKeyID == "" {
					return errors.New("bedrock access_key_id is required")
				}
				if v.SecretAccessKey == "" {
					return errors.New("bedrock secret_access_key is required")
				}
				if v.ModelID == "" {
					s.Models[i].ModelID = "anthropic.claude-3-sonnet-20240229-v1:0"
				}
				if v.RegionName == "" {
					s.Models[i].RegionName = "us-west-2"
				}
			case Azure:
				if v.APIKey == "" {
					return errors.New("azure api_key is required")
				}
				if v.APIVersion == "" {
					s.Models[i].APIVersion = "2024-02-01"
				}
				if v.BaseURL == "" {
					return errors.New("azure base_url is required")
				}
				if v.Model == "" {
					s.Models[i].Model = "gpt-4"
				}
			case Deepseek:
				if v.APIKey == "" {
					return errors.New("deepseek api_key is required")
				}
				if v.BaseURL == "" {
					s.Models[i].BaseURL = "https://api.deepseek.com/v1"
				}
				if v.Model == "" {
					s.Models[i].Model = "deepseek-chat"
				}
			case ChatGLM:
				if v.APIKey == "" {
					return errors.New("chatglm api_key is required")
				}
				if v.BaseURL == "" {
					s.Models[i].BaseURL = "https://open.bigmodel.cn/api/paas/v4"
				}
				if v.Model == "" {
					s.Models[i].Model = "glm-3-turbo"
				}
			default:
				return errors.New("unknown model name " + v.Name)
			}
		}
	}

	return nil
}

func (s Settings) GetModelName(input string) string {
	index := strings.Index(input, ":")
	if index == -1 {
		return ""
	}

	name := input[:index]
	for _, v := range s.Models {
		if v.Name == name {
			return v.Name
		}
	}

	return ""
}

func (s Settings) GetVisionSupport(name string) bool {
	for _, v := range s.Models {
		if v.Name == name {
			return v.HasVisionSupport
		}
	}
	return false
}

func (s Settings) GetAvailableModels() string {
	var models []string
	for _, v := range s.Models {
		if v.Enabled {
			models = append(models, "`"+v.Name+"`")
		}
	}
	return strings.Join(models, ", ")
}

func (s Settings) GetToolSupport(name string) bool {
	for _, v := range s.Models {
		if v.Name == name {
			return v.HasToolSupport
		}
	}
	return false
}

func ParseModelName(input string) string {
	index := strings.Index(input, ":")
	if index == -1 {
		return ""
	}
	return input[:index]
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
