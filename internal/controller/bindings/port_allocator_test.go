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
	assert.True(pb.IsSet(5))

	// Test SetAny
	port, err := pb.SetAny()
	assert.Nil(err)
	assert.True(pb.IsSet(port))

	// Test Unset
	pb.Unset(port)
	assert.False(pb.IsSet(port))

	// Test IsSet
	assert.True(pb.IsSet(5))
	assert.False(pb.IsSet(6))

	// Test Set duplicate
	err = pb.Set(5) // already set
	assert.Error(err)
	assert.True(pb.IsSet(5))
}
