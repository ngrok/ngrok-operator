HELM ?= helm

.PHONY: setup

setup:
	helm repo add bitnami https://charts.bitnami.com/bitnami
	helm dependency build .

.PHONY: lint
lint: ## Lint the helm chart
	helm lint .

.PHONY: template
template: ## Render the helm chart
	helm template ngrok-operator . --debug

.PHONY: test
test: ## Run helm unittest plugin
	helm unittest -f 'tests/**/*_test.yaml' .

.PHONY: update-snapshots
update-snapshots: ## Update helm unittest snapshots
	helm unittest -f 'tests/**/*_test.yaml' --update-snapshot .

.PHONY: update-readme
update-readme: ## Regenerate README.md and values.schema.json from values.yaml
	readme-generator --values "values.yaml" --readme "README.md" --schema "values.schema.json"
	python3 hack/schema-postprocess.py values.schema.json
