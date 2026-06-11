/*
MIT License

Copyright (c) 2026 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package env

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringFromEnv(t *testing.T) {
	t.Run("returns the value when set", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_DESCRIPTION", "from env")
		assert.Equal(t, "from env", StringFromEnv("NGROK_OPERATOR_DESCRIPTION", "default"))
	})

	t.Run("returns the default when unset", func(t *testing.T) {
		assert.Equal(t, "default", StringFromEnv("NGROK_OPERATOR_DESCRIPTION", "default"))
	})

	t.Run("treats empty as unset", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_DESCRIPTION", "")
		assert.Equal(t, "default", StringFromEnv("NGROK_OPERATOR_DESCRIPTION", "default"))
	})

	t.Run("values may contain '='", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_METADATA", "key1=value1,key2=value2")
		assert.Equal(t, "key1=value1,key2=value2", StringFromEnv("NGROK_OPERATOR_METADATA", ""))
	})
}

func TestBoolFromEnv(t *testing.T) {
	t.Run("parses true and false", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", "false")
		assert.False(t, BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", true))

		t.Setenv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", "true")
		assert.True(t, BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", false))
	})

	t.Run("returns the default when unset or empty", func(t *testing.T) {
		assert.True(t, BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", true))

		t.Setenv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", "")
		assert.True(t, BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", true))
	})

	t.Run("falls back to the default on unparseable values", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", "not-a-bool")
		assert.True(t, BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", true))
		assert.False(t, BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", false))
	})
}

func TestStringSliceFromEnv(t *testing.T) {
	t.Run("splits on commas and trims whitespace", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS", "a, b,c")
		assert.Equal(t, []string{"a", "b", "c"},
			StringSliceFromEnv("NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS", []string{"true"}))
	})

	t.Run("returns the default when unset or empty", func(t *testing.T) {
		assert.Equal(t, []string{"true"},
			StringSliceFromEnv("NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS", []string{"true"}))

		t.Setenv("NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS", "")
		assert.Equal(t, []string{"true"},
			StringSliceFromEnv("NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS", []string{"true"}))
	})
}

// newZapStyleFlagSet mimics how the manager commands bind the
// controller-runtime zap flags via a Go flag set.
func newZapStyleFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("manager", flag.ContinueOnError)
	fs.String("zap-log-level", "info", "")
	fs.String("zap-encoder", "json", "")
	fs.String("zap-stacktrace-level", "error", "")
	return fs
}

func TestBindZapFlagDefaults(t *testing.T) {
	t.Run("applies environment variables as defaults", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_LOG_LEVEL", "debug")
		t.Setenv("NGROK_OPERATOR_LOG_FORMAT", "console")

		fs := newZapStyleFlagSet()
		BindZapFlagDefaults(fs)

		assert.Equal(t, "debug", fs.Lookup("zap-log-level").Value.String())
		assert.Equal(t, "console", fs.Lookup("zap-encoder").Value.String())
		assert.Equal(t, "error", fs.Lookup("zap-stacktrace-level").Value.String())
	})

	t.Run("ignores unset variables and missing flags", func(t *testing.T) {
		fs := flag.NewFlagSet("manager", flag.ContinueOnError)
		fs.String("zap-log-level", "info", "")
		// no zap-encoder / zap-stacktrace-level flags defined
		t.Setenv("NGROK_OPERATOR_LOG_FORMAT", "console")

		BindZapFlagDefaults(fs)
		assert.Equal(t, "info", fs.Lookup("zap-log-level").Value.String())
	})

	t.Run("command line flags win over environment defaults", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_LOG_LEVEL", "debug")

		// Mirror the manager command wiring: bind the Go flag set, apply env
		// defaults, wrap into a cobra command, then parse CLI args.
		goFlagSet := newZapStyleFlagSet()
		BindZapFlagDefaults(goFlagSet)

		var ranWithLogLevel string
		c := &cobra.Command{
			Use: "test",
			RunE: func(cmd *cobra.Command, _ []string) error {
				ranWithLogLevel, _ = cmd.Flags().GetString("zap-log-level")
				return nil
			},
		}
		c.Flags().AddGoFlagSet(goFlagSet)

		c.SetArgs([]string{"--zap-log-level=warn"})
		require.NoError(t, c.Execute())
		assert.Equal(t, "warn", ranWithLogLevel)

		// without a CLI flag, the env-derived default applies
		c2 := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
		goFlagSet2 := newZapStyleFlagSet()
		BindZapFlagDefaults(goFlagSet2)
		c2.Flags().AddGoFlagSet(goFlagSet2)
		c2.SetArgs([]string{})
		require.NoError(t, c2.Execute())
		level, _ := c2.Flags().GetString("zap-log-level")
		assert.Equal(t, "debug", level)
	})

	t.Run("falls back to the default on unparseable values", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_LOG_LEVEL", "garbage")

		fs := flag.NewFlagSet("manager", flag.ContinueOnError)
		fs.Int("zap-log-level", 0, "") // int flag rejects "garbage"
		BindZapFlagDefaults(fs)
		assert.Equal(t, "0", fs.Lookup("zap-log-level").Value.String())
	})
}

// TestPrecedenceWithInlineDefaults verifies the full Argo CD-style precedence
// for flags that use the helpers inline as their defaults:
// CLI flag > environment variable > built-in default.
func TestPrecedenceWithInlineDefaults(t *testing.T) {
	newCmd := func() (*cobra.Command, *pflag.FlagSet) {
		c := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
		c.Flags().Bool("enable-feature-ingress", BoolFromEnv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", true), "")
		c.Flags().String("description", StringFromEnv("NGROK_OPERATOR_DESCRIPTION", "built-in"), "")
		return c, c.Flags()
	}

	t.Run("environment overrides the built-in default", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", "false")
		t.Setenv("NGROK_OPERATOR_DESCRIPTION", "from env")

		c, flags := newCmd()
		c.SetArgs([]string{})
		require.NoError(t, c.Execute())

		enabled, _ := flags.GetBool("enable-feature-ingress")
		assert.False(t, enabled)
		description, _ := flags.GetString("description")
		assert.Equal(t, "from env", description)
	})

	t.Run("CLI flags override the environment", func(t *testing.T) {
		t.Setenv("NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", "false")

		c, flags := newCmd()
		c.SetArgs([]string{"--enable-feature-ingress=true"})
		require.NoError(t, c.Execute())

		enabled, _ := flags.GetBool("enable-feature-ingress")
		assert.True(t, enabled)
	})
}
