# KubeChat developer commands. `make` alone lists them.
KUBECTL := kubectl --kubeconfig dev/kubeconfig -n kubechat

.PHONY: help up dev logs test down

help: ## list the commands
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-5s %s\n", $$1, $$2}'

up: ## create the kind cluster and install everything
	./deploy/kind/launch.sh

dev: ## rebuild the bot, redeploy it in kind, follow its logs
	./deploy/kind/launch.sh bot
	@$(MAKE) --no-print-directory logs

logs: ## follow the bot's logs
	@echo "==> following the bot's logs (Ctrl-C stops following; the bot keeps running)"
	$(KUBECTL) logs -f deploy/kubechat-bot

test: ## run the Go tests
	go test ./...

down: ## delete the kind cluster
	kind delete cluster --name kubechat
