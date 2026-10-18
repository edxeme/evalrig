// Package agent defines the agents that can attempt a task inside a
// container session.
package agent

import "context"

// Session is the interface an agent gets to act inside a task container.
type Session interface {
	// Exec runs cmd with /task as working directory. A non-zero exit
	// status is returned in exitCode; error is reserved for
	// infrastructure failures.
	Exec(ctx context.Context, cmd string) (output string, exitCode int, err error)
}

// Events receives the structured events emitted while an agent works.
// Implementations must be safe for use from the agent goroutine.
type Events interface {
	Emit(event string, data any)
}

// NoopEvents discards events; useful in tests.
type NoopEvents struct{}

// Emit implements Events.
func (NoopEvents) Emit(string, any) {}

// Summary describes how an attempt went, independent of grading.
type Summary struct {
	Turns      int    `json:"turns"`
	LastOutput string `json:"last_output,omitempty"`
}

// Agent attempts the task described by prompt inside sess. Passing or
// failing is decided later, by verifiers, never by the agent's exit
// status.
type Agent interface {
	Name() string
	Run(ctx context.Context, prompt string, sess Session, events Events) (Summary, error)
}
