package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenAI talks to the OpenAI-compatible chat completions API. The same
// client serves Ollama, vLLM and any server exposing /chat/completions
// when Options.BaseURL is overridden.
type OpenAI struct {
	opts Options
	hc   *http.Client
}

// NewOpenAI builds an OpenAI-compatible client.
func NewOpenAI(opts Options) *OpenAI {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultOpenAIBaseURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	return &OpenAI{opts: opts, hc: &http.Client{Timeout: opts.Timeout}}
}

type openaiRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type openaiResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// Complete implements Client.
func (o *OpenAI) Complete(ctx context.Context, msgs []Message) (string, error) {
	raw, err := retry(ctx, 3, func(ctx context.Context) (string, error) {
		return postJSON(ctx, o.hc, o.opts.BaseURL+"/chat/completions",
			"Bearer "+o.opts.APIKey, nil,
			openaiRequest{Model: o.opts.Model, Messages: msgs})
	})
	if err != nil {
		return "", err
	}
	var r openaiResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("openai response has no choices")
	}
	return strings.TrimSpace(r.Choices[0].Message.Content), nil
}
