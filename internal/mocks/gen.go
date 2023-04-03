package mocks

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination conn.go net Conn

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination tunnel.go golang.ngrok.com/ngrok Tunnel

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination dialer.go github.com/ngrok/kubernetes-ingress-controller/pkg/tunneldriver Dialer
