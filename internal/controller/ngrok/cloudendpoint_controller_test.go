package ngrok

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_extractDomain(t *testing.T) {
	r := &CloudEndpointReconciler{}

	tests := []struct {
		name     string
		inputURL string
		expected string
	}{
		{
			name:     "standard https URL",
			inputURL: "https://example.com",
			expected: "example.com",
		},
		{
			name:     "URL with port",
			inputURL: "https://example.com:8080",
			expected: "example.com",
		},
		{
			name:     "URL with path",
			inputURL: "https://example.com/path",
			expected: "example.com",
		},
		{
			name:     "tcp URL",
			inputURL: "tcp://example.com:443",
			expected: "example.com",
		},
		{
			name:     "invalid URL",
			inputURL: "http:/example.com",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a CloudEndpoint with the input URL
			clep := &ngrokv1alpha1.CloudEndpoint{
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: tt.inputURL,
				},
			}
			result := r.extractDomain(clep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_findTrafficPolicy(t *testing.T) {
	// Set up a fake client with a sample TrafficPolicy
	scheme := runtime.NewScheme()
	_ = ngrokv1alpha1.AddToScheme(scheme)

	rawPolicy := json.RawMessage(`{"type": "allow"}`)

	trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: rawPolicy,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(trafficPolicy).
		Build()

	r := &CloudEndpointReconciler{
		Client: fakeClient,
	}

	// Call the function under test
	policy, err := r.findTrafficPolicyByName(context.Background(), "policy-1", "default")

	// Assert that the correct policy is found
	assert.NoError(t, err)
	assert.Equal(t, `{"type":"allow"}`, policy)

	// Test case where TrafficPolicy is not found
	policy, err = r.findTrafficPolicyByName(context.Background(), "nonexistent-policy", "default")
	assert.Error(t, err)
	assert.Equal(t, "", policy)
}

func Test_ensureDomainExists(t *testing.T) {
	// Set up a fake client with a sample Domain
	scheme := runtime.NewScheme()
	_ = ingressv1alpha1.AddToScheme(scheme)

	existingDomain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example.com",
			Namespace: "default",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingDomain).
		Build()

	r := &CloudEndpointReconciler{
		Client: fakeClient,
		Log:    logr.Discard(),
	}

	// Case 1: Domain already exists
	clep := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-endpoint-1",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: "https://example.com",
		},
	}

	err := r.ensureDomainExists(context.Background(), clep)
	assert.NoError(t, err)

	// Case 2: Domain does not exist and should be created
	clep = &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-endpoint-2",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: "https://newdomain.com",
		},
	}

	err = r.ensureDomainExists(context.Background(), clep)
	assert.Equal(t, ErrDomainCreating, err)

	// Verify that the new domain was created
	newDomain := &ingressv1alpha1.Domain{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "newdomain.com", Namespace: "default"}, newDomain)
	assert.NoError(t, err)
	assert.Equal(t, "newdomain.com", newDomain.Spec.Domain)
}
