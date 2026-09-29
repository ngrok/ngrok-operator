package forwarder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := NewTable()
	assert.False(t, tbl.Synced())

	tbl.Replace([]Entry{
		{Hostname: "foo.internal", Port: 80, ClusterIP: "10.0.0.1"},
		{Hostname: "foo.internal", Port: 443, ClusterIP: "10.0.0.1"},
		{Hostname: "bar.internal", Port: 6379, ClusterIP: "10.0.0.2", ForwarderPort: 20000},
		{Hostname: "pending.internal", Port: 80},
	})
	assert.True(t, tbl.Synced())

	tests := []struct {
		name   string
		lookup func() (string, bool)
		want   string
		ok     bool
	}{
		{"ip known", func() (string, bool) { return tbl.IP("foo.internal") }, "10.0.0.1", true},
		{"ip case and trailing dot", func() (string, bool) { return tbl.IP("FOO.Internal.") }, "10.0.0.1", true},
		{"ip pending skipped", func() (string, bool) { return tbl.IP("pending.internal") }, "", false},
		{"shared http", func() (string, bool) { return tbl.SharedTarget("foo.internal", 80) }, "foo.internal:80", true},
		{"shared https", func() (string, bool) { return tbl.SharedTarget("foo.internal", 443) }, "foo.internal:443", true},
		{"shared not for dedicated host", func() (string, bool) { return tbl.SharedTarget("bar.internal", 6379) }, "", false},
		{"port target", func() (string, bool) { return tbl.PortTarget(20000) }, "bar.internal:6379", true},
		{"port unknown", func() (string, bool) { return tbl.PortTarget(20001) }, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.lookup()
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	tbl.Replace(nil)
	_, ok := tbl.IP("foo.internal")
	assert.False(t, ok, "Replace drops entries not in the new set")
	assert.True(t, tbl.Synced())
}
