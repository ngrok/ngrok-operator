##@ Kind

.PHONY: kind-create
kind-create: ## Create a local kind cluster for development.
	kind create cluster --name "$(KIND_CLUSTER_NAME)";

.PHONY: kind-delete
kind-delete: ## Delete the local kind cluster.
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"

.PHONY: kind-private-endpoints-dns
kind-private-endpoints-dns: ## Point kind's CoreDNS at the private endpoint forwarder.
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) PRIVATE_DNS_SERVICE=$(HELM_RELEASE_NAME)-private-dns ./scripts/kind-private-endpoints-dns.sh
