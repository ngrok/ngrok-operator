##@ Kind

.PHONY: kind-create
kind-create: ## Create a local kind cluster for development.
	kind create cluster --name "$(KIND_CLUSTER_NAME)";

.PHONY: kind-delete
kind-delete: ## Delete the local kind cluster.
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"

PE_INCLUDE_INTERNAL ?=

.PHONY: kind-private-endpoints-dns
kind-private-endpoints-dns: ## Add CoreDNS rewrites so kind pods resolve private endpoint names (*.ngrok.direct; PE_INCLUDE_INTERNAL=true adds *.internal).
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) PE_INCLUDE_INTERNAL=$(PE_INCLUDE_INTERNAL) ./scripts/kind-private-endpoints-dns.sh
