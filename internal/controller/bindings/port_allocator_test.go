package bindings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_portBitmap(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	// Test newPortBitmap
	pb := newPortBitmap(0, 10)
	assert.Equal(pb.start, uint16(0))
	assert.Equal(pb.NumFree(), uint64(10))

	// Test Set
	err := pb.Set(5)
	assert.Nil(err)
	isSet, err := pb.IsSet(5)
	assert.Nil(err)
	assert.True(isSet)

	// Test SetAny
	port, err := pb.SetAny()
	assert.Nil(err)
	isSet, err = pb.IsSet(port)
	assert.Nil(err)
	assert.True(isSet)

	// Test Unset
	err = pb.Unset(port)
	assert.Nil(err)
	isSet, err = pb.IsSet(port)
	assert.Nil(err)
	assert.False(isSet)

	// Test IsSet
	isSet, err = pb.IsSet(5)
	assert.Nil(err)
	assert.True(isSet)
	isSet, err = pb.IsSet(6)
	assert.Nil(err)
	assert.False(isSet)

	// Test Set duplicate
	err = pb.Set(5) // already set
	assert.Error(err)
	isSet, err = pb.IsSet(5)
	assert.Nil(err)
	assert.True(isSet)
}

func Test_portBitmap_outOfRange(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	pb := newPortBitmap(10, 20)

	// Set with port below range returns error instead of panicking
	err := pb.Set(5)
	assert.Error(err)
	assert.Contains(err.Error(), "before start of port range")

	// IsSet with port below range returns error instead of panicking
	_, err = pb.IsSet(5)
	assert.Error(err)
	assert.Contains(err.Error(), "before start of port range")

	// Unset with port below range returns error instead of panicking
	err = pb.Unset(5)
	assert.Error(err)
	assert.Contains(err.Error(), "before start of port range")
}
