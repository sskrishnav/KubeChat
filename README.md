# KubeChat
Ask questions about your Kubernetes cluster in plain English, in Slack or the terminal.
It reads the cluster through MCP servers (kubernetes-mcp-server and K8sGPT) and a local LLM (Ollama). It never changes anything.

## Quick start (macOS, Docker Desktop)
Needs: Docker, kind, kubectl, helm, Ollama.

```bash
ollama pull qwen3:14b   # 9 GB
git clone https://github.com/sskrishnav/KubeChat.git && cd KubeChat
make up      # kind cluster + demo apps + MCP server + bot, about 3 minutes
```

Ask a question:
```bash
kubectl --kubeconfig dev/kubeconfig -n kubechat exec deploy/kubechat-bot -- \
  /kubechat -ask "why are pods failing in apps?"
```

`make down` deletes everything. Your `~/.kube/config` is never touched: all commands use `dev/kubeconfig`.

## Slack (optional)
1. At [api.slack.com/apps](https://api.slack.com/apps), create an app and turn on **Socket Mode**. Create an app-level token (`xapp-…`) with `connections:write`.
2. Bot scopes: `app_mentions:read`, `chat:write`, `channels:history`. Events: `app_mention`, `message.channels`.
3. Install the app to your workspace (this gives the bot token, `xoxb-…`) and invite it to a channel.
4. Create `.env` (git-ignored), run `chmod 600 .env`, then `make dev`:
   ```
   KUBECHAT_SLACK_ENABLED=true
   KUBECHAT_SLACK_APP_TOKEN=xapp-...
   KUBECHAT_SLACK_BOT_TOKEN=xoxb-...
   ```
5. In the channel: `@KubeChat why are pods failing in apps?`, and reply in the thread to follow up.

## More
`make` lists the commands. How it works: [docs/bot.md](docs/bot.md), [docs/architecture.md](docs/architecture.md).
