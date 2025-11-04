package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelTerminationPair(t *testing.T) {

	terminating, terminator := NewChannelTerminationPair()

	assert.False(t, terminating.IsTerminating(), "Should not be terminating initially")
	assert.False(t, terminating.IsTerminating(), "Should still not be terminating")

	terminator.StartTermination()

	assert.True(t, terminating.IsTerminating(), "Should be terminating after signal")
	assert.True(t, terminating.IsTerminating(), "Should remain terminating after multiple signals")

	// Calling StartTermination again should have no effect
	terminator.StartTermination()
	assert.True(t, terminating.IsTerminating(), "Should still be terminating after multiple StartTermination calls")
}

func TestNoOpTerminating(t *testing.T) {
	var nt Terminating
	nt = NewNoOpTerminating()

	assert.False(t, nt.IsTerminating(), "NoopTerminating should always return false")
	assert.False(t, nt.IsTerminating(), "NoopTerminating should always return false on multiple calls")
}
