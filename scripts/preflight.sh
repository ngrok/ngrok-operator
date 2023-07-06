#!/usr/bin/env bash

set -eu -o pipefail

GOVERSION="$(go env GOVERSION || echo "not installed")"

if ! [[ "$GOVERSION" == "go1.20" ||  "$GOVERSION" = "go1.20."* ]]; then
  echo "Detected go version $GOVERSION, but 1.20 is required"
  exit 1
else
  echo "Preflight passed ✅"
fi
