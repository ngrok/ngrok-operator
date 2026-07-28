package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestWatchNamespaceCacheConfig(t *testing.T) {
	t.Run("no flag watches all namespaces", func(t *testing.T) {
		require.Nil(t, watchNamespaceCacheConfig(nil))
		require.Nil(t, watchNamespaceCacheConfig([]string{}))
	})

	t.Run("single namespace scopes the cache", func(t *testing.T) {
		require.Equal(t, map[string]cache.Config{"foo": {}}, watchNamespaceCacheConfig([]string{"foo"}))
	})

	t.Run("repeated flags watch the union", func(t *testing.T) {
		require.Equal(t, map[string]cache.Config{
			"foo": {}, "ngrok-compute": {},
		}, watchNamespaceCacheConfig([]string{"foo", "ngrok-compute"}))
	})

	t.Run("duplicates collapse", func(t *testing.T) {
		require.Equal(t, map[string]cache.Config{"foo": {}}, watchNamespaceCacheConfig([]string{"foo", "foo"}))
	})

	t.Run("an empty value widens to all namespaces", func(t *testing.T) {
		require.Nil(t, watchNamespaceCacheConfig([]string{""}))
		require.Nil(t, watchNamespaceCacheConfig([]string{"", "ngrok-compute"}))
		require.Nil(t, watchNamespaceCacheConfig([]string{"ngrok-compute", ""}))
	})
}

func TestAgentWatchNamespaceFlagIsRepeatable(t *testing.T) {
	c := agentCmd()
	require.NoError(t, c.ParseFlags([]string{
		"--watch-namespace=foo", "--watch-namespace=ngrok-compute",
	}))
	watchNamespaces, err := c.Flags().GetStringArray("watch-namespace")
	require.NoError(t, err)
	require.Equal(t, []string{"foo", "ngrok-compute"}, watchNamespaces)
}
