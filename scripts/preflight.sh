#!/usr/bin/env bash

set -eu -o pipefail

GOVERSION="$(go env GOVERSION || echo "not installed")"

if ! [[ "$GOVERSION" == "go1.19" ||  "$GOVERSION" = "go1.19."* ]]; then
  echo "Detected go version $GOVERSION, but 1.19 is required"
  exit 1
fi
