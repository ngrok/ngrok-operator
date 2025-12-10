##@ Tools/Dependencies

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef


.PHONY: preflight
preflight: ## Verifies required things like the go version
	scripts/preflight.sh


.PHONY: bootstrap-tools
bootstrap-tools: controller-gen envtest kind helm helm-unittest-plugin ## Install common local tooling.

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))


.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.
$(KIND): $(LOCALBIN)
	$(call go-install-tool,$(KIND),sigs.k8s.io/kind,$(KIND_VERSION))


.PHONY: helm
helm: $(HELM) ## Download helm locally if necessary.
$(HELM): $(LOCALBIN)
	$(call go-install-tool,$(HELM),helm.sh/helm/v3/cmd/helm,$(HELM_VERSION))

HELM_PLUGIN_HOME ?= $(LOCALBIN)/helm-plugins

.PHONY: helm-unittest-plugin
helm-unittest-plugin: helm ## Install helm-unittest plugin if needed.
	@mkdir -p "$(HELM_PLUGIN_HOME)"
	@if [ ! -d "$(HELM_PLUGIN_HOME)/helm-unittest" ]; then \
		echo "Installing helm-unittest plugin"; \
		HELM_PLUGINS="$(HELM_PLUGIN_HOME)" "$(HELM)" plugin install https://github.com/helm-unittest/helm-unittest --version 0.6.1; \
	else \
		echo "helm-unittest plugin already present"; \
	fi

.PHONY: _helm_setup
_helm_setup: helm ## Setup helm chart dependencies
	HELM="$(HELM)" HELM_PLUGINS="$(HELM_PLUGIN_HOME)" $(MAKE) -C $(HELM_CHART_DIR) setup
