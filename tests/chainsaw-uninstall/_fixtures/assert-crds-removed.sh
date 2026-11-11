#!/bin/bash
# Asserts that uninstall removed every ngrok CRD except those that carry
# helm.sh/resource-policy=keep, and that no KubernetesOperator is left in the
# kept KubernetesOperator CRD.
#
# The kept set is read from the CRDs themselves, so migrating a kind, or R2
# removing the annotation, needs no change here.

set -o pipefail

remaining() {
  local crds
  if ! crds=$(kubectl get crds -o json); then
    echo "✗ could not list CRDs" >&2
    return 1
  fi
  if ! echo "$crds" | jq -r '.items[]
    | select(.metadata.name | test("ngrok"))
    | select(.metadata.annotations["helm.sh/resource-policy"] != "keep")
    | .metadata.name'; then
    echo "✗ could not read the CRD list" >&2
    return 1
  fi
}

# CRD deletion is asynchronous, so give the rest a moment to go.
for _ in $(seq 20); do
  leftover=$(remaining) || exit 1
  [ -z "$leftover" ] && break
  sleep 1
done
if [ -n "$leftover" ]; then
  echo "✗ ngrok CRDs still exist:"
  echo "$leftover"
  exit 1
fi

kos=$(kubectl get kubernetesoperators.ngrok.com -A -o name --ignore-not-found 2>/dev/null || true)
if [ -n "$kos" ]; then
  echo "✗ KubernetesOperators left in the kept CRD:"
  echo "$kos"
  exit 1
fi

echo "✓ ngrok CRDs removed, except the kept ones"
