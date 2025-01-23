package bindings

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_BoundEndpointPoller_filterBoundEndpointActions(t *testing.T) {
	t.Parallel()

	examplePoller := BoundEndpointPoller{
		Log: logr.Discard(),
	}

	// some example BoundEndpoints we can use for test cases
	uriExample1 := "http://service1.namespace1:8080"
	epdExample1 := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample1),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample1,
		},
	}

	uriExample2 := "https://service2.namespace2:443"
	epdExample2 := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample2),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample2,
		},
	}

	uriExample3 := "https://service3.namespace3:443"
	epdExample3 := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample3),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample3,
		},
	}

	epdExample4 := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			// Name does not match example3 on puprose
			// to test if re-names trigger delete/create rather than update
			Name: "abcd1234-abcd-1234-abcd-1234abcd1234",
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample3, // example 3 on purpose, see Name
		},
	}

	tests := []struct {
		name       string
		existing   []bindingsv1alpha1.BoundEndpoint
		desired    ngrokapi.AggregatedEndpoints
		wantCreate []bindingsv1alpha1.BoundEndpoint
		wantUpdate []bindingsv1alpha1.BoundEndpoint
		wantDelete []bindingsv1alpha1.BoundEndpoint
	}{
		{
			name:       "empty existing and desired",
			existing:   []bindingsv1alpha1.BoundEndpoint{},
			desired:    ngrokapi.AggregatedEndpoints{},
			wantCreate: []bindingsv1alpha1.BoundEndpoint{},
			wantUpdate: []bindingsv1alpha1.BoundEndpoint{},
			wantDelete: []bindingsv1alpha1.BoundEndpoint{},
		},
		{
			name:     "empty existing; create desired",
			existing: []bindingsv1alpha1.BoundEndpoint{},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample1: epdExample1,
				uriExample2: epdExample2,
			},
			wantCreate: []bindingsv1alpha1.BoundEndpoint{
				epdExample1,
				epdExample2,
			},
			wantUpdate: []bindingsv1alpha1.BoundEndpoint{},
			wantDelete: []bindingsv1alpha1.BoundEndpoint{},
		},
		{
			name: "update boundendpoints",
			existing: []bindingsv1alpha1.BoundEndpoint{
				epdExample1,
				epdExample2,
			},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample1: epdExample1,
				uriExample2: epdExample2,
			},
			wantCreate: []bindingsv1alpha1.BoundEndpoint{},
			wantUpdate: []bindingsv1alpha1.BoundEndpoint{
				epdExample1,
				epdExample2,
			},
			wantDelete: []bindingsv1alpha1.BoundEndpoint{},
		},
		{
			name: "create, delete, and update",
			existing: []bindingsv1alpha1.BoundEndpoint{
				epdExample1,
				epdExample2,
				// epdExample3 is missing, toCreate
			},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample1: epdExample1,
				// epdExample2 is missing, toDelete
				uriExample3: epdExample3,
			},
			wantCreate: []bindingsv1alpha1.BoundEndpoint{
				epdExample3,
			},
			wantUpdate: []bindingsv1alpha1.BoundEndpoint{
				epdExample1,
			},
			wantDelete: []bindingsv1alpha1.BoundEndpoint{
				epdExample2,
			},
		},
		{
			name: "delete/create, rather than update",
			existing: []bindingsv1alpha1.BoundEndpoint{
				epdExample4,
			},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample3: epdExample3, // example4 on purpose
			},
			wantCreate: []bindingsv1alpha1.BoundEndpoint{
				epdExample3,
			},
			wantUpdate: []bindingsv1alpha1.BoundEndpoint{},
			wantDelete: []bindingsv1alpha1.BoundEndpoint{
				epdExample4,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)

			gotCreate, gotUpdate, gotDelete := examplePoller.filterBoundEndpointActions(context.TODO(), test.existing, test.desired)

			assert.ElementsMatch(test.wantCreate, gotCreate)
			assert.ElementsMatch(test.wantUpdate, gotUpdate)
			assert.ElementsMatch(test.wantDelete, gotDelete)
		})
	}
}

func Test_BoundEndpointPoller_boundEndpointNeedsUpdate(t *testing.T) {
	t.Parallel()

	// some example BoundEndpoints we can use for test cases
	uriExample1 := "http://service1.namespace1:8080"
	epdExample1 := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample1),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample1,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace1",
				Service:   "service1",
				Port:      8080,
				Protocol:  "TCP",
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: hashURI(uriExample1),
			Endpoints: []bindingsv1alpha1.BindingEndpoint{
				{
					Ref:          ngrok.Ref{ID: "ep_abc123", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
			},
		},
	}

	epdExample1NewMetadata := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample1),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample1,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace1",
				Service:   "service1",
				Port:      8080,
				Protocol:  "TCP",
				Metadata: bindingsv1alpha1.TargetMetadata{
					Annotations: map[string]string{
						"key": "value",
					},
					Labels: map[string]string{
						"key": "value",
					},
				},
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: hashURI(uriExample1),
			Endpoints: []bindingsv1alpha1.BindingEndpoint{
				{
					Ref:          ngrok.Ref{ID: "ep_abc123", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
			},
		},
	}

	epdExample1EmptyMetadata := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample1),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample1,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace1",
				Service:   "service1",
				Port:      8080,
				Protocol:  "TCP",
				Metadata: bindingsv1alpha1.TargetMetadata{
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: hashURI(uriExample1),
			Endpoints: []bindingsv1alpha1.BindingEndpoint{
				{
					Ref:          ngrok.Ref{ID: "ep_abc123", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
			},
		},
	}

	uriExample2 := "https://service2.namespace2:443"
	epdExample2 := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample2),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample2,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace2",
				Service:   "service2",
				Port:      443,
				Protocol:  "TCP",
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: hashURI(uriExample2),
			Endpoints: []bindingsv1alpha1.BindingEndpoint{
				{
					Ref:          ngrok.Ref{ID: "ep_def456", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
			},
		},
	}

	epdExample2NewStatus := bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample2),
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: uriExample2,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace2",
				Service:   "service2",
				Port:      443,
				Protocol:  "TCP",
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: hashURI(uriExample2),
			Endpoints: []bindingsv1alpha1.BindingEndpoint{
				{
					Ref:          ngrok.Ref{ID: "ep_def456", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
				{
					Ref:          ngrok.Ref{ID: "ep_xyz999", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
				{
					Ref:          ngrok.Ref{ID: "ep_www000", URI: "example-uri"},
					Status:       bindingsv1alpha1.StatusProvisioning,
					ErrorCode:    "",
					ErrorMessage: "",
				},
			},
		},
	}

	tests := []struct {
		name     string
		existing bindingsv1alpha1.BoundEndpoint
		desired  bindingsv1alpha1.BoundEndpoint
		want     bool
	}{
		{
			name:     "no change",
			existing: epdExample1,
			desired:  epdExample1,
			want:     false,
		},
		{
			name:     "different objects",
			existing: epdExample1,
			desired:  epdExample2,
			want:     true,
		},
		{
			name:     "different statuses",
			existing: epdExample2,
			desired:  epdExample2NewStatus,
			want:     true,
		},
		{
			name:     "different metadata",
			existing: epdExample1,
			desired:  epdExample1NewMetadata,
			want:     true,
		},
		{
			name:     "different empty/nil metadata",
			existing: epdExample1,
			desired:  epdExample1EmptyMetadata,
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)

			got := boundEndpointNeedsUpdate(context.TODO(), test.existing, test.desired)
			assert.Equal(test.want, got)
		})
	}
}

func Test_BoundEndpointPoller_hashURI(t *testing.T) {
	assert := assert.New(t)

	endpointURI := "http://service.namespace:8080"

	// hash must be consistent
	for i := 0; i < 100; i++ {
		hashed := hashURI(endpointURI)

		// ensure hashed name meets k8s DNS naming requirements
		assert.True(len(hashed) <= 63)
		assert.Regexp("^[a-z]([-a-z0-9]*[a-z0-9])?$", hashed)
	}
}

func Test_BoundEndpointPoller_targetMetadataIsEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    bindingsv1alpha1.TargetMetadata
		b    bindingsv1alpha1.TargetMetadata
		want bool
	}{
		{
			name: "both unset",
			a:    bindingsv1alpha1.TargetMetadata{},
			b:    bindingsv1alpha1.TargetMetadata{},
			want: true,
		},
		{
			name: "different annotations",
			a:    bindingsv1alpha1.TargetMetadata{},
			b: bindingsv1alpha1.TargetMetadata{
				Annotations: map[string]string{
					"key": "value",
				},
			},
			want: false,
		},
		{
			name: "different labels",
			a:    bindingsv1alpha1.TargetMetadata{},
			b: bindingsv1alpha1.TargetMetadata{
				Labels: map[string]string{
					"key": "value",
				},
			},
			want: false,
		},
		{
			name: "changed annotations",
			a: bindingsv1alpha1.TargetMetadata{
				Annotations: map[string]string{
					"key": "value",
				},
			},
			b: bindingsv1alpha1.TargetMetadata{
				Annotations: map[string]string{
					"key": "changed",
				},
			},
			want: false,
		},
		{
			name: "changed labels",
			a: bindingsv1alpha1.TargetMetadata{
				Labels: map[string]string{
					"key": "value",
				},
			},
			b: bindingsv1alpha1.TargetMetadata{
				Labels: map[string]string{
					"key": "changed",
				},
			},
			want: false,
		},
		{
			name: "annotations: nil vs. empty",
			a: bindingsv1alpha1.TargetMetadata{
				Annotations: nil,
			},
			b: bindingsv1alpha1.TargetMetadata{
				Annotations: map[string]string{},
			},
			want: true,
		},
		{
			name: "labels: nil vs. empty",
			a: bindingsv1alpha1.TargetMetadata{
				Labels: nil,
			},
			b: bindingsv1alpha1.TargetMetadata{
				Labels: map[string]string{},
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)

			got := targetMetadataIsEqual(test.a, test.b)
			assert.Equal(test.want, got)
		})
	}
}
