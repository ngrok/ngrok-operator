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

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/go-logr/logr"
)

// Storer is the interface that wraps the required methods to gather information
// about ingresses, services, secrets and ingress annotations.
// It exposes methods to list both all and filtered resources
type Storer interface {
	Get(obj runtime.Object) (item interface{}, exists bool, err error)
	Add(runtime.Object) error
	Update(runtime.Object) error
	Delete(runtime.Object) error

	GetIngressClassV1(name string) (*netv1.IngressClass, error)
	GetIngressV1(name, namespace string) (*netv1.Ingress, error)
	GetNgrokIngressV1(name, namespace string) (*netv1.Ingress, error)

	ListIngressClassesV1() []*netv1.IngressClass
	ListNgrokIngressClassesV1() []*netv1.IngressClass

	ListIngressesV1() []*netv1.Ingress
	ListNgrokIngressesV1() []*netv1.Ingress

	ListDomainsV1() []*ingressv1alpha1.Domain
	ListTunnelsV1() []*ingressv1alpha1.Tunnel
	ListHTTPSEdgesV1() []*ingressv1alpha1.HTTPSEdge
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
	p, exists, err := s.stores.IngressClassV1.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewErrorNotFound(fmt.Sprintf("IngressClass %v not found", name))
	}
	return p.(*netv1.IngressClass), nil
}

// GetIngressV1 returns the 'name' Ingress resource.
func (s Store) GetIngressV1(name, namespcae string) (*netv1.Ingress, error) {
	p, exists, err := s.stores.IngressV1.GetByKey(getKey(name, namespcae))
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewErrorNotFound(fmt.Sprintf("Ingress %v not found", name))
	}
	return p.(*netv1.Ingress), nil
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

// ListIngressClassesV1 returns the list of Ingresses in the Ingress v1 store.
func (s Store) ListIngressClassesV1() []*netv1.IngressClass {
	// filter ingress rules
	var classes []*netv1.IngressClass
	for _, item := range s.stores.IngressClassV1.List() {
		class, ok := item.(*netv1.IngressClass)
		if !ok {
			s.log.Info("listIngressClassesV1: dropping object of unexpected type: %#v", item)
			continue
		}
		classes = append(classes, class)
	}

	sort.SliceStable(classes, func(i, j int) bool {
		return strings.Compare(classes[i].Name, classes[j].Name) < 0
	})

	return classes
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
	// filter ingress rules
	var ingresses []*netv1.Ingress

	for _, item := range s.stores.IngressV1.List() {
		ing, ok := item.(*netv1.Ingress)
		if !ok {
			e := fmt.Sprintf("listIngressesV1: dropping object of unexpected type: %#v", item)
			s.log.Error(fmt.Errorf(e), e)
			continue
		}
		ingresses = append(ingresses, ing)
	}

	sort.SliceStable(ingresses, func(i, j int) bool {
		return strings.Compare(fmt.Sprintf("%s/%s", ingresses[i].Namespace, ingresses[i].Name),
			fmt.Sprintf("%s/%s", ingresses[j].Namespace, ingresses[j].Name)) < 0
	})

	return ingresses
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
	// filter ingress rules
	var domains []*ingressv1alpha1.Domain
	for _, item := range s.stores.DomainV1.List() {
		domain, ok := item.(*ingressv1alpha1.Domain)
		if !ok {
			s.log.Info("listDomainsV1: dropping object of unexpected type: %#v", item)
			continue
		}
		domains = append(domains, domain)
	}

	sort.SliceStable(domains, func(i, j int) bool {
		return strings.Compare(fmt.Sprintf("%s/%s", domains[i].Namespace, domains[i].Name),
			fmt.Sprintf("%s/%s", domains[j].Namespace, domains[j].Name)) < 0
	})

	return domains
}

// ListTunnelsV1 returns the list of Tunnels in the Tunnel v1 store.
func (s Store) ListTunnelsV1() []*ingressv1alpha1.Tunnel {
	var tunnels []*ingressv1alpha1.Tunnel
	for _, item := range s.stores.TunnelV1.List() {
		tunnel, ok := item.(*ingressv1alpha1.Tunnel)
		if !ok {
			s.log.Info("listTunnelsV1: dropping object of unexpected type: %#v", item)
			continue
		}
		tunnels = append(tunnels, tunnel)
	}

	sort.SliceStable(tunnels, func(i, j int) bool {
		return strings.Compare(fmt.Sprintf("%s/%s", tunnels[i].Namespace, tunnels[i].Name),
			fmt.Sprintf("%s/%s", tunnels[j].Namespace, tunnels[j].Name)) < 0
	})

	return tunnels
}

// ListHTTPSEdgesV1 returns the list of HTTPSEdges in the HTTPSEdge v1 store.
func (s Store) ListHTTPSEdgesV1() []*ingressv1alpha1.HTTPSEdge {
	var edges []*ingressv1alpha1.HTTPSEdge
	for _, item := range s.stores.HTTPSEdgeV1.List() {
		edge, ok := item.(*ingressv1alpha1.HTTPSEdge)
		if !ok {
			s.log.Info("listHTTPSEdgesV1: dropping object of unexpected type: %#v", item)
			continue
		}
		edges = append(edges, edge)
	}

	sort.SliceStable(edges, func(i, j int) bool {
		return strings.Compare(fmt.Sprintf("%s/%s", edges[i].Namespace, edges[i].Name),
			fmt.Sprintf("%s/%s", edges[j].Namespace, edges[j].Name)) < 0
	})

	return edges
}

func (s Store) shouldHandleIngress(ing *netv1.Ingress) (bool, error) {
	ok, err := s.shouldHandleIngressIsValid(ing)
	if err != nil {
		return ok, err
	}
	return s.shouldHandleIngressCheckClass(ing)
}

// shouldHandleIngressCheckClass checks if the ingress should be handled by the controller based on the ingress class
func (s Store) shouldHandleIngressCheckClass(ing *netv1.Ingress) (bool, error) {
	ngrokClasses := s.ListNgrokIngressClassesV1()
	if ing.Spec.IngressClassName != nil {
		for _, class := range ngrokClasses {
			if *ing.Spec.IngressClassName == class.Name {
				return true, nil
			}
		}
	} else {
		for _, class := range ngrokClasses {
			if class.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
				return true, nil
			}
		}
	}
	return false, errors.NewErrDifferentIngressClass(s.ListNgrokIngressClassesV1(), ing.Spec.IngressClassName)
}

// shouldHandleIngressIsValid checks if the ingress should be handled by the controller based on the ingress spec
func (s Store) shouldHandleIngressIsValid(ing *netv1.Ingress) (bool, error) {
	errs := errors.NewErrInvalidIngressSpec()
	if len(ing.Spec.Rules) > 1 {
		errs.AddError(fmt.Sprintf("A maximum of one rule is required to be set"))
	}
	if len(ing.Spec.Rules) == 0 {
		errs.AddError(fmt.Sprintf("At least one rule is required to be set"))
	} else {
		if ing.Spec.Rules[0].Host == "" {
			errs.AddError(fmt.Sprintf("A host is required to be set"))
		}

		for _, path := range ing.Spec.Rules[0].HTTP.Paths {
			if path.Backend.Resource != nil {
				errs.AddError(fmt.Sprintf("Resource backends are not supported"))
			}
		}
	}

	if errs.HasErrors() {
		return false, errs
	}
	return true, nil
}
