#!/usr/bin/env bash

# Remove finalizers from all ngrok-related CRs in all namespaces
# Handles all CRDs in *.k8s.ngrok.com API groups

set -euo pipefail

# Get all CRDs in any *.k8s.ngrok.com group
crds=$(kubectl get crds -o json | jq -r '.items[] | select(.spec.group | test("k8s.ngrok.com$")) | .metadata.name')

for crd in $crds; do
  kind=$(kubectl get crd "$crd" -o json | jq -r '.spec.names.kind')
  echo "Processing CRD: $crd (Kind: $kind)"
  # List all resources of this kind in all namespaces
  kubectl get "$crd" -A -o custom-columns=NAMESPACE:metadata.namespace,NAME:metadata.name --no-headers 2>/dev/null | \
  while read -r ns name; do
    if [[ -n "$ns" && -n "$name" ]]; then
      echo "Removing finalizers from $kind $name in namespace $ns"
      kubectl get "$crd" -n "$ns" "$name" -o json | jq '.metadata.finalizers = null' | kubectl apply -f -
    fi
  done
done
