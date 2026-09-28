package cmd

import (
	"github.com/spf13/cobra"

	"github.com/ngrok/ngrok-operator/internal/config"
)

// loadConfig reads the operator configuration from NGROK_OPERATOR_*
// environment variables and applies its log settings to c's --zap-* flags.
func loadConfig(c *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := config.ApplyLogFlags(c.Flags(), cfg.Log); err != nil {
		return nil, err
	}
	return cfg, nil
}
