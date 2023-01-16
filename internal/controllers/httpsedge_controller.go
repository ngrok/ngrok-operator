/*
MIT License

Copyright (c) 2022 ngrok, Inc.

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

package controllers

import (
	"context"
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/backends/tunnel_group"
	httpsedge "github.com/ngrok/ngrok-api-go/v5/edges/https"
	httpsedgeroutes "github.com/ngrok/ngrok-api-go/v5/edges/https_routes"
	ingressv1alpha1 "github.com/ngrok/ngrok-ingress-controller/api/v1alpha1"
)

// HTTPSEdgeReconciler reconciles a HTTPSEdge object
type HTTPSEdgeReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	HTTPSEdgeClient          *httpsedge.Client
	HTTPSEdgeRoutesClient    *httpsedgeroutes.Client
	TunnelGroupBackendClient *tunnel_group.Client
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPSEdgeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.HTTPSEdge{}).
		WithEventFilter(commonPredicateFilters).
		Complete(r)
}

//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=httpsedges,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=httpsedges/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=httpsedges/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HTTPSEdge object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *HTTPSEdgeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("V1Alpha1HTTPSEdge", req.NamespacedName)

	edge := new(ingressv1alpha1.HTTPSEdge)
	if err := r.Get(ctx, req.NamespacedName, edge); err != nil {
		log.Error(err, "unable to fetch Edge")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if edge == nil {
		return ctrl.Result{}, nil
	}

	if edge.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := registerAndSyncFinalizer(ctx, r.Client, edge); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// The object is being deleted
		if hasFinalizer(edge) {
			if edge.Status.ID != "" {
				r.Recorder.Event(edge, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting Edge %s", edge.Name))
				if err := r.HTTPSEdgeClient.Delete(ctx, edge.Status.ID); err != nil {
					if !ngrok.IsNotFound(err) {
						r.Recorder.Event(edge, v1.EventTypeWarning, "FailedDelete", fmt.Sprintf("Failed to delete Edge %s: %s", edge.Name, err.Error()))
						return ctrl.Result{}, err
					}
				}
				edge.Status.ID = ""
			}

			if err := removeAndSyncFinalizer(ctx, r.Client, edge); err != nil {
				return ctrl.Result{}, err
			}
		}

		r.Recorder.Event(edge, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted Edge %s", edge.Name))

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.reconcileEdge(ctx, edge)
}

func (r *HTTPSEdgeReconciler) reconcileEdge(ctx context.Context, edge *ingressv1alpha1.HTTPSEdge) error {
	var remoteEdge *ngrok.HTTPSEdge
	var err error

	if edge.Status.ID != "" {
		// We already have an ID, so we can just update the resource
		// TODO: Update the edge if the hostports don't match or the metadata doesn't match
		remoteEdge, err = r.HTTPSEdgeClient.Get(ctx, edge.Status.ID)
		if err != nil {
			return err
		}
	} else {
		// Try to find the edge by Hostports
		remoteEdge, err = r.findEdgeByHostports(ctx, edge.Spec.Hostports)
		if err != nil {
			return err
		}

		// Not found, so create it
		if remoteEdge == nil {
			remoteEdge, err = r.HTTPSEdgeClient.Create(ctx, &ngrok.HTTPSEdgeCreate{
				Metadata:    edge.Spec.Metadata,
				Description: edge.Spec.Description,
				Hostports:   edge.Spec.Hostports,
			})
			if err != nil {
				return err
			}
		}
	}

	if err = r.updateStatus(ctx, edge, remoteEdge); err != nil {
		return err
	}

	return r.reconcileRoutes(ctx, edge, remoteEdge)
}

// TODO: This is going to be a bit messy right now, come back and make this cleaner
func (r *HTTPSEdgeReconciler) reconcileRoutes(ctx context.Context, edge *ingressv1alpha1.HTTPSEdge, remoteEdge *ngrok.HTTPSEdge) error {
	routeStatuses := make([]ingressv1alpha1.HTTPSEdgeRouteStatus, len(edge.Spec.Routes))
	tunnelGroupReconciler, err := newTunnelGroupBackendReconciler(r.TunnelGroupBackendClient)
	if err != nil {
		return err
	}

	for i, routeSpec := range edge.Spec.Routes {
		backend, err := tunnelGroupReconciler.findOrCreate(ctx, routeSpec.Backend)
		if err != nil {
			return err
		}

		match := r.getMatchingRouteFromEdgeStatus(edge, routeSpec)
		var route *ngrok.HTTPSEdgeRoute
		if match == nil {
			r.Log.Info("Creating new route", "edgeID", edge.Status.ID, "match", routeSpec.Match, "matchType", routeSpec.MatchType, "backendID", backend.ID)
			// This is a new route, so we need to create it
			req := &ngrok.HTTPSEdgeRouteCreate{
				EdgeID:    edge.Status.ID,
				Match:     routeSpec.Match,
				MatchType: routeSpec.MatchType,
				Backend: &ngrok.EndpointBackendMutate{
					BackendID: backend.ID,
				},
			}
			if routeSpec.Compression != nil {
				req.Compression = &ngrok.EndpointCompression{
					Enabled: routeSpec.Compression.Enabled,
				}
			}
			route, err = r.HTTPSEdgeRoutesClient.Create(ctx, req)
		} else {
			r.Log.Info("Updating route", "edgeID", edge.Status.ID, "match", routeSpec.Match, "matchType", routeSpec.MatchType, "backendID", backend.ID)
			// This is an existing route, so we need to update it
			req := &ngrok.HTTPSEdgeRouteUpdate{
				EdgeID:    edge.Status.ID,
				ID:        match.ID,
				Match:     routeSpec.Match,
				MatchType: routeSpec.MatchType,
				Backend: &ngrok.EndpointBackendMutate{
					BackendID: backend.ID,
				},
			}
			if routeSpec.Compression != nil {
				req.Compression = &ngrok.EndpointCompression{
					Enabled: routeSpec.Compression.Enabled,
				}
			}
			route, err = r.HTTPSEdgeRoutesClient.Update(ctx, req)
		}
		if err != nil {
			return err
		}
		routeStatuses[i] = ingressv1alpha1.HTTPSEdgeRouteStatus{
			ID:        route.ID,
			URI:       route.URI,
			Match:     route.Match,
			MatchType: route.MatchType,
		}
		if route.Backend != nil {
			routeStatuses[i].Backend = ingressv1alpha1.TunnelGroupBackendStatus{
				ID: route.Backend.Backend.ID,
			}
		}
	}

	edge.Status.Routes = routeStatuses

	return r.Status().Update(ctx, edge)
}

func (r *HTTPSEdgeReconciler) findEdgeByHostports(ctx context.Context, hostports []string) (*ngrok.HTTPSEdge, error) {
	iter := r.HTTPSEdgeClient.List(&ngrok.Paging{})
	for iter.Next(ctx) {
		edge := iter.Item()

		// If the number of hostports doesn't match, then we can't match this edge
		if len(edge.Hostports) != len(hostports) {
			continue
		}

		// if the edge has all hostports, then it is the one we want. It might have
		// additional hostports.
		if r.edgeHasAllHostports(edge, hostports) {
			return edge, nil
		}
	}

	return nil, iter.Err()
}

func (r *HTTPSEdgeReconciler) edgeHasAllHostports(edge *ngrok.HTTPSEdge, hostports []string) bool {
	edgeHostportMap := make(map[string]bool)
	for _, hostport := range edge.Hostports {
		edgeHostportMap[hostport] = true
	}

	for _, hostport := range hostports {
		if _, ok := edgeHostportMap[hostport]; !ok {
			return false
		}
	}

	return true
}

func (r *HTTPSEdgeReconciler) updateStatus(ctx context.Context, edge *ingressv1alpha1.HTTPSEdge, remoteEdge *ngrok.HTTPSEdge) error {
	edge.Status.ID = remoteEdge.ID
	edge.Status.URI = remoteEdge.URI
	edge.Status.Routes = make([]ingressv1alpha1.HTTPSEdgeRouteStatus, len(remoteEdge.Routes))
	for i, route := range remoteEdge.Routes {
		edge.Status.Routes[i] = ingressv1alpha1.HTTPSEdgeRouteStatus{
			ID:        route.ID,
			URI:       route.URI,
			Match:     route.Match,
			MatchType: route.MatchType,
		}

		if route.Backend != nil {
			edge.Status.Routes[i].Backend = ingressv1alpha1.TunnelGroupBackendStatus{
				ID: route.Backend.Backend.ID,
			}
		}
	}

	return r.Status().Update(ctx, edge)
}

// getMatchingRouteFromEdgeStatus returns the route status for the given ingressv1alpha1.HTTPSEdgeRouteSpec. If there is
// no match in the ingressv1alpha1.HTTPSEdge.Status.Routes, then nil is returned.
func (r *HTTPSEdgeReconciler) getMatchingRouteFromEdgeStatus(edge *ingressv1alpha1.HTTPSEdge, route ingressv1alpha1.HTTPSEdgeRouteSpec) *ingressv1alpha1.HTTPSEdgeRouteStatus {
	for _, routeStatus := range edge.Status.Routes {
		if route.MatchType == routeStatus.MatchType && route.Match == routeStatus.Match {
			return &routeStatus
		}
	}
	return nil
}

// Tunnel Group Backend planner
type tunnelGroupBackendReconciler struct {
	client   *tunnel_group.Client
	backends []*ngrok.TunnelGroupBackend
}

func newTunnelGroupBackendReconciler(client *tunnel_group.Client) (*tunnelGroupBackendReconciler, error) {
	backends := make([]*ngrok.TunnelGroupBackend, 0)
	iter := client.List(&ngrok.Paging{})
	for iter.Next(context.Background()) {
		backends = append(backends, iter.Item())
	}

	return &tunnelGroupBackendReconciler{
		client:   client,
		backends: backends,
	}, iter.Err()
}

func (r *tunnelGroupBackendReconciler) findOrCreate(ctx context.Context, backend ingressv1alpha1.TunnelGroupBackend) (*ngrok.TunnelGroupBackend, error) {
	for _, b := range r.backends {
		// The labels match, so we can use this backend
		if reflect.DeepEqual(b.Labels, backend.Labels) {
			return b, nil
		}
	}

	be, err := r.client.Create(ctx, &ngrok.TunnelGroupBackendCreate{
		Description: backend.Description,
		Metadata:    backend.Metadata,
		Labels:      backend.Labels,
	})
	if err != nil {
		return nil, err
	}
	r.backends = append(r.backends, be)
	return be, nil
}
