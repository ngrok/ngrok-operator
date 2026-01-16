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

// Package drainstate provides the shared DrainState interface for checking
// if the operator is in drain mode during uninstall.
package drainstate

import "context"

// State is an interface for checking if the operator is in drain mode.
// Implementations should cache the draining state - once true, it should never reset.
type State interface {
	IsDraining(ctx context.Context) bool
}

// IsDraining is a helper function that safely checks if drain mode is active.
// Returns false if state is nil, avoiding the need for nil checks at every call site.
func IsDraining(ctx context.Context, state State) bool {
	if state == nil {
		return false
	}
	return state.IsDraining(ctx)
}

// NeverDraining is a State implementation that always returns false.
// Useful for testing or when drain mode is not enabled.
type NeverDraining struct{}

var _ State = NeverDraining{}

func (NeverDraining) IsDraining(context.Context) bool { return false }

// AlwaysDraining is a State implementation that always returns true.
// Useful for testing drain behavior.
type AlwaysDraining struct{}

var _ State = AlwaysDraining{}

func (AlwaysDraining) IsDraining(context.Context) bool { return true }
