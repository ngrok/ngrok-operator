##@ Linting/Formatting

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run


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
	$(MAKE) -C $(HELM_CHART_DIR) lint
