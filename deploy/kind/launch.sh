#!/usr/bin/env bash
# Local KubeChat playground on kind (build guide, steps 2-3):
#   kind cluster -> dev/kubeconfig -> namespaces -> metrics-server -> demo apps.
#
# Credentials go only to dev/kubeconfig (git-ignored):
#   - kind gets --kubeconfig, so it never writes to ~/.kube/config or changes
#     your current context;
#   - every kubectl and helm call passes --kubeconfig, so $KUBECONFIG and your
#     current context are ignored.
#
# Safe to re-run: steps that are already done are skipped or applied idempotently.
set -euo pipefail

CLUSTER_NAME="kubechat"
NAMESPACES=(apps staging)                 # writable namespaces in policy config
METRICS_SERVER_CHART_VERSION="3.14.0"     # app v0.9.0
METRICS_SERVER_REPO="https://kubernetes-sigs.github.io/metrics-server/"
DEMO_NAMESPACE="apps"                     # must match namespace: in deploy/demo/kustomization.yaml

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KUBECONFIG_FILE="$ROOT/dev/kubeconfig"
DEMO_DIR="$ROOT/deploy/demo"

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

check_tools() {
  local tool
  for tool in kind kubectl helm; do
    command -v "$tool" >/dev/null || die "$tool not found in PATH"
  done
}

cluster_exists() {
  local clusters
  clusters="$(kind get clusters 2>/dev/null)"
  grep -qx "$CLUSTER_NAME" <<<"$clusters"
}

# One control plane, one worker so workloads land off the control plane.
create_cluster() {
  if cluster_exists; then
    log "kind cluster '$CLUSTER_NAME' exists, skipping create"
    return
  fi
  log "creating kind cluster '$CLUSTER_NAME'"
  mkdir -p "$(dirname "$KUBECONFIG_FILE")"
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --kubeconfig "$KUBECONFIG_FILE" \
    --wait 120s \
    --config - <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
EOF
}

# Rewritten on every run so the file holds exactly one context (kind-kubechat),
# however the cluster was created. umask 077: the file is never world-readable.
write_kubeconfig() {
  log "writing $KUBECONFIG_FILE"
  mkdir -p "$(dirname "$KUBECONFIG_FILE")"
  (umask 077 && kind get kubeconfig --name "$CLUSTER_NAME" >"$KUBECONFIG_FILE")
}

wait_for_nodes() {
  log "waiting for nodes to be Ready"
  kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes --all --timeout=120s
}

create_namespaces() {
  local ns
  for ns in "${NAMESPACES[@]}"; do
    kubectl --kubeconfig "$KUBECONFIG_FILE" create namespace "$ns" --dry-run=client -o yaml |
      kubectl --kubeconfig "$KUBECONFIG_FILE" apply -f -
  done
}

# wait_for_rollout NAMESPACE DEPLOYMENT TIMEOUT
# The first image pull can be slow, and Docker Desktop's DNS sometimes fails for a
# while; the kubelet retries with backoff (up to 5 min). On timeout, print the
# deployment's recent pod events so the cause is visible.
wait_for_rollout() {
  local ns="$1" deploy="$2" timeout="$3"
  if ! kubectl --kubeconfig "$KUBECONFIG_FILE" -n "$ns" rollout status "deployment/$deploy" --timeout="$timeout"; then
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
  helm upgrade --install metrics-server metrics-server \
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
  log "deploying demo apps from $DEMO_DIR"
  kubectl --kubeconfig "$KUBECONFIG_FILE" apply -k "$DEMO_DIR"

  log "waiting for demo-web (the healthy app)"
  wait_for_rollout "$DEMO_NAMESPACE" demo-web 300s
}

show_pods() {
  log "pods in $DEMO_NAMESPACE (the faults need ~1 min to show their failure state)"
  kubectl --kubeconfig "$KUBECONFIG_FILE" -n "$DEMO_NAMESPACE" get pods
}

main() {
  check_tools
  create_cluster
  write_kubeconfig
  wait_for_nodes
  create_namespaces
  install_metrics_server
  deploy_demo
  show_pods

  echo
  log "done. Use: kubectl --kubeconfig $KUBECONFIG_FILE ..."
}

main "$@"
