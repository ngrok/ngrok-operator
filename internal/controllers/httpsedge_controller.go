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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"github.com/ngrok/kubernetes-ingress-controller/internal/ngrokapi"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/backends/tunnel_group"
)

// HTTPSEdgeReconciler reconciles a HTTPSEdge object
type HTTPSEdgeReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	NgrokClientset ngrokapi.Clientset
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
				if err := r.NgrokClientset.HTTPSEdges().Delete(ctx, edge.Status.ID); err != nil {
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
		remoteEdge, err = r.NgrokClientset.HTTPSEdges().Get(ctx, edge.Status.ID)
		if err != nil {
			return err
		}

		if !edge.Equal(remoteEdge) {
			remoteEdge, err = r.NgrokClientset.HTTPSEdges().Update(ctx, &ngrok.HTTPSEdgeUpdate{
				ID:          edge.Status.ID,
				Metadata:    &edge.Spec.Metadata,
				Description: &edge.Spec.Description,
				Hostports:   edge.Spec.Hostports,
			})
			if err != nil {
				return err
			}
		}
	} else {
		// Try to find the edge by Hostports
		remoteEdge, err = r.findEdgeByHostports(ctx, edge.Spec.Hostports)
		if err != nil {
			return err
		}

		// Not found, so create it
		if remoteEdge == nil {
			remoteEdge, err = r.NgrokClientset.HTTPSEdges().Create(ctx, &ngrok.HTTPSEdgeCreate{
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

	if err = r.reconcileRoutes(ctx, edge, remoteEdge); err != nil {
		return err
	}

	return r.setEdgeTLSTermination(ctx, remoteEdge, edge.Spec.TLSTermination)
}

// TODO: This is going to be a bit messy right now, come back and make this cleaner
func (r *HTTPSEdgeReconciler) reconcileRoutes(ctx context.Context, edge *ingressv1alpha1.HTTPSEdge, remoteEdge *ngrok.HTTPSEdge) error {
	routeStatuses := make([]ingressv1alpha1.HTTPSEdgeRouteStatus, len(edge.Spec.Routes))
	tunnelGroupReconciler, err := newTunnelGroupBackendReconciler(r.NgrokClientset.TunnelGroupBackends())
	if err != nil {
		return err
	}

	routeModuleUpdater := &edgeRouteModuleUpdater{
		log:              r.Log,
		edge:             edge,
		clientset:        r.NgrokClientset.EdgeModules().HTTPS().Routes(),
		ipPolicyResolver: ipPolicyResolver{r.Client},
		secretResolver:   secretResolver{r.Client},
	}

	// TODO: clean this up. This is way too much nesting
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
			route, err = r.NgrokClientset.HTTPSEdgeRoutes().Create(ctx, req)
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
			route, err = r.NgrokClientset.HTTPSEdgeRoutes().Update(ctx, req)
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

		// Update all route modules for a given route
		if err := routeModuleUpdater.updateModulesForRoute(ctx, route, &routeSpec); err != nil {
			return err
		}
	}

	// Delete any routes that are no longer in the spec
	for _, remoteRoute := range remoteEdge.Routes {
		found := false
		for _, routeStatus := range routeStatuses {
			if routeStatus.ID == remoteRoute.ID {
				found = true
				break
			}
		}
		if !found {
			r.Log.Info("Deleting route", "edgeID", edge.Status.ID, "routeID", remoteRoute.ID)
			if err := r.NgrokClientset.HTTPSEdgeRoutes().Delete(ctx, &ngrok.EdgeRouteItem{EdgeID: edge.Status.ID, ID: remoteRoute.ID}); err != nil {
				return err
			}
		}
	}

	edge.Status.Routes = routeStatuses

	return r.Status().Update(ctx, edge)
}

func (r *HTTPSEdgeReconciler) setEdgeTLSTermination(ctx context.Context, edge *ngrok.HTTPSEdge, tlsTermination *ingressv1alpha1.EndpointTLSTerminationAtEdge) error {
	client := r.NgrokClientset.EdgeModules().HTTPS().TLSTermination()
	if tlsTermination == nil {
		return client.Delete(ctx, edge.ID)
	}

	_, err := client.Replace(ctx, &ngrok.EdgeTLSTerminationAtEdgeReplace{
		ID: edge.ID,
		Module: ngrok.EndpointTLSTerminationAtEdge{
			MinVersion: pointer.String(tlsTermination.MinVersion),
		},
	})
	return err
}

func (r *HTTPSEdgeReconciler) findEdgeByHostports(ctx context.Context, hostports []string) (*ngrok.HTTPSEdge, error) {
	iter := r.NgrokClientset.HTTPSEdges().List(&ngrok.Paging{})
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

func (r *HTTPSEdgeReconciler) listHTTPSEdgesForIPPolicy(obj client.Object) []reconcile.Request {
	r.Log.Info("Listing HTTPSEdges for ip policy to determine if they need to be reconciled")
	policy, ok := obj.(*ingressv1alpha1.IPPolicy)
	if !ok {
		r.Log.Error(nil, "failed to convert object to IPPolicy", "object", obj)
		return []reconcile.Request{}
	}

	edges := &ingressv1alpha1.HTTPSEdgeList{}
	if err := r.Client.List(context.Background(), edges); err != nil {
		r.Log.Error(err, "failed to list HTTPSEdges for ippolicy", "name", policy.Name, "namespace", policy.Namespace)
		return []reconcile.Request{}
	}

	recs := []reconcile.Request{}

	for _, edge := range edges.Items {
		for _, route := range edge.Spec.Routes {
			if route.IPRestriction == nil {
				continue
			}

			for _, edgePolicyID := range route.IPRestriction.IPPolicies {
				if edgePolicyID == policy.Name || edgePolicyID == policy.Status.ID {
					recs = append(recs, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      edge.GetName(),
							Namespace: edge.GetNamespace(),
						},
					})
					break
				}
			}
		}
	}

	r.Log.Info("IPPolicy change triggered HTTPSEdge reconciliation", "count", len(recs), "policy", policy.Name, "namespace", policy.Namespace)
	return recs
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

type edgeRouteModuleUpdater struct {
	log logr.Logger

	edge *ingressv1alpha1.HTTPSEdge

	clientset ngrokapi.HTTPSEdgeRouteModulesClientset

	ipPolicyResolver ipPolicyResolver
	secretResolver   secretResolver
}

func (u *edgeRouteModuleUpdater) updateModulesForRoute(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	funcs := []func(context.Context, *ngrok.HTTPSEdgeRoute, *ingressv1alpha1.HTTPSEdgeRouteSpec) error{
		u.setEdgeRouteCircuitBreaker,
		u.setEdgeRouteCompression,
		u.setEdgeRouteIPRestriction,
		u.setEdgeRouteRequestHeaders,
		u.setEdgeRouteResponseHeaders,
		u.setEdgeRouteOAuth,
		u.setEdgeRouteOIDC,
		u.setEdgeRouteSAML,
		u.setEdgeRouteWebhookVerification,
	}

	for _, f := range funcs {
		if err := f(ctx, route, routeSpec); err != nil {
			return err
		}
	}
	return nil
}

func (u *edgeRouteModuleUpdater) edgeRouteItem(route *ngrok.HTTPSEdgeRoute) *ngrok.EdgeRouteItem {
	return &ngrok.EdgeRouteItem{
		EdgeID: route.EdgeID,
		ID:     route.ID,
	}
}

func (u *edgeRouteModuleUpdater) setEdgeRouteCircuitBreaker(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	circuitBreaker := routeSpec.CircuitBreaker

	client := u.clientset.CircuitBreaker()

	// Early return if nothing to be done
	if circuitBreaker == nil {
		if route.CircuitBreaker == nil {
			u.log.Info("CircuitBreaker matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	module := ngrok.EndpointCircuitBreaker{
		TrippedDuration:          uint32(circuitBreaker.TrippedDuration.Seconds()),
		RollingWindow:            uint32(circuitBreaker.RollingWindow.Seconds()),
		NumBuckets:               circuitBreaker.NumBuckets,
		VolumeThreshold:          circuitBreaker.VolumeThreshold,
		ErrorThresholdPercentage: circuitBreaker.ErrorThresholdPercentage.AsApproximateFloat64(),
	}

	if reflect.DeepEqual(module, route.CircuitBreaker) {
		u.log.Info("CircuitBreaker matches desired state, skipping update")
		return nil
	}

	_, err := client.Replace(ctx, &ngrok.EdgeRouteCircuitBreakerReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteCompression(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	compression := routeSpec.Compression

	client := u.clientset.Compression()

	// Early return if nothing to be done
	if compression == nil {
		if route.Compression == nil {
			u.log.Info("Compression matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	_, err := client.Replace(ctx, &ngrok.EdgeRouteCompressionReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: ngrok.EndpointCompression{
			Enabled: pointer.Bool(routeSpec.Compression.Enabled),
		},
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteIPRestriction(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	ipRestriction := routeSpec.IPRestriction
	client := u.clientset.IPRestriction()

	if ipRestriction == nil || len(ipRestriction.IPPolicies) == 0 {
		if route.IpRestriction == nil || len(route.IpRestriction.IPPolicies) == 0 {
			u.log.Info("IP Restriction matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	policyIds, err := u.ipPolicyResolver.resolveIPPolicyNamesorIds(ctx, u.edge.Namespace, ipRestriction.IPPolicies)
	if err != nil {
		return err
	}
	u.log.Info("Resolved IP Policy NamesOrIDs to IDs", "NamesOrIds", ipRestriction.IPPolicies, "policyIds", policyIds)

	var remoteIPPolicies []string
	if route.IpRestriction != nil && len(route.IpRestriction.IPPolicies) > 0 {
		remoteIPPolicies := make([]string, 0, len(route.IpRestriction.IPPolicies))
		for _, policy := range route.IpRestriction.IPPolicies {
			remoteIPPolicies = append(remoteIPPolicies, policy.ID)
		}
	}

	if reflect.DeepEqual(remoteIPPolicies, policyIds) {
		u.log.Info("IP Restriction matches desired state, skipping update")
		return nil
	}

	_, err = client.Replace(ctx, &ngrok.EdgeRouteIPRestrictionReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: ngrok.EndpointIPPolicyMutate{
			IPPolicyIDs: policyIds,
		},
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteRequestHeaders(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	var requestHeaders *ingressv1alpha1.EndpointRequestHeaders
	if routeSpec.Headers != nil {
		requestHeaders = routeSpec.Headers.Request
	}

	client := u.clientset.RequestHeaders()

	if requestHeaders == nil {
		if route.RequestHeaders == nil {
			u.log.Info("Request Headers matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	module := ngrok.EndpointRequestHeaders{}
	if len(requestHeaders.Add) > 0 {
		module.Add = requestHeaders.Add
	}
	if len(requestHeaders.Remove) > 0 {
		module.Remove = requestHeaders.Remove
	}

	if reflect.DeepEqual(&module, route.RequestHeaders) {
		u.log.Info("Request Headers matches desired state, skipping update")
		return nil
	}

	_, err := client.Replace(ctx, &ngrok.EdgeRouteRequestHeadersReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteResponseHeaders(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	var responseHeaders *ingressv1alpha1.EndpointResponseHeaders
	if routeSpec.Headers != nil {
		responseHeaders = routeSpec.Headers.Response
	}

	client := u.clientset.ResponseHeaders()
	if responseHeaders == nil {
		if route.ResponseHeaders == nil {
			u.log.Info("Response Headers matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	module := ngrok.EndpointResponseHeaders{}
	if len(responseHeaders.Add) > 0 {
		module.Add = responseHeaders.Add
	}
	if len(responseHeaders.Remove) > 0 {
		module.Remove = responseHeaders.Remove
	}

	if reflect.DeepEqual(&module, route.ResponseHeaders) {
		u.log.Info("Response Headers matches desired state, skipping update")
		return nil
	}

	_, err := client.Replace(ctx, &ngrok.EdgeRouteResponseHeadersReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteOAuth(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	oauth := routeSpec.OAuth
	oauthClient := u.clientset.OAuth()

	if oauth == nil {
		if route.OAuth == nil {
			u.log.Info("OAuth matches desired state, skipping update")
			return nil
		}

		return oauthClient.Delete(ctx, u.edgeRouteItem(route))
	}

	var module *ngrok.EndpointOAuth
	var err error

	providers := []OAuthProvider{
		oauth.Google,
		oauth.Github,
		oauth.Gitlab,
		oauth.Amazon,
		oauth.Facebook,
		oauth.Microsoft,
		oauth.Twitch,
		oauth.Linkedin,
	}

	for _, p := range providers {
		if p == nil {
			continue
		}

		var secret *string
		secretKeyRef := p.ClientSecretKeyRef()

		// Look up the client secret key if its specified,
		// otherwise default to nil
		if secretKeyRef != nil {
			secret, err = u.getSecret(ctx, *secretKeyRef)
			if client.IgnoreNotFound(err) != nil {
				return err
			}
		}

		module = p.ToNgrok(secret)
		break
	}

	if module == nil {
		return fmt.Errorf("no OAuth provider configured")
	}

	if reflect.DeepEqual(module, route.OAuth) {
		u.log.Info("OAuth matches desired state, skipping update")
		return nil
	}

	_, err = oauthClient.Replace(ctx, &ngrok.EdgeRouteOAuthReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: *module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteOIDC(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	oidc := routeSpec.OIDC
	client := u.clientset.OIDC()

	if oidc == nil {
		if route.OIDC == nil {
			u.log.Info("OIDC matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	clientSecret, err := u.getSecret(ctx, oidc.ClientSecret)
	if err != nil {
		return err
	}
	if clientSecret == nil {
		return errors.NewErrMissingRequiredSecret("missing clientSecret for OIDC")
	}

	module := ngrok.EndpointOIDC{
		OptionsPassthrough: oidc.OptionsPassthrough,
		CookiePrefix:       oidc.CookiePrefix,
		InactivityTimeout:  uint32(oidc.InactivityTimeout.Seconds()),
		MaximumDuration:    uint32(oidc.MaximumDuration.Seconds()),
		Issuer:             oidc.Issuer,
		ClientID:           oidc.ClientID,
		ClientSecret:       *clientSecret,
		Scopes:             oidc.Scopes,
	}

	if reflect.DeepEqual(&module, route.OIDC) {
		u.log.Info("OIDC matches desired state, skipping update")
		return nil
	}

	_, err = client.Replace(ctx, &ngrok.EdgeRouteOIDCReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteSAML(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	saml := routeSpec.SAML
	client := u.clientset.SAML()

	if saml == nil {
		if route.SAML == nil {
			u.log.Info("SAML matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	module := ngrok.EndpointSAMLMutate{
		OptionsPassthrough: saml.OptionsPassthrough,
		CookiePrefix:       saml.CookiePrefix,
		InactivityTimeout:  uint32(saml.InactivityTimeout.Seconds()),
		MaximumDuration:    uint32(saml.MaximumDuration.Seconds()),
		IdPMetadata:        saml.IdPMetadata,
		ForceAuthn:         saml.ForceAuthn,
		AllowIdPInitiated:  saml.AllowIdPInitiated,
		AuthorizedGroups:   saml.AuthorizedGroups,
		NameIDFormat:       saml.NameIDFormat,
	}

	if reflect.DeepEqual(&module, route.SAML) {
		u.log.Info("SAML matches desired state, skipping update")
		return nil
	}

	_, err := client.Replace(ctx, &ngrok.EdgeRouteSAMLReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) setEdgeRouteWebhookVerification(ctx context.Context, route *ngrok.HTTPSEdgeRoute, routeSpec *ingressv1alpha1.HTTPSEdgeRouteSpec) error {
	webhookVerification := routeSpec.WebhookVerification

	client := u.clientset.WebhookVerification()

	if webhookVerification == nil {
		if route.WebhookVerification == nil {
			u.log.Info("Webhook Verification matches desired state, skipping update")
			return nil
		}

		return client.Delete(ctx, u.edgeRouteItem(route))
	}

	// Some WebhookVerification providers don't require a secret,
	// so default to an empty string.
	var webhookSecret = ""

	if webhookVerification.SecretRef != nil {
		s, err := u.getSecret(ctx, *webhookVerification.SecretRef)
		if err != nil {
			return err
		}
		webhookSecret = *s
	}

	module := ngrok.EndpointWebhookValidation{
		Provider: webhookVerification.Provider,
		Secret:   webhookSecret,
	}

	if reflect.DeepEqual(&module, route.WebhookVerification) {
		u.log.Info("Webhook Verification matches desired state, skipping update")
		return nil
	}

	_, err := client.Replace(ctx, &ngrok.EdgeRouteWebhookVerificationReplace{
		EdgeID: route.EdgeID,
		ID:     route.ID,
		Module: module,
	})
	return err
}

func (u *edgeRouteModuleUpdater) getSecret(ctx context.Context, secretRef ingressv1alpha1.SecretKeyRef) (*string, error) {
	secret, err := u.secretResolver.getSecret(ctx,
		u.edge.Namespace,
		secretRef.Name,
		secretRef.Key,
	)
	return &secret, err
}

type OAuthProvider interface {
	ClientSecretKeyRef() *ingressv1alpha1.SecretKeyRef
	ToNgrok(*string) *ngrok.EndpointOAuth
}
