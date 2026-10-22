package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edxeme/evalrig/internal/harness"
	"github.com/edxeme/evalrig/internal/verify"
)

func sampleReport() *Report {
	started := time.Date(2026, 10, 26, 9, 47, 0, 0, time.UTC)
	return &Report{
		Suite:     "core-skills",
		StartedAt: started,
		Results: []harness.Result{
			{Task: "fix-grep", Agent: "golden", Pass: true, Turns: 1, DurationMS: 2100,
				Checks: []verify.Check{{Cmd: "verify.sh", Pass: true}}},
			{Task: "json-sum", Agent: "golden", Pass: false, Turns: 1, DurationMS: 800,
				Error:  "verification failed",
				Checks: []verify.Check{{Cmd: "verify.sh", Pass: false, ExitCode: 1}}},
		},
	}
}

func TestMarkdownSummary(t *testing.T) {
	md := sampleReport().Markdown()
	for _, want := range []string{
		"# evalrig · core-skills",
		"1/2 passed (50%)",
		"| fix-grep | golden | ✅ pass | 1 | 2.1s |  |",
		"| json-sum | golden | ❌ fail | 1 | 800ms | verification failed |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestWriteJSONRoundtrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	md, js, err := sampleReport().Write(dir)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(md); err != nil {
		t.Errorf("report.md missing: %v", err)
	}
	raw, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("decode report.json: %v", err)
	}
	if back.Suite != "core-skills" || len(back.Results) != 2 {
		t.Errorf("roundtrip mismatch: %+v", back)
	}
}

func TestOneLineTruncates(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := oneLine("a\n\nb\t c " + long)
	if strings.ContainsAny(got, "\n\t") || !strings.HasSuffix(got, "…") {
		t.Errorf("oneLine = %q, want flattened truncated string", got)
	}
}
