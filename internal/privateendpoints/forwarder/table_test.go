package forwarder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := NewTable()
	assert.False(t, tbl.Synced())

	dupes := tbl.Replace([]Entry{
		{Hostname: "FOO.ngrok.direct", Port: 80, ForwarderPort: 20000},
		{Hostname: "bar.ngrok.direct", Port: 6379, ForwarderPort: 20001},
		{Hostname: "a.ngrok.direct", Port: 5432, ForwarderPort: 20003},
		{Hostname: "b.ngrok.direct", Port: 5432, ForwarderPort: 20003},
		{Hostname: "c.ngrok.direct", Port: 5432, ForwarderPort: 20003},
	})
	assert.Equal(t, []int32{20003}, dupes, "a port claimed twice is reported once")
	assert.True(t, tbl.Synced())

	tests := []struct {
		name string
		port int32
		want string
		ok   bool
	}{
		{"http endpoint", 20000, "foo.ngrok.direct:80", true},
		{"tcp endpoint", 20001, "bar.ngrok.direct:6379", true},
		{"unknown port", 20002, "", false},
		{"ambiguous port resolves to nothing", 20003, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tbl.PortTarget(tt.port)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	assert.Empty(t, tbl.Replace(nil))
	_, ok := tbl.PortTarget(20000)
	assert.False(t, ok, "Replace drops entries not in the new set")
	assert.True(t, tbl.Synced())
}
