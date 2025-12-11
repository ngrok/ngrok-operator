#!/usr/bin/env bash
# lint-markers.sh - Detect invalid kubebuilder marker prefixes in Go files
#
# This script checks for common typos in kubebuilder marker comments that would
# be silently ignored by controller-gen, causing validation rules to not be
# applied to generated CRDs.

set -euo pipefail

# Colors for output (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    NC=''
fi

# Directory to scan (default to api/)
SCAN_DIR="${1:-api/}"

# Invalid marker patterns that look like kubebuilder markers but use wrong prefix
# These are silently ignored by controller-gen
INVALID_PATTERNS=(
    '+kube:validation:'      # Should be +kubebuilder:validation:
    '+kube:default:'         # Should be +kubebuilder:default:
    '+kube:object:'          # Should be +kubebuilder:object:
    '+kube:subresource:'     # Should be +kubebuilder:subresource:
    '+kube:printcolumn:'     # Should be +kubebuilder:printcolumn:
    '+kube:resource:'        # Should be +kubebuilder:resource:
    '+kube:rbac:'            # Should be +kubebuilder:rbac:
    '+kube:webhook:'         # Should be +kubebuilder:webhook:
    '+kube:skip'             # Should be +kubebuilder:skip
    '+kube:storageversion'   # Should be +kubebuilder:storageversion
)

errors_found=0

echo "Checking for invalid kubebuilder marker prefixes in ${SCAN_DIR}..."
echo ""

for pattern in "${INVALID_PATTERNS[@]}"; do
    # Use grep to find matches, suppress errors for no matches
    if matches=$(grep -rn --include='*.go' "$pattern" "$SCAN_DIR" 2>/dev/null); then
        if [[ -n "$matches" ]]; then
            echo -e "${RED}ERROR: Found invalid marker prefix '${pattern}'${NC}"
            echo "       These markers are silently ignored by controller-gen."
            echo "       Use '+kubebuilder:' prefix instead of '+kube:'"
            echo ""
            echo "$matches" | while IFS= read -r line; do
                echo "  $line"
            done
            echo ""
            errors_found=$((errors_found + 1))
        fi
    fi
done

if [[ $errors_found -gt 0 ]]; then
    echo -e "${RED}Found $errors_found invalid marker pattern(s).${NC}"
    echo ""
    echo "To fix: Replace '+kube:' with '+kubebuilder:' in the marker comments."
    echo "Example: '// +kube:validation:Optional' -> '// +kubebuilder:validation:Optional'"
    exit 1
fi

echo -e "${GREEN}✓ All kubebuilder markers use correct prefix${NC}"
exit 0
