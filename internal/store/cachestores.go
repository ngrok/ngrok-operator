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
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// CacheStores stores cache.Store for all Kinds of k8s objects that
// the Ingress Controller reads.
type CacheStores struct {
	// Core Kubernetes Stores
	IngressV1      cache.Store
	IngressClassV1 cache.Store

	// Ngrok Stores
	DomainV1      cache.Store
	TunnelV1      cache.Store
	HTTPSEdgeV1   cache.Store
	NgrokModuleV1 cache.Store

	log logr.Logger
	l   *sync.RWMutex
}

// NewCacheStores is a convenience function for CacheStores to initialize all attributes with new cache stores.
func NewCacheStores(logger logr.Logger) CacheStores {
	return CacheStores{
		IngressV1:      cache.NewStore(keyFunc),
		IngressClassV1: cache.NewStore(clusterResourceKeyFunc),
		DomainV1:       cache.NewStore(keyFunc),
		TunnelV1:       cache.NewStore(keyFunc),
		HTTPSEdgeV1:    cache.NewStore(keyFunc),
		NgrokModuleV1:  cache.NewStore(keyFunc),
		l:              &sync.RWMutex{},
		log:            logger,
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
	default:
		return fmt.Errorf("unsupported object type: %T", obj)
	}
}
