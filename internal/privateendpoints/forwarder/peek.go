package forwarder

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// PeekBufferSize fits a full TLS record (16KiB + header) or an HTTP head.
const PeekBufferSize = 17 << 10

// PeekHTTPHost returns the Host of the HTTP/1.x request at the head of br
// without consuming any bytes.
func PeekHTTPHost(br *bufio.Reader) (string, error) {
	n := 1
	for {
		b, err := br.Peek(n)
		if i := bytes.Index(b, []byte("\r\n\r\n")); i >= 0 {
			req, perr := http.ReadRequest(bufio.NewReader(bytes.NewReader(b[:i+4])))
			if perr != nil {
				return "", fmt.Errorf("parsing http request head: %w", perr)
			}
			host := req.Host
			if h, _, serr := net.SplitHostPort(host); serr == nil {
				host = h
			}
			if host == "" {
				return "", errors.New("http request has no Host")
			}
			return strings.ToLower(host), nil
		}
		if err != nil {
			return "", fmt.Errorf("reading http request head: %w", err)
		}
		if n = br.Buffered(); n == len(b) {
			n++
		}
	}
}

var errStopHandshake = errors.New("stop after ClientHello")

// PeekSNI returns the SNI of the TLS ClientHello at the head of br without
// consuming any bytes. It never completes a handshake.
func PeekSNI(br *bufio.Reader) (string, error) {
	hdr, err := br.Peek(5)
	if err != nil {
		return "", fmt.Errorf("reading tls record header: %w", err)
	}
	if hdr[0] != 0x16 {
		return "", errors.New("not a tls handshake")
	}
	rec, err := br.Peek(5 + int(binary.BigEndian.Uint16(hdr[3:5])))
	if err != nil {
		return "", fmt.Errorf("reading tls ClientHello: %w", err)
	}
	var sni string
	herr := tls.Server(readOnlyConn{r: bytes.NewReader(rec)}, &tls.Config{
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			sni = h.ServerName
			return nil, errStopHandshake
		},
	}).Handshake()
	if sni == "" {
		return "", fmt.Errorf("no SNI in ClientHello: %w", herr)
	}
	return strings.ToLower(sni), nil
}

// readOnlyConn feeds recorded bytes to crypto/tls and discards its writes.
type readOnlyConn struct {
	r io.Reader
}

func (c readOnlyConn) Read(p []byte) (int, error)     { return c.r.Read(p) }
func (readOnlyConn) Write(p []byte) (int, error)      { return len(p), nil }
func (readOnlyConn) Close() error                     { return nil }
func (readOnlyConn) LocalAddr() net.Addr              { return nil }
func (readOnlyConn) RemoteAddr() net.Addr             { return nil }
func (readOnlyConn) SetDeadline(time.Time) error      { return nil }
func (readOnlyConn) SetReadDeadline(time.Time) error  { return nil }
func (readOnlyConn) SetWriteDeadline(time.Time) error { return nil }
