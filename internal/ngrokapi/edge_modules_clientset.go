package ngrokapi

import (
	"context"

	"github.com/ngrok/ngrok-api-go/v7"
)

type EdgeModulesClientset interface {
	TCP() TCPEdgeModulesClientset
	HTTPS() HTTPSEdgeModulesClientset
	TLS() TLSEdgeModulesClientset
}

type edgeModulesClient[R, T any] interface {
	Deletor
	Replace(context.Context, R) (T, error)
}

type EdgeRouteModulesDeletor interface {
	Delete(context.Context, *ngrok.EdgeRouteItem) error
}

type EdgeRouteModulesReplacer[R, T any] interface {
	Replace(context.Context, R) (T, error)
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
