package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIComplete(t *testing.T) {
	var gotAuth, gotModel, gotPath string
	var gotRoles []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotModel = req.Model
		for _, m := range req.Messages {
			gotRoles = append(gotRoles, m.Role)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":" echo hi "}}]}`))
	}))
	defer srv.Close()

	orig := backoff
	backoff = func(int) time.Duration { return time.Millisecond }
	defer func() { backoff = orig }()

	c := NewOpenAI(Options{BaseURL: srv.URL, APIKey: "k", Model: "m1"})
	out, err := c.Complete(context.Background(), []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "go"},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if out != "echo hi" {
		t.Errorf("Complete() = %q, want echo hi", out)
	}
	if gotAuth != "Bearer k" || gotModel != "m1" || gotPath != "/chat/completions" {
		t.Errorf("request shape wrong: auth=%q model=%q path=%q", gotAuth, gotModel, gotPath)
	}
	if strings.Join(gotRoles, ",") != "system,user" {
		t.Errorf("roles = %v, want [system user]", gotRoles)
	}
}

func TestRetryOnServerError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"flaky"}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	orig := backoff
	backoff = func(int) time.Duration { return time.Millisecond }
	defer func() { backoff = orig }()

	c := NewOpenAI(Options{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	out, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if out != "ok" || calls != 2 {
		t.Errorf("out=%q calls=%d, want ok after retry", out, calls)
	}
}

func TestNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`bad key`))
	}))
	defer srv.Close()

	orig := backoff
	backoff = func(int) time.Duration { return time.Millisecond }
	defer func() { backoff = orig }()

	c := NewOpenAI(Options{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("Complete() succeeded, want 401 error")
	}
}

func TestAnthropicMovesSystemField(t *testing.T) {
	var body anthropicRequest
	var sysHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sysHeader = r.Header.Get("anthropic-version")
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"content":[{"type":"text","text":"hello"},{"type":"tool_use"}]}`))
	}))
	defer srv.Close()

	orig := backoff
	backoff = func(int) time.Duration { return time.Millisecond }
	defer func() { backoff = orig }()

	c := NewAnthropic(Options{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	out, err := c.Complete(context.Background(), []Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if out != "hello" {
		t.Errorf("Complete() = %q, want hello", out)
	}
	if body.System != "be brief" || len(body.Messages) != 1 || body.Messages[0].Role != "user" {
		t.Errorf("system/messages not split correctly: %+v", body)
	}
	if sysHeader != "2023-06-01" {
		t.Errorf("anthropic-version header = %q", sysHeader)
	}
}

func TestNewProviderValidation(t *testing.T) {
	if _, err := New(ProviderOpenAI, Options{}); err == nil {
		t.Error("openai without key: want error")
	}
	if _, err := New("bogus", Options{}); err == nil {
		t.Error("unknown provider: want error")
	}
	if _, err := New(ProviderOllama, Options{BaseURL: "http://x"}); err != nil {
		t.Errorf("ollama without key should work: %v", err)
	}
}
