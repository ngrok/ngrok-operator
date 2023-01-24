package ngrokapi

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/stretchr/testify/assert"
)

func TestDefaultClientsetImplementsInterface(t *testing.T) {
	cs := &DefaultClientset{}
	assert.Implements(t, (*Clientset)(nil), cs)
}

func ExampleClientset() {
	// Create a ngrok client config
	config := ngrok.NewClientConfig("YOUR_API_KEY")
	// Create a clientset using the provided ngrok client configuration.
	cs := NewClientSet(config)
	// Access a client for the domains API.
	cs.Domains()
	// Access a client for TCP Edge modules
	cs.EdgeModules().TCP()
	// Access a client for HTTPS Edge Route Modules
	cs.EdgeModules().HTTPS().Routes().Compression()
}
