#!/usr/bin/env bash

echo "~~~ Migrating https edges"

CONTROLLER_NAMESPACE=${1:?missing controller namespace}
CONTROLLER_NAME=${2:?missing controller name}

kubectl label httpsedges.ingress.k8s.ngrok.com --all --all-namespaces k8s.ngrok.com/controller-namespace=${CONTROLLER_NAMESPACE}
kubectl label httpsedges.ingress.k8s.ngrok.com --all --all-namespaces k8s.ngrok.com/controller-name=${CONTROLLER_NAME}

kubectl get httpsedges.ingress.k8s.ngrok.com --all-namespaces -o custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name --no-headers | while IFS= read -r edge; do
    NAMESPACE=$(echo $edge | cut -d' ' -f 1) 
    NAME=$(echo $edge | cut -d' ' -f 2) 
    HOSTPORT=$(kubectl get -o=jsonpath='{.spec.hostports[0]}' httpsedges.ingress.k8s.ngrok.com $NAME --namespace $NAMESPACE | cut -d':' -f 1)
    kubectl label httpsedges.ingress.k8s.ngrok.com $NAME --namespace $NAMESPACE k8s.ngrok.com/domain=${HOSTPORT}
done

