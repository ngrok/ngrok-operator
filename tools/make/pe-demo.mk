##@ Private endpoints demo (kind)

PE_DEMO_SCRIPT = ./scripts/private-endpoints-demo/demo.sh
# Set to true to also delete the kind cluster in pe-demo-down.
PE_DEMO_DELETE_CLUSTER ?= false
# Fault scenario for pe-demo-faults: origin-restart, rolling-restart, uninstall, revoke.
FAULT ?=
# Extra args for the fault scenario, e.g. FAULT_ARGS=--yes for uninstall.
FAULT_ARGS ?=

.PHONY: pe-demo-up
pe-demo-up: _deploy-check-env-vars ## Check token, then kind cluster + operator (private endpoints on) + DNS + demo origins, wait until Ready.
	$(PE_DEMO_SCRIPT) preflight
	kind get clusters 2>/dev/null | grep -qx "$(KIND_CLUSTER_NAME)" || kind create cluster --name "$(KIND_CLUSTER_NAME)"
	kubectl config use-context "kind-$(KIND_CLUSTER_NAME)"
	$(MAKE) -f tools/make/_common.mk deploy_with_private_endpoints kind-private-endpoints-dns
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) $(PE_DEMO_SCRIPT) apply
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) $(PE_DEMO_SCRIPT) wait

.PHONY: pe-demo-status
pe-demo-status: ## Show PrivateEndpoints, per-host Services, forwarder pods, demo AgentEndpoints.
	@KUBE_NAMESPACE=$(KUBE_NAMESPACE) $(PE_DEMO_SCRIPT) status

.PHONY: pe-demo-test
pe-demo-test: ## Run PASS/FAIL checks against the demo endpoints from a client pod.
	@KUBE_NAMESPACE=$(KUBE_NAMESPACE) $(PE_DEMO_SCRIPT) test

.PHONY: pe-demo-shell
pe-demo-shell: ## Open a shell in the demo client pod (netshoot).
	@KUBE_NAMESPACE=$(KUBE_NAMESPACE) $(PE_DEMO_SCRIPT) shell

.PHONY: pe-demo-faults
pe-demo-faults: ## Run a live fault scenario on the demo cluster: FAULT=origin-restart|rolling-restart|uninstall|revoke (FAULT_ARGS=--yes for uninstall).
	@KUBE_NAMESPACE=$(KUBE_NAMESPACE) HELM_RELEASE_NAME=$(HELM_RELEASE_NAME) ./scripts/private-endpoints-demo/faults.sh $(FAULT) $(FAULT_ARGS)

.PHONY: pe-demo-down
pe-demo-down: ## Delete demo origins. PE_DEMO_DELETE_CLUSTER=true also deletes the kind cluster.
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) $(PE_DEMO_SCRIPT) down
ifeq ($(PE_DEMO_DELETE_CLUSTER),true)
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"
endif
