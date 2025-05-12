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
	"sort"
	"strings"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/errors"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
)

// Storer is the interface that wraps the required methods to gather information
// about ingresses, services, and other CRDs.
// It exposes methods to list both all and filtered resources
type Storer interface {
	Get(obj runtime.Object) (item any, exists bool, err error)
	Add(runtime.Object) error
	Update(runtime.Object) error
	Delete(runtime.Object) error

	GetIngressClassV1(name string) (*netv1.IngressClass, error)
	GetIngressV1(name, namespace string) (*netv1.Ingress, error)
	GetServiceV1(name, namespace string) (*corev1.Service, error)
	GetSecretV1(name, namespace string) (*corev1.Secret, error)
	GetReferenceGrant(name, namespace string) (*gatewayv1beta1.ReferenceGrant, error)
	GetNamespaceV1(name string) (*corev1.Namespace, error)
	GetConfigMapV1(name, namespace string) (*corev1.ConfigMap, error)
	GetNgrokIngressV1(name, namespace string) (*netv1.Ingress, error)
	GetNgrokModuleSetV1(name, namespace string) (*ingressv1alpha1.NgrokModuleSet, error)
	GetNgrokTrafficPolicyV1(name, namespace string) (*ngrokv1alpha1.NgrokTrafficPolicy, error)
	GetGateway(name string, namespace string) (*gatewayv1.Gateway, error)
	GetGatewayClass(name string) (*gatewayv1.GatewayClass, error)
	GetHTTPRoute(name string, namespace string) (*gatewayv1.HTTPRoute, error)
	GetTCPRoute(name string, namespace string) (*gatewayv1alpha2.TCPRoute, error)
	GetTLSRoute(name string, namespace string) (*gatewayv1alpha2.TLSRoute, error)

	ListIngressClassesV1() []*netv1.IngressClass
	ListNgrokIngressClassesV1() []*netv1.IngressClass

	ListIngressesV1() []*netv1.Ingress
	ListNgrokIngressesV1() []*netv1.Ingress

	ListGateways() []*gatewayv1.Gateway
	ListGatewayClasses() []*gatewayv1.GatewayClass
	ListHTTPRoutes() []*gatewayv1.HTTPRoute
	ListTCPRoutes() []*gatewayv1alpha2.TCPRoute
	ListTLSRoutes() []*gatewayv1alpha2.TLSRoute
	ListReferenceGrants() []*gatewayv1beta1.ReferenceGrant

	ListDomainsV1() []*ingressv1alpha1.Domain
	ListTunnelsV1() []*ingressv1alpha1.Tunnel
	ListHTTPSEdgesV1() []*ingressv1alpha1.HTTPSEdge
	ListNgrokModuleSetsV1() []*ingressv1alpha1.NgrokModuleSet
}

// Store implements Storer and can be used to list Ingress, Services
// and other resources from k8s APIserver. The backing stores should
// be synced and updated by the caller.
// It is ingressClass filter aware.
type Store struct {
	stores         CacheStores
	controllerName string
	log            logr.Logger
}

var _ Storer = Store{}

// New creates a new object store to be used in the ingress controller.
func New(cs CacheStores, controllerName string, logger logr.Logger) Storer {
	return Store{
		stores:         cs,
		controllerName: controllerName,
		log:            logger,
	}
}

// Get proxies the call to the underlying store.
func (s Store) Get(obj runtime.Object) (interface{}, bool, error) {
	return s.stores.Get(obj)
}

// Add proxies the call to the underlying store.
func (s Store) Add(obj runtime.Object) error {
	return s.stores.Add(obj.DeepCopyObject())
}

// Update proxies the call to the underlying store.
// An add for an object with the same key thats already present is just an update
func (s Store) Update(obj runtime.Object) error {
	return s.stores.Add(obj.DeepCopyObject())
}

// Delete proxies the call to the underlying store.
func (s Store) Delete(obj runtime.Object) error {
	return s.stores.Delete(obj)
}

// GetIngressClassV1 returns the 'name' IngressClass resource.
func (s Store) GetIngressClassV1(name string) (*netv1.IngressClass, error) {
	return genericGetByKey[netv1.IngressClass](s.stores.IngressClassV1, name)
}

// GetIngressV1 returns the 'name' Ingress resource.
func (s Store) GetIngressV1(name, namespace string) (*netv1.Ingress, error) {
	return genericGetByKey[netv1.Ingress](s.stores.IngressV1, getKey(name, namespace))
}

func (s Store) GetServiceV1(name, namespace string) (*corev1.Service, error) {
	return genericGetByKey[corev1.Service](s.stores.ServiceV1, getKey(name, namespace))
}

// GetIngressV1 returns the named Secret
func (s Store) GetSecretV1(name, namespace string) (*corev1.Secret, error) {
	return genericGetByKey[corev1.Secret](s.stores.SecretV1, getKey(name, namespace))
}

// GetConfigMapV1 returns the named ConfigMap
func (s Store) GetConfigMapV1(name, namespace string) (*corev1.ConfigMap, error) {
	return genericGetByKey[corev1.ConfigMap](s.stores.ConfigMapV1, getKey(name, namespace))
}

// GetNgrokIngressV1 looks up the Ingress resource by name and namespace and returns it if it's found
func (s Store) GetNgrokIngressV1(name, namespace string) (*netv1.Ingress, error) {
	ing, err := s.GetIngressV1(name, namespace)
	if err != nil {
		return nil, err
	}
	ok, err := s.shouldHandleIngress(ing)
	if !ok || err != nil {
		return nil, err
	}

	return ing, nil
}

func (s Store) GetNgrokModuleSetV1(name, namespace string) (*ingressv1alpha1.NgrokModuleSet, error) {
	return genericGetByKey[ingressv1alpha1.NgrokModuleSet](s.stores.NgrokModuleV1, getKey(name, namespace))
}

func (s Store) GetNgrokTrafficPolicyV1(name, namespace string) (*ngrokv1alpha1.NgrokTrafficPolicy, error) {
	return genericGetByKey[ngrokv1alpha1.NgrokTrafficPolicy](s.stores.NgrokTrafficPolicyV1, getKey(name, namespace))
}

func (s Store) GetGateway(name string, namespace string) (*gatewayv1.Gateway, error) {
	return genericGetByKey[gatewayv1.Gateway](s.stores.Gateway, getKey(name, namespace))
}

func (s Store) GetGatewayClass(name string) (*gatewayv1.GatewayClass, error) {
	return genericGetByKey[gatewayv1.GatewayClass](s.stores.GatewayClass, name)
}

func (s Store) GetHTTPRoute(name string, namespace string) (*gatewayv1.HTTPRoute, error) {
	return genericGetByKey[gatewayv1.HTTPRoute](s.stores.HTTPRoute, getKey(name, namespace))
}

func (s Store) GetTCPRoute(name string, namespace string) (*gatewayv1alpha2.TCPRoute, error) {
	return genericGetByKey[gatewayv1alpha2.TCPRoute](s.stores.TCPRoute, getKey(name, namespace))
}

func (s Store) GetTLSRoute(name string, namespace string) (*gatewayv1alpha2.TLSRoute, error) {
	return genericGetByKey[gatewayv1alpha2.TLSRoute](s.stores.TLSRoute, getKey(name, namespace))
}

// GetReferenceGrant returns the named ReferenceGrant
func (s Store) GetReferenceGrant(name, namespace string) (*gatewayv1beta1.ReferenceGrant, error) {
	return genericGetByKey[gatewayv1beta1.ReferenceGrant](s.stores.ReferenceGrant, getKey(name, namespace))
}

// GetNamespaceV1 returns the named Namespace
func (s Store) GetNamespaceV1(name string) (*corev1.Namespace, error) {
	return genericGetByKey[corev1.Namespace](s.stores.NamespaceV1, name)
}

func genericGetByKey[T any, PT interface {
	*T
	client.Object
}](s cache.Store, key string) (*T, error) {
	p, exists, err := s.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		var e PT = new(T)
		return nil, errors.NewErrorNotFound(fmt.Sprintf("%T %v not found", e.GetObjectKind().GroupVersionKind().Kind, key))
	}
	return p.(PT), nil
}

// ListIngressClassesV1 returns the list of Ingresses in the Ingress v1 store.
func (s Store) ListIngressClassesV1() []*netv1.IngressClass {
	return genericListSorted[netv1.IngressClass](s.log, s.stores.IngressClassV1)
}

// ListNgrokIngressClassesV1 returns the list of Ingresses in the Ingress v1 store filtered
// by ones that match the controllerName
func (s Store) ListNgrokIngressClassesV1() []*netv1.IngressClass {
	filteredClasses := []*netv1.IngressClass{}
	classes := s.ListIngressClassesV1()
	for _, class := range classes {
		if class.Spec.Controller == s.controllerName {
			filteredClasses = append(filteredClasses, class)
		}
	}

	return filteredClasses
}

// ListIngressesV1 returns the list of Ingresses in the Ingress v1 store.
func (s Store) ListIngressesV1() []*netv1.Ingress {
	return genericListSorted[netv1.Ingress](s.log, s.stores.IngressV1)
}

func (s Store) ListGateways() []*gatewayv1.Gateway {
	return genericListSorted[gatewayv1.Gateway](s.log, s.stores.Gateway)
}

func (s Store) ListGatewayClasses() []*gatewayv1.GatewayClass {
	return genericListSorted[gatewayv1.GatewayClass](s.log, s.stores.GatewayClass)
}

func (s Store) ListHTTPRoutes() []*gatewayv1.HTTPRoute {
	return genericList[gatewayv1.HTTPRoute](s.log, s.stores.HTTPRoute)
}

func (s Store) ListTCPRoutes() []*gatewayv1alpha2.TCPRoute {
	return genericList[gatewayv1alpha2.TCPRoute](s.log, s.stores.TCPRoute)
}

func (s Store) ListTLSRoutes() []*gatewayv1alpha2.TLSRoute {
	return genericList[gatewayv1alpha2.TLSRoute](s.log, s.stores.TLSRoute)
}

// ListReferenceGrants returns the stored ReferenceGrants
func (s Store) ListReferenceGrants() []*gatewayv1beta1.ReferenceGrant {
	return genericListSorted[gatewayv1beta1.ReferenceGrant](s.log, s.stores.ReferenceGrant)
}

// ListNamespaces returns the stored Namespaces
func (s Store) ListNamespaces() []*corev1.Namespace {
	return genericListSorted[corev1.Namespace](s.log, s.stores.NamespaceV1)
}

func (s Store) ListNgrokIngressesV1() []*netv1.Ingress {
	ings := s.ListIngressesV1()

	var ingresses []*netv1.Ingress
	for _, ing := range ings {
		ok, err := s.shouldHandleIngress(ing)
		if ok && err == nil {
			ingresses = append(ingresses, ing)
		}
	}
	return ingresses
}

// ListDomainsV1 returns the list of Domains in the Domain v1 store.
func (s Store) ListDomainsV1() []*ingressv1alpha1.Domain {
	return genericListSorted[ingressv1alpha1.Domain](s.log, s.stores.DomainV1)
}

// ListTunnelsV1 returns the list of Tunnels in the Tunnel v1 store.
func (s Store) ListTunnelsV1() []*ingressv1alpha1.Tunnel {
	return genericListSorted[ingressv1alpha1.Tunnel](s.log, s.stores.TunnelV1)
}

// ListHTTPSEdgesV1 returns the list of HTTPSEdges in the HTTPSEdge v1 store.
func (s Store) ListHTTPSEdgesV1() []*ingressv1alpha1.HTTPSEdge {
	return genericListSorted[ingressv1alpha1.HTTPSEdge](s.log, s.stores.HTTPSEdgeV1)
}

// ListNgrokModuleSetsV1 returns the list of NgrokModules in the NgrokModuleSet v1 store.
func (s Store) ListNgrokModuleSetsV1() []*ingressv1alpha1.NgrokModuleSet {
	return genericListSorted[ingressv1alpha1.NgrokModuleSet](s.log, s.stores.NgrokModuleV1)
}

func genericList[T any, PT interface {
	*T
	client.Object
}](log logr.Logger, s cache.Store) []PT {
	var result []PT
	for _, item := range s.List() {
		obj, ok := item.(PT)
		if !ok {
			var e PT = new(T)
			log.Error(nil, "List: dropping object of unexpected type", "expected", e.GetObjectKind().GroupVersionKind().Kind, "type", fmt.Sprintf("%#v", item))
			continue
		}
		result = append(result, obj)
	}

	return result
}

func genericListSorted[T any, PT interface {
	*T
	client.Object
}](log logr.Logger, s cache.Store) []PT {
	var result = genericList[T, PT](log, s)
	sort.SliceStable(result, func(i, j int) bool {
		is := fmt.Sprintf("%s/%s", result[i].GetNamespace(), result[i].GetName())
		js := fmt.Sprintf("%s/%s", result[j].GetNamespace(), result[j].GetName())
		return strings.Compare(is, js) < 0
	})
	return result
}

// shouldHandleIngress checks if the ingress object is valid and belongs to the correct class.
func (s Store) shouldHandleIngress(ing *netv1.Ingress) (bool, error) {
	if ing.Annotations != nil {
		if deprecatedClass, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
			s.log.Info(fmt.Sprintf("Deprecated annotation 'kubernetes.io/ingress.class' detected with value: %s", deprecatedClass))
		}
	}

	ok, err := s.shouldHandleIngressIsValid(ing)
	if err != nil {
		return ok, err
	}
	return s.shouldHandleIngressCheckClass(ing)
}

// shouldHandleIngressCheckClass checks if the ingress should be handled by the controller based on the ingress class
func (s Store) shouldHandleIngressCheckClass(ing *netv1.Ingress) (bool, error) {
	ngrokClasses := s.ListNgrokIngressClassesV1()
	if len(ngrokClasses) == 0 {
		return false, errors.NewNoDefaultIngressClassFound()
	}
	if ing.Spec.IngressClassName != nil {
		for _, class := range ngrokClasses {
			if *ing.Spec.IngressClassName == class.Name {
				return true, nil
			}
		}
		// Log a specific warning message for unmatched ingress class
		s.log.Info(fmt.Sprintf("Ingress is not of type %s so skipping it", s.controllerName), "class", *ing.Spec.IngressClassName)
		return false, errors.NewErrDifferentIngressClass(s.ListNgrokIngressClassesV1(), ing.Spec.IngressClassName)
	}

	// Check if any class is marked as default
	for _, class := range ngrokClasses {
		if class.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			return true, nil
		}
	}

	// Log if no default class found and no specific class match
	s.log.Info(fmt.Sprintf("Matching ingress class %s not found and no suitable default", s.controllerName), "ingress", ing.Name)
	return false, errors.NewErrDifferentIngressClass(ngrokClasses, ing.Spec.IngressClassName)
}

// shouldHandleIngressIsValid checks if the ingress spec meets controller requirements.
func (s Store) shouldHandleIngressIsValid(ing *netv1.Ingress) (bool, error) {
	errs := errors.NewErrInvalidIngressSpec()
	useEdges, err := annotations.ExtractUseEdges(ing)
	if err != nil {
		errs.AddError(fmt.Sprintf("failed to check %q annotation. defaulting to using endpoints: %s",
			annotations.MappingStrategyAnnotation,
			err.Error(),
		))
	}
	if len(ing.Spec.Rules) == 0 {
		errs.AddError("At least one rule is required to be set")
	} else {
		for _, rule := range ing.Spec.Rules {
			if rule.Host == "" {
				errs.AddError("A host is required to be set for each rule")
			}
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					switch {
					case path.Backend.Resource != nil:
						if useEdges {
							errs.AddError(fmt.Sprintf("Resource backends are not supported for ingresses with the %q: %q annotation. Ingresses provided by endpoints instead of edges do support default backends",
								annotations.MappingStrategyAnnotation,
								annotations.MappingStrategy_Edges,
							))
						}
					case path.Backend.Service == nil:
						errs.AddError(fmt.Sprintf("A valid service backend is required for this ingress since a resource backend was not provided (resource backends are only supported for ingresses without the %q: %q annotation.)",
							annotations.MappingStrategyAnnotation,
							annotations.MappingStrategy_Edges,
						))
					}
				}
			} else {
				errs.AddError("HTTP rules are required for ingress")
			}
		}
	}

	if ing.Spec.DefaultBackend != nil {
		if useEdges {
			errs.AddError(fmt.Sprintf("Default backends are not supported for ingresses with the %q: %q annotation. Ingresses provided by endpoints instead of edges do support default backends",
				annotations.MappingStrategyAnnotation,
				annotations.MappingStrategy_Edges,
			))
		}
	}

	if errs.HasErrors() {
		return false, errs
	}
	return true, nil
}
