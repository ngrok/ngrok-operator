# CRD Documentation Generation: Implementation Critique

**Status**: Implementation complete and working ✅  
**Generated**: 7 CRDs producing markdown and JSON successfully

---

## Executive Summary

The implementation is **functionally correct** and follows good patterns, but has **a few brittle spots** that will cause problems when:
- API versions evolve (v1alpha1 → v1beta1)
- CRD filenames change
- Multiple versions are served simultaneously

**Recommended**: Make 2-3 small robustness fixes now (1 hour), defer everything else.

---

## Critical Assessment

### ✅ What's Right

1. **Architecture is sound**: Single source (CRD YAML) → dual output (markdown + JSON) prevents drift
2. **Tool choices correct**: `crdoc` and `yq` are appropriate for this use case
3. **Makefile patterns followed**: Integrates cleanly with existing `tools/make/*.mk` structure
4. **Works today**: All 7 CRDs generate successfully

### ⚠️ What's Fragile

1. **Hard-coded version**: `version=v1alpha1` will break when v1beta1 is introduced
2. **Filename parsing**: Relies on naming convention (`group.domain_kinds.yaml`) instead of reading from CRD content
3. **Plural vs singular**: Filenames use plurals (`boundendpoints.md`) but Kind is singular (`BoundEndpoint`)
4. **Non-deterministic ordering**: No sort on file list causes inconsistent output
5. **Version mismatch**: Plan says v0.7.1, code uses v0.6.4
6. **Cleanup target**: `docs-clean` doesn't match what's generated (targets wrong paths)

### 🚨 Will Break When

**Multi-version CRDs** (serving both v1alpha1 and v1beta1):
```yaml
spec:
  versions:
  - name: v1alpha1
    served: true
    storage: false
  - name: v1beta1
    served: true
    storage: true
```

Current code only creates `docs/crds/ngrok/v1alpha1/` — where does v1beta1 go?

**CRD filename changes**: If helm generation changes naming (e.g., adds namespace or changes delimiter), parsing breaks.

**Manual condition files**: `docs-clean` will delete them if placed in wrong location, or leave generated files if placed in right location.

---

## Detailed Critique

### 1. Technical Soundness ✅

**Single-source approach**: ✅ Correct
- Controller-gen CRDs are already validated by CI
- Using them as source of truth prevents drift
- Both markdown and JSON derive from same data

**Tool choices**: ✅ Appropriate
- **crdoc**: Designed for CRD YAML → markdown, actively maintained
- **yq**: Standard YAML/JSON tool, minimal overhead
- Both install cleanly via Go

**Alternative considered and rejected**: Custom Go generator. Would provide more control but adds maintenance burden without clear benefit at this stage.

### 2. Implementation Quality ⚠️

#### Good Practices ✅

```makefile
# Follows existing patterns
$(call go-install-tool,$(CRDOC),fybrik.io/crdoc,$(CRDOC_VERSION))

# Proper dependencies
docs-generate: manifests crdoc

# CI-ready verification
docs-verify: docs-generate
	@git diff --exit-code docs/crds/
```

#### Issues ⚠️

**Issue 1: Hard-coded API version**

```makefile
version=v1alpha1; \  # ← Hard-coded!
```

**Problem**: When you add v1beta1, this will only generate docs for v1alpha1.

**Fix**: Extract versions from CRD:

```makefile
for ver in $$($(YQ) -r '.spec.versions[] | select(.served==true) | .name' "$$crd"); do
  mkdir -p docs/crds/$$group/$$ver
  $(CRDOC) --resources "$$crd" --output docs/crds/$$group/$$ver/$$kind.md
done
```

---

**Issue 2: Brittle filename parsing**

```makefile
group=$$(basename $$crd | cut -d. -f1);  # ← Assumes filename format
kind=$$(basename $$crd .yaml | cut -d_ -f2);
```

**Problem**: Breaks if helm changes CRD filename format.

**Fix**: Read from CRD content:

```makefile
group=$$($(YQ) -r '.spec.group' "$$crd");
kind=$$($(YQ) -r '.spec.names.kind' "$$crd");
```

**Bonus**: Gets singular form (`BoundEndpoint`) instead of plural (`boundendpoints`).

---

**Issue 3: Non-deterministic ordering**

```makefile
for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do
```

**Problem**: File order varies by filesystem, causing unnecessary diffs.

**Fix**: Sort inputs:

```makefile
for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do
```

---

**Issue 4: Ineffective cleanup**

```makefile
docs-clean:
	@rm -rf docs/crds/*.md docs/generated/
```

**Problem**: 
- Only deletes `docs/crds/*.md` (top-level), not nested files like `docs/crds/ngrok/v1alpha1/*.md`
- Will delete manual condition files if placed in `docs/crds/`

**Fix**: Clean only generated artifacts:

```makefile
docs-clean:
	@rm -rf docs/generated/
	@find docs/crds -name '*.md' -type f ! -name '*-conditions.md' -delete
```

Or better: separate generated and manual docs completely.

---

**Issue 5: Version mismatch**

| Location | Version |
|----------|---------|
| docs-gen-plan.md | v0.7.1 |
| tools/make/_common.mk | v0.6.4 |
| Installed binary | v0.6.2 |

**Problem**: Confusion about which version is intended, potential formatting differences.

**Fix**: Pick one version and use it everywhere. Recommend v0.6.4 (already working).

---

**Issue 6: Template not used**

Plan mentions `docs/templates/crdoc.tmpl` but implementation doesn't reference it.

**Options**:
1. Remove from plan (keep it simple)
2. Wire it up: `$(CRDOC) --template docs/templates/crdoc.tmpl --resources ...`

**Recommendation**: Remove from plan until needed.

---

### 3. Missing Gaps

#### No generated index/README

**Gap**: No `docs/crds/README.md` listing all CRDs.

**Impact**: Developers have to manually browse directory tree.

**Fix** (simple):

```makefile
docs-generate: manifests crdoc
	@echo "# CRD Reference Documentation" > docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do
		group=$$($(YQ) -r '.spec.group' "$$crd");
		kind=$$($(YQ) -r '.spec.names.kind' "$$crd");
		echo "- [$${kind}](./$${group%%.*}/v1alpha1/$$(echo $$kind | tr '[:upper:]' '[:lower:]').md)" >> docs/crds/README.md;
	done
	# ... rest of generation
```

---

#### No condition file validation

**Gap**: Nothing checks that condition files exist or are up-to-date.

**Impact**: Developers can forget to create/update them.

**Fix** (add target):

```makefile
docs-verify-conditions:
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
		group=$$($(YQ) -r '.spec.group' "$$crd"); \
		kind=$$($(YQ) -r '.spec.names.kind' "$$crd" | tr '[:upper:]' '[:lower:]'); \
		if grep -q "Condition" "$$crd"; then \
			if [ ! -f "docs/crds/$${group%%.*}/v1alpha1/$$kind-conditions.md" ]; then \
				echo "Missing: docs/crds/$${group%%.*}/v1alpha1/$$kind-conditions.md"; \
				exit 1; \
			fi; \
		fi; \
	done
```

---

#### No .gitignore entry

**Gap**: Plan says to add `docs/generated/` to `.gitignore` but verification needed.

**Check**: Is it there?

```bash
$ grep -q "docs/generated" .gitignore && echo "✅ Present" || echo "❌ Missing"
```

---

### 4. Condition Documentation Strategy

**Current Approach**: Manual creation of `*-conditions.md` files.

**Assessment**: 
- ✅ Simple, clear, human-readable
- ✅ Works fine with 5-7 CRDs
- ⚠️ Will drift as conditions change
- ⚠️ Scales poorly to 20+ CRDs

**Better Alternative** (defer unless pain increases):

Create a small Go tool (`tools/gen-conditions-docs/`) that:
1. Parses `api/*/v1alpha1/*_conditions.go`
2. Extracts const declarations and their godoc comments
3. Outputs markdown in the same format as manual files
4. Run via `go generate` or make target

**Effort**: Medium (2-3 hours to build, 30 min to integrate)

**Payoff**: Eliminates drift, reduces maintenance burden

**Recommendation**: Keep manual for now, revisit when:
- You add v1beta1 (doubles condition docs)
- Conditions change frequently
- Team size grows (more people forgetting to update)

---

### 5. Example Documentation Strategy

**Current Plan**: Separate example files in `docs/crds/*/v1alpha1/examples/`

**Options Evaluated**:

| Option | Pros | Cons | Recommendation |
|--------|------|------|----------------|
| **Separate files** | Clean, linkable, testable | Can drift from code | ✅ Use this |
| **Inline in godoc** | Guaranteed sync | Bloats Go files | ❌ No |
| **Copy from config/samples/** | Reuses existing | Samples may not be docs-friendly | ⚠️ Maybe |
| **Embed in crdoc template** | Single place | Long examples hard to read | ❌ No |

**Recommendation**: 

1. Create `docs/crds/*/v1alpha1/examples/basic.yaml` manually (one per CRD)
2. Optionally, copy from `config/samples/` if they exist and are suitable:

```makefile
docs-generate-examples:
	@for sample in config/samples/*_v1alpha1_*.yaml; do
		# Parse and copy to appropriate docs location
	done
```

3. Link from generated markdown (requires crdoc template customization)

**Decision**: Start with manual, add automation if examples become numerous.

---

### 6. JSON Format for Docs Site

**Current Implementation**: Raw CRD YAML → JSON via `yq`

**What Docs Site Likely Needs**:

```json
{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": { "name": "..." },
  "spec": {
    "group": "bindings.k8s.ngrok.com",
    "names": {
      "kind": "BoundEndpoint",
      "plural": "boundendpoints",
      "singular": "boundendpoint",
      "shortNames": ["bep"]
    },
    "versions": [
      {
        "name": "v1alpha1",
        "served": true,
        "storage": true,
        "schema": { /* OpenAPI schema */ }
      }
    ]
  }
}
```

**Is this sufficient?** Maybe. Depends on docs site parser.

**Potential Issues**:
- **Too verbose**: Includes internal fields docs site doesn't need
- **Too raw**: Docs site has to understand CRD structure
- **No conditions**: Condition types not in schema
- **No examples**: Examples are separate files

**Better Alternative** (if docs site needs it):

```json
{
  "group": "bindings.k8s.ngrok.com",
  "version": "v1alpha1",
  "kind": "BoundEndpoint",
  "plural": "boundendpoints",
  "shortNames": ["bep"],
  "description": "...",
  "fields": [
    {
      "name": "spec.target",
      "type": "string",
      "required": true,
      "description": "...",
      "example": "myservice.default:8080"
    }
  ],
  "conditions": [
    {
      "type": "Ready",
      "description": "...",
      "reasons": ["BoundEndpointReady", "ServicesNotCreated"]
    }
  ],
  "examples": [
    { "name": "basic", "path": "examples/basic.yaml" }
  ]
}
```

**Recommendation**:
1. **Keep simple conversion now** (raw YAML → JSON)
2. **Add to "Open Questions"** section: "Does docs site need flattened/transformed JSON?"
3. **Wait for feedback** from docs site team
4. **If custom format needed**, build a small Go transformer (effort: Medium, 3-4 hours)

---

### 7. Versioning (Multi-version CRDs)

**Current State**: Hard-coded `v1alpha1`

**Future Scenario**: 

```yaml
# cloudendpoint CRD serves two versions
spec:
  versions:
  - name: v1alpha1
    served: true
    storage: false
  - name: v1beta1
    served: true
    storage: true
```

**What Should Happen**:
```
docs/crds/ngrok/v1alpha1/cloudendpoints.md
docs/crds/ngrok/v1beta1/cloudendpoints.md
```

**What Will Happen** (current code):
```
docs/crds/ngrok/v1alpha1/cloudendpoints.md  # Only v1alpha1
```

**Fix Required**:

```makefile
@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
  group=$$($(YQ) -r '.spec.group' "$$crd"); \
  kind=$$($(YQ) -r '.spec.names.kind' "$$crd"); \
  for ver in $$($(YQ) -r '.spec.versions[] | select(.served==true) | .name' "$$crd"); do \
    mkdir -p docs/crds/$${group%%.*}/$$ver; \
    $(CRDOC) --resources "$$crd" --output docs/crds/$${group%%.*}/$$ver/$$(echo $$kind | tr '[:upper:]' '[:lower:]').md; \
  done; \
done
```

**Additional Consideration**: "Latest" version

Docs site may want a canonical/latest link. Options:
1. Symlink `latest` → version with `storage: true`
2. Duplicate docs in `latest/` directory
3. Let docs site handle via redirect

**Recommendation**: Fix multi-version support now (prevents future breakage), defer "latest" until docs site specifies need.

---

### 8. Maintenance Burden

**For Each API Change**, developer must:

| Task | Effort | Automated? |
|------|--------|-----------|
| Edit Go types | Medium | ❌ Manual |
| Run `make manifests` | None | ✅ Can be pre-commit hook |
| Run `make docs-generate` | None | ✅ Can be pre-commit hook |
| Update condition docs (if changed) | Low-Medium | ❌ Manual |
| Update examples (if needed) | Low | ❌ Manual |
| Commit all changes | None | ✅ Standard git workflow |

**Assessment**: 
- **Acceptable** for current scale (7 CRDs, small team)
- **Pre-commit hooks** could auto-run generation
- **Condition docs** are the main manual burden

**Scalability**:
- At 10-15 CRDs: Still manageable
- At 20+ CRDs: Consider condition auto-generation
- At multiple versions per CRD: Definitely automate conditions

**Recommendation**: Current approach is sustainable, add automation later if pain increases.

---

### 9. CI Integration

**What CI Should Do**:

#### On Pull Requests

```yaml
name: CRD Docs Verification
on:
  pull_request:
    paths:
      - 'api/**'
      - 'helm/ngrok-operator/templates/crds/**'
      - 'tools/make/generate.mk'

jobs:
  docs-verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'  # Use ${{ env.GO_VERSION }} from ci.yaml
      
      - name: Install tools
        run: make crdoc yq
      
      - name: Generate CRDs
        run: make manifests
      
      - name: Generate docs
        run: make docs-generate
      
      - name: Verify docs are current
        run: make docs-verify
```

**Optimizations**:
- Cache `bin/` directory to avoid reinstalling tools
- Only run on paths that affect docs
- Run in parallel with other checks

#### On Releases (Git Tags)

```yaml
name: Release Docs
on:
  release:
    types: [published]

jobs:
  publish-docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Install tools
        run: make yq
      
      - name: Generate JSON
        run: make docs-generate-json
      
      - name: Upload artifact
        uses: actions/upload-artifact@v3
        with:
          name: crd-docs-${{ github.ref_name }}
          path: docs/generated/crds/
      
      # Optionally: Push to docs repo
      - name: Publish to docs site
        run: |
          # Clone docs repo
          # Copy docs/generated/crds/*.json
          # Commit and push
```

**Recommendation**: Add PR verification first, defer release automation until docs site integration is clear.

---

### 10. Potential Failure Modes

#### Failure Mode 1: Version Drift

**Scenario**: Plan says v0.7.1, code uses v0.6.4, installed binary is v0.6.2

**Impact**: 
- Confusion during debugging
- Potential formatting differences
- CI failures if versions mismatch

**Mitigation**: ✅ **Fix now**
```makefile
# In tools/make/_common.mk, change:
CRDOC_VERSION ?= v0.6.4  # Match what's actually installed
```

**Verification**:
```bash
grep -r "CRDOC_VERSION" tools/make/
grep -r "crdoc" docs-gen-plan.md
# Ensure they match
```

---

#### Failure Mode 2: Multi-version CRD

**Scenario**: CRD serves both v1alpha1 and v1beta1

**Impact**: Only v1alpha1 docs generated, v1beta1 silently ignored

**Mitigation**: ✅ **Fix now** (see Versioning section)

---

#### Failure Mode 3: Filename Changes

**Scenario**: Helm chart generation changes CRD filename format

**Example**: 
- Old: `bindings.k8s.ngrok.com_boundendpoints.yaml`
- New: `bindings_boundendpoints.yaml`

**Impact**: Parsing fails, docs not generated

**Mitigation**: ✅ **Fix now** (use yq to read from CRD content, not filename)

---

#### Failure Mode 4: Condition Drift

**Scenario**: Developer adds new condition type, forgets to update `-conditions.md`

**Impact**: Docs incomplete, users confused

**Mitigation**: ⚠️ **Add later**
- CI check for condition presence
- Or auto-generate from Go code

---

#### Failure Mode 5: Example Drift

**Scenario**: API changes, examples no longer valid

**Impact**: Users copy broken examples

**Mitigation**: ⚠️ **Add later**
- Validate examples with `kubectl --dry-run=client`
- Add to CI: `kubectl apply --dry-run=client -f docs/crds/**/examples/`

---

#### Failure Mode 6: JSON Format Mismatch

**Scenario**: Docs site can't parse raw CRD JSON

**Impact**: Docs site breaks, manual intervention needed

**Mitigation**: ⚠️ **Coordinate now**
- Share sample JSON with docs site team
- Get explicit approval on format
- Document format contract in plan

---

#### Failure Mode 7: Tool Installation Fails in CI

**Scenario**: `go install fybrik.io/crdoc` fails due to network/proxy

**Impact**: CI fails randomly

**Mitigation**: ✅ **Already handled**
```makefile
# GOINSECURE allows HTTP (already in your code)
# Alternative: Download pre-built binary from GitHub releases
```

---

#### Failure Mode 8: Non-deterministic Output

**Scenario**: File iteration order differs between machines

**Impact**: Spurious git diffs, CI failures

**Mitigation**: ✅ **Fix now** (add `sort` to file loop)

---

#### Failure Mode 9: Cleanup Deletes Manual Files

**Scenario**: Developer runs `make docs-clean`, deletes hand-written docs

**Impact**: Manual work lost

**Mitigation**: ✅ **Fix now**
```makefile
docs-clean:
	@rm -rf docs/generated/
	@find docs/crds -name '*.md' ! -name '*-conditions.md' ! -name 'README.md' -delete
```

Or better: keep generated and manual docs in separate directories.

---

## Recommended Fixes (Priority Order)

### Priority 1: Must Fix Now ✅

These will cause breakage soon:

1. **Extract metadata from CRD** (not filename)
   ```makefile
   group=$$($(YQ) -r '.spec.group' "$$crd");
   kind=$$($(YQ) -r '.spec.names.kind' "$$crd");
   ```
   **Why**: Prevents breakage if filenames change or don't match expected pattern
   **Effort**: 15 minutes

2. **Loop over served versions**
   ```makefile
   for ver in $$($(YQ) -r '.spec.versions[] | select(.served==true) | .name' "$$crd"); do
   ```
   **Why**: Required for multi-version CRDs (coming soon with v1beta1)
   **Effort**: 15 minutes

3. **Sort file inputs**
   ```makefile
   for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do
   ```
   **Why**: Prevents spurious diffs between machines
   **Effort**: 5 minutes

4. **Fix docs-clean**
   ```makefile
   docs-clean:
   	@rm -rf docs/generated/
   	@find docs/crds -type d -name 'v1*' -exec rm -rf {} +
   ```
   **Why**: Prevents deleting manual files or leaving generated files
   **Effort**: 10 minutes

**Total Effort**: ~45 minutes

---

### Priority 2: Should Fix Soon ⚠️

Improves robustness but not blocking:

5. **Align crdoc version** everywhere
   - Update plan or code to match (pick v0.6.4)
   **Why**: Prevents confusion and formatting differences
   **Effort**: 5 minutes

6. **Generate README index**
   - Auto-create `docs/crds/README.md` listing all CRDs
   **Why**: Better developer experience
   **Effort**: 20 minutes

7. **Verify .gitignore**
   - Ensure `docs/generated/` is present
   **Why**: Prevents committing build artifacts
   **Effort**: 2 minutes

**Total Effort**: ~30 minutes

---

### Priority 3: Can Defer 📅

Nice to have, not urgent:

8. **Condition file validation**
   - Add `make docs-verify-conditions` target
   **When**: After 2-3 CRD additions
   **Effort**: 30 minutes

9. **Auto-generate condition docs**
   - Build Go tool to parse `*_conditions.go`
   **When**: After v1beta1 is added (doubles manual work)
   **Effort**: 2-3 hours

10. **Custom JSON format**
    - Transform CRD to docs site's preferred schema
    **When**: After docs site specifies requirements
    **Effort**: 3-4 hours

11. **Example validation**
    - `kubectl --dry-run=client` on example files
    **When**: After examples are created
    **Effort**: 30 minutes

---

## Revised Makefile Snippets

### Robust docs-generate (with fixes):

```makefile
.PHONY: docs-generate
docs-generate: manifests crdoc yq ## Generate CRD markdown documentation
	@echo "Generating CRD markdown documentation..."
	@mkdir -p docs/crds
	@echo "# CRD Reference Documentation" > docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "Auto-generated API reference for ngrok-operator CRDs." >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do \
		group=$$($(YQ) eval '.spec.group' $$crd); \
		shortgroup=$${group%%.*}; \
		kind=$$($(YQ) eval '.spec.names.kind' $$crd); \
		kindlower=$$(echo $$kind | tr '[:upper:]' '[:lower:]'); \
		for ver in $$($(YQ) eval '.spec.versions[] | select(.served==true) | .name' $$crd); do \
			dir=docs/crds/$$shortgroup/$$ver; \
			mkdir -p $$dir; \
			echo "  Generating $$shortgroup/$$ver/$$kindlower.md..."; \
			$(CRDOC) --resources $$crd --output $$dir/$$kindlower.md; \
			echo "- [$$kind]($$shortgroup/$$ver/$$kindlower.md)" >> docs/crds/README.md; \
		done; \
	done
	@echo "Markdown documentation generated in docs/crds/"
```

### Safe docs-clean:

```makefile
.PHONY: docs-clean
docs-clean: ## Clean generated documentation
	@echo "Cleaning generated documentation..."
	@rm -rf docs/generated/
	@find docs/crds -type d -name 'v1*' -exec rm -rf {} +
	@rm -f docs/crds/README.md
	@echo "Documentation cleaned (manual files preserved)"
```

---

## Summary & Verdict

### Overall Assessment: 🟢 **Good with Caveats**

The implementation is **solid and functional** but needs **robustness improvements** to handle:
- API version evolution
- Filename changes
- Multi-version CRDs

### Must-Do Now (45 min):
1. ✅ Extract metadata from CRD content (not filename)
2. ✅ Loop over served versions
3. ✅ Sort inputs
4. ✅ Fix docs-clean

### Should-Do Soon (30 min):
5. ⚠️ Align version numbers
6. ⚠️ Generate README index
7. ⚠️ Verify .gitignore

### Can-Defer (hours):
8. 📅 Condition validation
9. 📅 Condition auto-generation
10. 📅 Custom JSON format
11. 📅 Example validation

### After Fixes: 🟢 **Production-Ready**

With the priority 1 fixes applied, this system will be:
- ✅ Robust to version changes
- ✅ Deterministic and CI-friendly
- ✅ Safe for manual and generated docs
- ✅ Ready for v1beta1 migration
- ✅ Scalable to 20+ CRDs

**Total Investment**: ~1 hour to go from "working" to "production-ready"
