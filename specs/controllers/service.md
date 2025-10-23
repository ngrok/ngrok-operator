# Service LoadBalancer Controller Specification

## Executive Summary

The Service LoadBalancer controller reconciles Kubernetes Services of type `LoadBalancer` with the ngrok load balancer class
(`loadBalancerClass: ngrok`). It materializes these Services in ngrok endpoint Custom Resources (CloudEndpoint and/or AgentEndpoint), applies NgrokTrafficPolicy configurations, and updates Service status with the externally reachable address (hostname/port).

## Features

### Common Behavior

The ngrok Service LoadBalancer controller will only manage Services(`corev1.Service`) that meet the following criteria:
- `spec.type: LoadBalancer`
- `spec.loadBalancerClass: ngrok`

If these criteria are not met, the controller will clean up any previously created ngrok endpoints and remove its finalizer from the Service.

When managing a qualifying Service, the controller will:
1. Add a finalizer(`k8s.ngrok.com/finalizer`) to the Service to ensure proper cleanup on deletion.
2. Create and manage a single `CloudEndpoint` and/or `AgentEndpoint` resource based on the mapping strategy. An owner reference to the Service will be set on the created endpoint(s).
2. If the traffic-policy annotation is present, resolve the traffic policy and apply it to the created endpoint(s).
3. Update the Service's `status.loadBalancer.ingress` field with the externally reachable hostname and port.

### TCP Load Balancers

TCP Load Balancer is the default behavior. When the domain annotation is not specified, or the `k8s.ngrok.com/url` annotation specifies a `tcp://` scheme,
the controller will create a TCP Load Balancer.

### TLS Termination

When a Service specifies a domain or url with the `tls://` scheme, the controller will create a TLS-terminated load balancer.


### Annotations

#### `k8s.ngrok.com/domain` (deprecated)

Signifies intent to create a TLS-terminated load balancer with the specified domain.

#### `k8s.ngrok.com/mapping-strategy`

Allowed values: `endpoints`, `endpoints-verbose`
Default Value: `endpoints`

When unspecified, it defaults to `endpoints` and only an `AgentEndpoint` will be created for the Service.
When set to `endpoints-verbose`, both a `CloudEndpoint` and an internal `AgentEndpoint`, an endpoint with a url ending in `.internal` will be created for the Service.

#### `k8s.ngrok.com/url`

This replaces the deprecated `k8s.ngrok.com/domain` annotation.

Examples:
* `k8s.ngrok.com/url: "tcp://1.tcp.ngrok.io:12345"` - Creates a TCP load balancer using the specified ngrok TCP address. It must be reserved in the ngrok dashboard/API first.
* `k8s.ngrok.com/url: "tcp://"` - Creates a TCP load balancer using a dynamically assigned ngrok TCP address.
* `k8s.ngrok.com/url: "tls://example.com"` - Creates a TLS-terminated load balancer for the specified domain.

#### `k8s.ngrok.com/traffic-policy`

Specifies the name of a `NgrokTrafficPolicy` resource in the same namespace to apply to the created endpoint(s).
The controller will watch for changes to the referenced `NgrokTrafficPolicy` and update the endpoint(s) accordingly.

When the mapping strategy is `endpoints-verbose`, the traffic policy will be applied to the `CloudEndpoint`.
When the mapping strategy is `endpoints`, the traffic policy will be applied to the `AgentEndpoint`.

#### `k8s.ngrok.com/computed-url` (internal)

This annotation is set by the controller to reflect the actual externally reachable URL of the load balancer.
In the case of TCP load balancers with dynamically assigned addresses, this annotation will contain the assigned ngrok TCP address.

### Special Cases

When an eligible Service has no ports defined, the controller will emit a warning event and will not create any endpoints.
