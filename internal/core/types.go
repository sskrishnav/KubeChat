// Package core holds KubeChat's shared types and interfaces. It imports only
// the standard library (checked by imports_test.go), so it never depends on
// Slack, MCP or an LLM library. Types are added step by step, as they are used.
package core

import "encoding/json"

// Message is one entry in the conversation sent to the LLM.
type Message struct {
	Role       string // "system", "user", "assistant" or "tool"
	Text       string
	ToolCalls  []ToolCall // assistant: tools the model wants to run
	ToolCallID string     // tool: which call this result answers
}

// ToolSpec describes one tool, as listed by the MCP server.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema of the arguments
	Hints       *ToolHints      // nil when the server sent no hints
}

// ToolHints are the MCP server's own hints about a tool. Only hints:
// the policy decides what a tool may do.
type ToolHints struct {
	ReadOnly    bool
	Destructive *bool // nil means not set (MCP treats that as true)
}

// ToolCall is a request to run one tool.
type ToolCall struct {
	ID   string          // from the LLM; links the result to the call
	Name string          // tool name
	Args json.RawMessage // JSON object
}

// ToolResult is what a tool returned.
type ToolResult struct {
	Text    string
	IsError bool
}

// Tier is how much a tool may change. The policy assigns it.
type Tier string

const (
	TierRead        Tier = "read"        // runs at once
	TierWrite       Tier = "write"       // runs only after a human approves
	TierDestructive Tier = "destructive" // deletes and similar; not allowed in the MVP
	TierBlocked     Tier = "blocked"     // never runs
)
