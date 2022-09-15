#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-ingress-controller-different'
kubectl config set-context --current --namespace=$namespace

kubectl delete -f examples --ignore-not-found --wait=false
# Remove finalizers form ingress in namespace
for i in $(kubectl get ing -o name); do
  kubectl get $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -
done
helm uninstall ngrok-ingress-controller || true
kubectl delete namespace $namespace --ignore-not-found
kubectl create namespace $namespace

# TODO: Error check for auth token or api token not being set as environment variables
kubectl delete secret ngrok-ingress-controller-credentials --ignore-not-found
kubectl create secret generic ngrok-ingress-controller-credentials \
  --from-literal=AUTHTOKEN=$NGROK_AUTHTOKEN \
  --from-literal=API_KEY=$NGROK_API_KEY

make deploy
kubectl apply -f examples/

sleep 30

echo "Should 200"
curl -I https://minimal-ingress.ngrok.io
echo "Should 200"
curl -I https://minimal-ingress-2.ngrok.io
echo "Should   404"
curl -I https://different-ingress-class.ngrok.io
echo "Should 200"
curl -I https://ingress-class.ngrok.io
