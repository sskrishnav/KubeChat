package core

import "context"

// The interfaces the bot uses to reach the outside world. Adapters implement
// them (Ollama, MCP), so the rest of the code never imports those libraries
// and tests can use simple fakes.

// LLM returns the model's next message: an answer, or tool calls to run.
type LLM interface {
	Complete(ctx context.Context, history []Message, tools []ToolSpec) (Message, error)
}

// ToolRunner lists the MCP tools and runs one. Two types implement it: the
// MCP client (runs the tool) and the gate (checks the call, then hands it to
// the MCP client). The agent only ever gets the gate.
type ToolRunner interface {
	List(ctx context.Context) ([]ToolSpec, error)
	Call(ctx context.Context, c ToolCall) (ToolResult, error)
}
