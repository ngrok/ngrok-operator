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
package controller

import "sync"

type Terminator interface {
	StartTermination()
}

type Terminating interface {
	IsTerminating() bool
}

type channelTerminator struct {
	termChan chan<- struct{}
	termOnce sync.Once
}

func (ct *channelTerminator) StartTermination() {
	ct.termOnce.Do(func() {
		close(ct.termChan)
	})
}

type channelTerminating struct {
	termChan <-chan struct{}
}

func (ct *channelTerminating) IsTerminating() bool {
	select {
	case <-ct.termChan:
		return true
	default:
		return false
	}
}

// NewChannelTerminationPair creates a new pair of Terminator and Terminating
// that share the same termination channel.
func NewChannelTerminationPair() (Terminating, Terminator) {
	termChan := make(chan struct{})
	return &channelTerminating{termChan: termChan}, &channelTerminator{termChan: termChan}
}

// NewNoOpTerminating creates a new no-op Terminating instance.
func NewNoOpTerminating() *noOpTerminating {
	return &noOpTerminating{}
}

// noOpTerminating is a no-op implementation of the Terminating interface that always returns false for IsTerminating.
type noOpTerminating struct{}

// IsTerminating always returns false.
func (nt *noOpTerminating) IsTerminating() bool {
	return false
}
