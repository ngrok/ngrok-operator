package mocks

// Note: Generate the mock files with names like mock_*.go. This is so that
// the generated files are picked up by the .gitattributes file.

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination mock_conn.go net Conn

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination mock_tunnel.go golang.ngrok.com/ngrok Tunnel

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination mock_dialer.go github.com/ngrok/ngrok-operator/pkg/tunneldriver Dialer
