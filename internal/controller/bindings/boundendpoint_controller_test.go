/*
MIT License

Copyright (c) 2024 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package bindings

import (
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_BoundEndpoint(t *testing.T) {
	assert := assert.New(t)

	// TODO(hkatz) implement me
	assert.True(true)
}

func Test_convertBoundEndpointToServices(t *testing.T) {
	assert := assert.New(t)

	controller := &BoundEndpointReconciler{
		ClusterDomain: "svc.cluster.local",
	}

	boundEndpoint := &bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abc123", // hashed/unique name
			Namespace: "ngrok-op",
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			Scheme: "https",
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   "client-service",
				Namespace: "client-namespace",
				Protocol:  "TCP",
				Port:      8080,
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: "abc123",
		},
	}

	targetService, upstreamService := controller.convertBoundEndpointToServices(boundEndpoint)

	assert.Equal(targetService.Name, "client-service")
	assert.Equal(targetService.Namespace, "client-namespace")
	assert.Equal(targetService.Spec.Ports[0].Port, int32(8080))
	assert.Equal(targetService.Spec.Ports[0].Name, "https")
	assert.Equal(targetService.Spec.ExternalName, "abc123.ngrok-op.svc.cluster.local")

	assert.Equal(upstreamService.Name, "abc123")
	assert.Equal(upstreamService.Spec.Ports[0].Name, "https")
}

func Test_setEndpointsStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		boundEndpoint *bindingsv1alpha1.BoundEndpoint
		desired       *bindingsv1alpha1.BindingEndpoint
	}{
		{
			name: "Set provisioning status",
			boundEndpoint: &bindingsv1alpha1.BoundEndpoint{
				Status: bindingsv1alpha1.BoundEndpointStatus{
					Endpoints: []bindingsv1alpha1.BindingEndpoint{
						{
							Status:       bindingsv1alpha1.StatusUnknown,
							ErrorCode:    "",
							ErrorMessage: "",
						},
						{
							Status:       bindingsv1alpha1.StatusProvisioning,
							ErrorCode:    "",
							ErrorMessage: "",
						},
						{
							Status:       bindingsv1alpha1.StatusProvisioning,
							ErrorCode:    "",
							ErrorMessage: "",
						},
					},
				},
			},
			desired: &bindingsv1alpha1.BindingEndpoint{
				Status:       bindingsv1alpha1.StatusProvisioning,
				ErrorCode:    "",
				ErrorMessage: "",
			},
		},
		{
			name: "Set error status",
			boundEndpoint: &bindingsv1alpha1.BoundEndpoint{
				Status: bindingsv1alpha1.BoundEndpointStatus{
					Endpoints: []bindingsv1alpha1.BindingEndpoint{
						{
							Status:       bindingsv1alpha1.StatusProvisioning,
							ErrorCode:    "",
							ErrorMessage: "",
						},
						{
							Status:       bindingsv1alpha1.StatusProvisioning,
							ErrorCode:    "",
							ErrorMessage: "",
						},
						{
							Status:       bindingsv1alpha1.StatusProvisioning,
							ErrorCode:    "",
							ErrorMessage: "",
						},
					},
				},
			},
			desired: &bindingsv1alpha1.BindingEndpoint{
				Status:       bindingsv1alpha1.StatusError,
				ErrorCode:    "ERR_NGROK_1234",
				ErrorMessage: "Example Error Message",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)

			setEndpointsStatus(test.boundEndpoint, test.desired)

			for _, endpoint := range test.boundEndpoint.Status.Endpoints {
				assert.Equal(endpoint.Status, test.desired.Status)
				assert.Equal(endpoint.ErrorCode, test.desired.ErrorCode)
				assert.Equal(endpoint.ErrorMessage, test.desired.ErrorMessage)
			}
		})
	}

}
