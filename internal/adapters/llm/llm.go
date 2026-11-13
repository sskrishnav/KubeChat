// Package llm talks to an OpenAI-compatible chat API (Ollama's /v1) and
// implements core.LLM. Plain net/http: we only need one endpoint.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sskrishnav/kubechat/internal/core"
)

type Client struct {
	BaseURL string // e.g. http://localhost:11434/v1
	Model   string
	HTTP    *http.Client
}

func New(baseURL, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		HTTP:    &http.Client{Timeout: 5 * time.Minute}, // local models can be slow
	}
}

// The parts of the /v1/chat/completions format we use. Fields we don't need,
// such as the model's "reasoning" text, are simply not decoded.
type (
	message struct {
		Role       string     `json:"role"`
		Content    string     `json:"content"`
		ToolCalls  []toolCall `json:"tool_calls,omitempty"`
		ToolCallID string     `json:"tool_call_id,omitempty"`
	}
	toolCall struct {
		ID       string   `json:"id"`
		Type     string   `json:"type"`
		Function function `json:"function"`
	}
	function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // a JSON object, encoded as a string
	}
	tool struct {
		Type     string  `json:"type"`
		Function toolDef `json:"function"`
	}
	toolDef struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}
	request struct {
		Model       string    `json:"model"`
		Messages    []message `json:"messages"`
		Tools       []tool    `json:"tools,omitempty"`
		Temperature float64   `json:"temperature"`
	}
	response struct {
		Choices []struct {
			Message message `json:"message"`
		} `json:"choices"`
	}
)

// Complete sends the conversation and the tool list, and returns the model's reply.
func (c *Client) Complete(ctx context.Context, history []core.Message, tools []core.ToolSpec) (core.Message, error) {
	req := request{Model: c.Model, Temperature: 0}
	for _, m := range history {
		msg := message{Role: m.Role, Content: m.Text, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, toolCall{
				ID: tc.ID, Type: "function", Function: function{Name: tc.Name, Arguments: string(tc.Args)},
			})
		}
		req.Messages = append(req.Messages, msg)
	}
	for _, t := range tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}
		req.Tools = append(req.Tools, tool{Type: "function", Function: toolDef{Name: t.Name, Description: t.Description, Parameters: params}})
	}

	body, err := json.Marshal(req)
	if err != nil {
		return core.Message{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return core.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return core.Message{}, fmt.Errorf("call LLM at %s: %w", c.BaseURL, err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1000))
		return core.Message{}, fmt.Errorf("LLM returned %s: %s", httpResp.Status, snippet)
	}

	var resp response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return core.Message{}, fmt.Errorf("decode LLM reply: %w", err)
	}
	if len(resp.Choices) == 0 {
		return core.Message{}, fmt.Errorf("LLM reply has no choices")
	}

	m := resp.Choices[0].Message
	reply := core.Message{Role: "assistant", Text: strings.TrimSpace(m.Content)}
	for i, tc := range m.ToolCalls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		args := json.RawMessage(tc.Function.Arguments)
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		reply.ToolCalls = append(reply.ToolCalls, core.ToolCall{ID: id, Name: tc.Function.Name, Args: args})
	}
	return reply, nil
}
