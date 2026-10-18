package agent

import (
	"context"
	"fmt"
)

// Golden runs the task's reference solution. It doubles as the oracle
// that keeps suites honest: if the golden solution does not verify, the
// task is broken, not the model.
type Golden struct{}

// Name implements Agent.
func (Golden) Name() string { return "golden" }

// Run implements Agent.
func (Golden) Run(ctx context.Context, prompt string, sess Session, events Events) (Summary, error) {
	out, code, err := sess.Exec(ctx, "sh /task/.evalrig/solution.sh")
	if err != nil {
		return Summary{}, fmt.Errorf("golden: %w", err)
	}
	if code != 0 {
		return Summary{Turns: 1, LastOutput: out}, fmt.Errorf("golden: solution script exited %d:\n%s", code, out)
	}
	return Summary{Turns: 1, LastOutput: out}, nil
}
