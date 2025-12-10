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

.PHONY: _helm_setup
_helm_setup: ## Setup helm chart dependencies
	$(MAKE) -C $(HELM_CHART_DIR) setup
