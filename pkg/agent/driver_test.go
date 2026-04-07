package agent

import (
	"sync"
	"testing"
)

func TestDriverCloseOnceNoPanic(t *testing.T) {
	d := &driver{
		done: make(chan bool),
	}

	// Simulate multiple Stop/Restart RPCs closing d.done concurrently.
	// Without sync.Once this would panic on the second close.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.closeOnce.Do(func() { close(d.done) })
		}()
	}
	wg.Wait()

	// Verify channel is closed
	select {
	case <-d.done:
		// expected
	default:
		t.Fatal("expected d.done to be closed")
	}
}
