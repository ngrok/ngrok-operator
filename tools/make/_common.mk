# Wrapper to define common variables

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

ifndef ignore-not-found
  ignore-not-found = false
endif

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GIT_COMMIT = $(shell git rev-parse HEAD)
VERSION = $(shell cat VERSION)

# Image URL to use for all building/pushing image targets
IMG ?= ngrok-operator

# Repository URL
REPO_URL = github.com/ngrok/ngrok-operator

SCRIPT_DIR = ./scripts

CONTROLLER_GEN_PATHS = {./api/..., ./internal/controller/...}

# when true, deploy with --set oneClickDemoMode=true
DEPLOY_ONE_CLICK_DEMO_MODE ?= false

KUBE_DEPLOYMENT_NAME ?= ngrok-operator-manager
KUBE_NAMESPACE ?= ngrok-operator

HELM_RELEASE_NAME ?= ngrok-operator
HELM_CHART_DIR = ./helm/ngrok-operator
HELM_TEMPLATES_DIR = $(HELM_CHART_DIR)/templates


## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
BUF ?= $(LOCALBIN)/buf-$(BUF_VERSION)


## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.1
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.64.6
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0
BUF_VERSION ?= v1.52.1


# ==============================================
# Includes:
# ==============================================
include tools/make/help.mk
include tools/make/tools.mk
include tools/make/generate.mk
include tools/make/lint.mk
include tools/make/test.mk
include tools/make/build.mk
include tools/make/deploy.mk
include tools/make/release.mk
