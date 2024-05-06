package config

import (
	"encoding/json"
	"testing"
)

func TestConfig_UnmarshalJSON(t *testing.T) {
	var c Settings

	s := `
	{
		"discord_bot_token": "xxxx",
		"models": {
			"aws_bedrock": {
				"access_key_id": "",
				"enabled": false,
				"model_id": "anthropic.claude-3-sonnet-20240229-v1:0",
				"region_name": "us-west-2",
				"secret_access_key": ""
			},
			"google": {
				"api_key": "xxxx",
				"enabled": true,
				"model": "gemini-1.5-pro-latest"
			},
			"groq": {
				"api_key": "",
				"base_url": "https://api.groq.com/openai/v1",
				"enabled": false,
				"model": "llama3-70b-8192"
			},
			"mistral": {
				"api_key": "",
				"enabled": false,
				"model": "mistral-medium-latest"
			},
			"openai": {
				"api_key": "",
				"base_url": "https://api.openai.com/v1",
				"enabled": false,
				"model": "gpt-4"
			}
		}
	}
`
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatal(err)
	}

	t.Logf("discord_bot_token: %s, history_max_size: %d, system_prompt: %s, temperature: %.1f", c.DiscordBotToken, *c.HistoryMaxSize, c.SystemPrompt, *c.Temperature)
}
