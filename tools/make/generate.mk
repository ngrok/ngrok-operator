##@ Generated Code/Files

.PHONY: generate
generate: controller-gen generate-mocks ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="$(CONTROLLER_GEN_PATHS)"

.PHONY: generate-mocks
generate-mocks:
	go generate ./...

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=ngrok-operator-manager-role crd webhook paths="$(CONTROLLER_GEN_PATHS)" \
		output:crd:artifacts:config=$(HELM_TEMPLATES_DIR)/crds \
		output:rbac:artifacts:config=$(HELM_TEMPLATES_DIR)/rbac


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


##@ Documentation

.PHONY: docs-generate
docs-generate: manifests crdoc yq ## Generate CRD markdown documentation
	@echo "Generating CRD markdown documentation..."
	@mkdir -p docs/crds
	
	# Create README header
	@echo "# CRD Reference Documentation" > docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "Auto-generated API reference for ngrok-operator Custom Resource Definitions." >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "## Available CRDs" >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	
	# Generate docs and add to README
	@for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do \
		group=$$($(YQ) eval '.spec.group' "$$crd"); \
		shortgroup=$${group%%.*}; \
		kind=$$($(YQ) eval '.spec.names.kind' "$$crd"); \
		kindlower=$$(echo $$kind | tr '[:upper:]' '[:lower:]'); \
		for ver in $$($(YQ) eval '.spec.versions[] | select(.served==true) | .name' "$$crd"); do \
			dir=docs/crds/$$shortgroup/$$ver; \
			mkdir -p $$dir/examples; \
			echo "  Generating $$shortgroup/$$ver/$$kindlower.md..."; \
			$(CRDOC) --resources "$$crd" --output $$dir/$$kindlower.md; \
			echo "- [$$kind]($$shortgroup/$$ver/$$kindlower.md) - \`$$group/$$ver\`" >> docs/crds/README.md; \
		done; \
	done
	@echo "" >> docs/crds/README.md
	@echo "## Documentation Types" >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "- **API Reference**: Field descriptions, types, and validation rules" >> docs/crds/README.md
	@echo "- **Conditions**: Status condition types and reasons (in \`*-conditions.md\` files)" >> docs/crds/README.md
	@echo "- **Examples**: Sample YAML manifests (in \`examples/\` directories)" >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "Markdown documentation generated in docs/crds/"

.PHONY: docs-generate-json
docs-generate-json: manifests yq ## Generate JSON documentation for docs site
	@echo "Generating JSON documentation for docs site..."
	@mkdir -p docs/generated/crds
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
		filename=$$(basename $$crd .yaml); \
		echo "  Generating $$filename.json..."; \
		$(YQ) eval -o=json $$crd > docs/generated/crds/$$filename.json; \
	done
	@echo "JSON documentation generated in docs/generated/crds/"

.PHONY: docs-verify
docs-verify: docs-generate ## Verify CRD markdown is up to date
	@git diff --exit-code docs/crds/ || \
		(echo "ERROR: CRD documentation is out of date. Run 'make docs-generate' to update." && exit 1)

.PHONY: docs-clean
docs-clean: ## Clean generated documentation
	@echo "Cleaning generated documentation..."
	@rm -rf docs/generated/
	@find docs/crds -type d -name 'v1*' -exec rm -rf {} + 2>/dev/null || true
	@rm -f docs/crds/README.md
	@echo "Documentation cleaned (manual *-conditions.md files preserved)"
