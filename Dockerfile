# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.23 AS builder

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

# Prepare CA certificates
FROM golang:1.23 AS certs
WORKDIR /certs

## Copy the Intermediate CA Certificate Copy the Intermediate CA Certificate directly into the image
COPY ngrok-ca.crt /certs/ngrok-ca.crt
# Append it to the system CA bundle
RUN cat /etc/ssl/certs/ca-certificates.crt /certs/ngrok-ca.crt > /certs/ca-certificates.crt

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
# Copy compiled binaries
COPY --from=builder /workspace/bin/api-manager /workspace/bin/agent-manager /workspace/bin/bindings-forwarder-manager ./
# Copy the updated CA bundle into the final image
COPY --from=certs /certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER 65532:65532

ENTRYPOINT ["/api-manager"]
