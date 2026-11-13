package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sskrishnav/kubechat/internal/core"
)

// scriptedLLM returns its replies in order and remembers what it was sent.
type scriptedLLM struct {
	replies []core.Message
	seen    [][]core.Message
}

func (s *scriptedLLM) Complete(_ context.Context, history []core.Message, _ []core.ToolSpec) (core.Message, error) {
	s.seen = append(s.seen, append([]core.Message(nil), history...))
	if len(s.replies) == 0 {
		return core.Message{}, errors.New("script ran out")
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

// fakeGate returns a fixed output, or an error for the tool named "broken".
type fakeGate struct{ output string }

func (f *fakeGate) List(context.Context) ([]core.ToolSpec, error) { return nil, nil }

func (f *fakeGate) Call(_ context.Context, c core.ToolCall) (core.ToolResult, error) {
	if c.Name == "broken" {
		return core.ToolResult{}, errors.New("refused: nope")
	}
	return core.ToolResult{Text: f.output}, nil
}

func toolCall(id, name string) core.Message {
	return core.Message{Role: "assistant", ToolCalls: []core.ToolCall{{ID: id, Name: name, Args: []byte(`{}`)}}}
}

func TestAskRunsToolsThenAnswers(t *testing.T) {
	llm := &scriptedLLM{replies: []core.Message{
		toolCall("c1", "pods_log"),
		toolCall("c2", "broken"),
		{Role: "assistant", Text: "demo-crash: missing DB_URL"},
	}}
	a := &Agent{LLM: llm, Gate: &fakeGate{output: "fatal: missing DB_URL password=hunter2"}, MaxRounds: 5, MaxOutput: 1000}

	answer, err := a.Ask(context.Background(), "why is demo-crash failing?")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "demo-crash: missing DB_URL" {
		t.Errorf("answer = %q", answer)
	}

	// The last request the LLM saw: system, user, call c1, result c1, call c2, result c2.
	last := llm.seen[len(llm.seen)-1]
	if len(last) != 6 {
		t.Fatalf("history has %d messages, want 6", len(last))
	}
	if r := last[3]; r.ToolCallID != "c1" || strings.Contains(r.Text, "hunter2") {
		t.Errorf("tool result not linked or not redacted: %+v", r)
	}
	if r := last[5]; r.ToolCallID != "c2" || !strings.HasPrefix(r.Text, "error: refused") {
		t.Errorf("tool error not passed to the LLM: %+v", r)
	}
}

func TestAskStopsAfterMaxRounds(t *testing.T) {
	llm := &scriptedLLM{replies: []core.Message{toolCall("c1", "pods_log"), toolCall("c2", "pods_log")}}
	a := &Agent{LLM: llm, Gate: &fakeGate{}, MaxRounds: 2}
	if _, err := a.Ask(context.Background(), "loop forever"); err == nil {
		t.Error("want an error after MaxRounds")
	}
}
