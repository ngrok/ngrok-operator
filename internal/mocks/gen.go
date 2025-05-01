package mocks

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination conn.go net Conn

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination tunnel.go golang.ngrok.com/ngrok Tunnel

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination dialer.go github.com/ngrok/ngrok-operator/pkg/tunneldriver Dialer
