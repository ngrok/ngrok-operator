package bindings

import (
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_EndpointBindingPoller_filterEndpointBindingActions(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// some example EndpointBindings we can use for test cases
	uriExample1 := "http://service1.namespace1:8080"
	epdExample1 := bindingsv1alpha1.EndpointBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample1),
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			EndpointURI: uriExample1,
		},
	}

	uriExample2 := "https://service2.namespace2:443"
	epdExample2 := bindingsv1alpha1.EndpointBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample2),
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			EndpointURI: uriExample2,
		},
	}

	uriExample3 := "https://service3.namespace3:443"
	epdExample3 := bindingsv1alpha1.EndpointBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample3),
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			EndpointURI: uriExample3,
		},
	}

	tests := []struct {
		name       string
		existing   []bindingsv1alpha1.EndpointBinding
		desired    ngrokapi.AggregatedEndpoints
		wantCreate []bindingsv1alpha1.EndpointBinding
		wantUpdate []bindingsv1alpha1.EndpointBinding
		wantDelete []bindingsv1alpha1.EndpointBinding
	}{
		{
			name:       "empty existing and desired",
			existing:   []bindingsv1alpha1.EndpointBinding{},
			desired:    ngrokapi.AggregatedEndpoints{},
			wantCreate: []bindingsv1alpha1.EndpointBinding{},
			wantUpdate: []bindingsv1alpha1.EndpointBinding{},
			wantDelete: []bindingsv1alpha1.EndpointBinding{},
		},
		{
			name:     "empty existing; create desired",
			existing: []bindingsv1alpha1.EndpointBinding{},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample1: epdExample1,
				uriExample2: epdExample2,
			},
			wantCreate: []bindingsv1alpha1.EndpointBinding{
				epdExample1,
				epdExample2,
			},
			wantUpdate: []bindingsv1alpha1.EndpointBinding{},
			wantDelete: []bindingsv1alpha1.EndpointBinding{},
		},
		{
			name: "update endpointbindings",
			existing: []bindingsv1alpha1.EndpointBinding{
				epdExample1,
				epdExample2,
			},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample1: epdExample1,
				uriExample2: epdExample2,
			},
			wantCreate: []bindingsv1alpha1.EndpointBinding{},
			wantUpdate: []bindingsv1alpha1.EndpointBinding{
				epdExample1,
				epdExample2,
			},
			wantDelete: []bindingsv1alpha1.EndpointBinding{},
		},
		{
			name: "create, delete, and update",
			existing: []bindingsv1alpha1.EndpointBinding{
				epdExample1,
				epdExample2,
				// epdExample3 is missing, toCreate
			},
			desired: ngrokapi.AggregatedEndpoints{
				uriExample1: epdExample1,
				// epdExample2 is missing, toDelete
				uriExample3: epdExample3,
			},
			wantCreate: []bindingsv1alpha1.EndpointBinding{
				epdExample3,
			},
			wantUpdate: []bindingsv1alpha1.EndpointBinding{
				epdExample1,
			},
			wantDelete: []bindingsv1alpha1.EndpointBinding{
				epdExample2,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gotCreate, gotUpdate, gotDelete := filterEndpointBindingActions(test.existing, test.desired)

			assert.Equal(test.wantCreate, gotCreate)
			assert.Equal(test.wantUpdate, gotUpdate)
			assert.Equal(test.wantDelete, gotDelete)
		})
	}
}

func Test_EndpointBindingPoller_endpointBindingNeedsUpdate(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// some example EndpointBindings we can use for test cases
	uriExample1 := "http://service1.namespace1:8080"
	epdExample1 := bindingsv1alpha1.EndpointBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample1),
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			EndpointURI: uriExample1,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace1",
				Service:   "service1",
				Port:      8080,
				Protocol:  "TCP",
			},
		},
	}

	uriExample2 := "https://service2.namespace2:443"
	epdExample2 := bindingsv1alpha1.EndpointBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: hashURI(uriExample2),
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			EndpointURI: uriExample2,
			Target: bindingsv1alpha1.EndpointTarget{
				Namespace: "namespace2",
				Service:   "service2",
				Port:      443,
				Protocol:  "TCP",
			},
		},
	}

	tests := []struct {
		name     string
		existing bindingsv1alpha1.EndpointBinding
		desired  bindingsv1alpha1.EndpointBinding
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := endpointBindingNeedsUpdate(test.existing, test.desired)
			assert.Equal(test.want, got)
		})
	}
}
