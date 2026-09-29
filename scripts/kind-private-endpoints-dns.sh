#!/usr/bin/env bash
# Makes foo.internal and foo.ngrok.direct resolve to the operator's per-hostname
# Services (foo-internal / foo-ngrok-direct in $NS) by adding CoreDNS rewrite
# rules to the main server block. Only single-label names are rewritten, so
# names like metadata.google.internal keep resolving upstream. Kind only: real
# clusters manage CoreDNS differently. Keep in sync with
# internal/privateendpoints/naming.go (ServiceName).
set -euo pipefail

NS="${KUBE_NAMESPACE:-ngrok-operator}"
svc_domain="${NS}.svc.cluster.local"

rules="    # ngrok-private-endpoints begin
    rewrite stop name regex ^([a-z][a-z0-9-]*)\\.internal\\.\$ {1}-internal.${svc_domain}. answer auto
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
echo "CoreDNS now rewrites <name>.internal and <name>.ngrok.direct to Services in ${NS}"
