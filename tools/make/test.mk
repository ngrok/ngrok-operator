##@ Testing

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $(if $(PACKAGE),$(PACKAGE),./...) -coverprofile cover.out -timeout 120s

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
	chainsaw test ./tests/chainsaw --exclude-test-regex 'chainsaw/_skip_*.yaml'


.PHONY: e2e-clean
e2e-clean: ## Clean up e2e tests
	kubectl delete ns e2e
	kubectl delete --all boundendpoints -n ngrok-operator
	kubectl delete --all services -n ngrok-operator
	kubectl delete --all kubernetesoperators -n ngrok-operator
	helm --namespace ngrok-operator uninstall ngrok-operator


.PHONY: helm-test
helm-test: _helm_setup helm-unittest-plugin ## Run helm unittest plugin
	HELM="$(HELM)" HELM_PLUGINS="$(HELM_PLUGIN_HOME)" $(MAKE) -C $(HELM_CHART_DIR) test
