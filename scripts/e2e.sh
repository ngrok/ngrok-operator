#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-ingress-controller'
kubectl delete deployment ngrok-ingress-controller-manager -n $namespace --ignore-not-found
# Remove finalizers form ingress in namespace
for i in $(kubectl get ing -o name); do
  kubectl get $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -
done
kubectl delete namespace $namespace --ignore-not-found
kubectl create namespace $namespace

# TODO: Error check for auth token or api token not being set as environment variables
kubectl create secret generic ngrok-ingress-controller-credentials --from-literal=AUTHTOKEN=$NGROK_AUTHTOKEN --from-literal=API_KEY=$NGROK_API_KEY

make deploy
kubectl apply -n $namespace -f examples/

sleep 30

curl -I https://minimal-ingress.ngrok.io
curl -I https://minimal-ingress-2.ngrok.io
