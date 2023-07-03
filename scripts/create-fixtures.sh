#!/usr/bin/env bash

echo "--- Deploying e2e-fixtures"
if [ "${GOOGLE_CLIENT_ID:-}" != "" ]
then
  kubectl create secret generic ngrok-corp-ingress-oauth-credentials \
    --from-literal=ClientID=$GOOGLE_CLIENT_ID \
    --from-literal=ClientSecret=$GOOGLE_CLIENT_SECRET || true
fi
for example in $(ls -d e2e-fixtures/*)
do
    kubectl apply -k $example || true
done