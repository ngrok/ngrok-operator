package testutils

import (
	"path/filepath"
)

// OperatorCRDPath returns the relative path to the operator CRD templates
// used in envtest setups. The path is relative to the repository root.
//
// Example:
//
//	testutils.OperatorCRDPath() // returns "helm/ngrok-crds/templates"
//	testutils.OperatorCRDPath("..", "..") // returns "../../helm/ngrok-crds/templates"
func OperatorCRDPath(relPathParts ...string) string {
	return filepath.Join(append(relPathParts, "helm", "ngrok-crds", "templates")...)
}
