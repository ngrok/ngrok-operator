package ngrokapi

import "github.com/ngrok/ngrok-api-go/v6"

type EdgeModulesClientset interface {
	TCP() TCPEdgeModulesClientset
	HTTPS() HTTPSEdgeModulesClientset
	TLS() TLSEdgeModulesClientset
}

type defaultEdgeModulesClientset struct {
	tcp   *defaultTCPEdgeModulesClientset
	https *defaultHTTPSEdgeModulesClientset
	tls   *defaultTLSEdgeModulesClientset
}

func newEdgeModulesClientset(config *ngrok.ClientConfig) *defaultEdgeModulesClientset {
	return &defaultEdgeModulesClientset{
		tcp:   newTCPEdgeModulesClientset(config),
		https: newHTTPSEdgeModulesClientset(config),
		tls:   newTLSEdgeModulesClientset(config),
	}
}

func (c *defaultEdgeModulesClientset) TCP() TCPEdgeModulesClientset {
	return c.tcp
}

func (c *defaultEdgeModulesClientset) HTTPS() HTTPSEdgeModulesClientset {
	return c.https
}

func (c *defaultEdgeModulesClientset) TLS() TLSEdgeModulesClientset {
	return c.tls
}
