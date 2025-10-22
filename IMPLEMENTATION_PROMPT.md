# CRD Documentation - Phase 2 Implementation Prompt

Use this prompt to complete the next phase of CRD documentation generation.

---

## Task: Complete CRD Documentation System

You are completing the CRD documentation generation system for ngrok-operator. The basic generation is working, but examples and condition docs are missing. See `docs-gen-next-steps.md` for full context.

### Your Goals

1. **Create example YAML files** for all CRDs
2. **Create condition documentation** for CRDs with conditions
3. **Fix Makefile robustness issues** identified in the critique
4. **Generate README index** for easy navigation

---

## Part 1: Create Example Files

For each CRD, create a basic example YAML file at `docs/crds/{group}/v1alpha1/examples/basic.yaml`.

**CRDs that need examples:**

- [x] `bindings/v1alpha1/boundendpoint` - Already done
- [x] `ngrok/v1alpha1/cloudendpoint` - Already done
- [ ] `ngrok/v1alpha1/agentendpoint`
- [ ] `ngrok/v1alpha1/kubernetesoperator`
- [ ] `ngrok/v1alpha1/ngroktrafficpolicy`
- [ ] `ingress/v1alpha1/domain`
- [ ] `ingress/v1alpha1/ippolicy`

**How to create examples:**

1. Read the CRD file from `helm/ngrok-operator/templates/crds/{group}.k8s.ngrok.com_{kinds}.yaml`
2. Look at the spec fields and their descriptions in the OpenAPI schema
3. Create a minimal working example showing the most common use case
4. Include helpful comments explaining key fields
5. Use realistic values (not just "string" or "example")

**Example template:**

```yaml
apiVersion: {group}.k8s.ngrok.com/v1alpha1
kind: {Kind}
metadata:
  name: example-{kind}
  namespace: ngrok-operator
spec:
  # Add 2-3 most important fields with comments
  # Use realistic values
```

**Verification:**
```bash
# Test that examples are valid YAML
kubectl apply --dry-run=client -f docs/crds/**/examples/*.yaml
```

---

## Part 2: Create Condition Documentation

For CRDs with conditions, create `{kind}-conditions.md` files.

**Find which CRDs have conditions:**

```bash
# List all condition files
ls -1 api/*/v1alpha1/*_conditions.go
```

For each conditions file found, create corresponding documentation.

**Files to create:**

- [ ] `docs/crds/bindings/v1alpha1/boundendpoint-conditions.md`
- [ ] `docs/crds/ngrok/v1alpha1/agentendpoint-conditions.md`
- [ ] `docs/crds/ngrok/v1alpha1/cloudendpoint-conditions.md`
- [ ] `docs/crds/ingress/v1alpha1/domain-conditions.md`
- [ ] Check: Does IPPolicy have conditions? If yes, add `ippolicy-conditions.md`

**How to create condition docs:**

1. Read `api/{group}/v1alpha1/{kind}_conditions.go`
2. Extract condition types (constants like `CloudEndpointConditionReady`)
3. Extract reasons for each condition
4. Copy the godoc comments as descriptions
5. Use this template:

```markdown
# {Kind} Conditions

This document describes the status conditions for {Kind} resources.

## {ConditionName}

**Type**: `{ConditionName}`

**Description**: {Description from godoc}

**Possible Values**:
- `True`: {When true}
- `False`: {When false}
- `Unknown`: {When unknown}

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| {ReasonName} | {True/False} | {Description from godoc} |

## Example Status

\```yaml
status:
  conditions:
  - type: {ConditionName}
    status: "True"
    reason: {ReasonName}
    message: "{Descriptive message}"
    lastTransitionTime: "2024-01-15T10:30:00Z"
\```
```

**Example:**

For `api/ngrok/v1alpha1/cloudendpoint_conditions.go` with this code:

```go
// CloudEndpointConditionReady indicates whether the CloudEndpoint is fully ready
const CloudEndpointConditionReady = "Ready"

// CloudEndpointReasonReady is used when CloudEndpoint is active
const CloudEndpointReasonReady = "CloudEndpointReady"
```

Create `docs/crds/ngrok/v1alpha1/cloudendpoint-conditions.md`:

```markdown
# CloudEndpoint Conditions

## Ready

**Type**: `Ready`

**Description**: Indicates whether the CloudEndpoint is fully ready and active.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| CloudEndpointReady | True | CloudEndpoint is active and ready |
| CloudEndpointError | False | CloudEndpoint encountered an error |
```

---

## Part 3: Fix Makefile Robustness Issues

Apply these fixes to `tools/make/generate.mk`:

### Fix 1: Extract metadata from CRD (not filename)

**Current (brittle):**
```makefile
group=$$(basename $$crd | cut -d. -f1);
kind=$$(basename $$crd .yaml | cut -d_ -f2);
```

**Fixed (robust):**
```makefile
group=$$($(YQ) eval '.spec.group' "$$crd");
shortgroup=$${group%%.*};  # Extract first part (bindings, ngrok, ingress)
kind=$$($(YQ) eval '.spec.names.kind' "$$crd");
kindlower=$$(echo $$kind | tr '[:upper:]' '[:lower:]');
```

### Fix 2: Loop over served versions (support multi-version CRDs)

**Current (hard-coded v1alpha1):**
```makefile
version=v1alpha1;
mkdir -p docs/crds/$$group/$$version;
```

**Fixed (reads from CRD):**
```makefile
for ver in $$($(YQ) eval '.spec.versions[] | select(.served==true) | .name' "$$crd"); do
  mkdir -p docs/crds/$$shortgroup/$$ver;
  $(CRDOC) --resources "$$crd" --output docs/crds/$$shortgroup/$$ver/$$kindlower.md;
done
```

### Fix 3: Sort inputs for deterministic ordering

**Current:**
```makefile
@for crd in $(HELM_TEMPLATES_DIR)/crds/*.yaml; do
```

**Fixed:**
```makefile
@for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do
```

### Fix 4: Fix docs-clean to preserve manual files

**Current (deletes wrong things):**
```makefile
docs-clean:
	@rm -rf docs/crds/*.md docs/generated/
```

**Fixed:**
```makefile
docs-clean: ## Clean generated documentation
	@echo "Cleaning generated documentation..."
	@rm -rf docs/generated/
	@find docs/crds -type d -name 'v1*' -exec rm -rf {} +
	@rm -f docs/crds/README.md
	@echo "Documentation cleaned (manual *-conditions.md files preserved)"
```

---

## Part 4: Generate README Index

Add to the `docs-generate` target to create `docs/crds/README.md`:

```makefile
.PHONY: docs-generate
docs-generate: manifests crdoc yq ## Generate CRD markdown documentation
	@echo "Generating CRD markdown documentation..."
	@mkdir -p docs/crds
	
	# Create README header
	@echo "# CRD Reference Documentation" > docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "Auto-generated API reference for ngrok-operator Custom Resource Definitions." >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "## Available CRDs" >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	
	# Generate docs and add to README
	@for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do \
		group=$$($(YQ) eval '.spec.group' "$$crd"); \
		shortgroup=$${group%%.*}; \
		kind=$$($(YQ) eval '.spec.names.kind' "$$crd"); \
		kindlower=$$(echo $$kind | tr '[:upper:]' '[:lower:]'); \
		for ver in $$($(YQ) eval '.spec.versions[] | select(.served==true) | .name' "$$crd"); do \
			dir=docs/crds/$$shortgroup/$$ver; \
			mkdir -p $$dir/examples; \
			echo "  Generating $$shortgroup/$$ver/$$kindlower.md..."; \
			$(CRDOC) --resources "$$crd" --output $$dir/$$kindlower.md; \
			echo "- [$$kind]($$shortgroup/$$ver/$$kindlower.md) - \`$$group/$$ver\`" >> docs/crds/README.md; \
		done; \
	done
	@echo "" >> docs/crds/README.md
	@echo "## Documentation Types" >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "- **API Reference**: Field descriptions, types, and validation rules" >> docs/crds/README.md
	@echo "- **Conditions**: Status condition types and reasons (in \`*-conditions.md\` files)" >> docs/crds/README.md
	@echo "- **Examples**: Sample YAML manifests (in \`examples/\` directories)" >> docs/crds/README.md
	@echo "" >> docs/crds/README.md
	@echo "Markdown documentation generated in docs/crds/"
```

---

## Verification Steps

After completing all parts:

```bash
# 1. Test tool availability
make crdoc yq

# 2. Clean and regenerate everything
make docs-clean
make manifests
make docs-generate
make docs-generate-json

# 3. Verify markdown files
ls -R docs/crds/

# 4. Verify examples exist
find docs/crds -name "*.yaml"

# 5. Verify condition docs exist
find docs/crds -name "*-conditions.md"

# 6. Verify README was created
cat docs/crds/README.md

# 7. Test that docs are current
make docs-verify

# 8. Validate examples (optional, may fail if examples need cluster-specific config)
kubectl apply --dry-run=client -f docs/crds/**/examples/*.yaml
```

---

## What to Return

Report:

1. **Examples created**: List all example files with path
2. **Condition docs created**: List all condition doc files with path
3. **Makefile changes**: Confirm all 4 fixes were applied
4. **README generated**: Show first 20 lines of `docs/crds/README.md`
5. **Verification results**: Output of running verification steps
6. **Any issues encountered**: Problems and how you resolved them

---

## Important Notes

- **Use realistic values in examples** - not just "string" or "example"
- **Include comments in examples** - explain what fields do
- **Copy exact godoc descriptions** - don't paraphrase condition docs
- **Test the Makefile changes** - run `make docs-clean && make docs-generate` multiple times
- **Make examples kubectl-applicable** - they should be valid Kubernetes manifests
- **Check for existing condition files** - some CRDs might not have conditions

---

## If You Get Stuck

**Can't find what fields to use in examples?**
- Read the CRD YAML in `helm/ngrok-operator/templates/crds/`
- Look at the `openAPIV3Schema` section
- Check for required fields first

**Condition constants are confusing?**
- Look for constants with types ending in `ConditionType` and `Reason`
- Each condition type will have multiple reason constants
- Godoc comments explain when each is used

**Makefile syntax errors?**
- Test each change incrementally
- Run `make docs-generate` after each fix
- Use `make -n docs-generate` to dry-run without executing

**yq syntax questions?**
- Test yq commands directly: `bin/yq-v4.35.1 eval '.spec.group' helm/ngrok-operator/templates/crds/ngrok.k8s.ngrok.com_cloudendpoints.yaml`
- yq uses jq-like syntax for JSON path queries
