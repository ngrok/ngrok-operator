#!/usr/bin/env bash
#
# ngrok-api-helper.sh - Helper script for ngrok API assertions in e2e tests
#
# Usage:
#   ngrok-api-helper.sh endpoint exists <url-pattern>
#   ngrok-api-helper.sh endpoint absent <url-pattern>
#   ngrok-api-helper.sh endpoint list
#   ngrok-api-helper.sh endpoint delete-matching <pattern>
#   ngrok-api-helper.sh k8sop exists <id>
#   ngrok-api-helper.sh k8sop absent <id>
#
# Environment:
#   NGROK_API_KEY - Required for ngrok API access
#
# Examples:
#   ngrok-api-helper.sh endpoint exists "my-app.internal"
#   ngrok-api-helper.sh endpoint delete-matching "uninstall-test"
#   ngrok-api-helper.sh k8sop absent "k8sop_abc123"
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Retry settings
MAX_RETRIES="${MAX_RETRIES:-10}"
RETRY_DELAY="${RETRY_DELAY:-3}"
MAX_RETRY_DELAY="${MAX_RETRY_DELAY:-30}"

# Calculate backoff delay: linear backoff (delay * attempt), capped at MAX_RETRY_DELAY
get_backoff_delay() {
    local attempt=$1
    local delay=$((RETRY_DELAY * attempt))
    if [ "$delay" -gt "$MAX_RETRY_DELAY" ]; then
        delay=$MAX_RETRY_DELAY
    fi
    echo "$delay"
}

# API settings
NGROK_API_URL="${NGROK_API_URL:-https://api.ngrok.com}"

log_success() { echo -e "${GREEN}✓${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1" >&2; }
log_info() { echo -e "${YELLOW}→${NC} $1"; }

# Check for required env var
if [[ -z "${NGROK_API_KEY:-}" ]]; then
    log_error "NGROK_API_KEY environment variable is required"
    exit 1
fi

# ============================================================
# Endpoint functions
# ============================================================

fetch_endpoints() {
    ngrok api endpoints list --api-key "${NGROK_API_KEY}" 2>/dev/null
}

get_endpoint_urls() {
    fetch_endpoints | jq -r '.endpoints[].url // empty' 2>/dev/null | sort
}

endpoint_pattern_exists() {
    local pattern="$1"
    get_endpoint_urls | grep -qE "$pattern"
}

endpoint_exists() {
    local pattern="${1:?Missing pattern argument}"
    log_info "Checking if endpoint matching '$pattern' exists (max ${MAX_RETRIES} retries)..."
    for i in $(seq 1 "$MAX_RETRIES"); do
        if endpoint_pattern_exists "$pattern"; then
            log_success "Endpoint matching '$pattern' found"
            return 0
        fi
        if [ "$i" -lt "$MAX_RETRIES" ]; then
            delay=$(get_backoff_delay "$i")
            log_info "Attempt $i/$MAX_RETRIES failed, retrying in ${delay}s..."
            sleep "$delay"
        fi
    done
    log_error "Endpoint matching '$pattern' NOT found after $MAX_RETRIES attempts"
    echo "Current endpoints:"
    get_endpoint_urls | sed 's/^/  /'
    return 1
}

endpoint_absent() {
    local pattern="${1:?Missing pattern argument}"
    log_info "Checking if endpoint matching '$pattern' is absent (max ${MAX_RETRIES} retries)..."
    for i in $(seq 1 "$MAX_RETRIES"); do
        if ! endpoint_pattern_exists "$pattern"; then
            log_success "Endpoint matching '$pattern' is absent"
            return 0
        fi
        if [ "$i" -lt "$MAX_RETRIES" ]; then
            delay=$(get_backoff_delay "$i")
            log_info "Attempt $i/$MAX_RETRIES: still exists, retrying in ${delay}s..."
            sleep "$delay"
        fi
    done
    log_error "Endpoint matching '$pattern' still exists after $MAX_RETRIES attempts (expected absent)"
    echo "Matching endpoints:"
    get_endpoint_urls | grep -E "$pattern" | sed 's/^/  /'
    return 1
}

endpoint_list() {
    log_info "Listing all endpoints..."
    get_endpoint_urls | sed 's/^/  /'
}

endpoint_delete_matching() {
    local pattern="${1:?Missing pattern argument}"
    log_info "Deleting endpoints matching '$pattern'..."

    ENDPOINTS_JSON=$(fetch_endpoints)
    MATCHING=$(echo "$ENDPOINTS_JSON" | jq -r ".endpoints[] | select(.url | test(\"$pattern\")) | \"\(.id) \(.url)\"" 2>/dev/null)

    if [ -z "$MATCHING" ]; then
        log_info "No endpoints matching '$pattern' to delete"
        return 0
    fi

    FAIL_FILE=$(mktemp)
    echo "0" > "$FAIL_FILE"

    echo "$MATCHING" | while read -r id url; do
        log_info "Deleting endpoint: $url ($id)"
        DELETE_OUTPUT=$(ngrok api endpoints delete "$id" --api-key "${NGROK_API_KEY}" 2>&1) || {
            log_error "Failed to delete $id: $DELETE_OUTPUT"
            echo "1" > "$FAIL_FILE"
        }
    done

    FAILED=$(cat "$FAIL_FILE")
    rm -f "$FAIL_FILE"

    if [ "$FAILED" -eq 1 ]; then
        log_error "Some endpoints failed to delete"
        return 1
    fi
    log_success "Deleted all endpoints matching '$pattern'"
}

# ============================================================
# Kubernetes Operator functions
# ============================================================

k8sop_get() {
    local id="${1:?Missing k8sop ID argument}"
    curl -s -X GET \
        -H "Authorization: Bearer ${NGROK_API_KEY}" \
        -H "Ngrok-Version: 2" \
        "${NGROK_API_URL}/kubernetes_operators/${id}"
}

k8sop_exists() {
    local id="${1:?Missing k8sop ID argument}"
    log_info "Checking if KubernetesOperator '$id' exists..."

    RESPONSE=$(k8sop_get "$id")
    HTTP_STATUS=$(echo "$RESPONSE" | jq -r '.error_code // empty' 2>/dev/null)

    if [ -z "$HTTP_STATUS" ]; then
        log_success "KubernetesOperator '$id' exists"
        return 0
    else
        log_error "KubernetesOperator '$id' not found: $RESPONSE"
        return 1
    fi
}

k8sop_absent() {
    local id="${1:?Missing k8sop ID argument}"
    log_info "Checking if KubernetesOperator '$id' is absent (max ${MAX_RETRIES} retries)..."

    for i in $(seq 1 "$MAX_RETRIES"); do
        RESPONSE=$(k8sop_get "$id")
        ERROR_CODE=$(echo "$RESPONSE" | jq -r '.error_code // empty' 2>/dev/null)
        STATUS_CODE=$(echo "$RESPONSE" | jq -r '.status_code // empty' 2>/dev/null)

        # If we get an error code (like ERR_NGROK_404) or status_code 404, the k8sop is gone
        if [ -n "$ERROR_CODE" ]; then
            log_success "KubernetesOperator '$id' is absent (got error: $ERROR_CODE)"
            return 0
        fi
        if [ "$STATUS_CODE" = "404" ]; then
            log_success "KubernetesOperator '$id' is absent (got 404)"
            return 0
        fi

        if [ "$i" -lt "$MAX_RETRIES" ]; then
            delay=$(get_backoff_delay "$i")
            log_info "Attempt $i/$MAX_RETRIES: still exists, retrying in ${delay}s..."
            sleep "$delay"
        fi
    done

    log_error "KubernetesOperator '$id' still exists after $MAX_RETRIES attempts"
    echo "Response: $RESPONSE"
    return 1
}

# ============================================================
# Main command handling
# ============================================================

case "${1:-help}" in
    endpoint)
        case "${2:-}" in
            exists)       endpoint_exists "${3:-}" ;;
            absent)       endpoint_absent "${3:-}" ;;
            list)         endpoint_list ;;
            delete-matching) endpoint_delete_matching "${3:-}" ;;
            *)
                log_error "Unknown endpoint command: ${2:-}"
                echo "Usage: $0 endpoint {exists|absent|list|delete-matching} [args]"
                exit 1
                ;;
        esac
        ;;

    k8sop)
        case "${2:-}" in
            exists) k8sop_exists "${3:-}" ;;
            absent) k8sop_absent "${3:-}" ;;
            *)
                log_error "Unknown k8sop command: ${2:-}"
                echo "Usage: $0 k8sop {exists|absent} <id>"
                exit 1
                ;;
        esac
        ;;

    help|--help|-h)
        echo "Usage: $0 <resource> <command> [args]"
        echo ""
        echo "Endpoint commands:"
        echo "  endpoint exists <pattern>         Assert endpoint URL matching pattern exists"
        echo "  endpoint absent <pattern>         Assert endpoint URL matching pattern is absent"
        echo "  endpoint list                     List all endpoint URLs"
        echo "  endpoint delete-matching <pattern> Delete all endpoints matching pattern"
        echo ""
        echo "KubernetesOperator commands:"
        echo "  k8sop exists <id>                 Assert KubernetesOperator exists in ngrok API"
        echo "  k8sop absent <id>                 Assert KubernetesOperator is gone from ngrok API"
        echo ""
        echo "Environment:"
        echo "  NGROK_API_KEY          Required for ngrok API access"
        echo "  MAX_RETRIES            Max retry attempts (default: 10)"
        echo "  RETRY_DELAY            Base seconds for backoff (default: 3)"
        echo "  MAX_RETRY_DELAY        Max backoff delay cap (default: 30)"
        exit 0
        ;;

    *)
        log_error "Unknown resource: $1"
        echo "Run '$0 --help' for usage"
        exit 1
        ;;
esac
