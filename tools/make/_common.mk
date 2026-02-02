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
IMG ?= ngrok-operator-local

# Repository URL
REPO_URL = github.com/ngrok/ngrok-operator

SCRIPT_DIR = ./scripts

CONTROLLER_GEN_PATHS = {./api/..., ./internal/controller/..., ./internal/drain/...}

# when true, deploy with --set oneClickDemoMode=true
DEPLOY_ONE_CLICK_DEMO_MODE ?= false

# Timestamp used to force pod rollouts via annotations on each deploy.
DEPLOY_ROLLOUT_TIMESTAMP := $(shell date +%s)

KUBE_DEPLOYMENT_NAME ?= ngrok-operator-manager
KUBE_AGENT_MANAGER_DEPLOYMENT_NAME ?= ngrok-operator-agent
KUBE_NAMESPACE ?= ngrok-operator
KIND_CLUSTER_NAME ?= ngrok-operator

HELM_RELEASE_NAME ?= ngrok-operator
HELM_CHART_DIR = ./helm/ngrok-operator
CRD_CHART_DIR = ./helm/ngrok-crds
HELM_TEMPLATES_DIR = $(HELM_CHART_DIR)/templates
CRD_TEMPLATES_DIR = $(CRD_CHART_DIR)/templates


## Tool Versions
# controller-gen, setup-envtest, helm, kind are provided by nixpkgs; use 'nix develop'

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
include tools/make/kind.mk
include tools/make/release.mk
