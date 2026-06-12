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
Common annotations applied to all resources
*/}}
{{- define "ngrok-operator.commonAnnotations" -}}
{{- with .Values.commonAnnotations }}
{{- toYaml . }}
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
Fail the render when values from the pre-v1 layout are present. The values
tree was restructured (see specs/migration-v1.md) with no compatibility
shims; without this guard, old values would be silently ignored and the
install would proceed with defaults.
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
  "log" "ngrok.log"
  "ingress" "features.ingress"
  "gateway" "features.gateway"
  "bindings" "features.bindings (deployment settings: bindingsForwarder)"
  "ingressClass" "features.ingress.ingressClass"
  "watchNamespace" "features.ingress.watchNamespace"
  "controllerName" "features.ingress.controllerName"
  "drainPolicy" "features.drainPolicy"
  "defaultDomainReclaimPolicy" "features.defaultDomainReclaimPolicy"
  "oneClickDemoMode" "apiManager.config.oneClickDemoMode"
  "replicaCount" "apiManager.replicaCount"
  "podAnnotations" "apiManager.podAnnotations (and agent / bindingsForwarder)"
  "podLabels" "apiManager.podLabels (and agent / bindingsForwarder)"
  "affinity" "apiManager.affinity"
  "podAffinityPreset" "apiManager.podAffinityPreset"
  "podAntiAffinityPreset" "apiManager.podAntiAffinityPreset"
  "nodeAffinityPreset" "apiManager.nodeAffinityPreset"
  "nodeSelector" "apiManager.nodeSelector"
  "tolerations" "apiManager.tolerations"
  "topologySpreadConstraints" "apiManager.topologySpreadConstraints"
  "priorityClassName" "apiManager.priorityClassName"
  "terminationGracePeriodSeconds" "apiManager.terminationGracePeriodSeconds"
  "lifecycle" "apiManager.lifecycle"
  "podDisruptionBudget" "apiManager.podDisruptionBudget"
  "resources" "apiManager.resources"
  "extraVolumes" "apiManager.extraVolumes"
  "extraVolumeMounts" "apiManager.extraVolumeMounts"
  "extraEnv" "apiManager.extraEnv"
  "serviceAccount" "apiManager.serviceAccount"
-}}
{{- $found := list -}}
{{- range $old, $new := $moved -}}
{{- if hasKey $.Values $old }}{{ $found = append $found (printf "  %s -> %s" $old $new) }}{{ end -}}
{{- end -}}
{{- if $found }}
{{- fail (printf "\n\nThe following Helm values were moved in this release and are no longer supported at their old locations:\n\n%s\n\nUpdate your values and try again. See https://github.com/ngrok/ngrok-operator/blob/main/specs/migration-v1.md for the full mapping." (join "\n" (sortAlpha $found))) }}
{{- end -}}
{{- end -}}

{{/*
Render a map as a comma separated list of key=value pairs.
*/}}
{{- define "ngrok-operator.kvPairs" -}}
{{- $pairs := list -}}
{{- range $key, $value := . -}}
{{- $pairs = append $pairs (printf "%s=%v" $key $value) -}}
{{- end -}}
{{- $pairs | join "," -}}
{{- end -}}

{{/*
Render a single config value: maps become comma separated key=value pairs,
lists become comma separated values, scalars are printed as-is.
*/}}
{{- define "ngrok-operator.confValue" -}}
{{- if kindIs "map" . -}}
{{- include "ngrok-operator.kvPairs" . -}}
{{- else if kindIs "slice" . -}}
{{- $items := list -}}
{{- range . }}{{ $items = append $items (printf "%v" .) }}{{ end -}}
{{- $items | join "," -}}
{{- else -}}
{{- printf "%v" . -}}
{{- end -}}
{{- end -}}

{{/*
Shared app config built from `ngrok.*` and `features.*`, returned as a YAML
dict of dotted config keys to string values (parse with fromYaml). Booleans
are always rendered; strings, maps, and lists only when non-empty.
*/}}
{{- define "ngrok-operator.sharedConfig" -}}
{{- $config := dict -}}
{{- with .Values.ngrok -}}
{{- if .description }}{{ $_ := set $config "description" .description }}{{ end -}}
{{- if .region }}{{ $_ := set $config "region" .region }}{{ end -}}
{{- if .rootCAs }}{{ $_ := set $config "rootCAs" .rootCAs }}{{ end -}}
{{- if .serverAddr }}{{ $_ := set $config "serverAddr" .serverAddr }}{{ end -}}
{{- if .apiURL }}{{ $_ := set $config "apiURL" .apiURL }}{{ end -}}
{{- if .metadata }}{{ $_ := set $config "metadata" (include "ngrok-operator.kvPairs" .metadata) }}{{ end -}}
{{- if .clusterDomain }}{{ $_ := set $config "clusterDomain" .clusterDomain }}{{ end -}}
{{- with .log -}}
{{- if .level }}{{ $_ := set $config "log.level" (printf "%v" .level) }}{{ end -}}
{{- if .format }}{{ $_ := set $config "log.format" .format }}{{ end -}}
{{- if .stacktraceLevel }}{{ $_ := set $config "log.stacktraceLevel" .stacktraceLevel }}{{ end -}}
{{- end -}}
{{- end -}}
{{- with .Values.features -}}
{{- $_ := set $config "features.ingress.enabled" (printf "%v" .ingress.enabled) -}}
{{- if .ingress.controllerName }}{{ $_ := set $config "features.ingress.controllerName" .ingress.controllerName }}{{ end -}}
{{- if .ingress.watchNamespace }}{{ $_ := set $config "features.ingress.watchNamespace" .ingress.watchNamespace }}{{ end -}}
{{- $_ := set $config "features.gateway.enabled" (printf "%v" .gateway.enabled) -}}
{{- $_ := set $config "features.gateway.disableReferenceGrants" (printf "%v" .gateway.disableReferenceGrants) -}}
{{- $_ := set $config "features.bindings.enabled" (printf "%v" .bindings.enabled) -}}
{{- if .bindings.enabled -}}
{{- if .bindings.endpointSelectors }}{{ $_ := set $config "features.bindings.endpointSelectors" (include "ngrok-operator.confValue" .bindings.endpointSelectors) }}{{ end -}}
{{- if .bindings.serviceAnnotations }}{{ $_ := set $config "features.bindings.serviceAnnotations" (include "ngrok-operator.kvPairs" .bindings.serviceAnnotations) }}{{ end -}}
{{- if .bindings.serviceLabels }}{{ $_ := set $config "features.bindings.serviceLabels" (include "ngrok-operator.kvPairs" .bindings.serviceLabels) }}{{ end -}}
{{- if .bindings.ingressEndpoint }}{{ $_ := set $config "features.bindings.ingressEndpoint" .bindings.ingressEndpoint }}{{ end -}}
{{- end -}}
{{- if .drainPolicy }}{{ $_ := set $config "features.drainPolicy" (printf "%s" .drainPolicy) }}{{ end -}}
{{- if .defaultDomainReclaimPolicy }}{{ $_ := set $config "features.defaultDomainReclaimPolicy" (printf "%s" .defaultDomainReclaimPolicy) }}{{ end -}}
{{- end -}}
{{- $config | toYaml -}}
{{- end -}}

{{/*
The effective config for a component: the shared config with the component's
`config` map merged on top (component keys win). Takes a dict with "context"
(the chart root) and "config" (the component's config map). Returns a YAML
dict (parse with fromYaml).
*/}}
{{- define "ngrok-operator.componentConfig" -}}
{{- $config := fromYaml (include "ngrok-operator.sharedConfig" .context) -}}
{{- range $key, $value := .config -}}
{{- $_ := set $config $key (include "ngrok-operator.confValue" $value) -}}
{{- end -}}
{{- $config | toYaml -}}
{{- end -}}

{{/*
Environment variables injected from a component's config ConfigMap,
argocd-cmd-params-cm style: each entry is a valueFrom.configMapKeyRef with
optional: true so missing keys fall back to the binary's built-in defaults.
Explicit CLI flags > these env vars > built-in defaults.

Takes a dict with "context" (the chart root), "component" (the ConfigMap name
suffix), and "keys" (a list of (env var name, config key) pairs).
*/}}
{{- define "ngrok-operator.configEnv" -}}
{{- $cm := printf "%s-%s-config" (include "ngrok-operator.fullname" .context) .component -}}
{{- range .keys }}
- name: {{ index . 0 }}
  valueFrom:
    configMapKeyRef:
      name: {{ $cm }}
      key: {{ index . 1 }}
      optional: true
{{- end -}}
{{- end -}}

{{/*
Create the name of the api-manager service account to use
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
  - update
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
