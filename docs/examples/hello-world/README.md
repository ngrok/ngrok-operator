# Hello World - Game 2048

This tutorial will walk through setting up the ngrok Kubernetes Ingress and providing ingress to a simple 2048 game service.

## Prerequisites
- access to a remote or local kubernetes cluster such as [MiniKube](https://minikube.sigs.k8s.io/docs/start/)
- Helm 3.0.0+
- your api key and authtoken from your ngrok account

## Install the Controller

First we need to install the controller in the cluster. We'll export our credentials as environment variables and use helm to install the controller in its own namespace.


```bash
export NGROK_API_KEY=<YOUR Secret API KEY>
export NGROK_AUTHTOKEN=<YOUR Secret Auth Token>

helm repo add ngrok https://ngrok.github.io/kubernetes-ingress-controller
helm install ngrok-ingress-controller ngrok/kubernetes-ingress-controller \
  --set image.tag=0.4.0 \
  --namespace ngrok-ingress-controller \
  --create-namespace \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN
```

Verify the controller is running and healthy:

```bash
kubectl get pods -n ngrok-ingress-controller
NAME                                                              READY   STATUS    RESTARTS   AGE
ngrok-ingress-controller-kubernetes-ingress-controller-mank8zgx   1/1     Running   0          104s
```

## Setup Ingress for a Service

Next, lets deploy a simple demo service and expose it via the controller. First, we'll create a Deployment and Service for the 2048 game.

```yaml
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: game-2048
spec:
  ports:
    - name: http
      port: 80
      targetPort: 80
  selector:
    app: game-2048
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: game-2048
spec:
  replicas: 1
  selector:
    matchLabels:
      app: game-2048
  template:
    metadata:
      labels:
        app: game-2048
    spec:
      containers:
        - name: backend
          image: alexwhen/docker-2048
          ports:
            - name: http
              containerPort: 80
EOF
```

Verify the pod is running and healthy:

```bash
kubectl get pods
NAME                         READY   STATUS    RESTARTS   AGE
game-2048-6bb9fc59d9-rdpz7   1/1     Running   0          6s
```

Now we'll create an Ingress resource that will route traffic to the 2048 game service. This will just be the most basic ngrok tunnel providing access without extra features added yet.

For the `$SUBDOMAIN` variable, pick something unique that will be used for ingress to your service. This must be globally unique to be reserved in your account.

```yaml
export SUBDOMAIN="your-unique-subdomain"
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: game-2048
spec:
  ingressClassName: ngrok
  rules:
    - host: $SUBDOMAIN-game-2048.ngrok.io
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: game-2048
                port:
                  number: 80
EOF
```

Open your hostname in a browser and you should see the 2048 game running!

```bash
open https://$SUBDOMAIN-game-2048.ngrok.io
```
