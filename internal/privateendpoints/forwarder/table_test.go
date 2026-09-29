package forwarder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := NewTable()
	assert.False(t, tbl.Synced())

	tbl.Replace([]Entry{
		{Hostname: "FOO.internal", Port: 80, ForwarderPort: 20000},
		{Hostname: "bar.internal", Port: 6379, ForwarderPort: 20001},
	})
	assert.True(t, tbl.Synced())

	tests := []struct {
		name string
		port int32
		want string
		ok   bool
	}{
		{"http endpoint", 20000, "foo.internal:80", true},
		{"tcp endpoint", 20001, "bar.internal:6379", true},
		{"unknown port", 20002, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tbl.PortTarget(tt.port)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	tbl.Replace(nil)
	_, ok := tbl.PortTarget(20000)
	assert.False(t, ok, "Replace drops entries not in the new set")
	assert.True(t, tbl.Synced())
}
