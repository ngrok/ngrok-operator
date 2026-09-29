package cmd

import "github.com/spf13/pflag"

// managerOpts are the controller-runtime manager's flags, shared by every
// command. They are runtime plumbing the chart passes as args, not settings,
// so they have no environment variable.
type managerOpts struct {
	releaseName string
	metricsAddr string
	probeAddr   string
	managerName string
}

func addManagerFlags(fs *pflag.FlagSet, o *managerOpts, managerName string) {
	fs.StringVar(&o.releaseName, "release-name", "ngrok-operator", "Helm Release name for the deployed operator")
	fs.StringVar(&o.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	fs.StringVar(&o.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.StringVar(&o.managerName, "manager-name", managerName, "Manager name to identify unique instances of this component")
}
