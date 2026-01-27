##@ Building

.PHONY: all
all: build


.PHONY: build
build: preflight generate fmt vet _build ## Build binaries.


.PHONY: _build
_build:
	VERSION=$(VERSION) GIT_COMMIT=$(GIT_COMMIT) $(SCRIPT_DIR)/build.sh

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	DOCKER_BUILDKIT=1 docker build --build-arg GIT_COMMIT=$(GIT_COMMIT) -t ${IMG} .


.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}
