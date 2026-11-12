#!/usr/bin/env bash
#
# Rewrites a values file written for the ngrok-operator Helm chart 0.24 into
# the 0.25 layout (ngrok.* / components.*).
#
# Usage:
#   migrate-helm-values-to-0.25.sh <0.24-values.yaml> > values-0.25.yaml
#
# Get the values you set on a running release (without --all, so the old
# chart's defaults aren't copied in) with:
#   helm get values <release> -n <namespace> -o yaml > 0.24-values.yaml
#
# Notes about anything that needs a human (credentials, keys it didn't
# recognize) are printed to stderr and repeated as comments at the top of the
# output. Comments in the input are not preserved.
#
# The mapping keeps what each pod rendered in 0.24: settings that only the
# api-manager read move to components.apiManager, settings that every pod read
# move to components.common. Review the result with `helm diff upgrade` or by
# diffing `helm template` output before upgrading.
#
# Only helm is required: the rewrite runs as a Helm template.

set -euo pipefail

usage() {
  sed -n '3,22p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

[ $# -eq 1 ] || usage 1
case "$1" in -h|--help) usage 0 ;; esac
input=$1
[ -f "$input" ] || { echo "error: $input: no such file" >&2; exit 1; }
command -v helm >/dev/null || { echo "error: helm not found in PATH" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/chart/templates"

cat > "$tmp/chart/Chart.yaml" <<'EOF'
apiVersion: v2
name: migrate-values
version: 0.0.0
EOF

cat > "$tmp/chart/templates/_helpers.tpl" <<'EOF'
{{- /* Set .value at .path (a list of keys) in .root, creating maps on the way.
       A map landing on an existing map is merged into it, .value winning. */ -}}
{{- define "setPath" -}}
{{- $cur := .root -}}
{{- range initial .path -}}
{{- if not (kindIs "map" (get $cur .)) }}{{ $_ := set $cur . (dict) }}{{ end -}}
{{- $cur = get $cur . -}}
{{- end -}}
{{- $last := last .path -}}
{{- if and (hasKey $cur $last) (kindIs "map" (get $cur $last)) (kindIs "map" .value) -}}
{{- $_ := set $cur $last (mergeOverwrite (get $cur $last) .value) -}}
{{- else -}}
{{- $_ := set $cur $last .value -}}
{{- end -}}
{{- end -}}

{{- /* Move .in[.from] to .path in .out, if .from is set. */ -}}
{{- define "move" -}}
{{- if hasKey .in .from -}}
{{- include "setPath" (dict "root" .out "path" .path "value" (get .in .from)) -}}
{{- $_ := unset .in .from -}}
{{- end -}}
{{- end -}}
EOF

cat > "$tmp/chart/templates/values.yaml" <<'EOF'
# migrate-values
{{- $in := deepCopy .Values -}}
{{- $out := dict -}}
{{- $notes := list -}}

{{- /* Already in the 0.25 layout (a partly migrated file): start from it. */ -}}
{{- range list "ngrok" "components" -}}
{{- if hasKey $in . }}{{ $_ := set $out . (get $in .) }}{{ $_ := unset $in . }}{{ end -}}
{{- end -}}

{{- /* Unchanged top-level keys, plus subchart and Helm-global keys. */ -}}
{{- range list "nameOverride" "fullnameOverride" "commonLabels" "commonAnnotations" "image" "installCRDs" "crdAccessRoles" "ngrok-crds" "common" "global" "tags" -}}
{{- if hasKey $in . }}{{ $_ := set $out . (get $in .) }}{{ $_ := unset $in . }}{{ end -}}
{{- end -}}

{{- /* Operator settings: ngrok.* */ -}}
{{- range list "description" "region" "rootCAs" "serverAddr" "apiURL" "clusterDomain" "log" -}}
{{- include "move" (dict "in" $in "out" $out "from" . "path" (list "ngrok" .)) -}}
{{- end -}}

{{- /* 0.24 used metaData when set, else ngrokMetadata. */ -}}
{{- if and $in.metaData $in.ngrokMetadata -}}
{{- $notes = append $notes "Both metaData and ngrokMetadata were set. 0.24 used metaData only, so ngrok.metadata is metaData; ngrokMetadata was dropped." -}}
{{- $_ := unset $in "ngrokMetadata" -}}
{{- end -}}
{{- include "move" (dict "in" $in "out" $out "from" "ngrokMetadata" "path" (list "ngrok" "metadata")) -}}
{{- include "move" (dict "in" $in "out" $out "from" "metaData" "path" (list "ngrok" "metadata")) -}}

{{- with $in.credentials -}}
{{- if or .apiKey .authtoken -}}
{{- $notes = append $notes "credentials.apiKey and credentials.authtoken were removed and have no 0.25 equivalent. Create an access token at https://dashboard.ngrok.com/access-tokens and set ngrok.credentials.accessToken (in this file or with --set)." -}}
{{- end -}}
{{- if and .secret .secret.name -}}
{{- $notes = append $notes (printf "ngrok.credentials.secret.name is %q. 0.25 reads the keys AGENT_ACCESS_TOKEN and API_MANAGER_ACCESS_TOKEN instead of AUTHTOKEN and API_KEY. Create a new Secret with those keys under a new name and point this value at it, so the old one keeps working for a rollback." .secret.name) -}}
{{- end -}}
{{- $_ := unset . "apiKey" -}}
{{- $_ := unset . "authtoken" -}}
{{- end -}}
{{- if $in.credentials }}{{ include "move" (dict "in" $in "out" $out "from" "credentials" "path" (list "ngrok" "credentials")) }}{{ end -}}
{{- $_ := unset $in "credentials" -}}

{{- /* Features: ngrok.features.* */ -}}
{{- include "move" (dict "in" $in "out" $out "from" "ingress" "path" (list "ngrok" "features" "ingress")) -}}
{{- include "move" (dict "in" $in "out" $out "from" "ingressClass" "path" (list "ngrok" "features" "ingress" "ingressClass")) -}}
{{- /* 0.24 preferred the top-level controllerName and watchNamespace when non-empty. */ -}}
{{- range list "controllerName" "watchNamespace" -}}
{{- if get $in . -}}
{{- include "move" (dict "in" $in "out" $out "from" . "path" (list "ngrok" "features" "ingress" .)) -}}
{{- end -}}
{{- $_ := unset $in . -}}
{{- end -}}
{{- include "move" (dict "in" $in "out" $out "from" "gateway" "path" (list "ngrok" "features" "gateway")) -}}
{{- with $in.bindings -}}
{{- include "move" (dict "in" . "out" $out "from" "forwarder" "path" (list "components" "bindingsForwarder")) -}}
{{- end -}}
{{- include "move" (dict "in" $in "out" $out "from" "bindings" "path" (list "ngrok" "features" "bindings")) -}}
{{- include "move" (dict "in" $in "out" $out "from" "defaultDomainReclaimPolicy" "path" (list "ngrok" "features" "domains" "defaultReclaimPolicy")) -}}
{{- include "move" (dict "in" $in "out" $out "from" "drainPolicy" "path" (list "ngrok" "features" "cleanup" "drainPolicy")) -}}
{{- include "move" (dict "in" $in "out" $out "from" "oneClickDemoMode" "path" (list "ngrok" "features" "oneClickDemoMode" "enabled")) -}}
{{- with $in.cleanupHook -}}
{{- include "move" (dict "in" . "out" $out "from" "enabled" "path" (list "ngrok" "features" "cleanup" "enabled")) -}}
{{- include "move" (dict "in" . "out" $out "from" "timeout" "path" (list "ngrok" "features" "cleanup" "timeout")) -}}
{{- end -}}
{{- include "move" (dict "in" $in "out" $out "from" "cleanupHook" "path" (list "components" "cleanupHook")) -}}

{{- /* Pod settings every 0.24 pod read: components.common. */ -}}
{{- /* In 0.24 agent.podAnnotations replaced podAnnotations on the agent; in 0.25
       component maps merge with common, so keep the top-level ones off it. */ -}}
{{- /* An empty agent.podAnnotations fell back to podAnnotations in 0.24, but in
       0.25 a component's empty value clears the common one, so drop it. */ -}}
{{- if and $in.agent (hasKey $in.agent "podAnnotations") (not $in.agent.podAnnotations) -}}
{{- $_ := unset $in.agent "podAnnotations" -}}
{{- end -}}
{{- if and $in.podAnnotations $in.agent $in.agent.podAnnotations -}}
{{- $notes = append $notes "podAnnotations and agent.podAnnotations were both set. In 0.24 the agent used only its own, so podAnnotations moved to components.apiManager and components.bindingsForwarder instead of components.common, which would add them to the agent." -}}
{{- include "setPath" (dict "root" $out "path" (list "components" "apiManager" "podAnnotations") "value" (deepCopy $in.podAnnotations)) -}}
{{- include "move" (dict "in" $in "out" $out "from" "podAnnotations" "path" (list "components" "bindingsForwarder" "podAnnotations")) -}}
{{- end -}}
{{- range list "podAnnotations" "podLabels" "affinity" "podAffinityPreset" "podAntiAffinityPreset" "nodeAffinityPreset" "extraEnv" "extraVolumes" "extraVolumeMounts" "lifecycle" -}}
{{- include "move" (dict "in" $in "out" $out "from" . "path" (list "components" "common" .)) -}}
{{- end -}}

{{- /* Pod settings only the 0.24 api-manager read: components.apiManager. */ -}}
{{- $apiOnly := list -}}
{{- range list "nodeSelector" "tolerations" "topologySpreadConstraints" "priorityClassName" "terminationGracePeriodSeconds" -}}
{{- if get $in . }}{{ $apiOnly = append $apiOnly . }}{{ end -}}
{{- end -}}
{{- if $apiOnly -}}
{{- $notes = append $notes (printf "%s only applied to the api-manager in 0.24, so moved to components.apiManager. Move to components.common to apply to every pod." (join ", " $apiOnly)) -}}
{{- end -}}
{{- range list "nodeSelector" "tolerations" "topologySpreadConstraints" "priorityClassName" "terminationGracePeriodSeconds" "replicaCount" "resources" "podDisruptionBudget" "serviceAccount" -}}
{{- include "move" (dict "in" $in "out" $out "from" . "path" (list "components" "apiManager" .)) -}}
{{- end -}}

{{- include "move" (dict "in" $in "out" $out "from" "agent" "path" (list "components" "agent")) -}}

{{- /* Anything left is not a 0.24 chart value. */ -}}
{{- range $k, $v := $in -}}
{{- $notes = append $notes (printf "%s is not a value the 0.24 chart knew; copied unchanged. Check whether it is still needed." $k) -}}
{{- $_ := set $out $k $v -}}
{{- end -}}

{{- range $notes }}
# NOTE: {{ . }}
{{- end }}
{{- $top := omit $out "ngrok" "components" }}
{{- if $top }}
{{ toYaml $top }}
{{- end }}
{{- with pick $out "ngrok" }}
{{ toYaml . }}
{{- end }}
{{- with pick $out "components" }}
{{ toYaml . }}
{{- end }}
EOF

rendered=$(helm template migrate "$tmp/chart" -f "$input" --show-only templates/values.yaml)
# Drop helm's "---" and "# Source:" header lines and the marker that keeps
# helm from skipping an empty result.
rendered=$(printf '%s\n' "$rendered" | sed -e '/^---$/d' -e '/^# Source: /d' -e '/^# migrate-values$/d' -e '/^$/d')

printf '%s\n' "$rendered" | grep '^# NOTE: ' | sed 's/^# NOTE: /note: /' >&2 || true

echo "# Migrated from $(basename "$input") to the ngrok-operator 0.25 values layout."
echo "# Review with 'helm diff upgrade' (or diff 'helm template' output) before upgrading."
printf '%s\n' "$rendered"
