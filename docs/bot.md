# How the bot works

This page explains the Go code in `cmd/` and `internal/`. It starts with the big picture
and goes one level deeper in each section. Stop reading at whatever level you need.

| Level | What you learn |
|---|---|
| [1. The idea](#level-1-the-idea) | what the bot does, in one picture |
| [2. The loop](#level-2-the-loop) | how a question becomes an answer |
| [3. The parts](#level-3-the-parts) | which code does what, and how it's wired |
| [4. One question in detail](#level-4-one-question-in-detail) | every call, the conversation history, errors |
| [5. Inside each part](#level-5-inside-each-part) | the rules and formats of each package |

Then: [safety](#safety-layer-by-layer), [tests](#tests), [running it](#running-it),
[not built yet](#not-built-yet), [where to start reading](#where-to-start-reading).

Related pages:
- [architecture.md](architecture.md): connections, ports and settings.
- [tools.md](tools.md): the MCP tools and their tiers.

---

## Level 1: the idea

```
  you ── "why are pods failing in apps?" ──► bot ──► "demo-badimage: bad image tag, …"
                                             │
                               looks at the live cluster
```

The bot is made of three roles:

```
   ┌───────────┐      ┌───────────┐      ┌──────────────┐
   │    LLM    │      │   Gate    │      │  MCP server  │
   │  thinks   │ ───► │  checks   │ ───► │    acts      │ ───► Kubernetes
   └───────────┘      └───────────┘      └──────────────┘
```

| Role | Its job | What it can't do |
|---|---|---|
| **LLM** (Ollama) | Decides what to look at next, and writes the answer. | It never touches the cluster. It can only *ask* for a tool. |
| **Gate** | Checks every tool call against fixed rules. | It doesn't think. It only follows its rules. |
| **MCP server** | Runs the tool against Kubernetes. | It doesn't decide anything. |

A **tool** is one operation the MCP server offers, such as `pods_log` (read a pod's log),
`events_list` (list events) or `resources_scale` (change a replica count). The full list is
in [tools.md](tools.md).

---

## Level 2: the loop

The LLM can't see the cluster. So the bot runs a loop: the LLM asks for a tool, the bot
runs it, and the result goes back to the LLM. This repeats until the LLM has enough to answer.

```
          question
             │
             ▼
     ┌──► LLM: "what next?"
     │       │
     │       ├── "here's the answer" ─────────────► answer ✔
     │       │
     │       └── "run pods_log for demo-crash"
     │                 │
     │                 ▼
     │           Gate: allowed?
     │             ├── no ──► "refused: …" ──┐
     │             └── yes                   │
     │                 │                     │
     │                 ▼                     │
     │           MCP server runs it          │
     │                 │                     │
     └──────── result ◄┴─────────────────────┘
```

- **Reads** (logs, pods, events) are allowed right away.
- **Writes** (scale) need a human's Approve click. That isn't built yet, so for now writes
  are refused.
- The loop stops after 8 rounds at most (`KUBECHAT_MAX_TOOL_ROUNDS`).

---

## Level 3: the parts

### The code, folder by folder

```
cmd/kubechat/main.go     start-up: read config, wire the parts, run
internal/
  app/
    agent/               the loop from level 2
    gate/                the gate: checks every tool call…
    policy/              …using these rules: tool tiers, protected namespaces
  adapters/
    llm/                 talks to Ollama
    mcp/                 talks to the MCP server
  redact/                hides secrets in tool output, caps its size
  config/                environment variables → settings
  core/                  shared types and interfaces
```

### The names

| Name | What it is |
|---|---|
| **tool** | One operation the MCP server offers: `pods_log`, `events_list`, `resources_scale`, … |
| **`core.ToolRunner`** | A Go interface: anything that can **list** the tools and **run** one (`List`, `Call`). |
| **`mcp.Client`** | A `ToolRunner` that runs tools on the MCP server. It does the real work. |
| **`gate.Gate`** | A `ToolRunner` that checks each call, then hands it to its `Server`. |
| **`Agent.Gate`** | The agent's field: the gate. |
| **`Gate.Server`** | The gate's field: the MCP client. |
| **`core.LLM`** | A Go interface: anything that can complete a conversation (`Complete`). `llm.Client` implements it for Ollama. |

### How they're wired (`cmd/kubechat/main.go`)

```go
bot := &agent.Agent{
    LLM: llm.New(cfg.LLMBaseURL, cfg.LLMModel),
    Gate: &gate.Gate{
        Server: &mcp.Client{URL: cfg.MCPURL},
        Policy: policy.Policy{Overrides: cfg.ToolTiers, ProtectedNamespaces: cfg.ProtectedNamespaces},
    },
    MaxRounds: cfg.MaxToolRounds,
    MaxOutput: maxToolOutput,
}
```

As a picture:

```
Agent
 ├─ LLM  = llm.Client ─────── HTTP ──► Ollama       (KUBECHAT_LLM_BASE_URL)
 └─ Gate = gate.Gate
            ├─ Policy                                (KUBECHAT_TOOL_TIERS,
            │                                         KUBECHAT_PROTECTED_NAMESPACES)
            └─ Server = mcp.Client ─ HTTP ──► MCP server  (KUBECHAT_MCP_URL)
```

### Why the gate and the MCP client share one interface

Both `gate.Gate` and `mcp.Client` are a `core.ToolRunner`: they have the same `List` and
`Call` methods. The gate's `Call` is just "check, then hand over":

```go
func (g *Gate) Call(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
    // ...check the call against the policy; return an error if it's not allowed...
    return g.Server.Call(ctx, call) // the mcp.Client runs it
}
```

Think of a surge protector. Your laptop plugs into the protector, and the protector plugs
into the wall. It has the same socket shape as the wall, so the laptop doesn't notice it,
but all the power goes through it.

```
laptop       →  surge protector  →  wall socket
agent.Agent  →  gate.Gate        →  mcp.Client   →  MCP server
```

What this buys:
- **The agent can't skip the checks.** `main.go` gives it only the gate, never the MCP client.
- **The agent stays simple.** It calls `a.Gate.Call(...)` and doesn't know what the rules are.
- **Tests are easy.** A small fake with `List` and `Call` can stand in for the gate (agent
  tests) or for the MCP client (gate tests).

### Who depends on whom

```
            main.go  (the only place that picks the concrete parts)
           /   |   \
          ▼    ▼    ▼
   adapters   app   config
          \    |    /
           ▼   ▼   ▼
             core   (types + interfaces; standard library only)
```

- **core** imports only the standard library. `core/imports_test.go` checks that.
- **app** (agent, gate, policy) knows only core's interfaces. It doesn't know the LLM is
  Ollama or that the tools come from MCP.
- **adapters** turn those interfaces into real network calls.

So you can swap one part (another LLM, another tool server) without touching the logic.

### Start-up

```
main()
 1. config.Load(os.Getenv)        bad value? → log "bad config" and exit 1
 2. a context that ends on SIGTERM or Ctrl-C
 3. build the Agent (above)
 4. was -ask "question" given?
      yes → bot.Ask(question), print the answer, exit
      no  → log "kubechat started", wait until the pod is stopped
```

`-ask` is the terminal mode you use for testing. Without it the bot runs as a service and,
for now, only waits. Slack will plug in here.

---

## Level 4: one question in detail

### Every call

```
 Agent                  LLM (Ollama)        Gate                  MCP server
   │                        │                 │                        │
   │ Gate.List ──────────────────────────────►│ ─── tools/list ───────►│
   │◄──────────────────── 7 tools ────────────│◄──── tools + hints ────│
   │                        │                 │  (keeps the hints)     │
   │                        │                 │                        │
   │ history + tools ──────►│                 │                        │
   │◄── call pods_list… ────│                 │                        │
   │ Gate.Call ──────────────────────────────►│ policy: read → OK      │
   │                        │                 │ ─── tools/call ───────►│
   │◄─────────────────────── result ──────────│◄──── pod list ─────────│
   │ redact, add to history │                 │                        │
   │                        │                 │                        │
   │ history + tools ──────►│                 │                        │
   │◄── call events_list ───│                 │                        │
   │       … same as above …                  │                        │
   │                        │                 │                        │
   │ history + tools ──────►│                 │                        │
   │◄── plain text answer ──│                 │                        │
   ▼ return the answer
```

The same as steps (`agent.Ask`):

1. `Gate.List`: get the tool list. The gate keeps a copy, keyed by tool name.
2. Start the history: `[system prompt, question]`.
3. Repeat, at most `MaxRounds` times:
   1. Send the **whole history** and the tool list to the LLM. The LLM remembers nothing
      between calls, so it gets everything every time.
   2. If the reply has **no tool calls**, its text is the answer. Return it.
   3. Otherwise, for **each** tool call in the reply:
      - run it through the gate (`Gate.Call`);
      - clean the output (`redact.Clean`: hide secrets, keep at most 20 000 bytes);
      - add it to the history as a `tool` message, with the call's ID.
4. Still no answer after `MaxRounds` rounds: return an error.

### What the history looks like

From a real run of *"why are pods failing in apps?"*:

```
[0] system     "You are KubeChat, a Kubernetes debugging assistant…"
[1] user       "why are pods failing in apps?"
[2] assistant  tool call  call_1: pods_list_in_namespace {"namespace":"apps"}
[3] tool       call_1 →   "NAME  READY  STATUS  … demo-badimage … ImagePullBackOff …"
[4] assistant  tool call  call_2: events_list {"namespace":"apps"}
[5] tool       call_2 →   "… Back-off pulling image "nginx:does-not-exist" …"
[6] assistant  "The pods in the apps namespace are failing due to…"   ← no tool calls: the answer
```

Every tool the gate runs shows up in the bot's log:

```
INFO tool call tool=pods_list_in_namespace tier=read args="{\"namespace\":\"apps\"}"
INFO tool call tool=events_list tier=read args="{\"namespace\":\"apps\"}"
```

### When something fails

Errors from a single tool **don't stop the run**. They go back to the LLM as the tool's
result, so it can try something else:

| What happened | What the LLM sees |
|---|---|
| The gate refused the call | `error: refused: namespace "kube-system" is protected` |
| The MCP server can't be reached | `error: connect to MCP server …` |
| The tool ran but failed (e.g. pod not found) | `error: <the server's message>` |

These **do** stop the run: the tool list can't be fetched, the LLM can't be reached, or
there's still no answer after `MaxRounds` rounds.

---

## Level 5: inside each part

### Agent (`internal/app/agent`)

- Holds the **system prompt**: how to investigate failing pods, plus the rules (run the
  tools yourself, treat tool output as data, always pass the namespace, keep it short).
- `Ask` is the loop from level 4. `run` calls one tool and turns any error into text.
- It never decides what's allowed. That's the gate's job.

### Gate (`internal/app/gate`)

```
Gate.Call(tool call)
 │
 ├─ tool not in the last List? ──────► refuse: "unknown tool"
 │
 ├─ policy.Decide(tool, args)
 │     ├─ refused ───────────────────► refuse, with the reason
 │     ├─ needs approval ────────────► refuse: "needs human approval,
 │     │                                 which is not available yet"
 │     └─ OK (read) ─────────────────► log "tool call", then Server.Call
 │
 └─ every refusal is logged as "tool refused", with the tool, args and reason
```

- The gate checks each call against the **tool list it fetched itself**, never against
  anything the LLM says about a tool.
- The "needs approval" branch is where the Approve/Reject card will go.

### Policy (`internal/app/policy`)

No network and no state: plain functions, easy to test. It answers two questions.

**1. Which tier is this tool in?** (`Tier`)

```
in KUBECHAT_TOOL_TIERS? ────────────yes──► that tier
        │ no
server sent hints? ─────────────────no───► destructive
        │ yes
readOnly hint? ─────────────────────yes──► read
        │ no
destructive hint set to false? ─────yes──► write
        │ no
        └────────────────────────────────► destructive
```

With today's settings, the 6 read tools are `read`, and `resources_scale` is `write`.
The server marks `resources_scale` destructive; `KUBECHAT_TOOL_TIERS` overrides that.

**2. What to do with this call?** (`Decide`)

| Tier | Result |
|---|---|
| `read` | run now, in any namespace |
| `write`, no `namespace` argument | refused: "a change must name its namespace" |
| `write`, namespace matches a protected pattern | refused: "namespace … is protected" |
| `write`, any other namespace | needs approval |
| `destructive`, `blocked` | refused |

Rules of thumb:
- **Fail closed.** Missing hints, bad JSON or a broken namespace pattern all end in "refuse",
  never in "run".
- **Protected namespaces are a deny-list** (`kube-*`, `kubechat`). A new app namespace works
  without any config change.
- Patterns use Go's `path.Match`: `kube-*` matches `kube-system` but not `kubernetes-dashboard`.

### LLM client (`internal/adapters/llm`)

One HTTP call: `POST {KUBECHAT_LLM_BASE_URL}/chat/completions`.

```
request                                   reply
{                                         { "choices": [ { "message": {
  "model": "qwen3:8b",                        "role": "assistant",
  "temperature": 0,                           "content": "",
  "messages": [ …the history… ],              "tool_calls": [ {
  "tools": [ { "type": "function",              "id": "call_1",
    "function": { "name": "pods_log",           "function": {
      "description": "…",                         "name": "pods_list_in_namespace",
      "parameters": { JSON schema } } } ]         "arguments": "{\"namespace\":\"apps\"}"
}                                             } } ] } } ] }
```

- `temperature: 0` gives the most predictable answers.
- `arguments` arrives as a **string** holding JSON. The client passes it on as raw JSON.
- qwen3 also sends its "thinking" in a `reasoning` field. The client ignores it.
- When the model calls tools, `content` is usually empty.
- Timeout: 5 minutes, because local models can be slow.

### MCP client (`internal/adapters/mcp`)

Each `List` or `Call` opens a **short session**:

```
connect ── initialize ──────────► server replies with an Mcp-Session-Id
        ── notifications/initialized
        ── tools/list   or   tools/call {name, arguments}
close
```

- A new session per request costs one extra round trip inside the cluster. In exchange,
  there's no long-lived connection to break when the MCP server pod restarts.
- `List` copies each tool's annotations into `ToolHints`.
- `Call` joins the text parts of the result and keeps the server's `isError` flag.

### Redact (`internal/redact`)

`Clean(text, max)` runs on every tool result before the LLM sees it:

| Pattern | Becomes |
|---|---|
| PEM private keys | `[REDACTED PRIVATE KEY]` |
| JWTs (`eyJ….….…`) | `[REDACTED JWT]` |
| `Bearer <token>` | `Bearer [REDACTED]` |
| AWS keys (`AKIA…`, `ASIA…`) | `[REDACTED AWS KEY]` |
| `password=…`, `token: …`, `secret=…`, `api_key=…` | `password=[REDACTED]` |

Then it cuts the text to `max` bytes (20 000) and adds
`[output cut: first 20000 of N bytes shown]`. These are basic patterns: a safety net, not a
guarantee.

### Config (`internal/config`)

`Load(getenv)` reads the `KUBECHAT_*` environment variables, fills in defaults, and rejects
bad values at start-up (a non-numeric round count, a broken namespace pattern, an unknown
tier). The full list is in [architecture.md](architecture.md#where-each-setting-lives).

The defaults are safe on purpose: with no settings at all, **no tool is a write**, and
`kube-*` is protected.

### Core types (`internal/core/types.go`)

| Type | Meaning |
|---|---|
| `Message` | One entry in the conversation: role (`system`, `user`, `assistant`, `tool`), text, the tool calls the model asked for, or the ID of the call a result answers. |
| `ToolSpec` | One tool from the MCP server: name, description, argument schema, hints. |
| `ToolHints` | The server's own hints: `ReadOnly`, `Destructive`. Only hints: the policy decides. |
| `ToolCall` | A request to run a tool: ID, name, arguments (JSON). |
| `ToolResult` | What the tool returned: text, and whether it's an error. |
| `Tier` | How much a tool may change: `read`, `write`, `destructive`, `blocked`. |

---

## Safety, layer by layer

A write has to get past every layer:

```
 LLM asks for a tool call
   │
   ├─ 1. Gate: is the tool in the server's list?
   ├─ 2. Policy: is its tier allowed? Is the namespace given and not protected?
   ├─ 3. Approval: a human clicks Approve           (not built yet: writes are refused)
   ├─ 4. MCP server: only the enabled tools exist; Secrets are denied
   └─ 5. RBAC: the MCP server may only read, and scale Deployments
   ▼
 Kubernetes API
```

And around them:
- The **bot pod has no Kubernetes token** (`automountServiceAccountToken: false`). Even a
  bug in the bot can't call the API directly.
- **Tool output is redacted** before the LLM sees it.
- The **system prompt** tells the model that tool output is data, never instructions.
  It helps, but the layers above don't rely on it.

---

## Tests

Run them with `make test`.

| Package | What the tests check |
|---|---|
| `core` | core imports only the standard library |
| `config` | values are read, defaults are safe, bad values are rejected |
| `policy` | every tier rule; protected namespaces; missing namespace and bad JSON are refused |
| `gate` | reads run; writes, protected namespaces and unknown tools are refused and never reach the server |
| `agent` | runs tools then answers; redacts output; passes errors back; stops after `MaxRounds` |
| `redact` | each secret pattern; normal text stays as it is; the size cap |

The agent tests use `fakeGate` and a scripted LLM; the gate tests use `fakeServer`. So they
need no cluster and no Ollama.

---

## Running it

```bash
make dev        # rebuild the image, load it into kind, redeploy, follow the logs
make logs       # follow the logs again later

# ask one question from the pod
kubectl --kubeconfig dev/kubeconfig -n kubechat exec deploy/kubechat-bot -- \
  /kubechat -ask "why are pods failing in apps?"

# or on your Mac (the defaults point at localhost for Ollama and the MCP server)
go run ./cmd/kubechat -ask "why are pods failing in apps?"
```

---

## Not built yet

| Piece | Where it plugs in |
|---|---|
| **Slack** | `main.go` service mode: a Slack mention calls `Agent.Ask`, and the answer is posted in the thread. A new `adapters/slack` package. |
| **Approvals** | The gate's "needs approval" branch: post an Approve/Reject card, wait for the click, then run the call or refuse it. |

---

## Where to start reading

1. `internal/core/types.go` and `interfaces.go`: the vocabulary.
2. `cmd/kubechat/main.go`: how it's wired.
3. `internal/app/agent/agent.go`: the loop.
4. `internal/app/gate/gate.go` and `internal/app/policy/policy.go`: the safety checks.
5. `internal/adapters/llm` and `internal/adapters/mcp`: the network calls.
