// Package markerlint provides a go/analysis analyzer that detects invalid
// kubebuilder marker comments.
//
// Kubebuilder markers (e.g., +kubebuilder:validation:Required) are parsed from
// Go comments to generate CRD manifests. However, typos in these markers fail
// silently - controller-gen simply ignores unknown markers by design.
//
// This analyzer:
// 1. Catches common prefix typos (+kube: instead of +kubebuilder:)
// 2. Validates +kubebuilder: markers against the controller-tools marker registry
package markerlint

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the go/analysis analyzer for kubebuilder marker linting.
var Analyzer = &analysis.Analyzer{
	Name: "markerlint",
	Doc:  "detects invalid kubebuilder marker comments",
	Run:  run,
}

// suspiciousPattern represents a pattern that might be a typo.
type suspiciousPattern struct {
	pattern    *regexp.Regexp
	suggestion string
}

// suspiciousPatterns contains common typos for kubebuilder marker prefixes.
var suspiciousPatterns = []suspiciousPattern{
	{
		pattern:    regexp.MustCompile(`^\s*\+kube:`),
		suggestion: `did you mean "+kubebuilder:"? (missing "builder")`,
	},
	{
		pattern:    regexp.MustCompile(`^\s*\+kuberbuilder:`),
		suggestion: `did you mean "+kubebuilder:"? (extra 'r')`,
	},
	{
		pattern:    regexp.MustCompile(`^\s*\+kubebilder:`),
		suggestion: `did you mean "+kubebuilder:"? (missing 'u')`,
	},
	{
		pattern:    regexp.MustCompile(`^\s*\+kubebuidler:`),
		suggestion: `did you mean "+kubebuilder:"? (transposed letters)`,
	},
	{
		pattern:    regexp.MustCompile(`^\s*\+kubbuilder:`),
		suggestion: `did you mean "+kubebuilder:"? (missing 'e')`,
	},
	{
		pattern:    regexp.MustCompile(`^\s*\+kubebuiler:`),
		suggestion: `did you mean "+kubebuilder:"? (missing 'd')`,
	},
}

// markerRegex matches a kubebuilder marker and captures the marker name.
// It handles various forms like:
//   - +kubebuilder:validation:Required (no args)
//   - +kubebuilder:validation:Enum=a;b;c (with = args)
//   - +kubebuilder:printcolumn:JSONPath=".spec.field",name="Field" (with : args)
var markerRegex = regexp.MustCompile(`^\s*\+(kubebuilder:[a-zA-Z0-9:]+)`)

func run(pass *analysis.Pass) (interface{}, error) {
	validMarkers, err := getValidMarkerNames()
	if err != nil {
		// If we can't load the registry, report it but don't fail the analysis
		pass.Reportf(token.NoPos, "markerlint: failed to load marker registry: %v", err)
		return nil, nil
	}

	for _, file := range pass.Files {
		for _, cg := range file.Comments {
			for _, comment := range cg.List {
				checkComment(pass, comment, validMarkers)
			}
		}
	}
	return nil, nil
}

func checkComment(pass *analysis.Pass, comment *ast.Comment, validMarkers map[string]bool) {
	text := comment.Text
	if !strings.HasPrefix(text, "//") {
		return
	}

	content := strings.TrimPrefix(text, "//")

	// Check for suspicious prefix typos first
	for _, sp := range suspiciousPatterns {
		if sp.pattern.MatchString(content) {
			pass.Report(analysis.Diagnostic{
				Pos:      comment.Pos(),
				End:      token.Pos(int(comment.Pos()) + len(text)),
				Category: "markerlint",
				Message:  sp.suggestion,
			})
			return
		}
	}

	// Check for invalid kubebuilder markers
	matches := markerRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return
	}

	markerName := matches[1]

	// Try to find a matching valid marker
	if !isValidMarker(markerName, validMarkers) {
		suggestion := findSuggestion(markerName, validMarkers)
		msg := "unknown kubebuilder marker \"+" + markerName + "\""
		if suggestion != "" {
			msg += ", did you mean \"+" + suggestion + "\"?"
		}
		pass.Report(analysis.Diagnostic{
			Pos:      comment.Pos(),
			End:      token.Pos(int(comment.Pos()) + len(text)),
			Category: "markerlint",
			Message:  msg,
		})
	}
}

// isValidMarker checks if a marker name matches any registered marker.
// It handles markers with arguments like +kubebuilder:printcolumn:JSONPath=...
// by checking progressively shorter prefixes.
func isValidMarker(markerName string, validMarkers map[string]bool) bool {
	// First, try the full name
	if validMarkers[markerName] {
		return true
	}

	// For markers that take named arguments (like printcolumn:JSONPath=...),
	// try progressively shorter prefixes
	parts := strings.Split(markerName, ":")
	for i := len(parts) - 1; i >= 1; i-- {
		prefix := strings.Join(parts[:i], ":")
		if validMarkers[prefix] {
			return true
		}
	}

	return false
}

// findSuggestion uses Levenshtein distance to find the closest valid marker.
func findSuggestion(unknown string, validMarkers map[string]bool) string {
	minDist := 1000
	suggestion := ""

	for valid := range validMarkers {
		dist := levenshtein(unknown, valid)
		if dist < minDist && dist <= 3 {
			minDist = dist
			suggestion = valid
		}
	}

	return suggestion
}

// levenshtein calculates the Levenshtein distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	if len(a) > len(b) {
		a, b = b, a
	}

	prev := make([]int, len(a)+1)
	curr := make([]int, len(a)+1)

	for i := range prev {
		prev[i] = i
	}

	for j := 1; j <= len(b); j++ {
		curr[0] = j
		for i := 1; i <= len(a); i++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[i] = min(curr[i-1]+1, min(prev[i]+1, prev[i-1]+cost))
		}
		prev, curr = curr, prev
	}

	return prev[len(a)]
}
