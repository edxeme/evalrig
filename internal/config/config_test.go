package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validSuite = `name: test-suite
version: 1
defaults:
  timeout: 2m
tasks:
  - id: t1
    image: alpine:3.20
    dir: tasks/t1
    files: files
    prompt: prompt.md
    golden: solution.sh
    verify:
      - verify.sh
`

func writeTask(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"tasks/t1/prompt.md":   "fix the thing",
		"tasks/t1/solution.sh": "echo fixed",
		"tasks/t1/verify.sh":   "exit 0",
		"tasks/t1/files/x.txt": "payload",
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

func writeSuite(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	writeTask(t, root)
	path := filepath.Join(root, "suite.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidSuite(t *testing.T) {
	s, err := Load(writeSuite(t, validSuite))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if s.Name != "test-suite" {
		t.Errorf("Name = %q, want test-suite", s.Name)
	}
	if len(s.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(s.Tasks))
	}
	task := s.Tasks[0]
	if got := s.TaskPath(task, task.Prompt); !strings.HasSuffix(got, "tasks/t1/prompt.md") {
		t.Errorf("TaskPath = %q, want suffix tasks/t1/prompt.md", got)
	}
	timeout, err := s.Timeout(task)
	if err != nil || timeout != 2*time.Minute {
		t.Errorf("Timeout = %v, %v; want 2m, nil", timeout, err)
	}
}

func TestTaskTimeoutFallbacks(t *testing.T) {
	s := &Suite{Defaults: Defaults{Timeout: "90s"}}
	d, err := s.Timeout(Task{})
	if err != nil || d != 90*time.Second {
		t.Errorf("suite default not applied: %v, %v", d, err)
	}
	d, err = s.Timeout(Task{Timeout: "1m"})
	if err != nil || d != time.Minute {
		t.Errorf("task override not applied: %v, %v", d, err)
	}
	d, err = (&Suite{}).Timeout(Task{})
	if err != nil || d != DefaultTimeout {
		t.Errorf("DefaultTimeout not applied: %v, %v", d, err)
	}
}

func TestLoadFailures(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing name",
			content: strings.Replace(validSuite, "name: test-suite\n", "", 1),
			wantErr: "name is required",
		},
		{
			name:    "no tasks",
			content: "name: empty\nversion: 1\ntasks: []\n",
			wantErr: "at least one task",
		},
		{
			name: "duplicate id",
			content: strings.Replace(validSuite+"\n  - id: t1\n    image: alpine\n    dir: tasks/t1\n    prompt: prompt.md\n    golden: solution.sh\n    verify: [verify.sh]\n",
				"  - id: t1\n    image: alpine:3.20\n", "  - id: t1\n    image: alpine:3.20\n", 1),
			wantErr: "duplicate id",
		},
		{
			name:    "missing image",
			content: strings.Replace(validSuite, "    image: alpine:3.20\n", "", 1),
			wantErr: "image is required",
		},
		{
			name:    "missing verify",
			content: strings.Replace(validSuite, "    verify:\n      - verify.sh\n", "", 1),
			wantErr: "at least one verify entry",
		},
		{
			name:    "bad timeout",
			content: strings.Replace(validSuite, "  timeout: 2m\n", "  timeout: soon\n", 1),
			wantErr: "parse timeout",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeSuite(t, tc.content))
			if err == nil {
				t.Fatalf("Load() succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestMissingTaskFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	content := strings.Replace(validSuite, "    prompt: prompt.md\n", "    prompt: nope.md\n", 1)
	os.WriteFile(path, []byte(content), 0o644)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "prompt file") {
		t.Errorf("Load() error = %v, want missing prompt file", err)
	}
}

func TestInlineVerifyCommandsSkipFileCheck(t *testing.T) {
	content := strings.Replace(validSuite, "      - verify.sh\n", "      - echo done\n", 1)
	if _, err := Load(writeSuite(t, content)); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !IsInlineCommand("echo done") || IsInlineCommand("verify.sh") {
		t.Error("IsInlineCommand classification is wrong")
	}
}
