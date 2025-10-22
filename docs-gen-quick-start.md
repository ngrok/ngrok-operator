# CRD Documentation Generation - Quick Start

## TL;DR

**Problem with current plan**: Uses two separate doc generators (gen-crd-api-reference-docs + controller-gen), creating drift risk and unnecessary complexity.

**Solution**: Use controller-gen (already in use) as single source → generate both markdown (crdoc) and JSON (yq) from CRD YAML.

---

## One-Page Implementation Guide

### 1. Install Tools (5 min)

Add to `tools/make/_common.mk`:
```makefile
CRDOC ?= $(LOCALBIN)/crdoc-$(CRDOC_VERSION)
YQ ?= $(LOCALBIN)/yq-$(YQ_VERSION)
CRDOC_VERSION ?= v0.7.1
YQ_VERSION ?= v4.35.1
```

Add to `tools/make/tools.mk`:
```makefile
.PHONY: crdoc
crdoc: $(CRDOC)
$(CRDOC): $(LOCALBIN)
	$(call go-install-tool,$(CRDOC),fybrik.io/crdoc,$(CRDOC_VERSION))

.PHONY: yq
yq: $(YQ)
$(YQ): $(LOCALBIN)
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))
```

Test: `make crdoc && make yq`

---

### 2. Add Makefile Targets (10 min)

Add to `tools/make/generate.mk`:
```makefile
##@ Documentation

.PHONY: docs-generate
docs-generate: manifests crdoc ## Generate CRD markdown docs
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
		group=$$(basename $$crd | cut -d. -f1); \
		kind=$$(basename $$crd .yaml | cut -d_ -f2); \
		mkdir -p docs/crds/$$group/v1alpha1; \
		$(CRDOC) --resources $$crd --output docs/crds/$$group/v1alpha1/$$kind.md; \
	done

.PHONY: docs-verify
docs-verify: docs-generate ## Verify docs are current
	@git diff --exit-code docs/crds/ || (echo "Run 'make docs-generate'" && exit 1)

.PHONY: docs-generate-release
docs-generate-release: manifests yq ## Generate JSON for releases
	@mkdir -p docs/generated/crds/$(VERSION)
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
		group=$$(basename $$crd | cut -d. -f1); \
		kind=$$(basename $$crd .yaml | cut -d_ -f2); \
		mkdir -p docs/generated/crds/$(VERSION)/$$group/v1alpha1; \
		$(YQ) eval -o=json $$crd > docs/generated/crds/$(VERSION)/$$group/v1alpha1/$$kind.json; \
	done
```

Test: `make docs-generate && ls docs/crds/`

---

### 3. Create Directory Structure (5 min)

```bash
mkdir -p docs/crds/{bindings,ngrok,ingress}/v1alpha1
mkdir -p docs/templates
echo "docs/generated/" >> .gitignore
```

---

### 4. Add CI Workflow (10 min)

Create `.github/workflows/crd-docs-diff.yaml`:
```yaml
name: CRD Documentation Verification
on:
  pull_request:
    paths: ['api/**', 'docs/crds/**']
env:
  GO_VERSION: '1.24'
jobs:
  crd-docs-diff:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: ${{ env.GO_VERSION }}
      - run: make crdoc
      - run: make manifests
      - run: make docs-generate
      - run: make docs-verify
```

---

### 5. Create Condition Docs (Manual, 15 min per CRD)

Example `docs/crds/bindings/v1alpha1/boundendpoint-conditions.md`:
```markdown
## Conditions

### Ready
**Type**: `Ready`  
**Description**: BoundEndpoint is fully ready with services created and connectivity verified

**Values**:
- `True`: All prerequisites met
- `False`: One or more issues
- `Unknown`: Status being determined

**Reasons**:
| Reason | Status | Description |
|--------|--------|-------------|
| BoundEndpointReady | True | Fully operational |
| ServicesNotCreated | False | Services missing |
| ConnectivityNotVerified | False | Connectivity not checked |

[Repeat for ServicesCreated, ConnectivityVerified conditions...]
```

Repeat for all CRDs with conditions.

---

### 6. Initial Generation & Commit (5 min)

```bash
make manifests
make docs-generate
git add docs/crds/
git commit -m "Add auto-generated CRD documentation"
```

---

## Architecture

```
Go API Types (api/**/*_types.go)
    ↓
controller-gen (make manifests)
    ↓
CRD YAML (helm/.../crds/*.yaml) ← Single Source of Truth
    ↓
    ├─→ crdoc → Markdown (docs/crds/**/*.md) [committed]
    └─→ yq → JSON (docs/generated/...) [.gitignored, release only]
```

---

## Daily Workflow

### Developer Making API Changes

1. Edit `api/*/v1alpha1/*_types.go`
2. Run `make manifests` (generates CRDs)
3. Run `make docs-generate` (generates docs)
4. Commit both CRDs and docs
5. CI verifies docs are current

### Release Manager

1. Tag release
2. CI runs `make docs-generate-release`
3. Publishes `docs/generated/crds/$VERSION/` to docs site

---

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| **Use crdoc, not gen-crd-api-reference-docs** | Simpler, works with CRD YAML, avoids dual parsers |
| **Manual condition docs** | Schema doesn't contain condition constants |
| **Commit markdown, not JSON** | Dev docs should be in repo; release JSON is artifact |
| **Version in paths** | Supports v1alpha1 → v1beta1 transitions |
| **CI enforcement** | Prevents docs drift (like kubebuilder-diff) |

---

## Critical Questions to Answer

Before implementing Step 6 (release JSON), answer:

**Q: What JSON format does the docs site need?**
- [ ] Raw CRD YAML converted to JSON (simple yq conversion)?
- [ ] Flattened field list with specific schema?
- [ ] Custom format?

**Q: Where does JSON get published?**
- [ ] Separate docs repo?
- [ ] GitHub release artifacts?
- [ ] Cloud storage (S3/GCS)?
- [ ] Docs site API?

**Q: Who integrates the JSON?**
- [ ] Docs site team parses it?
- [ ] We need converter script?

---

## Files Changed

```
✏️  Modified:
  tools/make/_common.mk
  tools/make/tools.mk
  tools/make/generate.mk
  .gitignore

➕ Created:
  .github/workflows/crd-docs-diff.yaml
  docs/crds/README.md
  docs/crds/{group}/v1alpha1/{kind}.md (auto-generated)
  docs/crds/{group}/v1alpha1/{kind}-conditions.md (manual)
  docs/templates/crdoc-template.md.tmpl (optional)
  scripts/validate-conditions-docs.sh (optional)
  scripts/publish-docs-to-site.sh (release workflow)

🚫 .gitignored:
  docs/generated/
```

---

## Rollout Plan

### Week 1: Foundation
- [ ] Install tools
- [ ] Add Makefile targets  
- [ ] Generate initial docs
- [ ] Add CI workflow

### Week 2: Polish
- [ ] Write condition docs for all CRDs
- [ ] Create templates (if needed)
- [ ] Test with actual API change PR
- [ ] Train team

### Week 3: Release Integration
- [ ] Coordinate with docs site team on JSON format
- [ ] Implement release workflow
- [ ] Test dry-run release
- [ ] Document maintenance procedures

---

## Success Metrics

- ✅ `make docs-generate` runs in < 10s
- ✅ CI catches out-of-date docs
- ✅ All CRDs have complete documentation
- ✅ Zero manual intervention needed
- ✅ Docs site successfully consumes JSON

---

## Comparison to Original Plan

| Aspect | Original Plan | Recommended Approach |
|--------|---------------|---------------------|
| **Markdown tool** | gen-crd-api-reference-docs | crdoc |
| **JSON tool** | controller-gen + custom extraction | yq or custom converter |
| **Source of truth** | Two separate (Go source × 2) | One (controller-gen CRDs) |
| **Condition docs** | "Auto-generated" (won't work) | Manual or semi-automated |
| **Makefile** | Custom targets | Follows tools/make/*.mk pattern |
| **CI** | New separate workflows | Integrates with existing patterns |
| **Complexity** | High (dual parsers) | Low (single source) |
| **Drift risk** | High | Low |

---

## Need Help?

See full critique: `docs-gen-implementation-critique.md`

Key sections:
- **Phase 1**: Detailed technical critique
- **Phase 2**: Questions to answer
- **Phase 3**: Step-by-step implementation (8-12 hours)
- **Phase 4**: Success criteria
- **Phase 5**: Risk mitigation
