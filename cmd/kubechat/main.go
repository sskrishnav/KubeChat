// Command kubechat is the KubeChat bot.
//
//	kubechat -ask "why are pods failing in apps?"   answer one question and exit
//	kubechat                                       run as a service (answers in Slack when enabled)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sskrishnav/kubechat/internal/adapters/llm"
	"github.com/sskrishnav/kubechat/internal/adapters/mcp"
	"github.com/sskrishnav/kubechat/internal/adapters/slack"
	"github.com/sskrishnav/kubechat/internal/app/agent"
	"github.com/sskrishnav/kubechat/internal/app/gate"
	"github.com/sskrishnav/kubechat/internal/app/policy"
	"github.com/sskrishnav/kubechat/internal/config"
	"github.com/sskrishnav/kubechat/internal/core"
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

	servers := &mcp.Servers{}
	for _, s := range cfg.MCPServers {
		servers.All = append(servers.All, &mcp.Client{Name: s.Name, URL: s.URL, ReadOnly: s.ReadOnly})
	}
	rules := policy.Policy{Overrides: cfg.ToolTiers, ProtectedNamespaces: cfg.ProtectedNamespaces}

	// Wiring: the agent only ever sees the gate; only the gate sees the MCP servers.
	bot := &agent.Agent{
		LLM:       llm.New(cfg.LLMBaseURL, cfg.LLMModel),
		Gate:      &gate.Gate{Server: servers, Policy: rules},
		MaxRounds: cfg.MaxToolRounds,
		MaxOutput: maxToolOutput,
	}

	if *ask != "" {
		answer, err := bot.Ask(ctx, nil, *ask)
		if err != nil {
			slog.Error("no answer", "err", err)
			os.Exit(1)
		}
		fmt.Println(answer)
		return
	}

	slog.Info("kubechat started",
		"llm", cfg.LLMBaseURL, "model", cfg.LLMModel, "mcp_servers", cfg.MCPServers,
		"protected_namespaces", cfg.ProtectedNamespaces, "tool_tiers", cfg.ToolTiers,
		"slack", cfg.SlackEnabled)

	go logTools(ctx, bot.Gate, rules)
	if cfg.SlackEnabled {
		chat := &slack.Bot{AppToken: cfg.SlackAppToken, BotToken: cfg.SlackBotToken, Ask: bot.Ask}
		if err := chat.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("Slack connection failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Slack is disabled: only -ask works (set KUBECHAT_SLACK_ENABLED=true to enable it)")
		<-ctx.Done() // until Kubernetes stops the pod (SIGTERM) or Ctrl-C
	}
	slog.Info("kubechat stopped")
}

// logTools logs the tools the LLM gets and the tier the gate gives each one.
// The MCP servers may still be starting, so it tries for about a minute.
func logTools(ctx context.Context, runner core.ToolRunner, rules policy.Policy) {
	var err error
	for range 12 {
		var tools []core.ToolSpec
		if tools, err = runner.List(ctx); err == nil {
			for _, t := range tools {
				slog.Info("tool available", "server", t.Server, "name", t.Name, "tier", rules.Tier(t))
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
	slog.Warn("can't list the tools; trying again on every question", "err", err)
}
