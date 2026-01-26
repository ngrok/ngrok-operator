/*
MIT License

Copyright (c) 2025 ngrok, Inc.

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
package drain

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
)

type Drainer struct {
	Client client.Client
	Log    logr.Logger
	// Policy determines whether to delete ngrok API resources or just remove finalizers
	Policy ngrokv1alpha1.DrainPolicy

	// WatchNamespace limits draining to resources in this namespace (empty = all namespaces)
	WatchNamespace string
}

type DrainResult struct {
	Total     int
	Completed int
	Failed    int
	Errors    []error
}

func (r *DrainResult) Progress() string {
	return fmt.Sprintf("%d/%d", r.Completed+r.Failed, r.Total)
}

func (r *DrainResult) IsComplete() bool {
	return r.Completed+r.Failed >= r.Total
}

func (r *DrainResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *DrainResult) ErrorStrings() []string {
	strs := make([]string, len(r.Errors))
	for i, err := range r.Errors {
		strs[i] = err.Error()
	}
	return strs
}

type resourceHandler struct {
	name      string
	drainFunc func(ctx context.Context) (int, int, []error)
}

// RBAC permissions needed by the Drainer to list, update, and delete resources during drain.
// These are aggregated with the KubernetesOperator controller's RBAC.
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=agentendpoints,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=boundendpoints,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=get;list;watch;update;patch

func (d *Drainer) DrainAll(ctx context.Context) (*DrainResult, error) {
	result := &DrainResult{}

	handlers := []resourceHandler{
		{"HTTPRoute", d.drainHTTPRoutes},
		{"TCPRoute", d.drainTCPRoutes},
		{"TLSRoute", d.drainTLSRoutes},
		{"Ingress", d.drainIngresses},
		{"Service", d.drainServices},
		{"Gateway", d.drainGateways},
		{"CloudEndpoint", d.drainCloudEndpoints},
		{"AgentEndpoint", d.drainAgentEndpoints},
		{"Domain", d.drainDomains},
		{"IPPolicy", d.drainIPPolicies},
		{"BoundEndpoint", d.drainBoundEndpoints},
	}

	for _, h := range handlers {
		d.Log.Info("Draining resource type", "type", h.name)
		completed, total, errs := h.drainFunc(ctx)
		result.Completed += completed
		result.Total += total
		result.Failed += len(errs)
		result.Errors = append(result.Errors, errs...)
		d.Log.Info("Finished draining resource type",
			"type", h.name,
			"completed", completed,
			"total", total,
			"errors", len(errs),
		)
	}

	return result, nil
}

func (d *Drainer) drainUserResource(ctx context.Context, obj client.Object) error {
	if !util.HasFinalizer(obj) {
		return nil
	}

	util.RemoveFinalizer(obj)
	if err := d.Client.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to remove finalizer from %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	d.Log.V(1).Info("Removed finalizer from user resource", "namespace", obj.GetNamespace(), "name", obj.GetName())
	return nil
}

func (d *Drainer) drainOperatorResource(ctx context.Context, obj client.Object) error {
	switch d.Policy {
	case ngrokv1alpha1.DrainPolicyDelete:
		// Delete mode: Delete the CR without removing finalizer first.
		// The controller will handle ngrok API cleanup during the delete reconcile,
		// then remove the finalizer itself. This ensures proper cleanup ordering.
		if err := d.Client.Delete(ctx, obj); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
		}
		d.Log.V(1).Info("Deleted operator resource", "namespace", obj.GetNamespace(), "name", obj.GetName())

	case ngrokv1alpha1.DrainPolicyRetain:
		// Retain mode: Only remove finalizer so the CR can be garbage collected
		// when the CRD is removed. Do not delete to preserve ngrok API resources.
		if util.HasFinalizer(obj) {
			util.RemoveFinalizer(obj)
			if err := d.Client.Update(ctx, obj); err != nil {
				return fmt.Errorf("failed to remove finalizer from %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			d.Log.V(1).Info("Removed finalizer from operator resource", "namespace", obj.GetNamespace(), "name", obj.GetName())
		}
	}
	return nil
}

// namespaceListOption returns a list option to filter by WatchNamespace if set.
func (d *Drainer) namespaceListOption() []client.ListOption {
	if d.WatchNamespace == "" {
		return nil
	}
	return []client.ListOption{client.InNamespace(d.WatchNamespace)}
}

func (d *Drainer) drainHTTPRoutes(ctx context.Context) (completed, total int, errs []error) {
	list := &gatewayv1.HTTPRouteList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		if meta.IsNoMatchError(err) {
			d.Log.V(1).Info("HTTPRoute CRD not installed, skipping")
			return 0, 0, nil
		}
		return 0, 0, []error{fmt.Errorf("failed to list HTTPRoutes: %w", err)}
	}

	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainUserResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainTCPRoutes(ctx context.Context) (completed, total int, errs []error) {
	list := &gatewayv1alpha2.TCPRouteList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		if meta.IsNoMatchError(err) {
			d.Log.V(1).Info("TCPRoute CRD not installed, skipping")
			return 0, 0, nil
		}
		return 0, 0, []error{fmt.Errorf("failed to list TCPRoutes: %w", err)}
	}

	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainUserResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainTLSRoutes(ctx context.Context) (completed, total int, errs []error) {
	list := &gatewayv1alpha2.TLSRouteList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		if meta.IsNoMatchError(err) {
			d.Log.V(1).Info("TLSRoute CRD not installed, skipping")
			return 0, 0, nil
		}
		return 0, 0, []error{fmt.Errorf("failed to list TLSRoutes: %w", err)}
	}

	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainUserResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainIngresses(ctx context.Context) (completed, total int, errs []error) {
	list := &netv1.IngressList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list Ingresses: %w", err)}
	}

	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainUserResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainServices(ctx context.Context) (completed, total int, errs []error) {
	list := &corev1.ServiceList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list Services: %w", err)}
	}
	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainUserResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainGateways(ctx context.Context) (completed, total int, errs []error) {
	list := &gatewayv1.GatewayList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		if meta.IsNoMatchError(err) {
			d.Log.V(1).Info("Gateway CRD not installed, skipping")
			return 0, 0, nil
		}
		return 0, 0, []error{fmt.Errorf("failed to list Gateways: %w", err)}
	}

	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainUserResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainCloudEndpoints(ctx context.Context) (completed, total int, errs []error) {
	// CloudEndpoints can be user-created or operator-created.
	// List ALL CloudEndpoints and drain those that have our finalizer.
	list := &ngrokv1alpha1.CloudEndpointList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list CloudEndpoints: %w", err)}
	}
	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainOperatorResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainAgentEndpoints(ctx context.Context) (completed, total int, errs []error) {
	// AgentEndpoints can be user-created or operator-created.
	// List ALL AgentEndpoints and drain those that have our finalizer.
	list := &ngrokv1alpha1.AgentEndpointList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list AgentEndpoints: %w", err)}
	}
	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainOperatorResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainDomains(ctx context.Context) (completed, total int, errs []error) {
	// Domains are operator-managed and have our finalizer.
	list := &ingressv1alpha1.DomainList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list Domains: %w", err)}
	}
	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainOperatorResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainIPPolicies(ctx context.Context) (completed, total int, errs []error) {
	// IPPolicies are user-created CRDs.
	// List ALL IPPolicies and drain those that have our finalizer.
	list := &ingressv1alpha1.IPPolicyList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list IPPolicies: %w", err)}
	}
	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainOperatorResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}

func (d *Drainer) drainBoundEndpoints(ctx context.Context) (completed, total int, errs []error) {
	// BoundEndpoints are operator-managed.
	list := &bindingsv1alpha1.BoundEndpointList{}
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to list BoundEndpoints: %w", err)}
	}
	for i := range list.Items {
		if !util.HasFinalizer(&list.Items[i]) {
			continue
		}
		total++
		if err := d.drainOperatorResource(ctx, &list.Items[i]); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
	}
	return
}
