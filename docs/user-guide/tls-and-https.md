# TLS and HTTPS

For http based traffic, the ngrok Kubernetes Ingress Controller will and can only provide HTTPS secured traffic. This is because the controller is responsible for creating the ngrok tunnel and edge, and ngrok only supports HTTPS for http traffic. By default if you use a standard ngrok subdomain, all traffic will be over https. If you are using a custom domain, please see the [custom domain](./custom-domain.md) documentation for more details.

Additionally, [TLS Edges](https://ngrok.com/docs/api/resources/edges-tls) may be supported soon in the future!
