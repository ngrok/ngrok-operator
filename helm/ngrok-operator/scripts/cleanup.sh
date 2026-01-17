#!/bin/bash
set -euo pipefail

# Configuration from environment variables
TIMEOUT="${CLEANUP_TIMEOUT:-300}"
RETRIES="${CLEANUP_RETRIES:-3}"
RETRY_INTERVAL="${CLEANUP_RETRY_INTERVAL:-10}"
POLL_INTERVAL="${POLL_INTERVAL:-2}"
FINALIZER="k8s.ngrok.com/finalizer"
CLEANUP_ANNOTATION="k8s.ngrok.com/cleanup"

log() {
  echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] $*"
}

log_error() {
  echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] ERROR: $*" >&2
}

# Check if a CRD exists
crd_exists() {
  local group="$1"
  local version="$2"
  local resource="$3"

  kubectl get crd "${resource}.${group}" &>/dev/null
}

# Check if a resource type is available (handles both CRDs and core types)
resource_type_exists() {
  local api_version="$1"
  local kind="$2"

  # For core types (empty group), just try to list
  if [[ "$api_version" == "v1" ]] || [[ "$api_version" == "networking.k8s.io/v1" ]]; then
    return 0
  fi

  # For CRDs, check if they exist
  local group="${api_version%/*}"
  local plural="${kind,,}s"  # Simple pluralization
  # Handle special plural forms.
  # Example: ends in 'y' -> 'ies'
  if [[ "$kind" == *y ]]; then
    plural="${kind%y}ies"
  elif [[ "$kind" == "Ingress" ]]; then
    plural="ingresses"
  fi
  kubectl get crd "${plural}.${group}" &>/dev/null
}

# List resources with the ngrok finalizer
list_resources_with_finalizer() {
  local api_version="$1"
  local kind="$2"
  local namespaced="${3:-true}"

  local output
  if [[ "$namespaced" == "true" ]]; then
    if ! output=$(kubectl get "${kind}" -A -o json 2>&1); then
      log_error "Failed to list ${kind} resources: ${output}"
      return 1
    fi
    echo "$output" | jq -r --arg finalizer "$FINALIZER" \
      '.items[] | select(.metadata.finalizers[]? == $finalizer) | "\(.metadata.namespace)/\(.metadata.name)"'
  else
    if ! output=$(kubectl get "${kind}" -o json 2>&1); then
      log_error "Failed to list ${kind} resources: ${output}"
      return 1
    fi
    echo "$output" | jq -r --arg finalizer "$FINALIZER" \
      '.items[] | select(.metadata.finalizers[]? == $finalizer) | .metadata.name'
  fi
}

# Annotate a resource for cleanup
annotate_resource() {
  local kind="$1"
  local namespace="$2"
  local name="$3"

  if [[ -n "$namespace" ]]; then
    # Check if already annotated
    local current_annotation
    current_annotation=$(kubectl get "${kind}" -n "${namespace}" "${name}" -o jsonpath="{.metadata.annotations['${CLEANUP_ANNOTATION}']}" 2>/dev/null || echo "")

    if [[ "$current_annotation" == "true" ]]; then
      log "Resource ${kind}/${namespace}/${name} already has cleanup annotation"
      return 0
    fi

    log "Annotating ${kind}/${namespace}/${name} for cleanup"
    kubectl annotate "${kind}" -n "${namespace}" "${name}" "${CLEANUP_ANNOTATION}=true" --overwrite
  else
    # Cluster-scoped resource
    local current_annotation
    current_annotation=$(kubectl get "${kind}" "${name}" -o jsonpath="{.metadata.annotations['${CLEANUP_ANNOTATION}']}" 2>/dev/null || echo "")

    if [[ "$current_annotation" == "true" ]]; then
      log "Resource ${kind}/${name} already has cleanup annotation"
      return 0
    fi

    log "Annotating ${kind}/${name} for cleanup"
    kubectl annotate "${kind}" "${name}" "${CLEANUP_ANNOTATION}=true" --overwrite
  fi
}

# Check if a resource still has the finalizer
has_finalizer() {
  local kind="$1"
  local namespace="$2"
  local name="$3"

  local finalizers
  if [[ -n "$namespace" ]]; then
    finalizers=$(kubectl get "${kind}" -n "${namespace}" "${name}" -o jsonpath='{.metadata.finalizers[*]}' 2>/dev/null || echo "")
  else
    finalizers=$(kubectl get "${kind}" "${name}" -o jsonpath='{.metadata.finalizers[*]}' 2>/dev/null || echo "")
  fi

  [[ "$finalizers" =~ $FINALIZER ]]
}

# Wait for finalizers to be removed from a list of resources
wait_for_finalizers_removed() {
  local kind="$1"
  local -n resources_ref="$2"
  local timeout="$3"

  local end_time=$((SECONDS + timeout))
  local remaining=("${resources_ref[@]}")

  log "Waiting for ${#remaining[@]} ${kind} resources to have finalizers removed (timeout: ${timeout}s)"

  while [[ ${#remaining[@]} -gt 0 ]] && [[ $SECONDS -lt $end_time ]]; do
    local new_remaining=()

    for resource in "${remaining[@]}"; do
      local namespace=""
      local name="$resource"

      # Split namespace/name for namespaced resources
      if [[ "$resource" == *"/"* ]]; then
        namespace="${resource%%/*}"
        name="${resource##*/}"
      fi

      # Check if resource still exists
      local exists=true
      if [[ -n "$namespace" ]]; then
        kubectl get "${kind}" -n "${namespace}" "${name}" &>/dev/null || exists=false
      else
        kubectl get "${kind}" "${name}" &>/dev/null || exists=false
      fi

      if [[ "$exists" == "false" ]]; then
        log "Resource ${kind}/${resource} has been deleted"
        continue
      fi

      # Check if finalizer is still present
      if has_finalizer "$kind" "$namespace" "$name"; then
        new_remaining+=("$resource")
      else
        log "Finalizer removed from ${kind}/${resource}"
      fi
    done

    remaining=("${new_remaining[@]}")

    if [[ ${#remaining[@]} -gt 0 ]]; then
      log "Still waiting for ${#remaining[@]} ${kind} resources..."
      sleep "$POLL_INTERVAL"
    fi
  done

  if [[ ${#remaining[@]} -gt 0 ]]; then
    log_error "Timeout waiting for ${kind} finalizers to be removed. ${#remaining[@]} resources remaining:"
    printf '%s\n' "${remaining[@]}" >&2
    return 1
  fi

  log "All ${kind} finalizers removed"
  return 0
}

# Process a resource type with retries
process_resource_type() {
  local name="$1"
  local api_version="$2"
  local kind="$3"
  local namespaced="${4:-true}"
  local optional="${5:-false}"

  log "Processing resource type: ${name}"

  # Check if resource type exists
  if ! resource_type_exists "$api_version" "$kind"; then
    if [[ "$optional" == "true" ]]; then
      log "Optional resource type ${name} not found, skipping"
      return 0
    else
      log_error "Required resource type ${name} not found"
      return 1
    fi
  fi

  local attempt=0
  while [[ $attempt -le $RETRIES ]]; do
    if [[ $attempt -gt 0 ]]; then
      log "Retrying ${name} (attempt $((attempt + 1))/$((RETRIES + 1)))"
      sleep "$RETRY_INTERVAL"
    fi

    if process_resource_type_once "$name" "$api_version" "$kind" "$namespaced"; then
      return 0
    fi

    ((attempt++))
  done

  log_error "Failed to process ${name} after $((RETRIES + 1)) attempts"
  return 1
}

# Process a resource type once
process_resource_type_once() {
  local name="$1"
  local api_version="$2"
  local kind="$3"
  local namespaced="$4"

  # List resources with finalizer
  local resources
  mapfile -t resources < <(list_resources_with_finalizer "$api_version" "$kind" "$namespaced")

  if [[ ${#resources[@]} -eq 0 ]]; then
    log "No ${name} resources with ngrok finalizer found"
    return 0
  fi

  log "Found ${#resources[@]} ${name} resources to cleanup"

  # Annotate all resources
  for resource in "${resources[@]}"; do
    local namespace=""
    local resource_name="$resource"

    if [[ "$resource" == *"/"* ]]; then
      namespace="${resource%%/*}"
      resource_name="${resource##*/}"
    fi

    if ! annotate_resource "$kind" "$namespace" "$resource_name"; then
      log_error "Failed to annotate ${kind}/${resource}"
      return 1
    fi
  done

  # Wait for finalizers to be removed
  wait_for_finalizers_removed "$kind" resources "$TIMEOUT"
}

main() {
  log "Starting ngrok-operator cleanup"
  log "Configuration: timeout=${TIMEOUT}s, retries=${RETRIES}, retry_interval=${RETRY_INTERVAL}s"

  # Define resource types in processing order
  # Format: name api_version kind namespaced optional
  local resource_types=(
    # Gateway API Routes (depend on Gateways)
    "HTTPRoute|gateway.networking.k8s.io/v1|httproute|true|true"
    "TCPRoute|gateway.networking.k8s.io/v1alpha2|tcproute|true|true"
    "TLSRoute|gateway.networking.k8s.io/v1alpha2|tlsroute|true|true"

    # Core Kubernetes resources
    "Ingress|networking.k8s.io/v1|ingress|true|false"
    "Service|v1|service|true|false"

    # Gateway API Gateways
    "Gateway|gateway.networking.k8s.io/v1|gateway|true|true"

    # ngrok CRDs
    "CloudEndpoint|ngrok.k8s.ngrok.com/v1alpha1|cloudendpoint|true|false"
    "AgentEndpoint|ngrok.k8s.ngrok.com/v1alpha1|agentendpoint|true|false"
    "Domain|ingress.k8s.ngrok.com/v1alpha1|domain|true|false"
    "IPPolicy|ingress.k8s.ngrok.com/v1alpha1|ippolicy|true|false"
    "NgrokTrafficPolicy|ngrok.k8s.ngrok.com/v1alpha1|ngroktrafficpolicy|true|false"
    "KubernetesOperator|ngrok.k8s.ngrok.com/v1alpha1|kubernetesoperator|true|false"
  )

  local failed=0
  for resource_def in "${resource_types[@]}"; do
    IFS='|' read -r name api_version kind namespaced optional <<< "$resource_def"

    if ! process_resource_type "$name" "$api_version" "$kind" "$namespaced" "$optional"; then
      if [[ "$optional" == "false" ]]; then
        failed=1
        log_error "Failed to process required resource type: ${name}"
      else
        log "Failed to process optional resource type: ${name}, continuing..."
      fi
    fi
  done

  if [[ $failed -eq 1 ]]; then
    log_error "Cleanup failed for one or more required resource types"
    exit 1
  fi

  log "Cleanup completed successfully"
}

main "$@"
