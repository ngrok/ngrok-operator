package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	assert.Empty(t, cfg.Log, "log defaults belong to zap")

	assert.Equal(t, "The official ngrok Kubernetes Operator.", cfg.Ngrok.Description)
	assert.Equal(t, "trusted", cfg.Ngrok.RootCAs)
	assert.Equal(t, "svc.cluster.local", cfg.Ngrok.ClusterDomain)
	assert.Empty(t, cfg.Ngrok.Region)
	assert.Empty(t, cfg.Ngrok.ServerAddr)
	assert.Empty(t, cfg.Ngrok.APIURL)

	assert.True(t, cfg.Features.Ingress.Enabled)
	assert.Equal(t, "k8s.ngrok.com/ingress-controller", cfg.Features.Ingress.ControllerName)
	assert.True(t, cfg.Features.Gateway.Enabled)
	assert.False(t, cfg.Features.Gateway.DisableReferenceGrants)
	assert.False(t, cfg.Features.Bindings.Enabled)
	assert.Equal(t, []string{"true"}, cfg.Features.Bindings.EndpointSelectors)
	assert.Equal(t, "kubernetes-binding-ingress.ngrok.io:443", cfg.Features.Bindings.IngressEndpoint)

	assert.Equal(t, "Delete", cfg.Features.DefaultDomainReclaimPolicy)
	assert.Equal(t, "Retain", cfg.Features.DrainPolicy)
	assert.False(t, cfg.APIManager.OneClickDemoMode)
}

func TestDefaultIsNotShared(t *testing.T) {
	a := Default()
	b := Default()

	a.Features.Bindings.EndpointSelectors[0] = "mutated"
	a.Ngrok.Metadata["mutated"] = "true"

	assert.Equal(t, []string{"true"}, b.Features.Bindings.EndpointSelectors)
	assert.Empty(t, b.Ngrok.Metadata)
}
