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
	"context"
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_EndpointBinding(t *testing.T) {
	assert := assert.New(t)

	// TODO(hkatz) implement me
	assert.True(true)
}

func Test_convertEndpointBindingToServices(t *testing.T) {
	assert := assert.New(t)

	controller := &EndpointBindingReconciler{
		ClusterDomain: "svc.cluster.local",
	}

	endpointBinding := &bindingsv1alpha1.EndpointBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abc123", // hashed/unique name
			Namespace: "ngrok-op",
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			Scheme: "https",
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   "client-service",
				Namespace: "client-namespace",
				Protocol:  "TCP",
				Port:      8080,
			},
		},
		Status: bindingsv1alpha1.EndpointBindingStatus{
			HashedName: "abc123",
		},
	}

	targetService, upstreamService := controller.convertEndpointBindingToServices(context.TODO(), endpointBinding)

	assert.Equal(targetService.Name, "client-service")
	assert.Equal(targetService.Namespace, "client-namespace")
	assert.Equal(targetService.Spec.Ports[0].Port, int32(8080))
	assert.Equal(targetService.Spec.Ports[0].Name, "https")
	assert.Equal(targetService.Spec.ExternalName, "abc123.ngrok-op.svc.cluster.local")

	assert.Equal(upstreamService.Name, "abc123")
	assert.Equal(upstreamService.Spec.Ports[0].Name, "https")
}
