# CRDs

Kubernetes has the concept of [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (CRDs) which allow you to define your own custom resources. The ngrok Kubernetes Ingress Controller uses CRDs internally to represent the collection of ingress objects and other k8s resources as ngrok Edges and other resources which it synchronizes to the API.

_**Warning:**_  While all resources can be accessed via the k8s API, we don't recommend editing the internal resources directly.

They are however useful to inspect and query the state of the system. This document will go over all the CRDs and note the ones that are internal implementations for now. These API differences will be more clear when the controller moves the primary resources out of alpha, and the internal CRDs remain as alpha for flexibility.

## IP Policies

The `IPPolicy` CRD manages the ngrok [API resource](https://ngrok.com/docs/api/resources/ip-policies) directly. It is a first class CRD that you can manage to control these policies in your account.

Its optional to create IP Policies this way vs using the ngrok dashboard or [terraform provider](https://registry.terraform.io/providers/ngrok/ngrok/latest/docs/resources/ip_policy). Once created though, you can use it in your ingress objects using the [annotations](./user-guide/annotations.md#ip-restriction).

| Field | Description | Required | Type | Example |
| ----- | ----------- | -------- | ---- | ------- |
| metadata | Standard object metadata | No | [metav1.ObjectMeta](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta) | `name: my-ip-policy` |
| spec.ngrokAPICommon | Fields shared across all ngrok resources | Yes | [ngrokAPICommon](#ngrokapicommon) | `{}` |
| spec.rules | A list of rules that belong to the policy | No | `[]IPPolicyRule` | `[{CIDR: "1.2.3.4", Action: "allow"}]` |
| status.ID | The unique identifier for this policy | No | `string` | `"my-ip-policy-id"` |
| status.Rules | A list of IP policy rules and their status | No | `[]IPPolicyRuleStatus` | `[{ID: "my-rule-id", CIDR: "1.2.3.4", Action: "allow"}]` |

### `IPPolicyRule`
| Field | Description | Required | Type | Example |
| ----- | ----------- | -------- | ---- | ------- |
| CIDR | The CIDR block that the rule applies to | Yes | `string` | `"1.2.3.4/24"` |
| Action | The action to take for the rule, either "allow" or "deny" | Yes | `string` | `"allow"` |

### `IPPolicyRuleStatus`
| Field | Description | Required | Type | Example |
| ----- | ----------- | -------- | ---- | ------- |
| ID | The unique identifier for this rule | No | `string` | `"my-rule-id"` |
| CIDR | The CIDR block that the rule applies to | No | `string` | `"1.2.3.4/24"` |
| Action | The action to take for the rule, either "allow" or "deny" | No | `string` | `"allow"` |


## Domains

Domains are automatically created by the controller based on the ingress objects host values. Standard ngrok subdomains will automatically be created and reserved for you. Custom domains will also be created and reserved, but will be up to you to configure the DNS records for them. See the [custom domain](./user-guide/custom-domain.md) guide for more details.

If you delete all the ingress objects for a particular host, as a saftey precaution, the ingress controller does *NOT* delete the domains and thus does not un-register them. This ensures you don't lose domains while modifying or recreating ingress objects. You can still manually delete a domain CRD via `kubectl delete domain <name>` if you want to un-register it.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| apiVersion | string | Yes | The API version for this custom resource. |
| kind | string | Yes | The kind of the custom resource. |
| metadata | [metav1.ObjectMeta](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta) | No | Standard object's metadata. More info: [https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata](https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata) |
| spec | [DomainSpec](#domainspec) | Yes | Specification of the domain. |
| status | [DomainStatus](#domainstatus) | No | Observed status of the domain. |

### DomainSpec
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| ngrokAPICommon | [ngrokAPICommon](#ngrokapicommon) | No | Common fields shared by all ngrok resources. |
| domain | string | Yes | The domain name to reserve. |
| region | string | Yes | The region in which to reserve the domain. |

### DomainStatus
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| id | string | No | The unique identifier of the domain. |
| domain | string | No | The domain that was reserved. |
| region | string | No | The region in which the domain was created. |
| uri | string | No | The URI of the reserved domain API resource. |
| cnameTarget | string | No | The CNAME target for the domain. |

## Tunnels

Tunnels are automatically created by the controller based on the ingress objects' rules' backends. A tunnel will be created for each backend service name and port combination. This results in tunnels being created with those labels which can be matched by various edge backends. Tunnels are useful to inspect but are fully managed by the controller and should not be edited directly.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| apiVersion | string | Yes | The API version for this custom resource. |
| kind | string | Yes | The kind of the custom resource. |
| metadata | [metav1.ObjectMeta](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta) | No | Standard object's metadata. More info: [https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata](https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata) |
| spec | [TunnelSpec](#tunnelspec) | Yes | Specification of the tunnel. |
| status | [TunnelStatus](#tunnelstatus) | No | Observed status of the tunnel. |

### TunnelSpec
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| forwardsTo | string | Yes | The name and port of the service to forward traffic to. |
| labels | map[string]string | No | Key/value pairs that are attached to the tunnel. |

### TunnelStatus
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| No fields defined. | | | |

## HTTPS Edges

HTTPS Edges are the primary representation of all the ingress objects and various configuration's states that will be reflected to the ngrok API. While you could create https edge CRDs directly, its not recommended because:
- the api is internal and will likely change in the future
- if your edge conflicts with any edge managed by the controller, it may be overwritten

This may stabilize to a first class CRD in the future, but for now, its not recommended to use directly but may be useful to inspect the state of the system.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| apiVersion | string | Yes | The API version for this custom resource. |
| kind | string | Yes | The kind of the custom resource. |
| metadata | [metav1.ObjectMeta](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta) | No | Standard object's metadata. More info: [https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata](https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata) |
| spec | [HTTPSEdgeSpec](#httpsedgespec) | Yes | Specification of the HTTPS edge. |
| status | [HTTPSEdgeStatus](#httpsedgestatus) | No | Observed status of the HTTPS edge. |

### HTTPSEdgeSpec
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| ngrokAPICommon | [ngrokAPICommon](#ngrokapicommon) | No | Common fields shared by all ngrok resources. |
| hostports | []string | Yes | A list of hostports served by this edge. |
| routes | []HTTPSEdgeRouteSpec | No | A list of routes served by this edge. |
| tlsTermination | [EndpointTLSTerminationAtEdge](https://ngrok.com/docs/api#type-EndpointTLSTerminationAtEdge) | No | The TLS termination configuration for this edge. |

### HTTPSEdgeRouteSpec
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| ngrokAPICommon | [ngrokAPICommon](#ngrokapicommon) | No | Common fields shared by all ngrok resources. |
| matchType | string | Yes | The type of match to use for this route. Valid values are: `exact_path` and `path_prefix`. |
| match | string | Yes | The value to match against the request path. |
| backend | [TunnelGroupBackend](https://ngrok.com/docs/api#type-TunnelGroupBackend) | Yes | The definition for the tunnel group backend that serves traffic for this edge. |
| compression | [EndpointCompression](https://ngrok.com/docs/api#type-EndpointCompression) | No | Whether or not to enable compression for this route. |
| ipRestriction | [EndpointIPPolicy](https://ngrok.com/docs/api#type-EndpointIPPolicy) | No | An IPRestriction to apply to this route. |
| headers | [EndpointHeaders](https://ngrok.com/docs/api#type-EndpointHeaders) | No | Request/response headers to apply to this route. |
| webhookVerification | [EndpointWebhookVerification](https://ngrok.com/docs/api#type-EndpointWebhookVerification) | No | Webhook verification configuration to apply to this route. |

### HTTPSEdgeRouteStatus
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| id | string | No | The unique identifier for this route. |
| uri | string | No | The URI for this route. |
| match | string | No | The value to match against the request path. |
| matchType | string | No | The type of match to use for this route. Valid values are: `exact_path` and `path_prefix`. |
| backend | [TunnelGroupBackendStatus](https://ngrok.com/docs/api#type-TunnelGroupBackendStatus) | No | Stores the status of the tunnel group backend, mainly the ID of the backend. |

### HTTPSEdgeStatus
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| id | string | No | The unique identifier for this edge. |
| uri | string | No | The URI for this edge. |
| routes | []HTTPSEdgeRouteStatus | No | A list of routes served by this edge. |

## TCP Edges

The Kubernetes ingress spec does not directly support TCP traffic. The ngrok Kubernetes Ingress Controller supports TCP traffic via the [TCP Edge](https://ngrok.com/docs/api#tcp-edge) resource. This is a first class CRD that you can manage to control these edges in your account.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| apiVersion | string | Yes | The API version for this custom resource. |
| kind | string | Yes | The kind of the custom resource. |
| metadata | [metav1.ObjectMeta](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta) | No | Standard object's metadata. More info: [https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata](https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata) |
| spec | [TCPEdgeSpec](#tcpedgespec) | Yes | Specification of the TCP edge. |
| status | [TCPEdgeStatus](#tcpedgestatus) | No | Observed status of the TCP edge. |

### TCPEdgeSpec
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| ngrokAPICommon | [ngrokAPICommon](#ngrokapicommon) | No | Common fields shared by all ngrok resources. |
| backend | [TunnelGroupBackend](#tunnelgroupbackend) | Yes | The definition for the tunnel group backend that serves traffic for this edge. |
| ipRestriction | [EndpointIPPolicy](https://ngrok.com/docs/api#type-EndpointIPPolicy) | No | An IPRestriction to apply to this route. |

### TunnelGroupBackend
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| ngrokAPICommon | [ngrokAPICommon](#ngrokapicommon) | No | Common fields shared by all ngrok resources. |
| labels | map[string]string | No | Labels to watch for tunnels on this backend. |

### TCPEdgeStatus
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| id | string | No | The unique identifier for this edge. |
| uri | string | No | The URI of the edge. |
| hostports | []string | No | Hostports served by this edge. |
| backend | [TunnelGroupBackendStatus](#tunnelgroupbackendstatus) | No | Stores the status of the tunnel group backend, mainly the ID of the backend. |

### TunnelGroupBackendStatus
| Field | Type | Required | Description |
| --- | --- | --- | --- |
| id | string | No | The unique identifier for this backend. |