##@ Testing

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	setup-envtest use $$ENVTEST_K8S_VERSION
	go test $(if $(PACKAGE),$(PACKAGE),./...) -coverprofile cover.out -timeout 120s

.PHONY: test-coverage
test-coverage: test ## Run tests and open coverage report. Usage: make test-coverage PACKAGE=./internal/domain
	rm -rf coverage
	@mkdir -p coverage
	@if [ -n "$(PACKAGE)" ]; then \
		echo "Filtering coverage for package: $(PACKAGE)"; \
		go tool cover -html=cover.out -o coverage/index.html; \
		echo "Coverage report opened for package $(PACKAGE): coverage/index.html"; \
	else \
		go tool cover -html=cover.out -o coverage/index.html; \
		echo "Coverage report opened: coverage/index.html"; \
	fi
	open coverage/index.html

.PHONY: validate
validate: build test lint manifests helm-update-snapshots ## Validate the codebase before a PR


.PHONY: e2e-tests
e2e-tests: ## Run e2e tests
	chainsaw test ./tests/chainsaw \
		--exclude-test-regex 'chainsaw/_skip_*.yaml' \
		--namespace e2e \
		--cleanup-timeout 2m

.PHONY: e2e-tests-multi-ns
e2e-tests-multi-ns: ## Run multi-namespace e2e tests
	chainsaw test ./tests/chainsaw-multi-ns \
		--namespace namespace-a \
		--cleanup-timeout 2m

.PHONY: e2e-clean
e2e-clean: ## Clean up e2e tests
	kubectl delete ns e2e
	kubectl delete --all boundendpoints -n ngrok-operator
	kubectl delete --all services -n ngrok-operator
	kubectl delete --all kubernetesoperators -n ngrok-operator
	helm --namespace ngrok-operator uninstall ngrok-operator

##@ Uninstall E2E Tests
# Run specific scenario: make e2e-uninstall SCENARIO=delete-policy-bundled-crds
# Run with debug mode:   make e2e-uninstall SCENARIO=delete-policy-bundled-crds DEBUG=1

UNINSTALL_TEST_DIR := ./tests/chainsaw-uninstall
UNINSTALL_NAMESPACE := uninstall-test
SCENARIO ?= delete-policy-bundled-crds

.PHONY: e2e-uninstall
e2e-uninstall: _helm_setup ## Run uninstall e2e test. Usage: make e2e-uninstall SCENARIO=<scenario> [DEBUG=1]
	chainsaw test $(UNINSTALL_TEST_DIR)/$(SCENARIO) \
		--namespace $(UNINSTALL_NAMESPACE) \
		--cleanup-timeout 2m \
		$(if $(DEBUG),--skip-delete --pause-on-failure,)

.PHONY: e2e-uninstall-all
e2e-uninstall-all: ## Run all uninstall e2e test scenarios
	@for scenario in $$(ls -d $(UNINSTALL_TEST_DIR)/*/  | grep -v _fixtures | xargs -n1 basename); do \
		echo "=== Running scenario: $$scenario ==="; \
		$(MAKE) e2e-uninstall SCENARIO=$$scenario || exit 1; \
	done

.PHONY: e2e-clean-uninstall
e2e-clean-uninstall: ## Force cleanup uninstall test resources
	-helm uninstall ngrok-operator-uninstall-test --namespace $(UNINSTALL_NAMESPACE) 2>/dev/null || true
	-helm uninstall ngrok-operator-a --namespace namespace-a 2>/dev/null || true
	-helm uninstall ngrok-operator-b --namespace namespace-b 2>/dev/null || true
	-helm uninstall ngrok-operator-crds --namespace kube-system 2>/dev/null || true
	-kubectl delete namespace $(UNINSTALL_NAMESPACE) namespace-a namespace-b --ignore-not-found --wait=false
	-rm -f /tmp/ko-id-*.txt
	@echo "Uninstall test cleanup complete"


.PHONY: helm-test
helm-test: _helm_setup ## Run helm unittest plugin
	$(MAKE) -C $(HELM_CHART_DIR) test
