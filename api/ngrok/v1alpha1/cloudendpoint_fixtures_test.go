package v1alpha1

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// Keep the uninstall fixtures in sync with the API even when the live E2E
// suite only runs in the merge queue. A permissive decoder would silently
// discard removed fields instead of catching the invalid manifests.
func TestCloudEndpointUninstallFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "tests", "chainsaw-uninstall", "_fixtures", "cloudendpoint*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var endpoint CloudEndpoint
			require.NoError(t, yaml.UnmarshalStrict(data, &endpoint))
		})
	}
}
