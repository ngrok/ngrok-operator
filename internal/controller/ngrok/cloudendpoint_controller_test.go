package ngrok

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
		Client:   fakeClient,
		Recorder: record.NewFakeRecorder(10),
	}

	// Call the function under test
	policy, err := r.findTrafficPolicyByName(t.Context(), "policy-1", "default")

	// Assert that the correct policy is found
	assert.NoError(t, err)
	assert.Equal(t, `{"type":"allow"}`, policy)

	// Test case where TrafficPolicy is not found
	policy, err = r.findTrafficPolicyByName(t.Context(), "nonexistent-policy", "default")
	assert.Error(t, err)
	assert.Equal(t, "", policy)
}

func Test_ensureDomainExists(t *testing.T) {
	// Set up a fake client with a sample Domain
	scheme := runtime.NewScheme()
	_ = ingressv1alpha1.AddToScheme(scheme)

	existingNotReadyDomain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
	}
	existingReadyDomain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example2-com",
			Namespace: "default",
		},
		Status: ingressv1alpha1.DomainStatus{
			ID: "rd_123",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingNotReadyDomain, existingReadyDomain).
		Build()

	r := &CloudEndpointReconciler{
		Client:   fakeClient,
		Log:      logr.Discard(),
		Recorder: record.NewFakeRecorder(10),
	}

	// Case 1: Domain already exists, but is not ready
	clep := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-endpoint-1",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: "https://example.com",
		},
	}

	domain, err := r.ensureDomainExists(t.Context(), clep)
	assert.Equal(t, ErrDomainCreating, err)
	assert.Equal(t, existingNotReadyDomain, domain)

	// Case 2: Domain already exists, but is not ready
	clep = &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-endpoint-2",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: "https://example2.com",
		},
	}

	domain, err = r.ensureDomainExists(t.Context(), clep)
	assert.NoError(t, err)
	assert.Equal(t, existingReadyDomain, domain)

	// Case 3: Domain does not exist and should be created
	clep = &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-endpoint-2",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: "https://newdomain.com",
		},
	}

	domain, err = r.ensureDomainExists(t.Context(), clep)
	assert.Equal(t, ErrDomainCreating, err)
	assert.Empty(t, domain.Status.ID)
}
