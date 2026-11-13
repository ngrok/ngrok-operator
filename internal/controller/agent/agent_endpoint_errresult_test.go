package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// TestErrResultURLHeldByUnpooledEndpoint covers the retry for ERR_NGROK_334, returned while the
// unpooled CloudEndpoint an AgentEndpoint replaces still holds its URL: a short fixed requeue rather
// than the error, whose backoff grows on every attempt.
func TestErrResultURLHeldByUnpooledEndpoint(t *testing.T) {
	r := &AgentEndpointReconciler{Log: logr.Discard()}
	err := errors.New("The endpoint 'http://example.ngrok.app' is already online. ERR_NGROK_334")

	res, gotErr := r.errResult(&ngrokv1.AgentEndpoint{}, err)
	assert.NoError(t, gotErr)
	assert.Equal(t, 2*time.Second, res.RequeueAfter)
}
