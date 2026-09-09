/*
MIT License

Copyright (c) 2024 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package privatedial is a minimal client for ngrok's private-dial protocol:
// authenticate with a PAT (ngrok_pat_*) and open a raw TCP stream to a
// private (.internal) endpoint over a single HTTP/2 connection to the
// connect ingress.
//
// POC NOTE: this is deliberately not a dependency on
// golang.ngrok.com/ngrok/v2's private-dial support (ngrok-go PR #245,
// unmerged as of this writing). That branch's frame codec assumes a fixed
// 2-byte little-endian length prefix, but the gateway actually speaks
// protobuf-style base-128 varint-delimited framing (confirmed by probing
// the real gateway during this POC) — the mismatch makes every read hang
// forever in io.ReadFull. The gateway also sends one leading frame on the
// data stream after a successful /dial (observed to contain the resolved
// endpoint ID) that isn't documented in that branch's proto and that a
// naive raw passthrough would otherwise splice into the caller's byte
// stream. This package hand-encodes the tiny DialReq{host,port} message
// (two scalar fields — not worth a protobuf dependency) and skips that
// leading frame, and was validated end-to-end against a real .internal
// endpoint (HTTP and raw-TCP/redis) before being wired into the forwarder.
//
// Session bootstrap (POST /session) is skipped entirely: DialReq embeds
// enough to authenticate per connection via the Authorization header
// alone, which the gateway accepts standalone (also confirmed live). Fine
// for this POC's per-connection dial; a persistent session (for ping/drain
// signaling) is a K8SOP-261 throughput concern, not a POC blocker.
package privatedial

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/http2"
)

// Dialer opens raw TCP connections to ngrok private (.internal) endpoints
// via the private-dial gateway. It is safe for concurrent use.
type Dialer struct {
	authToken string
	transport *http2.Transport
	dialURL   string
}

// NewDialer returns a Dialer authenticating with authToken (a PAT,
// ngrok_pat_*) against the private-dial gateway at serverAddr
// ("host:port", e.g. "h2.connect-endpoint.ngrok.com:443").
func NewDialer(authToken, serverAddr string) *Dialer {
	return &Dialer{
		authToken: authToken,
		transport: &http2.Transport{
			AllowHTTP: false,
		},
		dialURL: (&url.URL{Scheme: "https", Host: serverAddr, Path: "/dial"}).String(),
	}
}

// DialContext opens a stream to address ("host:port") within the caller's
// account and returns it as a raw net.Conn. network is expected to be
// "tcp" and is otherwise ignored (private dial has no other transport).
func (d *Dialer) DialContext(ctx context.Context, _, address string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("private-dial: invalid address %q: %w", address, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("private-dial: invalid port %q: %w", portStr, err)
	}

	reqReader, reqWriter := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.dialURL, reqReader)
	if err != nil {
		_ = reqWriter.Close()
		return nil, fmt.Errorf("private-dial: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.authToken)
	req.Header.Set("Content-Type", "application/octet-stream")

	go func() {
		if err := writeVarintFrame(reqWriter, encodeDialReq(host, port)); err != nil {
			_ = reqWriter.CloseWithError(err)
		}
	}()

	resp, err := d.transport.RoundTrip(req)
	if err != nil {
		_ = reqWriter.Close()
		return nil, fmt.Errorf("private-dial: /dial request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		_ = reqWriter.Close()
		return nil, fmt.Errorf("private-dial: /dial to %s status %d (code=%s): %s",
			address, resp.StatusCode, resp.Header.Get("Ngrok-Error-Code"), body)
	}

	// The gateway sends one leading varint-framed message on the data
	// stream before raw bytes begin (the resolved endpoint ID); discard it
	// so it doesn't get spliced into the relayed byte stream.
	if _, err := skipVarintFrame(resp.Body); err != nil {
		_ = resp.Body.Close()
		_ = reqWriter.Close()
		return nil, fmt.Errorf("private-dial: reading dial-ack frame: %w", err)
	}

	return &conn{w: reqWriter, r: resp.Body, target: address}, nil
}

// conn adapts the /dial request/response body pair to a net.Conn. Write
// pushes into the HTTP/2 request body; Read pulls from the response body.
type conn struct {
	w      *io.PipeWriter
	r      io.ReadCloser
	target string
}

func (c *conn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *conn) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c *conn) Close() error {
	_ = c.w.Close()
	return c.r.Close()
}
func (c *conn) LocalAddr() net.Addr                { return privateDialAddr{} }
func (c *conn) RemoteAddr() net.Addr               { return privateDialAddr{addr: c.target} }
func (c *conn) SetDeadline(_ time.Time) error      { return nil }
func (c *conn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *conn) SetWriteDeadline(_ time.Time) error { return nil }

type privateDialAddr struct{ addr string }

func (privateDialAddr) Network() string  { return "privatedial" }
func (a privateDialAddr) String() string { return a.addr }
