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
Ngrok Operator manager cli feature flags
*/}}
{{- define "ngrok-operator.manager.cliFeatureFlags" -}}
{{- if .Values.ingress.enabled -}}
- --enable-feature-ingress={{ .Values.ingress.enabled }}
{{- end }}
{{- if .Values.useExperimentalGatewayApi | default .Values.gateway.enabled }}
- --enable-feature-gateway=true
{{- else }}
- --enable-feature-gateway=false
{{- end }}
{{- if .Values.gateway.disableReferenceGrants }}
- --disable-reference-grants=true
{{- else }}
- --disable-reference-grants=false
{{- end }}
{{- if .Values.bindings.enabled }}
- --enable-feature-bindings={{ .Values.bindings.enabled }}
{{- end }}
{{- end -}}

{{/*
Create the name of the controller service account to use
*/}}
{{- define "ngrok-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "ngrok-operator.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
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
{{- if .Values.bindings.forwarder.serviceAccount.create -}}
    {{ default (printf "%s-bindings-forwarder" (include "ngrok-operator.fullname" .)) .Values.bindings.forwarder.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.bindings.forwarder.serviceAccount.name }}
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
