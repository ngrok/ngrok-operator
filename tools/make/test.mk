##@ Testing

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out -timeout 60s


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
helm-test: _helm_setup ## Run helm unittest plugin
	$(MAKE) -C $(HELM_CHART_DIR) test
