# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.24.2 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go \
	go mod download

# Copy the go source
COPY . .

ARG TARGETOS TARGETARCH

# Build
RUN --mount=type=cache,target=/go \
	CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" make _build
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
COPY certs /etc/ssl/certs/ngrok
WORKDIR /
COPY --from=builder /workspace/bin/ngrok-operator ./
USER 65532:65532


ENTRYPOINT ["/ngrok-operator"]
