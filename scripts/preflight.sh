#!/usr/bin/env bash

set -eu -o pipefail

# The required minor comes from go.mod rather than being hardcoded here, so
# that a Go version bump only touches flake.nix and go.mod. The exact patch
# version is whatever flake.nix pins, and is not checked: the image is built
# from that same toolchain (see scripts/build.sh), so there is nothing to drift.
REQUIRED_MINOR="$(go mod edit -json | jq -r '.Go' | cut -d. -f1,2)"
GOVERSION="$(go env GOVERSION || echo "not installed")"

# Prerelease toolchains (go1.27rc2, go1.27beta1) are accepted: nixpkgs can ship
# an RC for weeks before the final release lands, and the language features we
# target are frozen at the first RC.
REQUIRED_MINOR_RE="${REQUIRED_MINOR//./\\.}"
if ! [[ "$GOVERSION" =~ ^go${REQUIRED_MINOR_RE}([.][0-9]+|(alpha|beta|rc)[0-9]+)?$ ]]; then
  echo "Detected go version $GOVERSION, but ${REQUIRED_MINOR} is required"
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
