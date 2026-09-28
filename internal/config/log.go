package config

import (
	"errors"
	"fmt"

	"github.com/spf13/pflag"
)

// ApplyLogFlags feeds the log settings into controller-runtime's --zap-*
// flags, already registered on fs by zap.Options.BindFlags. A flag passed on
// the command line wins; otherwise a non-empty setting applies; otherwise
// zap's own default stays.
func ApplyLogFlags(fs *pflag.FlagSet, log LogConfig) error {
	var errs []error
	for _, f := range []struct{ flag, path, value string }{
		{"zap-log-level", "log.level", log.Level},
		{"zap-encoder", "log.format", log.Format},
		{"zap-stacktrace-level", "log.stacktraceLevel", log.StacktraceLevel},
	} {
		if f.value == "" || fs.Changed(f.flag) {
			continue
		}
		if err := fs.Set(f.flag, f.value); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvName(f.path), err))
		}
	}
	return errors.Join(errs...)
}
