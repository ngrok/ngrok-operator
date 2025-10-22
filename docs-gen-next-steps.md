# CRD Documentation: Current State & Next Steps

## Current State ✅

### What Works
- ✅ **Tools installed**: `crdoc` and `yq` are installed and working
- ✅ **Make targets created**: `docs-generate`, `docs-generate-json`, `docs-verify`, `docs-clean`
- ✅ **Markdown generated**: All 7 CRDs have markdown files in `docs/crds/`
- ✅ **JSON generated**: All 7 CRDs have JSON files in `docs/generated/crds/`
- ✅ **Directory structure**: Organized by group/version

### What You're Seeing

#### 1. Markdown Files with HTML Tables ✅ This is Normal

The markdown files at `docs/crds/*/v1alpha1/*.md` contain HTML `<table>` elements. **This is intentional and correct**:

```markdown
## CloudEndpoint

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>The unique URL for this cloud endpoint...</td>
        <td>true</td>
    </tr></tbody>
</table>
```

**Why HTML?**
- GitHub markdown renders these tables beautifully
- More flexible than pure markdown tables for complex nested fields
- crdoc's default template uses this format
- **View them in GitHub or a markdown preview** - they look great rendered

**Do we need to change this?** Only if:
- You want plain markdown tables (requires custom crdoc template)
- You want a different format altogether

#### 2. JSON Files ✅ These Exist and Are Full CRDs

The JSON files in `docs/generated/crds/*.json` are **complete CRD definitions**:

```json
{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": { ... },
  "spec": {
    "group": "ngrok.k8s.ngrok.com",
    "names": {
      "kind": "CloudEndpoint",
      "plural": "cloudendpoints",
      "shortNames": ["clep"]
    },
    "versions": [{
      "name": "v1alpha1",
      "schema": {
        "openAPIV3Schema": { ... }  // Full OpenAPI schema here
      }
    }]
  }
}
```

**Is this what the docs site needs?** 
- ✅ **If yes**: You're done! These are ready to publish.
- ❓ **If no**: We need to know what structure the docs site wants.

The full CRD contains:
- Field types and descriptions
- Validation rules (required, enum, pattern, etc.)
- Default values
- Nested object definitions
- All the OpenAPI schema information

#### 3. Empty Examples Folders ⚠️ Need to Be Populated

The directories `docs/crds/*/v1alpha1/examples/` exist but are empty.

**This was intentional** - we deferred creating examples. But you're right, we should populate them.

---

## What's Missing

### 1. Example YAML Files ⚠️ High Priority

Each CRD should have at least one example showing how to use it.

**Options:**

#### Option A: Copy from config/samples (if they exist)

```bash
# Check if samples exist
ls config/samples/
```

If you have samples like `config/samples/bindings_v1alpha1_boundendpoint.yaml`, we can copy them:

```bash
# Add to Makefile
docs-generate-examples:
	@for sample in config/samples/*_v1alpha1_*.yaml; do \
		# Parse filename to extract group and kind
		# Copy to appropriate docs/crds/.../examples/ directory
	done
```

#### Option B: Create minimal examples manually

For each CRD, create a basic example:

**Example**: `docs/crds/ngrok/v1alpha1/examples/cloudendpoint-basic.yaml`
```yaml
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: CloudEndpoint
metadata:
  name: example-cloud-endpoint
  namespace: default
spec:
  url: https://example.ngrok.app
  bindings:
    - my-service
```

**Recommended approach**: Start with Option B (manual), add 1-2 examples per CRD, then automate later if needed.

---

### 2. Condition Documentation ⚠️ Medium Priority

Each CRD with conditions should have a `*-conditions.md` file.

**CRDs that need condition docs:**
- ✅ `boundendpoint-conditions.md` (BoundEndpoint has Ready, ServicesCreated, ConnectivityVerified)
- ✅ `cloudendpoint-conditions.md` (CloudEndpoint has Ready)
- ✅ `agentendpoint-conditions.md` (AgentEndpoint has Ready)
- ✅ `domain-conditions.md` (Domain has Ready)
- ❓ `ippolicy-conditions.md` (check if IPPolicy has conditions)

**Template for condition docs:**

Create `docs/crds/ngrok/v1alpha1/cloudendpoint-conditions.md`:

```markdown
# CloudEndpoint Conditions

This document describes the status conditions for CloudEndpoint resources.

## Ready

**Type**: `Ready`

**Description**: Indicates whether the CloudEndpoint is active and ready to accept traffic.

**Possible Values**:
- `True`: CloudEndpoint is fully operational
- `False`: CloudEndpoint is not ready (see reason for details)
- `Unknown`: Status is being determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| CloudEndpointReady | True | CloudEndpoint is active and ready |
| CloudEndpointError | False | CloudEndpoint creation or update failed |
| CloudEndpointPending | Unknown | CloudEndpoint is being created or updated |

## Example Status

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: CloudEndpointReady
    message: "CloudEndpoint is active and ready"
    lastTransitionTime: "2024-01-15T10:30:00Z"
```
```

**How to create these:**

1. Look at `api/ngrok/v1alpha1/cloudendpoint_conditions.go`
2. Extract condition types and reasons
3. Copy their godoc comments into the markdown

---

### 3. Docs Site Integration ❓ Needs Clarification

**Current JSON format**: Full CRD definitions (242 lines for CloudEndpoint)

**Questions for docs site team:**

1. **Is the full CRD JSON what you need?** 
   - If yes: ✅ We're done
   - If no: What structure do you need?

2. **Example of desired JSON format?**
   
   Do you want something simpler like:
   
   ```json
   {
     "kind": "CloudEndpoint",
     "group": "ngrok.k8s.ngrok.com",
     "version": "v1alpha1",
     "description": "CloudEndpoint manages ngrok cloud endpoints",
     "fields": [
       {
         "name": "spec.url",
         "type": "string",
         "required": true,
         "description": "The unique URL for this cloud endpoint"
       },
       {
         "name": "spec.bindings",
         "type": "array",
         "required": false,
         "description": "List of service bindings"
       }
     ]
   }
   ```

3. **Do you need conditions in the JSON?**
   - Conditions aren't in the OpenAPI schema
   - We'd need to extract them separately

4. **Do you need examples embedded in JSON?**
   - Or do you prefer separate example files?

**Action**: Share `docs/generated/crds/ngrok.k8s.ngrok.com_cloudendpoints.json` with docs site team and get feedback.

---

### 4. Makefile Robustness Issues ⚠️ Should Fix

The Oracle critique identified several brittleness issues:

#### Issue 1: Hard-coded v1alpha1

Current code:
```makefile
version=v1alpha1; \  # Will break when v1beta1 is added
```

**Fix**: Extract versions from CRD:
```makefile
for ver in $$($(YQ) eval '.spec.versions[] | select(.served==true) | .name' "$$crd"); do
```

#### Issue 2: Parsing filenames instead of reading CRD content

Current code:
```makefile
group=$$(basename $$crd | cut -d. -f1);  # Fragile
```

**Fix**: Read from CRD:
```makefile
group=$$($(YQ) eval '.spec.group' "$$crd");
kind=$$($(YQ) eval '.spec.names.kind' "$$crd");
```

#### Issue 3: Non-deterministic ordering

**Fix**: Sort inputs:
```makefile
for crd in $$(ls -1 $(HELM_TEMPLATES_DIR)/crds/*.yaml | sort); do
```

#### Issue 4: docs-clean doesn't work correctly

Current code deletes wrong paths.

**Fix**: 
```makefile
docs-clean:
	@rm -rf docs/generated/
	@find docs/crds -type d -name 'v1*' -exec rm -rf {} +
```

---

## Action Plan

### Phase 1: Make it Viable (2-3 hours)

#### Step 1: Create Basic Examples (1 hour)

For each CRD, create one minimal example:

```bash
# BoundEndpoint
cat > docs/crds/bindings/v1alpha1/examples/basic.yaml << 'EOF'
apiVersion: bindings.k8s.ngrok.com/v1alpha1
kind: BoundEndpoint
metadata:
  name: example-bound-endpoint
  namespace: default
spec:
  endpointURI: http://my-service.default:8080
  port: 8080
  protocol: http
  target: my-service.default.svc.cluster.local:8080
EOF

# CloudEndpoint
cat > docs/crds/ngrok/v1alpha1/examples/basic.yaml << 'EOF'
apiVersion: ngrok.k8s.ngrok.com/v1alpha1
kind: CloudEndpoint
metadata:
  name: example-cloud-endpoint
spec:
  url: https://example.ngrok.app
  bindings:
    - my-service
EOF

# ... and so on for each CRD
```

#### Step 2: Create Condition Documentation (1 hour)

For each CRD with conditions:
1. Open `api/*/v1alpha1/*_conditions.go`
2. Create corresponding `*-conditions.md` file
3. Document each condition type, reasons, and example status

See template above.

#### Step 3: Coordinate with Docs Site (30 min)

1. Send sample JSON file to docs site team
2. Ask: "Is this format what you need, or do you need something different?"
3. Wait for feedback

#### Step 4: Fix Makefile (30 min)

Apply the 4 fixes from the critique:
1. Loop over versions from CRD
2. Read group/kind from CRD content
3. Sort inputs
4. Fix docs-clean

---

### Phase 2: Polish & Automation (2-3 hours)

#### Step 5: Add README Index

Generate `docs/crds/README.md` with links to all CRDs.

#### Step 6: Custom JSON Format (if needed)

**Only if docs site requests it.**

Build a small Go tool or yq script to transform CRD → custom JSON format.

#### Step 7: CI Integration

Create `.github/workflows/crd-docs-diff.yaml` to enforce docs stay current.

#### Step 8: Example Validation

Add make target to validate examples:

```makefile
docs-verify-examples:
	@for example in docs/crds/**/examples/*.yaml; do \
		kubectl apply --dry-run=client -f $$example; \
	done
```

---

## Immediate Next Steps (Prioritized)

### Must Do Now (30 min)

1. ✅ **Verify JSON files exist and are complete**
   ```bash
   ls -lh docs/generated/crds/
   head -100 docs/generated/crds/ngrok.k8s.ngrok.com_cloudendpoints.json
   ```

2. ✅ **Check if markdown renders correctly**
   - Open `docs/crds/ngrok/v1alpha1/cloudendpoints.md` in GitHub or VS Code preview
   - Confirm tables look good

3. ❓ **Decide on JSON format**
   - Share sample JSON with docs site team
   - Get explicit confirmation on format

### Should Do This Week (3 hours)

4. 📝 **Create basic examples** (1 hour)
   - One example per CRD
   - Validate they're correct

5. 📝 **Write condition docs** (1 hour)
   - Use template above
   - Extract from `*_conditions.go` files

6. 🔧 **Fix Makefile robustness** (30 min)
   - Apply fixes from critique
   - Test with `make docs-clean && make docs-generate`

7. 📊 **Generate README index** (30 min)
   - List all CRDs with links
   - Add to `docs-generate` target

### Can Defer (future)

8. 🤖 **Automate condition docs** (when pain increases)
9. 🤖 **Automate example copying** (if examples change frequently)
10. 🔄 **CI workflow** (before merging to main)
11. 🎨 **Custom crdoc template** (if HTML tables are a problem)

---

## Decision Tree: What to Do Next?

### Question 1: Is the HTML in markdown files a problem?

**No → Continue with current format**
- Markdown files are meant to be rendered (GitHub, docs sites)
- HTML tables are standard practice in K8s docs
- ✅ Keep as-is

**Yes → Create custom crdoc template**
- Modify `docs/templates/crdoc.tmpl`
- Use pure markdown tables
- Effort: 1-2 hours

### Question 2: Do we need examples right now?

**Yes → Create them manually (Phase 1, Step 1)**
- Essential for users
- Shows real-world usage
- Effort: 1 hour

**No, but later → Document that they're coming**
- Add TODO to each examples/ directory
- Effort: 5 min

### Question 3: Does docs site need different JSON?

**Don't know yet → Ask them (Phase 1, Step 3)**
- Share sample file
- Get requirements
- Effort: 30 min

**Yes, they need custom format → Build transformer**
- Create Go tool or yq script
- Transform CRD → custom JSON
- Effort: 2-4 hours

**No, full CRD is fine → You're done!**
- Just publish to docs site
- Effort: 0 hours

### Question 4: Should we fix Makefile now?

**Yes if:**
- Planning to add v1beta1 soon
- Multiple people contributing
- Want deterministic CI

**Fix now: 30 minutes**

**No if:**
- Just prototyping
- Only you working on it
- Can fix later when it breaks

**Defer to later**

---

## My Recommendation

**Do this now (1 hour total):**

1. **Create one example for CloudEndpoint** (15 min)
   - Proves the concept
   - Shows what format looks like
   - Easy to replicate for other CRDs

2. **Create one condition doc for CloudEndpoint** (15 min)
   - Shows the pattern
   - Can batch the rest later

3. **Share JSON with docs site team** (10 min)
   - Get explicit "yes this works" or "no we need X"
   - Blocks further JSON work

4. **Preview markdown in GitHub** (5 min)
   - Confirm HTML tables render correctly
   - Decide if format is acceptable

5. **Fix the Makefile robustness issues** (15 min)
   - Prevents future breakage
   - Small effort now vs. big pain later

**Result**: 
- You'll know if the approach works
- You'll have concrete examples to show
- You'll have feedback on JSON format
- The system will be robust for v1beta1

**Then decide**:
- Continue with remaining examples/docs (Phase 1)
- Or adjust based on docs site feedback

---

## Questions for You

1. **Are the HTML tables in markdown acceptable?** 
   - View `docs/crds/ngrok/v1alpha1/cloudendpoints.md` in GitHub or preview
   - Do they look okay rendered?

2. **Do you have existing example YAMLs** (like in `config/samples/`)?
   - We could copy those instead of creating from scratch

3. **When do you plan to add v1beta1?**
   - Determines urgency of Makefile fixes

4. **Who on the docs site team should we coordinate with?**
   - Need to share sample JSON and get format requirements

---

## Summary

**Current state**: ✅ System works, generates all docs  
**Your concerns**: Valid - missing examples, unclear on JSON format, HTML looks weird  
**Reality**: HTML is normal, JSON exists but needs validation with docs team, examples need to be created  

**Next action**: Spend 1 hour creating one example + one condition doc + sharing JSON with docs site team. Then you'll know if this approach is viable or needs adjustment.
