# ngrok Region

ngrok runs globally distributed tunnel servers around the world to enable fast, low latency traffic to your applications.
See https://ngrok.com/docs/network-edge/#points-of-presence for more information on ngrok's regions.

Similar to the agent, if you do not explicitly pick a region via helm when installing the controller, the controller will attempt to pick the region with the least latency, which is usually the one geographically closest to your machine.

See the [helm value `region`](https://github.com/ngrok/kubernetes-ingress-controller/blob/main/helm/ingress-controller/README.md#controller-parameters) to configure a specific region for the controller to use.
