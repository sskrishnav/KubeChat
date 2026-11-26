# KubeChat: design

Slack-driven Kubernetes debugging via MCP · v0.1.0 · 26 Nov 2026 · @Krishna

## Idea

KubeChat is a small Go bot that answers questions about a Kubernetes cluster in plain
English ("why are pods failing in apps?"), in Slack or the terminal. It answers from live
cluster data, read through MCP servers, with a local LLM. **v0.1.0 is read-only:** it
never changes the cluster.

It reuses existing projects and adds only the bot, the agent loop and the consent gate:

| Need | Project |
|---|---|
| Kubernetes access | [kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) |
| Rule-based health checks | [K8sGPT](https://github.com/k8sgpt-ai/k8sgpt) analyzers, through its MCP server |
| LLM | [Ollama](https://ollama.com) on the laptop, OpenAI-compatible `/v1` API |

> **Core rule: the LLM never holds write access.** Every tool call goes through the
> consent gate. There is no bypass, not even for debugging.

## What v0.1.0 has

- **Slack**, optional and off by default: Socket Mode (no public URL). An @mention starts a thread,
  and replies in the thread are follow-up questions with the earlier ones as context.
- **Terminal:** `kubechat -ask "…"`.
- **Agent loop:** the LLM asks for tools, the gate runs them, until it answers. The prompt is a
  general way to investigate, not hints for particular faults.
- **MCP servers from config** (`KUBECHAT_MCP_SERVERS`): kubernetes-mcp-server and K8sGPT.
  The LLM gets all their tools and picks.
- **Consent gate:** each tool gets a tier (read, write, destructive, blocked) from the
  server's hints, config overrides, or a read-only server setting. Reads run; everything
  else is refused for now.
- **One Helm chart** (MCP server, K8sGPT, bot) and a kind playground with three planted
  faults: `make up`.
- **Experiments** (`experiments/`): 15 questions of different kinds, scored after every
  change to the model, the prompt or the tools.

How it fits together: [architecture.md](architecture.md). How the code works: [bot.md](bot.md).

## Safety

Three layers, each enough on its own to stop a write:

| Layer | What it enforces |
|---|---|
| Kubernetes RBAC | The bot has no Kubernetes permissions. kubernetes-mcp-server: `view` + cluster-read (nodes, PVs, storage classes, webhooks) + scale. K8sGPT: `view` + cluster-read only. `view` excludes Secrets. |
| MCP server config | kubernetes-mcp-server: an `enabled_tools` list, and Secrets denied. |
| Consent gate | Unknown or unclear tools count as destructive (fail closed); writes need a namespace that isn't protected, then approval, and approval isn't built yet. |

Tool output is untrusted data: the prompt says never to follow instructions found in it,
and it's redacted (tokens, keys, passwords) and size-capped before the LLM sees it.

## Decisions, and why

| Decision | Why |
|---|---|
| Local model (Ollama), qwen3:14b | Cluster data stays on the laptop. 14b scored 11/15 against 9/15 for 8b, with nothing invented; it's about 3 times slower. |
| K8sGPT as a second MCP server, all tools offered | `analyze` finds known problems in one call. K8sGPT also reads nodes and the cluster version. |
| All K8sGPT tools count as reads (`KUBECHAT_MCP_READ_ONLY`) | Its MCP library marks every tool destructive by default, so the hints mean nothing. Its RBAC can only read. |
| Our own K8sGPT template, not their chart | Their chart isn't published, and its role reads everything, Secrets included. |
| Settings as environment variables | No config library needed; the chart renders them into a ConfigMap and a Secret. |
| Writes deferred | Read-only answers first. When writes come back, anyone in the thread may approve. |

## Known limits

- **Invented answers after an empty filtered list.** The model filters on a wrong field or an
  exact name, gets nothing, and then guesses (questions 9, 11 and 12 in `experiments/`).
  Prompt rules didn't fix it, so the next fix is in code.
- **Speed:** about 1 to 2 minutes per answer with 14b, up to 5 on hard questions.
- **Thread memory is in the bot's memory:** a restart forgets it.
- **One cluster**, and Ollama must be running on the laptop.

## Roadmap

| Next | What |
|---|---|
| Accuracy | When a filtered list comes back empty, tell the model right in that tool result; measure over several runs. |
| Writes | Approve/Reject card in the thread, bound to the exact change, single use, 10-minute expiry. |
| v0.2 | Separate reader and writer MCP servers with their own RBAC; a diff before each change; thread history in SQLite. |
| v0.3 | `/k8s` slash command and a CLI. |
| v0.4 | NetworkPolicy, metrics. |
| v0.5 | Destructive tier, several clusters, tracing. |
