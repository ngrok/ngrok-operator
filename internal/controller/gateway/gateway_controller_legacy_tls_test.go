/*
MIT License

Copyright (c) 2022 ngrok, Inc.

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

package gateway

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// legacyTLSFakeRecorder is a minimal fake satisfying events.EventRecorder
// (the type of GatewayReconciler.Recorder) for unit-testing
// warnIfLegacyTLSOptions without booting the envtest suite.
type legacyTLSFakeRecorder struct {
	events int
}

func (f *legacyTLSFakeRecorder) Eventf(_, _ runtime.Object, _, _, _, _ string, _ ...any) {
	f.events++
}

func TestWarnIfLegacyTLSOptions(t *testing.T) {
	testCases := []struct {
		name       string
		listeners  []gatewayv1.Listener
		wantEvents int
	}{
		{
			name: "canonical-only TLS options emits no event",
			listeners: []gatewayv1.Listener{
				{
					TLS: &gatewayv1.ListenerTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"ngrok.com/terminate-tls.min_version": "1.3",
						},
					},
				},
			},
			wantEvents: 0,
		},
		{
			name: "legacy-only across multiple listeners emits exactly one event",
			listeners: []gatewayv1.Listener{
				{
					TLS: &gatewayv1.ListenerTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"k8s.ngrok.com/terminate-tls.min_version": "1.2",
						},
					},
				},
				{
					TLS: &gatewayv1.ListenerTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"k8s.ngrok.com/terminate-tls.mutual_tls_crt": "abc",
						},
					},
				},
			},
			wantEvents: 1,
		},
		{
			name: "listener with TLS nil does not panic and emits no event",
			listeners: []gatewayv1.Listener{
				{TLS: nil},
			},
			wantEvents: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &legacyTLSFakeRecorder{}
			r := &GatewayReconciler{Recorder: rec}
			gw := &gatewayv1.Gateway{Spec: gatewayv1.GatewaySpec{Listeners: tc.listeners}}

			r.warnIfLegacyTLSOptions(logr.Discard(), gw)

			assert.Equal(t, tc.wantEvents, rec.events)
		})
	}
}

func TestWarnIfLegacyTLSOptionsNilRecorderDoesNotPanic(t *testing.T) {
	r := &GatewayReconciler{Recorder: nil}
	gw := &gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{
					TLS: &gatewayv1.ListenerTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"k8s.ngrok.com/terminate-tls.min_version": "1.2",
						},
					},
				},
			},
		},
	}

	assert.NotPanics(t, func() {
		r.warnIfLegacyTLSOptions(logr.Discard(), gw)
	})
}
