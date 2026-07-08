# Events

This spec defines how the ngrok-operator emits Kubernetes Events. It is the authoritative reference for what a "good" event looks like across every controller, so that events are consistent, machine-readable, useful to end users running `kubectl describe`, and safe from spam/rate-limiting.

This is a **conventions spec** — it describes the patterns and requirements we want, not the current state. See [events-audit-plan.md](events-audit-plan.md) for the temporary remediation plan that brings the codebase into conformance.

## Why events

Events are one of three complementary observability surfaces. Each has a distinct job; do not overload one to do another's:

| Surface | Job | Audience | Lifetime |
|---------|-----|----------|----------|
| **Events** | "Something happened at a point in time" — a state transition, notable action, or failure a user should see. | Humans running `kubectl describe` / `kubectl get events`. | Ephemeral (~1h default TTL), best-effort. |
| **Status conditions** | "The current, ongoing state of this resource." Survives restarts, machine-queryable (`kubectl wait --for=condition=...`). | Humans **and** controllers. | Persistent in `.status`. |
| **Logs** | Internal operational detail for debugging the controller itself. | Operator developers. | Per log backend. |

Rule of thumb: if a user debugging their own resource with `kubectl describe` would want to know it, it's an event. If another controller or `kubectl wait` needs to poll it, it's a condition. If only we (the operator authors) need it, it's a log.

The two are not mutually exclusive — a meaningful transition often both sets a condition and emits an event. But emit the event **on the transition**, not on every reconcile that re-observes the same state.

## API

- **Use the modern Events API** (`events.k8s.io/v1`) via controller-runtime's `mgr.GetEventRecorder(name)`, which returns an `events.EventRecorder`. Do **not** use the deprecated `mgr.GetEventRecorderFor` / legacy `k8s.io/client-go/tools/record` API.
- The emit signature is:

  ```go
  Eventf(regarding runtime.Object, related runtime.Object,
         eventtype, reason, action, note string, args ...interface{})
  ```

- The `name` passed to `GetEventRecorder` becomes the event's `ReportingController` and must be a stable, kebab-case identifier for the controller (e.g. `ingress-controller`).
- RBAC must grant `create` and `patch` on `events` in the `events.k8s.io` group — `patch` is what lets the recorder coalesce recurrences into a `Series`. The same grant on core (`""`) `events` must also stay: controller-runtime's leader election (as of v0.24.x) still emits through the deprecated core-events recorder, so core events RBAC cannot be dropped even though our own code never uses the legacy API. Neither recorder needs `update`.

## Field requirements

### `regarding` / `related`

- `regarding` is the primary resource the reconciler owns — the object in `req.NamespacedName`.
- `related` is the optional secondary object involved in the action (e.g. a `Service` created for a `BoundEndpoint`). Set it whenever the event is fundamentally about the interaction between two objects; otherwise pass `nil`.
- Surface a child's problem on the parent: when a watched/owned child fails, emit the event on the parent (`regarding`) with the child as `related`, so users have one place to look.
- Do **not** emit events on objects we do not author (Nodes, Namespaces we don't own, etc.).

### `eventtype`

- Always use the `corev1.EventTypeNormal` / `corev1.EventTypeWarning` constants — never the string literals `"Normal"` / `"Warning"`.
- `Normal` = a successful or expected transition. `Warning` = a genuine problem the user may need to act on.
- Never emit `Warning` for routine, expected code paths, and never emit `Normal` for a failure.

### `reason`

- **UpperCamelCase**, short, machine-readable — imagine someone writing a `switch` over it. No spaces, dots, lowercase, or embedded runtime data.
- A **closed, stable set**. Never interpolate object names, IDs, timestamps, or error strings into a reason.
- Defined as a **named constant**, never an inline string literal. Reason constants live in a central, shared location (see below) so the whole vocabulary is visible and reviewable.
- The reason must be consistent with the event type: success reasons (`Created`, `Updated`, `Synced`) only appear on `Normal` events; failure reasons (`CreateFailed`, `SyncFailed`) only on `Warning` events. A `Warning` with reason `Created` is a bug.

### `action`

- Required and non-empty on the new API. UpperCamelCase, ≤128 chars.
- Describes **what** the controller did; `reason` describes **why / the outcome**. They must not be the same string, and `action` must not be a catch-all (`"Reconcile"` on every event defeats its purpose).
- Draw from a small, shared vocabulary of verbs, e.g. `Create`, `Update`, `Delete`, `Sync`, `Validate`, `Resolve`, `Reserve`, `Drain`.

### `note` (message)

- Human-readable, plain English, ≤1024 chars (API-enforced).
- The note is **not** part of the Series dedup key — when an event recurs, later notes are silently discarded and only the Series count bumps. Varying detail (timestamps, UIDs, full error chains) does not create extra events; it just gets lost.
- Raw `err.Error()` in the note is prohibited — it can leak internal detail, reads poorly, and any per-occurrence variation is discarded anyway (see above). Log the error at an appropriate verbosity and keep the note a short, stable summary.
- Never include secrets, tokens, or credentials — events are readable by anyone with `get events`.

## Emission cadence (anti-spam)

The modern `events.k8s.io` recorder has **no client-side spam filter** — the well-known ≈25-burst token bucket belongs to the legacy `record` API we don't use, and server-side the `EventRateLimit` admission plugin is off by default. Its only defense is dedup: recurrences identical on (type, action, reason, reportingController, reportingInstance, regarding, related) collapse into an `EventSeries`. The `note` is **not** part of that key.

The dedup key is weaker than it looks: `regarding` and `related` are compared as **full `ObjectReference` structs, including `resourceVersion` and `uid`** (client-go `tools/events/event_broadcaster.go` `getKey`, verified v0.36.1). Any write to the object — a user edit, *or our own status update* — changes `resourceVersion`, so the next emit is a brand-new Event object, not a Series bump. Verified empirically: five annotation bumps on an `NgrokTrafficPolicy` produced six distinct `PolicyDeprecation` Event objects, zero coalescing. Consequences:

- Series coalescing only helps for reconciles where the object was **not** written in between (e.g. periodic resyncs). Any reconcile-triggered-by-change that re-emits produces a new Event object.
- A controller that writes status every reconcile and also emits every reconcile gets **zero** dedup — unbounded distinct Event objects (observed: 24 Event objects on one CloudEndpoint in 2 seconds during a create-retry loop).
- A hot loop with a **varying** reason or action likewise produces unbounded distinct Event objects with zero throttling anywhere.
- Coalescing also has a **time window**: the broadcaster evicts a Series from its dedup cache after ~6 min without a recurrence. Requeue backoff caps at 1000s (16.7 min), so a *chronically failing* resource ends up minting a fresh batch of Event objects on **every** retry, forever (~14 objects/hr observed for one broken resource emitting 4 events per reconcile). Neither Series dedup nor backoff bounds a per-reconcile emit — only transition-gating does.
- `kubectl get events` counts **lag real emissions by minutes**: series increments flush on 6/30-minute heartbeats (observed 72 actual emits displayed as 18). Don't treat live counts as ground truth, and don't put per-occurrence detail in the note expecting it to be visible.

The empirically confirmed good news: a healthy resource that nobody touches emits nothing — controller-runtime's ~10h sync period plus generation/annotation predicates mean steady-state reconciles simply don't happen. Event volume is entirely a function of churn and *failing* resources, which is exactly what transition-gating addresses.
- The broadcaster queue is 1000 deep and silently drops on overflow; Series counts flush on 6/30-minute heartbeats, so `kubectl get events` counts can lag.

Discipline at the emit site is therefore the only real control. Requirements:

- **Emit on state transition, not on every reconcile.** A reconcile that changes nothing emits nothing.
- Do not emit an event on every retry of a failing operation — emit on the first failure and again on resolution.
- Never emit a per-reconcile "reconciled successfully" / "status reconciled" event unconditionally.
- Keep `reason` and `action` stable for a given situation so Series coalescing works — a dynamic reason or action defeats it entirely and multiplies Event objects.

## Coverage requirements

Every controller must give a user running `kubectl describe <resource>` a useful picture of what happened. Concretely, each reconciler should emit:

1. A `Warning` on every **user-actionable failure** — invalid spec, missing referenced resource (Secret, TrafficPolicy, Domain), failed dependency creation. These are the events users most need.
2. A `Normal` on **meaningful success transitions** — resource accepted/programmed, endpoint provisioned, first successful sync after a failure. Emit on transition, not on every steady-state reconcile.
3. Events for **lifecycle operations** whose outcome isn't otherwise visible — drain start/complete, deletion of external resources.

A controller that only sets status conditions and emits no events does not satisfy this spec. Gateway API resources (Gateway, HTTPRoute, TLSRoute, TCPRoute, GatewayClass) must emit a `Warning` on Accepted/Programmed/ResolvedRefs **failures** and a `Normal` on the first successful Programmed transition — these are the states users debug most. Note this deliberately exceeds ecosystem baseline (surveyed Gateway implementations emit nothing here and rely on conditions alone), so keep the Normals strictly transition-gated.

## Safety

- Guard against a `nil` recorder, or guarantee via construction that the recorder is always set, so a misconfigured controller degrades gracefully instead of panicking.
- Emission is async and fire-and-forget — `Eventf` returns nothing, and a rejected or dropped event surfaces only as a client log line. Failure modes that silently lose events:
  - Server-side validation: `action` and `reason` are required and ≤128 chars, `note` ≤1024 chars. A violating event is rejected by the API server, not truncated. Cover the reason/action catalog with a unit test rather than discovering this in production.
  - `ReportingInstance` is derived as recorder name + `-` + hostname and capped at 128 chars — keep recorder names short or a long pod hostname silently kills every event from that component.
  - `regarding`/`related` objects must be registered in the scheme with usable TypeMeta; if the object reference can't be built, the event is dropped.

## Checklist

Grade any new or changed event against this:

- [ ] Uses `GetEventRecorder` (new `events.k8s.io/v1` API).
- [ ] `eventtype` is a `corev1.EventType*` constant.
- [ ] `reason` is an UpperCamelCase **named constant** from the shared vocabulary; no runtime data; consistent with the event type (success reason ⇒ Normal, failure reason ⇒ Warning).
- [ ] `action` is non-empty, from the shared verb vocabulary, and distinct from `reason`.
- [ ] `note` is short, stable, plain-English, ≤1024 chars, with no raw error dumps or secrets.
- [ ] `regarding` is the owned resource; `related` set for cross-object events.
- [ ] Emitted on a transition, not on every reconcile; retries don't spam.
- [ ] User-actionable failures and meaningful successes are both covered.
- [ ] RBAC grants `create;patch` on core + `events.k8s.io` events.
