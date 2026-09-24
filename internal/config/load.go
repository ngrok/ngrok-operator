package config

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

// Load reads each path in order onto the built-in defaults.
//
// Only fields present in a document are written, so precedence falls out of
// decode order: later files override earlier ones, and anything no file
// mentions keeps its value from Default().
//
// There is deliberately no notion of a component here. The chart resolves the
// shared-to-component merge at render time and hands each component a document
// that is already its own, so loading is a decode and nothing more.
func Load(paths []string) (*Config, error) {
	cfg := Default()

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config %s: %w", path, err)
		}

		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	return cfg, nil
}

// ConfigFlag is the flag name used to point at config files, on every component.
const ConfigFlag = "config"

// PreParseConfigPaths extracts --config values from args before the real flag
// set is built. Config has to be loaded first so its values can be used as the
// defaults for every other flag, which is what makes an explicitly passed flag
// win over the file without any per-flag merge logic.
//
// Unknown flags are tolerated because at this point none of the real flags are
// registered yet. Parse errors are ignored: the real parse reports them.
func PreParseConfigPaths(args []string) []string {
	var paths []string

	fs := pflag.NewFlagSet("preparse", pflag.ContinueOnError)
	fs.ParseErrorsWhitelist.UnknownFlags = true
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.StringArrayVar(&paths, ConfigFlag, nil, "")

	_ = fs.Parse(args)

	// ApplyEnv cannot reach this flag, so read its variable here to keep
	// NGROK_OPERATOR_CONFIG working like every other setting.
	if len(paths) == 0 {
		if value, ok := os.LookupEnv(EnvName(ConfigFlag)); ok && value != "" {
			paths = []string{value}
		}
	}

	return paths
}
