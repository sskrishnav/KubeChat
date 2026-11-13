// Command kubechat is the KubeChat bot.
//
//	kubechat -ask "why are pods failing in apps?"   answer one question and exit
//	kubechat                                       run as a service
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sskrishnav/kubechat/internal/adapters/llm"
	"github.com/sskrishnav/kubechat/internal/adapters/mcp"
	"github.com/sskrishnav/kubechat/internal/app/agent"
	"github.com/sskrishnav/kubechat/internal/app/gate"
	"github.com/sskrishnav/kubechat/internal/app/policy"
	"github.com/sskrishnav/kubechat/internal/config"
)

const maxToolOutput = 20000 // bytes of each tool result sent to the LLM

func main() {
	ask := flag.String("ask", "", "answer one question in the terminal and exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("bad config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wiring: the agent only ever sees the gate; only the gate sees the MCP server.
	bot := &agent.Agent{
		LLM: llm.New(cfg.LLMBaseURL, cfg.LLMModel),
		Gate: &gate.Gate{
			Server: &mcp.Client{URL: cfg.MCPURL},
			Policy: policy.Policy{Overrides: cfg.ToolTiers, ProtectedNamespaces: cfg.ProtectedNamespaces},
		},
		MaxRounds: cfg.MaxToolRounds,
		MaxOutput: maxToolOutput,
	}

	if *ask != "" {
		answer, err := bot.Ask(ctx, *ask)
		if err != nil {
			slog.Error("no answer", "err", err)
			os.Exit(1)
		}
		fmt.Println(answer)
		return
	}

	slog.Info("kubechat started",
		"llm", cfg.LLMBaseURL, "model", cfg.LLMModel, "mcp", cfg.MCPURL,
		"protected_namespaces", cfg.ProtectedNamespaces, "tool_tiers", cfg.ToolTiers)
	<-ctx.Done() // until Kubernetes stops the pod (SIGTERM) or Ctrl-C
	slog.Info("kubechat stopped")
}
