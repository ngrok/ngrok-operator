/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package store

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// CacheStores stores cache.Store for all Kinds of k8s objects that
// the Ingress Controller reads.
type CacheStores struct {
	// Core Kubernetes Stores
	IngressV1      cache.Store
	IngressClassV1 cache.Store
	ServiceV1      cache.Store
	SecretV1       cache.Store
	ConfigMapV1    cache.Store

	// Gateway API Stores
	Gateway      cache.Store
	GatewayClass cache.Store
	HTTPRoute    cache.Store

	// Ngrok Stores
	DomainV1             cache.Store
	TunnelV1             cache.Store
	HTTPSEdgeV1          cache.Store
	NgrokModuleV1        cache.Store
	NgrokTrafficPolicyV1 cache.Store
	AgentEndpointV1      cache.Store
	CloudEndpointV1      cache.Store

	log logr.Logger
	l   *sync.RWMutex
}

// NewCacheStores is a convenience function for CacheStores to initialize all attributes with new cache stores.
func NewCacheStores(logger logr.Logger) CacheStores {
	return CacheStores{
		// Core Kubernetes Stores
		IngressV1:      cache.NewStore(keyFunc),
		IngressClassV1: cache.NewStore(clusterResourceKeyFunc),
		ServiceV1:      cache.NewStore(keyFunc),
		SecretV1:       cache.NewStore(keyFunc),
		ConfigMapV1:    cache.NewStore(keyFunc),
		// Gateway API Stores
		Gateway:      cache.NewStore(keyFunc),
		GatewayClass: cache.NewStore(keyFunc),
		HTTPRoute:    cache.NewStore(keyFunc),
		// Ngrok Stores
		DomainV1:             cache.NewStore(keyFunc),
		TunnelV1:             cache.NewStore(keyFunc),
		HTTPSEdgeV1:          cache.NewStore(keyFunc),
		NgrokModuleV1:        cache.NewStore(keyFunc),
		NgrokTrafficPolicyV1: cache.NewStore(keyFunc),
		AgentEndpointV1:      cache.NewStore(keyFunc),
		CloudEndpointV1:      cache.NewStore(keyFunc),
		l:                    &sync.RWMutex{},
		log:                  logger,
	}
}

func keyFunc(obj interface{}) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(obj))
	name := v.FieldByName("Name")
	namespace := v.FieldByName("Namespace")
	return namespace.String() + "/" + name.String(), nil
}

func getKey(name, namespace string) string {
	return namespace + "/" + name
}

func clusterResourceKeyFunc(obj interface{}) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(obj))
	return v.FieldByName("Name").String(), nil
}

// Get checks whether or not there's already some version of the provided object present in the cache.
// The CacheStore must be initialized (see NewCacheStores()) or this will panic.
func (c CacheStores) Get(obj runtime.Object) (item interface{}, exists bool, err error) {
	c.l.RLock()
	defer c.l.RUnlock()

	switch obj := obj.(type) {
	// ----------------------------------------------------------------------------
	// Kubernetes Core API Support
	// ----------------------------------------------------------------------------
	case *netv1.Ingress:
		return c.IngressV1.Get(obj)
	case *netv1.IngressClass:
		return c.IngressClassV1.Get(obj)
	case *corev1.Service:
		return c.ServiceV1.Get(obj)
	case *corev1.Secret:
		return c.SecretV1.Get(obj)
	case *corev1.ConfigMap:
		return c.ConfigMapV1.Get(obj)

	// ----------------------------------------------------------------------------
	// Kubernetes Gateway API Support
	// ----------------------------------------------------------------------------
	case *gatewayv1.HTTPRoute:
		return c.HTTPRoute.Get(obj)
	case *gatewayv1.Gateway:
		return c.Gateway.Get(obj)
	case *gatewayv1.GatewayClass:
		return c.GatewayClass.Get(obj)

	// ----------------------------------------------------------------------------
	// Ngrok API Support
	// ----------------------------------------------------------------------------
	case *ingressv1alpha1.Domain:
		return c.DomainV1.Get(obj)
	case *ingressv1alpha1.Tunnel:
		return c.TunnelV1.Get(obj)
	case *ingressv1alpha1.HTTPSEdge:
		return c.HTTPSEdgeV1.Get(obj)
	case *ingressv1alpha1.NgrokModuleSet:
		return c.NgrokModuleV1.Get(obj)
	case *ngrokv1alpha1.NgrokTrafficPolicy:
		return c.NgrokTrafficPolicyV1.Get(obj)
	case *ngrokv1alpha1.AgentEndpoint:
		return c.AgentEndpointV1.Get(obj)
	case *ngrokv1alpha1.CloudEndpoint:
		return c.CloudEndpointV1.Get(obj)
	default:
		return nil, false, fmt.Errorf("unsupported object type: %T", obj)
	}
}

// Add stores a provided runtime.Object into the CacheStore if it's of a supported type.
// The CacheStore must be initialized (see NewCacheStores()) or this will panic.
func (c CacheStores) Add(obj runtime.Object) error {
	c.l.Lock()
	defer c.l.Unlock()

	switch obj := obj.(type) {
	// ----------------------------------------------------------------------------
	// Kubernetes Core API Support
	// ----------------------------------------------------------------------------
	case *netv1.Ingress:
		return c.IngressV1.Add(obj)
	case *netv1.IngressClass:
		return c.IngressClassV1.Add(obj)
	case *corev1.Service:
		return c.ServiceV1.Add(obj)
	case *corev1.Secret:
		return c.SecretV1.Add(obj)
	case *corev1.ConfigMap:
		return c.ConfigMapV1.Add(obj)

	// ----------------------------------------------------------------------------
	// Kubernetes Gateway API Support
	// ----------------------------------------------------------------------------
	case *gatewayv1.HTTPRoute:
		return c.HTTPRoute.Add(obj)
	case *gatewayv1.Gateway:
		return c.Gateway.Add(obj)
	case *gatewayv1.GatewayClass:
		return c.GatewayClass.Add(obj)

	// ----------------------------------------------------------------------------
	// Ngrok API Support
	// ----------------------------------------------------------------------------
	case *ingressv1alpha1.Domain:
		return c.DomainV1.Add(obj)
	case *ingressv1alpha1.Tunnel:
		return c.TunnelV1.Add(obj)
	case *ingressv1alpha1.HTTPSEdge:
		return c.HTTPSEdgeV1.Add(obj)
	case *ingressv1alpha1.NgrokModuleSet:
		return c.NgrokModuleV1.Add(obj)
	case *ngrokv1alpha1.NgrokTrafficPolicy:
		return c.NgrokTrafficPolicyV1.Add(obj)
	case *ngrokv1alpha1.AgentEndpoint:
		return c.AgentEndpointV1.Add(obj)
	case *ngrokv1alpha1.CloudEndpoint:
		return c.CloudEndpointV1.Add(obj)

	default:
		return fmt.Errorf("unsupported object type: %T", obj)
	}
}

// Delete removes a provided runtime.Object from the CacheStore if it's of a supported type.
// The CacheStore must be initialized (see NewCacheStores()) or this will panic.
func (c CacheStores) Delete(obj runtime.Object) error {
	c.l.Lock()
	defer c.l.Unlock()

	switch obj := obj.(type) {
	// ----------------------------------------------------------------------------
	// Kubernetes Core API Support
	// ----------------------------------------------------------------------------
	case *netv1.Ingress:
		return c.IngressV1.Delete(obj)
	case *netv1.IngressClass:
		return c.IngressClassV1.Delete(obj)
	case *corev1.Service:
		return c.ServiceV1.Delete(obj)
	case *corev1.Secret:
		return c.SecretV1.Delete(obj)
	case *corev1.ConfigMap:
		return c.ConfigMapV1.Delete(obj)

	// ----------------------------------------------------------------------------
	// Kubernetes Gateway API Support
	// ----------------------------------------------------------------------------
	case *gatewayv1.HTTPRoute:
		return c.HTTPRoute.Delete(obj)
	case *gatewayv1.Gateway:
		return c.Gateway.Delete(obj)
	case *gatewayv1.GatewayClass:
		return c.GatewayClass.Delete(obj)

	// ----------------------------------------------------------------------------
	// Ngrok API Support
	// ----------------------------------------------------------------------------
	case *ingressv1alpha1.Domain:
		return c.DomainV1.Delete(obj)
	case *ingressv1alpha1.Tunnel:
		return c.TunnelV1.Delete(obj)
	case *ingressv1alpha1.HTTPSEdge:
		return c.HTTPSEdgeV1.Delete(obj)
	case *ingressv1alpha1.NgrokModuleSet:
		return c.NgrokModuleV1.Delete(obj)
	case *ngrokv1alpha1.NgrokTrafficPolicy:
		return c.NgrokTrafficPolicyV1.Delete(obj)
	case *ngrokv1alpha1.AgentEndpoint:
		return c.AgentEndpointV1.Delete(obj)
	case *ngrokv1alpha1.CloudEndpoint:
		return c.CloudEndpointV1.Delete(obj)

	default:
		return fmt.Errorf("unsupported object type: %T", obj)
	}
}
