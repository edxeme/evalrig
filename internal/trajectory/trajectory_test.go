package trajectory

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecorderWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "run.jsonl")
	r, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	r.Emit("task_start", map[string]any{"task": "t1"})
	r.Emit("exec_command", map[string]any{"command": "ls"})
	if err := r.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var types []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %q is not valid JSON: %v", sc.Text(), err)
		}
		if e.Time.IsZero() {
			t.Error("event has zero time")
		}
		types = append(types, e.Type)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 || types[0] != "task_start" || types[1] != "exec_command" {
		t.Errorf("types = %v, want [task_start exec_command]", types)
	}
}

func TestRecorderConcurrentEmit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	r, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				r.Emit("beat", map[string]any{"i": i})
			}
		}()
	}
	wg.Wait()
	if err := r.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	lines := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
	}
	if lines != 200 {
		t.Errorf("lines = %d, want 200 (no torn writes)", lines)
	}
}

func TestNilRecorderIsSafe(t *testing.T) {
	var r *Recorder
	r.Emit("nothing", nil)
	if err := r.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
