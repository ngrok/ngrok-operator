#!/usr/bin/env bash
# Print the state of every ngrok object in both API groups, so you can diff
# before/after upgrade, rollback, and each migration step.
#
#   ./snapshot.sh            objects: kind, name, ID/URL, Ready reason
#   ./snapshot.sh legacy     legacy k8s.ngrok.com/* labels, annotations,
#                            finalizers still on objects
set -euo pipefail

RESOURCES=(
  kubernetesoperators.ngrok.k8s.ngrok.com
  kubernetesoperators.ngrok.com
  ngroktrafficpolicies.ngrok.k8s.ngrok.com
  trafficpolicies.ngrok.com
  ippolicies.ingress.k8s.ngrok.com
  ippolicies.ngrok.com
  domains.ingress.k8s.ngrok.com
  domains.ngrok.com
  cloudendpoints.ngrok.k8s.ngrok.com
  cloudendpoints.ngrok.com
  agentendpoints.ngrok.k8s.ngrok.com
  agentendpoints.ngrok.com
)

present() { kubectl get crd "$1" >/dev/null 2>&1; }

objects() {
  for r in "${RESOURCES[@]}"; do
    if ! present "$r"; then
      echo "## $r: CRD not installed"
      continue
    fi
    echo "## $r"
    kubectl get "$r" -A --sort-by=.metadata.name \
      -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,ID:.status.id,URL:.status.assignedURL,DOMAIN:.status.domain,READY:.status.conditions[?(@.type=="Ready")].status,REASON:.status.conditions[?(@.type=="Ready")].reason' \
      2>/dev/null || true
  done
  echo "## services (computed-url)"
  kubectl get svc -n upgrade-test -o json |
    jq -r '.items[] | select(.spec.type == "LoadBalancer")
      | "\(.metadata.name)  new=\(.metadata.annotations["ngrok.com/computed-url"] // "-")  legacy=\(.metadata.annotations["k8s.ngrok.com/computed-url"] // "-")"'
}

legacy() {
  for r in "${RESOURCES[@]}" services; do
    [ "$r" = services ] || present "$r" || continue
    kubectl get "$r" -A -o json |
      jq -r --arg r "$r" '
        .items[]
        | "\($r) \(.metadata.namespace // "-")/\(.metadata.name)" as $obj
        | ( (.metadata.labels // {} | keys[] | select(startswith("k8s.ngrok.com/") or startswith("bindings.k8s.ngrok.com/")) | "label " + .),
            (.metadata.annotations // {} | keys[] | select(. == "k8s.ngrok.com/computed-url" or startswith("bindings.k8s.ngrok.com/")) | "annotation " + .),
            (.metadata.finalizers // [] | .[] | select(startswith("k8s.ngrok.com/")) | "finalizer " + .) )
        | "\($obj)  \(.)"
      ' 2>/dev/null | sort || true
  done
}

case "${1:-objects}" in
  objects) objects ;;
  legacy) legacy ;;
  *) echo "usage: $0 [objects|legacy]" >&2; exit 2 ;;
esac
