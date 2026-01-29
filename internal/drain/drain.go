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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
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
			// Already gone, nothing more to do
			d.Log.V(1).Info("Resource already deleted", "namespace", obj.GetNamespace(), "name", obj.GetName())
			return nil
		}
		d.Log.V(1).Info("Issued delete for operator resource", "namespace", obj.GetNamespace(), "name", obj.GetName())

		// Wait for the resource to be fully deleted (finalizer removed by controller).
		// This ensures the controller has finished processing the delete before we continue.
		key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
		if err := d.waitForDeletion(ctx, obj, key); err != nil {
			return fmt.Errorf("failed waiting for deletion of %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		d.Log.V(1).Info("Resource fully deleted", "namespace", obj.GetNamespace(), "name", obj.GetName())

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

// waitForDeletion polls until the resource no longer exists or context is cancelled.
func (d *Drainer) waitForDeletion(ctx context.Context, obj client.Object, key types.NamespacedName) error {
	// Create a new instance of the same type to use for Get calls
	gvk := obj.GetObjectKind().GroupVersionKind()

	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		// We need a fresh object for each Get call
		fresh := obj.DeepCopyObject().(client.Object)
		err := d.Client.Get(ctx, key, fresh)
		if err != nil {
			if errors.IsNotFound(err) {
				return true, nil // Resource is gone
			}
			d.Log.V(1).Info("Error checking resource deletion status", "gvk", gvk, "key", key, "error", err)
			return false, nil // Retry on transient errors
		}
		// Resource still exists, keep waiting
		return false, nil
	})
}

// namespaceListOption returns a list option to filter by WatchNamespace if set.
func (d *Drainer) namespaceListOption() []client.ListOption {
	if d.WatchNamespace == "" {
		return nil
	}
	return []client.ListOption{client.InNamespace(d.WatchNamespace)}
}

// drainList is a generic helper that lists resources, iterates items with our finalizer,
// and calls the provided drain function. It handles optional CRD skip logic for Gateway API types.
func (d *Drainer) drainList(
	ctx context.Context,
	kind string,
	list client.ObjectList,
	skipNoMatch bool,
	drainOne func(context.Context, client.Object) error,
) (completed, total int, errs []error) {
	if err := d.Client.List(ctx, list, d.namespaceListOption()...); err != nil {
		if skipNoMatch && meta.IsNoMatchError(err) {
			d.Log.V(1).Info(kind + " CRD not installed, skipping")
			return 0, 0, nil
		}
		return 0, 0, []error{fmt.Errorf("failed to list %s: %w", kind, err)}
	}

	if err := meta.EachListItem(list, func(obj runtime.Object) error {
		co, ok := obj.(client.Object)
		if !ok {
			return fmt.Errorf("%s list item does not implement client.Object: %T", kind, obj)
		}
		if !util.HasFinalizer(co) {
			return nil
		}

		total++
		if err := drainOne(ctx, co); err != nil {
			errs = append(errs, err)
		} else {
			completed++
		}
		return nil
	}); err != nil {
		return 0, 0, []error{fmt.Errorf("failed to iterate %s list: %w", kind, err)}
	}

	return completed, total, errs
}

func (d *Drainer) drainHTTPRoutes(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "HTTPRoute", &gatewayv1.HTTPRouteList{}, true, d.drainUserResource)
}

func (d *Drainer) drainTCPRoutes(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "TCPRoute", &gatewayv1alpha2.TCPRouteList{}, true, d.drainUserResource)
}

func (d *Drainer) drainTLSRoutes(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "TLSRoute", &gatewayv1alpha2.TLSRouteList{}, true, d.drainUserResource)
}

func (d *Drainer) drainIngresses(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "Ingress", &netv1.IngressList{}, false, d.drainUserResource)
}

func (d *Drainer) drainServices(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "Service", &corev1.ServiceList{}, false, d.drainUserResource)
}

func (d *Drainer) drainGateways(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "Gateway", &gatewayv1.GatewayList{}, true, d.drainUserResource)
}

func (d *Drainer) drainCloudEndpoints(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "CloudEndpoint", &ngrokv1alpha1.CloudEndpointList{}, false, d.drainOperatorResource)
}

func (d *Drainer) drainAgentEndpoints(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "AgentEndpoint", &ngrokv1alpha1.AgentEndpointList{}, false, d.drainOperatorResource)
}

func (d *Drainer) drainDomains(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "Domain", &ingressv1alpha1.DomainList{}, false, d.drainOperatorResource)
}

func (d *Drainer) drainIPPolicies(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "IPPolicy", &ingressv1alpha1.IPPolicyList{}, false, d.drainOperatorResource)
}

func (d *Drainer) drainBoundEndpoints(ctx context.Context) (completed, total int, errs []error) {
	return d.drainList(ctx, "BoundEndpoint", &bindingsv1alpha1.BoundEndpointList{}, false, d.drainOperatorResource)
}
