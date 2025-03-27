package bindings

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestGetIngressEndpointWithFallback(t *testing.T) {
	cases := []struct {
		input            string
		expectedEndpoint string
		shouldErr        bool
	}{
		{
			"",
			"",
			true,
		},
		{
			"foo.example.com",
			"foo.example.com:443",
			false,
		},
		{
			"foo.example.com:443",
			"foo.example.com:443",
			false,
		},
		{
			"foo.example.com:443:1234",
			"",
			true,
		},
	}

	for _, c := range cases {
		ingressEndpoint, err := getIngressEndpointWithFallback(c.input, logr.Discard())
		assert.Equal(t, c.expectedEndpoint, ingressEndpoint)
		if c.shouldErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}
