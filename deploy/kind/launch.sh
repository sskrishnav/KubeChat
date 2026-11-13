#!/usr/bin/env bash
# Local KubeChat playground on kind. Everything goes into one kind cluster:
#   kind cluster -> dev/kubeconfig -> namespaces -> metrics-server
#   -> demo apps (namespace apps) -> kubechat chart with the MCP server (namespace kubechat)
#
# Credentials go only to dev/kubeconfig (git-ignored):
#   - kind gets --kubeconfig, so it never writes to ~/.kube/config or changes
#     your current context;
#   - every kubectl and helm call passes --kubeconfig, so $KUBECONFIG and your
#     current context are ignored.
#
# Every command that changes something is printed before it runs ("$ ..."),
# with paths relative to the repo root, so you can copy and re-run it by hand.
#
# Safe to re-run: steps that are already done are skipped or applied idempotently.
set -euo pipefail

CLUSTER_NAME="kubechat"
NAMESPACES=(apps staging)                 # writable namespaces in policy config
METRICS_SERVER_CHART_VERSION="3.14.0"     # app v0.9.0
METRICS_SERVER_REPO="https://kubernetes-sigs.github.io/metrics-server/"
DEMO_NAMESPACE="apps"                     # must match namespace: in deploy/demo/kustomization.yaml

KUBECHAT_RELEASE="kubechat"
KUBECHAT_NAMESPACE="kubechat"
MCP_DEPLOYMENT="kubechat-mcp"             # mcp.fullnameOverride in the chart's values.yaml
MCP_NODE_PORT=30080                       # mcpNodePort.nodePort in the chart's values.yaml
MCP_HOST_PORT=8080                        # MCP server reachable at http://localhost:8080/mcp

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KUBECONFIG_FILE="$ROOT/dev/kubeconfig"
DEMO_DIR="$ROOT/deploy/demo"
CHART_DIR="$ROOT/deploy/helm/kubechat"

log() { printf '\n==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# show CMD...: print a command the way you'd type it from the repo root: $ROOT/
# shortened to a relative path, and arguments with special characters wrapped
# in single quotes. Goes to stderr so it never ends up in captured output.
show() {
  local out="" arg safe='^[A-Za-z0-9_./:=@%+,-]+$' q="'"
  for arg in "$@"; do
    arg="${arg//"$ROOT"\//}"
    if [[ $arg =~ $safe ]]; then
      out+="$arg "
    else
      out+="$q${arg//$q/$q\\$q$q}$q "
    fi
  done
  printf '$ %s\n' "${out% }" >&2
}

# run CMD...: print the command, then run it.
run() {
  show "$@"
  "$@"
}

check_tools() {
  local tool
  for tool in docker kind kubectl helm curl; do
    command -v "$tool" >/dev/null || die "$tool not found in PATH"
  done
}

cluster_exists() {
  local clusters
  clusters="$(kind get clusters 2>/dev/null)"
  grep -qx "$CLUSTER_NAME" <<<"$clusters"
}

# kind can only add port mappings when it creates a cluster, so an older
# cluster without the MCP mapping has to be recreated.
check_port_mapping() {
  if [[ -z "$(docker port "${CLUSTER_NAME}-control-plane" "${MCP_NODE_PORT}/tcp" 2>/dev/null)" ]]; then
    die "cluster '$CLUSTER_NAME' has no port mapping for the MCP server.
Recreate it: kind delete cluster --name $CLUSTER_NAME && $0"
  fi
}

# One control plane, one worker so workloads land off the control plane.
# localhost:MCP_HOST_PORT -> node port MCP_NODE_PORT -> MCP server Service.
create_cluster() {
  if cluster_exists; then
    log "kind cluster '$CLUSTER_NAME' exists, skipping create"
    check_port_mapping
    return
  fi
  if command -v lsof >/dev/null && lsof -nP -iTCP:"$MCP_HOST_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    die "port $MCP_HOST_PORT is in use (an MCP server started with npx?); stop it and re-run"
  fi

  local kind_config
  kind_config="$(cat <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: ${MCP_NODE_PORT}
        hostPort: ${MCP_HOST_PORT}
        listenAddress: "127.0.0.1"
        protocol: TCP
  - role: worker
EOF
)"

  log "creating kind cluster '$CLUSTER_NAME' with this config (passed on stdin):"
  sed 's/^/    /' <<<"$kind_config"
  mkdir -p "$(dirname "$KUBECONFIG_FILE")"
  run kind create cluster \
    --name "$CLUSTER_NAME" \
    --kubeconfig "$KUBECONFIG_FILE" \
    --wait 120s \
    --config - <<<"$kind_config"
}

# Rewritten on every run so the file holds exactly one context (kind-kubechat),
# however the cluster was created. umask 077: the file is never world-readable.
write_kubeconfig() {
  log "writing dev/kubeconfig"
  show kind get kubeconfig --name "$CLUSTER_NAME"
  printf '  (output saved to dev/kubeconfig)\n' >&2
  mkdir -p "$(dirname "$KUBECONFIG_FILE")"
  (umask 077 && kind get kubeconfig --name "$CLUSTER_NAME" >"$KUBECONFIG_FILE")
}

wait_for_nodes() {
  log "waiting for nodes to be Ready"
  run kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes --all --timeout=120s
}

create_namespaces() {
  log "creating namespaces: ${NAMESPACES[*]}"
  local ns
  for ns in "${NAMESPACES[@]}"; do
    run kubectl --kubeconfig "$KUBECONFIG_FILE" create namespace "$ns" --dry-run=client -o yaml |
      run kubectl --kubeconfig "$KUBECONFIG_FILE" apply -f -
  done
}

# wait_for_rollout NAMESPACE DEPLOYMENT TIMEOUT
# The first image pull can be slow, and Docker Desktop's DNS sometimes fails for a
# while; the kubelet retries with backoff (up to 5 min). On timeout, print the
# deployment's recent pod events so the cause is visible.
wait_for_rollout() {
  local ns="$1" deploy="$2" timeout="$3"
  if ! run kubectl --kubeconfig "$KUBECONFIG_FILE" -n "$ns" rollout status "deployment/$deploy" --timeout="$timeout"; then
    echo "$deploy not ready after $timeout; recent pod events:" >&2
    kubectl --kubeconfig "$KUBECONFIG_FILE" -n "$ns" get events \
      --field-selector involvedObject.kind=Pod --sort-by=.lastTimestamp |
      grep "$deploy" | tail -5 >&2 || true
    exit 1
  fi
}

# Needed by `kubectl top` and the MCP server's pods_top/nodes_top.
# --kubelet-insecure-tls because kind's kubelets serve self-signed certificates.
# --repo installs straight from the chart URL, so your global `helm repo` list is
# left alone.
install_metrics_server() {
  log "installing metrics-server chart $METRICS_SERVER_CHART_VERSION"
  run helm upgrade --install metrics-server metrics-server \
    --repo "$METRICS_SERVER_REPO" \
    --version "$METRICS_SERVER_CHART_VERSION" \
    --kubeconfig "$KUBECONFIG_FILE" \
    --namespace kube-system \
    --set 'args={--kubelet-insecure-tls}'

  log "waiting for metrics-server to become ready"
  wait_for_rollout kube-system metrics-server 300s
}

# Three apps broken on purpose plus one healthy one. Only demo-web is waited for;
# the other three never become ready, by design.
deploy_demo() {
  log "deploying demo apps from deploy/demo"
  run kubectl --kubeconfig "$KUBECONFIG_FILE" apply -k "$DEMO_DIR"

  log "waiting for demo-web (the healthy app)"
  wait_for_rollout "$DEMO_NAMESPACE" demo-web 300s
}

# The same chart users install on any cluster; values-kind.yaml adds the
# NodePort so the MCP server is reachable from the laptop.
install_kubechat() {
  log "fetching chart dependencies (kubernetes-mcp-server)"
  run helm dependency build "$CHART_DIR"

  log "installing kubechat chart into namespace $KUBECHAT_NAMESPACE"
  run helm upgrade --install "$KUBECHAT_RELEASE" "$CHART_DIR" \
    --kubeconfig "$KUBECONFIG_FILE" \
    --namespace "$KUBECHAT_NAMESPACE" \
    --create-namespace \
    --values "$CHART_DIR/values-kind.yaml" \
    --set mcpNodePort.nodePort="$MCP_NODE_PORT"

  log "waiting for the MCP server to become ready"
  wait_for_rollout "$KUBECHAT_NAMESPACE" "$MCP_DEPLOYMENT" 300s
}

# Smoke test: an MCP initialize request must return a session id.
# The NodePort route can lag the rollout by a few seconds (kube-proxy has to
# program it; until then Docker's port forward answers with an empty reply),
# so retry for up to ~30s before giving up.
check_mcp() {
  local url="http://localhost:${MCP_HOST_PORT}/mcp" headers attempt
  local init='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"launch.sh","version":"0"}}}'
  local curl_args=(-sS -D - -o /dev/null --max-time 5
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream'
    -d "$init" "$url")

  log "checking MCP server at $url (expects an Mcp-Session-Id header; retries up to 30s)"
  show curl "${curl_args[@]}"
  for attempt in $(seq 1 15); do
    if headers="$(curl "${curl_args[@]}" 2>/dev/null)" && grep -qi '^mcp-session-id:' <<<"$headers"; then
      echo "MCP server answers (attempt $attempt)"
      return 0
    fi
    sleep 2
  done
  die "MCP server at $url did not answer an MCP initialize request within 30s"
}

show_status() {
  log "pods in $DEMO_NAMESPACE (the faults need ~1 min to show their failure state)"
  run kubectl --kubeconfig "$KUBECONFIG_FILE" -n "$DEMO_NAMESPACE" get pods
  log "pods in $KUBECHAT_NAMESPACE"
  run kubectl --kubeconfig "$KUBECONFIG_FILE" -n "$KUBECHAT_NAMESPACE" get pods
}

main() {
  check_tools
  create_cluster
  write_kubeconfig
  wait_for_nodes
  create_namespaces
  install_metrics_server
  deploy_demo
  install_kubechat
  check_mcp
  show_status

  log "done."
  echo "kubectl:    kubectl --kubeconfig dev/kubeconfig ..."
  echo "MCP server: http://localhost:${MCP_HOST_PORT}/mcp"
}

main "$@"
