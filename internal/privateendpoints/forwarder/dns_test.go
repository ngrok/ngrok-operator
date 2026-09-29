package forwarder

import (
	"context"
	"errors"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDNSAnswer(t *testing.T) {
	synced := NewTable()
	synced.Replace([]Entry{
		{Hostname: "foo.internal", Port: 80, ClusterIP: "10.0.0.1"},
		{Hostname: "foo.ngrok.direct", Port: 443, ClusterIP: "10.0.0.1"},
		{Hostname: "bar.internal", Port: 6379, ClusterIP: "10.0.0.2", ForwarderPort: 20000},
	})

	upstreamOK := func(_ context.Context, m *dns.Msg, _ string) (*dns.Msg, error) {
		r := new(dns.Msg)
		r.SetReply(m)
		r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: []byte{169, 254, 169, 254}}}
		r.Id = 9999 // handler must restore the client's id
		return r, nil
	}
	upstreamFail := func(context.Context, *dns.Msg, string) (*dns.Msg, error) { return nil, errors.New("timeout") }

	tests := []struct {
		name     string
		table    *Table
		exchange ExchangeFunc
		qname    string
		qtype    uint16
		rcode    int
		wantA    string
	}{
		{"not synced", NewTable(), upstreamOK, "foo.internal.", dns.TypeA, dns.RcodeServerFailure, ""},
		{"known http host", synced, upstreamOK, "foo.internal.", dns.TypeA, dns.RcodeSuccess, "10.0.0.1"},
		{"known tcp host", synced, upstreamOK, "bar.internal.", dns.TypeA, dns.RcodeSuccess, "10.0.0.2"},
		{"known ngrok.direct", synced, upstreamOK, "foo.ngrok.direct.", dns.TypeA, dns.RcodeSuccess, "10.0.0.1"},
		{"mixed case", synced, upstreamOK, "FOO.Internal.", dns.TypeA, dns.RcodeSuccess, "10.0.0.1"},
		{"known host AAAA is empty", synced, upstreamOK, "foo.internal.", dns.TypeAAAA, dns.RcodeSuccess, ""},
		{"unknown ngrok.direct NXDOMAIN", synced, upstreamOK, "nope.ngrok.direct.", dns.TypeA, dns.RcodeNameError, ""},
		{"unknown internal forwarded", synced, upstreamOK, "metadata.google.internal.", dns.TypeA, dns.RcodeSuccess, "169.254.169.254"},
		{"unknown internal upstream fails", synced, upstreamFail, "metadata.google.internal.", dns.TypeA, dns.RcodeServerFailure, ""},
		{"other zone refused", synced, upstreamOK, "example.com.", dns.TypeA, dns.RcodeRefused, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &DNSHandler{Table: tt.table, Upstream: "192.0.2.53:53", TTL: 5, Exchange: tt.exchange}
			q := new(dns.Msg)
			q.SetQuestion(tt.qname, tt.qtype)
			q.Id = 42

			resp := h.Answer(context.Background(), q)
			require.NotNil(t, resp)
			assert.Equal(t, uint16(42), resp.Id)
			assert.Equal(t, tt.rcode, resp.Rcode, dns.RcodeToString[resp.Rcode])
			if tt.wantA == "" {
				assert.Empty(t, resp.Answer)
				return
			}
			require.Len(t, resp.Answer, 1)
			a, ok := resp.Answer[0].(*dns.A)
			require.True(t, ok)
			assert.Equal(t, tt.wantA, a.A.String())
			if tt.table == synced && tt.wantA != "169.254.169.254" {
				assert.Equal(t, uint32(5), a.Hdr.Ttl)
				assert.True(t, resp.Authoritative)
			}
		})
	}
}
