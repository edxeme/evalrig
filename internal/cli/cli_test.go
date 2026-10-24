package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const suiteYAML = `name: cli-test
version: 1
tasks:
  - id: t1
    image: alpine:3.20
    dir: tasks/t1
    prompt: prompt.md
    golden: solution.sh
    verify:
      - verify.sh
`

func writeSuite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"suite.yaml":           suiteYAML,
		"tasks/t1/prompt.md":   "do it",
		"tasks/t1/solution.sh": "echo solved",
		"tasks/t1/verify.sh":   "exit 0",
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
	return filepath.Join(root, "suite.yaml")
}

func TestValidateOK(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"validate", "--suite", writeSuite(t)}, &out, &errOut)
	if code != 0 {
		t.Errorf("code = %d, stderr = %q, want 0", code, errOut.String())
	}
	if !strings.Contains(out.String(), "cli-test: 1 tasks ok") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestValidateBadSuite(t *testing.T) {
	path := writeSuite(t)
	raw, _ := os.ReadFile(path)
	broken := strings.Replace(string(raw), "  - id: t1\n", "  - id: \n", 1)
	os.WriteFile(path, []byte(broken), 0o644)

	var out, errOut bytes.Buffer
	code := Run([]string{"validate", "--suite", path}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "id") {
		t.Errorf("code = %d, stderr = %q, want 1 with id error", code, errOut.String())
	}
}

func TestListPrintsInventory(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"list", "--suite", writeSuite(t)}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "t1\talpine:3.20\t1 verifier(s)") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestVersionAndHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"version"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "evalrig 0.1.0") {
		t.Errorf("version: code=%d out=%q", code, out.String())
	}
	out.Reset()
	if code := Run([]string{"--help"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("help: code=%d out=%q", code, out.String())
	}
}

func TestUnknownCommandAndMissingSuite(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"deploy"}, &out, &errOut); code != 2 {
		t.Errorf("unknown command code = %d, want 2", code)
	}
	if code := Run([]string{"run", "--suite", "/nonexistent.yaml"}, &out, &errOut); code != 1 {
		t.Errorf("missing suite code = %d, want 1", code)
	}
}

func TestRunRejectsUnknownAgentBeforeDocker(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"run", "--suite", writeSuite(t), "--agents", "hal9000"}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "unknown agent") {
		t.Errorf("code = %d, stderr = %q, want unknown agent error", code, errOut.String())
	}
}
