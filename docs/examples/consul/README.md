# Hello World - Game 2048

This tutorial will walk through setting up the ngrok Kubernetes Ingress and providing ingress to a simple 2048 game service.

## Prerequisites
- access to a remote or local kubernetes cluster such as [MiniKube](https://minikube.sigs.k8s.io/docs/start/)
- Helm 3.0.0+
- your api key and authtoken from your ngrok account

## Install the Controller

First we need to install the controller in the cluster. We'll export our credentials as environment variables and use helm to install the controller in its own namespace.

```
helm install --values values.yaml consul hashicorp/consul --create-namespace --namespace consul --version "1.0.0"

helm repo add ngrok https://ngrok.github.io/kubernetes-ingress-controller
helm install ngrok-ingress-controller ngrok/kubernetes-ingress-controller \
  --set image.tag=0.4.0 \
  --namespace default \
  --set-string podAnnotations.consul\\.hashicorp\\.com/connect-inject=true \
  --set podAnnotations."consul\.hashicorp\.com/transparent-proxy-exclude-outbound-cidrs"="10.96.0.1/32" \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN




```bash
export NGROK_API_KEY=<YOUR Secret API KEY>
export NGROK_AUTHTOKEN=<YOUR Secret Auth Token>

helm repo add ngrok https://ngrok.github.io/kubernetes-ingress-controller
helm install ngrok-ingress-controller ngrok/kubernetes-ingress-controller \
  --set image.tag=0.4.0 \
  --namespace default \
  --set-string podAnnotations.consul\\.hashicorp\\.com/connect-inject=true \
  --set podAnnotations."consul\.hashicorp\.com/transparent-proxy-exclude-outbound-cidrs"="10.96.0.1/32" \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN
```

Verify the controller is running and healthy:

```bash
    app.kubernetes.io/name: MyApp
ngrok-ingress-controller
NAME                                                              READY   STATUS    RESTARTS   AGE
ngrok-ingress-controller-kubernetes-ingress-controller-mank8zgx   1/1     Running   0          104s
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













  # the CIDR of your Kubernetes API: `kubectl get svc kubernetes --output jsonpath='{.spec.clusterIP}'

# Setup Guide with Consul

Include the following set annotations in your helm cli install command

```bash
  --set podAnnotations."consul\.hashicorp\.com/connect-inject"="\"true\"" \
  # the CIDR of your Kubernetes API: `kubectl get svc kubernetes --output jsonpath='{.spec.clusterIP}'
  --set podAnnotations."consul\.hashicorp\.com/transparent-proxy-exclude-outbound-cidrs"="10.96.0.1/32" \
```

or to your values.yaml file

```yaml
podAnnotations:
  consul.hashicorp.com/connect-inject: "true"
  # And the CIDR of your Kubernetes API: `kubectl get svc kubernetes --output jsonpath='{.spec.clusterIP}'
  consul.hashicorp.com/transparent-proxy-exclude-outbound-cidrs: "10.108.0.1/32"
```

https://developer.hashicorp.com/consul/docs/k8s/connect/ingress-controllers

<img src="../../assets/images/Under-Construction-Sign.png" alt="Under Construction" width="350" />
