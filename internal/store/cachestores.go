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
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
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
	NamespaceV1    cache.Store

	// Gateway API Stores
	Gateway        cache.Store
	GatewayClass   cache.Store
	HTTPRoute      cache.Store
	TCPRoute       cache.Store
	TLSRoute       cache.Store
	ReferenceGrant cache.Store

	// Ngrok Stores
	DomainV1             cache.Store
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
		NamespaceV1:    cache.NewStore(clusterResourceKeyFunc),
		ConfigMapV1:    cache.NewStore(keyFunc),
		// Gateway API Stores
		Gateway:        cache.NewStore(keyFunc),
		GatewayClass:   cache.NewStore(keyFunc),
		HTTPRoute:      cache.NewStore(keyFunc),
		TCPRoute:       cache.NewStore(keyFunc),
		TLSRoute:       cache.NewStore(keyFunc),
		ReferenceGrant: cache.NewStore(keyFunc),
		// Ngrok Stores
		DomainV1:             cache.NewStore(keyFunc),
		NgrokTrafficPolicyV1: cache.NewStore(keyFunc),
		AgentEndpointV1:      cache.NewStore(keyFunc),
		CloudEndpointV1:      cache.NewStore(keyFunc),
		l:                    &sync.RWMutex{},
		log:                  logger,
	}
}

func keyFunc(obj any) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(obj))
	name := v.FieldByName("Name")
	namespace := v.FieldByName("Namespace")
	return namespace.String() + "/" + name.String(), nil
}

func getKey(name, namespace string) string {
	return namespace + "/" + name
}

func clusterResourceKeyFunc(obj any) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(obj))
	return v.FieldByName("Name").String(), nil
}

// storeFor returns the cache.Store holding the provided object's kind. Get, Add
// and Delete all share this one mapping rather than repeating it: cache.Store
// accepts `any` and derives keys reflectively, so handing it the untyped object
// behaves identically to the type-asserted value.
func (c CacheStores) storeFor(obj runtime.Object) (cache.Store, error) {
	switch obj.(type) {
	// ----------------------------------------------------------------------------
	// Kubernetes Core API Support
	// ----------------------------------------------------------------------------
	case *netv1.Ingress:
		return c.IngressV1, nil
	case *netv1.IngressClass:
		return c.IngressClassV1, nil
	case *corev1.Service:
		return c.ServiceV1, nil
	case *corev1.Secret:
		return c.SecretV1, nil
	case *corev1.Namespace:
		return c.NamespaceV1, nil
	case *corev1.ConfigMap:
		return c.ConfigMapV1, nil

	// ----------------------------------------------------------------------------
	// Kubernetes Gateway API Support
	// ----------------------------------------------------------------------------
	case *gatewayv1.HTTPRoute:
		return c.HTTPRoute, nil
	case *gatewayv1alpha2.TCPRoute:
		return c.TCPRoute, nil
	case *gatewayv1alpha2.TLSRoute:
		return c.TLSRoute, nil
	case *gatewayv1.Gateway:
		return c.Gateway, nil
	case *gatewayv1.GatewayClass:
		return c.GatewayClass, nil
	case *gatewayv1beta1.ReferenceGrant:
		return c.ReferenceGrant, nil

	// ----------------------------------------------------------------------------
	// Ngrok API Support
	// ----------------------------------------------------------------------------
	case *ingressv1alpha1.Domain:
		return c.DomainV1, nil
	case *ngrokv1alpha1.NgrokTrafficPolicy:
		return c.NgrokTrafficPolicyV1, nil
	case *ngrokv1alpha1.AgentEndpoint:
		return c.AgentEndpointV1, nil
	case *ngrokv1alpha1.CloudEndpoint:
		return c.CloudEndpointV1, nil

	default:
		return nil, fmt.Errorf("unsupported object type: %T", obj)
	}
}

// Get checks whether or not there's already some version of the provided object present in the cache.
// The CacheStore must be initialized (see NewCacheStores()) or this will panic.
func (c CacheStores) Get(obj runtime.Object) (item any, exists bool, err error) {
	c.l.RLock()
	defer c.l.RUnlock()

	st, err := c.storeFor(obj)
	if err != nil {
		return nil, false, err
	}
	return st.Get(obj)
}

// Add stores a provided runtime.Object into the CacheStore if it's of a supported type.
// The CacheStore must be initialized (see NewCacheStores()) or this will panic.
func (c CacheStores) Add(obj runtime.Object) error {
	c.l.Lock()
	defer c.l.Unlock()

	st, err := c.storeFor(obj)
	if err != nil {
		return err
	}
	return st.Add(obj)
}

// Delete removes a provided runtime.Object from the CacheStore if it's of a supported type.
// The CacheStore must be initialized (see NewCacheStores()) or this will panic.
func (c CacheStores) Delete(obj runtime.Object) error {
	c.l.Lock()
	defer c.l.Unlock()

	st, err := c.storeFor(obj)
	if err != nil {
		return err
	}
	return st.Delete(obj)
}
