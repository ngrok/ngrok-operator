#!/usr/bin/env bash

set -e

direnv allow
direnv exec . make bootstrap-tools
direnv exec . make kind-create || true
