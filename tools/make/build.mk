##@ Building

.PHONY: all
all: build


.PHONY: build
build: preflight generate fmt vet _build ## Build binaries.


.PHONY: _build
_build:
	VERSION=$(VERSION) GIT_COMMIT=$(GIT_COMMIT) $(SCRIPT_DIR)/build.sh

# Architecture the local docker daemon runs, which is also what a kind node
# runs. Deliberately not `go env GOARCH`: on Apple Silicon with an amd64 daemon
# those differ, and we need the binary to match the daemon. Recursively
# expanded so `docker version` only runs for targets that need it.
DOCKER_ARCH = $(shell docker version --format '{{.Server.Arch}}' 2>/dev/null || go env GOARCH)

# Platforms to build image binaries for. Defaults to just the local daemon's
# so the dev/e2e loop stays fast; CI overrides it with the full release set.
IMAGE_PLATFORMS ?= linux/$(DOCKER_ARCH)

.PHONY: image-binaries
image-binaries: ## Build the linux binaries that get baked into the container image.
	VERSION=$(VERSION) GIT_COMMIT=$(GIT_COMMIT) $(SCRIPT_DIR)/build.sh $(IMAGE_PLATFORMS)

.PHONY: docker-build
docker-build: image-binaries ## Build docker image with the manager.
	DOCKER_BUILDKIT=1 docker build --platform=linux/$(DOCKER_ARCH) -t ${IMG} .


.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}
