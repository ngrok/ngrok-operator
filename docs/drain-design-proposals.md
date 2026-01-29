# Drain System Design Proposals

> **Note:** Design Proposal B (DrainOrchestrator) was implemented. See the implementation in:
> - `internal/drain/orchestrator.go` - The Orchestrator implementation
> - `internal/drain/state.go` - StateChecker with monotonic `SetDraining()`
> - `internal/controller/ngrok/kubernetesoperator_controller.go` - Simplified controller
> - `cmd/api-manager.go` - Wiring of the Orchestrator

---

## Current State Analysis

### Components and Their Responsibilities

**1. StateChecker** (`internal/drain/state.go`)
- Constructor: `NewStateChecker(client, operatorNamespace, operatorConfigName)`
- Purpose: Detect if operator is draining by querying KubernetesOperator CR
- Methods: `IsDraining(ctx)`, `SetDraining(bool)`
- Used by: All controllers across all 3 pods

**2. Drainer** (`internal/drain/drain.go`)
- Constructor: Created inline in controller with `&drain.Drainer{Client, Log, Policy, WatchNamespace}`
- Purpose: Loop over resources and remove finalizers / delete CRs
- Methods: `DrainAll(ctx) (*DrainResult, error)`
- Used by: Only KubernetesOperatorReconciler

**3. drainstate.State interface** (`internal/drainstate/drainstate.go`)
- Read-only interface for `IsDraining(ctx) bool`
- Re-exported by multiple packages

### Current Flow in KubernetesOperatorReconciler

```
Reconcile()
  └─> Check DrainState.IsDraining()
      └─> handleDrain()
          ├─> DrainState.SetDraining(true)     // Propagate to other controllers
          ├─> Update ko.Status.DrainStatus     // Set to "draining"
          ├─> Create Drainer inline
          ├─> drainer.DrainAll()
          ├─> Update ko.Status based on result
          ├─> If complete + KO being deleted:
          │   ├─> Delete from ngrok API
          │   └─> Remove finalizer
          └─> Return result
```

### Problems with Current Design

1. **Mixed concerns in controller**: The controller handles status updates, event recording, drain orchestration, AND ngrok API cleanup. It's doing too much.

2. **Leaky abstraction**: `*StateChecker` passed to controller to call `SetDraining()`, but this method isn't on the `State` interface.

3. **`SetDraining(bool)` is a footgun**: Takes bool but should be monotonic.

4. **Drainer constructed inline**: Mixes wiring (what client/namespace) with business logic.

5. **Different constructor parameters**: StateChecker needs `(client, namespace, releaseName)`, Drainer needs `(client, log, policy, watchNamespace)`. No natural shared construction.

6. **Who owns status updates?**: Currently controller does, but this spreads drain logic across two places.

---

## Design Proposal A: Minimal Cleanup (Conservative)

Keep the current structure but fix the obvious issues.

### Changes

1. **Rename `SetDraining(bool)` → `SetDraining()`** - Make it monotonic, no parameter
2. **Add `Triggerable` interface** - Explicit interface for the one controller that triggers drain
3. **Keep Drainer as-is** - It's only used in one place, construction inline is fine

### Interface Design

```go
// internal/drainstate/drainstate.go
type State interface {
    IsDraining(ctx context.Context) bool
}

// internal/drain/state.go  
type Triggerable interface {
    State
    SetDraining()  // Monotonic - once called, always draining
}

// StateChecker implements both
var _ Triggerable = (*StateChecker)(nil)
```

### Wiring

```go
// cmd/api-manager.go
stateChecker := drain.NewStateChecker(client, namespace, releaseName)

// Pass as Triggerable to KubernetesOperatorReconciler
&KubernetesOperatorReconciler{
    DrainState: stateChecker,  // type: drain.Triggerable
}

// Pass as State to other controllers  
&CloudEndpointReconciler{
    DrainState: stateChecker,  // type: drainstate.State (implicit conversion)
}

// cmd/agent-manager.go - only needs read-only
stateChecker := drain.NewStateChecker(client, namespace, releaseName)
// Pass as drainstate.State everywhere
```

### Pros
- Minimal change
- Clear that only `Triggerable` holder can trigger drain
- Other pods still just use `State`

### Cons
- Controller still orchestrates everything
- Status update logic still in controller
- Drainer still constructed inline

---

## Design Proposal B: DrainOrchestrator (Encapsulated Drain Workflow)

Extract all drain orchestration into a single object that the controller delegates to.

### Key Insight

The controller shouldn't need to know about:
- Status field names (`DrainStatus`, `DrainMessage`, `DrainProgress`)
- The specific drain workflow steps
- How to propagate drain state to other controllers

It should just say "handle drain for this KO" and get back a result.

### New Component: DrainOrchestrator

```go
// internal/drain/orchestrator.go

type Orchestrator struct {
    client         client.Client
    recorder       record.EventRecorder
    ngrokClientset ngrokapi.Clientset
    log            logr.Logger
    stateChecker   *StateChecker
    watchNamespace string
}

type OrchestratorConfig struct {
    Client         client.Client
    Recorder       record.EventRecorder
    NgrokClientset ngrokapi.Clientset
    Log            logr.Logger
    Namespace      string      // Operator namespace
    ReleaseName    string      // KubernetesOperator CR name
    WatchNamespace string      // Namespace to drain (empty = all)
}

func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator

// State returns the read-only drain state for other controllers
func (o *Orchestrator) State() drainstate.State

// HandleDrain performs the full drain workflow for the given KO.
// It updates status, runs the drainer, and returns whether drain is complete.
// The caller is responsible for finalizer removal after HandleDrain returns complete.
func (o *Orchestrator) HandleDrain(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) (DrainOutcome, error)

type DrainOutcome int
const (
    DrainInProgress DrainOutcome = iota
    DrainComplete
    DrainFailed
)
```

### Controller Changes

```go
// internal/controller/ngrok/kubernetesoperator_controller.go

type KubernetesOperatorReconciler struct {
    client.Client
    // ... other fields ...
    DrainOrchestrator *drain.Orchestrator
}

func (r *KubernetesOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ko := &ngrokv1alpha1.KubernetesOperator{}
    if err := r.Client.Get(ctx, req.NamespacedName, ko); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check if draining using orchestrator's state
    if r.DrainOrchestrator.State().IsDraining(ctx) {
        return r.handleDrain(ctx, ko)
    }
    return r.controller.Reconcile(ctx, req, new(ngrokv1alpha1.KubernetesOperator))
}

func (r *KubernetesOperatorReconciler) handleDrain(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) (ctrl.Result, error) {
    outcome, err := r.DrainOrchestrator.HandleDrain(ctx, ko)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    switch outcome {
    case drain.DrainInProgress:
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    case drain.DrainComplete:
        // Orchestrator already updated status to "completed"
        // Controller handles finalizer removal (KO lifecycle)
        if !ko.DeletionTimestamp.IsZero() {
            return r.cleanupAndRemoveFinalizer(ctx, ko)
        }
        return ctrl.Result{}, nil
    case drain.DrainFailed:
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }
    return ctrl.Result{}, nil
}
```

### Wiring in cmd/api-manager.go

```go
orchestrator := drain.NewOrchestrator(drain.OrchestratorConfig{
    Client:         mgr.GetClient(),
    Recorder:       mgr.GetEventRecorderFor("drain-orchestrator"),
    NgrokClientset: ngrokClientset,
    Log:            ctrl.Log.WithName("drain"),
    Namespace:      opts.namespace,
    ReleaseName:    opts.releaseName,
    WatchNamespace: opts.ingressWatchNamespace,
})

// Pass State() to other controllers
&CloudEndpointReconciler{
    DrainState: orchestrator.State(),
}

// Pass full orchestrator to KubernetesOperatorReconciler
&KubernetesOperatorReconciler{
    DrainOrchestrator: orchestrator,
}
```

### Other Pods (agent-manager, bindings-forwarder)

These don't need the orchestrator, just the state checker:

```go
stateChecker := drain.NewStateChecker(client, namespace, releaseName)
// Use as drainstate.State
```

### Pros
- Clear separation: Orchestrator owns drain workflow, Controller owns KO lifecycle
- Controller becomes simpler - just delegates to orchestrator
- Status update logic encapsulated
- Easier to test drain workflow in isolation
- Single place to modify drain behavior

### Cons
- More abstraction
- Orchestrator needs access to client, recorder, ngrokClientset
- Two objects still needed (Orchestrator for api-manager, StateChecker for other pods)

---

## Design Proposal C: Functional Composition (Middle Ground)

Keep Drainer simple, but extract status management into a helper.

### Key Insight

The issue isn't that the controller orchestrates - that's appropriate. The issue is that status update code is verbose and interleaved with logic.

### Changes

1. **Add status helper methods to KubernetesOperator type**
2. **Make Drainer injectable via factory**
3. **Keep StateChecker simple with monotonic `SetDraining()`**

### Status Helpers on CRD Type

```go
// api/ngrok/v1alpha1/kubernetesoperator_types.go

func (ko *KubernetesOperator) SetDrainStarting() {
    ko.Status.DrainStatus = DrainStatusDraining
    ko.Status.DrainMessage = "Drain in progress"
}

func (ko *KubernetesOperator) SetDrainProgress(result *drain.DrainResult) {
    ko.Status.DrainProgress = result.Progress()
    ko.Status.DrainErrors = result.ErrorStrings()
    if result.HasErrors() {
        ko.Status.DrainMessage = fmt.Sprintf("Drain completed with %d errors", result.Failed)
    } else if !result.IsComplete() {
        ko.Status.DrainMessage = "Drain in progress"
    }
}

func (ko *KubernetesOperator) SetDrainCompleted() {
    ko.Status.DrainStatus = DrainStatusCompleted
    ko.Status.DrainMessage = "Drain completed successfully"
    ko.Status.DrainErrors = nil
}

func (ko *KubernetesOperator) SetDrainFailed(err error) {
    ko.Status.DrainStatus = DrainStatusFailed
    ko.Status.DrainMessage = fmt.Sprintf("Drain failed: %v", err)
}
```

### Drainer Factory

```go
// internal/drain/drain.go

type DrainerFactory func(log logr.Logger, ko *ngrokv1alpha1.KubernetesOperator) *Drainer

func NewDrainerFactory(client client.Client, watchNamespace string) DrainerFactory {
    return func(log logr.Logger, ko *ngrokv1alpha1.KubernetesOperator) *Drainer {
        return &Drainer{
            Client:         client,
            Log:            log,
            Policy:         ko.GetDrainPolicy(),
            WatchNamespace: watchNamespace,
        }
    }
}
```

### Simplified Controller

```go
func (r *KubernetesOperatorReconciler) handleDrain(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator, log logr.Logger) (ctrl.Result, error) {
    r.DrainState.SetDraining()
    log.Info("Starting drain process")

    if ko.Status.DrainStatus != ngrokv1alpha1.DrainStatusDraining {
        ko.SetDrainStarting()
        if err := r.Client.Status().Update(ctx, ko); err != nil {
            return ctrl.Result{}, err
        }
        r.Recorder.Event(ko, v1.EventTypeNormal, "DrainStarted", "Starting drain")
    }

    drainer := r.DrainerFactory(log, ko)
    result, err := drainer.DrainAll(ctx)
    if err != nil {
        ko.SetDrainFailed(err)
        r.Client.Status().Update(ctx, ko)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    ko.SetDrainProgress(result)
    
    if result.HasErrors() {
        r.Client.Status().Update(ctx, ko)
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    if !result.IsComplete() {
        r.Client.Status().Update(ctx, ko)
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    ko.SetDrainCompleted()
    r.Client.Status().Update(ctx, ko)
    r.Recorder.Event(ko, v1.EventTypeNormal, "DrainCompleted", "All resources drained")

    if !ko.DeletionTimestamp.IsZero() {
        return r.cleanupAndRemoveFinalizer(ctx, ko)
    }
    return ctrl.Result{}, nil
}
```

### Pros
- Minimal new abstractions
- Status logic on the type where it belongs
- Factory enables testing
- Controller still clearly orchestrates

### Cons
- Status methods on CRD type might feel wrong to some
- Still need two separate constructions (StateChecker + DrainerFactory)

---

## Recommendation

**Proposal B (DrainOrchestrator)** for these reasons:

1. **Clearest separation of concerns**: The controller manages KO lifecycle (finalizers, ngrok API deletion). The orchestrator manages drain workflow (status, events, calling drainer).

2. **Testable**: You can test the orchestrator's drain workflow independently of the controller.

3. **Encapsulation**: If drain behavior changes, you modify one place.

4. **Natural boundary**: The orchestrator exposes `State()` for read-only consumers, and `HandleDrain()` for the one controller that needs it.

5. **Other pods unchanged**: They still just create a simple StateChecker.

### Proposed Package Layout

```
internal/
├── drainstate/
│   └── drainstate.go        # State interface + helpers (unchanged)
├── drain/
│   ├── state.go             # StateChecker (read-only check + cache)
│   ├── drainer.go           # Drainer (loops over resources)
│   ├── orchestrator.go      # NEW: Orchestrator (full workflow)
│   └── result.go            # DrainResult type
```

### Open Questions

1. Should the orchestrator also handle ngrok API deletion of the KubernetesOperator, or should that stay in the controller?
   - **Recommendation**: Keep in controller. That's KO lifecycle, not drain workflow.

2. Should DrainResult be returned from HandleDrain for the controller to inspect, or should it be fully opaque?
   - **Recommendation**: Return an outcome enum + optional error. Controller doesn't need internals.

3. Should the orchestrator own the Recorder, or receive events to record?
   - **Recommendation**: Own it. Events are part of drain workflow observability.

---

## Next Steps

1. Review this document and choose a proposal
2. If Proposal B, I'll implement the Orchestrator
3. Update controllers to use new design
4. Update cmd files for wiring
5. Add/update tests
