package cmd

import (
	"regexp"
	"strings"
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
	t.Setenv("NGROK_OPERATOR_ROOT_CAS", "host") // only the agent reads it
	require.NoError(t, flags.Validate(newRoot()))

	t.Setenv("NGROK_OPERATOR_FEATURES__DRAIN_POLICY_TYPO", "Delete")
	require.ErrorContains(t, flags.Validate(newRoot()), "NGROK_OPERATOR_FEATURES__DRAIN_POLICY_TYPO")
}

// envFormat is NGROK_OPERATOR_ and a values path: "__" between levels, "_"
// between words.
var envFormat = regexp.MustCompile(`^NGROK_OPERATOR_[A-Z0-9]+(_[A-Z0-9]+)*(__[A-Z0-9]+(_[A-Z0-9]+)*)*$`)

// TestSettingNames checks each setting's variable is in the documented format
// and its flag is the variable without the prefix, in kebab case.
func TestSettingNames(t *testing.T) {
	kebab := strings.NewReplacer("__", "-", "_", "-")
	for f := range flags.Flags(newRoot()) {
		env := f.Annotations[flags.AnnotationEnv]
		if env == nil {
			continue
		}
		assert.Regexp(t, envFormat, env[0])
		assert.Equal(t, strings.ToLower(kebab.Replace(strings.TrimPrefix(env[0], "NGROK_OPERATOR_"))), f.Name, env[0])
	}
}
