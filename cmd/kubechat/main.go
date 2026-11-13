// Command kubechat is the KubeChat bot.
//
// Step 5: it loads its settings, logs them and waits to be stopped.
// Step 6 adds MCP and Ollama, step 7 Slack, step 8 the Approve card.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sskrishnav/kubechat/internal/config"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("bad config", "err", err)
		os.Exit(1)
	}
	slog.Info("kubechat started",
		"llm", cfg.LLMBaseURL, "model", cfg.LLMModel, "mcp", cfg.MCPURL,
		"protected_namespaces", cfg.ProtectedNamespaces, "tool_tiers", cfg.ToolTiers)

	// Run until Kubernetes stops the pod (SIGTERM) or Ctrl-C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	slog.Info("kubechat stopped")
}
