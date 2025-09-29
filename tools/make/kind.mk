##@ Kind

.PHONY: kind-create
kind-create: kind ## Create a local kind cluster for development.
	"$(KIND)" create cluster --name "$(KIND_CLUSTER_NAME)";

.PHONY: kind-delete
kind-delete: kind ## Delete the local kind cluster.
	"$(KIND)" delete cluster --name "$(KIND_CLUSTER_NAME)"
