package markerlint

import (
	"sync"

	"sigs.k8s.io/controller-tools/pkg/crd"
	crdmarkers "sigs.k8s.io/controller-tools/pkg/crd/markers"
	"sigs.k8s.io/controller-tools/pkg/deepcopy"
	"sigs.k8s.io/controller-tools/pkg/markers"
	"sigs.k8s.io/controller-tools/pkg/rbac"
	"sigs.k8s.io/controller-tools/pkg/schemapatcher"
	"sigs.k8s.io/controller-tools/pkg/webhook"
)

var (
	registryOnce  sync.Once
	markerNames   map[string]bool
	registryError error
)

// getValidMarkerNames returns all valid kubebuilder marker names from controller-tools.
// The registry is built once and cached for subsequent calls.
func getValidMarkerNames() (map[string]bool, error) {
	registryOnce.Do(func() {
		markerNames, registryError = buildMarkerRegistry()
	})
	return markerNames, registryError
}

// buildMarkerRegistry creates a registry with all markers from controller-tools generators.
func buildMarkerRegistry() (map[string]bool, error) {
	reg := &markers.Registry{}

	// Register markers from all standard generators
	generators := []interface {
		RegisterMarkers(*markers.Registry) error
	}{
		crd.Generator{},
		rbac.Generator{},
		webhook.Generator{},
		deepcopy.Generator{},
		schemapatcher.Generator{},
	}

	for _, gen := range generators {
		if err := gen.RegisterMarkers(reg); err != nil {
			return nil, err
		}
	}

	// Also register CRD validation markers directly (some might not be covered by Generator)
	if err := crdmarkers.Register(reg); err != nil {
		return nil, err
	}

	// Extract all marker names
	names := make(map[string]bool)
	for _, def := range reg.AllDefinitions() {
		names[def.Name] = true
	}

	return names, nil
}
