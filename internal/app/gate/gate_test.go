package gate

import (
	"context"
	"testing"

	"github.com/sskrishnav/kubechat/internal/app/policy"
	"github.com/sskrishnav/kubechat/internal/core"
)

// fakeServer stands in for the MCP client and records what actually ran.
type fakeServer struct {
	specs []core.ToolSpec
	ran   []string
}

func (f *fakeServer) List(context.Context) ([]core.ToolSpec, error) { return f.specs, nil }

func (f *fakeServer) Call(_ context.Context, c core.ToolCall) (core.ToolResult, error) {
	f.ran = append(f.ran, c.Name)
	return core.ToolResult{Text: "ok"}, nil
}

func TestGate(t *testing.T) {
	yes := true
	server := &fakeServer{specs: []core.ToolSpec{
		{Name: "pods_log", Hints: &core.ToolHints{ReadOnly: true}},
		{Name: "resources_scale", Hints: &core.ToolHints{Destructive: &yes}},
	}}
	g := &Gate{Server: server, Policy: policy.Policy{
		Overrides:           map[string]core.Tier{"resources_scale": core.TierWrite},
		ProtectedNamespaces: []string{"kube-*"},
	}}
	if _, err := g.List(context.Background()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		call    core.ToolCall
		wantRan bool
	}{
		{"read runs", core.ToolCall{Name: "pods_log", Args: []byte(`{"name":"x","namespace":"apps"}`)}, true},
		{"write is refused (no approvals yet)", core.ToolCall{Name: "resources_scale", Args: []byte(`{"name":"demo-web","namespace":"apps","scale":3}`)}, false},
		{"write in a protected namespace is refused", core.ToolCall{Name: "resources_scale", Args: []byte(`{"name":"coredns","namespace":"kube-system","scale":0}`)}, false},
		{"unknown tool is refused", core.ToolCall{Name: "pods_delete", Args: []byte(`{"name":"x","namespace":"apps"}`)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server.ran = nil
			_, err := g.Call(context.Background(), tt.call)
			ran := len(server.ran) == 1
			if ran != tt.wantRan || (err == nil) != tt.wantRan {
				t.Errorf("ran=%v err=%v, want ran=%v", ran, err, tt.wantRan)
			}
		})
	}
}
