package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/sskrishnav/kubechat/internal/core"
)

// fakeServer has the named tools; Call answers with the server's name.
type fakeServer struct {
	name  string
	tools []string
	err   error // returned by List
}

func (f *fakeServer) List(context.Context) ([]core.ToolSpec, error) {
	specs := make([]core.ToolSpec, len(f.tools))
	for i, t := range f.tools {
		specs[i] = core.ToolSpec{Name: t}
	}
	return specs, f.err
}

func (f *fakeServer) Call(context.Context, core.ToolCall) (core.ToolResult, error) {
	return core.ToolResult{Text: f.name}, nil
}

func TestServersRoutesEachCallToItsServer(t *testing.T) {
	s := &Servers{All: []core.ToolRunner{
		&fakeServer{name: "kubernetes", tools: []string{"pods_log", "resources_list"}},
		&fakeServer{name: "k8sgpt", tools: []string{"analyze"}},
	}}
	ctx := context.Background()
	specs, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 {
		t.Fatalf("got %d tools, want 3", len(specs))
	}
	for tool, server := range map[string]string{"pods_log": "kubernetes", "analyze": "k8sgpt"} {
		res, err := s.Call(ctx, core.ToolCall{Name: tool})
		if err != nil || res.Text != server {
			t.Errorf("%s went to %q (err %v), want %q", tool, res.Text, err, server)
		}
	}
	if _, err := s.Call(ctx, core.ToolCall{Name: "nope"}); err == nil {
		t.Error("an unknown tool must fail")
	}
}

func TestServersListFails(t *testing.T) {
	for name, servers := range map[string][]core.ToolRunner{
		"same tool on two servers": {
			&fakeServer{tools: []string{"analyze"}},
			&fakeServer{tools: []string{"analyze"}},
		},
		"a server is down": {
			&fakeServer{tools: []string{"pods_log"}},
			&fakeServer{err: errors.New("connection refused")},
		},
	} {
		if _, err := (&Servers{All: servers}).List(context.Background()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
