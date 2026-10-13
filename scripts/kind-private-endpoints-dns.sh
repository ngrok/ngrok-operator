#!/usr/bin/env bash
# Makes foo.ngrok.direct resolve to the operator's per-hostname Services
# (foo-ngrok-direct in $NS) by adding CoreDNS rewrite rules to the main server
# block. With PE_INCLUDE_INTERNAL=true it also rewrites foo.internal to
# foo-internal (pair with ngrok.features.privateEndpoints.includeInternal). Re-running replaces
# the rules, so running without it removes the .internal rule. Only
# single-label names are rewritten, so names like metadata.google.internal keep
# resolving upstream. Kind only: real clusters manage CoreDNS differently. Keep
# in sync with internal/privateendpoints/naming.go (ServiceName).
set -euo pipefail

NS="${KUBE_NAMESPACE:-ngrok-operator}"
svc_domain="${NS}.svc.cluster.local"

include_internal="${PE_INCLUDE_INTERNAL:-false}"

rules="    # ngrok-private-endpoints begin"
suffixes=".ngrok.direct"
if [[ "$include_internal" == "true" ]]; then
  rules="${rules}
    rewrite stop name regex ^([a-z][a-z0-9-]*)\\.internal\\.\$ {1}-internal.${svc_domain}. answer auto"
  suffixes="${suffixes} and <name>.internal"
fi
rules="${rules}
    rewrite stop name regex ^([a-z][a-z0-9-]*)\\.ngrok\\.direct\\.\$ {1}-ngrok-direct.${svc_domain}. answer auto
    # ngrok-private-endpoints end"

corefile="$(kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}')"
corefile="$(printf '%s\n' "$corefile" | RULES="$rules" awk '
  /^[[:space:]]*# ngrok-private-endpoints begin/ {skip=1}
  !skip {print}
  /^[[:space:]]*# ngrok-private-endpoints end/ {skip=0; next}
  !skip && !done && /^\.:53[[:space:]]*\{/ {print ENVIRON["RULES"]; done=1}
')"

kubectl -n kube-system create configmap coredns --from-literal=Corefile="$corefile" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n kube-system rollout restart deployment coredns
kubectl -n kube-system rollout status deployment coredns --timeout=120s
echo "CoreDNS now rewrites <name>${suffixes} to Services in ${NS}"
