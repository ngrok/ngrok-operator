# Conditions in ngrok-operator

Kubernetes conditions describe the current observed state of resources and are fundamental to providing users with clear visibility into resource status and debugging information.

## Condition Philosophy

- **Conditions are observations, not state machines** - Multiple conditions can coexist
- **Events capture history, conditions capture current state** - Use events for debugging, conditions for status
- **"Abnormal-True" polarity preferred** - Conditions should indicate exceptional states (prefer `Invalid` over `Valid`)
- **Ready condition aggregates overall health** - The most important condition for users and monitoring

## Standard Condition Types

### Core Conditions (All Resources)
- **`Ready`** - Overall readiness indicator, aggregates other conditions
- **`Reconciling`** - Currently processing changes toward desired state

### Resource-Specific Conditions
- **`DomainReady`** - Domain is reserved and ready (DNS + certificates provisioned)
- **`EndpointCreated`** - Endpoint successfully created in ngrok API
- **`TrafficPolicyApplied`** - Traffic policy configuration applied successfully
- **`CertificateReady`** - TLS certificates provisioned (Domain resources)
- **`DNSConfigured`** - DNS records properly configured (Domain resources)

## Unified Conditions System

### Architecture

The ngrok-operator uses a centralized conditions system to eliminate duplication and ensure consistency:

```
internal/
├── conditions/           # Centralized condition management
│   ├── types.go         # Condition types and reason constants
│   ├── manager.go       # Fluent conditions API
│   └── helpers.go       # Composite condition patterns
├── util/
│   ├── domains.go       # Shared domain condition logic
│   └── traffic_policy.go # Shared traffic policy logic
```

### Conditions Manager

Controllers use a fluent `conditions.Manager` for consistent condition updates:

```go
func (r *CloudEndpointReconciler) update(ctx context.Context, obj *v1alpha1.CloudEndpoint) error {
    cm := conditions.NewManager(&obj.Status.Conditions)
    cm.SetReconciling("Processing CloudEndpoint changes")

    var err error
    defer func() {
        // Single status update per reconcile
        _ = r.controller.ReconcileStatus(ctx, obj, err)
    }()

    // Domain dependency
    if err = util.EnsureDomain(ctx, r.Client, r.Recorder, obj, obj.Spec.URL, cm); err != nil {
        return err // cm already set appropriate conditions
    }

    // Traffic policy resolution
    policy, err := util.ResolveTrafficPolicy(ctx, r.Client, obj.Namespace, obj.Spec.TrafficPolicy, cm)
    if err != nil {
        return err // cm already set appropriate conditions
    }

    // Endpoint creation
    result, err := r.createEndpoint(ctx, obj, policy)
    cm.ApplyEndpointResult(result, err, policy != "")

    return err
}
```

### Composite Condition Patterns

Following the Domain controller pattern, use composite helpers that set multiple related conditions:

```go
// ✅ Good - Single call sets related conditions
cm.SetDomainWaiting("Domain provisioning in progress")
// Sets: DomainReady=False + Ready=False

cm.SetTrafficPolicyError("Invalid policy configuration")
// Sets: TrafficPolicyApplied=False + Ready=False

cm.SetEndpointSuccess("Endpoint active and serving traffic")
// Sets: EndpointCreated=True + Ready=True + TrafficPolicyApplied=True

// ❌ Avoid - Multiple individual condition calls
cm.SetCondition("DomainReady", metav1.ConditionFalse, ...)
cm.SetCondition("Ready", metav1.ConditionFalse, ...)
```

## Implementation Best Practices

### Controller Patterns

1. **Single Status Update Per Reconcile**
   - Use deferred `ReconcileStatus()` call to commit all condition changes once
   - Avoids multiple API calls and potential conflicts

2. **Shared Logic for Common Dependencies**
   - Domain creation/checking: `util.EnsureDomain()`
   - Traffic policy resolution: `util.ResolveTrafficPolicy()`
   - Eliminates code duplication between AgentEndpoint and CloudEndpoint

3. **Consistent Error Handling**
   - Sentinel errors for requeue scenarios (`ErrDomainCreating`)
   - BaseController `ErrResult` handles error-to-requeue mapping
   - Conditions set before returning errors

### CRD Annotations

Use standard Kubernetes condition annotations:

```go
type MyResourceStatus struct {
    // Conditions describe the current state of the resource
    //
    // +kubebuilder:validation:Optional
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

**Note:** Avoid `MaxItems` restrictions on conditions - they limit future extensibility.

### Condition Messages

- **Ready=False**: Show the most actionable blocking issue
- **Be specific**: Include resource names, error details, expected vs actual state
- **Actionable**: Help users understand what they need to do

```go
// ✅ Good messages
"Domain 'api.example.com' certificate provisioning failed: DNS validation timeout"
"TrafficPolicy 'rate-limit-policy' not found in namespace 'default'"
"Endpoint created successfully, serving traffic at https://api.example.com"

// ❌ Poor messages
"Something went wrong"
"Error occurred"
"Not ready"
```

## Monitoring and Observability

### kubectl Integration

Conditions appear in `kubectl describe` output and can be accessed via JSONPath:

```bash
# Check overall readiness
kubectl get cloudendpoints -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}'

# Get condition details
kubectl describe cloudendpoint my-endpoint
```

### Metrics and Alerting

Monitor condition states for operational visibility:

```promql
# Resources not ready
sum(kube_customresource_conditions{type="Ready",status="false"}) by (customresource_kind)

# Resources stuck reconciling
sum(kube_customresource_conditions{type="Ready",status="false",reason="DomainCreating"})
```

## Testing Conditions

Test condition behavior comprehensively:

```go
func TestConditionProgression(t *testing.T) {
    // Initial state
    assert.True(t, meta.IsStatusConditionPresentAndEqual(obj.Status.Conditions, "Ready", metav1.ConditionFalse))

    // After domain ready
    reconcile(t, obj)
    assert.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, "DomainReady"))

    // After endpoint creation
    reconcile(t, obj)
    assert.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, "Ready"))
}
```

This unified approach ensures consistent user experience, reduces maintenance overhead, and follows Kubernetes best practices across all ngrok-operator resources.


https://maelvls.dev/kubernetes-conditions/