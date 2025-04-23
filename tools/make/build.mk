##@ Building

.PHONY: all
all: build


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


.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	DOCKER_BUILDKIT=1 docker build -t ${IMG} .


.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}
