// Package trajectory records structured run events as JSONL files,
// one event per line, for offline analysis of agent behaviour.
package trajectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is a single timestamped record in a run trajectory.
type Event struct {
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	Data any       `json:"data,omitempty"`
}

// Recorder appends events to a JSONL file, safe for concurrent use.
// The zero value and nil receivers discard everything.
type Recorder struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// New creates the parent directory of path and opens the file for writing.
func New(path string) (*Recorder, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("trajectory: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("trajectory: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return &Recorder{f: f, enc: enc}, nil
}

// Emit appends one event. Failures are swallowed: a telemetry problem
// must never fail an evaluation run. Nil receivers discard everything.
func (r *Recorder) Emit(event string, data any) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	_ = r.enc.Encode(Event{Time: time.Now().UTC(), Type: event, Data: data})
}

// Close flushes and closes the underlying file.
func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
