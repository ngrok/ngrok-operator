# Controllers — Common Behavior

## Base Controller Pattern

All ngrok-operator controllers use `BaseController[T]`, a generic base type that implements the standard reconciliation pattern.

## Reconciliation Flow

```
Fetch object
  → Object not found? → Done (no-op)
  → DeletionTimestamp set? → Delete handler → Remove finalizer
  → Draining? → Skip (no-op)
  → StatusID empty? → Create handler → Update status
  → StatusID present? → Update handler → Update status
```

## Finalizers

The operator uses the finalizer `k8s.ngrok.com/finalizer` on all managed resources. The lifecycle is:

1. **Create/Update**: Finalizer is added via `util.RegisterAndSyncFinalizer()`.
2. **Delete (with finalizer)**: Delete handler runs (cleans up remote resources), then finalizer is removed via `util.RemoveAndSyncFinalizer()`.
3. **Delete (without finalizer)**: Delete handler is skipped; Kubernetes deletes the resource immediately.

## Drain Awareness

During drain (see [features/draining.md](../features/draining.md)):

- Non-delete reconciles are skipped entirely to prevent adding new finalizers.
- Delete reconciles proceed normally to allow cleanup.
- Controllers check `DrainState.IsDraining()` at the start of each reconcile.

## StatusID

Controllers define a `StatusID` function that returns a remote resource identifier (typically `Status.ID`):

- **Empty**: Triggers the Create path.
- **Non-empty**: Triggers the Update path.

## Status Updates

`ReconcileStatus()` provides conflict-aware status updates:

1. Wraps the update in `retry.RetryOnConflict()`.
2. On conflict, re-fetches the latest `resourceVersion` and retries.
3. Emits a `Status` event on success.
4. Emits a `StatusError` event on failure.
5. Returns a `StatusError` type that wraps the original error.

## Error Handling

`CtrlResultForErr()` maps errors to requeue behavior:

| Error Type                | Behavior                    |
|---------------------------|-----------------------------|
| Server errors (5xx)       | Return error (requeue with backoff) |
| 429 TooManyRequests       | Requeue after 1 minute      |
| 404 NotFound              | Return error                |
| Client errors (4xx)       | No requeue                  |
| EndpointDenied            | No requeue (handled by poller) |
| Domain not ready          | Requeue after 10 seconds    |
| Status update conflict    | Requeue after 10 seconds    |
| CSR creation failure      | Requeue after 30 seconds    |
| Service creation failure  | Requeue after 1 minute      |

## Predicates

Most controllers use `AnnotationChangedPredicate` OR `GenerationChangedPredicate` to filter unnecessary reconciles. This means:

- Changes to annotations trigger reconciliation.
- Changes to `spec` fields trigger reconciliation (via generation increment).
- Changes to status or other metadata generally do not trigger reconciliation.
