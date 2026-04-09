package healthcheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestChannelHealthChecker(t *testing.T) {
	// This test will check the functionality of the ChannelHealthChecker.
	// It should ensure that the health checker can correctly identify the health status of channels.
	// The test will create mock channels and simulate health checks, verifying that the results match expected outcomes.
	readyChan := make(chan error, 1)
	aliveChan := make(chan error, 1)

	chc := NewChannelHealthChecker(readyChan, aliveChan)

	ctx := t.Context()

	// Test initial state
	assert.NoError(t, chc.Ready(ctx, nil), "Expected no error for initial Ready check")
	assert.NoError(t, chc.Alive(ctx, nil), "Expected no error for initial Alive check")

	// Simulate a ready error
	readyChan <- errors.New("ready error")
	assert.Eventually(t, func() bool {
		err := chc.Ready(ctx, nil)
		return err != nil && err.Error() == "ready error"
	}, 3*time.Second, 100*time.Millisecond, "Expected Ready check to return 'ready error' after sending it on channel")

	// Simulate an alive error
	aliveChan <- errors.New("alive error")
	assert.Eventually(t, func() bool {
		err := chc.Alive(ctx, nil)
		return err != nil && err.Error() == "alive error"
	}, 3*time.Second, 100*time.Millisecond, "Expected Alive check to return 'alive error' after sending it on channel")

	// Test that closing channels does not cause panic
	// and that they continue to return the last error
	close(readyChan)
	close(aliveChan)

	// After closing the channels, the last error should still be returned
	assert.Error(t, chc.Ready(ctx, nil), "Expected Ready check to return last error after channel close")
	assert.Error(t, chc.Alive(ctx, nil), "Expected Alive check to return last error after channel close")
}

func TestAliveDataRace(t *testing.T) {
	t.Parallel()

	aliveChan := make(chan error)
	readyChan := make(chan error)

	chc := NewChannelHealthChecker(readyChan, aliveChan)

	var wg sync.WaitGroup
	const iterations = 1000

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			aliveChan <- errors.New("not alive")
			aliveChan <- nil
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations*2; i++ {
			_ = chc.Alive(context.Background(), nil)
		}
	}()

	wg.Wait()
}
