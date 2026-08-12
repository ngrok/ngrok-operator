# The manager binary is built on the host by scripts/build.sh rather than in a
# golang builder stage, so that images are always built with the Go toolchain
# pinned in flake.nix and there is no second Go version to keep in sync.
#
# Use `make docker-build`, or run `make image-binaries` before `docker build`.
#
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

ARG TARGETOS
ARG TARGETARCH

COPY certs /etc/ssl/certs/ngrok
COPY bin/ngrok-operator-${TARGETOS}-${TARGETARCH} /ngrok-operator
WORKDIR /
USER 65532:65532


ENTRYPOINT ["/ngrok-operator"]
