#!/bin/sh
# Shared build script used by Makefile and Dockerfile
set -e

VERSION="${VERSION:-$(cat VERSION 2>/dev/null || echo "0.0.0")}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo "")}"
REPO_URL="github.com/ngrok/ngrok-operator"

go build -o bin/ngrok-operator -trimpath -ldflags "-s -w \
    -X ${REPO_URL}/internal/version.gitCommit=${GIT_COMMIT} \
    -X ${REPO_URL}/internal/version.version=${VERSION}"
