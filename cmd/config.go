package cmd

import (
	"github.com/spf13/cobra"

	"github.com/ngrok/ngrok-operator/internal/config"
)

// loadConfig builds the operator configuration: NGROK_OPERATOR_* environment
// variables over the built-in defaults, then any config flags passed on the
// command line. Its log settings are then applied to c's --zap-* flags.
func loadConfig(c *cobra.Command, flags *config.Flags) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := flags.Apply(cfg); err != nil {
		return nil, err
	}
	if err := config.ApplyLogFlags(c.Flags(), cfg.Log); err != nil {
		return nil, err
	}
	return cfg, nil
}
