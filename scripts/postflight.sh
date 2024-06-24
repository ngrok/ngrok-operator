#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-operator'
kubectl config set-context --current --namespace=$namespace

# Run tests
echo "--- Running e2e tests"
failed="false"
for e2e_config in $(find ./e2e-fixtures -name 'e2e-expected.yaml')
do
  example_dir=$(dirname $e2e_config)
  for config_file in $(cat $e2e_config | yq -r '(. | keys)[]')
  do
    if [ -f "$example_dir/$config_file" ]
    then
      example_config="$example_dir/$config_file"
      edge_fqdn=$(yq -r '.[0].value' $example_config)

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
