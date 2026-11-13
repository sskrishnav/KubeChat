package core

import "context"

// The interfaces the bot uses to reach the outside world. Adapters implement
// them (Ollama, MCP), so the rest of the code never imports those libraries
// and tests can use simple fakes.

// LLM returns the model's next message: an answer, or tool calls to run.
type LLM interface {
	Complete(ctx context.Context, history []Message, tools []ToolSpec) (Message, error)
}

// Tools lists and runs tools. The agent always gets the consent-gated
// version, never the MCP adapter directly.
type Tools interface {
	List(ctx context.Context) ([]ToolSpec, error)
	Call(ctx context.Context, c ToolCall) (ToolResult, error)
}
