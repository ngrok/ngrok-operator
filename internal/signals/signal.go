/*
MIT License

Copyright (c) 2025 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package signals

import (
	"sync"
	"sync/atomic"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	setupShutdownSignalHandlerOnce sync.Once
	isShuttingDown                 atomic.Bool
	shutdownCallbacks              []func()
	shutdownCallbacksMutex         sync.Mutex
)

func init() {
	isShuttingDown.Store(false)
}

// registerShutdownHandler sets up a context that is canceled when a shutdown signal is received.
// This is a wrapper around controller-runtime's SetupSignalHandler to ensure
// it is only set up once.
func registerShutdownHandlers() {
	setupShutdownSignalHandlerOnce.Do(func() {
		shutdown := ctrl.SetupSignalHandler()
		// Wait for shutdown signal
		go func() {
			<-shutdown.Done()
			isShuttingDown.Store(true)
			// Call all registered callbacks
			for _, cb := range shutdownCallbacks {
				go cb()
			}
		}()
	})
}

func OnShutdown(f func()) {
	registerShutdownHandlers()

	shutdownCallbacksMutex.Lock()
	defer shutdownCallbacksMutex.Unlock()
	shutdownCallbacks = append(shutdownCallbacks, f)
}

// IsShuttingDown returns true if a shutdown signal has been received.
func IsShuttingDown() bool {
	return isShuttingDown.Load()
}
