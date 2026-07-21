# User-Facing Annotation Prefix Migration (R1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** R1 (migration release) support for user-written `k8s.ngrok.com/*` annotations: the operator reads both `ngrok.com/*` (canonical, wins) and `k8s.ngrok.com/*` (legacy, fallback) and emits `LegacyAnnotation` Warning events so users can find and migrate their manifests before 1.0 drops the legacy read.

**Architecture:** Dual-read is hidden inside `internal/annotations/parser` (the single chokepoint all seven parser-funneled keys flow through), matching the repo's established hide-dual-read-in-helper pattern (`HasFinalizer`, `GetResolvesTo`, `ExtractComputedURL`). Caller signatures do not change. Deprecation signaling is decoupled from the read path: a new sentinel-wrapped `internal/deprecation` package scans object annotations once per reconcile in the Ingress/Gateway/Service controllers. Off-parser read sites (Gateway `terminate-tls.*` TLS options, `app-protocols`, the `http2` appProtocol value) get explicit dual-read with the same canonical-wins precedence. The bindings-forwarder pod identity is the one exception to canonical-wins: it is a prefix *filter*, not a key lookup — pod annotations under either prefix are forwarded verbatim, side by side, with no precedence between them.

**Tech Stack:** Go, controller-runtime, `k8s.io/client-go/tools/events` (events/v1 `EventRecorder` — note the 6-arg `Eventf`), table-driven tests + golden translator fixtures.

## Global Constraints

- **DO NOT COMMIT and DO NOT create a PR.** All work happens on branch `alex/migrate-user-facing-annotations-r1` (created from `origin/main`) and is left **uncommitted** in the working tree for local review. Every "commit" step in the usual TDD cycle is replaced by "leave uncommitted; move on".
- This is **R1 only**: read both prefixes, write nothing, warn on legacy. Read-side cleanup ships at 1.0 and is NOT part of this work.
- Every legacy-only code site carries a `LEGACY-PREFIX-MIGRATION` sentinel with the cleanup kind, per `docs/developer-guide/passivity-shims.md`. All shims here are `(read-side cleanup)` — there is no write side for user-owned keys.
- Precedence when both prefixes are present: **canonical (`ngrok.com/`) wins, decided by key presence** — a canonical key with invalid content fails parsing rather than silently deferring to the legacy value.
- Do NOT touch: CRD API groups (`*.k8s.ngrok.com` in `api/`, `config/`, helm RBAC), any existing `Legacy*` symbol from the operator-written migrations (#819/#820/#821), the helm chart `ingress.controllerName` (stays legacy until R2 per the rollout-race deferral).
- Go style: table-driven tests, follow existing repo test patterns, no comments beyond what the surrounding code does.
- Run `nix develop` shell (or rely on direnv) for all commands.
- The specs are the authoritative inventory of user-facing surfaces, split by topic: `specs/annotations.md` (annotations, including `app-protocols`), `specs/upstream-protocols.md` (new file: `app-protocols` + recognized `appProtocol` field values), `specs/features/gateway-api.md` (`terminate-tls.*` listener TLS option keys), `specs/features/bindings.md` (pod-identity annotation forwarding), plus three files with smaller corrections from planning: `specs/README.md` (index entry), `specs/controllers/service.md` (description/metadata not read from Services), `specs/controllers/gateway-api/httproute.md` (annotations read from parent Gateway). All **seven** sit uncommitted in the working tree — carry every one onto the branch. If implementation reveals a behavior the specs don't describe, update the spec in the same change.

---

### Task 0: Branch setup

**Files:** none

- [ ] **Step 1: Create the branch**

```bash
git fetch origin main
git checkout -b alex/migrate-user-facing-annotations-r1 origin/main
```

- [ ] **Step 2: Verify the baseline builds green**

The working tree is *intentionally dirty*: the seven spec files from Global Constraints, this plan under `docs/superpowers/`, and `resurrect-research.md` must survive the branch switch. Record `git status --short` before Step 1, and confirm the same list (nothing more, nothing less) after — `git checkout -b` carries uncommitted changes, but verify rather than assume.

Run: `go build ./... && go test ./internal/annotations/... ./pkg/managerdriver/ ./internal/controller/bindings/`
Expected: PASS (baseline green before any change — the dirty files are docs/specs only and can't affect this)

---

### Task 1: Parser dual-read + bindings-forwarder prefix filter

The parser and the forwarder are compile-coupled (the forwarder is the only external consumer of `parser.DefaultAnnotationsPrefix`, which this task removes), so both change together.

**Files:**
- Modify: `internal/annotations/parser/parser.go:31-37` (consts), `:129-204` (helpers)
- Modify: `internal/controller/bindings/forwarder_controller.go:356-364`
- Test: `internal/annotations/parser/parser_test.go` (create)
- Test: `internal/controller/bindings/forwarder_controller_test.go` (extend existing `podIdentityFromPod` coverage)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `parser.CanonicalAnnotationsPrefix = "ngrok.com"` (const), `parser.LegacyAnnotationsPrefix = "k8s.ngrok.com"` (const, sentinel-wrapped), unchanged signatures `parser.GetStringAnnotation(name string, obj client.Object) (string, error)` / `GetBoolAnnotation` / `GetStringSliceAnnotation` / `GetStringMapAnnotation` / `GetIntAnnotation` / `GetFloatAnnotation` / `GetAnnotationWithPrefix(suffix string) string` (now returns the canonical key). `DefaultAnnotationsPrefix` and the mutable `AnnotationsPrefix` var are **deleted**. Tasks 3, 5, 6 use the two new consts.

- [ ] **Step 1: Write the failing parser test**

Create `internal/annotations/parser/parser_test.go`:

```go
package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ngrok/ngrok-operator/internal/errors"
)

func objWithAnnotations(anns map[string]string) client.Object {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: anns}}
}

func TestGetStringAnnotationDualRead(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		want        string
		wantErr     func(error) bool
	}{
		{
			name:        "canonical only",
			annotations: map[string]string{"ngrok.com/url": "tcp://a"},
			want:        "tcp://a",
		},
		{
			name:        "legacy only falls back",
			annotations: map[string]string{"k8s.ngrok.com/url": "tcp://b"},
			want:        "tcp://b",
		},
		{
			name: "both present canonical wins",
			annotations: map[string]string{
				"ngrok.com/url":     "tcp://new",
				"k8s.ngrok.com/url": "tcp://old",
			},
			want: "tcp://new",
		},
		{
			name: "canonical present but empty does not fall back",
			annotations: map[string]string{
				"ngrok.com/url":     "",
				"k8s.ngrok.com/url": "tcp://old",
			},
			wantErr: errors.IsInvalidContent,
		},
		{
			name:        "neither present",
			annotations: map[string]string{"other/url": "x"},
			wantErr:     errors.IsMissingAnnotations,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetStringAnnotation("url", objWithAnnotations(tc.annotations))
			if tc.wantErr != nil {
				assert.Error(t, err)
				assert.True(t, tc.wantErr(err))
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetBoolAnnotationDualRead(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		want        bool
		wantErr     bool
	}{
		{
			name:        "legacy only falls back",
			annotations: map[string]string{"k8s.ngrok.com/pooling-enabled": "true"},
			want:        true,
		},
		{
			name: "both present canonical wins",
			annotations: map[string]string{
				"ngrok.com/pooling-enabled":     "false",
				"k8s.ngrok.com/pooling-enabled": "true",
			},
			want: false,
		},
		{
			name: "invalid canonical does not fall back",
			annotations: map[string]string{
				"ngrok.com/pooling-enabled":     "not-a-bool",
				"k8s.ngrok.com/pooling-enabled": "true",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetBoolAnnotation("pooling-enabled", objWithAnnotations(tc.annotations))
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetStringSliceAnnotationDualRead(t *testing.T) {
	got, err := GetStringSliceAnnotation("bindings", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/bindings": "a, b"},
	))
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)

	got, err = GetStringSliceAnnotation("bindings", objWithAnnotations(map[string]string{
		"ngrok.com/bindings":     "new",
		"k8s.ngrok.com/bindings": "old",
	}))
	assert.NoError(t, err)
	assert.Equal(t, []string{"new"}, got)
}

// The implementation changes all six Get* helpers via annotationKeyFor, so
// pin the remaining value types too (one legacy-fallback + one canonical-wins
// case each is enough).
func TestGetStringMapAnnotationDualRead(t *testing.T) {
	got, err := GetStringMapAnnotation("metadata", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/metadata": `{"a":"b"}`},
	))
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b"}, got)

	got, err = GetStringMapAnnotation("metadata", objWithAnnotations(map[string]string{
		"ngrok.com/metadata":     `{"x":"y"}`,
		"k8s.ngrok.com/metadata": `{"a":"b"}`,
	}))
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"x": "y"}, got)
}

func TestGetIntAnnotationDualRead(t *testing.T) {
	got, err := GetIntAnnotation("port", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/port": "8080"},
	))
	assert.NoError(t, err)
	assert.Equal(t, 8080, got)

	got, err = GetIntAnnotation("port", objWithAnnotations(map[string]string{
		"ngrok.com/port":     "9090",
		"k8s.ngrok.com/port": "8080",
	}))
	assert.NoError(t, err)
	assert.Equal(t, 9090, got)
}

func TestGetFloatAnnotationDualRead(t *testing.T) {
	// GetFloatAnnotation returns float32 (parser.go:191) — typed expectations
	// or assert.Equal fails on float64-vs-float32.
	got, err := GetFloatAnnotation("weight", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/weight": "0.5"},
	))
	assert.NoError(t, err)
	assert.Equal(t, float32(0.5), got)

	got, err = GetFloatAnnotation("weight", objWithAnnotations(map[string]string{
		"ngrok.com/weight":     "0.75",
		"k8s.ngrok.com/weight": "0.5",
	}))
	assert.NoError(t, err)
	assert.Equal(t, float32(0.75), got)
}
```

All six typed helpers change, so all six get a legacy-fallback and a canonical-wins case — no sampling.

Check `internal/errors` for the exact names of `IsInvalidContent` / `IsMissingAnnotations` before running (`grep -n "func Is" internal/errors/*.go`) and adjust the test to the real predicate names.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/annotations/parser/ -run 'DualRead' -v`
Expected: FAIL — legacy-fallback and canonical-wins cases fail (current code only reads `k8s.ngrok.com/*`)

- [ ] **Step 3: Implement parser dual-read**

In `internal/annotations/parser/parser.go`, replace lines 31-37:

```go
// CanonicalAnnotationsPrefix is the prefix for ngrok annotations on user
// objects (Ingress, Gateway, Service).
const CanonicalAnnotationsPrefix = "ngrok.com"

// LEGACY-PREFIX-MIGRATION: BEGIN
// LegacyAnnotationsPrefix is the deprecated user-annotation prefix. Every
// Get* helper falls back to it when the canonical key is absent — see
// annotationKeyFor. Read-side cleanup deletes this const and the fallback.
const LegacyAnnotationsPrefix = "k8s.ngrok.com"

// LEGACY-PREFIX-MIGRATION: END
```

(The mutable `AnnotationsPrefix` var and `DefaultAnnotationsPrefix` are deleted — nothing mutates the var, and the forwarder's read of `DefaultAnnotationsPrefix` is updated in Step 5.)

Replace `checkAnnotation` (line 129) and add `annotationKeyFor`:

```go
func checkAnnotation(name string, obj client.Object) error {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return errors.ErrMissingAnnotations
	}
	if name == "" {
		return errors.ErrInvalidAnnotationName
	}

	return nil
}

// annotationKeyFor resolves which prefixed key to read for the given suffix.
// The canonical key wins on presence alone, so a canonical key with invalid
// content surfaces a parse error instead of silently deferring to the legacy
// value.
func annotationKeyFor(suffix string, obj client.Object) string {
	canonical := GetAnnotationWithPrefix(suffix)
	anns := obj.GetAnnotations()
	if _, ok := anns[canonical]; ok {
		return canonical
	}
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the fallback below;
	// this function collapses to `return GetAnnotationWithPrefix(suffix)`.
	legacy := fmt.Sprintf("%v/%v", LegacyAnnotationsPrefix, suffix)
	if _, ok := anns[legacy]; ok {
		return legacy
	}
	return canonical
}
```

Update every `Get*Annotation` helper to the same shape (shown for bool and string; apply identically to `GetStringSliceAnnotation`, `GetStringMapAnnotation`, `GetIntAnnotation`, `GetFloatAnnotation`):

```go
// GetBoolAnnotation extracts a boolean from a client.Object annotation
func GetBoolAnnotation(name string, obj client.Object) (bool, error) {
	if err := checkAnnotation(name, obj); err != nil {
		return false, err
	}
	v := annotationKeyFor(name, obj)
	return annotations(obj.GetAnnotations()).parseBool(v)
}

// GetStringAnnotation extracts a string from an client.Object annotation
func GetStringAnnotation(name string, obj client.Object) (string, error) {
	if err := checkAnnotation(name, obj); err != nil {
		return "", err
	}
	v := annotationKeyFor(name, obj)
	return annotations(obj.GetAnnotations()).parseString(v)
}
```

Update `GetAnnotationWithPrefix`:

```go
// GetAnnotationWithPrefix returns the canonical (ngrok.com/) annotation key
// for the given suffix.
func GetAnnotationWithPrefix(suffix string) string {
	return fmt.Sprintf("%v/%v", CanonicalAnnotationsPrefix, suffix)
}
```

- [ ] **Step 4: Run parser tests**

Run: `go test ./internal/annotations/parser/ -v`
Expected: PASS

- [ ] **Step 5: Write failing forwarder test, then widen the pod-identity filter**

In `internal/controller/bindings/forwarder_controller_test.go`, find the existing `podIdentityFromPod` test and add cases (match the file's existing table/assert style):

```go
// pod with both-prefix annotations
pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
	Annotations: map[string]string{
		"ngrok.com/foo":     "new",
		"k8s.ngrok.com/bar": "old",
		"unrelated.io/baz":  "skip",
	},
}}
identity := podIdentityFromPod(pod)
assert.Equal(t, map[string]string{
	"ngrok.com/foo":     "new",
	"k8s.ngrok.com/bar": "old",
}, identity.Annotations)
```

Run: `go test ./internal/controller/bindings/` → expected FAIL (compile error on `parser.DefaultAnnotationsPrefix` plus the new-prefix key missing). Package-wide on purpose: if the file's style makes this a Ginkgo spec, a `-run PodIdentity` filter would match nothing once green (the suite's only top-level test is `TestControllers`).

Then in `internal/controller/bindings/forwarder_controller.go` replace `podIdentityFromPod` (line 356):

```go
// podIdentityFromPod extracts a PodIdentity from a Pod, pruning annotations
// to only include ngrok-prefixed keys. Exported for unit testing.
func podIdentityFromPod(pod *v1.Pod) *pb_agent.PodIdentity {
	anns := make(map[string]string)
	for key := range pod.Annotations {
		// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the legacy prefix match
		if strings.HasPrefix(key, parser.CanonicalAnnotationsPrefix+"/") ||
			strings.HasPrefix(key, parser.LegacyAnnotationsPrefix+"/") {
			anns[key] = pod.Annotations[key]
		}
	}

	return &pb_agent.PodIdentity{
		Uid:         string(pod.UID),
		Name:        pod.Name,
		Namespace:   pod.Namespace,
		Annotations: anns,
	}
}
```

Note the `+"/"` — the old code matched the bare `k8s.ngrok.com` prefix, which also matched keys like `k8s.ngrok.com.evil.io/x`. If the existing test pins the bare-prefix behavior, keep the assertions consistent with the new `/`-anchored behavior and adjust the test.

- [ ] **Step 6: Run both packages**

Run: `go test ./internal/annotations/parser/ ./internal/controller/bindings/ && go build ./...`
Expected: PASS. Leave uncommitted.

---

### Task 2: Flip annotation consts + extractor coverage

**Files:**
- Modify: `internal/annotations/annotations.go:38-69` (consts), `:93-217` (doc comments), `:133` (bindings literal)
- Modify: `pkg/managerdriver/translate_ingresses.go` (log strings ~lines 74, 81, 187, 193; wrong-const pooling messages at 37, 41), `pkg/managerdriver/translate_gatewayapi.go` (log strings ~lines 176, 183; wrong-const pooling messages at 150, 154), `internal/ir/ir.go:99` (comment)
- Test: `internal/annotations/annotations_test.go`

**Interfaces:**
- Consumes: Task 1 parser behavior (no signature changes).
- Produces: `annotations.MappingStrategyAnnotation = "ngrok.com/mapping-strategy"`, `EndpointPoolingAnnotation = "ngrok.com/pooling-enabled"`, `TrafficPolicyAnnotation = "ngrok.com/traffic-policy"`, `URLAnnotation = "ngrok.com/url"`, `MetadataAnnotation = "ngrok.com/metadata"`, `DescriptionAnnotation = "ngrok.com/description"`, **new** `BindingsAnnotation = "ngrok.com/bindings"` / `BindingsAnnotationKey = "bindings"`. All `Extract*` signatures unchanged.

- [ ] **Step 1: Write failing extractor tests**

In `internal/annotations/annotations_test.go`, ensure each of `ExtractNgrokTrafficPolicyFromAnnotations`, `ExtractUseEndpointPooling`, `ExtractUseBindings`, `ExtractURL`, `ExtractMetadata`, `ExtractDescription` has all **three** cases (add table cases following the file's existing pattern):
- object annotated with the **literal** `k8s.ngrok.com/` key only → value returned (legacy fallback)
- object annotated with the `ngrok.com/` key only → value returned
- object annotated with both keys → `ngrok.com/` value returned

The file is mixed today: the pooling, bindings, metadata, and description tests already use literal `k8s.ngrok.com/*` keys — **leave those literals alone** (do not rewrite them to consts; after the const flip they ARE the legacy-fallback coverage), so those four only need the canonical and both-keys cases. The traffic-policy test keys off `annotations.TrafficPolicyAnnotation` (flips to canonical with the const), and `ExtractURL` has no extractor-level test at all — those two need all three cases added. The existing error-message expectations that embed `k8s.ngrok.com/...` key names (e.g. the empty-value pooling and bindings cases) still pass unchanged: with only the legacy key present, the parser resolves and reports the legacy key.

Example shape (adapt to the file's existing helpers):

```go
{
	name: "new prefix",
	annotations: map[string]string{"ngrok.com/traffic-policy": "policy-a"},
	want: "policy-a",
},
{
	name: "both prefixes, new wins",
	annotations: map[string]string{
		"ngrok.com/traffic-policy":     "policy-new",
		"k8s.ngrok.com/traffic-policy": "policy-old",
	},
	want: "policy-new",
},
```

- [ ] **Step 2: Run to verify state**

Run: `go test ./internal/annotations/ -v`
Expected: the new-prefix cases already PASS (Task 1's parser dual-read covers them). That's fine — these tests pin the extractor-level contract so Task 1's parser can't regress silently. If they pass, continue.

- [ ] **Step 3: Flip the consts and doc comments**

In `internal/annotations/annotations.go`, update the const block (lines 38-69) — full-key consts are only used in log/error message text (`translate_ingresses.go:32-41`, `translate_gatewayapi.go:145-154`, `translator.go:919`), so flipping them is log-text-only:

```go
	// This annotation can be used on ingress/gateway resources to control which ngrok resources (endpoints/edges) get created from it
	MappingStrategyAnnotation    = "ngrok.com/mapping-strategy"
	MappingStrategyAnnotationKey = "mapping-strategy"

	EndpointPoolingAnnotation    = "ngrok.com/pooling-enabled"
	EndpointPoolingAnnotationKey = "pooling-enabled"

	TrafficPolicyAnnotation    = "ngrok.com/traffic-policy"
	TrafficPolicyAnnotationKey = "traffic-policy"

	// This annotation can be used on a service to control whether the endpoint is a TCP or TLS endpoint.
	// Examples:
	//   * tcp://1.tcp.ngrok.io:12345
	//   * tls://my-domain.com
	//
	URLAnnotation = "ngrok.com/url"
	URLKey        = "url"
```

Same flip for `MetadataAnnotation` and `DescriptionAnnotation`. Add after `TrafficPolicyAnnotationKey`:

```go
	// This annotation controls where the endpoint created from this resource is
	// bound (its visibility), e.g. public, internal, or kubernetes.
	BindingsAnnotation    = "ngrok.com/bindings"
	BindingsAnnotationKey = "bindings"
```

Update `ExtractUseBindings` (line 133) to use the const:

```go
	bindings, err := parser.GetStringSliceAnnotation(BindingsAnnotationKey, obj)
```

Update the doc comments that spell out full legacy keys — `k8s.ngrok.com/traffic-policy` (line 94), `k8s.ngrok.com/pooling-enabled` (line 114), `k8s.ngrok.com/bindings` (line 131), `k8s.ngrok.com/url` (line 152), `k8s.ngrok.com/metadata` (line 194), `k8s.ngrok.com/description` (line 207) — to the `ngrok.com/` form. Do NOT touch `ComputedURLAnnotation`, `LegacyComputedURLAnnotation`, or `ExtractComputedURL` (live #821 shims). Also update the stale parser reference inside the `ExtractComputedURL` doc comment (line 167-170): replace the sentence about `parser.AnnotationsPrefix == "k8s.ngrok.com"` with a note that it predates the parser's dual-read and now stays direct-read only to keep the operator-written key's behavior self-contained.

- [ ] **Step 4: Fix production log strings that hardcode the legacy prefix**

These are user-visible log messages (not comments) that spell out `k8s.ngrok.com/...` literally. Interpolate the (now-canonical) consts instead so the messages track the migration automatically:

In `pkg/managerdriver/translate_ingresses.go` (~line 74, and the matching description-read error nearby):

```go
			t.log.Error(err, fmt.Sprintf("failed to read %q annotation for ingress", annotations.MetadataAnnotation),
```

In the shared-hostname warnings (~lines 187, 193), replace the literal key in the message text:

```go
				t.log.Info(fmt.Sprintf("multiple ingresses sharing the same hostname have different %q annotations; the metadata from the first-processed ingress will be used", annotations.MetadataAnnotation),
```

(same pattern for the description warning). The two gateway messages at `pkg/managerdriver/translate_gatewayapi.go:176` and `:183` are the metadata/description **read-error** analogues of ingresses' :74/:81 (the gateway file has no shared-hostname warnings) — apply the same const interpolation there.

Also fix a pre-existing wrong-const bug in the pooling diagnostics (both reviews flagged it; these lines sit inside the blocks this step already edits). The messages at `translate_ingresses.go:37` and `:41` and `translate_gatewayapi.go:150` and `:154` are about `ExtractUseEndpointPooling` results but interpolate `annotations.MappingStrategyAnnotation` — a copy-paste from the mapping-strategy check above them. Change all four to `annotations.EndpointPoolingAnnotation`. The mapping-strategy messages themselves (`translate_ingresses.go:32`, `translate_gatewayapi.go:145`) are correct — leave them. Find every remaining hit with:

```bash
grep -rn 'k8s\.ngrok\.com' pkg/managerdriver/translate_ingresses.go pkg/managerdriver/translate_gatewayapi.go internal/ir/ir.go
```

and update the `internal/ir/ir.go:99` doc comment (`Sourced from the k8s.ngrok.com/description annotation`) to the `ngrok.com/` form. After this step that grep should return only the `terminate-tls` sites Task 5 handles.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/annotations/... ./pkg/managerdriver/ && go build ./...`
Expected: PASS. Leave uncommitted.

---

### Task 3: `internal/deprecation` package

**Files:**
- Create: `internal/deprecation/deprecation.go`
- Test: `internal/deprecation/deprecation_test.go` (create)

**Interfaces:**
- Consumes: `parser.CanonicalAnnotationsPrefix`, `parser.LegacyAnnotationsPrefix` (Task 1).
- Produces: `deprecation.ScanAnnotations(log logr.Logger, recorder EventRecorder, obj client.Object)`, `deprecation.ReasonLegacyAnnotation = "LegacyAnnotation"`, `type EventRecorder interface { Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args ...any) }`. Task 4 wires `ScanAnnotations` into the three controllers; the interface is satisfied by each controller's existing `Recorder events.EventRecorder` field.

- [ ] **Step 1: Write the failing test**

Create `internal/deprecation/deprecation_test.go`:

```go
package deprecation

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type recordedEvent struct {
	eventtype string
	reason    string
	note      string
}

type fakeRecorder struct {
	events []recordedEvent
}

func (f *fakeRecorder) Eventf(_, _ runtime.Object, eventtype, reason, _, note string, args ...any) {
	f.events = append(f.events, recordedEvent{eventtype, reason, fmt.Sprintf(note, args...)})
}

func TestScanAnnotations(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		wantReasons int
		wantNotes   []string
	}{
		{
			name: "one legacy key",
			annotations: map[string]string{
				"k8s.ngrok.com/traffic-policy": "p",
			},
			wantReasons: 1,
			wantNotes:   []string{`"k8s.ngrok.com/traffic-policy"`},
		},
		{
			name: "multiple legacy keys, one event each",
			annotations: map[string]string{
				"k8s.ngrok.com/url":           "tcp://x",
				"k8s.ngrok.com/app-protocols": `{"p":"http"}`,
			},
			wantReasons: 2,
		},
		{
			name: "canonical keys emit nothing",
			annotations: map[string]string{
				"ngrok.com/traffic-policy": "p",
				"ngrok.com/url":            "tcp://x",
			},
			wantReasons: 0,
		},
		{
			name:        "no annotations",
			annotations: nil,
			wantReasons: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations}}
			ScanAnnotations(logr.Discard(), rec, obj)
			assert.Len(t, rec.events, tc.wantReasons)
			for _, ev := range rec.events {
				assert.Equal(t, corev1.EventTypeWarning, ev.eventtype)
				assert.Equal(t, ReasonLegacyAnnotation, ev.reason)
			}
			for _, want := range tc.wantNotes {
				assert.Contains(t, rec.events[0].note, want)
			}
		})
	}
}

func TestScanAnnotationsNilRecorderDoesNotPanic(t *testing.T) {
	obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{"k8s.ngrok.com/url": "tcp://x"},
	}}
	ScanAnnotations(logr.Discard(), nil, obj)
}

// Guardrail: keeps the scanned suffix list from being edited accidentally.
// It compares two manually maintained lists, so it cannot detect a NEW
// user-facing annotation added elsewhere — the Task 10 completeness audit
// (rg for the legacy prefix) is what catches those.
func TestUserFacingAnnotationSuffixes(t *testing.T) {
	want := []string{
		"url",
		"mapping-strategy",
		"traffic-policy",
		"pooling-enabled",
		"bindings",
		"metadata",
		"description",
		"app-protocols",
	}
	assert.ElementsMatch(t, want, userFacingAnnotationSuffixes)
	for _, s := range userFacingAnnotationSuffixes {
		assert.NotContains(t, s, "/")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/deprecation/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement the package**

Create `internal/deprecation/deprecation.go`:

```go
// Package deprecation emits user-visible signals when deprecated
// k8s.ngrok.com/* annotation keys are in use, so users can find and migrate
// their manifests before the legacy read support is removed. See
// docs/v1-migration-guide.md.
//
// LEGACY-PREFIX-MIGRATION: BEGIN (package scope — read-side cleanup deletes
// this entire package and its call sites)
package deprecation

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
)

// ReasonLegacyAnnotation is the event reason for legacy-prefix annotation
// hits. Events expire, so this is an immediate signal for recently
// reconciled objects, not a complete inventory:
//
//	kubectl get events -A --field-selector reason=LegacyAnnotation
const ReasonLegacyAnnotation = "LegacyAnnotation"

// EventRecorder is the narrow slice of events.EventRecorder this package
// needs. A nil recorder degrades to log-only.
type EventRecorder interface {
	Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...any)
}

// userFacingAnnotationSuffixes are the user-written annotation suffixes that
// ScanAnnotations checks under the legacy prefix.
var userFacingAnnotationSuffixes = []string{
	"url",
	"mapping-strategy",
	"traffic-policy",
	"pooling-enabled",
	"bindings",
	"metadata",
	"description",
	"app-protocols",
}

// ScanAnnotations emits one Warning event and one log line per
// legacy-prefixed user annotation present on obj. Controllers call it once
// per reconcile of a user-owned object.
func ScanAnnotations(log logr.Logger, recorder EventRecorder, obj client.Object) {
	anns := obj.GetAnnotations()
	if len(anns) == 0 {
		return
	}
	for _, suffix := range userFacingAnnotationSuffixes {
		legacyKey := fmt.Sprintf("%s/%s", parser.LegacyAnnotationsPrefix, suffix)
		if _, ok := anns[legacyKey]; !ok {
			continue
		}
		newKey := fmt.Sprintf("%s/%s", parser.CanonicalAnnotationsPrefix, suffix)
		log.Info("legacy annotation key in use; please migrate",
			"legacyKey", legacyKey, "newKey", newKey)
		if recorder != nil {
			recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonLegacyAnnotation, "Reconcile",
				"annotation %q is deprecated and support for it will be removed in ngrok-operator 1.0; rename it to %q", legacyKey, newKey)
		}
	}
}

// LEGACY-PREFIX-MIGRATION: END
```

Note: `app-protocols` is kept in the scan list even though it is only *consumed* on backend Services of Ingress/Gateway routes (the LoadBalancer path never reads it — `getProtoForServicePort`'s only callers are in `translate_ingresses.go` and `translate_gatewayapi.go`). Backend Services are not reconciled by any controller with a recorder, so in practice legacy `app-protocols` surfaces via the translator log line only (Task 6); if the key happens to sit on a LoadBalancer Service the scan still flags it, which helps users clean up stray keys. The migration guide documents the log-only exception.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/deprecation/ -v`
Expected: PASS. Leave uncommitted.

---

### Task 4: Wire `ScanAnnotations` into the Ingress, Gateway, and Service controllers

**Files:**
- Modify: `internal/controller/ingress/ingress_controller.go` (after the delete block, ~line 116)
- Modify: `internal/controller/gateway/gateway_controller.go` (after the `ShouldHandleGatewayClass` check, ~line 112)
- Modify: `internal/controller/service/controller.go` (after the `shouldHandleService` block, ~line 276)

**Interfaces:**
- Consumes: `deprecation.ScanAnnotations(log, recorder, obj)` (Task 3). Each controller's `Recorder events.EventRecorder` field satisfies `deprecation.EventRecorder`.
- Produces: nothing consumed by later tasks.

The Service and Gateway controllers both have full envtest suites running real managers with `GetEventRecorder` (`internal/controller/service/suite_test.go`; `internal/controller/gateway/suite_test.go` registers the actual `GatewayReconciler` with `Recorder` and `Driver`), so the event wiring gets reconcile-level tests in both (Steps 4 and 6) — the Gateway one also proves the scan sits after the `ShouldHandleGatewayClass` filter. The Ingress reconciler is registered in **no** envtest suite (`internal/controller/ingress/suite_test.go` sets up only the Domain and IPPolicy reconcilers), and standing it up means wiring the driver's sync into envtest with side effects on the existing Domain specs — disproportionate for a one-line call into a function that two other real managers already exercise. Ingress wiring stays compile-covered.

**Coverage hazard from Task 2's const flip:** `internal/controller/service/controller_test.go:55-57` aliases `annotations.URLAnnotation` / `MappingStrategyAnnotation` / `TrafficPolicyAnnotation` as its input annotation keys. When Task 2 flips those consts to `ngrok.com/*`, the entire existing suite silently becomes canonical-prefix coverage — the controller-level legacy path would have zero tests. Step 5 restores explicit legacy coverage.

- [ ] **Step 1: Ingress controller**

In `internal/controller/ingress/ingress_controller.go`, after the `if controller.IsDelete(ingress) { ... }` block (ends line 115) and before the drain check (line 117), insert:

```go
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this scan
	deprecation.ScanAnnotations(log, r.Recorder, ingress)
```

Add `"github.com/ngrok/ngrok-operator/internal/deprecation"` to imports. Placement matters: it runs only for ngrok-class, non-deleted Ingresses (the `UpdateIngress` switch above already filtered other classes).

- [ ] **Step 2: Gateway controller**

In `internal/controller/gateway/gateway_controller.go`, immediately after the `if !ShouldHandleGatewayClass(gwClass) { ... }` early-return block, insert:

```go
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this scan
	deprecation.ScanAnnotations(log, r.Recorder, gw)
```

Add the same import.

- [ ] **Step 3: Service controller**

In `internal/controller/service/controller.go`, after the `if !shouldHandleService(svc) { ... }` block (ends line 275) and before the ports check (line 277), insert:

```go
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this scan
	deprecation.ScanAnnotations(log, r.Recorder, svc)
```

Add the same import.

- [ ] **Step 4: Envtest event-wiring case in the Service suite**

In `internal/controller/service/controller_test.go`, add a context to the existing `Describe("ServiceController")` (follow the suite's existing `ServiceModifier`/`Eventually` style):

```go
	When("a service uses legacy-prefixed annotations", func() {
		It("emits a LegacyAnnotation warning event", func() {
			svc := NewTestService(namespace, LoadBalancer,
				AddAnnotation("k8s.ngrok.com/url", "tcp://"),
			)
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())

			Eventually(func(g Gomega) {
				events := &corev1.EventList{}
				g.Expect(k8sClient.List(ctx, events, client.InNamespace(namespace))).To(Succeed())
				g.Expect(events.Items).To(ContainElement(HaveField("Reason", "LegacyAnnotation")))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("a service uses only canonical-prefixed annotations", func() {
		It("does not emit a LegacyAnnotation warning event", func() {
			svc := NewTestService(namespace, LoadBalancer,
				AddAnnotation("ngrok.com/url", "tcp://"),
			)
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())

			Consistently(func(g Gomega) {
				events := &corev1.EventList{}
				g.Expect(k8sClient.List(ctx, events, client.InNamespace(namespace))).To(Succeed())
				g.Expect(events.Items).NotTo(ContainElement(HaveField("Reason", "LegacyAnnotation")))
			}, "2s", interval).Should(Succeed())
		})
	})
```

Adapt the service-construction helper names to what the suite actually provides (`NewTestService` is illustrative — check the file). Use fresh namespaces per the suite's existing isolation pattern so event lists don't bleed between specs. The canonical case doubles as the "filtering works" assertion — a scan placed before the type/class filters would be caught by running it against a `ClusterIP` variant if the suite makes that easy.

- [ ] **Step 5: Restore explicit legacy-prefix coverage in the Service suite**

Task 2's const flip silently converts the aliased test keys (`controller_test.go:55-57`) to canonical coverage. Add one endpoint-creation spec that uses **literal** legacy keys so the controller-level legacy read path stays covered until read-side cleanup:

```go
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): delete this context
	When("a service uses legacy-prefixed annotations end to end", func() {
		It("still creates endpoints from them", func() {
			svc := NewTestService(namespace, LoadBalancer,
				AddAnnotation("k8s.ngrok.com/url", "tcp://"),
				AddAnnotation("k8s.ngrok.com/mapping-strategy", "endpoints-verbose"),
			)
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
			// assert CloudEndpoint/AgentEndpoint creation exactly as the
			// existing canonical specs do (reuse getCloudEndpoints /
			// getAgentEndpoints helpers)
		})
	})
```

- [ ] **Step 6: Envtest event-wiring case in the Gateway suite**

In `internal/controller/gateway/gateway_controller_test.go`, extend the existing `Describe("Gateway controller", Ordered, ...)`: inside the `When("the gateway's gateway class should be handled by us")` container, add a `When` that customizes `gw` in a `BeforeEach` (the suite's `JustBeforeEach` creates it) with a legacy annotation, and assert the event:

```go
	When("the gateway has a legacy-prefixed annotation", func() {
		BeforeEach(func() {
			gw.Annotations = map[string]string{"k8s.ngrok.com/pooling-enabled": "true"}
		})

		It("emits a LegacyAnnotation warning event", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				events := &corev1.EventList{}
				g.Expect(k8sClient.List(ctx, events, client.InNamespace(gw.Namespace))).To(Succeed())
				g.Expect(events.Items).To(ContainElement(HaveField("Reason", "LegacyAnnotation")))
			}, timeout, interval).Should(Succeed())
		})
	})
```

Add the mirror case under the suite's existing unhandled-gateway-class container (there is one — find it; otherwise create a Gateway whose class is not ours) asserting `Consistently` no `LegacyAnnotation` event — that pins the scan's placement after `ShouldHandleGatewayClass`. Watch event-list bleed: the suite shares a namespace across specs in places, so scope the assertion to the Gateway's namespace or filter by `regarding`/`involvedObject` name if needed.

- [ ] **Step 7: Compile and run controller suites**

Run: `go build ./... && go test ./internal/controller/...`
Expected: PASS. Leave uncommitted.

---

### Task 5: Gateway TLS option keys (`terminate-tls.*`) dual-read

**Files:**
- Modify: `pkg/managerdriver/translate_gatewayapi.go:22-34` (consts), `:1455-1465` (options loop)
- Modify: `internal/controller/gateway/gateway_controller.go` (add `warnIfLegacyTLSOptions`)
- Test: `pkg/managerdriver/translator_test.go` (new table test)

**Interfaces:**
- Consumes: `parser.LegacyAnnotationsPrefix` (Task 1), `deprecation.ReasonLegacyAnnotation` + `deprecation.EventRecorder` (Task 3).
- Produces: `TLSOptionKeyPrefix = "ngrok.com/terminate-tls."`, `LegacyTLSOptionKeyPrefix = "k8s.ngrok.com/terminate-tls."` (both in `pkg/managerdriver`).

- [ ] **Step 1: Write the failing precedence test**

In `pkg/managerdriver/translator_test.go`, add the test below. There are **no existing per-function TLS tests** in this file — TLS coverage today is golden-fixture-only (the `*.yaml` glob harness around line 510), so there is no harness to adapt. Construct the inputs directly: `gatewayTLSTermConfigToIR` is a method on `*translator` (`translate_gatewayapi.go:1345`, signature `(listenerTLS *gatewayv1.ListenerTLSConfig, gateway *gatewayv1.Gateway) (*ir.IRTLSTermination, error)`), and with options-only input it touches neither the store nor secrets — a bare `&translator{log: logr.New(logr.Discard().GetSink())}`, a `&gatewayv1.ListenerTLSConfig{Options: tc.options}`, and a minimal `&gatewayv1.Gateway{}` suffice. The table is the contract:

```go
func TestGatewayTLSTermConfigToIROptionPrecedence(t *testing.T) {
	testCases := []struct {
		name    string
		options map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue
		want    map[string]string
		wantErr bool
	}{
		{
			name: "canonical only",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"ngrok.com/terminate-tls.min_version": "1.3",
			},
			want: map[string]string{"min_version": "1.3"},
		},
		{
			name: "legacy only",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"k8s.ngrok.com/terminate-tls.min_version": "1.2",
			},
			want: map[string]string{"min_version": "1.2"},
		},
		{
			name: "both set canonical wins",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"ngrok.com/terminate-tls.min_version":     "1.3",
				"k8s.ngrok.com/terminate-tls.min_version": "1.2",
			},
			want: map[string]string{"min_version": "1.3"},
		},
		{
			name: "mixed suffixes merge",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"ngrok.com/terminate-tls.min_version":        "1.3",
				"k8s.ngrok.com/terminate-tls.mutual_tls_crt": "abc",
			},
			want: map[string]string{"min_version": "1.3", "mutual_tls_crt": "abc"},
		},
		{
			name: "legacy reserved key rejected",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"k8s.ngrok.com/terminate-tls.server_private_key": "x",
			},
			wantErr: true,
		},
		{
			name: "canonical reserved key rejected",
			options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
				"ngrok.com/terminate-tls.server_private_key": "x",
			},
			wantErr: true,
		},
	}
	// per the prose above: bare translator + &gatewayv1.ListenerTLSConfig{Options: tc.options}
	// + minimal &gatewayv1.Gateway{}; assert tlsTermCfg.ExtendedOptions == tc.want
	_ = testCases
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/managerdriver/ -run OptionPrecedence -v`
Expected: FAIL — canonical-prefix cases produce no options (current prefix const is legacy-only)

- [ ] **Step 3: Implement**

In `pkg/managerdriver/translate_gatewayapi.go`, replace the consts (lines 22-34):

```go
	// Within the gateway, any keys in the tls.options field with this prefix get added to the terminate-tls action
	TLSOptionKeyPrefix = "ngrok.com/terminate-tls."

	// LEGACY-PREFIX-MIGRATION: BEGIN
	// LegacyTLSOptionKeyPrefix is the deprecated form of TLSOptionKeyPrefix,
	// still read during the migration window. Read-side cleanup deletes this
	// const, the legacy branch in the options loop, and the legacy entries in
	// TLSOptionKeyReservedKeys.
	LegacyTLSOptionKeyPrefix = "k8s.ngrok.com/terminate-tls."

	// LEGACY-PREFIX-MIGRATION: END
```

and the reserved-keys list:

```go
	TLSOptionKeyReservedKeys = []string{
		TLSOptionKeyPrefix + "server_private_key",
		TLSOptionKeyPrefix + "server_certificate",
		TLSOptionKeyPrefix + "mutual_tls_certificate_authorities",
		// LEGACY-PREFIX-MIGRATION: BEGIN
		LegacyTLSOptionKeyPrefix + "server_private_key",
		LegacyTLSOptionKeyPrefix + "server_certificate",
		LegacyTLSOptionKeyPrefix + "mutual_tls_certificate_authorities",
		// LEGACY-PREFIX-MIGRATION: END
	}
```

Replace the options loop (lines 1455-1465). Two passes make precedence independent of Go's map iteration order:

```go
	// Canonical-prefixed options always win over legacy-prefixed ones with
	// the same suffix; collect canonical suffixes first so precedence does
	// not depend on map iteration order.
	canonicalSuffixes := map[string]bool{}
	for key := range listenerTLS.Options {
		if strings.HasPrefix(string(key), TLSOptionKeyPrefix) {
			canonicalSuffixes[strings.TrimPrefix(string(key), TLSOptionKeyPrefix)] = true
		}
	}
	for key, val := range listenerTLS.Options {
		var keySuffix string
		switch {
		case strings.HasPrefix(string(key), TLSOptionKeyPrefix):
			keySuffix = strings.TrimPrefix(string(key), TLSOptionKeyPrefix)
		// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this case
		case strings.HasPrefix(string(key), LegacyTLSOptionKeyPrefix):
			keySuffix = strings.TrimPrefix(string(key), LegacyTLSOptionKeyPrefix)
			if canonicalSuffixes[keySuffix] {
				continue
			}
		default:
			continue
		}
		for _, reservedKey := range TLSOptionKeyReservedKeys {
			if string(key) == reservedKey {
				return nil, fmt.Errorf("invalid option supplied to listener tls options. %q is a reserved field and may not be provided here", reservedKey)
			}
		}
		tlsTermCfg.ExtendedOptions[keySuffix] = string(val)
	}
```

- [ ] **Step 4: Add the gateway legacy-TLS warning**

In `internal/controller/gateway/gateway_controller.go`, add next to the Task 4 scan call:

```go
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this warning helper
	r.warnIfLegacyTLSOptions(log, gw)
```

and the helper at the bottom of the file:

```go
// LEGACY-PREFIX-MIGRATION: BEGIN (read-side cleanup deletes this helper)

// warnIfLegacyTLSOptions emits one Warning event when any listener on the
// Gateway still uses k8s.ngrok.com/terminate-tls.* TLS option keys.
func (r *GatewayReconciler) warnIfLegacyTLSOptions(log logr.Logger, gw *gatewayv1.Gateway) {
	for _, listener := range gw.Spec.Listeners {
		if listener.TLS == nil {
			continue
		}
		for key := range listener.TLS.Options {
			if strings.HasPrefix(string(key), managerdriver.LegacyTLSOptionKeyPrefix) {
				log.Info("legacy TLS option key in use; please migrate",
					"legacyPrefix", managerdriver.LegacyTLSOptionKeyPrefix,
					"newPrefix", managerdriver.TLSOptionKeyPrefix)
				if r.Recorder != nil {
					r.Recorder.Eventf(gw, nil, v1.EventTypeWarning, deprecation.ReasonLegacyAnnotation, "Reconcile",
						"TLS option keys with the %q prefix are deprecated and support will be removed in ngrok-operator 1.0; rename them to the %q prefix", managerdriver.LegacyTLSOptionKeyPrefix, managerdriver.TLSOptionKeyPrefix)
				}
				return
			}
		}
	}
}

// LEGACY-PREFIX-MIGRATION: END
```

Note: `gateway_controller.go` already imports core as `v1 "k8s.io/api/core/v1"` — use `v1.EventTypeWarning`, not `corev1`. Check for `strings` and `logr` in the existing imports; add as needed.

Unit-test the helper in `internal/controller/gateway/gateway_controller_test.go` (create if absent, package `gateway`), with a fake recorder matching the `deprecation.EventRecorder`-style narrow interface — but note `r.Recorder` is `events.EventRecorder`, so either construct a `GatewayReconciler` with a fake satisfying that interface or extract the loop into a testable function. Table cases:
- canonical-only TLS options → no event
- legacy-only → exactly one event (even with multiple legacy keys across listeners)
- listener with `TLS == nil` → no panic, no event
- nil recorder + legacy key → logs, no panic

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/managerdriver/ -run 'OptionPrecedence|TestTranslate' && go test ./internal/controller/gateway/ -run WarnIfLegacy && go build ./...`
Expected: PASS (existing legacy-prefix golden fixtures still pass via the legacy branch; the gateway `-run` filter executes the new helper test without booting the package's envtest suite). Leave uncommitted.

---

### Task 6: `app-protocols` annotation + `http2` appProtocol value dual-read

**Files:**
- Modify: `pkg/managerdriver/utils.go:189-192` (protocol map), `:354-387` (`getProtoForServicePort`)
- Test: `pkg/managerdriver/utils_test.go`

**Interfaces:**
- Consumes: nothing beyond Task 1 consts (keys are written literally here, matching the file's current style).
- Produces: `AppProtocolsAnnotation = "ngrok.com/app-protocols"`, `LegacyAppProtocolsAnnotation = "k8s.ngrok.com/app-protocols"` (package `managerdriver`).

- [ ] **Step 1: Write failing tests**

`utils_test.go` has **no existing tests** for these two functions — the only coverage is indirect, in `pkg/managerdriver/driver_test.go` (`k8s.ngrok.com/app-protocols` at :301, `appProtocol` values at :354-413). Create focused table-driven tests in `pkg/managerdriver/utils_test.go`:

```go
func TestGetProtoForServicePort(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		want        ir.IRProtocol
		wantErr     bool
	}{
		{
			name:        "canonical only",
			annotations: map[string]string{"ngrok.com/app-protocols": `{"p":"HTTPS"}`},
			want:        ir.IRProtocol_HTTPS,
		},
		{
			name:        "legacy only falls back",
			annotations: map[string]string{"k8s.ngrok.com/app-protocols": `{"p":"TLS"}`},
			want:        ir.IRProtocol_TLS,
		},
		{
			name: "both present canonical wins",
			annotations: map[string]string{
				"ngrok.com/app-protocols":     `{"p":"HTTPS"}`,
				"k8s.ngrok.com/app-protocols": `{"p":"TCP"}`,
			},
			want: ir.IRProtocol_HTTPS,
		},
		{
			name: "canonical present but invalid does not fall back",
			annotations: map[string]string{
				"ngrok.com/app-protocols":     `not-json`,
				"k8s.ngrok.com/app-protocols": `{"p":"TCP"}`,
			},
			wantErr: true,
		},
		{
			// Empty-as-unset is the pre-existing semantic (the current code
			// ignores an empty legacy annotation); making empty error would
			// break upgrades for anyone carrying an empty legacy key. An
			// empty canonical key still shadows the legacy key (presence
			// wins), it just resolves to "unset" → default protocol.
			name: "canonical present but empty shadows legacy and means unset",
			annotations: map[string]string{
				"ngrok.com/app-protocols":     "",
				"k8s.ngrok.com/app-protocols": `{"p":"TCP"}`,
			},
			want: ir.IRProtocol_HTTP,
		},
		{
			name:        "neither present uses default",
			annotations: nil,
			want:        ir.IRProtocol_HTTP,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: "svc", Namespace: "ns", Annotations: tc.annotations,
			}}
			got, err := getProtoForServicePort(logr.Discard(), svc, "p", ir.IRProtocol_HTTP)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetPortAppProtocol(t *testing.T) {
	testCases := []struct {
		name        string
		appProtocol *string
		want        *common.ApplicationProtocol
	}{
		{name: "nil", appProtocol: nil, want: nil},
		{name: "canonical http2", appProtocol: new("ngrok.com/http2"), want: new(common.ApplicationProtocol_HTTP2)},
		{name: "legacy http2", appProtocol: new("k8s.ngrok.com/http2"), want: new(common.ApplicationProtocol_HTTP2)},
		{name: "h2c", appProtocol: new("kubernetes.io/h2c"), want: new(common.ApplicationProtocol_HTTP2)},
		{name: "unknown ignored", appProtocol: new("grpc"), want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}}
			port := &corev1.ServicePort{Name: "p", AppProtocol: tc.appProtocol}
			assert.Equal(t, tc.want, getPortAppProtocol(logr.Discard(), svc, port))
		})
	}
}
```

(`new(...)` here is the generic pointer helper the repo already uses in `driver_test.go:354` — keep whatever form that file uses.) Also add a `ngrok.com/app-protocols` case and a `ngrok.com/http2` appProtocol case to `driver_test.go` — note that file is Ginkgo, not tables: add new `When`/`It` blocks alongside the existing ones (legacy annotation at ~:301, appProtocol values at ~:354-413), mirroring their `BeforeEach` service-mutation style. The existing blocks use literal legacy keys and survive the const flip as legacy coverage — leave them untouched.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/managerdriver/ -run 'Proto' -v`
Expected: FAIL on the new-prefix cases

- [ ] **Step 3: Implement**

In `pkg/managerdriver/utils.go`, update the protocol-value map (line 189):

```go
var knownApplicationProtocols = map[string]common.ApplicationProtocol{
	"ngrok.com/http2": common.ApplicationProtocol_HTTP2,
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the legacy value
	"k8s.ngrok.com/http2": common.ApplicationProtocol_HTTP2,
	"kubernetes.io/h2c":   common.ApplicationProtocol_HTTP2,
}
```

(Keep whatever other entries exist at line 189-192 — only add the `ngrok.com/http2` entry and the marker.)

Add consts near the top of the file (with the file's other consts):

```go
const (
	// AppProtocolsAnnotation maps service port names to protocols, e.g. '{"grpc-port":"http2"}'
	AppProtocolsAnnotation = "ngrok.com/app-protocols"

	// LEGACY-PREFIX-MIGRATION: BEGIN
	// LegacyAppProtocolsAnnotation is the deprecated form of
	// AppProtocolsAnnotation, still read during the migration window.
	LegacyAppProtocolsAnnotation = "k8s.ngrok.com/app-protocols"

	// LEGACY-PREFIX-MIGRATION: END
)
```

Rework the annotation read in `getProtoForServicePort` (lines 354-363). **Presence-based, not empty-string-based** — same precedence rule as the parser: a present-but-empty canonical key wins (and falls through to the default protocol) rather than silently deferring to the legacy value:

```go
func getProtoForServicePort(log logr.Logger, service *corev1.Service, portName string, defaultProtocol ir.IRProtocol) (ir.IRProtocol, error) {
	if service.Annotations != nil {
		annotation, ok := service.Annotations[AppProtocolsAnnotation]
		sourceKey := AppProtocolsAnnotation
		// LEGACY-PREFIX-MIGRATION: BEGIN (read-side cleanup deletes this fallback)
		if !ok {
			if legacyVal, legacyOK := service.Annotations[LegacyAppProtocolsAnnotation]; legacyOK {
				annotation = legacyVal
				sourceKey = LegacyAppProtocolsAnnotation
				log.Info("legacy annotation key in use; please migrate",
					"legacyKey", LegacyAppProtocolsAnnotation, "newKey", AppProtocolsAnnotation,
					"namespace", service.Namespace, "service", service.Name)
			}
		}
		// LEGACY-PREFIX-MIGRATION: END
		if annotation != "" {
```

and update the two hardcoded strings in the log/error messages at lines 358 and 378 to reference `sourceKey` — diagnostics must name the key the user actually wrote (a legacy-only annotation with a bad value should not produce an error blaming `ngrok.com/app-protocols`).

Add a deprecation log for the legacy `appProtocol` **value** in `getPortAppProtocol` (line 195) — without this, users have no signal that the value is going away (there is no annotation key for `ScanAnnotations` to catch; this is a port field):

```go
	proto := *port.AppProtocol
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this legacy-value log
	if proto == "k8s.ngrok.com/http2" {
		log.Info("legacy appProtocol value in use; please migrate",
			"legacyValue", "k8s.ngrok.com/http2", "newValue", "ngrok.com/http2",
			"namespace", service.Namespace, "service", service.Name, "port", port.Name)
	}
	if knownProto, ok := knownApplicationProtocols[proto]; ok {
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/managerdriver/ && go build ./...`
Expected: PASS. Leave uncommitted.

---

### Task 7: Service controller index-key cleanup

**Files:**
- Modify: `internal/controller/service/controller.go:69-71`

**Interfaces:**
- Consumes/Produces: internal only. `TrafficPolicyPath` renames to `TrafficPolicyIndexKey`; `ModuleSetPath` is deleted.

The traffic-policy field indexer (line 189) extracts via `annotations.ExtractNgrokTrafficPolicyFromAnnotations`, so it already dual-reads after Task 1 — no behavior change needed. But its index name is the literal string `metadata.annotations.k8s.ngrok.com/traffic-policy`, which now misdescribes what the index contains, and `ModuleSetPath` (line 70) has zero references.

- [ ] **Step 1: Rename and delete**

Replace lines 69-71:

```go
	// TrafficPolicyIndexKey is the field-index name for Services indexed by the
	// traffic policy their annotation references (either annotation prefix).
	// The name is opaque — it is an index key, not an object field path.
	TrafficPolicyIndexKey = "ngrok-operator.trafficpolicy-by-name"
```

Delete `ModuleSetPath` entirely (dead code — defined but never referenced). Update the two `TrafficPolicyPath` usages (lines 189, 341) to `TrafficPolicyIndexKey`.

- [ ] **Step 2: Compile and test**

Run: `go build ./... && go test ./internal/controller/service/`
Expected: PASS. Leave uncommitted.

---

### Task 8: Golden translator fixtures for the new prefix

**Files:**
- Create: `pkg/managerdriver/testdata/translator/gwapi-gateway-tls-valid-new-prefix.yaml`
- Create: `pkg/managerdriver/testdata/translator/gwapi-gateway-bindings-annotation-new-prefix.yaml`
- Rename (swap): `pkg/managerdriver/testdata/translator/gwapi-gateway-bindings-annotation.yaml` ↔ `gwapi-gateway-bindings-annotation-invalid.yaml` (names don't match content — pre-existing bug)
- Modify: chainsaw e2e fixtures (Step 5) — `tests/chainsaw/loadbalancer-services/test-tls-service.yaml`, `test-tls-custom-domain-service.yaml`, all eight `tests/chainsaw-uninstall/_fixtures/{ingress,gateway}*.yaml`, `tests/chainsaw-uninstall/README.md`; `tests/chainsaw/loadbalancer-services/test-tcp-service.yaml` stays legacy deliberately

The old PR #824 draft's known gap: unit tests covered dual-read, but zero golden-fixture coverage exercised the `ngrok.com/` input path end-to-end. Close it.

- [ ] **Step 1: Create the TLS fixture**

```bash
cp pkg/managerdriver/testdata/translator/gwapi-gateway-tls-valid.yaml \
   pkg/managerdriver/testdata/translator/gwapi-gateway-tls-valid-new-prefix.yaml
```

In the copy, replace every input-side `k8s.ngrok.com/terminate-tls.` with `ngrok.com/terminate-tls.`. Inspect the `expected:` section: if any expected output echoes the annotation keys, update those too; if it only contains derived config, it stays identical to the original fixture's expected output (same input semantics ⇒ same output).

- [ ] **Step 2: Fix the swapped bindings fixture names, then create the annotations fixture**

The two bindings fixtures are misnamed relative to their content (pre-existing bug): `gwapi-gateway-bindings-annotation.yaml`'s header says it tests the **invalid** multiple-bindings rejection, while `gwapi-gateway-bindings-annotation-invalid.yaml`'s header says it tests the **happy path** ("bindings annotation will be configured on generated CloudEndpoint resources"). Swap them back:

```bash
cd pkg/managerdriver/testdata/translator
git mv gwapi-gateway-bindings-annotation.yaml tmp-swap.yaml
git mv gwapi-gateway-bindings-annotation-invalid.yaml gwapi-gateway-bindings-annotation.yaml
git mv tmp-swap.yaml gwapi-gateway-bindings-annotation-invalid.yaml
cd -
```

Verify: after the swap, `head -1` of each file's comment matches its name (`-invalid` = multiple-bindings rejection). The test harness globs `*.yaml`, so renames don't break anything.

Then copy the (now correctly named) happy-path fixture:

```bash
cp pkg/managerdriver/testdata/translator/gwapi-gateway-bindings-annotation.yaml \
   pkg/managerdriver/testdata/translator/gwapi-gateway-bindings-annotation-new-prefix.yaml
```

In the copy, replace input-side `k8s.ngrok.com/` annotation keys (`bindings`, `mapping-strategy`) with `ngrok.com/`. Same expected-output rule as Step 1 — this fixture asserts bindings appear on generated CloudEndpoints, giving real end-to-end output coverage for the new prefix.

- [ ] **Step 3: Ingress + app-protocols fixtures**

Gateway fixtures alone don't exercise the Ingress translation path with canonical inputs. Two more copies:

```bash
cp pkg/managerdriver/testdata/translator/ingress-metadata-description-annotations.yaml \
   pkg/managerdriver/testdata/translator/ingress-metadata-description-annotations-new-prefix.yaml
cp pkg/managerdriver/testdata/translator/ingress-app-protocols.yaml \
   pkg/managerdriver/testdata/translator/ingress-app-protocols-new-prefix.yaml
```

In the metadata-description copy, replace input-side `k8s.ngrok.com/` annotation keys with `ngrok.com/`. `ingress-app-protocols.yaml` has **no** `app-protocols` annotation — its legacy input is the `appProtocol: k8s.ngrok.com/http2` port **field value** (line ~99); in that copy flip it to `ngrok.com/http2` (that's the canonical coverage this fixture exists to give; the `app-protocols` *annotation* keeps legacy golden coverage via the `gwapi-*-upstream*.yaml` fixtures and gets its canonical coverage from Task 6's unit and driver tests — no extra fixture needed). Same expected-output rule as Step 1.

- [ ] **Step 4: Run the golden suite**

Run: `go test ./pkg/managerdriver/ -run TestTranslate -v 2>&1 | tail -20`
Expected: PASS including both `-new-prefix` cases. If a new fixture fails, diff its actual-vs-expected output — a mismatch means an expected block echoed the old keys; fix the expected block, not the code. Leave uncommitted.

- [ ] **Step 5: Chainsaw e2e fixtures**

The e2e suites currently use only legacy user-annotation keys — zero e2e coverage of the canonical prefix, and every one of these would break at once at 1.0 read-side cleanup. Flip them now (they run against this branch's dual-read operator, so they keep passing) and keep exactly one legacy case as deliberate migration coverage:

- `tests/chainsaw/loadbalancer-services/test-tls-service.yaml` and `test-tls-custom-domain-service.yaml`: change `k8s.ngrok.com/url` and `k8s.ngrok.com/mapping-strategy` to `ngrok.com/`.
- All eight `tests/chainsaw-uninstall/_fixtures/ingress*.yaml` / `gateway*.yaml`: change `k8s.ngrok.com/mapping-strategy` to `ngrok.com/mapping-strategy`; update the sentence in `tests/chainsaw-uninstall/README.md` that names the annotation.
- `tests/chainsaw/loadbalancer-services/test-tcp-service.yaml`: **keep** `k8s.ngrok.com/mapping-strategy` and add above it:
  `# LEGACY-PREFIX-MIGRATION (read-side cleanup): deliberate legacy-prefix e2e coverage; flip to ngrok.com/ at 1.0`

Before flipping each directory, `rg 'k8s\.ngrok\.com' <dir>` and check the `chainsaw-test.yaml` asserts: none should reference user-annotation keys (the earlier audit found only fixture inputs; `computed-url` asserts, if any, are fine either way — #821 dual-writes it). e2e runs are cluster-gated, so verification here is static; Task 10's optional smoke test covers live behavior.

---

### Task 9: Documentation

**Files:**
- Modify: `docs/developer-guide/passivity-shims.md` (append to the per-shim catalog, before the `## Per-shim catalog: CRD field renames` section)
- Modify: `docs/v1-migration-guide.md` (new migration section after the IngressClass section, before `## What did *not* change`)

- [ ] **Step 1: Append the shim-catalog entries to `passivity-shims.md`**

```markdown
### User-facing annotations (read-side compatibility)

- **Pattern:** Two-release. These are user-written keys — the operator never
  writes them, so there is no write side and no delete-on-reconcile
  migration. The dual-read *is* the user contract, which places its removal
  at the 1.0 major-version boundary rather than the R3 read-side sweep:
  dropping it in a post-1.0 minor would be a user-visible breaking change.
- **R1 (0.24):** `internal/annotations/parser/parser.go` resolves each key
  via `annotationKeyFor` — canonical `ngrok.com/<suffix>` wins on presence,
  legacy `k8s.ngrok.com/<suffix>` is the fallback. All `Extract*` helpers in
  `internal/annotations/annotations.go` inherit this through the parser with
  no signature changes. The Ingress, Gateway, and Service controllers call
  `deprecation.ScanAnnotations` once per reconcile to emit `LegacyAnnotation`
  Warning events per legacy key present.
- **Cleanup (1.0, read-side):** delete the fallback in `annotationKeyFor`
  and the `LegacyAnnotationsPrefix` const, the entire `internal/deprecation`
  package, and the `ScanAnnotations` call sites.

### Gateway TLS option keys (read-side compatibility)

- **Pattern:** Two-release, read-side only (same rationale as user-facing
  annotations; removal at 1.0).
- **R1 (0.24):** `pkg/managerdriver/translate_gatewayapi.go` reads both
  `ngrok.com/terminate-tls.*` and `k8s.ngrok.com/terminate-tls.*`; when both
  prefixes define the same option suffix the canonical key wins,
  deterministically (canonical suffixes are collected before the merge loop
  so precedence never depends on map iteration order). The Gateway controller
  emits a single `LegacyAnnotation` Warning event per reconcile via
  `warnIfLegacyTLSOptions` when any listener uses legacy keys.
- **Cleanup (1.0, read-side):** delete `LegacyTLSOptionKeyPrefix`, the legacy
  case in the options loop, the legacy reserved-key entries, and
  `warnIfLegacyTLSOptions`.

### Service `app-protocols` annotation and `http2` appProtocol value (read-side compatibility)

- **Pattern:** Two-release, read-side only (removal at 1.0).
- **R1 (0.24):** `pkg/managerdriver/utils.go::getProtoForServicePort` reads
  `ngrok.com/app-protocols` (presence-based) and falls back to
  `k8s.ngrok.com/app-protocols`; `knownApplicationProtocols` accepts both
  `ngrok.com/http2` and `k8s.ngrok.com/http2` port `appProtocol` values,
  with a deprecation log on the legacy value in `getPortAppProtocol`. Both
  are read only from backend Services of Ingress/Gateway routes, in
  translator hot paths with no event recorder — legacy hits are log-only
  (`legacy annotation key in use` / `legacy appProtocol value in use`).
- **Cleanup (1.0, read-side):** delete `LegacyAppProtocolsAnnotation`, the
  fallback read, the legacy-value log, and the legacy `k8s.ngrok.com/http2`
  map entry.

### Bindings-forwarder pod identity prefix filter (read-side compatibility)

- **Pattern:** Two-release, read-side only (removal at 1.0).
- **R1 (0.24):** `internal/controller/bindings/forwarder_controller.go::podIdentityFromPod`
  forwards pod annotations under either prefix. Keys are forwarded verbatim,
  so upstream traffic-policy expressions that match on annotation key names
  migrate on the pod owner's schedule, not the operator's.
- **Cleanup (1.0, read-side):** drop the legacy prefix match.
```

- [ ] **Step 2: Add the user-facing section to `v1-migration-guide.md`**

Insert after the IngressClass section (line 165), before `## What did *not* change`:

```markdown
### User-set annotations: `k8s.ngrok.com/` → `ngrok.com/`

Status: in progress. 0.24 reads both prefixes; **1.0 reads `ngrok.com/`
only**. Unlike the operator-written keys above, these annotations live in
*your* manifests — the operator cannot migrate them for you, so the legacy
prefix's removal lands exactly at the 1.0 major version.

#### What changes for you

These are the affected keys. The 0.24 operator reads both prefixes; if both
are present on the same object, the `ngrok.com/` value wins.

| Legacy                                  | New                                | Applies to                |
| --------------------------------------- | ---------------------------------- | ------------------------- |
| `k8s.ngrok.com/url`                     | `ngrok.com/url`                    | Service (LoadBalancer)    |
| `k8s.ngrok.com/mapping-strategy`        | `ngrok.com/mapping-strategy`       | Service, Ingress, Gateway |
| `k8s.ngrok.com/traffic-policy`          | `ngrok.com/traffic-policy`         | Service, Ingress, Gateway |
| `k8s.ngrok.com/pooling-enabled`         | `ngrok.com/pooling-enabled`        | Service, Ingress, Gateway |
| `k8s.ngrok.com/bindings`                | `ngrok.com/bindings`               | Service, Ingress, Gateway |
| `k8s.ngrok.com/metadata`                | `ngrok.com/metadata`               | Ingress, Gateway          |
| `k8s.ngrok.com/description`             | `ngrok.com/description`            | Ingress, Gateway          |
| `k8s.ngrok.com/app-protocols`           | `ngrok.com/app-protocols`          | Service backing an Ingress / Gateway route |
| `k8s.ngrok.com/terminate-tls.<option>`  | `ngrok.com/terminate-tls.<option>` | Gateway listener TLS options |

The Service port `appProtocol` field value `k8s.ngrok.com/http2` also has a
new spelling, `ngrok.com/http2`; both are recognized through 0.24.

Pod annotations forwarded as bindings pod identity are also affected: the
forwarder passes along pod annotations under either prefix during the
migration window, but keys are forwarded **verbatim** — if your ngrok
traffic-policy expressions match on `k8s.ngrok.com/*` pod-annotation keys,
update the pod annotations and the policy expressions together; from 1.0 the
forwarder only passes `ngrok.com/*` keys.

**Labels:** there are no user-written ngrok-prefixed labels. This was
audited during this migration — the only prefixed label families are the
operator-written controller and bindings labels, covered by the
operator-written keys migration above.

#### How to migrate

The default, rollback-safe procedure:

1. On 0.24, **add** each `ngrok.com/` key alongside its `k8s.ngrok.com/`
   twin with the same value. With both present the operator uses the
   `ngrok.com/` key, and a rollback to a pre-0.24 operator still reads the
   legacy one — behavior is identical on both sides of a rollback.
2. Once rolling back below 0.24 is no longer possible, delete the legacy
   keys.
3. Finish both steps before upgrading to 1.0 — from 1.0 the operator reads
   `ngrok.com/` only.

If a rollback below 0.24 is already ruled out (or you can roll your
manifests back together with the operator), a straight rename is
equivalent. The two-step dance exists only because a pre-0.24 operator
silently ignores `ngrok.com/*` keys — after a rollback an endpoint would
keep serving, but without its traffic policy, bindings, or URL settings.

**`appProtocol` cannot dual-key:** `Service.spec.ports[].appProtocol` is a
single scalar value, so the recipe above does not apply to it. Switching a
port from `k8s.ngrok.com/http2` to `ngrok.com/http2` and then rolling back
below 0.24 silently drops HTTP/2 for that upstream. Keep the legacy value
until a rollback below 0.24 is ruled out, then switch it — before 1.0.

#### Finding legacy keys

When the operator reconciles an Ingress, Gateway, or LoadBalancer Service
it manages that carries legacy-prefixed annotations, it emits a Warning
event with reason `LegacyAnnotation`:

    kubectl get events -A --field-selector reason=LegacyAnnotation

Treat events as a best-effort immediate signal, **not** as proof your
cluster is ready for 1.0: they expire (typically after an hour), objects
that fail earlier reconcile checks are not scanned, and several surfaces
are log-only (see the exceptions below). The scan is also key-based, not
kind-aware — a stray legacy key that does nothing on that resource kind is
still flagged; deleting it is as valid as renaming it. For a complete
point-in-time inventory, audit directly:

```sh
# Ingresses, Gateways, and Services with legacy-prefixed annotations
for kind in ingress gateway service; do
  kubectl get "$kind" -A -o json | jq -r --arg k "$kind" \
    '.items[] | select((.metadata.annotations // {}) | keys | any(startswith("k8s.ngrok.com/"))) | "\($k) \(.metadata.namespace)/\(.metadata.name)"'
done

# Gateway listeners with legacy TLS option keys
kubectl get gateway -A -o json | jq -r \
  '.items[] | select([.spec.listeners[]?.tls.options // {} | keys[]] | any(startswith("k8s.ngrok.com/"))) | "gateway \(.metadata.namespace)/\(.metadata.name)"'

# Service ports with the legacy appProtocol value
kubectl get service -A -o json | jq -r \
  '.items[] | select([.spec.ports[]?.appProtocol] | any(. == "k8s.ngrok.com/http2")) | "service \(.metadata.namespace)/\(.metadata.name)"'
```

> **Exceptions (no events):**
>
> - `k8s.ngrok.com/app-protocols` and the `k8s.ngrok.com/http2` appProtocol
>   value are read from the backend Service of an Ingress or Gateway route —
>   those Services are not reconciled directly, so legacy use surfaces in the
>   operator logs only. Grep the logs for `legacy annotation key in use` and
>   `legacy appProtocol value in use`.
> - Legacy-prefixed **pod annotations** forwarded as bindings pod identity
>   produce no events or logs (they are read per connection on a hot path).
>   Audit for them directly:
>
>   ```sh
>   kubectl get pods -A -o json | jq -r \
>     '.items[] | select((.metadata.annotations // {}) | keys | any(startswith("k8s.ngrok.com/"))) | "\(.metadata.namespace)/\(.metadata.name)"'
>   ```

#### Action required, by release

| Release | Reads | What you do |
| ------- | ----- | ----------- |
| 0.24 (this) | Both prefixes | Add `ngrok.com/` keys alongside the legacy ones (see *How to migrate*); drop the legacy keys once rollback below 0.24 is ruled out. Use the `LegacyAnnotation` events and the audit commands above to find stragglers. |
| 1.0 | `ngrok.com/` only | Confirm no `k8s.ngrok.com/` annotation keys remain in your manifests. The operator no longer reads them. |
```

- [ ] **Step 3: Sanity-check doc links**

Run: `grep -n "migration-v1-prefix" -r docs/ internal/ pkg/ AGENTS.md`
Expected: no hits (that dead path was a bug in the old WIP branch; must not be reintroduced).

Out of repo, not part of this branch: the ngrok-docs website documents the `k8s.ngrok.com/*` annotation keys and needs the same rename plus a migration note when R1 ships — track as a follow-up item on K8SOP-273/K8SOP-268.

---

### Task 10: Full verification sweep

**Files:** none (verification only)

- [ ] **Step 1: Full build + tests**

Run: `make build && make test`
Expected: PASS, zero failures.

- [ ] **Step 2: Sentinel audit**

Everything on this branch is uncommitted and the new files (`internal/deprecation/`, new fixtures) are **untracked** — `git grep` skips untracked files and would silently miss them. Use `rg`:

Run: `rg -n 'LEGACY-PREFIX-MIGRATION' --glob '!docs/superpowers' --glob '!resurrect-research.md'`
Expected: every new hit from this branch is in the files this plan touches, each with a cleanup kind (`read-side cleanup` or a package/block-scope note). Confirm no pre-existing markers (from #819/#820/#821) were modified: `git diff HEAD -- internal/controller/labels internal/util/k8s.go internal/store/store.go internal/controller/bindings/boundendpoint_controller.go` should be empty.

- [ ] **Step 3: Legacy-read completeness audit**

Run: `rg -n 'k8s\.ngrok\.com' --type go --glob '!*_test.go' --glob '!api/**' --glob '!pkg/managerdriver/testdata/**'`
Expected: every hit is either (a) inside a `LEGACY-PREFIX-MIGRATION` block/line, (b) one of the untouched #819/#820/#821 shims, or (c) a CRD API-group string (`ingress.k8s.ngrok.com` / `ngrok.k8s.ngrok.com` / `bindings.k8s.ngrok.com` — out of scope). Anything else is a missed read site — fix it before finishing.

Then sweep the e2e fixtures (this migration's original audit was Go-only and missed them):

Run: `rg -n 'k8s\.ngrok\.com' tests/`
Expected: only API-group strings, `k8s.ngrok.com/finalizer` (#820 window), `k8s.ngrok.com/controller-*` labels and `ingress-controller` controllerName values (separate surfaces), and the one sentinel-marked `test-tcp-service.yaml` legacy annotation from Task 8 Step 5. Any other user-annotation key is a miss.

- [ ] **Step 4: Manual smoke test (optional, if a cluster is handy)**

```bash
# with a kind/dev cluster and make deploy:
kubectl annotate service some-lb-svc k8s.ngrok.com/traffic-policy=test --overwrite
kubectl get events -A --field-selector reason=LegacyAnnotation
```

Expected: one Warning event naming the legacy key and its `ngrok.com/` replacement.

- [ ] **Step 5: Summarize the diff for review**

Plain `git diff --stat` misses two things here: the fixture renames from Task 8 are **staged** (`git mv` stages), and the new files are **untracked**. Use:

```bash
git status --short
git diff HEAD --stat
```

Report the combined file list (modified + staged renames + untracked new files) to the reviewer. Everything stays uncommitted per the global constraint — staged-but-uncommitted is fine.
