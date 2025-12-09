##@ Linting/Formatting

.PHONY: lint
lint: ## Run golangci-lint linter & yamllint
	golangci-lint run


.PHONY: lint-fix
lint-fix: ## Run golangci-lint linter and perform fixes
	golangci-lint run --fix


.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...


.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...


.PHONY: helm-lint
helm-lint: _helm_setup ## Lint the helm chart
	HELM="$(HELM)" HELM_PLUGINS="$(HELM_PLUGIN_HOME)" $(MAKE) -C $(HELM_CHART_DIR) lint
