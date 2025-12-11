# Design: Fix Invalid Kubebuilder Validation Tags

## Problem Statement

The codebase contains invalid validation marker comments using `+kube:validation` prefix instead of the correct `+kubebuilder:validation` prefix. These invalid markers are silently ignored by `controller-gen`, meaning validation constraints are not applied to the generated CRD schemas.

### Root Cause

The `controller-gen` marker parser in [controller-tools](https://github.com/kubernetes-sigs/controller-tools) silently ignores unknown markers. When a marker like `+kube:validation:Enum` is encountered:
1. The comment is recognized as a marker (starts with `// +`)
2. The registry lookup fails because only `+kubebuilder:validation:` is registered
3. The marker is silently skipped with no warning or error

### Affected Files

| File | Line | Invalid Marker |
|------|------|----------------|
| `api/ngrok/v1alpha1/kubernetesoperator_types.go` | 68 | `+kube:validation:Enum=registered;error;pending` |
| `api/ngrok/v1alpha1/kubernetesoperator_types.go` | 73 | `+kube:validation:Optional` |
| `api/bindings/v1alpha1/boundendpoint_types.go` | 117 | `+kube:validation:Optional` |

## Solution

### Part 1: Fix Existing Invalid Tags

Replace all `+kube:validation` prefixes with `+kubebuilder:validation`:

#### kubernetesoperator_types.go (lines 68, 73)

```diff
  // RegistrationStatus is the status of the registration of this Kubernetes Operator with the ngrok API
  // +kubebuilder:validation:Required
- // +kube:validation:Enum=registered;error;pending
+ // +kubebuilder:validation:Enum=registered;error;pending
  // +kubebuilder:default="pending"
  RegistrationStatus string `json:"registrationStatus,omitempty"`

  // RegistrationErrorCode is the returned ngrok error code
- // +kube:validation:Optional
+ // +kubebuilder:validation:Optional
  // +kubebuilder:validation:Pattern=`^ERR_NGROK_\d+$`
  RegistrationErrorCode string `json:"registrationErrorCode,omitempty"`
```

#### boundendpoint_types.go (line 117)

```diff
  // Metadata is a subset of metav1.ObjectMeta that is added to the Service
- // +kube:validation:Optional
+ // +kubebuilder:validation:Optional
  Metadata TargetMetadata `json:"metadata,omitempty"`
```

### Part 2: Prevent Future Issues with kube-api-linter (Recommended)

Use [kube-api-linter](https://github.com/kubernetes-sigs/kube-api-linter) (KAL) - the official Kubernetes SIG linter for API types. It integrates with golangci-lint and provides the `forbiddenmarkers` linter that can detect invalid marker prefixes.

#### Installation

Create `.custom-gcl.yml` to build a custom golangci-lint with kube-api-linter:

```yaml
version: v2.5.0
name: golangci-lint-kube-api-linter
destination: ./bin
plugins:
  - module: 'sigs.k8s.io/kube-api-linter'
    version: 'v0.0.0-20251208100930-d3015c953951'  # Check pkg.go.dev for latest
```

Build the custom binary:

```bash
golangci-lint custom
```

#### Configuration

Update `.golangci.yml` to enable the forbidden markers linter:

```yaml
version: "2"

linters:
  enable:
    - kubeapilinter
    # ... other existing linters

  settings:
    custom:
      kubeapilinter:
        type: module
        description: Kube API Linter for Kubernetes API conventions
        settings:
          linters:
            forbiddenmarkers:
              enabled: true
          lintersConfig:
            forbiddenmarkers:
              markers:
                # Forbid common typos and invalid marker prefixes
                - identifier: "kube:validation:Optional"
                - identifier: "kube:validation:Required"
                - identifier: "kube:validation:Enum"
                - identifier: "kube:validation:Pattern"
                - identifier: "kube:validation:MaxLength"
                - identifier: "kube:validation:MinLength"
                - identifier: "kube:validation:Maximum"
                - identifier: "kube:validation:Minimum"
                - identifier: "kube:validation:MaxItems"
                - identifier: "kube:validation:MinItems"
                # Add any other known typo patterns
```

#### Additional KAL Benefits

Beyond `forbiddenmarkers`, kube-api-linter provides other useful linters for Kubernetes API types:

| Linter | Purpose |
|--------|---------|
| `optionalorrequired` | Ensures all fields are explicitly marked optional or required |
| `duplicatemarkers` | Detects exact duplicate markers |
| `conflictingmarkers` | Detects mutually exclusive markers on the same field |
| `defaultorrequired` | Ensures required fields don't have defaults |
| `statussubresource` | Validates status subresource configuration |
| `maxlength` | Checks for max length constraints on strings/arrays |

#### Makefile Integration

Update `tools/make/lint.mk`:

```makefile
GOLANGCI_LINT_KAL := $(LOCALBIN)/golangci-lint-kube-api-linter

.PHONY: golangci-lint-kal
golangci-lint-kal: ## Build custom golangci-lint with kube-api-linter
	@test -f $(GOLANGCI_LINT_KAL) || golangci-lint custom

.PHONY: lint
lint: golangci-lint golangci-lint-kal ## Run golangci-lint linter
	$(GOLANGCI_LINT) run
	$(GOLANGCI_LINT_KAL) run ./api/...
```

### Alternative: Simple Grep-based Check (Fallback)

If kube-api-linter integration is too complex for initial implementation, a simpler approach:

Add to `tools/make/lint.mk`:

```makefile
.PHONY: lint-markers
lint-markers: ## Check for invalid kubebuilder marker prefixes
	@if grep -rn --include='*.go' '+kube:validation:' api/; then \
		echo "ERROR: Found invalid marker prefix '+kube:validation:'. Use '+kubebuilder:validation:' instead."; \
		exit 1; \
	fi
	@echo "✓ All kubebuilder markers use correct prefix"
```

## Verification Steps

After applying fixes:

```bash
# Regenerate CRDs
make manifests

# Verify enum validation is now in CRD
grep -A5 'registrationStatus' config/crd/bases/ngrok.k8s.ngrok.com_kubernetesoperators.yaml

# Run lint check
make lint
```

Expected output in CRD after fix:
```yaml
registrationStatus:
  type: string
  enum:
    - registered
    - error
    - pending
```

## Summary

| Approach | Pros | Cons |
|----------|------|------|
| **kube-api-linter** (Recommended) | Official K8s SIG tool, comprehensive API linting, golangci-lint integration, auto-fixes | Requires golangci-lint v2, additional setup |
| **Simple grep check** | Easy to implement, no dependencies | Only catches specific patterns, no auto-fix |

## Rollout Plan

1. **Phase 1 (Immediate)**: Fix the 3 invalid markers in existing code
2. **Phase 2 (Short-term)**: Add simple grep-based lint check to CI
3. **Phase 3 (Medium-term)**: Integrate kube-api-linter for comprehensive API linting

## References

- [kube-api-linter](https://github.com/kubernetes-sigs/kube-api-linter) - Official Kubernetes SIG API linter
- [controller-tools markers](https://book.kubebuilder.io/reference/markers) - Kubebuilder marker documentation
- [Kubernetes API Conventions](https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md)
