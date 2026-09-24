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

{{/*
The effective values for a component: `defaults` with the component's own
section merged on top. Maps deep-merge with the component winning; arrays are
replaced wholesale, because merging them is ambiguous.

Usage: include "ngrok-operator.componentValues" (dict "context" $ "component" "agent")
Returns YAML; parse with fromYaml.
*/}}
{{- define "ngrok-operator.componentValues" -}}
{{- $defaults := deepCopy (.context.Values.defaults | default dict) -}}
{{- $component := deepCopy (index .context.Values .component | default dict) -}}
{{- mergeOverwrite $defaults $component | toYaml -}}
{{- end -}}

{{/*
Shared app config, as it appears at the top level of config.yaml. Only keys the
user actually set are emitted: anything absent falls back to the operator
binary's built-in default, which keeps a single source of truth for defaults.

`features.*.enabled` and the ingressClass settings are the exception. Helm
cannot tell `false` from unset, so those always render.

That "only keys the user set" promise cannot be inherited from Helm's own
values coalescing here, because this helper reads `.Values` directly: Helm
strips null-valued keys when it merges chart defaults with user overrides
before templates run, but a template reading straight from `.Values` sees
whatever the chart's own values.yaml wrote, nulls included. The explicit
`stripNils` call below is what makes the promise hold for every caller.
*/}}
{{- define "ngrok-operator.sharedConfig" -}}
{{- $config := dict -}}
{{- with .Values.ngrok }}{{ $_ := set $config "ngrok" (deepCopy .) }}{{ end -}}
{{- with .Values.log }}{{ $_ := set $config "log" (deepCopy .) }}{{ end -}}
{{- $features := dict -}}
{{- with .Values.features -}}
{{- $ingress := deepCopy (.ingress | default dict) -}}
{{- $_ := unset $ingress "ingressClass" -}}
{{- $_ := set $features "ingress" $ingress -}}
{{- $_ := set $features "gateway" (deepCopy (.gateway | default dict)) -}}
{{- $_ := set $features "bindings" (deepCopy (.bindings | default dict)) -}}
{{- with .defaultDomainReclaimPolicy }}{{ $_ := set $features "defaultDomainReclaimPolicy" . }}{{ end -}}
{{- with .drainPolicy }}{{ $_ := set $features "drainPolicy" . }}{{ end -}}
{{- end -}}
{{- $_ := set $config "features" $features -}}
{{- include "ngrok-operator.stripNils" $config -}}
{{- $config | toYaml -}}
{{- end -}}

{{/*
Recursively removes nil-valued keys from a map, in place. A component's own
log settings (apiManager.log.level and friends) default to explicit `null`
rather than being absent, to document the key in values.yaml; this turns that
`null` back into "not present" so it never overwrites a shared value during
the merge in componentConfig.

It also drops a map that ends up empty once its nils are stripped -- so
`ngrok.metadata: {}` and `features.bindings.serviceAnnotations`/`serviceLabels:
{}` render as absent, not as `{}`. That's harmless today because every one of
those keys' Go defaults is itself `{}` -- absent and `{}` decode the same way
-- but it is a latent hole: if a shared key ever grows a non-empty Go default,
a component would have no way to override it back to empty, since `{}` would
be stripped away before it ever reached the merge.

Usage: {{ include "ngrok-operator.stripNils" $someMap }}
*/}}
{{- define "ngrok-operator.stripNils" -}}
{{- $m := . -}}
{{- range $key, $val := $m -}}
{{- if kindIs "map" $val -}}
{{- include "ngrok-operator.stripNils" $val -}}
{{- if eq (len $val) 0 -}}
{{- $_ := unset $m $key -}}
{{- end -}}
{{- else if eq (kindOf $val) "invalid" -}}
{{- $_ := unset $m $key -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
The config one component actually runs with: the shared app config with that
component's own keys merged over it. Maps deep-merge, arrays replace.

App config is separated from pod config by removing the pod keys rather than by
listing the app keys. The pod key set is structural -- everything under
`defaults`, plus the four keys that only exist per component -- so it does not
grow when a config value is added. Listing app config instead would put every
new value in one more place.

Usage: include "ngrok-operator.componentConfig" (dict "context" $ "component" "agent")
*/}}
{{- define "ngrok-operator.componentConfig" -}}
{{- $shared := fromYaml (include "ngrok-operator.sharedConfig" .context) -}}
{{- $component := deepCopy (index .context.Values .component | default dict) -}}
{{- range $key := keys (.context.Values.defaults | default dict) -}}
{{- $_ := unset $component $key -}}
{{- end -}}
{{- range $key := list "replicaCount" "updateStrategy" "podDisruptionBudget" "serviceAccount" -}}
{{- $_ := unset $component $key -}}
{{- end -}}
{{- include "ngrok-operator.stripNils" $component -}}
{{/*
The agent has no shared watchNamespace of its own. On main, a single
--watch-namespace flag governed both the agent and the ingress
controller; this refactor split that into agent.watchNamespace and
features.ingress.watchNamespace, but nothing else bridges the two, and
isNamespaced still keys the agent's Role scope off
features.ingress.watchNamespace alone. Without this fallback, an install
that sets only features.ingress.watchNamespace renders a namespaced Role
for the agent while the agent itself still watches every namespace --
forbidden at startup. Falling back here when the component didn't set its
own watchNamespace preserves main's behavior and keeps the Role's scope
matching what the agent actually watches.
*/}}
{{- if eq .component "agent" -}}
{{- if not (hasKey $component "watchNamespace") -}}
{{- with .context.Values.features.ingress.watchNamespace -}}
{{- $_ := set $component "watchNamespace" . -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- mergeOverwrite $shared $component | toYaml -}}
{{- end -}}

{{/*
Renders "true" when one-click demo mode is on, and the empty string otherwise.

Demo mode runs the api-manager without credentials so a marketplace install can
reach Ready, so the agent and bindings-forwarder Deployments — which need an
authtoken to do anything but crashloop — must not render.
*/}}
{{- define "ngrok-operator.oneClickDemoMode" -}}
{{- if (.Values.apiManager | default dict).oneClickDemoMode -}}
true
{{- end -}}
{{- end -}}

{{/*
Checksum of a component's effective config, for the pod annotation that rolls
the Deployment when its configuration changes.
*/}}
{{- define "ngrok-operator.configChecksum" -}}
{{- include "ngrok-operator.componentConfig" . | sha256sum -}}
{{- end -}}

