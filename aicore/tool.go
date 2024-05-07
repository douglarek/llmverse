package aicore

import (
	"context"
	"io"
	"net/http"

	"github.com/tmc/langchaingo/llms"
)

var availableTools = []llms.Tool{
	{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "getExchangeRate",
			Description: "Get the exchange rate for currencies between countries",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"currency_date": map[string]any{
						"type":        "string",
						"description": "A date that must always be in YYYY-MM-DD format or the value 'latest' if a time period is not specified",
					},
					"currency_from": map[string]any{
						"type":        "string",
						"description": "The currency to convert from in ISO 4217 format",
					},
					"currency_to": map[string]any{
						"type":        "string",
						"description": "The currency to convert to in ISO 4217 format",
					},
				},
				"required": []string{"currency_from", "currency_date"},
			},
		},
	},
}

func getExchangeRate(ctx context.Context, currencyDate string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.frankfurter.app/"+currencyDate, nil)
	if err != nil {
		return nil, err
	}
	req.Close = true

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
