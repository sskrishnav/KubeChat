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
	MCPURL              string               // KUBECHAT_MCP_URL
	ProtectedNamespaces []string             // KUBECHAT_PROTECTED_NAMESPACES, e.g. "kube-*,kubechat"
	ToolTiers           map[string]core.Tier // KUBECHAT_TOOL_TIERS, e.g. "resources_scale=write"
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
		LLMModel:   get("KUBECHAT_LLM_MODEL", "qwen3:8b"),
		MCPURL:     get("KUBECHAT_MCP_URL", "http://localhost:8080/mcp"),
		ToolTiers:  map[string]core.Tier{},
	}

	rounds, err := strconv.Atoi(get("KUBECHAT_MAX_TOOL_ROUNDS", "8"))
	if err != nil || rounds < 1 {
		return Config{}, fmt.Errorf("KUBECHAT_MAX_TOOL_ROUNDS: want a positive number")
	}
	c.MaxToolRounds = rounds

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
	return c, nil
}
