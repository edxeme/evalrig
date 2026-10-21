package verify

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type fakeExecer struct {
	bySubstr map[string]int // cmd substring -> exit code
	failOn   string         // cmd substring -> infrastructure error
	calls    []string
}

func (f *fakeExecer) Exec(ctx context.Context, cmd string) (string, int, error) {
	f.calls = append(f.calls, cmd)
	if f.failOn != "" && strings.Contains(cmd, f.failOn) {
		return "", 0, fmt.Errorf("daemon gone")
	}
	for sub, code := range f.bySubstr {
		if strings.Contains(cmd, sub) {
			return fmt.Sprintf("out(%s)", cmd), code, nil
		}
	}
	return "ok", 0, nil
}

func TestRunCollectsAllChecks(t *testing.T) {
	ex := &fakeExecer{bySubstr: map[string]int{"first": 0, "second": 3}}
	checks := Run(context.Background(), ex, []string{"run first", "run second"})
	if len(checks) != 2 {
		t.Fatalf("len(checks) = %d, want 2", len(checks))
	}
	if !checks[0].Pass || checks[0].ExitCode != 0 {
		t.Errorf("check 0 = %+v, want pass", checks[0])
	}
	if checks[1].Pass || checks[1].ExitCode != 3 {
		t.Errorf("check 1 = %+v, want fail with exit 3", checks[1])
	}
	if !AllPass(checks[:1]) || AllPass(checks) {
		t.Error("AllPass is wrong")
	}
}

func TestRunStopsOnInfrastructureError(t *testing.T) {
	ex := &fakeExecer{failOn: "first"}
	checks := Run(context.Background(), ex, []string{"run first", "run second"})
	if len(checks) != 1 {
		t.Fatalf("len(checks) = %d, want 1 (stopped)", len(checks))
	}
	if len(ex.calls) != 1 {
		t.Errorf("calls = %v, want single call", ex.calls)
	}
	if !strings.Contains(checks[0].Output, "daemon gone") {
		t.Errorf("output = %q, want infrastructure error", checks[0].Output)
	}
}

func TestAllPassEmpty(t *testing.T) {
	if AllPass(nil) {
		t.Error("empty checks must not pass")
	}
}
