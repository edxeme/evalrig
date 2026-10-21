// Package verify grades a finished attempt with command-based
// verifiers. Exit status is the only judge: output is evidence, not
// signal.
package verify

import (
	"context"
	"fmt"
)

// Execer is the minimal container access verifiers need.
type Execer interface {
	Exec(ctx context.Context, cmd string) (output string, exitCode int, err error)
}

// Check is the outcome of a single verifier command.
type Check struct {
	Cmd      string `json:"cmd"`
	Pass     bool   `json:"pass"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output,omitempty"`
}

// Run executes every verifier command in order and collects the
// outcomes. An infrastructure error stops the sequence: without a
// container there is nothing left to verify.
func Run(ctx context.Context, ex Execer, cmds []string) []Check {
	checks := make([]Check, 0, len(cmds))
	for _, cmd := range cmds {
		c := Check{Cmd: cmd}
		out, code, err := ex.Exec(ctx, cmd)
		if err != nil {
			c.Output = fmt.Sprintf("infrastructure error: %v", err)
			checks = append(checks, c)
			break
		}
		c.Output, c.ExitCode, c.Pass = out, code, code == 0
		checks = append(checks, c)
	}
	return checks
}

// AllPass reports whether every check passed. Zero checks never pass.
func AllPass(checks []Check) bool {
	if len(checks) == 0 {
		return false
	}
	for _, c := range checks {
		if !c.Pass {
			return false
		}
	}
	return true
}
