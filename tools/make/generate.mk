##@ Generated Code/Files

.PHONY: generate
generate: generate-mocks ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="$(CONTROLLER_GEN_PATHS)"

.PHONY: generate-mocks
generate-mocks:
	go generate ./...

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	controller-gen rbac:roleName='"{{ include \"ngrok-operator.fullname\" . }}-manager-role"' crd webhook paths="$(CONTROLLER_GEN_PATHS)" \
		output:crd:artifacts:config=$(CRD_TEMPLATES_DIR) \
		output:rbac:artifacts:config=$(HELM_TEMPLATES_DIR)/rbac


.PHONY: manifest-bundle
manifest-bundle: _helm_setup ## Generates the manifest-bundle at the root of the repo.
	helm template ngrok-operator $(HELM_CHART_DIR) \
		--namespace $(KUBE_NAMESPACE) \
		--set credentials.secret.name="ngrok-operator-credentials" > manifest-bundle.yaml

.PHONY: helm-update-snapshots
helm-update-snapshots: _helm_setup ## Update helm unittest snapshots
	$(MAKE) -C $(HELM_CHART_DIR) update-snapshots


helm-update-snapshots-no-deps: ## Update helm unittest snapshots without rebuilding dependencies
	$(MAKE) -C $(HELM_CHART_DIR) update-snapshots
