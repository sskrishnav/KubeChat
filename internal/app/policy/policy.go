// Package policy decides what a tool call may do. No I/O, easy to test.
package policy

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/sskrishnav/kubechat/internal/core"
)

// Policy holds the rules from config.
type Policy struct {
	Overrides           map[string]core.Tier // tool name -> tier; beats the server's hints
	ProtectedNamespaces []string             // patterns like "kube-*": never changed
}

// Tier puts a tool in a tier: the config override if there is one, otherwise
// the server's hints. Anything unclear counts as destructive (fail closed).
func (p Policy) Tier(spec core.ToolSpec) core.Tier {
	if t, ok := p.Overrides[spec.Name]; ok {
		return t
	}
	h := spec.Hints
	switch {
	case h == nil:
		return core.TierDestructive
	case h.ReadOnly:
		return core.TierRead
	case h.Destructive != nil && !*h.Destructive:
		return core.TierWrite
	default:
		return core.TierDestructive
	}
}

// Decision is what to do with one tool call.
type Decision struct {
	Tier          core.Tier
	NeedsApproval bool   // true: ask a human first
	Refused       string // non-empty: never run, and why
}

// Decide checks a call to the tool spec with the given JSON arguments.
// Reads run at once. Writes need approval and an explicit namespace that is
// not protected. Everything else is refused (the MVP has no deletes).
func (p Policy) Decide(spec core.ToolSpec, args json.RawMessage) Decision {
	tier := p.Tier(spec)
	switch tier {
	case core.TierRead:
		return Decision{Tier: tier}
	case core.TierWrite:
		var a struct {
			Namespace string `json:"namespace"`
		}
		_ = json.Unmarshal(args, &a) // bad JSON leaves Namespace empty: refused below
		switch {
		case a.Namespace == "":
			// Without a namespace the MCP server uses its own default, which we can't check.
			return Decision{Tier: tier, Refused: "a change must name its namespace"}
		case p.protected(a.Namespace):
			return Decision{Tier: tier, Refused: fmt.Sprintf("namespace %q is protected", a.Namespace)}
		}
		return Decision{Tier: tier, NeedsApproval: true}
	default:
		return Decision{Tier: tier, Refused: fmt.Sprintf("tool %q is %s", spec.Name, tier)}
	}
}

// protected reports whether ns matches a protected pattern. A broken pattern
// counts as a match (fail closed); config.Load rejects them anyway.
func (p Policy) protected(ns string) bool {
	for _, pattern := range p.ProtectedNamespaces {
		if ok, err := path.Match(pattern, ns); ok || err != nil {
			return true
		}
	}
	return false
}
