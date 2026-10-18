package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/edxeme/evalrig/internal/llm"
)

type scriptedClient struct {
	resps []string
	i     int
}

func (s *scriptedClient) Complete(ctx context.Context, msgs []llm.Message) (string, error) {
	if s.i >= len(s.resps) {
		return "", fmt.Errorf("script exhausted")
	}
	r := s.resps[s.i]
	s.i++
	return r, nil
}

type fakeSession struct {
	cmds []string
}

func (f *fakeSession) Exec(ctx context.Context, cmd string) (string, int, error) {
	f.cmds = append(f.cmds, cmd)
	return "output of " + cmd, 0, nil
}

func TestChatExecutesFencedCommands(t *testing.T) {
	c := NewChat(&scriptedClient{resps: []string{
		"Let me look around.\n```bash\necho one\n```",
		"```sh\necho two\n```",
		"Done",
	}}, "m", 0)
	sess := &fakeSession{}

	summary, err := c.Run(context.Background(), "do it", sess, NoopEvents{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if summary.Turns != 3 {
		t.Errorf("Turns = %d, want 3", summary.Turns)
	}
	if strings.Join(sess.cmds, "|") != "echo one|echo two" {
		t.Errorf("commands = %v, want [echo one echo two]", sess.cmds)
	}
}

func TestChatStopsOnDoneImmediately(t *testing.T) {
	c := NewChat(&scriptedClient{resps: []string{"DONE"}}, "m", 0)
	sess := &fakeSession{}
	if _, err := c.Run(context.Background(), "p", sess, NoopEvents{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(sess.cmds) != 0 {
		t.Errorf("commands = %v, want none", sess.cmds)
	}
}

func TestChatMaxTurns(t *testing.T) {
	c := NewChat(&scriptedClient{resps: []string{
		"```bash\necho x\n```", "```bash\necho x\n```",
	}}, "m", 2)
	_, err := c.Run(context.Background(), "p", &fakeSession{}, NoopEvents{})
	if err == nil || !strings.Contains(err.Error(), "max turns (2)") {
		t.Errorf("error = %v, want max turns reached", err)
	}
}

func TestChatNoCommand(t *testing.T) {
	c := NewChat(&scriptedClient{resps: []string{"I think it works fine."}}, "m", 0)
	_, err := c.Run(context.Background(), "p", &fakeSession{}, NoopEvents{})
	if err == nil || !strings.Contains(err.Error(), "no shell command") {
		t.Errorf("error = %v, want no shell command", err)
	}
}

func TestParseCommand(t *testing.T) {
	cases := []struct {
		resp string
		cmd  string
		done bool
	}{
		{"```bash\nls -la\n```", "ls -la", false},
		{"```sh\npwd\n```", "pwd", false},
		{"```shell\nmake test\n```", "make test", false},
		{"no fence at all", "", false},
		{"DONE", "", true},
		{"Done.", "", true},
		{"FINISHED", "", true},
		{"```bash\necho hi\n```\nDone", "echo hi", false}, // fence wins
	}
	for _, tc := range cases {
		cmd, done := parseCommand(tc.resp)
		if cmd != tc.cmd || done != tc.done {
			t.Errorf("parseCommand(%q) = (%q, %v), want (%q, %v)", tc.resp, cmd, done, tc.cmd, tc.done)
		}
	}
}
