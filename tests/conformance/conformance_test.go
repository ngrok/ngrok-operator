// Package conformance runs the upstream Gateway API conformance suite against
// an ngrok-operator installed in the current kubeconfig context.
//
// It is its own Go module so it can pin the suite's Gateway API version
// independently of the operator's. See README.md for how to run it.
package conformance

import (
	"os"
	"testing"

	"sigs.k8s.io/gateway-api/conformance"
	confv1 "sigs.k8s.io/gateway-api/conformance/apis/v1"
	"sigs.k8s.io/gateway-api/conformance/utils/roundtripper"
)

func TestConformance(t *testing.T) {
	opts := conformance.DefaultOptions(t)

	suffix := os.Getenv("CONFORMANCE_HOSTNAME_SUFFIX")
	if suffix == "" {
		t.Fatal("CONFORMANCE_HOSTNAME_SUFFIX must be set to the operator's ngrok.features.gateway.conformanceHostnameSuffix")
	}
	opts.RoundTripper = mappingRoundTripper{
		inner:  &roundtripper.DefaultRoundTripper{Debug: opts.Debug, TimeoutConfig: opts.TimeoutConfig},
		mapper: hostMapper{suffix: suffix},
	}

	// Flags (--organization, --version, ...) win when given.
	if opts.Implementation.Organization == "" {
		opts.Implementation = confv1.Implementation{
			Organization: "ngrok",
			Project:      "ngrok-operator",
			URL:          "https://github.com/ngrok/ngrok-operator",
			Version:      os.Getenv("CONFORMANCE_OPERATOR_VERSION"),
			Contact:      []string{"https://github.com/ngrok/ngrok-operator/issues/new/choose"},
		}
	}

	conformance.RunConformanceWithOptions(t, opts)
}
