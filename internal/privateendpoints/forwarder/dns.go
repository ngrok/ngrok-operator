package forwarder

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

type ExchangeFunc func(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error)

// UDPExchange sends m to addr over UDP.
func UDPExchange(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error) {
	c := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	resp, _, err := c.ExchangeContext(ctx, m, addr)
	return resp, err
}

// DNSHandler is authoritative for known private endpoint names. Unknown
// ngrok.direct names are NXDOMAIN; unknown .internal names go upstream so
// provider names like metadata.google.internal keep resolving.
type DNSHandler struct {
	Table    *Table
	Upstream string
	TTL      uint32
	Exchange ExchangeFunc
}

func (h *DNSHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = w.WriteMsg(h.Answer(ctx, r))
}

func (h *DNSHandler) Answer(ctx context.Context, r *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	if len(r.Question) != 1 {
		m.Rcode = dns.RcodeFormatError
		return m
	}
	if !h.Table.Synced() {
		m.Rcode = dns.RcodeServerFailure
		return m
	}
	q := r.Question[0]
	name := pe.NormalizeHost(q.Name)

	if ip, ok := h.Table.IP(name); ok {
		m.Authoritative = true
		hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: h.TTL}
		parsed := net.ParseIP(ip)
		switch {
		case q.Qtype == dns.TypeA && parsed.To4() != nil:
			hdr.Rrtype = dns.TypeA
			m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: parsed.To4()})
		case q.Qtype == dns.TypeAAAA && parsed.To4() == nil:
			hdr.Rrtype = dns.TypeAAAA
			m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: parsed})
		}
		return m
	}

	switch {
	case inZone(name, "ngrok.direct"):
		m.Authoritative = true
		m.Rcode = dns.RcodeNameError
	case inZone(name, "internal"):
		resp, err := h.Exchange(ctx, r, h.Upstream)
		if err != nil || resp == nil {
			m.Rcode = dns.RcodeServerFailure
			return m
		}
		resp.Id = r.Id
		return resp
	default:
		m.Rcode = dns.RcodeRefused
	}
	return m
}

func inZone(name, zone string) bool {
	return name == zone || strings.HasSuffix(name, "."+zone)
}
