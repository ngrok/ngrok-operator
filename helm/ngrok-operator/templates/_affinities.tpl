{{/*
Render a value as YAML. Run it through tpl when it holds a template.
Usage: include "ngrok-operator.tplvalues.render" (dict "value" .Values.x "context" $)
*/}}
{{- define "ngrok-operator.tplvalues.render" -}}
{{- $value := typeIs "string" .value | ternary .value (.value | toYaml) -}}
{{- if contains "{{" (toJson .value) -}}
{{- tpl $value .context -}}
{{- else -}}
{{- $value -}}
{{- end -}}
{{- end -}}

{{/*
Return a podAffinity or podAntiAffinity preset for a component.
Usage: include "ngrok-operator.affinities.pods" (dict "type" "soft" "component" "agent" "context" $)
*/}}
{{- define "ngrok-operator.affinities.pods" -}}
{{- $matchLabels := include "ngrok-operator.selectorLabels" .context | fromYaml | toYaml -}}
{{- if eq .type "soft" -}}
preferredDuringSchedulingIgnoredDuringExecution:
  - podAffinityTerm:
      labelSelector:
        matchLabels: {{- $matchLabels | nindent 10 }}
          app.kubernetes.io/component: {{ .component }}
      topologyKey: kubernetes.io/hostname
    weight: 1
{{- else if eq .type "hard" -}}
requiredDuringSchedulingIgnoredDuringExecution:
  - labelSelector:
      matchLabels: {{- $matchLabels | nindent 8 }}
        app.kubernetes.io/component: {{ .component }}
    topologyKey: kubernetes.io/hostname
{{- end -}}
{{- end -}}

{{/*
Return a nodeAffinity preset.
Usage: include "ngrok-operator.affinities.nodes" (dict "type" "hard" "key" "kubernetes.io/arch" "values" (list "amd64"))
*/}}
{{- define "ngrok-operator.affinities.nodes" -}}
{{- if eq .type "soft" -}}
preferredDuringSchedulingIgnoredDuringExecution:
  - preference:
      matchExpressions:
        - key: {{ .key }}
          operator: In
          values:
            {{- range .values }}
            - {{ . | quote }}
            {{- end }}
    weight: 1
{{- else if eq .type "hard" -}}
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    - matchExpressions:
        - key: {{ .key }}
          operator: In
          values:
            {{- range .values }}
            - {{ . | quote }}
            {{- end }}
{{- end -}}
{{- end -}}
