# CRD Documentation Generation Plan

## Overview

This document outlines the strategy for generating documentation for our Kubernetes Custom Resource Definitions (CRDs). The approach uses a **single source of truth** to generate two output formats:

1. **Markdown files in the repo** - For developers working with the current branch
2. **Structured JSON/YAML** - For the docs site to parse and present

**Key Principle**: Use `controller-gen` (already in use) to generate CRDs with OpenAPI schemas, then derive both markdown and JSON from those CRD manifests.

## Current State

### Existing CRDs

We maintain 7 CRDs across 3 API groups:

- **bindings.k8s.ngrok.com/v1alpha1**: BoundEndpoint
- **ngrok.k8s.ngrok.com/v1alpha1**: AgentEndpoint, CloudEndpoint, KubernetesOperator, NgrokTrafficPolicy
- **ingress.k8s.ngrok.com/v1alpha1**: Domain, IPPolicy

### Existing Infrastructure

- **controller-gen v0.14.0**: Already generating CRDs at `helm/ngrok-operator/templates/crds/`
- **Makefile pattern**: `tools/make/*.mk` structure for tool management
- **CI verification**: `kubebuilder-diff` job ensures manifests stay current
- **Go version**: 1.24 in CI

### Well-Documented Conditions

Our condition files (`api/*/v1alpha1/*_conditions.go`) have excellent godoc comments:

```go
// BoundEndpointConditionReady indicates whether the BoundEndpoint is fully ready
// and all required Kubernetes services have been created and connectivity has been verified.
// This condition will be True when both ServicesCreated and ConnectivityVerified are True.
BoundEndpointConditionReady BoundEndpointConditionType = "Ready"
```

**Important Note**: These condition constants are **not included in the CRD OpenAPI schema** (which only contains the generic `metav1.Condition` type). Condition documentation must be maintained separately.

## Architecture: Single Source, Dual Output

```
Go API Types (api/*/v1alpha1/*_types.go)
    ↓
controller-gen (make manifests) ← already in use
    ↓
CRD YAML with OpenAPI Schema (helm/.../crds/*.yaml) ← SINGLE SOURCE OF TRUTH
    ↓
    ├─→ crdoc → Markdown (docs/crds/**/*.md) [committed]
    └─→ yq → JSON/YAML (docs/generated/**) [gitignored, for docs site]
```

**Benefits:**
- One source prevents drift between markdown and JSON
- Leverage existing `controller-gen` workflow
- Simple toolchain: two lightweight tools consuming the same input
- CI enforces docs stay current (like `kubebuilder-diff`)

## Recommended Tools

### 1. crdoc (Markdown Generation)

**Tool**: [fybrik/crdoc](https://github.com/fybrik/crdoc)

**Why**: 
- Designed to read CRD YAML and generate markdown
- Simple, focused, maintained
- Works directly with `controller-gen` output
- Templatable for customization

**Installation**: `go install fybrik.io/crdoc@v0.7.1`

**Usage**:
```bash
crdoc --resources helm/ngrok-operator/templates/crds/*.yaml \
      --output docs/crds/README.md
```

### 2. yq (JSON/YAML Conversion)

**Tool**: [mikefarah/yq](https://github.com/mikefarah/yq)

**Why**:
- Lightweight YAML ↔ JSON converter
- Can extract specific fields from CRD schema
- Flexible for whatever format docs site needs

**Installation**: `go install github.com/mikefarah/yq/v4@v4.35.1`

**Usage**:
```bash
# Simple conversion
yq eval -o=json helm/ngrok-operator/templates/crds/boundendpoint.yaml

# Extract specific paths
yq eval '.spec.versions[0].schema.openAPIV3Schema' crd.yaml
```

**Alternative Approach**: If docs site needs a custom JSON structure (not just converted YAML), we can build a small Go tool to extract and transform the OpenAPI schema into the desired format.

### 3. controller-gen (Already in Use)

**Current Usage**: Generates CRDs with OpenAPI schemas

**No Changes Needed**: Continue using as-is

## Directory Structure

```
docs/
├── crds/                                   # Committed to git
│   ├── README.md                          # Generated: overview of all CRDs
│   ├── bindings/
│   │   └── v1alpha1/
│   │       ├── boundendpoint.md           # Generated from CRD
│   │       ├── boundendpoint-conditions.md # Manual
│   │       └── examples/
│   │           └── basic.yaml
│   ├── ngrok/
│   │   └── v1alpha1/
│   │       ├── agentendpoint.md
│   │       ├── agentendpoint-conditions.md
│   │       ├── cloudendpoint.md
│   │       ├── cloudendpoint-conditions.md
│   │       ├── kubernetesoperator.md
│   │       ├── ngroktrafficpolicy.md
│   │       └── examples/
│   └── ingress/
│       └── v1alpha1/
│           ├── domain.md
│           ├── domain-conditions.md
│           ├── ippolicy.md
│           ├── ippolicy-conditions.md
│           └── examples/
│
├── generated/                             # NOT committed (.gitignore)
│   └── crds/
│       ├── bindings.k8s.ngrok.com_boundendpoints.json
│       ├── ngrok.k8s.ngrok.com_agentendpoints.json
│       ├── ngrok.k8s.ngrok.com_cloudendpoints.json
│       ├── ngrok.k8s.ngrok.com_kubernetesoperators.json
│       ├── ngrok.k8s.ngrok.com_ngroktrafficpolicies.json
│       ├── ingress.k8s.ngrok.com_domains.json
│       └── ingress.k8s.ngrok.com_ippolicies.json
│
└── templates/
    └── crdoc.tmpl                         # Custom template for crdoc
```

**Key Decisions:**

- **Version in path** (`v1alpha1/`): Supports future API versions (v1beta1, v1)
- **Conditions separate**: `*-conditions.md` files maintained manually (schema doesn't include them)
- **Examples included**: Sample YAML files per CRD
- **Generated JSON gitignored**: Build artifact for docs site, not committed

## Condition Documentation Strategy

### The Problem

Condition type constants in `*_conditions.go` are **not part of the CRD schema**. The schema only defines:

```yaml
status:
  conditions:
    type: array
    items:
      # metav1.Condition - generic type
```

Tools reading CRD YAML cannot extract our condition documentation.

### The Solution: Manual Documentation

Create per-CRD condition documentation files:

**Example**: `docs/crds/bindings/v1alpha1/boundendpoint-conditions.md`

```markdown
## Conditions

### Ready

**Type**: `Ready`

**Description**: Indicates whether the BoundEndpoint is fully ready and all required Kubernetes services have been created and connectivity has been verified.

**Possible Values**:
- `True`: All services created and connectivity verified
- `False`: One or more prerequisites not met
- `Unknown`: Status being determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| BoundEndpointReady | True | Fully operational |
| ServicesNotCreated | False | Required services not created |
| ConnectivityNotVerified | False | Connectivity not verified |

[Continue for other conditions...]
```

**Maintenance**: 
- Update when adding/modifying conditions in Go code
- Link from generated markdown: "See [Conditions](./boundendpoint-conditions.md)"
- CI can validate these files exist (simple check)

**Future Enhancement**: Could build a small Go tool to parse structured comments like:
```go
// +doc:condition Ready: True when all services created and connectivity verified
```

But for now, manual is simpler and clearer.

## Example Documentation

### The Challenge

Including full YAML examples in godoc comments would bloat Go source files. 

### Recommended Approach

**Option 1: Separate Example Files** (Recommended)

```
docs/crds/bindings/v1alpha1/examples/
├── basic.yaml                 # Minimal working example
├── with-tls.yaml             # TLS configuration example
└── advanced.yaml             # Complex scenario
```

Link from generated markdown or append examples section.

**Option 2: Leverage config/samples/**

If `config/samples/` exists, reference or copy those:

```bash
# In generation script
cp config/samples/bindings_v1alpha1_boundendpoint.yaml \
   docs/crds/bindings/v1alpha1/examples/basic.yaml
```

**Option 3: Inline in Template**

For very simple examples, embed in crdoc template:

```markdown
## Example

\```yaml
apiVersion: bindings.k8s.ngrok.com/v1alpha1
kind: BoundEndpoint
metadata:
  name: example
spec:
  target: myservice.default:8080
\```
```

**Recommendation**: Use Option 1 (separate files) for maintainability and flexibility.

## Makefile Integration

Following the existing `tools/make/*.mk` pattern:

### tools/make/_common.mk

```makefile
# Add after HELM line (~line 60)
CRDOC ?= $(LOCALBIN)/crdoc-$(CRDOC_VERSION)
YQ ?= $(LOCALBIN)/yq-$(YQ_VERSION)

# Add after HELM_VERSION line (~line 68)
CRDOC_VERSION ?= v0.7.1
YQ_VERSION ?= v4.35.1
```

### tools/make/tools.mk

```makefile
.PHONY: crdoc
crdoc: $(CRDOC) ## Download crdoc locally if necessary.
$(CRDOC): $(LOCALBIN)
	$(call go-install-tool,$(CRDOC),fybrik.io/crdoc,$(CRDOC_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

# Update bootstrap-tools target
.PHONY: bootstrap-tools
bootstrap-tools: controller-gen envtest golangci-lint kind helm helm-unittest-plugin crdoc yq
```

### tools/make/generate.mk

```makefile
##@ Documentation

.PHONY: docs-generate
docs-generate: manifests crdoc ## Generate CRD markdown documentation
	@echo "Generating CRD markdown documentation..."
	@mkdir -p docs/crds
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
		group=$$(basename $$crd | cut -d. -f1); \
		kind=$$(basename $$crd .yaml | cut -d_ -f2); \
		version=v1alpha1; \
		mkdir -p docs/crds/$$group/$$version; \
		$(CRDOC) --resources $$crd --output docs/crds/$$group/$$version/$$kind.md; \
	done
	@echo "Markdown documentation generated in docs/crds/"

.PHONY: docs-generate-json
docs-generate-json: manifests yq ## Generate JSON documentation for docs site
	@echo "Generating JSON documentation for docs site..."
	@mkdir -p docs/generated/crds
	@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do \
		filename=$$(basename $$crd .yaml); \
		$(YQ) eval -o=json $$crd > docs/generated/crds/$$filename.json; \
	done
	@echo "JSON documentation generated in docs/generated/crds/"

.PHONY: docs-verify
docs-verify: docs-generate ## Verify CRD markdown is up to date
	@git diff --exit-code docs/crds/ || \
		(echo "ERROR: CRD documentation is out of date. Run 'make docs-generate' to update." && exit 1)

.PHONY: docs-clean
docs-clean: ## Clean generated documentation
	@rm -rf docs/crds/*.md docs/generated/
	@echo "Documentation cleaned!"
```

## Usage Workflows

### For Developers (Daily Work)

When modifying API types:

```bash
# 1. Edit api/*/v1alpha1/*_types.go
vim api/ngrok/v1alpha1/cloudendpoint_types.go

# 2. Regenerate CRDs and docs
make manifests        # Generate CRDs (existing target)
make docs-generate    # Generate markdown

# 3. If you added/changed conditions
vim docs/crds/ngrok/v1alpha1/cloudendpoint-conditions.md

# 4. Commit everything
git add api/ helm/ngrok-operator/templates/crds/ docs/crds/
git commit -m "Add new field to CloudEndpoint"
```

### For CI (Automated Verification)

On PRs that touch `api/**`:

```bash
make manifests
make docs-generate
make docs-verify  # Fails if docs are out of sync
```

This mirrors the existing `kubebuilder-diff` pattern.

### For Docs Site (JSON Generation)

When docs site needs updated JSON:

```bash
# Generate structured data
make docs-generate-json

# Output in docs/generated/crds/*.json
# Push to docs repo or publish to docs site
```

**Note**: The JSON format can be customized. If docs site needs a different structure than raw CRD YAML → JSON, we can:
- Extract just the OpenAPI schema
- Flatten field definitions
- Create a custom converter tool

Once we know the exact format the docs site needs, we can adjust the `docs-generate-json` target.

## CI Integration Notes

While this plan focuses on generation commands (not CI automation), here's how these commands would be used:

### On Pull Requests (to main)

A CI job similar to `kubebuilder-diff` would:

```yaml
- run: make manifests
- run: make docs-generate
- run: make docs-verify  # Fails if uncommitted changes
```

This ensures developers can't merge API changes without updating documentation.

### On Merges to Main

Documentation in `docs/crds/` stays current with the main branch automatically (via the PR checks above).

### On Releases (git tags)

Generate JSON for docs site:

```yaml
- run: make docs-generate-json
- run: # Push docs/generated/crds/*.json to docs repo
```

The docs site team can then parse and present the structured JSON data.

## Migration from Current State

### Step 1: Install Tools

```bash
make crdoc
make yq
./bin/crdoc-v0.7.1 --version
./bin/yq-v4.35.1 --version
```

### Step 2: Generate Initial Documentation

```bash
make manifests
make docs-generate
```

Review the generated markdown in `docs/crds/`.

### Step 3: Create Condition Documentation

For each CRD with conditions, create `*-conditions.md`:

- `docs/crds/bindings/v1alpha1/boundendpoint-conditions.md`
- `docs/crds/ngrok/v1alpha1/agentendpoint-conditions.md`
- `docs/crds/ngrok/v1alpha1/cloudendpoint-conditions.md`
- `docs/crds/ingress/v1alpha1/domain-conditions.md`
- `docs/crds/ingress/v1alpha1/ippolicy-conditions.md`

### Step 4: Add Examples (Optional)

Create example YAML files in `docs/crds/*/v1alpha1/examples/`.

### Step 5: Commit Documentation

```bash
echo "docs/generated/" >> .gitignore
git add docs/crds/ .gitignore
git commit -m "Add generated CRD documentation"
```

### Step 6: Test JSON Generation

```bash
make docs-generate-json
ls docs/generated/crds/
```

Share sample JSON with docs site team to confirm format.

### Step 7: Add CI Verification

Create a CI job that runs `make docs-verify` on PRs touching `api/**`.

## Open Questions for Docs Site Team

Before finalizing JSON generation:

1. **Format**: Is raw CRD YAML → JSON sufficient, or do you need a custom structure?
2. **Fields**: Do you need the full CRD or just the OpenAPI schema?
3. **Flattening**: Should nested fields be flattened for easier parsing?
4. **Versioning**: How should multiple API versions (v1alpha1, v1beta1) be handled?
5. **Examples**: Should examples be embedded in JSON or separate files?

**Current Implementation**: Simple YAML → JSON conversion with `yq`. Can be customized once requirements are clear.

## Summary

This plan provides a **lean, focused approach** to CRD documentation generation:

**Single Source of Truth**: `controller-gen` CRDs (already in use)

**Two Outputs**:
1. **Markdown** (`crdoc`): Committed to repo, always current
2. **JSON** (`yq`): Build artifact for docs site

**Manual Elements**:
- Condition documentation (not in schema)
- Examples (separate from Go code)

**Integrated with Existing Patterns**:
- `tools/make/*.mk` structure
- Versioned tool binaries
- CI verification (like `kubebuilder-diff`)

**Next Steps**:
1. Install tools (`make crdoc yq`)
2. Generate initial docs (`make docs-generate`)
3. Create condition documentation
4. Confirm JSON format with docs site team
5. Add CI verification

The approach is **simple, maintainable, and prevents drift** between markdown and structured data by generating both from the same CRD manifests.
