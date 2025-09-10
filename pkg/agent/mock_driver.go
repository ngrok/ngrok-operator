package agent

import (
	"context"
	"crypto/tls"
	"net/http"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// MockAgentDriver is a mock implementation of Driver for testing purposes
type MockAgentDriver struct {
	createResults map[string]*EndpointResult // keyed by endpoint name
	createErrors  map[string]error           // keyed by endpoint name
	deleteErrors  map[string]error           // keyed by endpoint name

	// Default fallback values
	DefaultResult *EndpointResult
	DefaultError  error
	DeleteError   error

	// Call tracking
	CreateCalls []CreateCall
	DeleteCalls []DeleteCall
}

// CreateCall tracks parameters passed to CreateAgentEndpoint
type CreateCall struct {
	Name          string
	Spec          ngrokv1alpha1.AgentEndpointSpec
	TrafficPolicy string
	ClientCerts   []tls.Certificate
}

// DeleteCall tracks parameters passed to DeleteAgentEndpoint
type DeleteCall struct {
	Name string
}

// NewMockAgentDriver creates a new mock agent driver
func NewMockAgentDriver() *MockAgentDriver {
	return &MockAgentDriver{
		createResults: make(map[string]*EndpointResult),
		createErrors:  make(map[string]error),
		deleteErrors:  make(map[string]error),
	}
}

// SetEndpointResult configures the mock to return specific results for an endpoint
func (m *MockAgentDriver) SetEndpointResult(name string, result *EndpointResult) {
	m.createResults[name] = result
}

// SetEndpointError configures the mock to return specific errors for an endpoint
func (m *MockAgentDriver) SetEndpointError(name string, err error) {
	m.createErrors[name] = err
}

// Reset clears all configured results and errors
func (m *MockAgentDriver) Reset() {
	m.createResults = make(map[string]*EndpointResult)
	m.createErrors = make(map[string]error)
	m.deleteErrors = make(map[string]error)
	m.DefaultResult = nil
	m.DefaultError = nil
	m.DeleteError = nil
	m.CreateCalls = nil
	m.DeleteCalls = nil
}

// CreateAgentEndpoint implements Driver interface
func (m *MockAgentDriver) CreateAgentEndpoint(_ context.Context, name string, spec ngrokv1alpha1.AgentEndpointSpec, trafficPolicy string, clientCerts []tls.Certificate) (*EndpointResult, error) {
	// Track the call
	m.CreateCalls = append(m.CreateCalls, CreateCall{
		Name:          name,
		Spec:          spec,
		TrafficPolicy: trafficPolicy,
		ClientCerts:   clientCerts,
	})

	// Check for specific result for this endpoint name
	if result, ok := m.createResults[name]; ok {
		return result, nil
	}
	if err, ok := m.createErrors[name]; ok {
		return nil, err
	}

	// Return default values
	return m.DefaultResult, m.DefaultError
}

// DeleteAgentEndpoint implements Driver interface
func (m *MockAgentDriver) DeleteAgentEndpoint(_ context.Context, name string) error {
	// Track the call
	m.DeleteCalls = append(m.DeleteCalls, DeleteCall{
		Name: name,
	})

	if err, ok := m.deleteErrors[name]; ok {
		return err
	}
	return m.DeleteError
}

// Ready implements healthcheck.HealthChecker interface
func (m *MockAgentDriver) Ready(_ context.Context, _ *http.Request) error {
	return nil
}

// Alive implements healthcheck.HealthChecker interface
func (m *MockAgentDriver) Alive(_ context.Context, _ *http.Request) error {
	return nil
}
