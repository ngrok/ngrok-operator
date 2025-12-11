#!/usr/bin/env bash

set -eu -o pipefail

GOVERSION="$(go env GOVERSION || echo "not installed")"

if ! [[ "$GOVERSION" == "go1.25" ||  "$GOVERSION" = "go1.25."* ]]; then
  echo "Detected go version $GOVERSION, but 1.25 is required"
  exit 1
fi

if ! command -v controller-gen >/dev/null 2>&1; then
  echo "ERROR: controller-gen not found on PATH."
  echo "Use 'nix develop' (recommended), or install controller-gen >= v0.19.0 manually:"
  echo "  go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0"
  exit 1
fi

if ! command -v helm >/dev/null 2>&1; then
  echo "ERROR: helm not found on PATH."
  echo "Use 'nix develop' (recommended), or install Helm 3 manually:"
  echo "  https://helm.sh/docs/intro/install/"
  exit 1
fi

if ! helm plugin list 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -qx "unittest"; then
  echo "ERROR: helm-unittest plugin not found."
  echo "Use 'nix develop' (recommended), or install the plugin manually:"
  echo "  helm plugin install https://github.com/helm-unittest/helm-unittest --version 0.6.1"
  exit 1
fi

echo "Preflight passed ✅"
