package bindings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EndpointBindingPoller struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder

	// Channel to stop the API polling goroutine
	stopCh chan struct{}
}

// Start implements the manager.Runnable interface.
func (r *EndpointBindingPoller) Start(ctx context.Context) error {
	r.Log.Info("Starting the BindingConfiguration polling routine")
	r.stopCh = make(chan struct{})
	defer close(r.stopCh)
	go r.startPollingAPI(ctx)
	<-ctx.Done()
	r.Log.Info("Stopping the BindingConfiguration polling routine")
	return nil
}

// startPollingAPI polls a mock API every 10 seconds and updates the BindingConfiguration's status.
// TODO: Make the 10 seconds configurable via a helm option so clients can tune to their needs based on their api rate limits.
func (r *EndpointBindingPoller) startPollingAPI(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.Log.Info("Polling API for endpoint bindings")
			if err := r.reconcileEndpointBindingsFromAPI(ctx); err != nil {
				r.Log.Error(err, "Failed to update endpoint bindings from API")
			}
		case <-r.stopCh:
			r.Log.Info("Stopping API polling")
			return
		}
	}
}

func (r *EndpointBindingPoller) reconcileEndpointBindingsFromAPI(ctx context.Context) error {
	// Fetch the mock endpoint data from the API.
	endpoints, err := fetchEndpoints()
	if err != nil {
		return err
	}

	// Create a map to track desired endpoints by hashed name.
	desiredEndpoints := make(map[string]*EndpointBinding)
	for _, apiEndpoint := range endpoints.EndpointBindings {
		// NOTE: It was mentioned that the endpoint ID may change and we should use the URL as the unique identifier.
		// Hashing it for now to make it easy
		hashedName := hashURL(apiEndpoint.URL)
		desiredEndpoints[hashedName] = &apiEndpoint
	}

	// Get all current EndpointBinding resources in the cluster.
	var endpointBindings bindingsv1alpha1.EndpointBindingList
	if err := r.List(ctx, &endpointBindings); err != nil {
		return err
	}

	// Loop through each desired endpoint.
	for hashedName, apiEndpoint := range desiredEndpoints {
		urlBits, err := parseURLBits(apiEndpoint.URL)
		if err != nil {
			r.Log.Error(err, "Failed to parse URL", "url", apiEndpoint.URL)
			continue
		}

		// Find the corresponding existing binding.
		var existingBinding *bindingsv1alpha1.EndpointBinding
		for i := range endpointBindings.Items {
			if endpointBindings.Items[i].Name == hashedName {
				existingBinding = &endpointBindings.Items[i]
				break
			}
		}

		// If it doesn’t exist, create a new CRD.
		if existingBinding == nil {
			if err := r.createBinding(ctx, hashedName, apiEndpoint, urlBits); err != nil {
				r.Log.Error(err, "Failed to create EndpointBinding", "name", hashedName)
			}
		} else {
			// If it does exist, update it if necessary.
			if shouldUpdateBinding(existingBinding, apiEndpoint, urlBits) {
				if err := r.updateBinding(ctx, existingBinding, apiEndpoint, urlBits); err != nil {
					r.Log.Error(err, "Failed to update EndpointBinding", "name", hashedName)
				}
			}
		}
	}

	// Loop through all current bindings and delete those that are not in the desired list.
	for _, binding := range endpointBindings.Items {
		if _, exists := desiredEndpoints[binding.Name]; !exists {
			if err := r.Delete(ctx, &binding); err != nil {
				r.Log.Error(err, "Failed to delete stale EndpointBinding", "name", binding.Name)
			} else {
				r.Log.Info("Deleted stale EndpointBinding", "name", binding.Name)
			}
		}
	}

	return nil
}

func (r *EndpointBindingPoller) createBinding(ctx context.Context, hashedName string, apiEndpoint *EndpointBinding, urlBits *URLBits) error {
	binding := &bindingsv1alpha1.EndpointBinding{
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			Port:   urlBits.Port, // TODO: This is probably wrong and should be # assigned by operator to target the ngrok-operator-forwarder container
			Scheme: urlBits.Scheme,
			Target: bindingsv1alpha1.EndpointTarget{
				Protocol:  "TCP", // Only support tcp for now, scheme controls how ngrok handles the endpoint
				Namespace: urlBits.Namespace,
				Service:   urlBits.ServiceName,
				Port:      urlBits.Port,
			},
		},
		Status: bindingsv1alpha1.EndpointBindingStatus{
			HashedName: hashedName, // TODO: This exists in the code already, but the spec didn't mention the CR's name. I'm just using this for the name field
		},
	}
	binding.SetName(hashedName)
	binding.SetNamespace(urlBits.Namespace)

	r.Log.Info("Creating new EndpointBinding", "name", hashedName)
	if err := r.Create(ctx, binding); err != nil {
		return err
	}

	r.Recorder.Event(binding, "Normal", "Created", "EndpointBinding created successfully")
	return nil
}

func (r *EndpointBindingPoller) updateBinding(ctx context.Context, binding *bindingsv1alpha1.EndpointBinding, apiEndpoint *EndpointBinding, urlBits *URLBits) error {
	binding.Spec.Port = urlBits.Port
	binding.Spec.Scheme = urlBits.Scheme
	binding.Spec.Target.Namespace = urlBits.Namespace
	binding.Spec.Target.Service = urlBits.ServiceName
	binding.Spec.Target.Port = urlBits.Port

	// Commenting out for now as they aren't used and trying to set status requires the Status.Status to be set
	// binding.Status.ID = apiEndpoint.ID // This changes apparently, so might not be worth storing at all
	// binding.Status.HashedName = hashURL(apiEndpoint.URL)

	r.Log.Info("Updating EndpointBinding", "name", binding.Name)
	if err := r.Update(ctx, binding); err != nil {
		return err
	}

	// if err := r.Status().Update(ctx, binding); err != nil {
	// 	return err
	// }

	r.Recorder.Event(binding, "Normal", "Updated", "EndpointBinding updated successfully")
	return nil
}

func shouldUpdateBinding(binding *bindingsv1alpha1.EndpointBinding, apiEndpoint *EndpointBinding, urlBits *URLBits) bool {
	// Check if any of the relevant fields differ.
	return binding.Spec.Port != urlBits.Port ||
		binding.Spec.Scheme != urlBits.Scheme ||
		binding.Spec.Target.Namespace != urlBits.Namespace ||
		binding.Spec.Target.Service != urlBits.ServiceName ||
		binding.Status.HashedName != hashURL(apiEndpoint.URL)
}

func hashURL(urlString string) string {
	hash := sha256.Sum256([]byte(urlString))
	return hex.EncodeToString(hash[:])
}

func parseURLBits(urlStr string) (*URLBits, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	// Extract the service name and namespace from the URL's host part.
	// Format is expected as [<scheme>://]<service-name>.<namespace-name>
	parts := strings.Split(parsedURL.Hostname(), ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid URL format, expected <service-name>.<namespace-name>: %s", urlStr)
	}

	// Parse the port if available, defaulting to 80 or 443 based on the scheme.
	var port int32
	if parsedURL.Port() != "" {
		parsedPort, err := strconv.Atoi(parsedURL.Port())
		if err != nil {
			return nil, fmt.Errorf("invalid port value: %s", parsedURL.Port())
		}
		port = int32(parsedPort)
	} else {
		if parsedURL.Scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}

	return &URLBits{
		Scheme:      parsedURL.Scheme,
		ServiceName: parts[0],
		Namespace:   parts[1],
		Port:        port,
	}, nil
}

// fetchEndpoints mocks an API call to retrieve a list of endpoint bindings.
func fetchEndpoints() (*APIResponse, error) {
	// Mock response with sample data.
	return &APIResponse{
		EndpointBindings: []EndpointBinding{
			{
				ID:  "abc123",
				URL: "https://service1.namespace1",
			},
			{
				ID:  "def456",
				URL: "http://service2.namespace2",
			},
			{
				ID:  "3k45jl",
				URL: "tls://service-tls.namespace2",
			},
			{
				ID:  "asd9f9",
				URL: "tcp://service-tcp.namespace2",
			},
		},
	}, nil
}

type URLBits struct {
	Scheme      string
	ServiceName string
	Namespace   string
	Port        int32
}

// APIResponse represents a mock response from the API.
type APIResponse struct {
	EndpointBindings []EndpointBinding `json:"binding_endpoints"`
}

// EndpointBinding represents a binding endpoint object.
type EndpointBinding struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}
