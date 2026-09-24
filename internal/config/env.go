package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

// EnvPrefix is prepended to a flag's name to form its environment variable.
const EnvPrefix = "NGROK_OPERATOR_"

// EnvName returns the environment variable that backs a flag:
// --log-level reads NGROK_OPERATOR_LOG_LEVEL.
func EnvName(flag string) string {
	return EnvPrefix + strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
}

// ApplyEnv fills in any flag that was not passed on the command line from its
// environment variable, which puts the environment between the command line and
// the config file: flag > env > file > Default().
//
// The mapping is derived by walking the flag set rather than written down per
// flag, so a new flag gets environment support for free and the two can never
// disagree about a name. Values use the encodings pflag already implements, so
// maps and lists need no scheme of our own.
//
// ConfigFlag is skipped: the files it names are read before flags are
// registered, so setting it here would have no effect. PreParseConfigPaths
// reads its variable directly.
func ApplyEnv(fs *pflag.FlagSet) error {
	var errs []error

	fs.VisitAll(func(f *pflag.Flag) {
		if f.Changed || f.Name == ConfigFlag {
			return
		}

		value, ok := os.LookupEnv(EnvName(f.Name))
		if !ok {
			return
		}

		if err := fs.Set(f.Name, value); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvName(f.Name), err))
		}
	})

	return errors.Join(errs...)
}
