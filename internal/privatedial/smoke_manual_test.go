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

package privatedial

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// TestManualLiveDial is a manual, opt-in smoke test against the real
// private-dial gateway. It is skipped unless PRIVATEDIAL_MANUAL_TEST_PAT is
// set, so it never runs in CI. Used during POC development to validate this
// package end-to-end against real .internal endpoints; not a substitute for
// the CI-safe unit tests, of which there deliberately are none here since
// the wire protocol can't be exercised without a live gateway.
func TestManualLiveDial(t *testing.T) {
	pat := os.Getenv("PRIVATEDIAL_MANUAL_TEST_PAT")
	if pat == "" {
		t.Skip("set PRIVATEDIAL_MANUAL_TEST_PAT to run this manual live-gateway test")
	}
	target := os.Getenv("PRIVATEDIAL_MANUAL_TEST_TARGET") // e.g. "bar.internal:6379"
	if target == "" {
		t.Skip("set PRIVATEDIAL_MANUAL_TEST_TARGET (host:port of a live .internal endpoint)")
	}

	dialer := NewDialer(pat, "h2.connect-endpoint.ngrok.com:443")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		t.Fatalf("DialContext(%s): %v", target, err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "PING\r\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("reply: %q", buf[:n])
}
