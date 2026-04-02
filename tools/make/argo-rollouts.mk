##@ Argo Rollouts

PLUGIN_DIR = plugin
PLUGIN_IMAGE = ngrok-rollouts-plugin:latest

ARGO_ROLLOUTS_VERSION ?= v1.7.2
ARGO_ROLLOUTS_NAMESPACE ?= argo-rollouts
ROLLOUT_DEMO_NAMESPACE ?= rollout-demo

# Install URL — override ARGO_ROLLOUTS_VERSION to pin a different release.
ARGO_ROLLOUTS_INSTALL_URL = https://github.com/argoproj/argo-rollouts/releases/download/$(ARGO_ROLLOUTS_VERSION)/install.yaml

.PHONY: plugin-build
plugin-build: ## Build the plugin binary and package it into a Docker image.
	cd $(PLUGIN_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/ngrok-traffic-router ./cmd/main.go
	docker build -t $(PLUGIN_IMAGE) $(PLUGIN_DIR)
	@echo "Built image: $(PLUGIN_IMAGE)"

.PHONY: plugin-load
plugin-load: plugin-build ## Build plugin image and load it into the kind cluster.
	kind load docker-image $(PLUGIN_IMAGE) --name $(KIND_CLUSTER_NAME)
	@echo "Loaded $(PLUGIN_IMAGE) into kind cluster $(KIND_CLUSTER_NAME)"

.PHONY: plugin-deploy
plugin-deploy: plugin-load ## Build, load, patch argo-rollouts, and restart to activate the plugin.
	kubectl apply -f plugin/example/argo-rollouts-config.yaml
	kubectl patch deployment argo-rollouts -n argo-rollouts \
		--patch-file plugin/example/argo-rollouts-patch.yaml
	kubectl -n argo-rollouts rollout restart deploy/argo-rollouts
	kubectl -n argo-rollouts rollout status deploy/argo-rollouts --timeout=120s
	@echo "Plugin deployed. Argo Rollouts is using the ngrok traffic router."

.PHONY: argo-rollouts-install
argo-rollouts-install: ## Install Argo Rollouts controller and apply plugin ConfigMap.
	kubectl create namespace $(ARGO_ROLLOUTS_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -n $(ARGO_ROLLOUTS_NAMESPACE) -f $(ARGO_ROLLOUTS_INSTALL_URL)
	kubectl apply -f plugin/example/argo-rollouts-config.yaml
	@echo "Waiting for Argo Rollouts controller to be ready..."
	kubectl -n $(ARGO_ROLLOUTS_NAMESPACE) rollout status deploy/argo-rollouts --timeout=120s

.PHONY: argo-rollouts-uninstall
argo-rollouts-uninstall: ## Uninstall Argo Rollouts from the cluster.
	kubectl delete -n $(ARGO_ROLLOUTS_NAMESPACE) \
		-f $(ARGO_ROLLOUTS_INSTALL_URL) \
		--ignore-not-found=true
	kubectl delete namespace $(ARGO_ROLLOUTS_NAMESPACE) --ignore-not-found=true

.PHONY: argo-rollouts-example-apply
argo-rollouts-example-apply: _argo-rollouts-check-host ## Deploy the rollout demo app. Requires ROLLOUT_DEMO_HOST=<your-ngrok-subdomain>.
	kubectl apply -f plugin/example/app.yaml
	kubectl apply -f plugin/example/rbac.yaml
	ROLLOUT_DEMO_HOST=$(ROLLOUT_DEMO_HOST) envsubst < plugin/example/ingress.yaml | kubectl apply -f -
	@echo ""
	@echo "Demo deployed to namespace: $(ROLLOUT_DEMO_NAMESPACE)"
	@echo "Endpoint will be live at:   https://$(ROLLOUT_DEMO_HOST)"
	@echo ""
	@echo "Next steps:"
	@echo "  make argo-rollouts-example-status    # watch pods, agentendpoints, cloudendpoints"
	@echo "  make argo-rollouts-example-update    # trigger a canary rollout (v1 -> v2)"

.PHONY: argo-rollouts-example-delete
argo-rollouts-example-delete: ## Tear down the rollout demo app and its RBAC.
	kubectl delete namespace $(ROLLOUT_DEMO_NAMESPACE) --ignore-not-found=true
	kubectl delete clusterrole argo-rollouts-ngrok-plugin --ignore-not-found=true
	kubectl delete clusterrolebinding argo-rollouts-ngrok-plugin --ignore-not-found=true

.PHONY: argo-rollouts-example-reset
argo-rollouts-example-reset: _argo-rollouts-check-host ## Reset the demo back to stable-v1 so argo-rollouts-example-update can be run again.
	$(MAKE) argo-rollouts-example-delete
	$(MAKE) argo-rollouts-example-apply ROLLOUT_DEMO_HOST=$(ROLLOUT_DEMO_HOST)
	@echo ""
	@echo "Waiting for endpoint to come online..."
	@until kubectl get cloudendpoint -n $(ROLLOUT_DEMO_NAMESPACE) -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; do sleep 2; done
	@echo "Ready. Run: make argo-rollouts-example-update"

.PHONY: argo-rollouts-example-update
argo-rollouts-example-update: ## Trigger a canary rollout, alternating between stable-v1 and stable-v2 each run.
	@# Flip the canary target: if the current spec is v1 go to v2, otherwise go back to v1.
	@# This lets you re-run the demo indefinitely without a full reset.
	$(eval CURRENT := $(shell kubectl -n $(ROLLOUT_DEMO_NAMESPACE) get rollout rollout-demo \
		-o jsonpath='{.spec.template.spec.containers[0].args[0]}' 2>/dev/null))
	$(eval NEXT := $(if $(findstring v1,$(CURRENT)),canary-v2,stable-v1))
	$(eval LABEL := $(if $(findstring v1,$(CURRENT)),stable-v1 → canary-v2,stable-v2 → canary-v1))
	kubectl -n $(ROLLOUT_DEMO_NAMESPACE) patch rollout rollout-demo --subresource=status \
		--type=json -p='[{"op":"replace","path":"/status/abort","value":false}]' 2>/dev/null || true
	kubectl -n $(ROLLOUT_DEMO_NAMESPACE) patch rollout rollout-demo \
		--type=json \
		-p='[{"op":"replace","path":"/spec/template/spec/containers/0/args/0","value":"-text=$(NEXT)"}]'
	$(eval DEMO_HOST := $(shell kubectl get cloudendpoint -n $(ROLLOUT_DEMO_NAMESPACE) -o jsonpath='{.items[0].spec.url}' 2>/dev/null | sed 's|https://||'))
	@echo ""
	@echo "Canary triggered: $(LABEL)"
	@echo "Rollout will progress automatically:"
	@echo "  setWeight(25) → 30s pause → setWeight(50) → 30s pause → setWeight(75) → 30s pause → done"
	@echo ""
	@echo "Watch traffic split:"
	@echo "  while true; do curl -s https://$(DEMO_HOST); echo; sleep 0.5; done"
	@echo ""
	@echo "Watch endpoints:       kubectl get agentendpoints,cloudendpoints -n $(ROLLOUT_DEMO_NAMESPACE) -w"
	@echo "Watch traffic policy:  watch -n2 \"kubectl get cloudendpoint -n $(ROLLOUT_DEMO_NAMESPACE) -o jsonpath='{.items[0].spec.trafficPolicy}' | python3 -m json.tool\""

.PHONY: argo-rollouts-example-promote
argo-rollouts-example-promote: ## Promote past the manual pause step in the rollout.
	kubectl argo rollouts promote rollout-demo -n $(ROLLOUT_DEMO_NAMESPACE) 2>/dev/null || \
		kubectl -n $(ROLLOUT_DEMO_NAMESPACE) patch rollout rollout-demo --subresource=status \
			--type=json -p='[{"op":"remove","path":"/status/pauseConditions"},{"op":"replace","path":"/status/controllerPause","value":false}]'

.PHONY: argo-rollouts-example-abort
argo-rollouts-example-abort: ## Abort the in-progress rollout and roll back to stable.
	kubectl argo rollouts abort rollout-demo -n $(ROLLOUT_DEMO_NAMESPACE) 2>/dev/null || \
		kubectl -n $(ROLLOUT_DEMO_NAMESPACE) patch rollout rollout-demo --subresource=status \
			--type=json -p='[{"op":"add","path":"/status/abort","value":true}]'

.PHONY: argo-rollouts-example-status
argo-rollouts-example-status: ## Show rollout status, pods, AgentEndpoints, CloudEndpoints, and traffic policy.
	@echo "=== Rollout ==="
	kubectl argo rollouts get rollout rollout-demo -n $(ROLLOUT_DEMO_NAMESPACE) 2>/dev/null \
		|| kubectl -n $(ROLLOUT_DEMO_NAMESPACE) get rollout rollout-demo -o wide
	@echo ""
# 	@echo "=== Pods ==="
# 	kubectl -n $(ROLLOUT_DEMO_NAMESPACE) get pods -o wide
# 	@echo ""
	@echo "=== AgentEndpoints ==="
	kubectl get agentendpoints -n $(ROLLOUT_DEMO_NAMESPACE) -o wide 2>/dev/null || echo "(none)"
	@echo ""
	@echo "=== CloudEndpoints ==="
	kubectl get cloudendpoints -n $(ROLLOUT_DEMO_NAMESPACE) -o wide 2>/dev/null || echo "(none)"
	@echo ""
	@echo "=== Traffic Policy ==="
	@kubectl get cloudendpoint -n $(ROLLOUT_DEMO_NAMESPACE) \
		-o jsonpath='{.items[0].spec.trafficPolicy.policy}' 2>/dev/null \
		| python3 -m json.tool 2>/dev/null || echo "(no CloudEndpoint traffic policy)"

.PHONY: _argo-rollouts-check-host
_argo-rollouts-check-host:
ifndef ROLLOUT_DEMO_HOST
	$(error ROLLOUT_DEMO_HOST must be set. Example: make argo-rollouts-example-apply ROLLOUT_DEMO_HOST=rollout-demo.your-name.ngrok.app)
endif
