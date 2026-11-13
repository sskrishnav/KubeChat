// Package gate is the consent gate: every tool call from the agent passes
// through it, and only the gate can reach the MCP server.
//
// Reads run. Everything else is refused for now; writes will run after a
// human approves them.
package gate

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/sskrishnav/kubechat/internal/app/policy"
	"github.com/sskrishnav/kubechat/internal/core"
)

type Gate struct {
	Tools  core.Tools // the MCP server
	Policy policy.Policy

	mu    sync.RWMutex
	specs map[string]core.ToolSpec // from the last List
}

// List returns the MCP server's tools and remembers them, so Call can check
// each call against the tool's hints.
func (g *Gate) List(ctx context.Context) ([]core.ToolSpec, error) {
	specs, err := g.Tools.List(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]core.ToolSpec, len(specs))
	for _, s := range specs {
		byName[s.Name] = s
	}
	g.mu.Lock()
	g.specs = byName
	g.mu.Unlock()
	return specs, nil
}

// Call runs a tool only if the policy allows it without approval.
func (g *Gate) Call(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	g.mu.RLock()
	spec, known := g.specs[call.Name]
	g.mu.RUnlock()
	if !known {
		return refuse(call, "unknown tool")
	}

	d := g.Policy.Decide(spec, call.Args)
	switch {
	case d.Refused != "":
		return refuse(call, d.Refused)
	case d.NeedsApproval:
		return refuse(call, "this change needs human approval, which is not available yet")
	}

	slog.Info("tool call", "tool", call.Name, "tier", d.Tier, "args", string(call.Args))
	return g.Tools.Call(ctx, call)
}

func refuse(call core.ToolCall, reason string) (core.ToolResult, error) {
	slog.Info("tool refused", "tool", call.Name, "args", string(call.Args), "reason", reason)
	return core.ToolResult{}, fmt.Errorf("refused: %s", reason)
}
