##@ Linting/Formatting

.PHONY: lint
lint: golangci-lint lint-markers ## Run golangci-lint linter & marker validation
	$(GOLANGCI_LINT) run


.PHONY: lint-markers
lint-markers: ## Check for invalid kubebuilder marker prefixes
	@$(SCRIPT_DIR)/lint-markers.sh


.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix


.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...


.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...


.PHONY: helm-lint
helm-lint: _helm_setup ## Lint the helm chart
	HELM="$(HELM)" HELM_PLUGINS="$(HELM_PLUGIN_HOME)" $(MAKE) -C $(HELM_CHART_DIR) lint
