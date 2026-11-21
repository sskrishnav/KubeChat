// Package config reads the bot's settings from environment variables.
// The Helm chart sets them (deploy/helm/kubechat/values.yaml, bot.env).
package config

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/sskrishnav/kubechat/internal/core"
)

type Config struct {
	LLMBaseURL          string               // KUBECHAT_LLM_BASE_URL
	LLMModel            string               // KUBECHAT_LLM_MODEL
	MaxToolRounds       int                  // KUBECHAT_MAX_TOOL_ROUNDS
	MCPServers          []MCPServer          // KUBECHAT_MCP_SERVERS and KUBECHAT_MCP_READ_ONLY
	ProtectedNamespaces []string             // KUBECHAT_PROTECTED_NAMESPACES, e.g. "kube-*,kubechat"
	ToolTiers           map[string]core.Tier // KUBECHAT_TOOL_TIERS, e.g. "resources_scale=write"
	SlackEnabled        bool                 // KUBECHAT_SLACK_ENABLED; default false
	SlackAppToken       string               // KUBECHAT_SLACK_APP_TOKEN (xapp-…); read only when Slack is enabled
	SlackBotToken       string               // KUBECHAT_SLACK_BOT_TOKEN (xoxb-…); read only when Slack is enabled
}

// MCPServer is one MCP server the bot uses. The LLM sees the tools of all of them.
type MCPServer struct {
	Name     string // for logs, e.g. "kubernetes"
	URL      string // e.g. http://kubechat-mcp:8080/mcp
	ReadOnly bool   // all its tools count as reads; for servers whose RBAC can only read
}

// Load reads the settings with getenv (os.Getenv in main, a map in tests).
// Defaults are safe: no tool is a write until KUBECHAT_TOOL_TIERS says so,
// and kube-* namespaces are always protected unless overridden.
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	c := Config{
		LLMBaseURL: get("KUBECHAT_LLM_BASE_URL", "http://localhost:11434/v1"),
		LLMModel:   get("KUBECHAT_LLM_MODEL", "qwen3:14b"),
		ToolTiers:  map[string]core.Tier{},
	}

	rounds, err := strconv.Atoi(get("KUBECHAT_MAX_TOOL_ROUNDS", "8"))
	if err != nil || rounds < 1 {
		return Config{}, fmt.Errorf("KUBECHAT_MAX_TOOL_ROUNDS: want a positive number")
	}
	c.MaxToolRounds = rounds

	// MCP servers, in order: "name=url, ...". KUBECHAT_MCP_READ_ONLY names some of them.
	readOnly := map[string]bool{}
	for _, name := range strings.Split(get("KUBECHAT_MCP_READ_ONLY", ""), ",") {
		readOnly[strings.TrimSpace(name)] = true
	}
	seen := map[string]bool{}
	for _, pair := range strings.Split(get("KUBECHAT_MCP_SERVERS", "kubernetes=http://localhost:8080/mcp"), ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		name, url, _ := strings.Cut(pair, "=")
		name, url = strings.TrimSpace(name), strings.TrimSpace(url)
		switch {
		case name == "" || url == "":
			return Config{}, fmt.Errorf("KUBECHAT_MCP_SERVERS: bad entry %q, want name=url", pair)
		case seen[name]:
			return Config{}, fmt.Errorf("KUBECHAT_MCP_SERVERS: %q is listed twice", name)
		}
		seen[name] = true
		c.MCPServers = append(c.MCPServers, MCPServer{Name: name, URL: url, ReadOnly: readOnly[name]})
	}
	if len(c.MCPServers) == 0 {
		return Config{}, fmt.Errorf("KUBECHAT_MCP_SERVERS: want at least one name=url")
	}

	for _, pattern := range strings.Split(get("KUBECHAT_PROTECTED_NAMESPACES", "kube-*"), ",") {
		if pattern = strings.TrimSpace(pattern); pattern == "" {
			continue
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return Config{}, fmt.Errorf("KUBECHAT_PROTECTED_NAMESPACES: bad pattern %q", pattern)
		}
		c.ProtectedNamespaces = append(c.ProtectedNamespaces, pattern)
	}

	for _, pair := range strings.Split(get("KUBECHAT_TOOL_TIERS", ""), ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		name, tier, _ := strings.Cut(pair, "=")
		name, tier = strings.TrimSpace(name), strings.TrimSpace(tier)
		switch t := core.Tier(tier); t {
		case core.TierRead, core.TierWrite, core.TierDestructive, core.TierBlocked:
			c.ToolTiers[name] = t
		default:
			return Config{}, fmt.Errorf("KUBECHAT_TOOL_TIERS: bad entry %q, want tool=read|write|destructive|blocked", pair)
		}
	}

	// Slack is off unless enabled. Errors never include the token values.
	if c.SlackEnabled, err = strconv.ParseBool(get("KUBECHAT_SLACK_ENABLED", "false")); err != nil {
		return Config{}, fmt.Errorf("KUBECHAT_SLACK_ENABLED: want true or false")
	}
	if c.SlackEnabled {
		c.SlackAppToken = get("KUBECHAT_SLACK_APP_TOKEN", "")
		c.SlackBotToken = get("KUBECHAT_SLACK_BOT_TOKEN", "")
		if !strings.HasPrefix(c.SlackAppToken, "xapp-") {
			return Config{}, fmt.Errorf("Slack is enabled: KUBECHAT_SLACK_APP_TOKEN must be an app-level token (xapp-…)")
		}
		if !strings.HasPrefix(c.SlackBotToken, "xoxb-") {
			return Config{}, fmt.Errorf("Slack is enabled: KUBECHAT_SLACK_BOT_TOKEN must be a bot token (xoxb-…)")
		}
	}
	return c, nil
}
