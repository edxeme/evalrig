// Package config loads and validates evaluation suite definitions.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultTimeout applies when neither a task nor the suite define one.
const DefaultTimeout = 5 * time.Minute

// Suite is a named collection of tasks to evaluate.
type Suite struct {
	Name     string   `yaml:"name"`
	Version  int      `yaml:"version"`
	Defaults Defaults `yaml:"defaults"`
	Tasks    []Task   `yaml:"tasks"`

	// Dir is the absolute path of the directory holding the suite file.
	// Every task path is resolved relative to it.
	Dir string `yaml:"-"`
}

// Defaults holds values inherited by tasks that do not set them.
type Defaults struct {
	Timeout string `yaml:"timeout"`
}

// Task is a single evaluation unit: a prompt, an environment image, a
// golden solution and one or more verifiers.
type Task struct {
	ID      string   `yaml:"id"`
	Image   string   `yaml:"image"`
	Dir     string   `yaml:"dir"`
	Files   string   `yaml:"files"`
	Prompt  string   `yaml:"prompt"`
	Golden  string   `yaml:"golden"`
	Verify  []string `yaml:"verify"`
	Timeout string   `yaml:"timeout"`
}

// Load reads and validates the suite file at path.
func Load(path string) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite: %w", err)
	}
	var s Suite
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse suite %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.Dir = filepath.Dir(abs)
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Validate checks structural rules that YAML tags cannot express.
func (s *Suite) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("suite: name is required")
	}
	if len(s.Tasks) == 0 {
		return fmt.Errorf("suite: at least one task is required")
	}
	seen := make(map[string]bool, len(s.Tasks))
	for _, t := range s.Tasks {
		if strings.TrimSpace(t.ID) == "" {
			return fmt.Errorf("suite: every task needs an id")
		}
		if seen[t.ID] {
			return fmt.Errorf("task %s: duplicate id", t.ID)
		}
		seen[t.ID] = true

		if t.Image == "" {
			return fmt.Errorf("task %s: image is required", t.ID)
		}
		if t.Dir == "" {
			return fmt.Errorf("task %s: dir is required", t.ID)
		}
		if t.Prompt == "" {
			return fmt.Errorf("task %s: prompt is required", t.ID)
		}
		if t.Golden == "" {
			return fmt.Errorf("task %s: golden solution is required", t.ID)
		}
		if len(t.Verify) == 0 {
			return fmt.Errorf("task %s: at least one verify entry is required", t.ID)
		}

		if !exists(s.TaskPath(t, t.Prompt)) {
			return fmt.Errorf("task %s: prompt file %q not found", t.ID, t.Prompt)
		}
		if !exists(s.TaskPath(t, t.Golden)) {
			return fmt.Errorf("task %s: golden solution %q not found", t.ID, t.Golden)
		}
		for _, v := range t.Verify {
			if IsInlineCommand(v) {
				continue
			}
			if !exists(s.TaskPath(t, v)) {
				return fmt.Errorf("task %s: verify entry %q not found", t.ID, v)
			}
		}
		if t.Files != "" && !exists(s.FilesPath(t)) {
			return fmt.Errorf("task %s: files directory %q not found", t.ID, t.Files)
		}

		if _, err := s.Timeout(t); err != nil {
			return err
		}
	}
	return nil
}

// TaskPath resolves p relative to the task directory inside the suite.
func (s *Suite) TaskPath(t Task, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(s.Dir, t.Dir, p)
}

// FilesPath resolves the task files directory.
func (s *Suite) FilesPath(t Task) string {
	return s.TaskPath(t, t.Files)
}

// Timeout resolves the effective timeout for t.
func (s *Suite) Timeout(t Task) (time.Duration, error) {
	raw := t.Timeout
	if raw == "" {
		raw = s.Defaults.Timeout
	}
	if raw == "" {
		return DefaultTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("task %s: parse timeout %q: %w", t.ID, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("task %s: timeout must be positive", t.ID)
	}
	return d, nil
}

// IsInlineCommand reports whether a verify entry is a shell command
// rather than a script file reference. Entries without shell
// separators are treated as files living in the task directory.
func IsInlineCommand(entry string) bool {
	return strings.ContainsAny(entry, " \t;&|><")
}

// ReadPrompt returns the task prompt text.
func (s *Suite) ReadPrompt(t Task) (string, error) {
	data, err := os.ReadFile(s.TaskPath(t, t.Prompt))
	if err != nil {
		return "", fmt.Errorf("task %s: read prompt: %w", t.ID, err)
	}
	return string(data), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
