#!/usr/bin/env bash
# Points CoreDNS at the private endpoint forwarder for internal. and
# ngrok.direct. Kind only: real clusters manage CoreDNS differently.
set -euo pipefail

NS="${KUBE_NAMESPACE:-ngrok-operator}"
SVC="${PRIVATE_DNS_SERVICE:-ngrok-operator-private-dns}"

ip="$(kubectl -n "$NS" get svc "$SVC" -o jsonpath='{.spec.clusterIP}')"
if [[ -z "$ip" ]]; then
  echo "Service $NS/$SVC has no ClusterIP; is privateEndpoints.enabled=true deployed?" >&2
  exit 1
fi

corefile="$(kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}')"
corefile="$(printf '%s\n' "$corefile" | awk '
  /^# ngrok-private-endpoints begin/ {skip=1}
  !skip {print}
  /^# ngrok-private-endpoints end/ {skip=0}
')"
corefile+="
# ngrok-private-endpoints begin
internal:53 {
    forward . ${ip}
}
ngrok.direct:53 {
    forward . ${ip}
}
# ngrok-private-endpoints end
"

kubectl -n kube-system create configmap coredns --from-literal=Corefile="$corefile" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n kube-system rollout restart deployment coredns
kubectl -n kube-system rollout status deployment coredns --timeout=120s
echo "CoreDNS now forwards internal. and ngrok.direct. to $ip"
