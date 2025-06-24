package ngrokapi

import (
	"context"

	"github.com/ngrok/ngrok-api-go/v7"
)

type EdgeModulesClientset interface {
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
	https *defaultHTTPSEdgeModulesClientset
	tls   *defaultTLSEdgeModulesClientset
}

func newEdgeModulesClientset(config *ngrok.ClientConfig) *defaultEdgeModulesClientset {
	return &defaultEdgeModulesClientset{
		https: newHTTPSEdgeModulesClientset(config),
		tls:   newTLSEdgeModulesClientset(config),
	}
}

func (c *defaultEdgeModulesClientset) HTTPS() HTTPSEdgeModulesClientset {
	return c.https
}

func (c *defaultEdgeModulesClientset) TLS() TLSEdgeModulesClientset {
	return c.tls
}
