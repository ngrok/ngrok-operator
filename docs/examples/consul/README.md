# Ingress into Consul Service Mesh on Minikube

This tutorial will guide you through the process of installing the ngrok Kubernetes Ingress Controller into a local Minikube cluster running a Consul Service Mesh. We will first follow the Consul Minikube setup guide, and then install the ngrok Kubernetes Ingress Controller to provide ingress to the Demo Counter Application.

## Prerequisites
- your api key and authtoken from your ngrok account
- Helm 3.0.0+
- [MiniKube](https://minikube.sigs.k8s.io/docs/start/) 1.22+
- docker 20.0+
- consul 1.14.0+


## Initial Setup: Minikube and Consul

As we are integrating with Consul to provide ingress to our services within its service mesh, we first need to set up a local Minikube cluster and install Consul into it. Hashicorp has provided a comprehensive guide for this [here](https://developer.hashicorp.com/consul/tutorials/kubernetes/kubernetes-minikube). Follow this setup guide until the end, where you can access the Dashboard on localhost via port-forwarding. Once that is working, we are ready to make it accessible to our friends and family!

[Minikube and Consul Setup Guide](https://developer.hashicorp.com/consul/tutorials/kubernetes/kubernetes-minikube)

## Installing the Ingress Controller
First, we need to install the controller in the cluster.

Create a `values.yaml` file with the following values
```yaml
image:
  tag: 0.4.0
podAnnotations:
  consul.hashicorp.com/connect-inject: "true"
  # This is the CIDR of your Kubernetes API: `kubectl get svc kubernetes --output jsonpath='{.spec.clusterIP}'
  consul.hashicorp.com/transparent-proxy-exclude-outbound-cidrs: "10.96.0.1/32"
```

Next we'll export our credentials as environment variables and install the controller with Helm:

```bash
export NGROK_API_KEY=<YOUR Secret API KEY>
export NGROK_AUTHTOKEN=<YOUR Secret Auth Token>

helm install ngrok-ingress-controller ngrok/kubernetes-ingress-controller --version 0.6.0 \
  --namespace default \
  --set credentials.apiKey=$NGROK_API_KEY \
  --set credentials.authtoken=$NGROK_AUTHTOKEN
```


At this point, the ngrok ingress controller pods may not be running yet. This is because Consul requires a Kubernetes service that selects the pods to allow them to join the service mesh. For most ingress controllers that act as a load balancer service, this is required to route traffic to them in the first place. Ngrok works a bit differently, though. Since we establish an outbound tunnel from the controller to our edge servers that traffic routes through, we don't technically need any Kubernetes service as it has no directly accessible endpoints. However, to fit in the service mesh, we will create a service to target our pods:


```yaml
apiVersion: v1
kind: Service
metadata:
  name: ngrok-ingress-controller-kubernetes-ingress-controller
  namespace: default
  labels:
    app: ngrok-ingress-controller-kubernetes-ingress-controller
spec:
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app.kubernetes.io/name: kubernetes-ingress-controller
```

Now we can verify the controller is running and healthy:

```bash
kubectl get pods -l 'app.kubernetes.io/name=kubernetes-ingress-controller' -n default

NAME                                                              READY   STATUS    RESTARTS      AGE
ngrok-ingress-controller-kubernetes-ingress-controller-manqwlhz   2/2     Running   2 (93s ago)   2m17s
```

Setting Up Ingress for the Demo Counter Application
With the controller running, we can set up ingress for the Demo Counter Application. Since we are in the Consul service mesh, the first thing we'll have to do is allow fine-grained access from the ingress controller to the application. We'll do this by creating a Consul Service Intention that allows the ingress controller to access the application:


```yaml
apiVersion: consul.hashicorp.com/v1alpha1
kind: ServiceIntentions
metadata:
  name: ngrok-ingress-controller-kubernetes-ingress-controller
  namespace: default
spec:
  destination:
    name: dashboard
  sources:
  - action: allow
    name: ngrok-ingress-controller-kubernetes-ingress-controller
```


Now we can create the ingress resource for the dashboard application. This ingress resource will create a route to the dashboard application at the root path:


```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ingress-consul
  namespace: default
spec:
  ingressClassName: ngrok
  rules:
  - host: YOUR-CUSTOM-SUBDOMAIN-consul-ngrok-demo.ngrok.app
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: dashboard
            port:
              number: 80
```


Once applied, your domain should be available almost instantaneously! Open your hostname in a browser and you should see the Consul counter application.

```bash
open YOUR-CUSTOM-SUBDOMAIN-consul-ngrok-demo.ngrok.app
```

Stay tuned and next week we'll expose the Consul Dashboard protected by google oauth!
