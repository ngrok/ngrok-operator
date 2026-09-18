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
package cmd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunNgrokAPIPreflightProbes(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name   string
		probes []ngrokAPIPreflightProbe
		expect []error
	}{
		{
			name:   "no probes",
			probes: nil,
			expect: []error{},
		},
		{
			name: "results stay indexed to their probe",
			probes: []ngrokAPIPreflightProbe{
				{resource: "ok-1", read: func(context.Context) error { return nil }},
				{resource: "fails", read: func(context.Context) error { return errBoom }},
				{resource: "ok-2", read: func(context.Context) error { return nil }},
			},
			expect: []error{nil, errBoom, nil},
		},
		{
			name: "a failing probe does not stop the others",
			probes: []ngrokAPIPreflightProbe{
				{resource: "fails", required: true, read: func(context.Context) error { return errBoom }},
				{resource: "ok", read: func(context.Context) error { return nil }},
			},
			expect: []error{errBoom, nil},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := runNgrokAPIPreflightProbes(t.Context(), tc.probes)
			assert.Equal(t, tc.expect, errs)
		})
	}
}

// TestRunNgrokAPIPreflightProbesRunsConcurrently deadlocks if the probes are
// run in sequence: every probe waits for all of them to have started.
func TestRunNgrokAPIPreflightProbesRunsConcurrently(t *testing.T) {
	const probeCount = 4

	var started sync.WaitGroup
	started.Add(probeCount)

	probes := make([]ngrokAPIPreflightProbe, probeCount)
	for i := range probes {
		probes[i] = ngrokAPIPreflightProbe{
			resource: "probe",
			read: func(context.Context) error {
				started.Done()
				started.Wait()
				return nil
			},
		}
	}

	done := make(chan []error, 1)
	go func() {
		done <- runNgrokAPIPreflightProbes(t.Context(), probes)
	}()

	select {
	case errs := <-done:
		assert.Equal(t, make([]error, probeCount), errs)
	case <-time.After(10 * time.Second):
		t.Fatal("probes did not all start before the first one finished; they are not running concurrently")
	}
}

func TestPreflightNgrokAPIAccess(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		// setListErrors configures which resources fail to list.
		setListErrors func(*nmockapi.Clientset)
		expectErr     bool
	}{
		{
			name:          "every read succeeds",
			setListErrors: func(*nmockapi.Clientset) {},
		},
		{
			name: "a required read failing is fatal",
			setListErrors: func(cs *nmockapi.Clientset) {
				cs.Endpoints().(*nmockapi.EndpointsClient).SetListError(errBoom)
			},
			expectErr: true,
		},
		{
			name: "an optional read failing is not fatal",
			setListErrors: func(cs *nmockapi.Clientset) {
				cs.Domains().(*nmockapi.DomainClient).SetListError(errBoom)
				cs.TCPAddresses().(*nmockapi.TCPAddressesClient).SetListError(errBoom)
				cs.IPPolicyRules().(*nmockapi.IPPolicyRuleClient).SetListError(errBoom)
			},
		},
		{
			name: "every read failing is fatal",
			setListErrors: func(cs *nmockapi.Clientset) {
				cs.Endpoints().(*nmockapi.EndpointsClient).SetListError(errBoom)
				cs.Domains().(*nmockapi.DomainClient).SetListError(errBoom)
				cs.TCPAddresses().(*nmockapi.TCPAddressesClient).SetListError(errBoom)
				cs.IPPolicyRules().(*nmockapi.IPPolicyRuleClient).SetListError(errBoom)
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := nmockapi.NewClientset()
			tc.setListErrors(cs)

			err := preflightNgrokAPIAccess(t.Context(), cs)

			if !tc.expectErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			// The upgrade guide tells operators to look for this exact prefix.
			assert.Contains(t, err.Error(), "unable to verify ngrok access token")
			assert.ErrorIs(t, err, errBoom)
		})
	}
}
