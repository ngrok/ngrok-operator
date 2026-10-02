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
  "clusterDomain" "ngrok.clusterDomain"
  "log" "ngrok.log"
  "credentials" "ngrok.credentials (apiKey and authtoken are replaced by accessToken)"
  "ingress" "ngrok.features.ingress"
  "ingressClass" "ngrok.features.ingress.ingressClass"
  "controllerName" "ngrok.features.ingress.controllerName"
  "watchNamespace" "ngrok.features.ingress.watchNamespace"
  "gateway" "ngrok.features.gateway"
  "bindings" "ngrok.features.bindings (forwarder pod settings: components.bindingsForwarder)"
  "defaultDomainReclaimPolicy" "ngrok.features.domains.defaultReclaimPolicy"
  "drainPolicy" "ngrok.features.cleanup.drainPolicy"
  "oneClickDemoMode" "ngrok.features.oneClickDemoMode.enabled"
  "cleanupHook" "components.cleanupHook (enabled and timeout: ngrok.features.cleanup)"
  "agent" "components.agent"
  "podAnnotations" "components.common.podAnnotations"
  "podLabels" "components.common.podLabels"
  "nodeSelector" "components.common.nodeSelector"
  "tolerations" "components.common.tolerations"
  "affinity" "components.common.affinity"
  "podAffinityPreset" "components.common.podAffinityPreset"
  "podAntiAffinityPreset" "components.common.podAntiAffinityPreset"
  "nodeAffinityPreset" "components.common.nodeAffinityPreset"
  "topologySpreadConstraints" "components.common.topologySpreadConstraints"
  "priorityClassName" "components.common.priorityClassName"
  "extraEnv" "components.common.extraEnv"
  "replicaCount" "components.apiManager.replicaCount"
  "resources" "components.apiManager.resources"
  "lifecycle" "components.common.lifecycle"
  "terminationGracePeriodSeconds" "components.apiManager.terminationGracePeriodSeconds"
  "extraVolumes" "components.common.extraVolumes"
  "extraVolumeMounts" "components.common.extraVolumeMounts"
  "podDisruptionBudget" "components.apiManager.podDisruptionBudget"
  "serviceAccount" "components.apiManager.serviceAccount"
-}}
{{- /* Collect every old top-level key the user still sets, as "old -> new". */ -}}
{{- $found := list -}}
{{- range $old, $new := $moved -}}
{{- if hasKey $.Values $old }}{{ $found = append $found (printf "  %s -> %s" $old $new) }}{{ end -}}
{{- end -}}
{{- if $found -}}
{{- fail (printf "\n\nThese Helm values moved in 0.25 and are no longer read at their old location:\n\n%s\n\nSee the 0.25 upgrade guide for the full mapping." (join "\n" (sortAlpha $found))) -}}
{{- end -}}
{{- end -}}
