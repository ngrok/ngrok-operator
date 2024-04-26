package ngrokapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"text/template"

	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tls_edge_backend"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tls_edge_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tls_edge_mutual_tls"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tls_edge_policy"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tls_edge_tls_termination"
)

type RawTLSModuleSetPolicy json.RawMessage
type EdgeRawTLSPolicyReplace struct {
	ID     string                `json:"id,omitempty"`
	Module RawTLSModuleSetPolicy `json:"module,omitempty"`
}

type RawTLSEdgePolicyClient interface {
	Delete(context.Context, string) error
	Replace(context.Context, EdgeRawTLSPolicyReplace) (*RawTLSModuleSetPolicy, error)
}
type rawTLSPolicyClient struct {
	base   *ngrok.BaseClient
	policy *tls_edge_policy.Client
}

func newRawTLSPolicyClient(config *ngrok.ClientConfig) *rawTLSPolicyClient {
	return &rawTLSPolicyClient{
		base:   ngrok.NewBaseClient(config),
		policy: tls_edge_policy.NewClient(config),
	}
}
func (c *rawTLSPolicyClient) Delete(ctx context.Context, id string) error {
	return c.policy.Delete(ctx, id)
}

func (c *rawTLSPolicyClient) Replace(ctx context.Context, policy *EdgeRawTLSPolicyReplace) (*RawTLSModuleSetPolicy, error) {
	if policy == nil {
		return nil, errors.New("tls edge policy replace cannot be nil")
	}
	var path bytes.Buffer
	if err := template.Must(template.New("replace_path").Parse("/edges/tls/{{ .ID }}/policy")).Execute(&path, policy); err != nil {
		// api client panics on error also
		panic(err)
	}
	var res RawTLSModuleSetPolicy
	apiURL := &url.URL{Path: path.String()}

	if err := c.base.Do(ctx, "PUT", apiURL, policy.Module, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type TLSEdgeModulesClientset interface {
	Backend() *tls_edge_backend.Client
	IPRestriction() *tls_edge_ip_restriction.Client
	MutualTLS() *tls_edge_mutual_tls.Client
	TLSTermination() *tls_edge_tls_termination.Client
	Policy() *tls_edge_policy.Client
	RawPolicy() *rawTLSPolicyClient
}

type defaultTLSEdgeModulesClientset struct {
	backend        *tls_edge_backend.Client
	ipRestriction  *tls_edge_ip_restriction.Client
	mutualTLS      *tls_edge_mutual_tls.Client
	tlsTermination *tls_edge_tls_termination.Client
	policy         *tls_edge_policy.Client
	newPolicy      *rawTLSPolicyClient
}

func newTLSEdgeModulesClientset(config *ngrok.ClientConfig) *defaultTLSEdgeModulesClientset {
	return &defaultTLSEdgeModulesClientset{
		backend:        tls_edge_backend.NewClient(config),
		ipRestriction:  tls_edge_ip_restriction.NewClient(config),
		mutualTLS:      tls_edge_mutual_tls.NewClient(config),
		tlsTermination: tls_edge_tls_termination.NewClient(config),
		policy:         tls_edge_policy.NewClient(config),
		newPolicy:      newRawTLSPolicyClient(config),
	}
}

func (c *defaultTLSEdgeModulesClientset) Backend() *tls_edge_backend.Client {
	return c.backend
}

func (c *defaultTLSEdgeModulesClientset) IPRestriction() *tls_edge_ip_restriction.Client {
	return c.ipRestriction
}

func (c *defaultTLSEdgeModulesClientset) MutualTLS() *tls_edge_mutual_tls.Client {
	return c.mutualTLS
}

func (c *defaultTLSEdgeModulesClientset) TLSTermination() *tls_edge_tls_termination.Client {
	return c.tlsTermination
}

func (c *defaultTLSEdgeModulesClientset) Policy() *tls_edge_policy.Client {
	return c.policy
}

func (c *defaultTLSEdgeModulesClientset) RawPolicy() *rawTLSPolicyClient {
	return c.newPolicy
}
