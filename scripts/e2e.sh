#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-ingress-controller'
kubectl config set-context --current --namespace=$namespace

# TODO: Use ngrok cli api to delete all edges owned by the ingress controller

echo "~~~ Cleaning up previous deploy of examples"
for example in $(ls -d examples/*)
do
    kubectl delete -k $example --ignore-not-found --wait=false || true
done
sleep 5

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
    kubectl apply -k $example || true
done
sleep 30

# Run tests
echo "--- Running e2e tests"
failed="false"
for e2e_config in $(find ./examples -name 'e2e-expected.yaml')
do
  example_dir=$(dirname $e2e_config)
  for config_file in $(cat $e2e_config | yq -r '(. | keys)[]')
  do
    if [ -f "$example_dir/$config_file" ]
    then
      example_config="$example_dir/$config_file"
      edge_fqdn=$(yq '.[0].value' $example_config)

      echo "--- Testing '$example_config' with Edge '$edge_fqdn'"
      for test_path in $(yq -r "(.\"$config_file\".paths | keys)[]" $e2e_config)
      do
        expected=$(yq -r ".\"$config_file\".paths[\"$test_path\"]" $e2e_config)
        test_uri="https://${edge_fqdn}${test_path}"
        result=$(curl -Is $test_uri | xargs -0)
        status=$(printf "${result}" | strings | awk 'NR==1{$1=$1;print}')

        printf "\tTesting '${test_uri}' expecting '${expected}': "
        if [[ "$status" == "$expected" ]]
        then
          echo "Passed."
        else
          echo "FAILED!"
          echo -en "  Expected:\"${expected}\" received:\"${status}\" with:\n\n"
          echo "${result}" | sed 's/^/\t/'
          failed="true"
        fi
      done
    fi
  done
done

echo "--- Results"
if [[ "$failed" != "false" ]]
then
  echo "!!! Tests Failed!"
  exit 1
fi
echo "Tests Passed."