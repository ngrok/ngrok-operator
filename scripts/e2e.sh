#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-ingress-controller'
kubectl config set-context --current --namespace=$namespace

# TODO: Use ngrok cli api to delete all edges owned by the ingress controller

echo "~~~ Validating dependencies"
for prog in jq yq
do
  PROGPATH=`which $prog || echo ''`
  if [ "${PROGPATH:-}" == "" ]
  then
    echo "Program '$prog' not found, please install it. Exiting"
    exit
  fi
done

echo "~~~ Cleaning up previous deploy of e2e-fixtures"
for example in $(ls -d e2e-fixtures/*)
do
    kubectl delete -k $example --ignore-not-found --wait=false || true
done
sleep 10

echo "~~~ Cleaning up previous deploy of ngrok-ingress-controller"
make undeploy || true

# Remove finalizers from ingress in namespace
kubectl get ingress -A -o custom-columns=NAMESPACE:metadata.namespace,NAME:metadata.name --no-headers | \
while read -r i
do
  echo "kubectl get ingress -n $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -"
  kubectl get ingress -n $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -
done

echo "--- Deploying ngrok-ingress-controller"
make deploy

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

echo "--- Waiting for deployment"
sleep 120

exec scripts/postflight.sh
