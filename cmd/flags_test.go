package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ngrok/ngrok-operator/internal/flags"
)

// newRoot builds a fresh command tree, read from the environment as it is
// when called.
func newRoot() *cobra.Command {
	root := &cobra.Command{Use: "ngrok-operator"}
	root.AddCommand(apiCmd(), agentCmd(), bindingsForwarderCmd())
	return root
}

func TestEachEnvNamesOneFlag(t *testing.T) {
	flagByEnv := map[string]string{}
	for f := range flags.Flags(newRoot()) {
		env := f.Annotations[flags.AnnotationEnv]
		if env == nil {
			continue
		}
		if name, ok := flagByEnv[env[0]]; ok {
			assert.Equal(t, name, f.Name, "%s is read by two different flags", env[0])
		}
		flagByEnv[env[0]] = f.Name
	}
	assert.NotEmpty(t, flagByEnv)
}

func TestValidateCommands(t *testing.T) {
	t.Setenv("NGROK_OPERATOR_NGROK_ROOT_CAS", "host") // only the agent reads it
	require.NoError(t, flags.Validate(newRoot()))

	t.Setenv("NGROK_OPERATOR_FEATURES_DRAIN_POLICY_TYPO", "Delete")
	require.ErrorContains(t, flags.Validate(newRoot()), "NGROK_OPERATOR_FEATURES_DRAIN_POLICY_TYPO")
}
