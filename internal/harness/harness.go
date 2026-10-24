// Package harness orchestrates the evaluation: one fresh container per
// (task, agent) pair, bounded concurrency, verifiers as the only judge.
package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/edxeme/evalrig/internal/agent"
	"github.com/edxeme/evalrig/internal/config"
	"github.com/edxeme/evalrig/internal/runtime"
	"github.com/edxeme/evalrig/internal/trajectory"
	"github.com/edxeme/evalrig/internal/verify"
)

// Result is the graded outcome of one (task, agent) attempt.
type Result struct {
	Task       string         `json:"task"`
	Agent      string         `json:"agent"`
	Pass       bool           `json:"pass"`
	Turns      int            `json:"turns"`
	DurationMS int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Checks     []verify.Check `json:"checks,omitempty"`
}

// Harness runs suites against agents.
type Harness struct {
	Runtime     runtime.Runtime
	Concurrency int    // parallel (task, agent) pairs; 1 when <= 0
	OutDir      string // trajectories are written under OutDir/traj
}

type job struct {
	task  config.Task
	agent agent.Agent
	idx   int
}

// Run evaluates every task with every agent and returns results in a
// deterministic order: task-major, then agent order as given.
func (h *Harness) Run(ctx context.Context, suite *config.Suite, agents []agent.Agent) []Result {
	var jobs []job
	for _, t := range suite.Tasks {
		for _, a := range agents {
			jobs = append(jobs, job{task: t, agent: a, idx: len(jobs)})
		}
	}
	results := make([]Result, len(jobs))

	workers := h.Concurrency
	if workers <= 0 {
		workers = 1
	}
	ch := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				results[j.idx] = h.runPair(ctx, suite, j.task, j.agent)
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	return results
}

// runPair evaluates one task with one agent in a fresh container.
func (h *Harness) runPair(ctx context.Context, suite *config.Suite, t config.Task, a agent.Agent) Result {
	res := Result{Task: t.ID, Agent: a.Name()}
	start := time.Now()
	defer func() { res.DurationMS = time.Since(start).Milliseconds() }()

	timeout, err := suite.Timeout(t)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	rec, err := trajectory.New(filepath.Join(h.OutDir, "traj", t.ID+"-"+sanitize(a.Name())+".jsonl"))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer rec.Close()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt, err := suite.ReadPrompt(t)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	rec.Emit("task_start", map[string]any{"task": t.ID, "agent": a.Name(), "image": t.Image})

	cid, err := h.Runtime.Start(ctx, t.Image)
	if err != nil {
		res.Error = err.Error()
		rec.Emit("task_end", map[string]any{"status": "error", "error": res.Error})
		return res
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer stopCancel()
		_ = h.Runtime.Stop(stopCtx, cid)
	}()

	stage, cleanup, err := stageBundle(suite, t)
	if err != nil {
		res.Error = fmt.Sprintf("stage task bundle: %v", err)
		rec.Emit("task_end", map[string]any{"status": "error", "error": res.Error})
		return res
	}
	defer cleanup()

	// CopyToContainer requires the destination directory to exist.
	if _, err := h.Runtime.Exec(ctx, cid, "mkdir -p /task/.evalrig"); err != nil {
		res.Error = fmt.Sprintf("prepare container: %v", err)
		rec.Emit("task_end", map[string]any{"status": "error", "error": res.Error})
		return res
	}
	if err := h.Runtime.CopyDir(ctx, cid, filepath.Join(stage, ".evalrig"), "/task/.evalrig"); err != nil {
		res.Error = fmt.Sprintf("upload task bundle: %v", err)
		rec.Emit("task_end", map[string]any{"status": "error", "error": res.Error})
		return res
	}
	if t.Files != "" {
		if err := h.Runtime.CopyDir(ctx, cid, filepath.Join(stage, "files"), "/task"); err != nil {
			res.Error = fmt.Sprintf("upload task files: %v", err)
			rec.Emit("task_end", map[string]any{"status": "error", "error": res.Error})
			return res
		}
	}

	sess := &loggedSession{rt: h.Runtime, cid: cid, rec: rec}

	summary, err := a.Run(ctx, prompt, sess, rec)
	res.Turns = summary.Turns
	if err != nil {
		res.Error = err.Error()
		rec.Emit("task_end", map[string]any{"status": "error", "error": res.Error})
		return res
	}

	checks := verify.Run(ctx, sess, verifyCommands(t))
	res.Checks = checks
	rec.Emit("verify_result", map[string]any{"checks": checks})
	res.Pass = verify.AllPass(checks)
	if !res.Pass {
		res.Error = "verification failed"
	}
	rec.Emit("task_end", map[string]any{"status": status(res.Pass)})
	return res
}

// stageBundle assembles the payload uploaded to the container: the task
// bundle under /task/.evalrig (prompt, golden solution, verifiers) plus
// the task files at /task. The bundle is built in a temp directory so
// the agent never sees the solution unless it goes looking.
func stageBundle(suite *config.Suite, t config.Task) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "evalrig-stage-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmp) }
	bundle := filepath.Join(tmp, ".evalrig")

	taskDir := filepath.Join(suite.Dir, t.Dir)
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	for _, e := range entries {
		if t.Files != "" && (e.Name() == t.Files || e.Name() == filepath.Base(t.Files)) {
			continue
		}
		if e.Name() == ".evalrig" {
			continue
		}
		if err := copyInto(bundle, filepath.Join(taskDir, e.Name()), e.IsDir()); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	if t.Files != "" {
		if err := copyTree(filepath.Join(tmp, "files"), suite.FilesPath(t)); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return tmp, cleanup, nil
}

// verifyCommands maps suite verify entries to container commands:
// script file references are resolved under /task/.evalrig, inline
// commands run as-is.
func verifyCommands(t config.Task) []string {
	cmds := make([]string, len(t.Verify))
	for i, v := range t.Verify {
		if config.IsInlineCommand(v) {
			cmds[i] = v
		} else {
			cmds[i] = "sh /task/.evalrig/" + filepath.Base(v)
		}
	}
	return cmds
}

// loggedSession records every container command into the trajectory.
type loggedSession struct {
	rt  runtime.Runtime
	cid string
	rec *trajectory.Recorder
}

func (l *loggedSession) Exec(ctx context.Context, cmd string) (string, int, error) {
	l.rec.Emit("exec_command", map[string]any{"command": cmd})
	er, err := l.rt.Exec(ctx, l.cid, cmd)
	data := map[string]any{"exit_code": er.ExitCode, "output": truncate(er.Output, 8192)}
	if err != nil {
		data["error"] = err.Error()
	}
	l.rec.Emit("exec_output", data)
	return er.Output, er.ExitCode, err
}

func sanitize(name string) string {
	return strings.NewReplacer("/", "-", "\\", "-").Replace(name)
}

func status(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
