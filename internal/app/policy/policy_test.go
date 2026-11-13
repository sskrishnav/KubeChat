package policy

import (
	"testing"

	"github.com/sskrishnav/kubechat/internal/core"
)

var (
	yes, no = true, false

	readTool    = core.ToolSpec{Name: "pods_log", Hints: &core.ToolHints{ReadOnly: true}}
	writeTool   = core.ToolSpec{Name: "resources_apply", Hints: &core.ToolHints{Destructive: &no}}
	scaleTool   = core.ToolSpec{Name: "resources_scale", Hints: &core.ToolHints{Destructive: &yes}}
	unsetTool   = core.ToolSpec{Name: "unset", Hints: &core.ToolHints{}}
	noHintsTool = core.ToolSpec{Name: "mystery"}
)

func TestTier(t *testing.T) {
	plain := Policy{}
	override := Policy{Overrides: map[string]core.Tier{"resources_scale": core.TierWrite}}

	tests := []struct {
		name   string
		policy Policy
		spec   core.ToolSpec
		want   core.Tier
	}{
		{"read-only hint is read", plain, readTool, core.TierRead},
		{"destructive=false is write", plain, writeTool, core.TierWrite},
		{"destructive=true is destructive", plain, scaleTool, core.TierDestructive},
		{"destructive not set is destructive", plain, unsetTool, core.TierDestructive},
		{"no hints at all is destructive", plain, noHintsTool, core.TierDestructive},
		{"override beats hints", override, scaleTool, core.TierWrite},
		{"override for another tool doesn't apply", override, noHintsTool, core.TierDestructive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.policy.Tier(tt.spec); got != tt.want {
				t.Errorf("Tier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecide(t *testing.T) {
	p := Policy{
		Overrides:           map[string]core.Tier{"resources_scale": core.TierWrite},
		ProtectedNamespaces: []string{"kube-*", "kubechat"},
	}

	tests := []struct {
		name         string
		spec         core.ToolSpec
		args         string
		wantApproval bool
		wantRefused  bool
	}{
		{"read runs at once, even in kube-system", readTool, `{"namespace":"kube-system"}`, false, false},
		{"scale in apps needs approval", scaleTool, `{"name":"demo-web","namespace":"apps","scale":3}`, true, false},
		{"scale in any other app namespace needs approval", scaleTool, `{"name":"shop","namespace":"shop","scale":2}`, true, false},
		{"scale in kube-system is refused", scaleTool, `{"name":"coredns","namespace":"kube-system","scale":0}`, false, true},
		{"scale in kube-public is refused", scaleTool, `{"name":"x","namespace":"kube-public","scale":0}`, false, true},
		{"scale in kubechat (the bot itself) is refused", scaleTool, `{"name":"kubechat-mcp","namespace":"kubechat","scale":0}`, false, true},
		{"kube-* does not match kubernetes-dashboard", scaleTool, `{"name":"x","namespace":"kubernetes-dashboard","scale":1}`, true, false},
		{"scale without namespace is refused", scaleTool, `{"name":"demo-web","scale":3}`, false, true},
		{"scale with bad JSON is refused", scaleTool, `not json`, false, true},
		{"tool without hints is refused", noHintsTool, `{"namespace":"apps"}`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := p.Decide(tt.spec, []byte(tt.args))
			if d.NeedsApproval != tt.wantApproval || (d.Refused != "") != tt.wantRefused {
				t.Errorf("Decide() = %+v, want approval=%v refused=%v", d, tt.wantApproval, tt.wantRefused)
			}
		})
	}
}

func TestDecideBrokenPatternProtectsEverything(t *testing.T) {
	p := Policy{
		Overrides:           map[string]core.Tier{"resources_scale": core.TierWrite},
		ProtectedNamespaces: []string{"["}, // invalid pattern
	}
	if d := p.Decide(scaleTool, []byte(`{"namespace":"apps"}`)); d.Refused == "" {
		t.Errorf("Decide() = %+v, want refused (fail closed)", d)
	}
}
