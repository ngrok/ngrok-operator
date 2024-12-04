# Image URL to use all building/pushing image targets
IMG ?= ngrok-operator

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Repository URL
REPO_URL = github.com/ngrok/ngrok-operator

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = $(shell cat VERSION)

# Tools

SCRIPT_DIR = ./scripts

HELM_CHART_DIR = ./helm/ngrok-operator
HELM_TEMPLATES_DIR = $(HELM_CHART_DIR)/templates

CONTROLLER_GEN_PATHS = {./api/..., ./internal/controller/...}

# Default Environment Variables

# when true, deploy with --set oneClickDemoMode=true
DEPLOY_ONE_CLICK_DEMO_MODE ?= false

# Targets

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: preflight
preflight: ## Verifies required things like the go version
	scripts/preflight.sh

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=ngrok-operator-manager-role crd webhook paths="$(CONTROLLER_GEN_PATHS)" \
		output:crd:artifacts:config=$(HELM_TEMPLATES_DIR)/crds \
		output:rbac:artifacts:config=$(HELM_TEMPLATES_DIR)/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="$(CONTROLLER_GEN_PATHS)"

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out -timeout 20s

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: validate
validate: build test lint manifests helm-update-snapshots ## Validate the codebase before a PR

##@ Build

.PHONY: build
build: preflight generate fmt vet _build ## Build binaries.

.PHONY: _build
_build:
	go build -o bin/api-manager -trimpath -ldflags "-s -w \
		-X $(REPO_URL)/internal/version.gitCommit=$(GIT_COMMIT) \
		-X $(REPO_URL)/internal/version.version=$(VERSION)" cmd/api/main.go
	go build -o bin/agent-manager -trimpath -ldflags "-s -w \
		-X $(REPO_URL)/internal/version.gitCommit=$(GIT_COMMIT) \
		-X $(REPO_URL)/internal/version.version=$(VERSION)" cmd/agent/main.go
	go build -o bin/bindings-forwarder-manager -trimpath -ldflags "-s -w \
		-X $(REPO_URL)/internal/version.gitCommit=$(GIT_COMMIT) \
		-X $(REPO_URL)/internal/version.version=$(VERSION)" cmd/bindings-forwarder/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/api/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	DOCKER_BUILDKIT=1 docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

KUBE_NAMESPACE ?= ngrok-operator
HELM_RELEASE_NAME ?= ngrok-operator
KUBE_DEPLOYMENT_NAME ?= ngrok-operator-manager

.PHONY: release
release:
	$(SCRIPT_DIR)/release.sh

.PHONY: deploy
deploy: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile &&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)

.PHONY: deploy_gateway
deploy_gateway: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile \
		--set useExperimentalGatewayApi=true &&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)

.PHONY: deploy_with_bindings
deploy_with_bindings: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile \
		--set bindings.enabled=true \
		--set bindings.name=k8s/dev-testing \
		--set bindings.description="Example binding for dev testing" \
		--set bindings.allowedURLs="{*}" \
		&&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)

.PHONY: deploy_for_e2e
deploy_for_e2e: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set oneClickDemoMode=$(DEPLOY_ONE_CLICK_DEMO_MODE) \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"e2e\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile \
		--set bindings.enabled=true \
		--set bindings.name=$(E2E_BINDING_NAME) \
		--set bindings.description="Example binding for CI e2e tests" \
		--set bindings.allowedURLs='{*.e2e}' \
		--set bindings.serviceAnnotations.annotation1="val1" \
		--set bindings.serviceAnnotations.annotation2="val2" \
		--set bindings.serviceLabels.label1="val1"

.PHONY: _deploy-check-env-vars
_deploy-check-env-vars:
ifndef NGROK_API_KEY
	$(error An NGROK_API_KEY must be set)
endif
ifndef NGROK_AUTHTOKEN
	$(error An NGROK_AUTHTOKEN must be set)
endif

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	helm uninstall ngrok-operator

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.1
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.60.3

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

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

##@ Helm

.PHONY: _helm_setup
_helm_setup: ## Setup helm dependencies
	$(MAKE) -C $(HELM_CHART_DIR) setup

.PHONY: helm-lint
helm-lint: _helm_setup ## Lint the helm chart
	$(MAKE) -C $(HELM_CHART_DIR) lint

.PHONY: helm-test
helm-test: _helm_setup ## Run helm unittest plugin
	$(MAKE) -C $(HELM_CHART_DIR) test

.PHONY: helm-update-snapshots
helm-update-snapshots: _helm_setup ## Update helm unittest snapshots
	$(MAKE) -C $(HELM_CHART_DIR) update-snapshots

helm-update-snapshots-no-deps: ## Update helm unittest snapshots without rebuilding dependencies
	$(MAKE) -C $(HELM_CHART_DIR) update-snapshots

##@ E2E tests

.PHONY: e2e-tests
e2e-tests: ## Run e2e tests
	chainsaw test ./tests/chainsaw

.PHONY: e2e-clean
e2e-clean: ## Clean up e2e tests
	kubectl delete ns e2e
	kubectl delete --all boundendpoints -n ngrok-operator
	kubectl delete --all services -n ngrok-operator
	kubectl delete --all kubernetesoperators -n ngrok-operator
	helm --namespace ngrok-operator uninstall ngrok-operator
