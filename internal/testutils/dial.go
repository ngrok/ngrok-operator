package testutils

import (
	"context"
	"fmt"
	"io"
	"net"
)

// EchoDial stands in for a private dial: the "endpoint" writes back the
// target it was dialed with as "[target]", then echoes whatever it receives.
func EchoDial(_ context.Context, target string) (net.Conn, error) {
	a, b := net.Pipe()
	go func() {
		defer b.Close()
		_, _ = fmt.Fprintf(b, "[%s]", target)
		_, _ = io.Copy(b, b)
	}()
	return a, nil
}
