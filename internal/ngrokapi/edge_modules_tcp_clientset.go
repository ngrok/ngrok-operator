package ngrokapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"text/template"

	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tcp_edge_backend"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tcp_edge_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tcp_edge_policy"
)

type EdgeRawTCPPolicyReplace struct {
	ID     string          `json:"id,omitempty"`
	Module json.RawMessage `json:"module,omitempty"`
}

type RawTCPEdgePolicyClient interface {
	Delete(context.Context, string) error
	Replace(context.Context, EdgeRawTCPPolicyReplace) (*json.RawMessage, error)
}
type rawTCPPolicyClient struct {
	base   *ngrok.BaseClient
	policy *tcp_edge_policy.Client
}

func newRawTCPPolicyClient(config *ngrok.ClientConfig) *rawTCPPolicyClient {
	return &rawTCPPolicyClient{
		base:   ngrok.NewBaseClient(config),
		policy: tcp_edge_policy.NewClient(config),
	}
}

func (c *rawTCPPolicyClient) Delete(ctx context.Context, id string) error {
	return c.policy.Delete(ctx, id)
}

func (c *rawTCPPolicyClient) Replace(ctx context.Context, policy *EdgeRawTCPPolicyReplace) (*json.RawMessage, error) {
	if policy == nil {
		return nil, errors.New("tcp edge policy replace cannot be nil")
	}
	var path bytes.Buffer
	if err := template.Must(template.New("replace_path").Parse("/edges/tcp/{{ .ID }}/policy")).Execute(&path, policy); err != nil {
		// api client panics on error also
		panic(err)
	}
	var res json.RawMessage
	apiURL := &url.URL{Path: path.String()}

	if err := c.base.Do(ctx, "PUT", apiURL, policy.Module, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type TCPEdgeModulesClientset interface {
	Backend() *tcp_edge_backend.Client
	IPRestriction() *tcp_edge_ip_restriction.Client
	Policy() *tcp_edge_policy.Client
	RawPolicy() *rawTCPPolicyClient
}

type defaultTCPEdgeModulesClientset struct {
	backend       *tcp_edge_backend.Client
	ipRestriction *tcp_edge_ip_restriction.Client
	policy        *tcp_edge_policy.Client
	rawPolicy     *rawTCPPolicyClient
}

func newTCPEdgeModulesClientset(config *ngrok.ClientConfig) *defaultTCPEdgeModulesClientset {
	return &defaultTCPEdgeModulesClientset{
		backend:       tcp_edge_backend.NewClient(config),
		ipRestriction: tcp_edge_ip_restriction.NewClient(config),
		policy:        tcp_edge_policy.NewClient(config),
		rawPolicy:     newRawTCPPolicyClient(config),
	}
}

func (c *defaultTCPEdgeModulesClientset) Backend() *tcp_edge_backend.Client {
	return c.backend
}

func (c *defaultTCPEdgeModulesClientset) IPRestriction() *tcp_edge_ip_restriction.Client {
	return c.ipRestriction
}

func (c *defaultTCPEdgeModulesClientset) Policy() *tcp_edge_policy.Client {
	return c.policy
}

func (c *defaultTCPEdgeModulesClientset) RawPolicy() *rawTCPPolicyClient {
	return c.rawPolicy
}
