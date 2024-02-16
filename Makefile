
# Image URL to use all building/pushing image targets
IMG ?= kubernetes-ingress-controller
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23

REPO_URL = github.com/ngrok/kubernetes-ingress-controller

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

HELM_CHART_DIR = ./helm/ingress-controller
HELM_TEMPLATES_DIR = $(HELM_CHART_DIR)/templates

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
	$(CONTROLLER_GEN) rbac:roleName=ngrok-ingress-controller-manager-role crd webhook paths="{./api/ingress/v1alpha1/, ./internal/controller/ingress/, ./internal/controller/gateway}" \
		output:crd:artifacts:config=$(HELM_TEMPLATES_DIR)/crds \
		output:rbac:artifacts:config=$(HELM_TEMPLATES_DIR)/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="{./api/ingress/v1alpha1/, ./internal/controller/ingress/, ./internal/controller/gateway}"

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out -timeout 20s

##@ Build

.PHONY: build
build: preflight generate fmt vet _build ## Build manager binary.

.PHONY: _build
_build:
	go build -o bin/manager -trimpath -ldflags "-s -w \
		-X $(REPO_URL)/internal/version.gitCommit=$(GIT_COMMIT) \
		-X $(REPO_URL)/internal/version.version=$(VERSION)" cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	DOCKER_BUILDKIT=1 docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade ngrok-ingress-controller $(HELM_CHART_DIR) --install \
		--namespace ngrok-ingress-controller \
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
	kubectl rollout restart deployment ngrok-ingress-controller-kubernetes-ingress-controller-manager -n ngrok-ingress-controller

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
	helm uninstall ngrok-ingress-controller

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.9.2

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

##@ Helm

.PHONY: _helm_setup
_helm_setup:
	./scripts/helm-setup.sh
	helm repo add bitnami https://charts.bitnami.com/bitnami
	helm dependency update $(HELM_CHART_DIR)

.PHONY: helm-lint
helm-lint: _helm_setup ## Lint the helm chart
	helm lint $(HELM_CHART_DIR)

.PHONY: helm-test
helm-test: _helm_setup ## Run helm unittest plugin
	helm unittest --helm3 $(HELM_CHART_DIR)

.PHONY: helm-update-snapshots
helm-update-snapshots: _helm_setup ## Update helm unittest snapshots
	helm unittest --helm3 -u $(HELM_CHART_DIR)
