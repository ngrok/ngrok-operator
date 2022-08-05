package agentapiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	myJson, err := json.Marshal(t)
	if err != nil {
		return err
	}

	fmt.Printf("Creating tunnel with body of %s\n", myJson)
	resp, err := ac.client.Post(tunnelApiUrl, "application/json", bytes.NewBuffer(myJson))
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	fmt.Printf("Body : %s", body)
	return nil
}

func (ac agentApiClient) DeleteTunnel(_ context.Context, name string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/tunnels/%s", tunnelApiUrl, name), nil)
	resp, err := ac.client.Do(req)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	fmt.Printf("Body : %s", body)
	return nil
}

type TunnelsApiBody struct {
	Addr      string `json:"addr"`
	Proto     string `json:"proto"`
	Name      string `json:"name"`
	SubDomain string `json:"subdomain,omitempty"`
	// Labels    []string `json:"labels"`
}
