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
		"enable_debug": false,
		"history_max_size": 2048,
		"output_max_size": 4096,
		"system_prompt": "You are a helpful AI assistant.",
		"temperature": 0.7,
		"models": [
			{
				"name": "bedrock",
				"access_key_id": "",
				"enabled": false,
				"model_id": "anthropic.claude-3-sonnet-20240229-v1:0",
				"region_name": "us-west-2",
				"secret_access_key": ""
			},
			{
				"name": "google",
				"api_key": "",
				"enabled": false,
				"model": "gemini-1.5-pro-latest"
			},
			{
				"name": "groq",
				"api_key": "",
				"base_url": "https://api.groq.com/openai/v1",
				"enabled": false,
				"model": "llama3-70b-8192"
			},
			{
				"name": "mistral",
				"api_key": "",
				"enabled": false,
				"model": "mistral-large-latest"
			},
			{
				"name": "openai",
				"api_key": "",
				"base_url": "https://api.openai.com/v1",
				"enabled": false,
				"model": "gpt-4"
			},
			{
				"name": "azure",
				"api_key": "",
				"base_url": "",
				"enabled": false,
				"model": "gpt-4"
			},
			{
				"name": "deepseek",
				"api_key": "",
				"enabled": false,
				"base_url": "https://api.deepseek.com/v1",
				"model": "deepseek-chat"
			},
			{
				"name": "qwen",
				"api_key": "",
				"enabled": false,
				"base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
				"model": "qwen1.5-110b-chat"
			},
			{
				"name": "chatglm",
				"api_key": "xxx",
				"enabled": true,
				"base_url": "https://open.bigmodel.cn/api/paas/v4",
				"model": "glm-3-turbo"
			}
		]
	}
`
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatal(err)
	}

	t.Logf("discord_bot_token: %s, history_max_size: %d, system_prompt: %s, temperature: %.1f", c.DiscordBotToken, *c.HistoryMaxSize, c.SystemPrompt, *c.Temperature)
}
