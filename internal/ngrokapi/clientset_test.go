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
	// Use the clientset to access the various clients.
	cs.Domains()
}
