# CRDs

Kubernetes has the concept of [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (CRDs) which allow you to define your own custom resources. This document will cover the CRDs you might use to achieve your goals with the ngrok Kubernetes Ingress Controller.

_**Warning:**_  There are other CRDs not documented here that are used internally by the controller. It is not recomended to edit these, but inspecting them to query the state of the system could be useful at times. See the [internal CRDs](../developer-guide/internal-crds.md) document for more details.

## Ngrok Module Sets

`NgrokModuleSets` is a CRD that lets you define combinations of ngrok modules that can be set on your ingress objects and applied to all of their routes. For an in-depth guide on configuring `NgrokModuleSets` see the [Route Modules Guide](./route-modules.md).

### NgrokModuleSetModules

| Field | Type | Description |
| --- | --- | --- |
| `compression` | EndpointCompression | Configuration for compression for this module |
| `headers` | EndpointHeaders | Configuration for headers for this module |
| `ipRestriction` | EndpointIPPolicy | Configuration for IP restriction for this module |
| `tlsTermination` | EndpointTLSTerminationAtEdge | Configuration for TLS termination for this module |
| `webhookVerification` | EndpointWebhookVerification | Configuration for webhook verification for this module |

### NgrokModuleSet

| Field | Type | Description |
| --- | --- | --- |
| `apiVersion` | string | API version of the `NgrokModuleSet` custom resource definition |
| `kind` | string | Kind of the custom resource definition |
| `metadata` | ObjectMeta | Standard Kubernetes metadata |
| `modules` | NgrokModuleSetModules | The set of modules for this custom resource definition |

### EndpointCompression

| Field | Type | Description |
| --- | --- | --- |
| `enabled` | boolean | Whether or not compression is enabled for this endpoint |

### EndpointIPPolicy

| Field | Type | Description |
| --- | --- | --- |
| `ippolicies` | []string | List of IP policies for this endpoint |

### EndpointRequestHeaders

| Field | Type | Description |
| --- | --- | --- |
| `add` | map[string]string | Map of header key to header value that will be injected into the HTTP Request |
| `remove` | []string | List of header names that will be removed from the HTTP Request |

### EndpointResponseHeaders

| Field | Type | Description |
| --- | --- | --- |
| `add` | map[string]string | Map of header key to header value that will be injected into the HTTP Response |
| `remove` | []string | List of header names that will be removed from the HTTP Response |

### EndpointHeaders

| Field | Type | Description |
| --- | --- | --- |
| `request` | EndpointRequestHeaders | Configuration for request headers for this endpoint |
| `response` | EndpointResponseHeaders | Configuration for response headers for this endpoint |

### EndpointTLSTerminationAtEdge

| Field | Type | Description |
| --- | --- | --- |
| `minVersion` | string | Minimum TLS version to allow for connections to the edge |

### SecretKeyRef

| Field | Type | Description |
| --- | --- | --- |
| `name` | string | Name of the Kubernetes secret |
| `key` | string | Key in the secret to use |

### EndpointWebhookVerification

| Field | Type | Description |
| --- | --- | --- |
| `provider` | string | String indicating which webhook provider will be sending webhooks to this endpoint |
| `secret` | SecretKeyRef | Reference to a secret containing the secret used to validate requests from the given provider |


## IP Policies

The `IPPolicy` CRD manages the ngrok [API resource](https://ngrok.com/docs/api/resources/ip-policies) directly. It is a first class CRD that you can manage to control these policies in your account.

Its optional to create IP Policies this way vs using the ngrok dashboard or [terraform provider](https://registry.terraform.io/providers/ngrok/ngrok/latest/docs/resources/ip_policy). Once created though, you can use it in your ingress objects using the [annotations](./annotations.md#ip-restriction).

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


## TCP Edges

The Kubernetes ingress spec does not directly support TCP traffic. The ngrok Kubernetes Ingress Controller supports TCP traffic via the [TCP Edge](https://ngrok.com/docs/api#tcp-edge) resource. This is a first class CRD that you can manage to control these edges in your account. This is in progress and not yet fully supported. Check back soon for updates to the [TCP Edge Guide](./tcp-edge.md).

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