# CRDs — Common Patterns

## API Group and Version

All ngrok-operator CRDs belong to a single API group `ngrok.com` at version `v1`:

| API Group   | Version | CRDs                                                                             |
|-------------|---------|----------------------------------------------------------------------------------|
| `ngrok.com` | `v1`    | AgentEndpoint, CloudEndpoint, KubernetesOperator, TrafficPolicy, Domain, IPPolicy, BoundEndpoint |

See [migration-v1.md](../migration-v1.md) for the upgrade path from `v1alpha1` to `v1`.

## Scope

All CRDs are **Namespaced**.

## Field Naming

All JSON field names use **camelCase** (e.g., `trafficPolicy`, `resolvesTo`, `poolingEnabled`). No other conventions (snake_case, PascalCase) are permitted.

## Serialization (`omitempty`)

All fields must follow the Kubernetes API conventions for `omitempty`:

- **Optional fields** use `omitempty`. Pointer types, slices, and maps must always include `omitempty` to avoid serializing `null` or zero values.
- **Required fields** do not use `omitempty`. The field is always present in the serialized form.
- **Bools** that distinguish between unset and `false` must be pointer types with `omitempty`.

## Status Conditions

Most CRDs use `[]metav1.Condition` for status reporting with the following constraints:

- **MaxItems:** 8
- **List type:** map (keyed by `type`)

The common condition type is `Ready`, which summarizes the overall health of the resource. Individual controllers may define additional condition types specific to their resource.

**Exceptions:**
- `TrafficPolicy` does not use status conditions. It has no corresponding ngrok API resource and its validation result is surfaced via Events rather than conditions.
- `AgentEndpoint` sets a `DomainReady` condition that reflects the status of a child `Domain` resource rather than a direct ngrok API resource state.

Condition type constants must be defined in the API package, not in internal controller code. The API package is the public contract — condition types are part of that contract since users depend on them for `kubectl wait`, health checks, and GitOps tooling.

The API package must only contain API types and constants. Internal implementation details (controller interfaces, reconciliation helpers, etc.) must not be defined in the API package.

## Status `observedGeneration`

Every CRD status carries a top-level `observedGeneration` field recording the
`metadata.generation` the controller most recently reconciled.

**Why:** a status field like `Ready=True` is ambiguous on its own — right after a
user applies a spec change it may still describe the previous generation. External
tooling (`kubectl wait`, Argo CD health checks, Flux, other controllers composing on
our CRDs) compares `status.observedGeneration == metadata.generation` to decide
whether the status reflects the latest spec. It is the standard Kubernetes
convention for this (Deployments, Gateway API, cert-manager all follow it), and the
top-level field is where generic tooling looks — per-condition `observedGeneration`
alone is not enough.

**Rules:**

- Controllers stamp it on every status write for the generation they reconciled.
  In the implementation this happens centrally (`BaseController.ReconcileStatus` via
  the `ObservedGenerationSetter` interface), not per-controller.
- It is an **external contract only**. Controllers must not use it to skip
  reconciliation or ngrok API calls: `metadata.generation` only changes on spec
  writes, while ngrok-side state can drift without any generation change (dashboard
  edits, deletes, cert expiry). Reconciliation stays level-based.
- Conditions additionally carry their own per-condition `observedGeneration`, set
  automatically by the shared condition helper.

## Finalizers

The operator adds the finalizer `ngrok.com/finalizer` to resources it manages. This ensures that:

1. The operator can clean up remote ngrok API resources before the Kubernetes resource is deleted.
2. During drain, finalizers are removed to allow Kubernetes garbage collection.

## Owner References

Controllers set owner references on child resources they create. For example, the Service LoadBalancer controller sets an owner reference on created AgentEndpoint/CloudEndpoint resources pointing back to the parent Service.

## Status Reflects ngrok API State

For CRDs that correspond to ngrok API resources (AgentEndpoint, CloudEndpoint, Domain, IPPolicy, KubernetesOperator, BoundEndpoint), status fields reflect the state returned by the ngrok API after reconciliation. `TrafficPolicy` is an exception — it has no corresponding ngrok API resource and its status reflects local validation only.

## Default Field Values

Most CRDs that correspond to ngrok API resources include:

| Field         | Default Value                         |
|---------------|---------------------------------------|
| `description` | `"Created by the ngrok-operator"`     |
| `metadata`    | `{"owned-by": "ngrok-operator"}`      |

### `metadata` format

**End goal:** `metadata` is a map of string key/value pairs
(`map[string]string`) on every ngrok-backed CRD (`Domain`, `IPPolicy` and its
`rules[]`, `KubernetesOperator`, `CloudEndpoint`, `AgentEndpoint`). Users express
metadata as native YAML:

```yaml
spec:
  metadata:
    owned-by: ngrok-operator
    team: platform
```

#### How this maps to the ngrok API (and why it differs)

The ngrok API does **not** have a key/value metadata concept. Its `metadata`
field is a single **opaque free-form string** (length-capped) that ngrok stores
and returns verbatim — it never parses it. The `map[string]string` shape is an
operator-side convention layered on top: the operator serializes the map to a
**compact, key-sorted JSON-object string** and sends that as the opaque string.

Key-sorting is deliberate. The operator compares the serialized spec value
against the string ngrok has stored to decide whether an update is needed
(`endpointNeedsUpdate` and the equivalent per-controller drift checks). Go's
`json.Marshal` emits map keys in sorted order, so the same map always serializes
to the same bytes — without a stable order the comparison would flap and the
operator would issue redundant API updates on every reconcile.

A value that is neither a string nor a **flat** `string → string` object (for
example an object with nested values) is passed through to the ngrok API
verbatim rather than re-serialized, since the sorting normalization only applies
to flat maps. Nested metadata is outside the intended contract but is not
rejected.

#### Sources and precedence

The metadata on a resource comes from one of several layers depending on how the
resource was created:

1. **CRD default.** The API server applies `{"owned-by":"ngrok-operator"}` to any
   ngrok-backed CR whose `spec.metadata` is unset. This covers CRs a user authors
   directly, and also the `CloudEndpoint`/`AgentEndpoint` objects the
   **Service (LoadBalancer)** controller generates — that path does not set
   `spec.metadata`, so those objects take the default.
2. **Global operator metadata (`ngrokMetadata`).** The Helm value `ngrokMetadata`
   (a `key=value,key=value` map; deprecated alias: `metaData`) is passed to the
   operator as the `--ngrokMetadata` flag and applied to the resources the
   operator **generates from Ingress and Gateway** translation
   (`CloudEndpoint`/`AgentEndpoint`/`Domain`). It does **not** apply to
   user-authored CRs or to the Service-generated or `KubernetesOperator`
   resources.
3. **Per-CR `spec.metadata`.** A value set directly on a CR wins for that object.

#### Operator-injected metadata keys

The map is not always exactly what the user (or Helm value) supplied — the
operator injects keys in two places:

- **`owned-by`** — for Ingress/Gateway-generated resources, the operator merges
  `ngrokMetadata` and injects `owned-by` if the user did not already set it
  (`pkg/managerdriver/driver.go::setNgrokMetadataOwner`). The injected owner
  identifies the generating subsystem: `ngrok-operator` for the Ingress path,
  `kubernetes-gateway-api` for the Gateway path. This is the programmatic source
  of `owned-by` on generated objects; the identical value in the CRD default
  (layer 1) is what lands on everything else.
- **`namespace.uid`** — the `KubernetesOperator` controller injects the UID of
  its release namespace into the metadata it sends to the ngrok API, then reads
  it back to detect cluster identity. See
  [KubernetesOperator: Cluster Identity and Deduplication](../controllers/kubernetesoperator.md#cluster-identity-and-deduplication)
  and [multi-install](../features/multi-install.md).

#### Backward compatibility (migration window)

For backward compatibility the field currently also accepts a raw JSON string —
the form the operator previously required, where the object was hand-rolled into
a string (`metadata: '{"owned-by":"ngrok-operator"}'`). The string form is
**deprecated** and documented for removal at the cleanup release. Both forms
serialize to the same opaque string on the ngrok side, so switching a manifest
from the JSON-object string to the equivalent map is a no-op against the API.

During the migration window the CRD field is schemaless
(`x-kubernetes-preserve-unknown-fields`) so the API server accepts either shape;
the operator normalizes both to the same ngrok API string. The default value is still written in the
**string** form so objects that take the default remain readable by a rolled-back
prior release. In the cleanup release the field collapses to a real
`map[string]string` (`additionalProperties: {type: string}`) and the default
flips to the object form — the CRD default is preserved across the change, only
its representation changes. See
[`docs/developer-guide/passivity-shims.md`](../../docs/developer-guide/passivity-shims.md)
for the migration mechanics and [`docs/v1-migration-guide.md`](../../docs/v1-migration-guide.md)
for the user-facing timeline.

## Shared Types

### K8sObjectRef

A reference to a Kubernetes object in the same namespace.

| Field  | Type   | Required |
|--------|--------|----------|
| `name` | string | Yes      |

### K8sObjectRefOptionalNamespace

A reference to a Kubernetes object, optionally in a different namespace.

| Field       | Type    | Required |
|-------------|---------|----------|
| `name`      | string  | Yes      |
| `namespace` | *string | No       |

### TrafficPolicyCfg

Configures a traffic policy via either an inline definition or a reference to a TrafficPolicy resource. Exactly one of `inline` or `targetRef` must be specified (enforced via XValidation rules).

| Field       | Type            | Required | Description                              |
|-------------|-----------------|----------|------------------------------------------|
| `inline`    | json.RawMessage | No       | Inline traffic policy JSON (schemaless)  |
| `targetRef` | K8sObjectRef | No | Reference to a TrafficPolicy in the same namespace as the endpoint |

### ApplicationProtocol

Enum: `http1`, `http2`

### ProxyProtocolVersion

Enum: `"1"`, `"2"`
