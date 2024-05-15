package aicore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/tmc/langchaingo/llms"
)

// availableTools is a list of tools that the agent can use to help answer questions.
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

// getExchangeRate is a helper function that makes a request to the Frankfurter API
// to get the exchange rate for currencies between countries.
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

// executeToolCalls is a helper function that parses the response from a tool call
// and returns the content to be sent to the user, whether the response should be
// returned directly to the user, and any error that occurred.
func executeToolCalls(ctx context.Context, model llms.Model, options []llms.CallOption, content []llms.MessageContent, output chan<- string) ([]llms.MessageContent, bool, error) { // content, return_direct, error
	var isStreaming bool
	options = append(options, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		isStreaming = true
		output <- parseToolCallStreamingChunk(chunk)
		return nil
	}))
	resp, err := model.GenerateContent(ctx, content, options...)
	if err != nil {
		return nil, false, err
	}

	respChoice := resp.Choices[0]
	ar := llms.TextParts(llms.ChatMessageTypeAI, respChoice.Content)
	if len(respChoice.ToolCalls) == 0 {
		content = append(content, ar)
		return content, true, nil
	}

	for _, tc := range respChoice.ToolCalls {
		ar.Parts = append(ar.Parts, tc)
	}
	content = append(content, ar)

	for _, tc := range respChoice.ToolCalls {
		switch tc.FunctionCall.Name {
		case "getExchangeRate":
			var args struct {
				CurrencyDate string `json:"currency_date"`
			}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
				return nil, false, err
			}
			rs, err := getExchangeRate(ctx, args.CurrencyDate)
			if err != nil {
				return nil, false, err
			}
			tr := llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       tc.FunctionCall.Name,
						Content:    string(rs),
					},
				},
			}
			content = append(content, tr)
			if isStreaming {
				output <- "`\n\n"
			}
		default:
			slog.Error("[LLMAgent.Query] unknown tool call", "name", tc.FunctionCall.Name)
		}
	}

	return content, false, nil
}

type toolCallStreamingChunk struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func parseToolCallStreamingChunk(chunk []byte) string {
	slog.Debug("[tool.parseToolCallStreamingChunk]", "chunk", string(chunk))

	var tc []toolCallStreamingChunk
	if err := json.Unmarshal(chunk, &tc); err != nil {
		goto R
	}

	if len(tc) > 0 {

		if tc[0].Function.Name != "" {
			res := fmt.Sprintf("*** Running tool: [%s] with arguments: *** `", tc[0].Function.Name)
			if tc[0].Function.Arguments != "" {
				res += tc[0].Function.Arguments
			}
			return res
		}
		if tc[0].Function.Arguments != "" {
			return tc[0].Function.Arguments
		}
	}

R:
	return string(chunk)
}
