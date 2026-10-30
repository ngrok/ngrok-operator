#!/usr/bin/env bash
# Runs a shell command in the test namespace's client pod until its output
# contains the wanted substring. Retries ride out the poll interval and the
# CoreDNS negative cache right after endpoints come up.
# Usage: check.sh <want-substring> <command>
set -uo pipefail

want="$1"
cmd="$2"
out=""
for _ in $(seq 1 "${RETRIES:-20}"); do
  out="$(kubectl -n "$NAMESPACE" exec client -- bash -c "$cmd" 2>&1)"
  if [[ "$out" == *"$want"* ]]; then
    echo "PASS  $cmd"
    exit 0
  fi
  sleep 3
done
echo "FAIL  $cmd: want \"$want\", got: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-300)"
exit 1
