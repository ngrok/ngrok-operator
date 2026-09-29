{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "ngrok-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "ngrok-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "ngrok-operator.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create a default name for the credentials secret name using the helm release
*/}}
{{- define "ngrok-operator.credentialsSecretName" -}}
{{- if .Values.ngrok.credentials.secret.name -}}
{{- .Values.ngrok.credentials.secret.name -}}
{{- else -}}
{{- printf "%s-credentials" (include "ngrok-operator.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
The secret key a component reads its access token from.

Each component reads its own key, so each can be given a token scoped to
only the permissions it needs. The key holds the component's own token
when one is set, and the shared credentials.accessToken otherwise.

Note that both deployments annotate the same checksum over the whole rendered
secret, so changing any token restarts every component that holds one.

Usage: include "ngrok-operator.accessTokenSecretKey" "agent"
*/}}
{{- define "ngrok-operator.accessTokenSecretKey" -}}
{{- if eq . "agent" -}}
AGENT_ACCESS_TOKEN
{{- else if eq . "apiManager" -}}
API_MANAGER_ACCESS_TOKEN
{{- else -}}
{{- fail (printf "unknown component %q for accessTokenSecretKey" .) -}}
{{- end -}}
{{- end -}}

{{/*
The access token a component actually uses: its own override, else the shared one.

Usage: include "ngrok-operator.accessTokenFor" (dict "root" $ "component" "agent")
*/}}
{{- define "ngrok-operator.accessTokenFor" -}}
{{- $credentials := .root.Values.ngrok.credentials -}}
{{- /* credentials.<component>.accessToken, or empty when the component has no section. */ -}}
{{- $override := (get $credentials .component | default dict).accessToken -}}
{{- $override | default $credentials.accessToken -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "ngrok-operator.labels" -}}
helm.sh/chart: {{ include "ngrok-operator.chart" . }}
{{ include "ngrok-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/part-of: {{ template "ngrok-operator.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Values.commonLabels}}
{{ toYaml .Values.commonLabels }}
{{- end }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "ngrok-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ngrok-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the controller service account to use
*/}}
{{- define "ngrok-operator.serviceAccountName" -}}
{{- if .Values.components.apiManager.serviceAccount.create -}}
    {{ default (include "ngrok-operator.fullname" .) .Values.components.apiManager.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.components.apiManager.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the agent service account to use
*/}}
{{- define "ngrok-operator.agent.serviceAccountName" -}}
{{- if .Values.components.agent.serviceAccount.create -}}
    {{ default (printf "%s-agent" (include "ngrok-operator.fullname" .)) .Values.components.agent.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.components.agent.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the bindings-forwarder service account to use
*/}}
{{- define "ngrok-operator.bindings.forwarder.serviceAccountName" -}}
{{- if .Values.components.bindingsForwarder.serviceAccount.create -}}
    {{ default (printf "%s-bindings-forwarder" (include "ngrok-operator.fullname" .)) .Values.components.bindingsForwarder.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.components.bindingsForwarder.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Return the ngrok operator image name
*/}}
{{- define "ngrok-operator.image" -}}
{{- $registryName := .Values.image.registry -}}
{{- $repositoryName := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion | toString -}}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- end -}}

{{/*
Whether RBAC should use namespace-scoped Roles instead of ClusterRoles.
True when features.ingress.watchNamespace is set.
*/}}
{{- define "ngrok-operator.isNamespaced" -}}
{{- if .Values.ngrok.features.ingress.watchNamespace -}}
true
{{- end -}}
{{- end -}}

{{/*
The namespace to watch.
*/}}
{{- define "ngrok-operator.watchNamespace" -}}
{{- .Values.ngrok.features.ingress.watchNamespace -}}
{{- end -}}

{{/*
A component's pod settings: `components.common` with the component's own keys
on top. A key the component sets wins even when empty, so it can clear a common
value; a non-empty map merges with the common one, the component winning per
key. A key the component leaves unset inherits the common value.

Takes the root context for .Values and the component's key under
`components`, since an include passes only one argument.

Usage: fromYaml (include "ngrok-operator.componentValues" (dict "context" $ "component" "agent"))
*/}}
{{- define "ngrok-operator.componentValues" -}}
{{- /* deepCopy both, so the merge leaves .Values untouched. log is operator config, not a pod setting. */ -}}
{{- $values := deepCopy .context.Values.components.common -}}
{{- $component := deepCopy (omit (index .context.Values.components .component) "log") -}}
{{- range $key, $val := $component -}}
{{- $common := get $values $key -}}
{{- if and (kindIs "map" $val) $val (kindIs "map" $common) -}}
{{- $_ := set $values $key (mergeOverwrite $common $val) -}}
{{- else -}}
{{- $_ := set $values $key $val -}}
{{- end -}}
{{- end -}}
{{- $values | toYaml -}}
{{- end -}}

{{/*
The operator configuration for one component: the `ngrok` values, with
`components.<component>.log` overriding `ngrok.log` per key.

Empty values are dropped first, so an unset value falls back to the shared
value and then to the operator's built-in default.
*/}}
{{- define "ngrok-operator.componentConfig" -}}
{{- $component := index .context.Values.components .component -}}
{{- range $key := list "ngrok" "features" -}}
{{- if hasKey $component $key -}}
{{- fail (printf "components.%s.%s is not supported: ngrok settings apply to every component. Set them under ngrok instead." $.component $key) -}}
{{- end -}}
{{- end -}}
{{- /* Values only the chart reads: credentials go in a Secret, ingressClass renders the IngressClass, cleanup.enabled and timeout run the hook. */ -}}
{{- $config := omit (deepCopy .context.Values.ngrok) "credentials" -}}
{{- $_ := unset $config.features.ingress "ingressClass" -}}
{{- $_ = set $config.features "cleanup" (omit $config.features.cleanup "enabled" "timeout") -}}
{{- $log := deepCopy ($component.log | default dict) -}}
{{- /* Drop empties before merging, so an empty component value does not blank the shared one. */ -}}
{{- include "ngrok-operator.dropEmpty" $config -}}
{{- include "ngrok-operator.dropEmpty" $log -}}
{{- $config = mergeOverwrite $config (dict "log" $log) -}}
{{- /* And after, to remove sections left empty. */ -}}
{{- include "ngrok-operator.dropEmpty" $config -}}
{{- $config | toYaml -}}
{{- end -}}

{{/*
The component's operator configuration as container env entries: for each
setting in files/operator-env.yaml (paths under `ngrok`) that the component's
configuration holds, its variable. Strings are written as-is, booleans and numbers as text, lists
and maps as JSON.

Usage: include "ngrok-operator.componentEnv" (dict "context" $ "component" "agent")
*/}}
{{- define "ngrok-operator.componentEnv" -}}
{{- $config := fromYaml (include "ngrok-operator.componentConfig" .) -}}
{{- range $path, $name := .context.Files.Get "files/operator-env.yaml" | fromYaml }}
{{- /* Walk the dotted path ("features.gateway.enabled") into $config. $found turns false at the first missing key: the setting is unset. */ -}}
{{- $value := $config -}}
{{- $found := true -}}
{{- range splitList "." $path -}}
{{- if and $found (kindIs "map" $value) (hasKey $value .) -}}
{{- $value = get $value . -}}
{{- else -}}
{{- $found = false -}}
{{- end -}}
{{- end }}
{{- if $found }}
{{- /* ternary picks JSON for lists and maps, plain text for everything else. */}}
- name: {{ $name }}
  value: {{ ternary (toJson $value) (toString $value) (or (kindIs "map" $value) (kindIs "slice" $value)) | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{/*
Removes empty strings, lists and maps from a map in place, recursing into
nested maps. Booleans are kept, since false is a real value.
*/}}
{{- define "ngrok-operator.dropEmpty" -}}
{{- $m := . -}}
{{- range $key, $val := $m -}}
{{- /* Recurse first, so a map emptied by its children is dropped too. */ -}}
{{- if kindIs "map" $val -}}
{{- include "ngrok-operator.dropEmpty" $val -}}
{{- end -}}
{{- if and (not (kindIs "bool" $val)) (empty $val) -}}
{{- $_ := unset $m $key -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
api-manager rules for cluster-scoped Kubernetes resources.
These resources have no namespace and always require a ClusterRole, regardless of watchNamespace.
*/}}
{{- define "ngrok-operator.api-manager.clusterScopedRules" -}}
- apiGroups:
  - ""
  resources:
  - namespaces
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - networking.k8s.io
  resources:
  - ingressclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gatewayclasses
  verbs:
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gatewayclasses/status
  verbs:
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gatewayclasses/finalizers
  verbs:
  - patch
  - update
{{- end -}}
