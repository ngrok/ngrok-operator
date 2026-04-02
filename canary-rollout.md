Design Doc: ngrok Progressive Delivery — Full Roadmap
Status: Draft
Scope: Full evolution from a minimal Argo Rollouts plugin to first-class ngrok Operator progressive delivery support.

Background and Motivation
The ngrok Kubernetes Operator translates Ingress resources into CloudEndpoint and AgentEndpoint CRDs. CloudEndpoints are persistent, globally distributed edge configurations managed via the ngrok API. AgentEndpoints are in-memory tunnel sessions managed by the operator's agent deployment — they are not sidecars, not tied to pod count, and each CR creates one tunnel session regardless of how many pods back the upstream Service.
Users want progressive delivery (blue/green, canary) with ngrok as their ingress. Customers are already attempting this ad hoc (evidence: Slack threads, support tickets around argo-rollout-internal... domain reservation failures). There is no official integration today.
The core insight driving this design: because AgentEndpoints are cheap in-memory tunnel sessions rather than pod-coupled sidecars, the traffic routing layer is fully decoupled from pod scheduling. This is both a constraint (the Operator can't manage pod lifecycle) and an opportunity (routing changes are instant and don't require pod restarts).
The constraint driving phasing: ngrok billing is per active endpoint. Solutions that approximate traffic weights by scaling AgentEndpoint count are viable for v1 but have cost implications at scale that make precise Traffic Policy-based weighting the better long-term answer.

How the Operator Works Today (Relevant to This Design)
When a user creates an Ingress with ingressClassName: ngrok, the manager controller:

Creates a Domain CRD for the hostname
Creates a CloudEndpoint CRD whose Traffic Policy is derived from the Ingress spec and any attached NgrokTrafficPolicy
Creates one or more AgentEndpoint CRDs — one per route/service — with an upstream.url pointing at the in-cluster Service and an internal URL (e.g., https://my-svc.default.internal)

The Traffic Policy on the CloudEndpoint always ends with a forward-internal action pointing at the AgentEndpoint's internal URL. User-authored rules from NgrokTrafficPolicy are prepended before this forwarding action.
The CloudEndpoint and AgentEndpoint CRDs are operator-managed — users are not expected to touch them directly. The Ingress is the user-facing surface.
The operator's agent deployment watches AgentEndpoint CRs and creates tunnel sessions for them. Each AgentEndpoint CR = one tunnel session, independent of pod count.

What Argo Rollouts Needs From a Traffic Router
Argo Rollouts manages two Services during a canary:

stableService: selector points at the stable ReplicaSet
canaryService: selector points at the canary ReplicaSet

It calls into the traffic router at each rollout step:
gotype TrafficRoutingReconciler interface {
    UpdateHash(canaryHash, stableHash string, ...) error
    SetWeight(desiredWeight int32, ...) error   // 0–100, % to canary
    SetHeaderRoute(route *SetHeaderRoute) error
    SetMirrorRoute(route *SetMirrorRoute) error
    VerifyWeight(desiredWeight int32, ...) (bool, error)
    RemoveManagedRoutes(rollout *Rollout) error
}
For blue/green, Argo manages activeService and previewService and expects an atomic flip on promotion — SetWeight is not called.
For canary, SetWeight(N) is called at each step and the traffic router must express that N% of traffic goes to the canary backend.

Traffic Policy Primitives Available
ngrok's Traffic Policy CEL environment exposes rand.double() which returns a random float between 0 and 1 per request. This is documented in the canary deployments example and is the exact primitive needed for Phase 2 weighted routing:
yamlon_http_request:
  - expressions:
      - "rand.double() <= 0.2"    # 20% to canary
    actions:
      - type: forward-internal
        config:
          url: https://service-canary.internal
  - actions:
      - type: forward-internal
        config:
          url: https://service.internal
This means Phase 2 weighted canary is unblocked — no new Traffic Policy primitives are needed.

Plugin Architecture (All Phases)
Plugin Binary: ngrok/rollouts-plugin-trafficrouter-ngrok
A standalone Go binary implementing TrafficRouterPlugin via HashiCorp go-plugin (RPC over Unix socket, child process of the Argo Rollouts controller pod). Written in Go, consistent with all other Argo Rollouts plugins.
Installation:
yaml# argo-rollouts-config ConfigMap
data:
  trafficRouterPlugins: |-
    - name: "ngrok/ngrok"
      location: "https://github.com/ngrok/rollouts-plugin-trafficrouter-ngrok/releases/download/v0.1.0/plugin-linux-amd64"
RBAC: Plugin inherits the Argo Rollouts controller's service account. Needs:

get/list/watch/update/patch on cloudendpoints.ngrok.k8s.ngrok.com
get/list/watch/create/update/patch/delete on agentendpoints.ngrok.k8s.ngrok.com

The plugin operates entirely through Kubernetes CRDs — it does not need direct ngrok API credentials. The Operator syncs CRD changes to the ngrok API automatically.
Rollout CR shape (consistent across all phases):
yamlapiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: my-app
  namespace: default
spec:
  replicas: 5
  selector:
    matchLabels:
      app: my-app
  template: { ... }
  strategy:
    canary:
      stableService: my-app-stable
      canaryService: my-app-canary
      trafficRouting:
        plugins:
          ngrok/ngrok:
            # CloudEndpoint the Operator created from the Ingress
            cloudEndpoint: my-app
            cloudEndpointNamespace: default
      steps:
        - setWeight: 5
        - pause: {duration: 2m}
        - setWeight: 20
        - pause: {}
        - setWeight: 50
        - pause: {duration: 5m}
        - setWeight: 100
Locating the AgentEndpoint
The plugin needs to find the AgentEndpoint(s) that the Operator created for the stable service in order to clone one for the canary. There are a few approaches, in order of preference:
Preferred: label-based lookup. If the Operator stamps derived objects with a label indicating their source — e.g., ngrok.k8s.ngrok.com/ingress-resource: my-ingress — the plugin can list AgentEndpoints by that label and filter to the one matching the stable Service's upstream. This is a small Operator change worth doing regardless of this project (derived object traceability is broadly useful).
Fallback: service-based lookup. The plugin knows the stableService name from the Rollout spec. It can list all AgentEndpoints in the namespace and find the one whose spec.upstream.url contains the stable Service name.
Explicit config: As a last resort, the plugin config on the Rollout can take an explicit agentEndpoint name. Less magic, more user burden — acceptable as an escape hatch but not the default.
The right answer here requires some exploration of what labels/owner references the Operator currently sets on derived objects. This is a known open question.

Phase 1 — AgentEndpoint Scaling (Ship Now, No Operator Changes)
Concept
Multiple AgentEndpoints that share the same internal URL automatically pool and ngrok load-balances between them equally. Traffic weight can be approximated by the ratio of canary to total AgentEndpoint sessions — without touching Traffic Policy at all.
Importantly, the canary AgentEndpoints in this phase use a different internal URL than stable (my-svc-canary.default.internal vs my-svc-stable.default.internal) — they do not pool with the stable endpoints. The pooling here is within each group (multiple canary sessions share one URL, multiple stable sessions share another). The weight is expressed by the ratio of total canary sessions to total sessions across both groups.
The CloudEndpoint Traffic Policy in this phase still has two forward-internal rules (one for stable URL, one for canary URL), but the weight expression uses replica count rather than rand.double(). Actually, the simplest Phase 1 approach avoids even that: the CloudEndpoint Traffic Policy is left as-is (pointing only at the stable internal URL), and the canary AgentEndpoints use the same internal URL as stable to join the stable pool directly. This way:

Stable AgentEndpoints: https://my-svc.default.internal → upstream my-app-stable
Canary AgentEndpoints: https://my-svc.default.internal → upstream my-app-canary

All sessions pool under the same URL. The CloudEndpoint Traffic Policy doesn't change at all. Weight ≈ canary_sessions / total_sessions.
SetWeight Implementation
gofunc (p *Plugin) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, ...) types.RpcError {
    totalSessions := p.config.TotalPoolSize // configurable, default 10
    canaryCount := int(math.Round(float64(totalSessions) * float64(desiredWeight) / 100.0))
    stableCount := totalSessions - canaryCount

    // Scale canary AgentEndpoints (same URL as stable, different upstream)
    if err := p.scaleAgentEndpoints("canary", p.canaryUpstream, p.stableInternalURL, canaryCount); err != nil {
        return toRpcError(err)
    }
    // Scale stable AgentEndpoints
    return toRpcError(p.scaleAgentEndpoints("stable", p.stableUpstream, p.stableInternalURL, stableCount))
}
scaleAgentEndpoints creates or deletes AgentEndpoint CRs (named my-app-ngrok-rollout-canary-0, my-app-ngrok-rollout-canary-1, etc.) to reach the target count. Each CR is identical except for its name and upstream URL; all share the same internal URL.
Example at SetWeight(20) with totalPoolSize=10:

2 canary AgentEndpoint CRs, each with upstream.url: http://my-app-canary:80
8 stable AgentEndpoint CRs, each with upstream.url: http://my-app-stable:80
All 10 share url: https://my-svc.default.internal
ngrok distributes ~20% of requests to canary upstreams

VerifyWeight
Counts live AgentEndpoint CRs for each group and checks the ratio matches the desired weight within an acceptable tolerance (±1 session, or ~10% for small pool sizes).
RemoveManagedRoutes
Deletes all AgentEndpoint CRs created by the plugin (identifiable by naming convention or owner label). The Operator's agent deployment tears down those tunnel sessions. The original stable AgentEndpoint (the one the Operator created from the Ingress, not the plugin-created copies) is left untouched and resumes handling 100% of traffic.
Tradeoffs
✅ No Operator changes required✅ No Traffic Policy manipulation✅ No reconciler conflictCloudEndpoint is never touched✅ Works today, fully GA✅ Rollback is instant — delete canary CRs⚠️ Weight precision limited by pool sizePool of 10 = 10% granularity⚠️ Billing: N endpoints per rolloutMore AgentEndpoints = more cost⚠️ Distribution is probabilistic per sessionNot exactly N% per request❌ SetHeaderRoute not expressibleNo rule-based routing without Traffic Policy❌ SetMirrorRoute not expressible
Blue/Green in Phase 1
Blue/green is simpler — no weight approximation needed. At InitPlugin(), the plugin clones the stable AgentEndpoint into a preview endpoint with a different internal URL (https://my-svc-preview.default.internal) and patches the CloudEndpoint Traffic Policy to add a second forward-internal rule gated behind a preview-access condition (e.g., an auth header or cookie). On promotion, the plugin patches the single forward-internal URL in the Traffic Policy from stable to preview. On abort, the preview AgentEndpoint is deleted and the Traffic Policy rule is removed.
Note: blue/green requires a single Traffic Policy patch at promotion time (an atomic URL swap), which is a much narrower mutation than the continuous patching in canary. The reconciler conflict is minimal here — one patch, the reconciler will overwrite it on the next cycle, but if the patch and the annotation removal happen together that's fine.

Phase 2 — Traffic Policy Manipulation (Exact Weights, Header Routing)
Concept
Instead of approximating weight via endpoint count, the plugin directly manipulates the forward-internal routing section of the CloudEndpoint's Traffic Policy using rand.double(). This gives exact percentages, header-based routing, and eliminates the per-endpoint billing concern — one canary AgentEndpoint, one stable AgentEndpoint, any weight expressible in CEL.
The Reconciler Conflict
The Operator's manager controller continuously reconciles CloudEndpoint CRDs from the Ingress. If the plugin patches the Traffic Policy routing section, the next reconcile cycle overwrites it.
Solution: opt-in suspension annotation on the Ingress.
k8s.ngrok.com/rollout-managed: "true"
While this annotation is present:

The Operator reconciler builds and applies the prefix of the Traffic Policy (domain, TLS, user auth/rate-limit/header rules from NgrokTrafficPolicy) as normal
It skips writing the forwarding suffix — the terminal forward-internal action(s)
The plugin owns the forwarding suffix for the duration of the rollout

When the rollout completes and the annotation is removed, the next reconcile restores full Operator ownership and rewrites the canonical single-service forwarding rule.
This is the same pattern Flux uses (kustomize.toolkit.fluxcd.io/reconcile: disabled) but scoped to just the routing section rather than the whole object. The Operator continues to react to auth policy changes, TLS changes, domain changes while the canary is running — only backend selection is suspended.
Annotation lifecycle:

Added by user (or mutating webhook — Phase 3 quality-of-life) when associating the Ingress with a Rollout
Persists for the full duration of the rollout
Removed at rollout completion/deletion; next reconcile restores canonical state

Why this boundary is clean: The Operator already constructs the Traffic Policy by building user rules first and appending the forwarding action last. Under rollout-managed, it simply stops before the append. There is no need to parse or diff the forwarding section — the Operator just doesn't write it.
Operator Changes Required
The Traffic Policy build step changes from:
policy = build_prefix(ingress, ngrokTrafficPolicy)
policy += build_forwarding(stable_agent_endpoint)    // always done today
apply(cloudEndpoint, policy)
To:
policy = build_prefix(ingress, ngrokTrafficPolicy)
if not rollout_managed(ingress):
    policy += build_forwarding(stable_agent_endpoint)
apply(cloudEndpoint, policy)
That's the minimal Operator change. The AgentEndpoint setup also simplifies: the plugin creates one canary AgentEndpoint with a distinct internal URL, rather than N pool members:

https://my-svc-stable.default.internal → upstream my-app-stable
https://my-svc-canary.default.internal → upstream my-app-canary

These do not pool with each other. Traffic distribution is handled entirely by the Traffic Policy CEL expression.
SetWeight Implementation
yaml# Plugin-owned forwarding suffix, appended after user prefix rules
on_http_request:
  # [ngrok-rollout-canary] marker comment so plugin can find and replace this block
  - expressions:
      - "rand.double() <= 0.05"    # 5% to canary
    actions:
      - type: forward-internal
        config:
          url: https://my-svc-canary.default.internal
  - actions:
      - type: forward-internal
        config:
          url: https://my-svc-stable.default.internal
SetWeight(0) → emit only the stable rule (no canary rule)
SetWeight(100) → emit only the canary rule (unconditional)
SetWeight(N) → emit rand.double() <= N/100 canary rule + stable fallback
The plugin identifies its managed block by the marker comment (or a metadata annotation on the CloudEndpoint storing the last-written forwarding hash). It strips and rewrites only that block on each SetWeight() call, leaving the user prefix rules untouched.
gofunc (p *Plugin) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, ...) types.RpcError {
    ce := p.fetchCloudEndpoint()
    policy := parsePolicy(ce.Spec.TrafficPolicy.Policy)

    // Strip previous rollout-managed forwarding rules
    policy.OnHTTPRequest = stripManagedRoutes(policy.OnHTTPRequest)

    // Build new forwarding suffix
    threshold := float64(desiredWeight) / 100.0
    if desiredWeight > 0 && desiredWeight < 100 {
        policy.OnHTTPRequest = append(policy.OnHTTPRequest,
            canaryRule(p.canaryURL, threshold),
            stableRule(p.stableURL),
        )
    } else if desiredWeight == 100 {
        policy.OnHTTPRequest = append(policy.OnHTTPRequest, stableRule(p.canaryURL))
    } else {
        policy.OnHTTPRequest = append(policy.OnHTTPRequest, stableRule(p.stableURL))
    }

    return toRpcError(p.patchCloudEndpoint(ce, policy))
}
SetHeaderRoute Implementation
Header-based canary routing, inserted before the weight rule. Maps directly to the header-based pattern from ngrok's own canary deployment docs:
yamlon_http_request:
  # [ngrok-rollout-header-set-header-1]
  - expressions:
      - "req.headers.exists_one(x, x == 'x-canary')"
      - "req.headers['x-canary'].join(',') == 'true'"
    actions:
      - type: forward-internal
        config:
          url: https://my-svc-canary.default.internal
  # weight rule follows...
Supports exact, prefix, and regex header value matching from the Argo Rollouts SetHeaderRoute spec. Enables targeted canaries (internal testers, synthetic probes, QA) independent of the percentage traffic split.
VerifyWeight
Fetches the CloudEndpoint CR, parses the Traffic Policy, finds the managed canary rule, extracts the rand.double() threshold, and compares to the desired weight. Returns true if within tolerance.
Cost Comparison vs Phase 1
PhaseAgentEndpoints at 5% canaryAgentEndpoints at 50% canaryPhase 1 (pool scaling, T=10)10 total10 totalPhase 2 (Traffic Policy)2 total2 total
Phase 2 is strictly better on billing and weight precision.

Phase 3 — First-Class Operator Support (No Argo Required)
The Fundamental Constraint
Argo Rollouts exists because someone has to manage the pod lifecycle — scaling the canary ReplicaSet up, scaling stable down, watching metrics, triggering rollback. The ngrok Operator has no concept of pod scheduling or deployment strategy. It only knows about tunnel sessions and traffic routing.
A first-class ngrok progressive delivery story without Argo therefore cannot mean replacing Argo Rollouts entirely. The clean separation is:

ngrok owns the traffic routing layer. The user (or Argo, or a CI pipeline, or a human) owns the pod scheduling layer.

"First-class" means ngrok provides excellent CRDs and Operator support for the routing side, such that users who don't want to understand the Argo plugin or deal with pool-size approximations have a clean, native way to express canary routing. The pod lifecycle question is explicitly out of scope.
New CRD: NgrokCanary (working name)
A higher-level CRD that the Operator expands into the appropriate CloudEndpoint + AgentEndpoint configuration, with routing semantics built in. An external system — Argo Rollouts (via an updated plugin that patches this CRD instead of the CloudEndpoint directly), a CI pipeline, a human with kubectl — drives the rollout by patching spec.canaryWeight.
yamlapiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: NgrokCanary
metadata:
  name: my-app
  namespace: default
spec:
  # References the CloudEndpoint to manage routing on
  cloudEndpoint: my-app

  stable:
    service: my-app-stable
    internalUrl: https://my-svc-stable.default.internal

  canary:
    service: my-app-canary
    internalUrl: https://my-svc-canary.default.internal

  # This is the field an external system writes to drive the rollout
  canaryWeight: 20          # 0–100

  # Optional: header-based canary routes
  headerRoutes:
    - name: internal-tester
      match:
        header: x-canary
        value: "true"

  # Optional: named NgrokTrafficPolicy to apply on the preview endpoint
  previewAccessPolicy: my-oauth-policy

status:
  canaryWeight: 20
  conditions:
    - type: Progressing
      status: "True"
The Operator's NgrokCanary controller:

Ensures stable and canary AgentEndpoint CRDs exist for the referenced Services
Handles the rollout-managed annotation on the CloudEndpoint's source Ingress automatically — no user action required
Reconciles the Traffic Policy routing section based on spec.canaryWeight and spec.headerRoutes using the same rand.double() approach as Phase 2
Exposes status conditions for external systems to observe

Updated Argo Plugin in Phase 3
The plugin becomes much simpler: instead of directly patching the CloudEndpoint Traffic Policy, it patches NgrokCanary.spec.canaryWeight. The Operator does the Traffic Policy work. This moves the Traffic Policy manipulation logic into its natural home (the Operator) rather than a plugin binary.
go// Phase 3 SetWeight — trivial
func (p *Plugin) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, ...) types.RpcError {
    return toRpcError(p.patchNgrokCanaryWeight(p.config.NgrokCanary, desiredWeight))
}
Mutating Webhook (Quality of Life)
To avoid requiring users to manually add k8s.ngrok.com/rollout-managed: "true" to their Ingress, a mutating webhook can detect when an NgrokCanary or Argo Rollout (referencing a known CloudEndpoint) is created and auto-add the annotation. Makes the Phase 2/3 experience seamless.

Summary: Evolution Table
Phase 1Phase 2Phase 3MechanismAgentEndpoint pool scalingTraffic Policy rand.double()NgrokCanary CRDWeight precisionCoarse (~1/T granularity)ExactExactOperator changesNoneAdd annotation-based reconcile suspensionNew NgrokCanary controller + CRDTraffic Policy changesNonePlugin patches forwarding suffixOperator manages routing sectionReconciler conflictNone — CloudEndpoint never touchedAnnotation suspends forwarding reconcileNative — no annotation neededBillingHigher (N endpoints per rollout)Minimal (2 endpoints always)Minimal (2 endpoints always)Header routing❌✅✅Argo requiredYesYesNo (Argo optional)Blue/green✅ (simple flip)✅✅Canary✅ (approximate)✅ (exact)✅ (exact)

Open Questions

AgentEndpoint discovery / derived object labels. The plugin needs to find the AgentEndpoint the Operator created for the stable service without requiring explicit user config. The right mechanism is label-based lookup — the Operator should stamp derived objects with a label like ngrok.k8s.ngrok.com/ingress-resource: <name> so the plugin (and humans) can trace derived objects back to their source. Whether these labels already exist on derived objects needs to be verified; if not, adding them is a small Operator change worth doing for general traceability regardless of this project. As an interim fallback, the plugin can look up AgentEndpoints by matching spec.upstream.url against the known stable Service name/namespace.
Phase 1 pool size configuration. The totalPoolSize config value on the Rollout determines weight granularity. A sensible default (10) with clear documentation of the tradeoff is sufficient for v1. Consider whether the Operator's existing agent replica count should inform or constrain this value — if the operator has 3 agent replicas, there are already 3 stable sessions in the pool.
Phase 3 CRD name and API shape. NgrokCanary is a working name. The shape above is a strawman — the actual API design needs input from the Operator team on how it fits with existing CRD conventions (NgrokTrafficPolicy, CloudEndpoint, AgentEndpoint naming patterns).
Blue/green Traffic Policy patch in Phase 1. The promotion flip (single forward-internal URL swap on the CloudEndpoint) is a narrow Traffic Policy mutation that conflicts with the Ingress reconciler. For Phase 1 blue/green specifically, a short-lived version of the rollout-managed annotation (set at promotion, cleared after one reconcile cycle confirms the new state) may be needed even without full Phase 2 Operator changes. Alternatively, a webhook or direct ngrok API call from the plugin could handle the flip outside the CRD layer — worth evaluating.
