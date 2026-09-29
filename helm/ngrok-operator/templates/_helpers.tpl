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
{{- if .Values.credentials.secret.name -}}
{{- .Values.credentials.secret.name -}}
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
{{- $credentials := .root.Values.credentials -}}
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
{{- if .Values.apiManager.serviceAccount.create -}}
    {{ default (include "ngrok-operator.fullname" .) .Values.apiManager.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.apiManager.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the agent service account to use
*/}}
{{- define "ngrok-operator.agent.serviceAccountName" -}}
{{- if .Values.agent.serviceAccount.create -}}
    {{ default (printf "%s-agent" (include "ngrok-operator.fullname" .)) .Values.agent.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.agent.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the bindings-forwarder service account to use
*/}}
{{- define "ngrok-operator.bindings.forwarder.serviceAccountName" -}}
{{- if .Values.bindingsForwarder.serviceAccount.create -}}
    {{ default (printf "%s-bindings-forwarder" (include "ngrok-operator.fullname" .)) .Values.bindingsForwarder.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.bindingsForwarder.serviceAccount.name }}
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
{{- if .Values.features.ingress.watchNamespace -}}
true
{{- end -}}
{{- end -}}

{{/*
The namespace to watch.
*/}}
{{- define "ngrok-operator.watchNamespace" -}}
{{- .Values.features.ingress.watchNamespace -}}
{{- end -}}

{{/*
Fail the render when values from the pre-0.25 layout are present, so they are
never silently ignored.
*/}}
{{- define "ngrok-operator.validateValues" -}}
{{- $moved := dict
  "description" "ngrok.description"
  "region" "ngrok.region"
  "rootCAs" "ngrok.rootCAs"
  "serverAddr" "ngrok.serverAddr"
  "apiURL" "ngrok.apiURL"
  "ngrokMetadata" "ngrok.metadata"
  "metaData" "ngrok.metadata"
  "clusterDomain" "ngrok.clusterDomain"
  "ingress" "features.ingress"
  "ingressClass" "features.ingress.ingressClass"
  "controllerName" "features.ingress.controllerName"
  "watchNamespace" "features.ingress.watchNamespace"
  "gateway" "features.gateway"
  "bindings" "features.bindings (forwarder pod settings: bindingsForwarder)"
  "defaultDomainReclaimPolicy" "features.defaultDomainReclaimPolicy"
  "drainPolicy" "features.drainPolicy"
  "oneClickDemoMode" "features.oneClickDemoMode"
  "podAnnotations" "defaults.podAnnotations"
  "podLabels" "defaults.podLabels"
  "nodeSelector" "defaults.nodeSelector"
  "tolerations" "defaults.tolerations"
  "affinity" "defaults.affinity"
  "podAffinityPreset" "defaults.podAffinityPreset"
  "podAntiAffinityPreset" "defaults.podAntiAffinityPreset"
  "nodeAffinityPreset" "defaults.nodeAffinityPreset"
  "topologySpreadConstraints" "defaults.topologySpreadConstraints"
  "priorityClassName" "defaults.priorityClassName"
  "extraEnv" "defaults.extraEnv"
  "replicaCount" "apiManager.replicaCount"
  "resources" "apiManager.resources"
  "lifecycle" "apiManager.lifecycle"
  "terminationGracePeriodSeconds" "apiManager.terminationGracePeriodSeconds"
  "extraVolumes" "apiManager.extraVolumes"
  "extraVolumeMounts" "apiManager.extraVolumeMounts"
  "podDisruptionBudget" "apiManager.podDisruptionBudget"
  "serviceAccount" "apiManager.serviceAccount"
-}}
{{- $found := list -}}
{{- range $old, $new := $moved -}}
{{- if hasKey $.Values $old }}{{ $found = append $found (printf "  %s -> %s" $old $new) }}{{ end -}}
{{- end -}}
{{- if $found -}}
{{- fail (printf "\n\nThese Helm values moved in 0.25 and are no longer read at their old location:\n\n%s\n\nSee the 0.25 upgrade guide for the full mapping." (join "\n" (sortAlpha $found))) -}}
{{- end -}}
{{- end -}}

{{/*
A component's pod settings: `defaults` with the component's own keys merged on
top. Maps merge with the component winning per key; lists replace.

Usage: fromYaml (include "ngrok-operator.componentValues" (dict "context" $ "component" "agent"))
*/}}
{{- define "ngrok-operator.componentValues" -}}
{{- $component := omit (index .context.Values .component) "config" -}}
{{- mergeOverwrite (deepCopy .context.Values.defaults) (deepCopy $component) | toYaml -}}
{{- end -}}

{{/*
The operator configuration for one component: the shared `ngrok`, `log` and
`features` values, plus the component's own `config` section.

`<component>.config.log` overrides the shared `log` for that component, per
key. `ngrok` and `features` are set once for every component, so `log` is the
only key `<component>.config` takes.

Empty values are dropped first, so an unset value falls back to the shared
value and then to the operator's built-in default.
*/}}
{{- define "ngrok-operator.componentConfig" -}}
{{- $component := deepCopy ((index .context.Values .component).config | default dict) -}}
{{- range $key := keys (omit $component "log") -}}
{{- fail (printf "%s.config.%s is not supported: only log can be set per component. Set %s under the top-level ngrok or features instead." $.component $key $key) -}}
{{- end -}}
{{- $features := deepCopy .context.Values.features -}}
{{- $_ := unset $features.ingress "ingressClass" -}}
{{- $config := dict "ngrok" (deepCopy .context.Values.ngrok) "log" (deepCopy .context.Values.log) "features" $features -}}
{{- $log := $component.log | default dict -}}
{{- include "ngrok-operator.dropEmpty" $config -}}
{{- include "ngrok-operator.dropEmpty" $log -}}
{{- $config = mergeOverwrite $config (dict "log" $log) -}}
{{- include "ngrok-operator.dropEmpty" $config -}}
{{- $config | toYaml -}}
{{- end -}}

{{/*
The component's operator configuration as container env entries: for each
setting in files/operator-env.yaml that the component's configuration holds,
its variable. Strings are written as-is, booleans and numbers as text, lists
and maps as JSON.

Values live in the pod spec rather than a ConfigMap, so every ReplicaSet keeps
the configuration it was rolled out with, even when an old pod restarts during
a rollout.

Usage: include "ngrok-operator.componentEnv" (dict "context" $ "component" "agent")
*/}}
{{- define "ngrok-operator.componentEnv" -}}
{{- $config := fromYaml (include "ngrok-operator.componentConfig" .) -}}
{{- range $path, $name := .context.Files.Get "files/operator-env.yaml" | fromYaml }}
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
{{- if kindIs "map" $val -}}
{{- include "ngrok-operator.dropEmpty" $val -}}
{{- end -}}
{{- if and (not (kindIs "bool" $val)) (empty $val) -}}
{{- $_ := unset $m $key -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
"true" when one-click demo mode is on. The agent and bindings-forwarder need
credentials to do anything but crashloop, so they are not rendered then.
*/}}
{{- define "ngrok-operator.oneClickDemoMode" -}}
{{- if .Values.features.oneClickDemoMode -}}
true
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
