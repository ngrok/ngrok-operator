##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

IMAGE ?= ngrok-rollouts-plugin:latest
BINARY = bin/ngrok-traffic-router

.PHONY: build
build: ## Build the plugin binary.
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/...

.PHONY: test
test: ## Run unit and integration tests.
	go test ./...

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Run gofmt.
	gofmt -w .

.PHONY: tidy
tidy: ## Tidy go modules.
	go mod tidy

##@ Docker

.PHONY: docker-build
docker-build: build ## Build the plugin Docker image.
	docker build -t $(IMAGE) .
	@echo "Built image: $(IMAGE)"

.PHONY: docker-push
docker-push: ## Push the plugin Docker image.
	docker push $(IMAGE)

##@ Kind (local development)

KIND_CLUSTER_NAME ?= ngrok

.PHONY: kind-load
kind-load: docker-build ## Build and load the image into the kind cluster.
	kind load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)
	@echo "Loaded $(IMAGE) into kind cluster $(KIND_CLUSTER_NAME)"
