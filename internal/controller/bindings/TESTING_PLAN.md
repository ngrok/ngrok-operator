# BoundEndpoint Controller Testing Plan

## Overview

This plan outlines the testing strategy for the BoundEndpoint controller and poller after the status improvements described in PLAN.md. The main challenges are:

1. **Dual reconciliation model**: Both a poller (syncing from ngrok API) and a controller (managing K8s services) modify the same CRD
2. **Removed fields**: Status/ErrorCode/ErrorMessage removed from BindingEndpoint type
3. **New fields**: Conditions, service refs, EndpointsSummary added to BoundEndpointStatus
4. **Bug fix**: Testing that provisioning status doesn't get stuck when endpoints are added

## Testing Strategy

### High-Level Approach

We'll use a **hybrid testing strategy**:

1. **Env tests** (primary): Integration-style tests using controller-runtime's envtest framework
   - Tests the full controller + poller interaction
   - Uses real Kubernetes API (test environment)
   - Mocks only the ngrok API
   - Better coverage of actual runtime behavior

2. **Unit tests** (secondary): Direct function testing for complex logic
   - Keep existing useful unit tests from boundendpoint_poller_test.go
   - Add new unit tests for condition calculation logic
   - Test edge cases and branching logic

### Env Test Architecture

Following the pattern from `internal/controller/agent/suite_test.go` and `internal/controller/ingress/suite_test.go`:

#### Suite Setup (`suite_test.go`)

```go
// Uses Ginkgo/Gomega testing framework
var (
    cfg       *rest.Config
    k8sClient client.Client
    testEnv   *envtest.Environment
    envMgr    ctrl.Manager
    envCtx    context.Context
    envCancel context.CancelFunc

    // Mock ngrok API client
    mockNgrokClient *ngrokapi.MockClientset
)

BeforeSuite:
- Start envtest (real K8s API server)
- Register BoundEndpoint CRD
- Create controller manager
- Register BoundEndpointReconciler with mock ngrok client
- Register BoundEndpointPoller with mock ngrok client
- Start manager in goroutine
- Wait for cache sync

AfterSuite:
- Cancel context
- Stop envtest
```

#### Poller Integration Challenge

**Problem**: The poller runs on a loop with `PollingInterval`. In tests, we don't want to wait for actual polling intervals.

**Solution Options**:

**Option A (Recommended): Controlled Poller Execution**
- Create a test-specific poller interface that allows manual triggering
- In suite setup, use a very long PollingInterval (e.g., 1 hour) so it doesn't auto-trigger
- Expose a method like `TriggerPoll(ctx)` that directly calls the reconciliation logic
- Tests call `TriggerPoll()` explicitly, then use `Eventually()` to wait for controller reconciliation

```go
// In test setup:
testPoller := &BoundEndpointPoller{
    Client: envMgr.GetClient(),
    // ... other fields
    PollingInterval: 1 * time.Hour, // effectively disabled
}

// In tests:
testPoller.TriggerPoll(ctx) // Manually trigger one poll cycle
Eventually(func(g Gomega) {
    // Wait for controller to reconcile
}).Should(Succeed())
```

**Option B: Short Polling Interval**
- Use a very short PollingInterval (e.g., 100ms) in tests
- Let poller run naturally
- Tests just set up mock ngrok responses and wait with `Eventually()`
- Simpler but less deterministic, may be slower

**Recommendation**: Use **Option A** for more deterministic tests. We can add a `TriggerPoll()` method to the poller that's only used in tests, or expose the internal reconciliation function.

## Test Structure

### File Organization

```
internal/controller/bindings/
├── suite_test.go                          # NEW: Env test setup
├── boundendpoint_controller_test.go       # REWRITE: Env tests for controller
├── boundendpoint_poller_test.go           # UPDATE: Keep unit tests, add env tests
├── boundendpoint_conditions_test.go       # NEW: Unit tests for condition helpers
└── TESTING_PLAN.md                        # This file
```

### Unit Tests to Keep (boundendpoint_poller_test.go)

These test pure functions with no external dependencies:

✅ **KEEP** (update for API changes):
- `Test_BoundEndpointPoller_filterBoundEndpointActions` - Update to remove Status/ErrorCode/ErrorMessage assertions
- `Test_BoundEndpointPoller_boundEndpointNeedsUpdate` - Update to remove Status/ErrorCode/ErrorMessage from test fixtures
- `Test_BoundEndpointPoller_hashURI` - No changes needed
- `Test_BoundEndpointPoller_targetMetadataIsEqual` - No changes needed

✅ **ADD NEW**:
- `Test_computeEndpointsSummary` - Test "1 endpoint" vs "N endpoints" logic

### Unit Tests to Add (boundendpoint_conditions_test.go)

Test the condition helper functions:

✅ **NEW TESTS**:
- `Test_calculateReadyCondition` - Test Ready condition derivation from other conditions
- `Test_setServicesCreatedCondition` - Test setting ServicesCreated condition
- `Test_setConnectivityVerifiedCondition` - Test setting ConnectivityVerified condition

### Env Tests to Write

#### Phase 1: Basic Functionality

Focus: Prove the system works end-to-end with minimal test cases

**Test 1: Single Endpoint Happy Path**
```ginkgo
It("should create BoundEndpoint with services and conditions", func(ctx) {
    // 1. Mock ngrok API to return one endpoint
    mockNgrokClient.SetEndpoints(map[string]ngrokapi.AggregatedEndpoint{
        "http://service.namespace:8080": {
            Endpoints: []ngrok.Ref{{ID: "ep_123", URI: "https://example.ngrok.io"}},
            Target: {...},
        },
    })

    // 2. Trigger poller (creates BoundEndpoint)
    testPoller.TriggerPoll(ctx)

    // 3. Wait for BoundEndpoint to exist
    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        // Poller sets these:
        g.Expect(be.Status.Endpoints).To(HaveLen(1))
        g.Expect(be.Status.Endpoints[0].ID).To(Equal("ep_123"))
        g.Expect(be.Status.EndpointsSummary).To(Equal("1 endpoint"))
    }).Should(Succeed())

    // 4. Wait for controller to create services and set conditions
    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        // Controller sets these:
        cond := findCondition(be.Status.Conditions, ConditionServicesCreated)
        g.Expect(cond).NotTo(BeNil())
        g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))

        g.Expect(be.Status.TargetServiceRef).NotTo(BeNil())
        g.Expect(be.Status.UpstreamServiceRef).NotTo(BeNil())
    }).Should(Succeed())

    // 5. Verify services actually exist
    targetSvc := &v1.Service{}
    err := k8sClient.Get(ctx, types.NamespacedName{
        Name: "service",
        Namespace: "namespace",
    }, targetSvc)
    Expect(err).NotTo(HaveOccurred())
})
```

**Test 2: Multiple Endpoints to Same Target**
```ginkgo
It("should aggregate multiple endpoints into one BoundEndpoint", func(ctx) {
    // Mock 2 endpoints pointing to same service
    mockNgrokClient.SetEndpoints(map[string]ngrokapi.AggregatedEndpoint{
        "http://service.namespace:8080": {
            Endpoints: []ngrok.Ref{
                {ID: "ep_123", URI: "https://example1.ngrok.io"},
                {ID: "ep_456", URI: "https://example2.ngrok.io"},
            },
            Target: {...},
        },
    })

    testPoller.TriggerPoll(ctx)

    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        g.Expect(be.Status.Endpoints).To(HaveLen(2))
        g.Expect(be.Status.EndpointsSummary).To(Equal("2 endpoints"))
    }).Should(Succeed())
})
```

#### Phase 2: Bug Fix Validation

Focus: Verify the bug fix described in PLAN.md

**Test 3: Status Not Stuck in Provisioning**
```ginkgo
It("should not get stuck in provisioning when adding endpoints", func(ctx) {
    // 1. Create initial BoundEndpoint with 1 endpoint
    mockNgrokClient.SetEndpoints(map[string]ngrokapi.AggregatedEndpoint{
        "http://service.namespace:8080": {
            Endpoints: []ngrok.Ref{{ID: "ep_123"}},
            Target: {...},
        },
    })

    testPoller.TriggerPoll(ctx)

    // 2. Wait for Ready condition
    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        cond := findCondition(be.Status.Conditions, ConditionReady)
        g.Expect(cond).NotTo(BeNil())
        g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
    }).Should(Succeed())

    // 3. Add a second endpoint (simulates launching another agent)
    mockNgrokClient.SetEndpoints(map[string]ngrokapi.AggregatedEndpoint{
        "http://service.namespace:8080": {
            Endpoints: []ngrok.Ref{
                {ID: "ep_123"},
                {ID: "ep_789"}, // NEW endpoint
            },
            Target: {...},
        },
    })

    testPoller.TriggerPoll(ctx)

    // 4. Verify Ready condition stays True (not reset to provisioning)
    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        g.Expect(be.Status.Endpoints).To(HaveLen(2))

        // KEY: Ready condition should still be True
        cond := findCondition(be.Status.Conditions, ConditionReady)
        g.Expect(cond).NotTo(BeNil())
        g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
    }).Should(Succeed())
})
```

#### Phase 3: New Features

Focus: Test the new status fields and conditions

**Test 4: Conditions Progression**
```ginkgo
It("should show condition progression from creation to ready", func(ctx) {
    mockNgrokClient.SetEndpoints(map[string]ngrokapi.AggregatedEndpoint{
        "http://service.namespace:8080": {
            Endpoints: []ngrok.Ref{{ID: "ep_123"}},
            Target: {...},
        },
    })

    testPoller.TriggerPoll(ctx)

    // Eventually all conditions should be True
    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        servicesCreated := findCondition(be.Status.Conditions, ConditionServicesCreated)
        g.Expect(servicesCreated).NotTo(BeNil())
        g.Expect(servicesCreated.Status).To(Equal(metav1.ConditionTrue))

        connectivityVerified := findCondition(be.Status.Conditions, ConditionConnectivityVerified)
        g.Expect(connectivityVerified).NotTo(BeNil())
        g.Expect(connectivityVerified.Status).To(Equal(metav1.ConditionTrue))

        ready := findCondition(be.Status.Conditions, ConditionReady)
        g.Expect(ready).NotTo(BeNil())
        g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
    }).Should(Succeed())
})
```

**Test 5: Service Creation Failure**
```ginkgo
It("should set ServicesCreated condition to False on error", func(ctx) {
    // Don't create target namespace - this will cause service creation to fail
    mockNgrokClient.SetEndpoints(map[string]ngrokapi.AggregatedEndpoint{
        "http://service.missing-namespace:8080": {
            Endpoints: []ngrok.Ref{{ID: "ep_123"}},
            Target: {Namespace: "missing-namespace", ...},
        },
    })

    testPoller.TriggerPoll(ctx)

    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        cond := findCondition(be.Status.Conditions, ConditionServicesCreated)
        g.Expect(cond).NotTo(BeNil())
        g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
        g.Expect(cond.Reason).To(Equal(ReasonServiceCreationFailed))
        g.Expect(cond.Message).To(ContainSubstring("namespace"))

        // Ready should also be False
        ready := findCondition(be.Status.Conditions, ConditionReady)
        g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
    }).Should(Succeed())
})
```

**Test 6: Service References Populated**
```ginkgo
It("should populate service references in status", func(ctx) {
    mockNgrokClient.SetEndpoints(...)
    testPoller.TriggerPoll(ctx)

    Eventually(func(g Gomega) {
        be := &bindingsv1alpha1.BoundEndpoint{}
        err := k8sClient.Get(ctx, types.NamespacedName{...}, be)
        g.Expect(err).NotTo(HaveOccurred())

        g.Expect(be.Status.TargetServiceRef).NotTo(BeNil())
        g.Expect(be.Status.TargetServiceRef.Name).To(Equal("service"))
        g.Expect(*be.Status.TargetServiceRef.Namespace).To(Equal("namespace"))

        g.Expect(be.Status.UpstreamServiceRef).NotTo(BeNil())
        g.Expect(be.Status.UpstreamServiceRef.Name).To(MatchRegexp("^ngrok-"))
    }).Should(Succeed())
})
```

#### Phase 4: Edge Cases (Optional - Can defer)

These test cases are lower priority and can be added later:

- Updating BoundEndpoint spec (metadata changes)
- Connectivity test failures
- Service deletion detection and recreation
- Multiple BoundEndpoints in different namespaces
- Port allocation and conflicts

## Implementation Plan

### Step 1: Update Existing Unit Tests ✅
- Fix `boundendpoint_poller_test.go` to remove Status/ErrorCode/ErrorMessage references
- Add `Test_computeEndpointsSummary` unit test
- Ensure these pass with new API types

### Step 2: Create Suite Setup 🔧
- Create `suite_test.go` following agent/suite_test.go pattern
- Set up mock ngrok client
- Register both controller and poller with test manager
- Implement poller trigger mechanism (Option A)

### Step 3: Phase 1 Tests 🧪
- Test 1: Single endpoint happy path
- Test 2: Multiple endpoints to same target
- Verify basic functionality works

### Step 4: Phase 2 Tests 🐛
- Test 3: Not stuck in provisioning
- Verify bug fix works

### Step 5: Phase 3 Tests ✨
- Tests 4-6: Conditions and new features
- Verify new status fields work correctly

### Step 6: Remove Old Controller Tests 🗑️
- Delete minimal tests in `boundendpoint_controller_test.go` that tested removed functions
- Keep only `Test_convertBoundEndpointToServices` if still relevant

## Testing Helpers Needed

```go
// Helper to find a condition by type
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
    for i, c := range conditions {
        if c.Type == condType {
            return &conditions[i]
        }
    }
    return nil
}

// Random string for test namespace generation
func RandomString(n int) string {
    // ... implementation
}
```

## Success Criteria

✅ All unit tests pass
✅ All Phase 1 env tests pass (basic functionality)
✅ All Phase 2 env tests pass (bug fix validated)
✅ All Phase 3 env tests pass (new features validated)
✅ No test flakiness from poller timing
✅ Tests run in <30s total

## Out of Scope

The following are explicitly **NOT** covered by this testing plan:

- Testing unchanged poller logic (port allocation, URI hashing, metadata comparison) beyond what's needed to verify our changes
- 100% code coverage - we focus on testing our changes
- Performance/load testing with many BoundEndpoints
- Network-level connectivity testing (the connectivity check is mocked)
- RBAC permission testing (requires complex envtest setup)

## Open Questions

1. **Mock ngrok client interface**: Do we already have a mock implementation we can use? Or do we need to create one?
   - Check `internal/ngrokapi/` for existing mocks
   - May need to create `MockClientset` with `SetEndpoints()` method

2. **Poller trigger mechanism**: Should we expose `TriggerPoll()` as a public method, or keep it test-only?
   - Recommendation: Add a build tag `// +build testing` or use interface approach

3. **Test isolation**: How do we prevent tests from interfering with each other?
   - Use unique namespaces per test (already done in agent tests)
   - Reset mock client state in BeforeEach

4. **Test naming**: Use Ginkgo/Gomega (BDD style) or traditional Go tests?
   - Recommendation: Use Ginkgo/Gomega to match existing controller tests
   - Allows for better test organization and Eventually/Consistently helpers

## Timeline Estimate

- Step 1 (Update unit tests): **30 minutes**
- Step 2 (Suite setup): **1 hour**
- Step 3 (Phase 1 tests): **1 hour**
- Step 4 (Phase 2 tests): **1 hour**
- Step 5 (Phase 3 tests): **1 hour**
- Step 6 (Cleanup): **15 minutes**

**Total: ~4-5 hours** for comprehensive test coverage of the changes.
