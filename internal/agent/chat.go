package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/edxeme/evalrig/internal/llm"
)

// DefaultMaxTurns bounds the chat loop when none is configured.
const DefaultMaxTurns = 8

const systemPrompt = `You are an expert software engineer working inside a Linux container.
Your working directory is /task, which already contains the task files.

Solve the task using the shell. Every reply must contain EXACTLY ONE fenced bash block with a single shell command, like:

` + "```bash" + `
git status
` + "```" + `

The command's exit status and combined output are returned to you on the next turn.
When you are confident the task is complete, reply with just: DONE`

// Chat drives a coding agent through a plain chat-completion loop: the
// model proposes a shell command, evalrig executes it in the container
// and feeds the output back, until the model signals completion or the
// turn budget runs out.
type Chat struct {
	client   llm.Client
	model    string
	maxTurns int
}

// NewChat builds a chat agent. maxTurns <= 0 means DefaultMaxTurns.
func NewChat(client llm.Client, model string, maxTurns int) *Chat {
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}
	return &Chat{client: client, model: model, maxTurns: maxTurns}
}

// Name implements Agent.
func (c *Chat) Name() string { return "chat/" + c.model }

// Run implements Agent.
func (c *Chat) Run(ctx context.Context, prompt string, sess Session, events Events) (Summary, error) {
	msgs := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}
	for turn := 1; turn <= c.maxTurns; turn++ {
		events.Emit("llm_request", map[string]any{"model": c.model, "turn": turn})
		resp, err := c.client.Complete(ctx, msgs)
		if err != nil {
			return Summary{Turns: turn}, fmt.Errorf("chat: turn %d: %w", turn, err)
		}
		events.Emit("llm_response", map[string]any{"turn": turn, "response": resp})

		cmd, done := parseCommand(resp)
		if done {
			return Summary{Turns: turn, LastOutput: resp}, nil
		}
		if cmd == "" {
			return Summary{Turns: turn}, fmt.Errorf("chat: turn %d: no shell command in model response", turn)
		}

		out, code, err := sess.Exec(ctx, cmd)
		if err != nil {
			return Summary{Turns: turn}, fmt.Errorf("chat: turn %d: %w", turn, err)
		}
		msgs = append(msgs,
			llm.Message{Role: "assistant", Content: resp},
			llm.Message{Role: "user", Content: fmt.Sprintf("exit status: %d\n\n%s", code, out)},
		)
	}
	return Summary{Turns: c.maxTurns}, fmt.Errorf("chat: max turns (%d) reached", c.maxTurns)
}

var (
	fenceRe = regexp.MustCompile("(?s)```(?:bash|sh|shell)?\\s*\n(.*?)```")
	doneRe  = regexp.MustCompile(`(?i)^\W*(done|finished|complete)\W*$`)
)

// parseCommand extracts the shell command to run. done reports whether
// the model signalled completion instead.
func parseCommand(resp string) (cmd string, done bool) {
	if m := fenceRe.FindStringSubmatch(resp); m != nil {
		return strings.TrimSpace(m[1]), false
	}
	if doneRe.MatchString(strings.TrimSpace(resp)) {
		return "", true
	}
	return "", false
}
