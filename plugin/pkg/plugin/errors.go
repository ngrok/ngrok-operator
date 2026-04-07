package plugin

import (
	"fmt"

	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
)

func rpcErrorf(format string, args ...any) pluginTypes.RpcError {
	return pluginTypes.RpcError{ErrorString: fmt.Sprintf(format, args...)}
}

func toRpcError(err error) pluginTypes.RpcError {
	if err == nil {
		return pluginTypes.RpcError{}
	}
	return pluginTypes.RpcError{ErrorString: err.Error()}
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
