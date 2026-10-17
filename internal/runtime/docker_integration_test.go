//go:build integration

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerEndToEnd(t *testing.T) {
	ctx := context.Background()
	rt, err := NewDocker(ctx)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer rt.Close()

	id, err := rt.Start(ctx, "alpine:3.20")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := rt.Stop(stopCtx, id); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "hello.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rt.CopyDir(ctx, id, src, "/task"); err != nil {
		t.Fatalf("CopyDir() error = %v", err)
	}

	res, err := rt.Exec(ctx, id, "cat /task/hello.txt && echo done")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Output, "hello world") || !strings.Contains(res.Output, "done") {
		t.Errorf("Output = %q, want hello world + done", res.Output)
	}

	res, err = rt.Exec(ctx, id, "false")
	if err != nil {
		t.Fatalf("Exec(false) error = %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", res.ExitCode)
	}
}
