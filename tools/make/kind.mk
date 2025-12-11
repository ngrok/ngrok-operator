##@ Kind

.PHONY: kind-create
kind-create: ## Create a local kind cluster for development.
	kind create cluster --name "$(KIND_CLUSTER_NAME)";

.PHONY: kind-delete
kind-delete: ## Delete the local kind cluster.
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"
