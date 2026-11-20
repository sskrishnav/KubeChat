package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/sskrishnav/kubechat/internal/core"
)

// Servers joins several MCP servers into one core.ToolRunner: List returns
// the tools of all of them, and Call sends each call to the server that has
// the tool.
type Servers struct {
	All []core.ToolRunner

	mu    sync.RWMutex
	owner map[string]core.ToolRunner // tool name -> its server, from the last List
}

// List fails if any server fails, or if two servers have a tool with the same
// name, since a call to it would be ambiguous.
func (s *Servers) List(ctx context.Context) ([]core.ToolSpec, error) {
	var specs []core.ToolSpec
	owner := map[string]core.ToolRunner{}
	for _, server := range s.All {
		list, err := server.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, spec := range list {
			if _, dup := owner[spec.Name]; dup {
				return nil, fmt.Errorf("two MCP servers have a tool named %q", spec.Name)
			}
			owner[spec.Name] = server
			specs = append(specs, spec)
		}
	}
	s.mu.Lock()
	s.owner = owner
	s.mu.Unlock()
	return specs, nil
}

func (s *Servers) Call(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	s.mu.RLock()
	server, ok := s.owner[call.Name]
	s.mu.RUnlock()
	if !ok {
		return core.ToolResult{}, fmt.Errorf("no MCP server has a tool named %q", call.Name)
	}
	return server.Call(ctx, call)
}
