package forwarder

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardConnJoinsBothWays(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() { _ = ForwardConn(context.Background(), echoDial, server, "foo.internal:80", time.Second) }()

	assert.Equal(t, "[foo.internal:80]", readN(t, client, len("[foo.internal:80]")))
	_, err := io.WriteString(client, "ping")
	require.NoError(t, err)
	assert.Equal(t, "ping", readN(t, client, 4))
}

func TestForwardConnDialFailureClosesClient(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	failDial := func(context.Context, string) (net.Conn, error) { return nil, errors.New("endpoint offline") }

	err := ForwardConn(context.Background(), failDial, server, "gone.internal:80", time.Second)
	require.ErrorContains(t, err, "endpoint offline")
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	_, err = client.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

// A client that never closes must not pin the private dial stream forever
// once the endpoint has finished sending.
func TestForwardConnDrainsAfterUpstreamEOF(t *testing.T) {
	upstreamL := listen(t)
	defer upstreamL.Close()
	upstreamSawClose := make(chan struct{})
	go func() {
		c, err := upstreamL.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.WriteString(c, "bye")
		_ = c.(*net.TCPConn).CloseWrite()
		_, _ = io.Copy(io.Discard, c) // returns once the forwarder closes its side
		close(upstreamSawClose)
	}()
	dial := func(context.Context, string) (net.Conn, error) { return net.Dial("tcp", upstreamL.Addr().String()) }

	clientL := listen(t)
	defer clientL.Close()
	go func() {
		server, err := clientL.Accept()
		if err == nil {
			_ = ForwardConn(context.Background(), dial, server, "done.internal:80", 200*time.Millisecond)
		}
	}()
	client, err := net.Dial("tcp", clientL.Addr().String())
	require.NoError(t, err)
	defer client.Close() // stays open and silent for the whole test
	assert.Equal(t, "bye", readN(t, client, 3))

	select {
	case <-upstreamSawClose:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream connection still open long after the drain timeout")
	}
}

// TestForwardConnOverTCP runs the forwarder between a real TCP client and a
// real TCP upstream. ForwardConn returning proves both copy goroutines exited.
func TestForwardConnOverTCP(t *testing.T) {
	const drain = 50 * time.Millisecond
	tests := []struct {
		name string
		// upstream serves the dialed conn and returns what it observed.
		upstream func(c *net.TCPConn) error
		client   func(t *testing.T, c *net.TCPConn)
		// wantForwardErr: ForwardConn reports a copy error.
		wantForwardErr  bool
		wantUpstreamErr bool
	}{
		{
			name: "bidirectional exchange",
			upstream: func(c *net.TCPConn) error {
				_, err := io.Copy(c, c)
				_ = c.CloseWrite()
				return err
			},
			client: func(t *testing.T, c *net.TCPConn) {
				for _, msg := range []string{"ping", "pong"} {
					_, err := io.WriteString(c, msg)
					require.NoError(t, err)
					assert.Equal(t, msg, readN(t, c, len(msg)))
				}
				require.NoError(t, c.CloseWrite())
				rest, err := io.ReadAll(c)
				require.NoError(t, err)
				assert.Empty(t, rest)
			},
		},
		{
			// R21: a client half-close must not start the drain.
			name: "client half-close then reply slower than drain",
			upstream: func(c *net.TCPConn) error {
				req, err := io.ReadAll(c)
				if err != nil {
					return err
				}
				time.Sleep(6 * drain)
				_, err = c.Write(append([]byte("reply to "), req...))
				_ = c.CloseWrite()
				return err
			},
			client: func(t *testing.T, c *net.TCPConn) {
				_, err := io.WriteString(c, "req")
				require.NoError(t, err)
				require.NoError(t, c.CloseWrite())
				resp, err := io.ReadAll(c)
				require.NoError(t, err)
				assert.Equal(t, "reply to req", string(resp))
			},
		},
		{
			// R27: truncation must not look like an orderly close.
			name: "upstream reset mid-response",
			upstream: func(c *net.TCPConn) error {
				if _, err := io.WriteString(c, "partial"); err != nil {
					return err
				}
				time.Sleep(drain) // let "partial" reach the client first
				_ = c.SetLinger(0)
				return nil
			},
			client: func(t *testing.T, c *net.TCPConn) {
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, err := io.ReadAll(c)
				assert.ErrorIs(t, err, syscall.ECONNRESET)
			},
			wantForwardErr: true,
		},
		{
			name: "client reset mid-request",
			upstream: func(c *net.TCPConn) error {
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, err := io.ReadAll(c)
				return err
			},
			client: func(t *testing.T, c *net.TCPConn) {
				_, err := io.WriteString(c, "partial")
				require.NoError(t, err)
				time.Sleep(drain)
				require.NoError(t, c.SetLinger(0))
			},
			wantForwardErr:  true,
			wantUpstreamErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstreamL := listen(t)
			defer upstreamL.Close()
			upstreamErr := make(chan error, 1)
			go func() {
				c, err := upstreamL.Accept()
				if err != nil {
					upstreamErr <- err
					return
				}
				defer c.Close()
				upstreamErr <- tt.upstream(c.(*net.TCPConn))
			}()
			dial := func(context.Context, string) (net.Conn, error) { return net.Dial("tcp", upstreamL.Addr().String()) }

			clientL := listen(t)
			defer clientL.Close()
			forwardErr := make(chan error, 1)
			go func() {
				server, err := clientL.Accept()
				if err != nil {
					forwardErr <- err
					return
				}
				forwardErr <- ForwardConn(context.Background(), dial, server, "x.internal:80", drain)
			}()

			c, err := net.Dial("tcp", clientL.Addr().String())
			require.NoError(t, err)
			tt.client(t, c.(*net.TCPConn))
			_ = c.Close()

			select {
			case err := <-forwardErr:
				if tt.wantForwardErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("ForwardConn did not return")
			}
			select {
			case err := <-upstreamErr:
				if tt.wantUpstreamErr {
					assert.ErrorIs(t, err, syscall.ECONNRESET)
				} else {
					assert.NoError(t, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("upstream conn still open")
			}
		})
	}
}
