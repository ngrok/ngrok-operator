package healthcheck

import (
	"context"
	"net/http"
	"sync"
)

// ChannelHealthChecker is an implementation of the HealthChecker interface
// that receives updates to the readiness and aliveness of a resource over a channel
// and stores it. It returns the latest state when queried.
//
// This is useful for cases where the health state is updated asynchronously,
// such as when using a channel to receive updates from an agent or service.
type ChannelHealthChecker struct {
	readyErr error
	readyMu  sync.Mutex
	aliveErr error
	aliveMu  sync.Mutex
}

// NewChannelHealthChecker creates a new [ChannelHealthChecker] receives updates
// on the provided readiness and aliveness channels.
func NewChannelHealthChecker(readyChan, aliveChan <-chan error) HealthChecker {
	chc := &ChannelHealthChecker{
		readyErr: nil,
		readyMu:  sync.Mutex{},
		aliveErr: nil,
		aliveMu:  sync.Mutex{},
	}

	go func() {
		for err := range readyChan {
			chc.readyMu.Lock()
			chc.readyErr = err
			chc.readyMu.Unlock()
		}
	}()

	go func() {
		for err := range aliveChan {
			chc.aliveMu.Lock()
			chc.aliveErr = err
			chc.aliveMu.Unlock()
		}
	}()

	return chc
}

func (chc *ChannelHealthChecker) Ready(_ context.Context, _ *http.Request) error {
	chc.readyMu.Lock()
	defer chc.readyMu.Unlock()
	return chc.readyErr
}

func (chc *ChannelHealthChecker) Alive(_ context.Context, _ *http.Request) error {
	chc.readyMu.Lock()
	defer chc.readyMu.Unlock()
	return chc.aliveErr
}
