#!/usr/bin/env bash

# Checks a cluster running ngrok-operator 0.24 for config that uses
# forms deprecated in 0.24. See docs/upgrading-to-0.25.md. Read-only.
#
# Requires kubectl and jq 1.6+. Exits 1 if a check in the
# "Required before 0.25" section finds anything.
#
# Set KUBECTL to use a wrapper, e.g.
#   KUBECTL="kubectl --context prod" ./check-0.24-deprecations.sh

# The single-quoted strings below are jq programs, not shell.
# shellcheck disable=SC2016

KUBECTL=${KUBECTL:-kubectl}
REQUIRED=0
BLOCKED=0

audit() {
  title="$1"
  resource="$2"
  filter="$3"
  if ! json=$($KUBECTL get "$resource" -A -o json 2>/dev/null); then
    echo "skipped: $title ($resource not found)"
    return
  fi
  out=$(printf '%s' "$json" | jq -r "$filter")
  if [ -n "$out" ]; then
    echo "NEEDS ACTION: $title"
    [ "$REQUIRED" = 1 ] && BLOCKED=1
    printf '%s\n' "$out" | sed 's/^/    /'
    echo
  else
    echo "ok: $title"
  fi
}

# Skip objects the operator generated from an Ingress,
# Gateway, or Service. The operator migrates those itself.
USER_OWNED='
def user_owned:
  (.metadata.labels // {}) as $l
  | ($l["ngrok.com/controller-name"] // "") == ""
  and ($l["k8s.ngrok.com/controller-name"] // "") == ""
  and ([.metadata.ownerReferences[]?
        | select(.controller == true)
        | select(.kind == "Service")]
       | length == 0);
'

# A string metadata that only holds a default carries no user
# data, so the v1 rewrite can drop it. Unparseable strings are
# flagged because they can't be converted to a map.
CUSTOM_METADATA='
def custom_metadata:
  type == "string"
  and (. as $s
       | (try fromjson catch "invalid")
       | IN({}, {"owned-by": "ngrok-operator"},
            {"owned-by": "kubernetes-ingress-controller"})
       or $s == ""
       | not);
'

# Annotation keys the operator reads with the legacy prefix.
LEGACY_KEY='
def legacy_key:
  IN("k8s.ngrok.com/url",
     "k8s.ngrok.com/mapping-strategy",
     "k8s.ngrok.com/traffic-policy",
     "k8s.ngrok.com/pooling-enabled",
     "k8s.ngrok.com/bindings",
     "k8s.ngrok.com/metadata",
     "k8s.ngrok.com/description",
     "k8s.ngrok.com/app-protocols")
  or startswith("k8s.ngrok.com/terminate-tls.");
'

NAME='"\(.metadata.namespace)/\(.metadata.name)"'

LEGACY_KEYS='
(.metadata.annotations // {} | keys
 | map(select(legacy_key))) as $k
| select($k | length > 0)
| "\(.metadata.namespace)/\(.metadata.name): \($k | join(", "))"
'

DOMAINS=domains.ingress.k8s.ngrok.com
IPPOLICIES=ippolicies.ingress.k8s.ngrok.com
CLOUDEPS=cloudendpoints.ngrok.k8s.ngrok.com
AGENTEPS=agentendpoints.ngrok.k8s.ngrok.com
GATEWAYS=gateways.gateway.networking.k8s.io

echo "== Required before 0.25"
REQUIRED=1

audit "Domain spec.resolves_to -> spec.resolvesTo" \
  "$DOMAINS" \
  "$USER_OWNED .items[]
   | select(user_owned and .spec.resolves_to != null)
   | $NAME"

audit "CloudEndpoint legacy traffic policy fields" \
  "$CLOUDEPS" \
  "$USER_OWNED .items[]
   | select(user_owned)
   | select(.spec.trafficPolicyName != null
            or .spec.trafficPolicy.policy != null)
   | $NAME"

echo
echo "== Required when writing v1 objects"
REQUIRED=0

for resource in "$DOMAINS" "$IPPOLICIES" "$CLOUDEPS" "$AGENTEPS"
do
  audit "$resource spec.metadata string -> map" \
    "$resource" \
    "$USER_OWNED $CUSTOM_METADATA .items[]
     | select(user_owned)
     | select(.spec.metadata | custom_metadata)
     | $NAME + \": \" + .spec.metadata"
done

audit "IPPolicy spec.rules[].metadata string -> map" \
  "$IPPOLICIES" \
  "$CUSTOM_METADATA"'
   .items[] as $i
   | $i.spec.rules[]?
   | select(.metadata | custom_metadata)
   | "\($i.metadata.namespace)/\($i.metadata.name): \(.cidr)"'

echo
echo "== Required before 1.0"

for kind in ingresses "$GATEWAYS" services
do
  audit "$kind k8s.ngrok.com/* annotations" \
    "$kind" \
    "$LEGACY_KEY .items[] | $LEGACY_KEYS"
done

audit "Gateway listener k8s.ngrok.com/terminate-tls.* options" \
  "$GATEWAYS" \
  ".items[]
   | select([.spec.listeners[]?.tls.options // {} | keys[]]
            | any(startswith(\"k8s.ngrok.com/terminate-tls.\")))
   | $NAME"

audit "Service appProtocol k8s.ngrok.com/http2" \
  services \
  ".items[]
   | select([.spec.ports[]?.appProtocol]
            | any(. == \"k8s.ngrok.com/http2\"))
   | $NAME"

exit "$BLOCKED"
