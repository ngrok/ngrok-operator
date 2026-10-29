#!/usr/bin/env bash
# LEGACY-domain-kind: delete this script once ingress.k8s.ngrok.com/v1alpha1
# Domain is no longer served.
#
# Migrates ingress.k8s.ngrok.com/v1alpha1 Domains that have no ngrok.com/v1
# twin (a v1 Domain with the same name and namespace) to ngrok.com/v1.
#
# The operator already does this for every Domain it uses, so what's left is
# usually Domains you created yourself and no Ingress, Gateway or endpoint
# refers to. Each v1 Domain is a copy of the legacy one: same name, namespace,
# labels and spec. The operator hands it the legacy Domain's reservation, so
# the reserved domain, its ID and its CNAME target don't change.
#
# Usage:
#   scripts/migrate-domains-to-ngrok-v1.sh                 # dry run: print what would be created
#   scripts/migrate-domains-to-ngrok-v1.sh --apply         # create the v1 Domains
#   scripts/migrate-domains-to-ngrok-v1.sh --delete-legacy # delete legacy Domains whose v1 twin
#                                                          # is Ready and holds the same reservation
#
# Options:
#   -n, --namespace NS   only this namespace (default: all namespaces)
#
# If you manage Domains with GitOps, change apiVersion in the source manifests
# instead, so your tool doesn't recreate or prune them.
#
# Requires kubectl and jq.
set -euo pipefail

mode=dry-run
namespace_args=(-A)

while [ $# -gt 0 ]; do
  case "$1" in
    --apply) mode=apply ;;
    --delete-legacy) mode=delete-legacy ;;
    -n | --namespace)
      namespace_args=(-n "$2")
      shift
      ;;
    -h | --help)
      sed -n '/^# Migrates/,/^set -euo/p' "$0" | sed -e '/^set -euo/d' -e 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
  shift
done

for tool in kubectl jq; do
  command -v "$tool" >/dev/null || {
    echo "$tool is required" >&2
    exit 1
  }
done

# Both CRDs use the plural "domains", so always use the fully qualified names.
legacy_json=$(kubectl get domains.ingress.k8s.ngrok.com "${namespace_args[@]}" -o json)
v1_json=$(kubectl get domains.ngrok.com "${namespace_args[@]}" -o json)

# to_v1 converts a legacy Domain to its v1 copy. spec.metadata must end up as
# a JSON object of strings: the legacy string form holding such an object is
# unwrapped; anything else can't be represented and is reported instead.
# shellcheck disable=SC2016
to_v1='
  def metadata_map:
    if . == null or . == "" then null
    elif type == "object" then .
    elif type == "string" then (try fromjson catch "invalid")
    else "invalid" end
    | if . == null then null
      elif type == "object" and ([.[] | type == "string"] | all) then .
      else "invalid" end;

  {
    apiVersion: "ngrok.com/v1",
    kind: "Domain",
    metadata: ({name: .metadata.name, namespace: .metadata.namespace}
      + (if .metadata.labels then {labels: .metadata.labels} else {} end)),
    spec: (.spec
      | (.metadata |= metadata_map)
      | with_entries(select(.value != null)))
  }
'

case "$mode" in
  dry-run | apply)
    pending=$(jq -c --argjson v1 "$v1_json" '
      ($v1.items | map(.metadata.namespace + "/" + .metadata.name)) as $twins
      | .items[]
      | select((.metadata.namespace + "/" + .metadata.name) as $k | $twins | index($k) | not)
      | select(.metadata.deletionTimestamp == null)
    ' <<<"$legacy_json")

    if [ -z "$pending" ]; then
      echo "Every ingress.k8s.ngrok.com/v1alpha1 Domain already has an ngrok.com/v1 twin."
      exit 0
    fi

    blocked=0
    while IFS= read -r legacy; do
      v1=$(jq "$to_v1" <<<"$legacy")
      ref=$(jq -r '.metadata.namespace + "/" + .metadata.name' <<<"$v1")
      if [ "$(jq -r '.spec.metadata' <<<"$v1")" = "invalid" ]; then
        echo "SKIP $ref: spec.metadata $(jq -c '.spec.metadata' <<<"$legacy") is not a JSON object of strings." >&2
        echo "     Change it to a map of strings, then re-run this script." >&2
        blocked=$((blocked + 1))
        continue
      fi
      if [ "$mode" = apply ]; then
        kubectl apply -f - <<<"$v1"
      else
        echo "---"
        jq . <<<"$v1"
      fi
    done <<<"$pending"

    if [ "$mode" = dry-run ]; then
      echo
      echo "Dry run. Re-run with --apply to create these Domains." >&2
    else
      echo
      echo "Wait for the new Domains to be Ready, then run with --delete-legacy." >&2
    fi
    [ "$blocked" -eq 0 ] || exit 1
    ;;

  delete-legacy)
    # A legacy Domain is safe to delete once its v1 twin is Ready and holds the
    # same reservation: the operator keeps the reservation for the twin.
    # A legacy Domain covered by a wildcard holds no reservation of its own
    # (empty status.id); its twin is safe once it is Ready and covered too.
    ready=$(jq -r --argjson legacy "$legacy_json" '
      ([$legacy.items[] | {key: (.metadata.namespace + "/" + .metadata.name), value: (.status.id // "")}]
        | from_entries) as $ids
      | .items[]
      | select(.metadata.deletionTimestamp == null)
      | (.metadata.namespace + "/" + .metadata.name) as $k
      | select($ids | has($k))
      | (.status.id // "") as $id
      | select(($id != "" and $id == $ids[$k])
          or ($id == "" and $ids[$k] == "" and .status.coveredByWildcardDomain != null))
      | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
      | $k
    ' <<<"$v1_json")

    if [ -z "$ready" ]; then
      echo "No legacy Domain has a Ready ngrok.com/v1 twin holding its reservation yet."
      exit 0
    fi
    while IFS= read -r ref; do
      kubectl delete domains.ingress.k8s.ngrok.com "${ref#*/}" -n "${ref%%/*}"
    done <<<"$ready"
    ;;
esac
