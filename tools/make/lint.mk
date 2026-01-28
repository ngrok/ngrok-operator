##@ Linting/Formatting

.PHONY: lint
lint: lint-markers ## Run golangci-lint linter & yamllint
	golangci-lint run


.PHONY: lint-markers
lint-markers: ## Lint kubebuilder markers for common typos
	@go run ./tools/markerlint/cmd/markerlint ./api/...


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
	$(MAKE) -C $(HELM_CHART_DIR) lint
