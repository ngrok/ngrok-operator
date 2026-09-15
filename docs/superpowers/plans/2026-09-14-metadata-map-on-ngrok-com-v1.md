# `spec.metadata` as `map[string]string`, delivered by the `ngrok.com/v1` group move

**Status:** R2 implemented on `alex/k8sop-305-r2-metadata-write-side`; revised after a four-pass adversarial review.
**Authority:** the as-built description lives in [`docs/developer-guide/passivity-shims.md`](../../developer-guide/passivity-shims.md) §"CRD `spec.metadata` type change". Where the two disagree, that section wins. This doc is the rationale record: the measurements, the rejected options, and the review findings.
**Date:** 2026-09-14.
**Author:** alex@ngrok.com.
**Ticket:** [K8SOP-305](https://linear.app/ngrok/issue/K8SOP-305/cleanup-specmetadata-type-flip-schemaless-string-mapstringstring), under [K8SOP-296](https://linear.app/ngrok/issue/K8SOP-296/specstatusannotations-non-passive-changes-round-2).
**Depends on:** the `ngrok.com/v1` group move for every metadata-bearing CRD, all targeting 0.25. Ticketed so far: K8SOP-317 (IPPolicy), K8SOP-318 (KubernetesOperator), K8SOP-319 (Domain); CloudEndpoint and AgentEndpoint are in the same batch but not yet filed.
**Supersedes:** branch `alex/k8sop-305-cleanup-specmetadata-type-flip-schemaless-string` (in-place v1alpha1 retype — do not merge; see [§ Why not the in-place flip](#why-not-the-in-place-flip)).

## TL;DR

- Three releases. R1 (0.24) made the field schemaless so it accepts both
  shapes. **R2 (0.25) stops the operator writing the string form** and flips
  the CRD default to the object form — the v1alpha1 *schema* stays permissive,
  only what the operator puts in it changes. R3 deletes the string branch
  along with the v1alpha1 CRDs.
- The real `map[string]string` lands on the `ngrok.com/v1` CRDs, which have no
  stored objects to strand. The legacy shape is retired by deleting the old
  CRDs, never by tightening them.
- The in-place retype the ticket describes is **measured** unsafe, not
  theorized: a stored legacy string fails the typed List wholesale, which
  fails the manager's cache sync, which **crashloops the whole operator**.
  Evidence in [§ Why not the in-place flip](#why-not-the-in-place-flip).
- This is the **R-cleanup** half of the `LEGACY-metadata-format` migration
  whose R1 shipped in 0.24. What changes is the delivery mechanism, and
  therefore the end date: the sentinel now dies with the v1alpha1 group
  removal, alongside `LEGACY-trafficpolicy-kind`.
- **R2 is not blocked on the v1 moves.** It is written against the v1alpha1
  types and shipped on its own branch. What the v1 types add is one line per
  kind. See [§ What R2 does, and what is left](#what-r2-does-and-what-is-left).
- Cost is larger than "one extra `jq` clause", the first draft's claim.
  Divergent normalization makes the two controllers fight over one ngrok
  resource; non-object metadata has no v1 representation at all; and a
  defaulted map field cannot express empty. Each is addressed below.

## What the adversarial review changed

Four review passes ran against the first draft. What they overturned:

- The central hazard is now **measured**, and worse than stated — whole
  operator crashloop, not one stalled controller. Every row of the
  self-healing table became "Never".
- `passivity-shims.md` did not *miss* the hazard. It **states** it at
  `:751-754` and then contradicts itself at `:787-792` by prescribing the
  retype anyway. The genuinely new finding is that the CRD *default* is the
  legacy string.
- The "silent drop the conversion introduces" premise was **false**.
  `ir.MergeMetadata` already discards unparseable overrides
  (`internal/ir/ir.go:507-511`, pinned by `internal/ir/ir_test.go:491-496`).
- Diverging the normalization creates a **controller fight** during the
  transitional window — see [§ Normalize the legacy branch](#normalize-the-legacy-branch).
- Three new user-facing dead ends: non-object metadata, unclearable
  defaults, and a `jq` recipe that aborts rather than failing at admission.
- "13 read sites" is 15, and five of them are on value types, so the
  accessor seam is not the pure refactor the draft claimed.
- The proposed experiment step was dropped: no outcome would have changed
  the decision. It ran during review anyway, and its result is recorded
  below.

## Context

### What 0.24 shipped (R1)

`spec.metadata` became a bare `json.RawMessage` carrying
`+kubebuilder:validation:Schemaless` and
`+kubebuilder:pruning:PreserveUnknownFields`, so the API server accepts
either a JSON string or a JSON object under the same key:

| Field | Location |
| --- | --- |
| `Domain.spec.metadata` | `api/ingress/v1alpha1/domain_types.go:54` |
| `IPPolicy.spec.metadata` | `api/ingress/v1alpha1/ippolicy_types.go:46` |
| `IPPolicy.spec.rules[].metadata` | `api/ingress/v1alpha1/ippolicy_types.go:71` |
| `AgentEndpoint.spec.metadata` | `api/ngrok/v1alpha1/agentendpoint_types.go:140` |
| `CloudEndpoint.spec.metadata` | `api/ngrok/v1alpha1/cloudendpoint_types.go:80` |
| `KubernetesOperator.spec.metadata` | `api/ngrok/v1alpha1/kubernetesoperator_types.go:185` |

All six default to the legacy JSON **string**
`{"owned-by":"ngrok-operator"}` — five spell the marker
`+kubebuilder:default:=`, `agentendpoint_types.go:139` spells it
`+kubebuilder:default=`; both render the same string default.

`common.MetadataAPIString` (`api/common/v1alpha1/metadata_types.go:29`)
normalizes both shapes into the string the ngrok API takes. Operator-written
objects deliberately kept writing the **string** form for rollback safety
(`pkg/managerdriver/translator.go:817,865`;
`pkg/managerdriver/domains.go:43,80`).

The user-facing deprecation shipped in `docs/upgrading-to-0.24.md:431-485`,
with `jq` audits, and in the future-requirements table at `:537`. That guide
is frozen.

### Where this sits in the sequence

Use the release-role vocabulary from `passivity-shims.md:64-78`, not a
second numbering scheme:

| Role | Content | Release | State |
| --- | --- | --- | --- |
| R1 | Field goes schemaless, accepts string and object; controllers normalize both; operator keeps writing the string form; deprecation documented | 0.24 | **Shipped** |
| R2 | Operator writes the object form only; CRD default flips to object form; legacy write helper deleted; `ngrok.com/v1` CRDs born map-typed; users convert their own objects | 0.25 | **This work** |
| R3 | Legacy string branch and sentinel deleted with the `v1alpha1` CRDs | with the group removal | Pending |

The original entry called this a two-release migration ending in an in-place
retype at 1.0. It is three, and the retype never happens: R3 is the deletion of
the deprecated CRDs, which takes the string form with them.

### What is in flight for 0.25

All CRDs move from their `*.k8s.ngrok.com/v1alpha1` groups to `ngrok.com/v1`
as dual-CRD dual-read migrations — no conversion webhook, both CRDs served,
canonical-first lookup, objects never copied, users re-stamp manifests on
their own schedule (`specs/migration-v1.md:52-102`).

`TrafficPolicy` already landed this way in #875 (`5b2b4601`), establishing:
a kind-agnostic interface plus pointer constraint so one generic
implementation serves both kinds
(`internal/trafficpolicy/resource.go:57-97`); one kind-aware resolver with
the fallback fenced in sentinels (`:136`); and a deliberately
byte-compatible v1 spec (`api/ngrok/v1/trafficpolicy_types.go:36-39`).

## Why not the in-place flip

The abandoned branch retypes the v1alpha1 fields and tightens the schema to
`additionalProperties: {type: string}`. Three findings rule that out.

### 1. The CRD default is itself the legacy string

Defaulting is applied at admission, so any object created under ≤0.24
without an explicit `metadata` has the **string** persisted. Measured: an
object with the field unset *and* an object with `metadata: null` both
persist `'{"owned-by":"ngrok-operator"}'`. This is not a long tail of
hand-authored manifests; it is the default state of the field.

### 2. Retyping the key crashloops the operator — measured

Run on kind v1.36.1 against the repo's real generated Domain CRD, swapping
in a map-typed variant over live objects:

```
A) typed List, repo scheme (json.RawMessage)      LIST OK, 6 items
B) typed List, FLIPPED scheme (map[string]string) LIST ERROR: json: cannot unmarshal
   string into Go struct field DomainSpec.items.spec.metadata of type map[string]string
C) typed Get per object, FLIPPED scheme           d1-unset GET ERROR; d3-map OK
```

Whole-List failure, zero items returned — one bad object poisons the list.
Then, with the broken informer in a controller-runtime cache beside a
healthy one:

```
WaitForCacheSync returned false after 30s
```

In a manager that aborts `Start`. So the blast radius is not a stalled
controller: it is the **operator process failing to start**, taking every
unrelated controller with it.

That conclusion depends on these kinds being read through the cached client,
which they are: every reconciler is constructed with `mgr.GetClient()`
(`cmd/api-manager.go:344,570,582,598,…`) and each watches its kind with
`For(&Domain{})` / `For(&IPPolicy{})` / `For(&CloudEndpoint{})` /
`For(&AgentEndpoint{})`, so each gets an informer.

Recoverable, though — **pruning here is read-time only, so stored data
survives**. Swapping the CRD back to schemaless restored a nested value
intact. A botched CRD swap is undone by rolling the CRD back. The exception
is any *write* while the strict schema is live: an unrelated `kubectl patch`
of `spec.description` made the loss permanent.

### 3. Nothing self-heals

Because the List never succeeds, no reconcile ever runs:

| Kind | Operator writes `Spec.Metadata`? | Heals itself after a flip? |
| --- | --- | --- |
| AgentEndpoint | Yes — `pkg/managerdriver/endpoints.go:77` | **Never** (no sync happens) |
| CloudEndpoint | Yes — `pkg/managerdriver/endpoints.go:158` | **Never** |
| KubernetesOperator | Whole Spec replaced with the field unset, so defaulting re-stamps (`cmd/api-manager.go:819-831`) | **Never** |
| Domain | **No.** `ingressToDomains`/`gatewayToDomains` build it (`domains.go:43,80`); `applyDomains` never assigns it (`:101-116`) | Never |
| IPPolicy | User-authored only | Never |

### The doctrine already said so

`passivity-shims.md:751-754` states the hazard verbatim — "re-typing the key
to an object would make them unreadable by the typed operator" — and then
`:787-792` prescribes exactly that retype as R-cleanup. Step 4 resolves a
self-contradiction in the guide, not an oversight. Note also that §746-809
never pins a release; "ending at 1.0" was the first draft's invention.

## The plan

### Decision

1. `ngrok.com/v1` types are born with `Metadata map[string]string` and an
   object-form default. Real `additionalProperties: {type: string}` schema —
   no `Schemaless`, no `PreserveUnknownFields`.
2. `v1alpha1` types are **frozen**: `json.RawMessage`, schemaless, string
   default, legacy branch intact. They must keep decoding string-form
   objects for as long as the dual-read fallback exists.
3. Operator write paths target v1 and emit the map form. Once no write path
   targets v1alpha1, `MetadataFromLegacyString` is deleted.
4. `LEGACY-metadata-format` is re-scoped: it dies with the v1alpha1 group
   removal. Only the **string branch** of `MetadataAPIString` carries the
   tag, not the whole function.

### The accessor seam

Every consumer converts `spec.metadata` to the ngrok API string
immediately. All **15** sites:

```
internal/controller/ingress/ippolicy_controller.go:111,147,152,409,417,424
internal/controller/ingress/domain_controller.go:176,203
internal/controller/ngrok/kubernetesoperator_controller.go:638,641,643
internal/controller/ngrok/cloudendpoint_controller.go:246,307,559
pkg/agent/driver.go:284
```

Ten are object-level and convert mechanically to `x.GetNgrokMetadata()`.
**Five are not**, and need a decision rather than a rename:

- `ippolicy_controller.go:409,417,424` take `rule ingressv1alpha1.IPPolicyRule`
  by value → needs a method on `IPPolicyRule` itself.
- `cloudendpoint_controller.go:559` takes `spec CloudEndpointSpec`.
- `pkg/agent/driver.go:284` takes `spec AgentEndpointSpec`.

The last two need either spec-level accessors or signature changes. Add
accessors on the **spec** types and have the object-level method delegate;
that covers all 15 without touching signatures.

Three of the 15 don't want a string at all:
`kubernetesoperator_controller.go:635-647` feeds it to `mergeMetadata`
(`:653+`), which unmarshals back into a map to inject `namespace.uid` and
re-marshals. The flip simplifies those.

Shape, matching `GetPolicy()`/`GetConditions()` on `TrafficPolicyResource`:

```go
// api/ngrok/v1 — canonical
func (s *DomainSpec) GetNgrokMetadata() string {
	return common.MetadataAPIStringFromMap(s.Metadata)
}

// api/ngrok/v1alpha1 — deprecated
func (s *DomainSpec) GetNgrokMetadata() string {
	return common.MetadataAPIString(s.Metadata)
}
```

Generic controller bodies never branch on kind. **The accessor lands on the
v1alpha1 types first** (step 2), so the types being duplicated already read
metadata through it and the v1 copies inherit the seam — rather than relying
on telling the v1-move authors not to reach into `.Spec.Metadata`.

### Normalize the legacy branch

The first draft's divergence created a live defect. In the transitional
window both a v1alpha1 and a v1 Domain exist for one hostname, and
`findReservedDomainByHostname` (`domain_controller.go:165-183`) makes both
adopt the **same** reserved-domain ID. The drift check is a byte comparison
(`domain_controller.go:202-204`):

- `MetadataAPIString` returns a legacy string **verbatim**
  (`metadata_types.go:37-43`).
- `MetadataAPIStringFromMap` returns compact, key-sorted JSON.

For any user whose stored string wasn't already compact and sorted —
`'{"team": "platform", "env":"prod"}'` — the two controllers disagree on
every reconcile and alternate `ReservedDomainUpdate` calls against one ngrok
resource, indefinitely. TrafficPolicy's byte-compatible spec made its
double-write a benign no-op; diverging the normalization converts it into
sustained API churn.

**Fix:** the legacy branch parses the string and, when it is a flat object,
re-marshals it compact and key-sorted — identical bytes to the map path.
Non-flat content still passes through verbatim. One convergent update
settles each object; both controllers then agree forever.

This is a behaviour change on the frozen v1alpha1 path: objects whose stored
string was non-canonical get one normalizing `Update` against the ngrok API
at 0.25. That cost is paid either way — without the fix it is paid at
re-stamp time, per object, and never settles while both kinds exist.

### `api/common/v1alpha1/metadata_types.go`

- `MetadataAPIString(json.RawMessage) string` — kept for v1alpha1, with the
  normalization above; only the string branch tagged
  `LEGACY-metadata-format (read-side)`.
- `MetadataAPIStringFromMap(map[string]string) string` — new.
- `MetadataMapFromJSON(string) (map[string]string, error)` — new, the
  boundary between the internal string pipeline and the v1 map field.
  Returns an error; **callers fall back to the explicit default map
  literal, never nil** (see below).
- `MetadataFromLegacyString`, `MetadataFromMap` — deleted once no write path
  targets a v1alpha1 metadata field. Four production writers, not two:
  `translator.go:817,865` **and** `domains.go:43,80`.

### Why the fallback cannot be nil

With `omitempty` on a map, a nil or empty map is dropped from the wire, so
CRD defaulting stamps the stored object while `desired` stays nil. The
`reflect.DeepEqual` at `endpoints.go:76,157` is then false forever — an
Update every Sync, permanently. Fall back to the explicit
`{"owned-by": "ngrok-operator"}` literal.

### Empty metadata becomes unrepresentable

Measured against the map-typed CRD:

```
typed Create with Metadata=map{}     -> stored: {"owned-by":"ngrok-operator"}
typed Update setting Metadata=map{}  -> stored: {"owned-by":"ngrok-operator"}
typed Update setting Metadata=nil    -> stored: {"owned-by":"ngrok-operator"}
```

`json.RawMessage("{}")` has length 2 so `omitempty` emits it; `map[string]string{}`
has length 0 so `omitempty` drops it. **No typed client can write empty
metadata**, and the default re-stamps on every update. Users can still reach
`{}` through raw `kubectl apply` or by patching away the last key, but the
field can never return to absent.

Today `{}` round-trips fine. This is a real regression and it raises a
question the plan cannot answer alone — see
[§ Open questions](#open-questions) Q1.

### The internal pipeline stays a string

helm value (`helm/ngrok-operator/values.yaml:64`) → flattened
`--ngrokMetadata=k=v,k=v`
(`helm/ngrok-operator/templates/api-manager/deployment.yaml:117-121`,
`cmd/api-manager.go:154`) → `util.ParseHelmDictionary` (`:545`) →
`setNgrokMetadataOwner` marshals to JSON (`pkg/managerdriver/driver.go:196-208`)
→ `ir.IRVirtualHost.Metadata string` (`internal/ir/ir.go:96`) →
`ir.MergeMetadata` (`:493`) → CRD field.

Threading a map end to end changes the IR type and the merge helper, and the
override side is a user-supplied JSON string from the `ngrok.com/metadata`
annotation that must be parsed somewhere regardless. **Out of scope.**
Convert at the last hop.

Note the first draft was wrong that the type flip introduces a silent drop:
`MergeMetadata` already discards an unparseable override and returns the base
unchanged (`internal/ir/ir.go:507-511`, pinned by `ir_test.go:491-496`), so
garbage is *already* replaced by the operator default. The only genuine
improvement available is the missing **warning event**, and it can only be
raised where user input is still visible — `translate_ingresses.go:72` /
`translate_gatewayapi.go:188`, not at the translator write sites. That is a
standalone nicety, not a prerequisite, and it is sequenced accordingly.

### What the user has to do differently

`specs/migration-v1.md` needs more than an appended section:

- `:19-22` promises the spec is unchanged so a re-stamp is a kind/apiVersion
  swap. False for five of seven kinds now; scope that claim to TrafficPolicy.
- Add a "Changed Field Types" table beside the existing "Changed Condition
  Types" (`:113-117`).
- **Fully qualify resource names.** `domains.ingress.k8s.ngrok.com` and
  `domains.ngrok.com` share the plural `domains`, so `kubectl get domain foo`
  is ambiguous once both are installed. TrafficPolicy escaped this by having
  distinct plurals; these five do not. The 0.24 guide already qualifies
  (`upgrading-to-0.24.md:456-460`).
- **The re-stamp order is dangerous for these kinds.** `:77,91` prescribe
  "apply new, then delete old". That was safe for TrafficPolicy — no
  external resource, no finalizer. Here `Domain.spec.reclaimPolicy` defaults
  to `Delete` (`domain_types.go:82`), so deleting the v1alpha1 object calls
  `DomainsClient.Delete` (`domain_controller.go:222-230`) and drops the
  reservation and its cert; IPPolicy/CloudEndpoint/AgentEndpoint delete
  through `base_controller.go:114-130`. The documented order must be: apply
  new → verify adoption → patch the old object's `reclaimPolicy` to `Retain`
  (or strip its finalizer) → delete.
- The conversion clause for objects holding a JSON-object string:

  ```
  .spec.metadata |= (if type == "string" then fromjson else . end)
  ```

  plus `.spec.rules[]?.metadata` for IPPolicy.

- **An audit must run before the re-stamp**, separating "converts cleanly"
  from "needs a human". The 0.24 audits
  (`docs/upgrading-to-0.24.md:454-482`) find objects with string metadata but
  do not make that distinction, and the conversion clause is not safe to run
  blind. Shape: `try fromjson catch "UNCONVERTIBLE"`, plus a flat-string-map
  check on the result. Three classes it has to catch, all verified created
  successfully under the 0.24 CRD:
  - *Non-object strings* — `metadata: "hello world"`. `MetadataAPIString`
    forwards these verbatim to the ngrok API today
    (`metadata_types.go:56-58`, doctrine at `passivity-shims.md:766-768`), so
    the shape exists in the wild. `jq` **aborts** —
    `Invalid numeric literal at line 1, column 6` — piping empty stdin into
    `kubectl apply`. The user sees a `jq` parse error, not the documented
    admission error. `map[string]string` cannot represent the value at all;
    it must be re-expressed as key/value pairs by hand.
  - *Non-string scalars and arrays* — `'{"count":3}'`, `'["a","b"]'`. `jq`
    succeeds, admission then rejects with
    `spec.metadata.count ... must be of type string: "integer"` or
    `spec.metadata ... must be of type array`. Hand-editing required.
  - *Nested objects* — rejected on create with
    `unknown field "spec.metadata.team.name"`, but **silently pruned** for
    objects already stored.
- **Null values are silently dropped.** `metadata: {team: null}` is accepted
  by `additionalProperties: {type: string}` and stored as `{}` — no error
  anywhere. This reintroduces exactly the coercion
  `api/common/v1alpha1/metadata_types.go:46-48` was written to prevent: that
  function decodes into `map[string]json.RawMessage` specifically so
  `json.Unmarshal` "would coerce a null value to `""`, silently changing it"
  cannot happen. The map-typed field moves that coercion up into the API
  server, where we cannot intercept it. Accept and document; there is no
  schema-level fix short of CEL.
- *Not* a problem: an object explicitly storing `metadata: {}` under 0.24 is
  stored as `{}` and not defaulted, so it survives the flip cleanly.

So the first draft's "one extra `jq` clause is the whole price" is wrong.
The price is: an audit, a clause for the common case, a manual rewrite for
three classes, a silent null coercion we cannot prevent, a safe delete
order, and one normalizing API update per non-canonical object.

The admission error for the headline case, verbatim, for the doc to quote:

```
The Domain "d7-restamp" is invalid: 
* spec.metadata: Invalid value: "string": spec.metadata in body must be of type object: "string"
* <nil>: Invalid value: null: some validation rules were not checked because the object was invalid; correct the existing errors to complete validation
```

Quote **both** lines and say the second is boilerplate noise, or readers will
hunt for a second problem.

## What R2 does, and what is left

R2 is the write-side cleanup and it is **not blocked on the `ngrok.com/v1`
types**. Everything below landed on `alex/k8sop-305-r2-metadata-write-side`
against the v1alpha1 types, which keep accepting both shapes:

- `api/common/v1alpha1/metadata_types.go`: added `MetadataAPIStringFromMap` and
  `MetadataMapFromJSON` (error-returning), added `DefaultMetadataMap`,
  canonicalized the legacy string branch, deleted `MetadataFromLegacyString`,
  and narrowed the sentinel to the string branch alone.
- `pkg/managerdriver/metadata.go`: `metadataForGeneratedObject`, used by the
  four write sites in `translator.go` and `domains.go`.
- The `+kubebuilder:default` on all six v1alpha1 fields flipped from the string
  form to the object form; CRDs, manifest bundle and helm snapshots
  regenerated.
- `internal/testutils.LegacyMetadataString` so the compat tests keep
  constructing the legacy shape that production code no longer produces.
- `docs/upgrading-to-0.25.md` (user procedure) and the rewritten
  `passivity-shims.md` catalog entry.

What still needs the v1 types, as a one-line change per kind in whichever PR
introduces them:

- `Metadata map[string]string` with `+kubebuilder:default:={"owned-by":"ngrok-operator"}`
  — **not** the backtick form, see [§ Testing](#testing) — and no
  schemaless/pruning markers.
- A `GetNgrokMetadata() string` accessor delegating to
  `MetadataAPIStringFromMap`, added to whatever kind-agnostic interface the
  moves introduce, so generic code never branches on the shape.
- Rewrite the field doc comment: all six currently say "A raw JSON string is
  also accepted for backward compatibility", which is true on v1alpha1 and
  false on v1, and it ships into `kubectl explain`.

R3 deletes the string branch, `LegacyMetadataString`, and the sentinel along
with the v1alpha1 CRDs.


## Testing

### Unit

- `metadata_types_test.go` — `MetadataAPIStringFromMap` (empty, single key,
  multi-key sorted, quotes/unicode); `MetadataMapFromJSON` (valid flat,
  empty, `not-json`, nested, non-string values, `null`); and the
  normalization: a non-canonical legacy string and the equivalent map must
  produce **identical** bytes.
- Translator golden fixtures regenerate to map form.

### envtest

Suites load real CRDs (`internal/controller/ingress/suite_test.go:88-90`),
so they catch the marker footgun from step 7 — keep that property in mind
before anyone "simplifies" the suite setup.

- v1 CRD rejects string metadata with an error naming the field; accepts the
  map form; an object created without `metadata` is defaulted to an
  **object**.
- v1alpha1 CRD still accepts both forms — the dual-read floor.
- A v1alpha1 object holding a **non-canonical** string and a v1 object
  holding the equivalent map produce the same `GetNgrokMetadata()` output —
  i.e. equal *after canonicalization*. Measured cases that are unequal
  today: `'{"b":"2","a":"1"}'` (ordering), `'{"owned-by": "ngrok-operator"}'`
  (whitespace), and an indented multi-line string. Equality holds only for
  already-compact, already-sorted input, which the CRD default happens to be
  — which is why this looks fine at a glance. The assertion is false without
  step 1's normalization; that is the point of testing it.
- Typed clients cannot express `{}`: create/update with an empty or nil map
  stores the default.
- IPPolicy rule-level metadata gets the same cases.

### kind

1. Install 0.24 CRDs; create objects with string metadata, with the field
   unset, with `metadata: null`, with a non-flat object, and with a
   non-object string.
2. Upgrade to the 0.25 chart. All reconcile through the v1alpha1 path; the
   only ngrok API change should be the one-time normalization.
3. Re-stamp with the documented procedure → accepted, reconciles, metadata
   identical.
4. Re-stamp without the conversion clause → rejected. Capture the verbatim
   error for the docs.
5. Re-stamp a non-object string → confirm `jq` aborts, and that the
   documented manual path works.
6. Ingress + Gateway → operator-generated objects land under v1 with
   object-form metadata, and no Update-every-Sync loop (watch for the
   `DeepEqual` wedge).
7. `ngrokMetadata` chart value reaches generated objects as map entries.

### Gate

`make validate`, `make helm-test`, **and `make e2e-uninstall-all`** — the
last is not in `make validate` and is the only thing covering step 11's
fixtures. Plus a clean `make generate manifests manifest-bundle` diff.

## Risks

| Risk | Mitigation |
| --- | --- |
| Transitional two-object window churns the ngrok API | Step 1's normalization makes both controllers emit identical bytes. Without it, sustained alternating updates. |
| Backtick default marker copied into v1 | Explicit warning in step 7; envtest catches it because suites load real CRDs. |
| Nil metadata fallback wedges the sync loop | Fall back to the explicit default literal, never nil. |
| Users with non-object metadata have no path | Documented manual rewrite; `jq` clause explicitly does not cover them. |
| Empty metadata unrepresentable via typed clients | Open question Q1 — decide whether the v1 default stays. |
| A v1 move slips out of 0.25 | That kind's flip slips too. Never substitute the in-place retype. |
| Users who never migrate get no forcing function | R1 deliberately shipped no deprecation event because the strict schema would be the hard error; that no longer arrives. Accept and document, or add an event. |
| Null metadata values silently coerced to absent by the API server | Unpreventable at the schema level; reintroduces what `metadata_types.go:46-48` guards against. Document. |
| Nested metadata destroyed permanently if an object is touched under the strict schema | Pruning is read-time only, so a CRD rollback recovers — but only until the next write. Audit before, not after. |

## Related issues found during review (not this change)

Found while reviewing this plan. None are caused by the metadata
change and none are fixed by it, but they land in the same release and
some of them make its transitional window more dangerous. Recorded here
so the information is not lost; who picks them up is a separate call.
   list `ngrokv1alpha1` types; `endpoints.go:100,181` delete only from those
   lists. Once the driver writes v1, operator-generated v1alpha1 AEPs/CLEPs
   fall out of `current` — never deleted, still finalized, still driving
   their own ngrok endpoints. Likeliest source of duplicate live endpoints in
   the whole migration.
2. **Domains get recreated under v1alpha1** after a user re-stamps, via
   `domains.go:91-116` and `internal/domain/manager.go:186-203,264-280`,
   neither of which looks for a v1 twin. No Domain GC exists
   (`manager.go:317-334` is binding-specific).
3. **Duplicate CloudEndpoint creation** — `cloudendpoint_controller.go:246-260`
   creates unconditionally, so two objects on one `spec.url` mean two ngrok
   endpoints or a hard rejection.
4. **Rollback deletes the v1 CRDs.** They ship as ordinary chart templates
   with no `helm.sh/resource-policy: keep` (none anywhere in `helm/`), so a
   `helm rollback` to 0.24 removes them and apiextensions cascades to stored
   objects — whose finalizers the 0.24 operator cannot remove, wedging them
   in `Terminating`.
5. **Drain and the cleanup hook miss v1.** `internal/drain/` is v1alpha1-typed
   throughout, and `cleanup-hook/job.yaml:34` uses the ambiguous short name
   `kubernetesoperator` while its Role grants only `ngrok.k8s.ngrok.com`
   (`cleanup-hook/rbac.yaml:25-33`) — Forbidden, swallowed by `|| true`, so
   no drain runs and uninstall looks clean.
6. **`installCRDs=false` ordering.** No presence guard for ngrok's own kinds
   (`cmd/api-manager.go:642`, versus the TCPRoute guards at `:200-253`);
   operator upgraded before `ngrok-crds` crashloops.
   `specs/migration-v1.md:76` says "before migrating objects", not "before
   the operator".
7. **`getDomainsByDomain` is v1alpha1-only** (`pkg/managerdriver/driver.go:695-700`),
   so re-stamping a Domain loses Ingress load-balancer status.

## Collisions with in-flight work

- **#872 (K8SOP-303 prefix sweep)** — rewrites 49 `pkg/managerdriver/testdata/`
  files and `internal/controller/service/controller.go`. Step 12 touches
  `internal/annotations/`; sequence after. Also decides whether fixtures
  spell `ngrok.com/metadata` or `k8s.ngrok.com/metadata` (dual-read at
  `internal/annotations/parser/parser.go:39,215`).
- **#887 (wildcard domains)** — touches `domain_types.go`,
  `domain_controller.go` (step 2 rewrites `:176,203`), the Domain CRD
  template, `manifest-bundle.yaml`, `specs/crds/domain.md`. Sequence after.
- **#829 (helm values refactor)** — moves config to env/configmap, so this
  plan's `deployment.yaml:117-121` and `cmd/api-manager.go:154` citations
  will rot. Conclusion unaffected.
- **#874 (K8SOP-307)** — no overlap.

## Out of scope

- **`applyDomains` never writes `Spec.Metadata`** (`domains.go:101-116`), so
  operator-created Domains carry whatever the CRD default stamped. A second
  path, `internal/domain/manager.go:264-271`, also creates them with metadata
  unset. Pre-existing, own ticket — recorded here because it is why Domains
  cannot self-heal.
- Threading `map[string]string` through the IR.
- The other round-2 cleanups (303, 304, 306, 307, 298).

## Alternatives considered

- **In-place retype of v1alpha1** — measured unsafe; see above.
- **Defer past 0.25** — costs a v1→v2 dual-CRD cycle later for a two-line
  type change. 0.25 is the only free window.
- **Map-typed Go field + schemaless schema + custom `UnmarshalJSON`** —
  preserves a zero-edit re-stamp and keeps `migration-v1.md:19-22` intact.
  Rejected: this is precisely the `LEGACY-enabledfeatures-format` shape, and
  K8SOP-306 is that approach's bill coming due — a hand-written codec that
  outlives the migration and has to be cleaned up on its own schedule. Also
  it keeps the weak schema (`passivity-shims.md:805-809`) forever.

## Open questions

1. **Does the operator need to be able to clear metadata?** If yes, the v1
   default has to go, since no typed client can write `{}` against a
   defaulted map field. If no, document that clearing requires raw `kubectl`
   and accept the re-stamping default. This is the one open design decision
   in the plan.
2. **Sequencing (a) or (b)** for the flip — recommendation is (a), given the
   seam lands first.
3. Does the round-2 batch (303/307 open as #872/#874; 304/306/298 unpushed)
   land in 0.25? K8SOP-304 is marked Done in Linear while `resolves_to` is
   still on `main`, so that status is stale.
