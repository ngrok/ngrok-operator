# Internal CRDs

Kubernetes has the concept of [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (CRDs) which allow you to define your own custom resources. This document covers the CRDs created and managed by the controller internally to manage the state of the system across various controller components. It's generally unsafe to modify these directly and would likely result in strange effects as the controller fights you. They are useful however to inspect the state of the system and to debug issues.

## HTTPS Edges

HTTPS Edges are the primary representation of all the ingress objects and various configuration's states that will be reflected to the ngrok API. While you could create https edge CRDs directly, it's not recommended because:
- the api is internal and will likely change in the future
- if your edge conflicts with any edge managed by the controller, it may be overwritten

This may stabilize to a first class CRD in the future, but for now, it's not recommended to use directly but may be useful to inspect the state of the system.

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