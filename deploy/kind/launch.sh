#!/usr/bin/env bash
# Creates the local kind cluster for KubeChat development (build guide, step 2).
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
NAMESPACES=(apps staging)                  # writable namespaces in policy config
METRICS_SERVER_CHART_VERSION="3.14.0"      # app v0.9.0

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KUBECONFIG_FILE="$ROOT/dev/kubeconfig"
mkdir -p "$(dirname "$KUBECONFIG_FILE")"

# 1. Cluster: one control plane, one worker so workloads land off the control plane.
existing_clusters="$(kind get clusters 2>/dev/null)"
if grep -qx "$CLUSTER_NAME" <<<"$existing_clusters"; then
  echo "==> kind cluster '$CLUSTER_NAME' exists, skipping create"
else
  echo "==> creating kind cluster '$CLUSTER_NAME'"
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
fi

# Rewrite on every run so the file holds exactly one context (kind-kubechat),
# however the cluster was created.
kind get kubeconfig --name "$CLUSTER_NAME" >"$KUBECONFIG_FILE"
chmod 600 "$KUBECONFIG_FILE"

kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes --all --timeout=120s

# 2. Namespaces (create-or-keep).
for ns in "${NAMESPACES[@]}"; do
  kubectl --kubeconfig "$KUBECONFIG_FILE" create namespace "$ns" --dry-run=client -o yaml |
    kubectl --kubeconfig "$KUBECONFIG_FILE" apply -f -
done

# 3. metrics-server: needed by `kubectl top` and the MCP server's pods_top/nodes_top.
# --kubelet-insecure-tls because kind's kubelets serve self-signed certificates.
# --repo installs straight from the chart URL, so your global `helm repo` list is
# left alone.
echo "==> installing metrics-server $METRICS_SERVER_CHART_VERSION"
helm upgrade --install metrics-server metrics-server \
  --repo https://kubernetes-sigs.github.io/metrics-server/ \
  --version "$METRICS_SERVER_CHART_VERSION" \
  --kubeconfig "$KUBECONFIG_FILE" \
  --namespace kube-system \
  --set 'args={--kubelet-insecure-tls}'

# The first image pull can be slow, and Docker Desktop's DNS sometimes fails for a
# while; the kubelet retries with backoff (up to 5 min), so allow time and show why
# if it still isn't ready.
echo "==> waiting for metrics-server to become ready"
if ! kubectl --kubeconfig "$KUBECONFIG_FILE" -n kube-system rollout status deployment/metrics-server --timeout=300s; then
  echo "metrics-server not ready; recent pod events:" >&2
  kubectl --kubeconfig "$KUBECONFIG_FILE" -n kube-system get events \
    --field-selector involvedObject.kind=Pod --sort-by=.lastTimestamp |
    grep metrics-server | tail -5 >&2 || true
  exit 1
fi

echo
echo "==> done. Use: kubectl --kubeconfig $KUBECONFIG_FILE get nodes"
