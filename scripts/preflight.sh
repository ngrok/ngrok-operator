#!/usr/bin/env bash

set -eu -o pipefail

GOVERSION="$(go env GOVERSION || echo "not installed")"

if ! [[ "$GOVERSION" == "go1.24" ||  "$GOVERSION" = "go1.24."* ]]; then
  echo "Detected go version $GOVERSION, but 1.24 is required"
  exit 1
fi

if ! command -v controller-gen >/dev/null 2>&1; then
  echo "ERROR: controller-gen not found on PATH."
  echo "Use 'nix develop' (recommended), or install controller-gen >= v0.19.0 manually:"
  echo "  go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0"
  exit 1
fi

echo "Preflight passed ✅"
