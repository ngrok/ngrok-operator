#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-ingress-controller'
kubectl config set-context --current --namespace=$namespace

# TODO: Use ngrok cli api to delete all edges owned by the ingress controller

echo "~~~ Cleaning up previous deploy of examples"
for example in $(ls -d examples/*)
do
    kubectl delete -k $example --ignore-not-found --wait=false
done

echo "~~~ Cleaning up previous deploy of ngrok-ingress-controller"
make undeploy || true

# Remove finalizers from ingress in namespace
kubectl get ingress -A -o custom-columns=NAMESPACE:metadata.namespace,NAME:metadata.name --no-headers | \
while read -r i
do
  echo "kubectl get ingress -n $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -"
  kubectl get ingress -n $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -
done

kubectl delete namespace $namespace --ignore-not-found
kubectl create namespace $namespace

# TODO: Error check for auth token or api token not being set as environment variables
kubectl delete secret ngrok-ingress-controller-credentials --ignore-not-found

echo "~~~ Creating ngrok secret"
kubectl create secret generic ngrok-ingress-controller-credentials \
  --from-literal=AUTHTOKEN=$NGROK_AUTHTOKEN \
  --from-literal=API_KEY=$NGROK_API_KEY
sleep 10

echo "--- Deploying ngrok-ingress-controller"
make deploy

echo "--- Deploying examples"
for example in $(ls -d examples/*)
do
    kubectl apply -k $example
done
sleep 30

# Run tests
failed="false"
for example_config in $(find ./examples -name 'config*.yaml')
do
    expected=$(awk -F ': ' '/^# e2e-expected/ {print $2}' $example_config)

    if [[ "$expected" != "" ]]
    then
      echo "--- Testing $example_config"
      edge_fqdn=$(yq '.[0].value' $example_config)

      echo -en "Performing 'curl https://${edge_fqdn}': "

      result=$(curl -Is "https://${edge_fqdn}" | xargs -0)
      status=$(printf "${result}" | strings | awk 'NR==1{$1=$1;print}' )

      #echo -en "\tDEBUG\t expected:\"${expected}\"\n"
      #echo -en "\tDEBUG\t status:\"${status}\"\n\n"

      if [[ "$status" == "$expected" ]]
      then
        echo "Passed."
      else
        echo "FAILED!"
        echo -en "  Expected:\"${expected}\" received:\"${status}\" with:\n\n"
        echo "${result}" | sed 's/^/\t/'
        failed="true"
      fi
    fi
done

echo "--- Results"
if [[ "$failed" != "false" ]]
then
  echo "!!! Tests Failed!"
  exit 1
fi
echo "Tests Passed."