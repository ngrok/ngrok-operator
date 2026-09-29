##@ Kind

.PHONY: kind-create
kind-create: ## Create a local kind cluster for development.
	kind create cluster --name "$(KIND_CLUSTER_NAME)";

.PHONY: kind-delete
kind-delete: ## Delete the local kind cluster.
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"

.PHONY: kind-private-endpoints-dns
kind-private-endpoints-dns: ## Add CoreDNS rewrites so kind pods resolve private endpoint names.
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) ./scripts/kind-private-endpoints-dns.sh
