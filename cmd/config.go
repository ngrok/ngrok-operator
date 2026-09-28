package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ngrok/ngrok-operator/internal/config"
)

// bindConfig loads the --config files and registers every shared app config
// flag on c, using the loaded values as the flag defaults. The resulting
// precedence is flag > NGROK_OPERATOR_* env var > config file > built-in
// default.
//
// The files are read before cobra parses the command line, so a load error is
// held until PreRunE. By then every flag is registered, and cobra reports the
// load error instead of an unknown flag.
func bindConfig(c *cobra.Command) (*config.Config, *zap.Options) {
	paths := config.PreParseConfigPaths(os.Args[1:])
	c.Flags().StringArray(config.ConfigFlag, paths, "Path to a YAML config file. May be repeated; later files override earlier ones.")

	cfg, loadErr := config.Load(paths)
	if loadErr != nil {
		cfg = config.Default()
	}
	zapOpts, flagErr := config.RegisterFlags(c.Flags(), cfg)

	c.PreRunE = func(c *cobra.Command, _ []string) error {
		if err := errors.Join(loadErr, flagErr); err != nil {
			return err
		}
		return config.ApplyEnv(c.Flags())
	}

	return cfg, zapOpts
}
