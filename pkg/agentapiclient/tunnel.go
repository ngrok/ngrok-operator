package agentapiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const tunnelApiUrl = "http://localhost:4040/api/tunnels"

type AgentApiClient interface {
	CreateTunnel(ctx context.Context, t TunnelsApiBody) error
	DeleteTunnel(ctx context.Context, name string) error
}

type agentApiClient struct {
	client *http.Client
}

func NewAgentApiClient() AgentApiClient {
	return &agentApiClient{
		client: &http.Client{Timeout: time.Duration(1) * time.Second},
	}
}

func (ac agentApiClient) CreateTunnel(_ context.Context, t TunnelsApiBody) error {
	resp, err := ac.client.Get(tunnelApiUrl + "/" + t.Name)
	if err != nil {
		return err
	}
	if resp.Status != "404 Not Found" {
		// we found the tunnel so skip
		return nil
	}

	myJson, err := json.Marshal(t)
	if err != nil {
		return err
	}

	_, err = ac.client.Post(tunnelApiUrl, "application/json", bytes.NewBuffer(myJson))
	return err
}

func (ac agentApiClient) DeleteTunnel(_ context.Context, name string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", tunnelApiUrl, name), nil)
	if err != nil {
		return err
	}
	_, err = ac.client.Do(req)
	return err
}

type TunnelsApiBody struct {
	Addr      string   `json:"addr"`
	Name      string   `json:"name"`
	SubDomain string   `json:"subdomain,omitempty"`
	Labels    []string `json:"labels"`
}
