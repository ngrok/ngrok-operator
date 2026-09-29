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

var rootCmd = &cobra.Command{
	Use: "ngrok-operator",
	PersistentPreRunE: func(c *cobra.Command, _ []string) error {
		return flags.Validate(c.Root())
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
