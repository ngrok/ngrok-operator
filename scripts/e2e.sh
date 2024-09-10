#!/usr/bin/env bash

set -eu -o pipefail

namespace='ngrok-operator'
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

./scripts/cleanup-fixtures.sh

echo "~~~ Cleaning up previous deploy of ngrok-operator"
make undeploy || true

./scripts/remove-finalizers.sh

echo "--- Deploying ngrok-operator"
make deploy

./scripts/create-fixtures.sh

echo "--- Waiting for deployment"
sleep 120

exec scripts/postflight.sh
