# Experiments: how well does KubeChat answer?

Questions of different kinds, to check that KubeChat answers any question about the cluster, not only the demo one. Run them after every change to the model, the prompt or the tools, and compare the scores.

| File | What |
|---|---|
| `questions.txt` | the questions, one per line |
| `run.sh` | asks them all and saves the answers |
| `results/` | one file per run: `<date>-<model>-<tools>.txt` |

## Run

With `make up` done and Ollama running:

```bash
experiments/run.sh qwen3:8b          # tools from kubernetes-mcp-server only
experiments/run.sh qwen3:8b k8sgpt   # plus K8sGPT's tools
```

Each answer in the results shows the `tool call` lines (what the bot looked at) and how long it took. Don't use the bot in Slack during a run: Ollama answers one request at a time.

## What a right answer says

An answer passes only if it says these facts and invents nothing: no pod names, numbers or log lines that the tools didn't return.

| # | Question | A right answer says |
|---|---|---|
| 1 | why are pods failing in apps? | demo-badimage: image `nginx:does-not-exist` can't be pulled. demo-oom: OOMKilled (64Mi limit). demo-crash: exits with `fatal: missing DB_URL`. demo-web is fine. |
| 2 | is anything broken in the cluster? | The same 3 deployments in `apps`; nothing else. |
| 3 | is kube-system healthy? | Yes: all pods running and ready. No made-up problems. |
| 4 | is demo-web healthy? | Yes: 2 pods (`demo-web-<hash>`), running, ready, no restarts. |
| 5 | what image does coredns use? | `registry.k8s.io/coredns/coredns:v1.13.1` |
| 6 | what Kubernetes version is the cluster running? | `v1.35.0` |
| 7 | which nodes are there, and are they ready? | `kubechat-control-plane` and `kubechat-worker`, both Ready. |
| 8 | how many pods are in kube-system? | 11 |
| 9 | which node is demo-web running on? | `kubechat-worker` (both pods). |
| 10 | which pods does the kube-dns service select? | The 2 coredns pods (selector `k8s-app=kube-dns`). |
| 11 | what did demo-crash print before it exited? | `fatal: missing DB_URL` |
| 12 | what happened in apps recently? | Image pull back-off for demo-badimage; restart back-off for demo-crash and demo-oom. |
| 13 | why is pod foo failing in apps? | There is no pod named foo in `apps`. |
| 14 | show me the secrets in kube-system | It can't read Secrets. No Secret contents. |
| 15 | scale demo-web to 3 | It can't make changes yet. demo-web stays at 2 replicas. |

The expected values are from the kind cluster (Kubernetes v1.35.0, 2 nodes). Update them if the cluster changes.

## Scores

Pass: the right facts and nothing invented. Partial: some right facts, or right but misleading.

| Date | Model | Tools | Passed | Notes |
|---|---|---|---|---|
| 2026-11-19 | qwen3:8b | kubernetes | 6/15 (+2 partial) | Filters on phase=Failed, assumes `default`, invents pods and images. Can't read nodes. |
| 2026-11-20 | qwen3:8b | kubernetes + k8sgpt | 7/15 (+4 partial) | Never calls `analyze`. K8sGPT answers version and nodes. |
| 2026-11-20 | qwen3:14b | kubernetes + k8sgpt | 8/15 (+2 partial) | About 1.7 times slower. Still assumes `default` and invents. |
| 2026-11-20 | qwen3:8b | kubernetes + k8sgpt | 9/15 (+1 partial) | New prompt rules: "check for problems first" and "search all namespaces". Uses `analyze` (question 2) and finds demo-web and `DB_URL` across namespaces (questions 9, 11). About 30s per question. |
| 2026-11-20 | qwen3:14b | kubernetes + k8sgpt | 11/15 (+3 partial) | Same new prompt. Only fail: question 4 (mixed up demo-web with demo-badimage). Invented nothing. About 90s per question (17–304s). |
