// Package mcp talks to the Kubernetes MCP server and implements core.Tools.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sskrishnav/kubechat/internal/core"
)

// Client opens a short MCP session for every request. That costs one extra
// round trip inside the cluster, but there is no long-lived connection to
// break when the MCP server pod restarts.
type Client struct {
	URL string // e.g. http://kubechat-mcp:8080/mcp
}

func (c *Client) connect(ctx context.Context) (*sdk.ClientSession, error) {
	client := sdk.NewClient(&sdk.Implementation{Name: "kubechat", Version: "dev"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             c.URL,
		DisableStandaloneSSE: true, // we only need replies to our own requests
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP server %s: %w", c.URL, err)
	}
	return session, nil
}

// List returns the server's tools with their hints.
func (c *Client) List(ctx context.Context) ([]core.ToolSpec, error) {
	session, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	res, err := session.ListTools(ctx, &sdk.ListToolsParams{}) // one page holds our few tools
	if err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	specs := make([]core.ToolSpec, 0, len(res.Tools))
	for _, t := range res.Tools {
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %s: bad input schema: %w", t.Name, err)
		}
		spec := core.ToolSpec{Name: t.Name, Description: t.Description, InputSchema: schema}
		if a := t.Annotations; a != nil {
			spec.Hints = &core.ToolHints{ReadOnly: a.ReadOnlyHint, Destructive: a.DestructiveHint}
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// Call runs one tool and returns its text output.
func (c *Client) Call(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	session, err := c.connect(ctx)
	if err != nil {
		return core.ToolResult{}, err
	}
	defer session.Close()

	args := call.Args
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: call.Name, Arguments: args})
	if err != nil {
		return core.ToolResult{}, fmt.Errorf("call %s: %w", call.Name, err)
	}
	var text strings.Builder
	for _, content := range res.Content {
		if t, ok := content.(*sdk.TextContent); ok {
			text.WriteString(t.Text)
		}
	}
	return core.ToolResult{Text: text.String(), IsError: res.IsError}, nil
}
