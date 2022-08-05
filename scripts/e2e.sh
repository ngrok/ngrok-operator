#!/usr/bin/env bash

set -eu -o pipefail

kill $(lsof -t -i:4040) || true
namespace='ngrok-ingress-controller'
kubectl delete namespace $namespace --ignore-not-found
kubectl create namespace $namespace

# TODO: Error check for auth token or api token not being set as environment variables
kubectl create secret generic ngrok-ingress-controller-credentials --from-literal=AUTHTOKEN=$NGROK_AUTHTOKEN --from-literal=API_KEY=$NGROK_API_KEY

make deploy
kubectl apply -n $namespace -f examples/

sleep 30

aPod=$(kubectl get pods -l control-plane=controller-manager -o json -n $namespace | jq -r '.items[0].metadata.name')
kubectl port-forward $aPod 4040:4040 &
sleep 5

aDomain=$(curl localhost:4040/api/tunnels | jq -r '.tunnels[0].public_url')
curl $aDomain
