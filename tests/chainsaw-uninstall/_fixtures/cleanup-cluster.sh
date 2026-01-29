#!/bin/bash
# Cleanup script to ensure a clean cluster state before running uninstall tests
# Usage: ./cleanup-cluster.sh [namespace1] [namespace2] ...
#
# This script:
# 1. Uninstalls any existing ngrok helm releases that might conflict
# 2. Removes ngrok CRDs (after removing finalizers from CRs)
# 3. Deletes specified test namespaces

set -o pipefail

# Default namespaces to clean up (can be overridden by args)
NAMESPACES="${*:-uninstall-test bound-target-ns}"

echo "=== Cleaning up ngrok helm releases ==="
# Force delete any stuck releases (pending-install, pending-upgrade, etc.)
for release in ngrok-operator-crds ngrok-operator-uninstall-test ngrok-operator-a ngrok-operator-b; do
  for ns in kube-system uninstall-test namespace-a namespace-b; do
    # Check if release exists in this namespace (any status)
    if helm list -n "$ns" --all 2>/dev/null | grep -q "^$release"; then
      echo "Uninstalling $release from $ns..."
      helm uninstall "$release" -n "$ns" --no-hooks --wait=false 2>/dev/null || true
      # Also delete the helm secret directly for stuck releases
      kubectl delete secret -n "$ns" "sh.helm.release.v1.${release}.v1" --ignore-not-found 2>/dev/null || true
    fi
  done
done

# Clean up any helm secrets left behind 
for ns in uninstall-test namespace-a namespace-b kube-system; do
  kubectl delete secret -n "$ns" -l owner=helm --ignore-not-found 2>/dev/null || true
done

echo "=== Removing finalizers from ngrok CRs and deleting CRDs ==="
# Delete CRDs so helm can recreate them with correct ownership annotations
# This is necessary because CRDs are not removed by helm uninstall by default
for crd in $(kubectl get crds -o name 2>/dev/null | grep ngrok); do
  crd_name=$(echo "$crd" | sed 's|customresourcedefinition.apiextensions.k8s.io/||')
  resource_name=$(echo "$crd_name" | cut -d. -f1)
  
  # Remove finalizers from all CRs of this CRD type first
  kubectl get "$resource_name" -A -o json 2>/dev/null | \
    jq -r '.items[] | "\(.metadata.namespace)/\(.metadata.name)"' 2>/dev/null | \
    while read -r ns_name; do
      ns=$(echo "$ns_name" | cut -d/ -f1)
      name=$(echo "$ns_name" | cut -d/ -f2)
      kubectl patch "$resource_name" "$name" -n "$ns" -p '{"metadata":{"finalizers":null}}' --type=merge 2>/dev/null || true
    done
  
  # Delete the CRD
  kubectl delete "$crd" --wait=false 2>/dev/null || true
done

# Wait briefly for CRD deletion to propagate
sleep 2

echo "=== Cleaning up resources in test namespaces ==="
# Don't delete namespaces - chainsaw manages them
# Just clean up resources within namespaces that might cause conflicts
for ns in $NAMESPACES; do
  if kubectl get ns "$ns" &>/dev/null; then
    # Delete any leftover deployments, services, etc.
    kubectl delete deployments,services,configmaps,secrets -n "$ns" -l app.kubernetes.io/managed-by=Helm --ignore-not-found 2>/dev/null || true
  fi
done

echo "✓ Cleanup complete"
