package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEndpointForwarderMap(t *testing.T) {
	m := newEndpointForwarderMap()

	ep, ok := m.Get("test")
	assert.False(t, ok)
	assert.Nil(t, ep)

	m.Add("test", nil)
	ep, ok = m.Get("test")
	assert.True(t, ok)
	assert.Nil(t, ep)

	m.Delete("test")
	ep, ok = m.Get("test")
	assert.False(t, ok)
	assert.Nil(t, ep)
}
