package config

import (
	"slices"
	"testing"

	"github.com/sskrishnav/kubechat/internal/core"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	c, err := Load(env(map[string]string{
		"KUBECHAT_PROTECTED_NAMESPACES": " kube-*, kubechat ,",
		"KUBECHAT_TOOL_TIERS":           "resources_scale=write",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.ProtectedNamespaces, []string{"kube-*", "kubechat"}) {
		t.Errorf("ProtectedNamespaces = %v", c.ProtectedNamespaces)
	}
	if c.ToolTiers["resources_scale"] != core.TierWrite {
		t.Errorf("ToolTiers = %v", c.ToolTiers)
	}
}

func TestLoadDefaultsAreSafe(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ToolTiers) != 0 {
		t.Errorf("default ToolTiers = %v, want none (no tool is a write)", c.ToolTiers)
	}
	if !slices.Equal(c.ProtectedNamespaces, []string{"kube-*"}) {
		t.Errorf("default ProtectedNamespaces = %v, want [kube-*]", c.ProtectedNamespaces)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	for key, value := range map[string]string{
		"KUBECHAT_TOOL_TIERS":           "resources_scale=superuser",
		"KUBECHAT_PROTECTED_NAMESPACES": "kube-[",
		"KUBECHAT_MAX_TOOL_ROUNDS":      "0",
	} {
		if _, err := Load(env(map[string]string{key: value})); err == nil {
			t.Errorf("%s=%q: want an error", key, value)
		}
	}
}
