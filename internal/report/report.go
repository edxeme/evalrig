// Package report renders evaluation summaries as markdown and JSON.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edxeme/evalrig/internal/harness"
)

// Report aggregates the results of one evalrig invocation.
type Report struct {
	Suite     string           `json:"suite"`
	StartedAt time.Time        `json:"started_at"`
	Results   []harness.Result `json:"results"`
}

// Passed counts passing results.
func (r *Report) Passed() int {
	n := 0
	for _, res := range r.Results {
		if res.Pass {
			n++
		}
	}
	return n
}

// PassRate returns the pass percentage, 0 when there are no results.
func (r *Report) PassRate() float64 {
	if len(r.Results) == 0 {
		return 0
	}
	return 100 * float64(r.Passed()) / float64(len(r.Results))
}

// Markdown renders a human-readable summary table.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# evalrig · %s\n\n", r.Suite)
	fmt.Fprintf(&b, "_%s · %d/%d passed (%.0f%%)_\n\n",
		r.StartedAt.UTC().Format("2006-01-02 15:04 MST"),
		r.Passed(), len(r.Results), r.PassRate())

	b.WriteString("| task | agent | result | turns | duration | error |\n")
	b.WriteString("| :--- | :--- | :--- | ---: | ---: | :--- |\n")
	for _, res := range r.Results {
		status := "❌ fail"
		if res.Pass {
			status = "✅ pass"
		}
		dur := time.Duration(res.DurationMS) * time.Millisecond
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s |\n",
			res.Task, res.Agent, status, res.Turns, dur, oneLine(res.Error))
	}
	return b.String()
}

// Write persists report.json and report.md under dir and returns both paths.
func (r *Report) Write(dir string) (mdPath, jsonPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	jsonPath = filepath.Join(dir, "report.json")
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(jsonPath, append(raw, '\n'), 0o644); err != nil {
		return "", "", err
	}
	mdPath = filepath.Join(dir, "report.md")
	if err := os.WriteFile(mdPath, []byte(r.Markdown()), 0o644); err != nil {
		return "", "", err
	}
	return mdPath, jsonPath, nil
}

// oneLine flattens an error message for a table cell.
func oneLine(s string) string {
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}
