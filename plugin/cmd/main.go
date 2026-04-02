package main

import (
	rolloutsPlugin "github.com/argoproj/argo-rollouts/rollout/trafficrouting/plugin/rpc"
	goPlugin "github.com/hashicorp/go-plugin"

	"github.com/ngrok/rollouts-plugin-trafficrouter-ngrok/pkg/plugin"
)

// handshakeConfig must match the values used by the Argo Rollouts controller client.
// These are not security features — they prevent accidentally running the wrong binary.
var handshakeConfig = goPlugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ARGO_ROLLOUTS_RPC_PLUGIN",
	MagicCookieValue: "trafficrouter",
}

func main() {
	pluginMap := map[string]goPlugin.Plugin{
		"RpcTrafficRouterPlugin": &rolloutsPlugin.RpcTrafficRouterPlugin{
			Impl: &plugin.NgrokTrafficRouter{},
		},
	}

	goPlugin.Serve(&goPlugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
	})
}
