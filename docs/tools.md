# MCP tools

The tools KubeChat's MCP server exposes, as reported by the running server (`tools/list`).
The policy code (read vs write) is based on this list. Re-check it when you upgrade the server.

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
