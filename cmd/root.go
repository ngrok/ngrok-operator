package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ngrok/ngrok-operator/internal/flags"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// unknownEnv holds the NGROK_OPERATOR__* variables no command reads, found
// before the command runs and logged once it has a logger.
var unknownEnv []string

var rootCmd = &cobra.Command{
	Use: "ngrok-operator",
	PersistentPreRunE: func(c *cobra.Command, _ []string) error {
		unknownEnv = flags.Unknown(c.Root())
		return flags.Validate(c.Root())
	},
}

// warnUnknownEnv logs each NGROK_OPERATOR__* variable no command reads, so a
// misspelled setting is visible instead of silently ignored.
func warnUnknownEnv() {
	for _, name := range unknownEnv {
		setupLog.Info("ignoring environment variable that is not an ngrok-operator setting", "name", name)
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
