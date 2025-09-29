HELM ?= helm
HELM_PLUGINS ?= $(abspath ../../bin/helm-plugins)

.PHONY: setup

setup:
	HELM_PLUGINS="$(HELM_PLUGINS)" "$(HELM)" repo add bitnami https://charts.bitnami.com/bitnami
	HELM_PLUGINS="$(HELM_PLUGINS)" "$(HELM)" dependency build .

.PHONY: lint
lint: ## Lint the helm chart
	"$(HELM)" lint .

.PHONY: template
template: ## Render the helm chart
	"$(HELM)" template ngrok-operator . --debug

.PHONY: test
test: ## Run helm unittest plugin
	"$(HELM)" unittest -f 'tests/**/*_test.yaml' .

.PHONY: update-snapshots
update-snapshots: ## Update helm unittest snapshots
	"$(HELM)" unittest -f 'tests/**/*_test.yaml' --update-snapshot .

.PHONY: update-readme
update-readme: ## Updates the README.md file with the chart values
	readme-generator --values "values.yaml" --readme "README.md" --schema "values.schema.json"
