package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddErrorToNewInvalidIngressSpec(t *testing.T) {
	err := NewErrInvalidIngressSpec()
	err.AddError("error1")
	err.AddError("error2")

	assert.True(t, err.HasErrors())
	assert.Len(t, err.errors, 2)
}
