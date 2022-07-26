#!/bin/bash

GO_VERSION=$(go version)

# Ensure go version is 1.17
if [[ $GO_VERSION != *"go version go1.17"* ]]; then
  echo "Go version must be 1.17"
  exit 1
fi
