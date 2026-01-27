#!/usr/bin/env bash

# Delete all ngrok KubernetesOperator resources in your account
# Requires NGROK_API_KEY environment variable

set -euo pipefail

: "${NGROK_API_KEY:?NGROK_API_KEY environment variable is not set}"

API_URL="https://api.ngrok.com/kubernetes_operators"

while true; do
  ids=$(curl -sSL -H "Authorization: Bearer $NGROK_API_KEY" -H "Ngrok-Version: 2" "$API_URL" | jq -r '.operators[].id // empty')
  
  if [[ -z "$ids" ]]; then
    echo "Done - no more operators to delete."
    break
  fi
  
  for id in $ids; do
    echo "Deleting $id"
    curl -sSL -X DELETE -H "Authorization: Bearer $NGROK_API_KEY" -H "Ngrok-Version: 2" "$API_URL/$id" || true
  done
done
