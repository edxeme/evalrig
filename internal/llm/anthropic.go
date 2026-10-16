package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Anthropic talks to the Anthropic messages API.
type Anthropic struct {
	opts Options
	hc   *http.Client
}

// NewAnthropic builds an Anthropic client.
func NewAnthropic(opts Options) *Anthropic {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultAnthropicBaseURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	return &Anthropic{opts: opts, hc: &http.Client{Timeout: opts.Timeout}}
}

type anthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// Complete implements Client. A leading system message is moved to the
// dedicated system field, as the API requires.
func (a *Anthropic) Complete(ctx context.Context, msgs []Message) (string, error) {
	req := anthropicRequest{Model: a.opts.Model, MaxTokens: 4096}
	rest := msgs
	if len(msgs) > 0 && msgs[0].Role == "system" {
		req.System = msgs[0].Content
		rest = msgs[1:]
	}
	req.Messages = rest

	raw, err := retry(ctx, 3, func(ctx context.Context) (string, error) {
		return postJSON(ctx, a.hc, a.opts.BaseURL+"/v1/messages", "",
			map[string]string{"x-api-key": a.opts.APIKey, "anthropic-version": "2023-06-01"}, req)
	})
	if err != nil {
		return "", err
	}
	var r anthropicResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}
	var b strings.Builder
	for _, block := range r.Content {
		if block.Type == "text" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(block.Text)
		}
	}
	return strings.TrimSpace(b.String()), nil
}
