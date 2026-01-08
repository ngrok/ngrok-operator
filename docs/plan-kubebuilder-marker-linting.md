# Kubebuilder Marker Linting - Tech Design

## Problem Statement

Kubebuilder markers (e.g., `+kubebuilder:validation:Required`) are parsed from Go comments to generate CRD manifests. However, **typos in these markers fail silently**:

```go
// +kube:validation:Required   // ❌ Typo - silently ignored
// +kubebuilder:validation:Required  // ✅ Correct
```

This is particularly problematic because:
- Case sensitivity matters (`enum` vs `Enum`)
- Prefix typos are easy (`+kube:` vs `+kubebuilder:`)
- Invalid argument patterns are ignored without warning
- No feedback until you notice missing CRD constraints in production

### Root Cause

controller-gen's marker system is **designed to ignore unknown markers**. From [GitHub issue #887](https://github.com/kubernetes-sigs/controller-tools/issues/887):

> "controller-gen has a wide variety of markers that don't start with +kubebuilder: (e.g. +output, +groupName, +optional). So there's no way to check a +marker against a known list of registered markers."

This is intentional - multiple tools share the same source files, each ignoring markers they don't recognize.

---

## Experimental Findings

We ran experiments with controller-gen to understand exactly what it catches vs silently ignores.

### Test Cases and Results

| Test | Marker | Result | Line |
|------|--------|--------|------|
| Wrong prefix | `+kube:validation:Enum=a;b;c` | ❌ **Silently ignored** | - |
| Lowercase enum | `+kubebuilder:validation:enum=a;b;c` | ❌ **Silently ignored** | - |
| Correct enum | `+kubebuilder:validation:Enum=a;b;c` | ✅ Works | - |
| Comma-separated enum | `+kubebuilder:validation:Enum=a,b,c` | ⚠️ **Error**: `extra arguments provided: ",b,c"` | 29 |
| Typo in marker name | `+kubebuilder:validtion:Required` | ❌ **Silently ignored** | - |
| Unclosed quote | `+kubebuilder:validation:Pattern="abc` | ⚠️ **Error**: `literal not terminated` | 37 |
| Invalid regex | `+kubebuilder:validation:Pattern=\`[abc\`` | ❌ **Silently ignored** (not validated) | - |
| Pattern with `:=` | `+kubebuilder:validation:Pattern:=\`^[a-z]+$\`` | ✅ Works | - |
| Missing rule arg | `+kubebuilder:validation:XValidation:message="must be valid"` | ⚠️ **Error**: `missing argument "rule"` | 49 |
| Made up marker | `+kubebuilder:foobar:baz=qux` | ❌ **Silently ignored** | - |
| Type marker on field | `+kubebuilder:storageversion` | ❌ **Silently ignored** | - |
| Maximum no value | `+kubebuilder:validation:Maximum` | ⚠️ **Error**: `missing argument ""` | 61 |
| Maximum string value | `+kubebuilder:validation:Maximum=abc` | ⚠️ **Error**: `expected integer or float, got "abc"` | 65 |

### Key Insights

**controller-gen DOES catch:**
- ✅ Syntax errors (unclosed quotes, unterminated literals)
- ✅ Type mismatches (string where number expected)
- ✅ Missing required arguments on known markers
- ✅ Malformed argument syntax (wrong separators)

**controller-gen SILENTLY IGNORES:**
- ❌ Wrong prefix (`+kube:` instead of `+kubebuilder:`)
- ❌ Typos in marker names (`+kubebuilder:validtion:`)
- ❌ Case sensitivity (`enum` vs `Enum`)
- ❌ Completely made-up markers (`+kubebuilder:foobar:baz`)
- ❌ Markers on wrong targets (type marker on field)
- ❌ Invalid regex patterns (not semantically validated)

### Implications for Linting

This is **better than expected**. controller-gen already validates:
1. Argument syntax and types for **known markers**
2. Required arguments for **known markers**

The gap is specifically:
1. **Unknown marker names** (typos, wrong case) → our primary target
2. **Wrong prefixes** (different namespace entirely)
3. **Wrong target placement** (field vs type)

The `:=` vs `=` syntax concern from the GitHub issue is a non-issue—both work.

---

## Community Landscape

### What Exists Today

| Solution | Catches Typos? | Notes |
|----------|----------------|-------|
| **kube-api-linter (KAL)** | ❌ No | Validates API conventions, not marker syntax |
| **controller-gen** | ❌ No | Silently ignores unknown markers by design |
| **Major operators** (cert-manager, ArgoCD, Crossplane) | ❌ No | Rely on code review |
| **DIY grep scripts** | ⚠️ Brittle | Simple patterns, hard to maintain |

### Related GitHub Issues

- [controller-tools#887](https://github.com/kubernetes-sigs/controller-tools/issues/887) - Closed, deemed "hard to fix"
- [controller-tools#1126](https://github.com/kubernetes-sigs/controller-tools/issues/1126) - Similar request, went stale

### Marker Ecosystem

Multiple tools use comment markers with different prefixes:

| Tool | Prefix | Example |
|------|--------|---------|
| controller-gen | `+kubebuilder:` | `+kubebuilder:validation:Required` |
| deepcopy-gen | `+k8s:deepcopy-gen` | `+k8s:deepcopy-gen=package` |
| client-gen | `+genclient` | `+genclient:noStatus` |
| Go toolchain | `//go:` | `//go:generate` |

**Key insight**: Each tool only knows its own markers. A universal "error on unknown" approach would break multi-tool workflows.

---

## Proposed Solution

### Overview

Build a **`go/analysis`-based linter** (as a golangci-lint module plugin + standalone tool) that:

1. **Reuses controller-tools' marker registry** to know valid `+kubebuilder:` markers
2. **Flags unknown kubebuilder markers** (catches typos like `+kubebuilder:validtion`)
3. **Warns on suspicious prefixes** (catches `+kube:validation` vs `+kubebuilder:validation`)
4. **Validates marker placement** (field-only markers on types, etc.)

### Why This Approach

| Factor | Benefit |
|--------|---------|
| **Uses official registry** | Not brittle - auto-updates with controller-tools |
| **go/analysis pattern** | Community standard for Go linters |
| **golangci-lint integration** | Works with existing CI/IDE workflows |
| **Configurable allowlist** | Won't break teams with custom markers |

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    pkg/markerlint                            │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              analysis.Analyzer                       │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │    │
│  │  │ Comment     │→ │ Marker      │→ │ Registry    │  │    │
│  │  │ Scanner     │  │ Parser      │  │ Validator   │  │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  │    │
│  └─────────────────────────────────────────────────────┘    │
│                            ↓                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  controller-tools/pkg/markers.Registry               │   │
│  │  (crd/markers, webhook/markers, schemapatcher/...)   │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
           ↙                              ↘
┌──────────────────────┐        ┌──────────────────────────┐
│ cmd/marker-lint      │        │ golangci-lint module     │
│ (standalone CLI)     │        │ plugin integration       │
└──────────────────────┘        └──────────────────────────┘
```

---

## Lint Rules

### KB001: Unknown kubebuilder marker

```go
// +kubebuilder:validtion:Required  // ❌ KB001: unknown marker "kubebuilder:validtion:Required"
```

Severity: **Error**
How: Look up marker name in controller-tools registry

### KB002: Invalid marker target

```go
type MySpec struct {
    // +kubebuilder:storageversion  // ❌ KB002: marker only valid on types, not fields
    Field string
}
```

Severity: **Error**
How: Check `Definition.Target` matches AST node kind

### KB003: Unknown marker prefix (optional)

```go
// +kube:validation:Required  // ⚠️ KB003: unknown prefix "kube", did you mean "kubebuilder"?
```

Severity: **Warning** (configurable)
How: Check against allowlist of known prefixes

### KB004: Deprecated marker (future)

```go
// +kubebuilder:validation:XPreserveUnknownFields  // ⚠️ KB004: deprecated, use X instead
```

Severity: **Warning**
How: Maintain deprecation list

---

## Implementation Plan

### Phase 0: Immediate Fix ⬅️ START HERE

Fix the existing `+kube:validation` typos in the codebase:

```bash
# Find all instances
grep -rn "+kube:" api/

# Current known issues in kubernetesoperator_types.go:
# Line 67: +kube:validation:Enum=registered;error;pending
# Line 72: +kube:validation:Optional
```

### Phase 1: Minimal Viable Lint

**Goal**: Catch common prefix typos with zero false positives.

1. Create `tools/marker-lint/main.go`:
   ```go
   // Simple pattern matching - no controller-tools dependency
   var suspiciousPatterns = map[string]string{
       `^\s*//\s*\+kube:`:          "did you mean '+kubebuilder:'?",
       `^\s*//\s*\+kuberbuilder:`:  "did you mean '+kubebuilder:'? (extra 'r')",
       `^\s*//\s*\+kubebilder:`:    "did you mean '+kubebuilder:'? (missing 'u')",
   }
   ```

2. Add to CI:
   ```yaml
   - name: Lint markers
     run: go run ./tools/marker-lint ./api/...
   ```

3. Add to `make lint` target

**Why start here**: Zero configuration, zero dependencies, catches our actual bug.

### Phase 2: golangci-lint Integration

Convert to a proper `go/analysis` analyzer for IDE integration:

1. Create `pkg/markerlint/analyzer.go` implementing `analysis.Analyzer`
2. Add as golangci-lint custom linter:
   ```yaml
   linters-settings:
     custom:
       kubebuildermarkers:
         path: github.com/ngrok/ngrok-operator/tools/marker-lint-plugin
   ```

**Why**: Better DX with IDE squiggles, unified lint output.

### Phase 3: Registry-Based Validation (optional)

**Only if Phase 1-2 prove insufficient.** Add:

1. Import `sigs.k8s.io/controller-tools/pkg/markers`
2. Validate unknown `+kubebuilder:*` markers against registry
3. Add allowlist configuration for custom markers
4. KB002: Marker target validation (field vs type)

**Trade-offs**: More coverage, but requires configuration for non-standard projects.

### Phase 4: Community Contribution (optional)

If the tool proves useful:

1. Extract to standalone repo
2. Propose to kubernetes-sigs (as KAL extension or separate tool)
3. Engage with [issue #887](https://github.com/kubernetes-sigs/controller-tools/issues/887) discussion

---

## Configuration

### Default Config

```yaml
markerlint:
  # Only lint these directories
  api-dirs:
    - api

  # Known safe prefixes (won't warn)
  allowed-prefixes:
    - kubebuilder
    - k8s
    - genclient
    - groupName
    - versionName
    - optional
    - default

  # Enable unknown prefix warnings
  check-unknown-prefix: true
```

### Project-Specific Overrides

```yaml
markerlint:
  allowed-prefixes:
    - kubebuilder
    - k8s
    - ngrok  # Custom company markers
```

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| **False positives on custom prefixes** | Configurable allowlist |
| **Version skew with controller-tools** | Document version matching; include in error message |
| **AST node matching edge cases** | Start with high-confidence cases; allow disabling KB002 |
| **Maintenance burden** | Uses official registry, not hand-maintained patterns |

---

## Critical Question: Is This Approach Valid?

### The Core Problem

From [GitHub issue #887](https://github.com/kubernetes-sigs/controller-tools/issues/887):

> "Developers often implement their own markers using kubebuilder as a library, and kubebuilder itself provides its own markers (e.g., `kubebuilder:scaffold:`). This means that just because a marker is not defined in controller-tools, it does not imply that it is invalid."
>
> "This raises a critical question: **How could we effectively differentiate between valid, user-defined markers and those that are genuinely invalid?**"

### Why This Is Hard

1. **The `+kubebuilder:` namespace is not reserved** - Anyone can define custom markers with this prefix
2. **No central registry** - controller-tools doesn't know about all valid markers
3. **Extensibility by design** - The marker system was built to be extended

### Our Approach: Opt-In Strictness with Allowlists

We acknowledge this is an **opinionated trade-off**. Our proposed solution:

```yaml
markerlint:
  # Strict mode for kubebuilder: prefix
  strict-kubebuilder-prefix: true

  # But allow specific custom markers
  allowed-markers:
    - "kubebuilder:scaffold:*"   # kubebuilder's own scaffolding markers
    - "kubebuilder:mycustom:*"   # team-specific markers
```

**This shifts the question from:**
> "Is this marker invalid?" (unknowable)

**To:**
> "Is this marker in our known-good list?" (answerable)

### When This Works Well

| Scenario | Works? | Notes |
|----------|--------|-------|
| Standard kubebuilder project | ✅ Yes | Default allowlist covers all official markers |
| Using kubebuilder as a library | ⚠️ Needs config | Add custom markers to allowlist |
| Heavy marker customization | ❌ Too noisy | May want to disable strict mode |

### When This Breaks Down

1. **Teams defining many custom `+kubebuilder:` markers** - High false positive rate
2. **Adopting new controller-tools versions** - New markers flagged until linter updated
3. **Monorepos with mixed conventions** - Different teams, different markers

### Honest Assessment

| Aspect | Assessment |
|--------|------------|
| **Catches our actual bug** (`+kube:validation`) | ✅ Yes, definitively |
| **Catches typos in known markers** | ✅ Yes |
| **Works without configuration** | ⚠️ Mostly (for standard projects) |
| **Zero false positives guarantee** | ❌ No (by design) |
| **Community-standard solution** | ❌ No (novel approach) |

### Why We Think It's Still Worth It

1. **The failure mode is safe** - False positives are annoying but fixable; false negatives (our current state) cause production bugs
2. **Configuration is one-time** - Add custom markers to allowlist once
3. **Default covers 95%+ of use cases** - Most projects use only official markers
4. **Better than nothing** - The community has no solution; this fills the gap

### Alternative: Minimal Viable Lint

If the full approach feels too risky, a **much simpler alternative**:

```go
// Just check for common typos, no registry needed
var suspiciousPatterns = []string{
    `\+kube:`,           // missing "builder"
    `\+kuberbuilder:`,   // extra 'r'
    `\+kubebilder:`,     // missing 'u'
    `\+kubebuidler:`,    // transposed letters
}
```

**Pros**: Zero false positives, zero configuration
**Cons**: Only catches specific known typos, not general case

### Recommendation

Start with the **minimal viable lint** (pattern matching for common typos), and iterate toward the registry-based approach if we find we need more coverage. This gives us:

1. ✅ Immediate value (catches `+kube:` typo)
2. ✅ Zero configuration
3. ✅ Zero false positives
4. ✅ Room to grow

---

## Alternatives Considered

### 1. Simple grep/regex script

**Pros**: Quick to implement
**Cons**: Brittle, hard to maintain, no IDE integration
**Verdict**: ❌ Doesn't scale

### 2. Upstream change to controller-gen

**Pros**: Built-in, benefits everyone
**Cons**: Conflicts with design philosophy; already rejected in #887
**Verdict**: ❌ Not feasible

### 3. Unit tests for each marker

**Pros**: Catches issues per-field
**Cons**: Tedious, doesn't scale, no coverage guarantee
**Verdict**: ❌ Too manual

### 4. Fork controller-tools

**Pros**: Full control
**Cons**: Massive maintenance burden
**Verdict**: ❌ Overkill

---

## Success Criteria

1. ✅ Catches `+kube:validation` → `+kubebuilder:validation` typos
2. ✅ Catches case sensitivity issues (`enum` vs `Enum`)
3. ✅ Integrates with existing golangci-lint workflow
4. ✅ Zero false positives on current codebase (after allowlist config)
5. ✅ < 5s added to CI time
6. ✅ Maintainable without hand-curated marker lists

---

## References

- [controller-tools marker package](https://pkg.go.dev/sigs.k8s.io/controller-tools/pkg/markers)
- [GitHub issue #887](https://github.com/kubernetes-sigs/controller-tools/issues/887)
- [kube-api-linter](https://github.com/kubernetes-sigs/kube-api-linter)
- [gocheckcompilerdirectives](https://github.com/leighmcculloch/gocheckcompilerdirectives) - similar pattern
- [golangci-lint module plugin docs](https://golangci-lint.run/plugins/module-plugins/)
