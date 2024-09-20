package healthcheck_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/ngrok/ngrok-operator/internal/healthcheck"
	"github.com/stretchr/testify/assert"
)

type mockChecker struct {
	aliveError error
	readyError error
}

func (m *mockChecker) Alive(ctx context.Context, r *http.Request) error {
	return m.aliveError
}

func (m *mockChecker) Ready(ctx context.Context, r *http.Request) error {
	return m.readyError
}
func Test_HealthChecker(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background() // fake context
	var req *http.Request       // fake request

	// nothing in the instance
	// should return nil
	assert.Nil(healthcheck.Alive(ctx, req))
	assert.Nil(healthcheck.Ready(ctx, req))

	healthyChecker := &mockChecker{
		aliveError: nil,
		readyError: nil,
	}

	unaliveChecker := &mockChecker{
		aliveError: errors.New("not alive"),
		readyError: nil,
	}

	unreadyChecker := &mockChecker{
		aliveError: nil,
		readyError: errors.New("not ready"),
	}

	// register multiple healthy checkers
	healthcheck.RegisterHealthChecker(healthyChecker)
	healthcheck.RegisterHealthChecker(healthyChecker)
	healthcheck.RegisterHealthChecker(healthyChecker)

	// all healthy checkers
	// should return nil
	assert.Nil(healthcheck.Alive(ctx, req))
	assert.Nil(healthcheck.Ready(ctx, req))

	// add an unalive checker
	// should return the error from the unalive checker
	healthcheck.RegisterHealthChecker(unaliveChecker)
	assert.EqualError(healthcheck.Alive(ctx, req), "not alive")
	assert.Nil(healthcheck.Ready(ctx, req))

	// add an unready checker
	// should return the error from the unready checker
	healthcheck.RegisterHealthChecker(unreadyChecker)
	assert.EqualError(healthcheck.Alive(ctx, req), "not alive") // still registered
	assert.EqualError(healthcheck.Ready(ctx, req), "not ready")

	// add new healthy checker
	// should still receive same errors as previous (short circuit)
	healthcheck.RegisterHealthChecker(healthyChecker)
	assert.EqualError(healthcheck.Alive(ctx, req), "not alive") // still registered
	assert.EqualError(healthcheck.Ready(ctx, req), "not ready") // still registered
}
