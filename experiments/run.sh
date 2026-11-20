#!/usr/bin/env bash
# Asks every question in questions.txt and saves the answers in results/.
#
# Usage, from anywhere (needs `make up` done and Ollama running):
#   experiments/run.sh MODEL            tools from kubernetes-mcp-server only
#   experiments/run.sh MODEL k8sgpt     plus K8sGPT's tools
#
# It runs the bot from this checkout on the laptop (no deploy needed): the MCP
# server is at localhost:8080 (kind NodePort), K8sGPT through a port-forward.
set -euo pipefail
cd "$(dirname "$0")/.."

model="${1:?usage: experiments/run.sh MODEL [k8sgpt]}"
servers="kubernetes=http://localhost:8080/mcp"
read_only=""
tools="kubernetes"

if [[ "${2:-}" == k8sgpt ]]; then
  kubectl --kubeconfig dev/kubeconfig -n kubechat port-forward svc/kubechat-k8sgpt 8089 >/dev/null 2>&1 &
  forward=$!
  trap 'kill $forward' EXIT
  # Wait for it. POST, not GET: a GET to /mcp opens an event stream that never ends.
  curl -s -o /dev/null --max-time 10 --retry 10 --retry-connrefused --retry-delay 1 -X POST \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' http://localhost:8089/mcp
  servers+=",k8sgpt=http://localhost:8089/mcp"
  read_only="k8sgpt"
  tools="kubernetes+k8sgpt"
fi

go build -o bin/kubechat ./cmd/kubechat
mkdir -p experiments/results
out="experiments/results/$(date +%Y-%m-%d-%H%M)-${model//:/-}-$tools.txt"
echo "model=$model tools=$tools -> $out"

while IFS= read -r question; do
  [[ -z "$question" || "$question" == \#* ]] && continue
  echo "### $question"
  start=$(date +%s)
  KUBECHAT_LLM_MODEL="$model" KUBECHAT_MCP_SERVERS="$servers" KUBECHAT_MCP_READ_ONLY="$read_only" \
    ./bin/kubechat -ask "$question" </dev/null 2>&1 || true
  echo "(took $(($(date +%s) - start))s)"
  echo
done <experiments/questions.txt | tee "$out"
