package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveComputeMode(t *testing.T) {
	t.Run("new poller setting enables polling", func(t *testing.T) {
		mode := resolveComputeMode(`{"poller":{"enabled":true}}`, false)
		require.True(t, mode.pollerEnabled)
		require.False(t, mode.legacyEnabledConfigured)
		require.False(t, mode.remoteAccessOverridesPoller)
	})

	t.Run("new poller setting disables polling", func(t *testing.T) {
		require.False(t, resolveComputeMode(`{"poller":{"enabled":false}}`, false).pollerEnabled)
	})

	t.Run("remote access overrides new poller setting", func(t *testing.T) {
		mode := resolveComputeMode(`{"poller":{"enabled":true},"remoteAccess":{"enabled":true}}`, true)
		require.False(t, mode.pollerEnabled)
		require.True(t, mode.remoteAccessOverridesPoller)
	})

	t.Run("remote access overrides deprecated enabled setting", func(t *testing.T) {
		mode := resolveComputeMode(`{"enabled":true,"remoteAccess":{"enabled":true}}`, true)
		require.False(t, mode.pollerEnabled)
		require.True(t, mode.legacyEnabledConfigured)
		require.True(t, mode.remoteAccessOverridesPoller)
	})

	t.Run("new poller setting takes precedence over deprecated setting", func(t *testing.T) {
		mode := resolveComputeMode(`{"enabled":false,"poller":{"enabled":true}}`, false)
		require.True(t, mode.pollerEnabled)
		require.True(t, mode.legacyEnabledConfigured)
	})

	t.Run("legacy metadata defaults to polling", func(t *testing.T) {
		require.True(t, resolveComputeMode(`{"pool_join_key":"cp_example"}`, false).pollerEnabled)
	})

	t.Run("missing or invalid metadata disables polling", func(t *testing.T) {
		require.False(t, resolveComputeMode("", false).pollerEnabled)
		require.False(t, resolveComputeMode("{", false).pollerEnabled)
	})
}
