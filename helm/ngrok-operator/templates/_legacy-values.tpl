{{- /*
LEGACY-values-layout: delete this file, the include at the top of
api-manager/deployment.yaml and the tagged tests, once 0.25 is the oldest
release users upgrade from. See docs/developer-guide/passivity-shims.md.
*/ -}}

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
  "ingress" "features.ingress"
  "ingressClass" "features.ingress.ingressClass"
  "controllerName" "features.ingress.controllerName"
  "watchNamespace" "features.ingress.watchNamespace"
  "gateway" "features.gateway"
  "bindings" "features.bindings (forwarder pod settings: bindingsForwarder)"
  "defaultDomainReclaimPolicy" "features.domains.defaultReclaimPolicy"
  "drainPolicy" "features.cleanup.drainPolicy"
  "oneClickDemoMode" "features.oneClickDemoMode.enabled"
  "podAnnotations" "pod.podAnnotations"
  "podLabels" "pod.podLabels"
  "nodeSelector" "pod.nodeSelector"
  "tolerations" "pod.tolerations"
  "affinity" "pod.affinity"
  "podAffinityPreset" "pod.podAffinityPreset"
  "podAntiAffinityPreset" "pod.podAntiAffinityPreset"
  "nodeAffinityPreset" "pod.nodeAffinityPreset"
  "topologySpreadConstraints" "pod.topologySpreadConstraints"
  "priorityClassName" "pod.priorityClassName"
  "extraEnv" "pod.extraEnv"
  "replicaCount" "apiManager.replicaCount"
  "resources" "apiManager.resources"
  "lifecycle" "apiManager.lifecycle"
  "terminationGracePeriodSeconds" "apiManager.terminationGracePeriodSeconds"
  "extraVolumes" "apiManager.extraVolumes"
  "extraVolumeMounts" "apiManager.extraVolumeMounts"
  "podDisruptionBudget" "apiManager.podDisruptionBudget"
  "serviceAccount" "apiManager.serviceAccount"
-}}
{{- /* Collect every old top-level key the user still sets, as "old -> new". */ -}}
{{- $found := list -}}
{{- range $old, $new := $moved -}}
{{- if hasKey $.Values $old }}{{ $found = append $found (printf "  %s -> %s" $old $new) }}{{ end -}}
{{- end -}}
{{- /* cleanupHook stays, but its enabled and timeout moved. */ -}}
{{- range $key := list "enabled" "timeout" -}}
{{- if hasKey ($.Values.cleanupHook | default dict) $key }}{{ $found = append $found (printf "  cleanupHook.%s -> features.cleanup.%s" $key $key) }}{{ end -}}
{{- end -}}
{{- if $found -}}
{{- fail (printf "\n\nThese Helm values moved in 0.25 and are no longer read at their old location:\n\n%s\n\nSee the 0.25 upgrade guide for the full mapping." (join "\n" (sortAlpha $found))) -}}
{{- end -}}
{{- end -}}
