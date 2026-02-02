# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.25.5 AS builder

ARG TARGETOS TARGETARCH
ARG GIT_COMMIT=""

WORKDIR /workspace

# Copy the Go Modules manifests first for better layer caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go \
	go mod download

# Copy the rest
COPY . .

# Build
RUN --mount=type=cache,target=/go \
	CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" GIT_COMMIT="${GIT_COMMIT}" ./scripts/build.sh
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
COPY certs /etc/ssl/certs/ngrok
WORKDIR /
COPY --from=builder /workspace/bin/ngrok-operator ./
USER 65532:65532


ENTRYPOINT ["/ngrok-operator"]
