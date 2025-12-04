##@ Generated Code/Files

.PHONY: generate
generate: controller-gen generate-mocks ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="$(CONTROLLER_GEN_PATHS)"

.PHONY: generate-mocks
generate-mocks:
	go generate ./...

.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=ngrok-operator-manager-role crd webhook paths="$(CONTROLLER_GEN_PATHS)" \
		output:crd:artifacts:config=$(HELM_TEMPLATES_DIR)/crds \
		output:rbac:artifacts:config=$(HELM_CHART_DIR)/generated/rbac



.PHONY: manifest-bundle
manifest-bundle: _helm_setup ## Generates the manifest-bundle at the root of the repo.
	$(HELM) template ngrok-operator $(HELM_CHART_DIR) \
		--namespace $(KUBE_NAMESPACE) \
		--set credentials.secret.name="ngrok-operator-credentials" > manifest-bundle.yaml

.PHONY: helm-update-snapshots
helm-update-snapshots: _helm_setup helm-unittest-plugin ## Update helm unittest snapshots
	HELM="$(HELM)" HELM_PLUGINS="$(HELM_PLUGIN_HOME)" $(MAKE) -C $(HELM_CHART_DIR) update-snapshots


helm-update-snapshots-no-deps: helm-unittest-plugin ## Update helm unittest snapshots without rebuilding dependencies
	HELM="$(HELM)" HELM_PLUGINS="$(HELM_PLUGIN_HOME)" $(MAKE) -C $(HELM_CHART_DIR) update-snapshots
