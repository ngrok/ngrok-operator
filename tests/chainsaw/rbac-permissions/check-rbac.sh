#!/usr/bin/env bash
# check-rbac.sh — Verify RBAC permissions for all operator ServiceAccounts.
#
# Usage: ./check-rbac.sh [namespace]
#
# Runs kubectl auth can-i assertions for api-manager, agent-manager, and
# bindings-forwarder. Each assertion is "yes <verb> <resource>" (should have)
# or "no <verb> <resource>" (should NOT have). Cluster-scoped checks use
# "yes-cluster" / "no-cluster".
#
# Exit code 0 if all pass, 1 if any fail.

set -euo pipefail

NS="${1:-ngrok-operator}"
FAIL=0
PASS=0
TOTAL=0

check() {
  local sa="$1" expectation="$2" verb="$3" resource="$4"
  local full_sa="system:serviceaccount:${NS}:${sa}"
  local ns_args=() expected result

  TOTAL=$((TOTAL + 1))

  case "$expectation" in
    yes)         ns_args=(-n "$NS"); expected="yes" ;;
    no)          ns_args=(-n "$NS"); expected="no" ;;
    yes-cluster) ns_args=();         expected="yes" ;;
    no-cluster)  ns_args=();         expected="no" ;;
  esac

  # Use exit code (0=yes, non-zero=no) and discard stderr so kubectl warnings
  # like "resource 'X' is not namespace scoped" don't pollute the comparison.
  if kubectl auth can-i "$verb" "$resource" --as="$full_sa" "${ns_args[@]}" >/dev/null 2>&1; then
    result="yes"
  else
    result="no"
  fi

  if [ "$result" != "$expected" ]; then
    echo "FAIL: ${sa} ${expectation} ${verb} ${resource} (expected: ${expected}, got: ${result})"
    FAIL=$((FAIL + 1))
  else
    PASS=$((PASS + 1))
  fi
}

# ============================================================================
# api-manager (SA: ngrok-operator)
# Runs: Ingress, Domain, IPPolicy, CloudEndpoint, NgrokTrafficPolicy,
#   KubernetesOperator, BoundEndpoint, Gateway, HTTPRoute, TCPRoute, TLSRoute,
#   GatewayClass, Namespace, ReferenceGrant, Service controllers + Drain
# ============================================================================
SA=ngrok-operator

# Core API
for v in create delete get list patch update watch; do check $SA yes $v configmaps; done
for v in create patch;                               do check $SA yes $v events; done
for v in create get list patch update watch;         do check $SA yes $v secrets; done
for v in create delete get list patch update watch;  do check $SA yes $v services; done
for v in patch update;                               do check $SA yes $v services/finalizers; done
for v in get list patch update watch;                do check $SA yes $v services/status; done

# networking.k8s.io
for v in get list watch;                do check $SA yes-cluster $v ingressclasses.networking.k8s.io; done
for v in get list patch update watch;   do check $SA yes $v ingresses.networking.k8s.io; done
for v in patch update;                  do check $SA yes $v ingresses/finalizers.networking.k8s.io; done
for v in get list update watch;         do check $SA yes $v ingresses/status.networking.k8s.io; done

# gateway.networking.k8s.io — gatewayclasses (cluster-scoped)
for v in get list patch update watch;   do check $SA yes-cluster $v gatewayclasses.gateway.networking.k8s.io; done
for v in get list patch update watch;   do check $SA yes-cluster $v gatewayclasses/status.gateway.networking.k8s.io; done
for v in patch update;                  do check $SA yes-cluster $v gatewayclasses/finalizers.gateway.networking.k8s.io; done

# gateway.networking.k8s.io — namespace-scoped resources
for r in gateways httproutes tcproutes tlsroutes; do
  for v in get list patch update watch; do check $SA yes $v ${r}.gateway.networking.k8s.io; done
  for v in patch update;                do check $SA yes $v ${r}/finalizers.gateway.networking.k8s.io; done
  for v in get list update watch;       do check $SA yes $v ${r}/status.gateway.networking.k8s.io; done
done
for v in get list watch; do check $SA yes $v referencegrants.gateway.networking.k8s.io; done

# ingress.k8s.ngrok.com
for r in domains ippolicies; do
  for v in create delete get list patch update watch; do check $SA yes $v ${r}.ingress.k8s.ngrok.com; done
  for v in patch update;                              do check $SA yes $v ${r}/finalizers.ingress.k8s.ngrok.com; done
  for v in get patch update;                          do check $SA yes $v ${r}/status.ingress.k8s.ngrok.com; done
done

# bindings.k8s.ngrok.com
for v in create delete get list patch update watch; do check $SA yes $v boundendpoints.bindings.k8s.ngrok.com; done
for v in patch update;                              do check $SA yes $v boundendpoints/finalizers.bindings.k8s.ngrok.com; done
for v in get patch update;                          do check $SA yes $v boundendpoints/status.bindings.k8s.ngrok.com; done

# ngrok.k8s.ngrok.com
for r in agentendpoints cloudendpoints kubernetesoperators ngroktrafficpolicies; do
  for v in create delete get list patch update watch; do check $SA yes $v ${r}.ngrok.k8s.ngrok.com; done
  for v in patch update;                              do check $SA yes $v ${r}/finalizers.ngrok.k8s.ngrok.com; done
  for v in get patch update;                          do check $SA yes $v ${r}/status.ngrok.k8s.ngrok.com; done
done

# namespaces (cluster-scoped)
for v in get list update watch; do check $SA yes-cluster $v namespaces; done

# Negative: things api-manager must NOT do
check $SA no delete secrets
check $SA no-cluster create namespaces
check $SA no-cluster delete namespaces
check $SA no create pods
check $SA no delete pods
check $SA no-cluster create nodes

# ============================================================================
# agent-manager (SA: ngrok-operator-agent)
# Runs: AgentEndpoint controller only
# ============================================================================
SA=ngrok-operator-agent

# Core API
for v in create patch;       do check $SA yes $v events; done
for v in get list watch;     do check $SA yes $v secrets; done

# ingress.k8s.ngrok.com
for v in create delete get list patch update watch; do check $SA yes $v domains.ingress.k8s.ngrok.com; done

# ngrok.k8s.ngrok.com
for v in get list patch update watch; do check $SA yes $v agentendpoints.ngrok.k8s.ngrok.com; done
for v in patch update;                do check $SA yes $v agentendpoints/finalizers.ngrok.k8s.ngrok.com; done
for v in get patch update;            do check $SA yes $v agentendpoints/status.ngrok.k8s.ngrok.com; done
for v in get list watch;              do check $SA yes $v kubernetesoperators.ngrok.k8s.ngrok.com; done
for v in get list watch;              do check $SA yes $v ngroktrafficpolicies.ngrok.k8s.ngrok.com; done

# Negative: agent should be tightly scoped
for v in create update delete; do check $SA no $v secrets; done
check $SA no create cloudendpoints.ngrok.k8s.ngrok.com
check $SA no delete cloudendpoints.ngrok.k8s.ngrok.com
check $SA no create boundendpoints.bindings.k8s.ngrok.com
check $SA no delete boundendpoints.bindings.k8s.ngrok.com
for v in create delete update; do check $SA no $v kubernetesoperators.ngrok.k8s.ngrok.com; done
check $SA no get services
check $SA no get configmaps
check $SA no get ingresses.networking.k8s.io

# ============================================================================
# bindings-forwarder (SA: ngrok-operator-bindings-forwarder)
# Runs: Forwarder controller only. Always namespace-scoped.
# ============================================================================
SA=ngrok-operator-bindings-forwarder

# Core API
for v in create patch;   do check $SA yes $v events; done
for v in get list watch; do check $SA yes $v pods; done
for v in get list watch; do check $SA yes $v secrets; done

# bindings.k8s.ngrok.com
for v in get list patch update watch; do check $SA yes $v boundendpoints.bindings.k8s.ngrok.com; done

# ngrok.k8s.ngrok.com
for v in get list watch; do check $SA yes $v kubernetesoperators.ngrok.k8s.ngrok.com; done

# Negative: forwarder should be tightly scoped
check $SA no create secrets
check $SA no delete secrets
check $SA no create boundendpoints.bindings.k8s.ngrok.com
check $SA no delete boundendpoints.bindings.k8s.ngrok.com
check $SA no get agentendpoints.ngrok.k8s.ngrok.com
check $SA no get cloudendpoints.ngrok.k8s.ngrok.com
check $SA no get ngroktrafficpolicies.ngrok.k8s.ngrok.com
check $SA no get services
check $SA no get configmaps
check $SA no get ingresses.networking.k8s.io

# ============================================================================
# Summary
# ============================================================================
echo ""
echo "RBAC check complete: ${PASS}/${TOTAL} passed, ${FAIL} failed"
if [ "$FAIL" -ne 0 ]; then
  exit 1
fi
