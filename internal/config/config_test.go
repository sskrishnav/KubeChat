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
	want := []MCPServer{{Name: "kubernetes", URL: "http://localhost:8080/mcp"}}
	if !slices.Equal(c.MCPServers, want) {
		t.Errorf("default MCPServers = %+v, want %+v (one server, not read-only)", c.MCPServers, want)
	}
}

func TestLoadMCPServers(t *testing.T) {
	c, err := Load(env(map[string]string{
		"KUBECHAT_MCP_SERVERS":   " kubernetes=http://kubechat-mcp:8080/mcp , k8sgpt=http://kubechat-k8sgpt:8089/mcp?a=b ,",
		"KUBECHAT_MCP_READ_ONLY": "k8sgpt",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []MCPServer{
		{Name: "kubernetes", URL: "http://kubechat-mcp:8080/mcp"},
		{Name: "k8sgpt", URL: "http://kubechat-k8sgpt:8089/mcp?a=b", ReadOnly: true},
	}
	if !slices.Equal(c.MCPServers, want) {
		t.Errorf("MCPServers = %+v, want %+v", c.MCPServers, want)
	}

	for _, servers := range []string{
		",",                             // none
		"http://kubechat-mcp/mcp",       // no name
		"kubernetes=",                   // no URL
		"a=http://x/mcp,a=http://y/mcp", // the same name twice
	} {
		if _, err := Load(env(map[string]string{"KUBECHAT_MCP_SERVERS": servers})); err == nil {
			t.Errorf("KUBECHAT_MCP_SERVERS=%q: want an error", servers)
		}
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

func TestLoadSlack(t *testing.T) {
	const on, app, bot = "KUBECHAT_SLACK_ENABLED", "KUBECHAT_SLACK_APP_TOKEN", "KUBECHAT_SLACK_BOT_TOKEN"

	c, err := Load(env(map[string]string{app: "xapp-1", bot: "xoxb-1"}))
	if err != nil || c.SlackEnabled || c.SlackAppToken != "" {
		t.Errorf("off by default, tokens ignored: got enabled=%v app=%q err=%v", c.SlackEnabled, c.SlackAppToken, err)
	}

	c, err = Load(env(map[string]string{on: "true", app: "xapp-1", bot: "xoxb-1"}))
	if err != nil || !c.SlackEnabled || c.SlackAppToken != "xapp-1" || c.SlackBotToken != "xoxb-1" {
		t.Errorf("enabled: got enabled=%v app=%q bot=%q err=%v", c.SlackEnabled, c.SlackAppToken, c.SlackBotToken, err)
	}

	for name, vars := range map[string]map[string]string{
		"enabled, no tokens":      {on: "true"},
		"enabled, no bot token":   {on: "true", app: "xapp-1"},
		"enabled, swapped tokens": {on: "true", app: "xoxb-1", bot: "xapp-1"},
		"not a boolean":           {on: "yes please"},
	} {
		if _, err := Load(env(vars)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
