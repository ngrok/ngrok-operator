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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func Test_convertBoundEndpointToServices_HTTP(t *testing.T) {
	assert := assert.New(t)

	controller := &BoundEndpointReconciler{
		ClusterDomain: "svc.cluster.local",
	}

	boundEndpoint := &bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-http",
			Namespace: "ngrok-op",
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			Scheme: "http",
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   "web-service",
				Namespace: "default",
				Protocol:  "TCP",
				Port:      80,
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: "test-http",
		},
	}

	targetService, upstreamService := controller.convertBoundEndpointToServices(boundEndpoint)

	assert.Equal(targetService.Name, "web-service")
	assert.Equal(targetService.Namespace, "default")
	assert.Equal(targetService.Spec.Ports[0].Port, int32(80))
	assert.Equal(targetService.Spec.Ports[0].Name, "http")
	assert.Equal(targetService.Spec.ExternalName, "test-http.ngrok-op.svc.cluster.local")

	assert.Equal(upstreamService.Name, "test-http")
	assert.Equal(upstreamService.Namespace, "ngrok-op")
	assert.Equal(upstreamService.Spec.Ports[0].Name, "http")
}

func Test_convertBoundEndpointToServices_TCP(t *testing.T) {
	assert := assert.New(t)

	controller := &BoundEndpointReconciler{
		ClusterDomain: "cluster.local",
	}

	boundEndpoint := &bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tcp",
			Namespace: "ngrok-op",
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			Scheme: "tcp",
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   "db-service",
				Namespace: "databases",
				Protocol:  "TCP",
				Port:      5432,
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			HashedName: "test-tcp",
		},
	}

	targetService, upstreamService := controller.convertBoundEndpointToServices(boundEndpoint)

	assert.Equal(targetService.Name, "db-service")
	assert.Equal(targetService.Namespace, "databases")
	assert.Equal(targetService.Spec.Ports[0].Port, int32(5432))
	assert.Equal(targetService.Spec.Ports[0].Name, "tcp")
	assert.Equal(targetService.Spec.Type, v1.ServiceTypeExternalName)
	assert.Equal(targetService.Spec.ExternalName, "test-tcp.ngrok-op.cluster.local")

	assert.Equal(upstreamService.Name, "test-tcp")
	assert.Equal(upstreamService.Spec.Type, v1.ServiceTypeClusterIP)
	assert.Equal(upstreamService.Spec.Ports[0].Name, "tcp")
	assert.Equal(upstreamService.Spec.Ports[0].Port, int32(5432))
}
