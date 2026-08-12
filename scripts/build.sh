#!/bin/sh
# Shared build script used by the Makefile and CI.
#
# With no arguments, builds for the host into bin/ngrok-operator. This is the
# dev workflow (make build).
#
# Given one or more <os>/<arch> platforms (comma- or space-separated), builds
# one binary per platform into bin/ngrok-operator-<os>-<arch>. Those are what
# the Dockerfile COPYs, which is how images end up built with the same Go
# toolchain that flake.nix pins for the devShell.
set -e

VERSION="${VERSION:-$(cat VERSION 2>/dev/null || echo "0.0.0")}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo "")}"
REPO_URL="github.com/ngrok/ngrok-operator"

LDFLAGS="-s -w \
    -X ${REPO_URL}/internal/version.gitCommit=${GIT_COMMIT} \
    -X ${REPO_URL}/internal/version.version=${VERSION}"

if [ "$#" -eq 0 ]; then
    go build -o bin/ngrok-operator -trimpath -ldflags "$LDFLAGS"
    exit 0
fi

for platform in $(echo "$@" | tr ',' ' '); do
    os="${platform%/*}"
    arch="${platform#*/}"

    if [ "$os" = "$platform" ] || [ -z "$os" ] || [ -z "$arch" ]; then
        echo "invalid platform '${platform}', expected <os>/<arch>" >&2
        exit 1
    fi

    echo "building bin/ngrok-operator-${os}-${arch}"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -o "bin/ngrok-operator-${os}-${arch}" -trimpath -ldflags "$LDFLAGS"
done
