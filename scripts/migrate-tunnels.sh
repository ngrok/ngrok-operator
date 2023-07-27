#!/usr/bin/env bash

echo "~~~ Migrating tunnels"

CONTROLLER_NAMESPACE=${1:?missing controller namespace}
CONTROLLER_NAME=${2:?missing controller name}

kubectl label tunnels.ingress.k8s.ngrok.com --all --all-namespaces k8s.ngrok.com/controller-namespace=${CONTROLLER_NAMESPACE}
kubectl label tunnels.ingress.k8s.ngrok.com --all --all-namespaces k8s.ngrok.com/controller-name=${CONTROLLER_NAME}

kubectl get tunnels.ingress.k8s.ngrok.com --all-namespaces -o custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name --no-headers | while IFS= read -r tunnel; do
    NAMESPACE=$(echo $tunnel | cut -d' ' -f 1) 
    NAME=$(echo $tunnel | cut -d' ' -f 2) 
    SVC=$(kubectl get -o=jsonpath='{.spec.labels.k8s\.ngrok\.com/service}' tunnels.ingress.k8s.ngrok.com $NAME --namespace $NAMESPACE)
    PORT=$(kubectl get -o=jsonpath='{.spec.labels.k8s\.ngrok\.com/port}' tunnels.ingress.k8s.ngrok.com $NAME --namespace $NAMESPACE)

    kubectl label tunnels.ingress.k8s.ngrok.com $NAME --namespace $NAMESPACE k8s.ngrok.com/service=${SVC}
    kubectl label tunnels.ingress.k8s.ngrok.com $NAME --namespace $NAMESPACE k8s.ngrok.com/port=${PORT}
done

