# MCP tools

The tools KubeChat's MCP servers expose, as reported by the running servers (`tools/list`).
The policy code (read vs write) is based on this list. Re-check it when you upgrade a server.
The bot also logs the full list, with each tool's tier, at start-up (`make logs`).

Server: kubernetes-mcp-server **0.1.2** (chart `deploy/helm/kubechat`) · checked 2026-11-13

| Tool | Server's readOnly hint | Server's destructive hint | Required args | KubeChat tier |
|---|---|---|---|---|
| `events_list` | true | false | — | read |
| `pods_get` | true | false | `name` | read |
| `pods_list_in_namespace` | true | false | `namespace` | read |
| `pods_log` | true | false | `name` | read |
| `resources_get` | true | false | `apiVersion`, `kind`, `name` | read |
| `resources_list` | true | false | `apiVersion`, `kind` | read |
| `resources_scale` | false | **true** | `apiVersion`, `kind`, `name` | **write** (needs Approve) |

## Notes

- `resources_scale` is marked destructive by the server. KubeChat's policy overrides it to
  **write**, the only change the MVP allows.
- `resources_scale` without a `scale` argument only reads the current scale. KubeChat still
  treats every call as a write, which is the safe choice; the model can read replicas with `resources_get`.
- Always pass `namespace`. Without it the server uses its default namespace.
- `pods_log` returns the last 100 lines by default (`tail`). For a crash-looping pod the error
  can be in the current container's log or in the previous one's (`previous: true`). Try both.
- How to re-check: `tools/list` against `http://localhost:8080/mcp` (see `check_mcp` in
  `deploy/kind/launch.sh` for the handshake).

## K8sGPT

Server: K8sGPT **v0.4.41** (`templates/k8sgpt.yaml`) · checked 2026-11-20

Every tool reports `readOnly: false, destructive: true`. That is its MCP library's default,
not a real label. KubeChat counts all of them as **read**, because K8sGPT is in
`KUBECHAT_MCP_READ_ONLY` and its RBAC can only read (no Secrets).

| Tool | Required args | What it does |
|---|---|---|
| `analyze` | — | Rule-based checks (Pods, Deployments, Services, Nodes, PVCs, …) → a list of problems. About 1 second. |
| `cluster-info` | — | Kubernetes version |
| `list-namespaces` | — | namespace names |
| `list-resources` | `resourceType` | list pods, deployments, nodes, … (14 types) |
| `get-resource` | `resourceType`, `name` | one object, as JSON |
| `get-logs` | `podName`, `namespace` | container logs (`previous`, `tailLines`) |
| `list-events` | — | events, filterable by object |
| `list-filters`, `list-integrations` | — | K8sGPT's analyzers and integrations |
| `add-filters`, `remove-filters`, `config` | some | change K8sGPT's own settings (lost on restart: they live in an `emptyDir`) |

Notes:
- `analyze` doesn't read logs. For a crash-looping pod it reports only the last termination
  reason (`Error`, `OOMKilled`), so the cause still needs `pods_log` or `get-logs`.
- Cluster-wide `analyze` also lists harmless "ConfigMap not used by any pods" findings.
- `analyze` with `explain: true` asks an AI backend. KubeChat sets `noopai`, so no data
  leaves the cluster.
- `get-resource` and `list-resources` accept Secrets; Kubernetes refuses them (RBAC `view`).
