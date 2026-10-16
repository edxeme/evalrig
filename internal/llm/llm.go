// Package llm provides minimal chat-completion clients for the
// providers evalrig can drive an agent loop with.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Supported providers.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderOllama    = "ollama"
)

// Default endpoints and models.
const (
	DefaultOpenAIBaseURL    = "https://api.openai.com/v1"
	DefaultAnthropicBaseURL = "https://api.anthropic.com"
	DefaultOllamaBaseURL    = "http://localhost:11434/v1"

	DefaultOpenAIModel    = "gpt-4o"
	DefaultAnthropicModel = "claude-sonnet-4-5"
	DefaultOllamaModel    = "qwen2.5-coder:7b"
)

// DefaultModel returns the fallback model for provider.
func DefaultModel(provider string) string {
	switch provider {
	case ProviderAnthropic:
		return DefaultAnthropicModel
	case ProviderOllama:
		return DefaultOllamaModel
	default:
		return DefaultOpenAIModel
	}
}

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Options configures a provider client.
type Options struct {
	BaseURL string        // API root, no trailing slash
	APIKey  string        // not needed for Ollama
	Model   string        // provider model identifier
	Timeout time.Duration // per request; 0 means the default
}

// Client performs a full chat completion for a conversation.
type Client interface {
	Complete(ctx context.Context, msgs []Message) (string, error)
}

// New builds the client for the given provider.
func New(provider string, opts Options) (Client, error) {
	switch provider {
	case ProviderOpenAI:
		if opts.APIKey == "" {
			return nil, fmt.Errorf("openai: set OPENAI_API_KEY or choose another provider")
		}
		return NewOpenAI(opts), nil
	case ProviderAnthropic:
		if opts.APIKey == "" {
			return nil, fmt.Errorf("anthropic: set ANTHROPIC_API_KEY or choose another provider")
		}
		return NewAnthropic(opts), nil
	case ProviderOllama:
		if opts.BaseURL == "" {
			opts.BaseURL = DefaultOllamaBaseURL
		}
		return NewOpenAI(opts), nil // Ollama serves the OpenAI-compatible API
	default:
		return nil, fmt.Errorf("unknown provider %q (want openai, anthropic or ollama)", provider)
	}
}

// postJSON sends body as JSON and returns the raw response body.
// Non-2xx status codes become errors carrying the response snippet.
func postJSON(ctx context.Context, hc *http.Client, url, auth string, headers map[string]string, body any) (string, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s %s: status %d: %s", req.Method, url, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return string(raw), nil
}

// backoff is a variable so tests can collapse retry delays.
var backoff = func(attempt int) time.Duration {
	return time.Duration(500*(1<<(attempt-1))) * time.Millisecond
}

// retry runs fn up to attempts times, waiting between transient failures.
func retry(ctx context.Context, attempts int, fn func(ctx context.Context) (string, error)) (string, error) {
	var last error
	for i := 1; i <= attempts; i++ {
		if i > 1 {
			select {
			case <-time.After(backoff(i - 1)):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		out, err := fn(ctx)
		if err == nil {
			return out, nil
		}
		last = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("giving up after %d attempts: %w", attempts, last)
}
