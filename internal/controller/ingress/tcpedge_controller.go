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

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/controller/controllers"
	"github.com/ngrok/kubernetes-ingress-controller/internal/ngrokapi"
	"github.com/ngrok/ngrok-api-go/v5"
)

// TCPEdgeReconciler reconciles a TCPEdge object
type TCPEdgeReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controllers.IpPolicyResolver

	NgrokClientset ngrokapi.Clientset

	controller *baseController[*ingressv1alpha1.TCPEdge]
}

// SetupWithManager sets up the controller with the Manager.
func (r *TCPEdgeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.IpPolicyResolver = controllers.IpPolicyResolver{Client: mgr.GetClient()}

	r.controller = &baseController[*ingressv1alpha1.TCPEdge]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		kubeType: "v1alpha1.TCPEdge",
		statusID: func(cr *ingressv1alpha1.TCPEdge) string { return cr.Status.ID },
		create:   r.create,
		update:   r.update,
		delete:   r.delete,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.TCPEdge{}).
		Watches(
			&ingressv1alpha1.IPPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.listTCPEdgesForIPPolicy),
		).
		Complete(r)
}

//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tcpedges,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tcpedges/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tcpedges/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *TCPEdgeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.reconcile(ctx, req, new(ingressv1alpha1.TCPEdge))
}

func (r *TCPEdgeReconciler) create(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	if err := r.reconcileTunnelGroupBackend(ctx, edge); err != nil {
		return err
	}

	if err := r.reserveAddrIfEmpty(ctx, edge); err != nil {
		return err
	}

	// Try to find the edge by the backend labels
	resp, err := r.findEdgeByBackendLabels(ctx, edge.Spec.Backend.Labels)
	if err != nil {
		return err
	}

	if resp != nil {
		return r.updateEdge(ctx, edge, resp)
	}

	// No edge has been created for this edge, create one
	r.Log.Info("Creating new TCPEdge", "namespace", edge.Namespace, "name", edge.Name)
	resp, err = r.NgrokClientset.TCPEdges().Create(ctx, &ngrok.TCPEdgeCreate{
		Description: edge.Spec.Description,
		Metadata:    edge.Spec.Metadata,
		Backend: &ngrok.EndpointBackendMutate{
			BackendID: edge.Status.Backend.ID,
		},
	})
	if err != nil {
		return err
	}
	r.Log.Info("Created new TCPEdge", "edge.ID", resp.ID, "name", edge.Name, "namespace", edge.Namespace)

	return r.updateEdge(ctx, edge, resp)
}

func (r *TCPEdgeReconciler) update(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	if err := r.reconcileTunnelGroupBackend(ctx, edge); err != nil {
		return err
	}

	if err := r.reserveAddrIfEmpty(ctx, edge); err != nil {
		return err
	}

	resp, err := r.NgrokClientset.TCPEdges().Get(ctx, edge.Status.ID)
	if err != nil {
		// If we can't find the edge in the ngrok API, it's been deleted, so clear the ID
		// and requeue the edge. When it gets reconciled again, it will be recreated.
		if ngrok.IsNotFound(err) {
			r.Log.Info("TCPEdge not found, clearing ID and requeuing", "edge.ID", edge.Status.ID)
			edge.Status.ID = ""
			//nolint:errcheck
			r.Status().Update(ctx, edge)
		}
		return err
	}

	// If the backend or hostports do not match, update the edge with the desired backend and hostports
	if resp.Backend.Backend.ID != edge.Status.Backend.ID ||
		!slices.Equal(resp.Hostports, edge.Status.Hostports) {
		resp, err = r.NgrokClientset.TCPEdges().Update(ctx, &ngrok.TCPEdgeUpdate{
			ID:          resp.ID,
			Description: ptr.To(edge.Spec.Description),
			Metadata:    ptr.To(edge.Spec.Metadata),
			Hostports:   edge.Status.Hostports,
			Backend: &ngrok.EndpointBackendMutate{
				BackendID: edge.Status.Backend.ID,
			},
		})
		if err != nil {
			return err
		}
	}

	return r.updateEdge(ctx, edge, resp)
}

func (r *TCPEdgeReconciler) delete(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	err := r.NgrokClientset.TCPEdges().Delete(ctx, edge.Status.ID)
	if err == nil || ngrok.IsNotFound(err) {
		edge.Status.ID = ""
	}
	return err
}

func (r *TCPEdgeReconciler) reconcileTunnelGroupBackend(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	specBackend := edge.Spec.Backend
	// First make sure the tunnel group backend matches
	if edge.Status.Backend.ID != "" {
		// A backend has already been created for this edge, make sure the labels match
		backend, err := r.NgrokClientset.TunnelGroupBackends().Get(ctx, edge.Status.Backend.ID)
		if err != nil {
			if ngrok.IsNotFound(err) {
				r.Log.Info("TunnelGroupBackend not found, clearing ID and requeuing", "TunnelGroupBackend.ID", edge.Status.Backend.ID)
				edge.Status.Backend.ID = ""
				//nolint:errcheck
				r.Status().Update(ctx, edge)
			}
			return err
		}

		// If the labels don't match, update the backend with the desired labels
		if !maps.Equal(backend.Labels, specBackend.Labels) {
			_, err = r.NgrokClientset.TunnelGroupBackends().Update(ctx, &ngrok.TunnelGroupBackendUpdate{
				ID:          backend.ID,
				Metadata:    ptr.To(specBackend.Metadata),
				Description: ptr.To(specBackend.Description),
				Labels:      specBackend.Labels,
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	// No backend has been created for this edge, create one
	backend, err := r.NgrokClientset.TunnelGroupBackends().Create(ctx, &ngrok.TunnelGroupBackendCreate{
		Metadata:    edge.Spec.Backend.Metadata,
		Description: edge.Spec.Backend.Description,
		Labels:      edge.Spec.Backend.Labels,
	})
	if err != nil {
		return err
	}
	edge.Status.Backend.ID = backend.ID

	return r.Status().Update(ctx, edge)
}

func (r *TCPEdgeReconciler) findEdgeByBackendLabels(ctx context.Context, backendLabels map[string]string) (*ngrok.TCPEdge, error) {
	r.Log.Info("Searching for existing TCPEdge with backend labels", "labels", backendLabels)
	iter := r.NgrokClientset.TCPEdges().List(&ngrok.Paging{})
	for iter.Next(ctx) {
		edge := iter.Item()
		if edge.Backend == nil {
			continue
		}

		backend, err := r.NgrokClientset.TunnelGroupBackends().Get(ctx, edge.Backend.Backend.ID)
		if err != nil {
			// If we get an error looking up the backend, return the error and
			// hopefully the next reconcile will fix it.
			return nil, err
		}
		if backend == nil {
			continue
		}

		if maps.Equal(backend.Labels, backendLabels) {
			r.Log.Info("Found existing TCPEdge with matching backend labels", "labels", backendLabels, "edge.ID", edge.ID)
			return edge, nil
		}
	}
	return nil, iter.Err()
}

// Update the edge status and modules, called from both create and update.
func (r *TCPEdgeReconciler) updateEdge(ctx context.Context, edge *ingressv1alpha1.TCPEdge, remoteEdge *ngrok.TCPEdge) error {
	if err := r.updateEdgeStatus(ctx, edge, remoteEdge); err != nil {
		return err
	}

	if err := r.updateIPRestrictionModule(ctx, edge, remoteEdge); err != nil {
		return err
	}

	if err := r.updatePolicyModule(ctx, edge, remoteEdge); err != nil {
		return err
	}

	return nil
}

func (r *TCPEdgeReconciler) updateEdgeStatus(ctx context.Context, edge *ingressv1alpha1.TCPEdge, remoteEdge *ngrok.TCPEdge) error {
	edge.Status.ID = remoteEdge.ID
	edge.Status.URI = remoteEdge.URI
	edge.Status.Hostports = remoteEdge.Hostports
	edge.Status.Backend.ID = remoteEdge.Backend.Backend.ID

	return r.Status().Update(ctx, edge)
}

func (r *TCPEdgeReconciler) reserveAddrIfEmpty(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	if edge.Status.Hostports == nil || len(edge.Status.Hostports) == 0 {
		addr, err := r.findAddrWithMatchingMetadata(ctx, r.metadataForEdge(edge))
		if err != nil {
			return err
		}

		// If we found an addr with matching metadata, use it
		if addr != nil {
			edge.Status.Hostports = []string{addr.Addr}
			return r.Status().Update(ctx, edge)
		}

		// No hostports have been assigned to this edge, assign one
		addr, err = r.NgrokClientset.TCPAddresses().Create(ctx, &ngrok.ReservedAddrCreate{
			Description: r.descriptionForEdge(edge),
			Metadata:    r.metadataForEdge(edge),
		})
		if err != nil {
			return err
		}

		edge.Status.Hostports = []string{addr.Addr}
		return r.Status().Update(ctx, edge)
	}
	return nil
}

func (r *TCPEdgeReconciler) findAddrWithMatchingMetadata(ctx context.Context, metadata string) (*ngrok.ReservedAddr, error) {
	iter := r.NgrokClientset.TCPAddresses().List(&ngrok.Paging{})
	for iter.Next(ctx) {
		addr := iter.Item()
		if addr.Metadata == metadata {
			return addr, nil
		}
	}
	return nil, iter.Err()
}

func (r *TCPEdgeReconciler) metadataForEdge(edge *ingressv1alpha1.TCPEdge) string {
	return fmt.Sprintf(`{"namespace": "%s", "name": "%s"}`, edge.Namespace, edge.Name)
}

func (r *TCPEdgeReconciler) descriptionForEdge(edge *ingressv1alpha1.TCPEdge) string {
	return fmt.Sprintf("Reserved for %s/%s", edge.Namespace, edge.Name)
}

func (r *TCPEdgeReconciler) updateIPRestrictionModule(ctx context.Context, edge *ingressv1alpha1.TCPEdge, remoteEdge *ngrok.TCPEdge) error {
	if edge.Spec.IPRestriction == nil || len(edge.Spec.IPRestriction.IPPolicies) == 0 {
		return r.NgrokClientset.EdgeModules().TCP().IPRestriction().Delete(ctx, edge.Status.ID)
	}
	policyIds, err := r.IpPolicyResolver.ResolveIPPolicyNamesorIds(ctx, edge.Namespace, edge.Spec.IPRestriction.IPPolicies)
	if err != nil {
		return err
	}
	r.Log.Info("Resolved IP Policy NamesOrIDs to IDs", "policyIds", policyIds)

	_, err = r.NgrokClientset.EdgeModules().TCP().IPRestriction().Replace(ctx, &ngrok.EdgeIPRestrictionReplace{
		ID: edge.Status.ID,
		Module: ngrok.EndpointIPPolicyMutate{
			IPPolicyIDs: policyIds,
		},
	})
	return err
}

func (r *TCPEdgeReconciler) listTCPEdgesForIPPolicy(ctx context.Context, obj client.Object) []reconcile.Request {
	r.Log.Info("Listing TCPEdges for ip policy to determine if they need to be reconciled")
	policy, ok := obj.(*ingressv1alpha1.IPPolicy)
	if !ok {
		r.Log.Error(nil, "failed to convert object to IPPolicy", "object", obj)
		return []reconcile.Request{}
	}

	edges := &ingressv1alpha1.TCPEdgeList{}
	if err := r.Client.List(ctx, edges); err != nil {
		r.Log.Error(err, "failed to list TCPEdges for ippolicy", "name", policy.Name, "namespace", policy.Namespace)
		return []reconcile.Request{}
	}

	recs := []reconcile.Request{}

	for _, edge := range edges.Items {
		if edge.Spec.IPRestriction == nil {
			continue
		}
		for _, edgePolicyID := range edge.Spec.IPRestriction.IPPolicies {
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

	r.Log.Info("IPPolicy change triggered TCPEdge reconciliation", "count", len(recs), "policy", policy.Name, "namespace", policy.Namespace)
	return recs
}

func (r *TCPEdgeReconciler) updatePolicyModule(ctx context.Context, edge *ingressv1alpha1.TCPEdge, remoteEdge *ngrok.TCPEdge) error {
	policy := edge.Spec.Policy
	client := r.NgrokClientset.EdgeModules().TCP().RawPolicy()

	// Early return if nothing to be done
	if policy == nil {
		if remoteEdge.Policy == nil {
			r.Log.Info("Module matches desired state, skipping update", "module", "Policy", "comparison", routeModuleComparisonBothNil)

			return nil
		}

		r.Log.Info("Deleting Policy module")
		return client.Delete(ctx, edge.Status.ID)
	}

	r.Log.Info("Updating Policy module")
	_, err := client.Replace(ctx, &ngrokapi.EdgeRawTCPPolicyReplace{
		ID:     remoteEdge.ID,
		Module: policy,
	})

	return err
}
