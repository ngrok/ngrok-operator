#!/bin/bash
# Asserts that uninstall removed every ngrok CRD except the ngrok.com CRDs that
# carry helm.sh/resource-policy=keep, and that no KubernetesOperator is left in
# the kept KubernetesOperator CRD.
#
# LEGACY-kubernetesoperator-kind / LEGACY-trafficpolicy-kind / LEGACY-ippolicy-kind /
# LEGACY-agentendpoint-kind:
# R2 removes the keep annotation; then assert that every ngrok CRD is gone again.

set -o pipefail

KEPT='^customresourcedefinition.apiextensions.k8s.io/(agentendpoints|ippolicies|kubernetesoperators|trafficpolicies)\.ngrok\.com$'

# CRD deletion is asynchronous, so give the rest a moment to go.
remaining() { kubectl get crds -o name | grep ngrok | grep -v -E "$KEPT" || true; }
for _ in $(seq 20); do
  leftover=$(remaining)
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

echo "✓ ngrok CRDs removed, except the kept ngrok.com CRDs"
