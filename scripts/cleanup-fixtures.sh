#!/usr/bin/env bash

echo "~~~ Cleaning up previous deploy of e2e-fixtures"
for example in $(ls -d e2e-fixtures/*)
do
    kubectl delete -k $example --ignore-not-found --wait=false || true
done
sleep 10