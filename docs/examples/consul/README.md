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
