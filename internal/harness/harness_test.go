package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edxeme/evalrig/internal/agent"
	"github.com/edxeme/evalrig/internal/config"
	"github.com/edxeme/evalrig/internal/runtime"
)

type fakeRuntime struct {
	started   atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
	failStart bool
	ExecFn    func(cmd string) (string, int, error) // optional scripted exec

	mu      sync.Mutex
	copied  []string
	execLog []string
}

func (f *fakeRuntime) Start(ctx context.Context, image string) (string, error) {
	if f.failStart {
		return "", fmt.Errorf("daemon refuses")
	}
	n := f.started.Add(1)
	active := f.active.Add(1)
	for {
		max := f.maxActive.Load()
		if active <= max || f.maxActive.CompareAndSwap(max, active) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	f.active.Add(-1)
	return fmt.Sprintf("c%d", n), nil
}

func (f *fakeRuntime) CopyDir(ctx context.Context, id, src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = append(f.copied, src+" -> "+dst)
	return nil
}

func (f *fakeRuntime) Exec(ctx context.Context, id, cmd string) (runtime.ExecResult, error) {
	f.mu.Lock()
	f.execLog = append(f.execLog, cmd)
	f.mu.Unlock()
	if f.ExecFn != nil {
		out, code, err := f.ExecFn(cmd)
		return runtime.ExecResult{Output: out, ExitCode: code}, err
	}
	switch {
	case strings.Contains(cmd, "solution.sh"):
		return runtime.ExecResult{Output: "solution ran"}, nil
	case strings.Contains(cmd, "verify.sh"):
		return runtime.ExecResult{Output: "verified"}, nil
	}
	return runtime.ExecResult{Output: "ok"}, nil
}

func (f *fakeRuntime) Stop(ctx context.Context, id string) error { return nil }
func (f *fakeRuntime) Close() error                              { return nil }

func taskOnDisk(t *testing.T, root, id string) {
	t.Helper()
	files := map[string]string{
		filepath.Join("tasks", id, "prompt.md"):      "do the thing",
		filepath.Join("tasks", id, "solution.sh"):    "echo solved",
		filepath.Join("tasks", id, "verify.sh"):      "exit 0",
		filepath.Join("tasks", id, "files", "a.txt"): "payload",
	}
	for p, c := range files {
		path := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testSuite(t *testing.T, ids ...string) *config.Suite {
	t.Helper()
	root := t.TempDir()
	s := &config.Suite{Name: "test-suite", Dir: root}
	for _, id := range ids {
		taskOnDisk(t, root, id)
		s.Tasks = append(s.Tasks, config.Task{
			ID: id, Image: "alpine:3.20", Dir: "tasks/" + id,
			Files: "files", Prompt: "prompt.md", Golden: "solution.sh",
			Verify: []string{"verify.sh"},
		})
	}
	return s
}

func TestRunGradesGoldenPass(t *testing.T) {
	rt := &fakeRuntime{}
	h := &Harness{Runtime: rt, Concurrency: 2, OutDir: t.TempDir()}

	results := h.Run(context.Background(), testSuite(t, "t1"), []agent.Agent{agent.Golden{}})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	res := results[0]
	if !res.Pass || res.Error != "" {
		t.Errorf("result = %+v, want pass without error", res)
	}
	if res.Turns != 1 || res.Agent != "golden" || res.Task != "t1" {
		t.Errorf("result identity wrong: %+v", res)
	}
	if len(res.Checks) != 1 || !res.Checks[0].Pass {
		t.Errorf("checks = %+v, want one passing check", res.Checks)
	}
}

func TestRunGradesVerifierFailure(t *testing.T) {
	rt := &fakeRuntime{}
	// Make the verifier command fail.
	rt.ExecFn = func(cmd string) (string, int, error) {
		if strings.Contains(cmd, "verify.sh") {
			return "tests failed", 1, nil
		}
		return "ok", 0, nil
	}
	h := &Harness{Runtime: rt, Concurrency: 1, OutDir: t.TempDir()}
	suite := testSuite(t, "t1")

	results := h.Run(context.Background(), suite, []agent.Agent{agent.Golden{}})
	if results[0].Pass || results[0].Error != "verification failed" {
		t.Errorf("result = %+v, want fail with verification failed", results[0])
	}
}

func TestRunReportsStartFailure(t *testing.T) {
	rt := &fakeRuntime{failStart: true}
	h := &Harness{Runtime: rt, OutDir: t.TempDir()}

	results := h.Run(context.Background(), testSuite(t, "t1"), []agent.Agent{agent.Golden{}})
	if results[0].Pass || !strings.Contains(results[0].Error, "daemon refuses") {
		t.Errorf("result = %+v, want start error", results[0])
	}
}

func TestRunBoundsConcurrency(t *testing.T) {
	rt := &fakeRuntime{}
	h := &Harness{Runtime: rt, Concurrency: 2, OutDir: t.TempDir()}

	results := h.Run(context.Background(), testSuite(t, "t1", "t2", "t3", "t4", "t5", "t6"), []agent.Agent{agent.Golden{}})
	if len(results) != 6 {
		t.Fatalf("len(results) = %d, want 6", len(results))
	}
	max := rt.maxActive.Load()
	if max != 2 {
		t.Errorf("max concurrent containers = %d, want 2", max)
	}
	if rt.started.Load() != 6 {
		t.Errorf("containers started = %d, want 6", rt.started.Load())
	}
}

func TestRunStagesBundleAndFiles(t *testing.T) {
	rt := &fakeRuntime{}
	h := &Harness{Runtime: rt, OutDir: t.TempDir()}

	h.Run(context.Background(), testSuite(t, "t1"), []agent.Agent{agent.Golden{}})
	rt.mu.Lock()
	defer rt.mu.Unlock()
	joined := strings.Join(rt.copied, "\n")
	if !strings.Contains(joined, ".evalrig -> /task/.evalrig") || !strings.Contains(joined, "files -> /task") {
		t.Errorf("copied = %s, want bundle and files uploads", joined)
	}
	if len(rt.execLog) == 0 || rt.execLog[0] != "mkdir -p /task/.evalrig" {
		t.Errorf("execLog = %v, want mkdir -p /task/.evalrig first", rt.execLog)
	}
}

func TestRunWritesTrajectory(t *testing.T) {
	out := t.TempDir()
	h := &Harness{Runtime: &fakeRuntime{}, OutDir: out}
	h.Run(context.Background(), testSuite(t, "t1"), []agent.Agent{agent.Golden{}})

	matches, err := filepath.Glob(filepath.Join(out, "traj", "*.jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("trajectory files = %v, %v; want exactly one", matches, err)
	}
	raw, _ := os.ReadFile(matches[0])
	if !strings.Contains(string(raw), `"task_start"`) || !strings.Contains(string(raw), `"verify_result"`) {
		t.Errorf("trajectory = %s, want task_start and verify_result events", raw)
	}
}

func TestGoldenSolutionFailureIsReported(t *testing.T) {
	rt := &fakeRuntime{}
	// Make every exec of solution.sh fail.
	rt.ExecFn = func(cmd string) (string, int, error) { return "boom", 3, nil }
	h := &Harness{Runtime: rt, OutDir: t.TempDir()}

	results := h.Run(context.Background(), testSuite(t, "t1"), []agent.Agent{agent.Golden{}})
	if results[0].Pass || !strings.Contains(results[0].Error, "exited 3") {
		t.Errorf("result = %+v, want golden solution failure", results[0])
	}
}
