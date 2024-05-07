package config

import (
	"encoding/json"
	"errors"
)

type openai struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"` // optional, default: "https://api.openai.com/v1"
	Model   string `json:"model"`    // optional, default: "gpt-4"
}

var _ json.Unmarshaler = (*openai)(nil)

func (o *openai) UnmarshalJSON(data []byte) error {
	type Alias openai
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(o),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if o.Enabled {
		if o.APIKey == "" {
			return errors.New("openai api_key is required")
		}

		if o.BaseURL == "" {
			o.BaseURL = "https://api.openai.com/v1"
		}

		if o.Model == "" {
			o.Model = "gpt-4"
		}
	}

	return nil
}

type google struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"` // optional, default: "gemini-1.5-pro-latest"
}

var _ json.Unmarshaler = (*google)(nil)

func (g *google) UnmarshalJSON(data []byte) error {
	type Alias google
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(g),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if g.Enabled {
		if g.APIKey == "" {
			return errors.New("google api_key is required")
		}

		if g.Model == "" {
			g.Model = "gemini-1.5-pro-latest"
		}
	}

	return nil
}

type mistral struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"` // optional, default: "mistral-medium-latest"
}

var _ json.Unmarshaler = (*mistral)(nil)

func (m *mistral) UnmarshalJSON(data []byte) error {
	type Alias mistral
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if m.Enabled {
		if m.APIKey == "" {
			return errors.New("mistral api_key is required")
		}

		if m.Model == "" {
			m.Model = "mistral-medium-latest"
		}
	}

	return nil
}

type groq struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"` // optional, default: "https://api.groq.com/openai/v1"
	Model   string `json:"model"`    // optional, default: "llama3-70b-8192"
}

var _ json.Unmarshaler = (*groq)(nil)

func (g *groq) UnmarshalJSON(data []byte) error {
	type Alias groq
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(g),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if g.Enabled {
		if g.APIKey == "" {
			return errors.New("groq api_key is required")
		}

		if g.BaseURL == "" {
			g.BaseURL = "https://api.groq.com/openai/v1"
		}

		if g.Model == "" {
			g.Model = "llama3-70b-8192"
		}
	}

	return nil
}

type bedrock struct {
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	ModelID         string `json:"model_id"`    // optional, default: "anthropic.claude-3-sonnet-20240229-v1:0"
	RegionName      string `json:"region_name"` // optional, default: "us-west-2"
}

var _ json.Unmarshaler = (*bedrock)(nil)

func (b *bedrock) UnmarshalJSON(data []byte) error {
	type Alias bedrock
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(b),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if b.Enabled {
		if b.AccessKeyID == "" {
			return errors.New("bedrock access_key_id is required")
		}

		if b.SecretAccessKey == "" {
			return errors.New("bedrock secret_access_key is required")
		}

		if b.ModelID == "" {
			b.ModelID = "anthropic.claude-3-sonnet-20240229-v1:0"
		}

		if b.RegionName == "" {
			b.RegionName = "us-west-2"
		}
	}

	return nil
}

type azure struct {
	Enabled    bool   `json:"enabled"`
	APIKey     string `json:"api_key"`
	APIVersion string `json:"api_version"` // optional, default: "2024-02-01"
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"` // optional, default: "gpt-4"
}

var _ json.Unmarshaler = (*azure)(nil)

func (a *azure) UnmarshalJSON(data []byte) error {
	type Alias azure
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if a.Enabled {
		if a.APIKey == "" {
			return errors.New("azure api_key is required")
		}

		if a.APIVersion == "" {
			a.APIVersion = "2024-02-01"
		}

		if a.BaseURL == "" {
			return errors.New("azure base_url is required")
		}

		if a.Model == "" {
			a.Model = "gpt-4"
		}
	}

	return nil
}
