#!/usr/bin/env bash

# Remove finalizers from ingress in namespace
kubectl get ingress -A -o custom-columns=NAMESPACE:metadata.namespace,NAME:metadata.name --no-headers | \
while read -r i
do
  echo "kubectl get ingress -n $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -"
  kubectl get ingress -n $i -o=json | jq '.metadata.finalizers = null' | kubectl apply -f -
done