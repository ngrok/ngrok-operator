package agentapiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const tunnelAPIURL = "http://localhost:4040/api/tunnels"

type AgentAPIClient interface {
	CreateTunnel(ctx context.Context, t TunnelsAPIBody) error
	DeleteTunnel(ctx context.Context, name string) error
}

type agentAPIClient struct {
	client *http.Client
}

func NewAgentApiClient() AgentAPIClient {
	return agentAPIClient{
		client: &http.Client{Timeout: 1 * time.Second},
	}
}

func (ac agentAPIClient) CreateTunnel(ctx context.Context, t TunnelsAPIBody) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tunnelAPIURL+"/"+t.Name, nil)
	if err != nil {
		return err
	}
	resp, err := ac.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 404 {
		return nil
	}

	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, tunnelAPIURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	_, err = ac.client.Do(req)
	return err
}

func (ac agentAPIClient) DeleteTunnel(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/%s", tunnelAPIURL, name), nil)
	if err != nil {
		return err
	}
	_, err = ac.client.Do(req)
	return err
}

type TunnelsAPIBody struct {
	Addr      string   `json:"addr"`
	Name      string   `json:"name"`
	SubDomain string   `json:"subdomain,omitempty"`
	Labels    []string `json:"labels"`
}
