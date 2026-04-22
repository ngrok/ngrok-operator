# Service LoadBalancer Controller Specification

## Executive Summary

The Service LoadBalancer controller reconciles Kubernetes Services of type `LoadBalancer` with the ngrok load balancer class
(`loadBalancerClass: ngrok`). It materializes these Services in ngrok endpoint Custom Resources (CloudEndpoint and/or AgentEndpoint), applies TrafficPolicy configurations, and updates Service status with the externally reachable address (hostname/port).

## Features

### Common Behavior

The ngrok Service LoadBalancer controller will only manage Services(`corev1.Service`) that meet the following criteria:
- `spec.type: LoadBalancer`
- `spec.loadBalancerClass: ngrok`

If these criteria are not met, the controller will clean up any previously created ngrok endpoints and remove its finalizer from the Service.

When managing a qualifying Service, the controller will:
1. Add a finalizer(`ngrok.com/finalizer`) to the Service to ensure proper cleanup on deletion.
2. Create and manage a single `CloudEndpoint` and/or `AgentEndpoint` resource based on the mapping strategy. An owner reference to the Service will be set on the created endpoint(s).
3. If the traffic-policy annotation is present, resolve the traffic policy and apply it to the created endpoint(s).
4. Update the Service's `status.loadBalancer.ingress` field with the externally reachable hostname and port.

### TCP Load Balancers

TCP Load Balancer is the default behavior. When the `ngrok.com/url` annotation is not specified, or it specifies a `tcp://` scheme,
the controller will create a TCP Load Balancer.

### TLS Termination

When a Service specifies a domain or url with the `tls://` scheme, the controller will create a TLS-terminated load balancer.


### Annotations

#### `ngrok.com/mapping-strategy`

Allowed values: `endpoints`, `endpoints-verbose`
Default Value: `endpoints`

When unspecified, it defaults to `endpoints` and only an `AgentEndpoint` will be created for the Service.
When set to `endpoints-verbose`, both a `CloudEndpoint` and an internal `AgentEndpoint`, an endpoint with a url ending in `.internal` will be created for the Service.

#### `ngrok.com/url`

Examples:
* `ngrok.com/url: "tcp://1.tcp.ngrok.io:12345"` - Creates a TCP load balancer using the specified ngrok TCP address. It must be reserved in the ngrok dashboard/API first.
* `ngrok.com/url: "tcp://"` - Creates a TCP load balancer using a dynamically assigned ngrok TCP address.
* `ngrok.com/url: "tls://example.com"` - Creates a TLS-terminated load balancer for the specified domain.

#### `ngrok.com/traffic-policy`

Specifies the name of a `TrafficPolicy` resource in the same namespace to apply to the created endpoint(s).
The controller will watch for changes to the referenced `TrafficPolicy` and update the endpoint(s) accordingly.

When the mapping strategy is `endpoints-verbose`, the traffic policy will be applied to the `CloudEndpoint`.
When the mapping strategy is `endpoints`, the traffic policy will be applied to the `AgentEndpoint`.

#### `ngrok.com/pooling-enabled`

Controls whether the endpoint allows pooling with other endpoints sharing the same URL.

Allowed values: `"true"`, `"false"`
Default: (none — uses ngrok platform default)

When set, the value is passed through to the created `CloudEndpoint`'s `spec.poolingEnabled` field (for `endpoints-verbose` strategy) or to the `AgentEndpoint` (for `endpoints` strategy).

#### `ngrok.com/description`

Sets a human-readable description on the created ngrok endpoint resource(s).

Default: `"Created by the ngrok-operator"`

#### `ngrok.com/metadata`

Sets arbitrary key-value metadata on the created ngrok endpoint resource(s). Value is a JSON object string that is parsed into `map[string]string`. Merged with operator-level ``ngrok.metadata``; annotation keys take precedence on conflict.

Default: `{"owned-by": "ngrok-operator"}`

#### `ngrok.com/bindings`

Controls traffic visibility for the endpoint. Comma-separated list of binding types.

Allowed values: `public`, `internal`, `kubernetes`
Default: (none — uses ngrok platform default)

#### `ngrok.com/computed-url` (internal)

This annotation is set by the controller and serves as the single source of truth for the externally reachable URL of the load balancer. The controller uses this annotation to populate the Service's `status.loadBalancer.ingress` field.

**TCP Load Balancers:**
- When the URL annotation is unset or `tcp://`, the controller reserves a TCP address via the ngrok API and sets the computed-url to the assigned address (e.g., `tcp://5.tcp.ngrok.io:12345`).
- When the URL annotation specifies a pre-reserved TCP address (e.g., `tcp://1.tcp.ngrok.io:12345`), the computed-url is set to that address.

**TLS Load Balancers:**
- The computed-url is set to the value from the `ngrok.com/url` annotation (e.g., `tls://example.ngrok.app:443` or `tls://custom.example.com:443`).

### Service Status

The controller updates the Service's `status.loadBalancer.ingress` field to provide users with the externally reachable address for their load balancer.

#### TCP Load Balancers

For TCP load balancers, the status hostname and port are extracted directly from the `computed-url` annotation:

```yaml
status:
  loadBalancer:
    ingress:
    - hostname: 5.tcp.ngrok.io
      ports:
      - port: 12345
        protocol: TCP
```

#### TLS Load Balancers

For TLS load balancers, the controller **must wait** for the endpoint's `status.domainRef` to be populated before setting the Service status. This is because TLS endpoints are associated with a Domain CRD that contains the authoritative hostname information.

The flow is:
1. Service controller creates CloudEndpoint/AgentEndpoint with the TLS URL
2. CloudEndpoint/AgentEndpoint controller reconciles, creates/references the Domain, and sets `status.domainRef`
3. Service controller watches for status changes on owned endpoints (via `ResourceVersionChangedPredicate`)
4. When `domainRef` is set, Service controller looks up the Domain CRD to determine the correct hostname

**While waiting for domainRef:** The Service status remains empty (no ingress entries).

**Once domainRef is available:**

For **ngrok-managed domains** (e.g., `*.ngrok.app`, `*.ngrok.io`), the Domain CRD will not have a `cnameTarget`, so the hostname is taken directly from the domain:

```yaml
# With annotation: ngrok.com/url: "tls://myapp.ngrok.app:443"
# Domain CRD has: status.domain: "myapp.ngrok.app", status.cnameTarget: null
status:
  loadBalancer:
    ingress:
    - hostname: myapp.ngrok.app
      ports:
      - port: 443
        protocol: TCP
```

For **custom domains** (e.g., `app.example.com`), the Domain CRD will have a `cnameTarget` that users must configure in their DNS. The status hostname is set to this CNAME target, not the custom domain itself:

```yaml
# With annotation: ngrok.com/url: "tls://app.example.com:443"
# Domain CRD has: status.domain: "app.example.com", status.cnameTarget: "abc123.ngrok-cname.com"
status:
  loadBalancer:
    ingress:
    - hostname: abc123.ngrok-cname.com  # CNAME target, NOT app.example.com
      ports:
      - port: 443
        protocol: TCP
```

This allows users to:
1. See the CNAME target they need to configure in their DNS provider
2. Create a CNAME record: `app.example.com -> abc123.ngrok-cname.com`

#### domainRef Dependency

The `domainRef` field on CloudEndpoint/AgentEndpoint status is critical for TLS endpoints:

| Endpoint Type | domainRef Required | Status Behavior |
|--------------|-------------------|-----------------|
| TCP (`tcp://`) | No | Hostname from computed-url directly |
| TLS (`tls://`) | Yes | Wait for domainRef, then use Domain's cnameTarget or domain |

The Service controller watches for status changes on owned CloudEndpoint/AgentEndpoint resources. When the endpoint controller sets `status.domainRef`, the Service controller re-reconciles and populates the Service status with the correct hostname.

### Special Cases

When an eligible Service has no ports defined, the controller will emit a warning event and will not create any endpoints.
