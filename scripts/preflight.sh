#!/usr/bin/env bash

set -eu -o pipefail

GOVERSION="$(go env GOVERSION || echo "not installed")"

if ! [[ "$GOVERSION" == "go1.18" ||  "$GOVERSION" = "go1.18."* ]]; then
  echo "Detected go version $GOVERSION, but 1.18 is required"
  exit 1
fi
