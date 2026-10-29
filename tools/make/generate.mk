##@ Generated Code/Files

.PHONY: generate
generate: generate-mocks ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="$(CONTROLLER_GEN_PATHS)"

.PHONY: generate-mocks
generate-mocks:
	go generate ./...

.PHONY: manifests
manifests: ## Generate WebhookConfiguration and CustomResourceDefinition objects.
	controller-gen crd webhook paths="$(CONTROLLER_GEN_PATHS)" \
		output:crd:artifacts:config=$(CRD_TEMPLATES_DIR)
	@# LEGACY-domain-kind: delete at cleanup. Keep the ngrok.com/v1 Domain CRD
	@# when Helm rolls back to a release without it: deleting the CRD would
	@# cascade-delete every v1 Domain, and their reconciles could release
	@# reservations the rolled-back operator still serves through the legacy
	@# kind. controller-gen has no marker for CRD annotations, so add it here
	@# (awk rather than sed: BSD sed has no \n in replacements).
	@awk '{ print } /^    controller-gen\.kubebuilder\.io\/version: / { print "    helm.sh/resource-policy: keep" }' \
		$(CRD_TEMPLATES_DIR)/ngrok.com_domains.yaml > $(CRD_TEMPLATES_DIR)/ngrok.com_domains.yaml.tmp
	@mv $(CRD_TEMPLATES_DIR)/ngrok.com_domains.yaml.tmp $(CRD_TEMPLATES_DIR)/ngrok.com_domains.yaml


.PHONY: manifest-bundle
manifest-bundle: _helm_setup ## Generates the manifest-bundle at the root of the repo.
	helm template ngrok-operator $(HELM_CHART_DIR) \
		--namespace $(KUBE_NAMESPACE) \
		--set ngrok.credentials.secret.name="ngrok-operator-credentials" > manifest-bundle.yaml

.PHONY: helm-update-snapshots
helm-update-snapshots: _helm_setup ## Update helm unittest snapshots
	$(MAKE) -C $(HELM_CHART_DIR) update-snapshots


helm-update-snapshots-no-deps: ## Update helm unittest snapshots without rebuilding dependencies
	$(MAKE) -C $(HELM_CHART_DIR) update-snapshots
