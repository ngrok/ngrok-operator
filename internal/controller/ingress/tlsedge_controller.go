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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ierr "github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"github.com/ngrok/kubernetes-ingress-controller/internal/ngrokapi"
	"github.com/ngrok/ngrok-api-go/v5"
)

// TLSEdgeReconciler reconciles a TLSEdge object
type TLSEdgeReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controllers.IpPolicyResolver

	NgrokClientset ngrokapi.Clientset

	controller *baseController[*ingressv1alpha1.TLSEdge]
}

// SetupWithManager sets up the controller with the Manager.
func (r *TLSEdgeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.IpPolicyResolver = controllers.IpPolicyResolver{Client: mgr.GetClient()}

	r.controller = &baseController[*ingressv1alpha1.TLSEdge]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		kubeType: "v1alpha1.TLSEdge",
		statusID: func(cr *ingressv1alpha1.TLSEdge) string { return cr.Status.ID },
		create:   r.create,
		update:   r.update,
		delete:   r.delete,
		errResult: func(op baseControllerOp, cr *ingressv1alpha1.TLSEdge, err error) (ctrl.Result, error) {
			if errors.As(err, &ierr.ErrInvalidConfiguration{}) {
				return ctrl.Result{}, nil
			}
			if ngrok.IsErrorCode(err, 7117) { // https://ngrok.com/docs/errors/err_ngrok_7117, domain not found
				return ctrl.Result{}, err
			}
			return reconcileResultFromError(err)
		},
	}

	controller := ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.TLSEdge{}).
		Watches(
			&ingressv1alpha1.IPPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.listTLSEdgesForIPPolicy),
		).
		Watches(
			&ingressv1alpha1.Domain{},
			handler.EnqueueRequestsFromMapFunc(r.listTLSEdgesForDomain),
		)

	return controller.Complete(r)
}

//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tlsedges,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tlsedges/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tlsedges/finalizers,verbs=update
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains,verbs=get;list;watch;create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *TLSEdgeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.reconcile(ctx, req, new(ingressv1alpha1.TLSEdge))
}

func (r *TLSEdgeReconciler) create(ctx context.Context, edge *ingressv1alpha1.TLSEdge) error {
	if err := r.reconcileDomains(ctx, edge); err != nil {
		return err
	}

	if err := r.reconcileTunnelGroupBackend(ctx, edge); err != nil {
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
	r.Log.Info("Creating new TLSEdge", "namespace", edge.Namespace, "name", edge.Name)
	resp, err = r.NgrokClientset.TLSEdges().Create(ctx, &ngrok.TLSEdgeCreate{
		Hostports:   edge.Spec.Hostports,
		Description: edge.Spec.Description,
		Metadata:    edge.Spec.Metadata,
		Backend: &ngrok.EndpointBackendMutate{
			BackendID: edge.Status.Backend.ID,
		},
	})
	if err != nil {
		return err
	}
	r.Log.Info("Created new TLSEdge", "edge.ID", resp.ID, "name", edge.Name, "namespace", edge.Namespace)

	return r.updateEdge(ctx, edge, resp)
}

func (r *TLSEdgeReconciler) update(ctx context.Context, edge *ingressv1alpha1.TLSEdge) error {
	if err := r.reconcileDomains(ctx, edge); err != nil {
		return err
	}

	if err := r.reconcileTunnelGroupBackend(ctx, edge); err != nil {
		return err
	}

	resp, err := r.NgrokClientset.TLSEdges().Get(ctx, edge.Status.ID)
	if err != nil {
		// If we can't find the edge in the ngrok API, it's been deleted, so clear the ID
		// and requeue the edge. When it gets reconciled again, it will be recreated.
		if ngrok.IsNotFound(err) {
			r.Log.Info("TLSEdge not found, clearing ID and requeuing", "edge.ID", edge.Status.ID)
			edge.Status.ID = ""
			//nolint:errcheck
			r.Status().Update(ctx, edge)
		}
		return err
	}

	// If the backend or hostports do not match, update the edge with the desired backend and hostports
	if resp.Backend.Backend.ID != edge.Status.Backend.ID ||
		!slices.Equal(resp.Hostports, edge.Status.Hostports) {
		resp, err = r.NgrokClientset.TLSEdges().Update(ctx, &ngrok.TLSEdgeUpdate{
			ID:          resp.ID,
			Description: ptr.To(edge.Spec.Description),
			Metadata:    ptr.To(edge.Spec.Metadata),
			Hostports:   edge.Spec.Hostports,
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

// Update the edge status and modules, called from both create and update.
func (r *TLSEdgeReconciler) updateEdge(ctx context.Context, edge *ingressv1alpha1.TLSEdge, resp *ngrok.TLSEdge) error {
	if err := r.updateEdgeStatus(ctx, edge, resp); err != nil {
		return err
	}

	if err := r.setTLSTermination(ctx, resp, edge.Spec.TLSTermination); err != nil {
		return err
	}

	if err := r.setMutualTLS(ctx, resp, edge.Spec.MutualTLS); err != nil {
		return err
	}

	if err := r.updateIPRestrictionModule(ctx, edge, resp); err != nil {
		return err
	}

	if err := r.updatePolicyModule(ctx, edge, resp); err != nil {
		return err
	}

	return nil
}

func (r *TLSEdgeReconciler) delete(ctx context.Context, edge *ingressv1alpha1.TLSEdge) error {
	log := ctrl.LoggerFrom(ctx)

	edgeID := edge.Status.ID
	log.Info("Deleting TLSEdge", "edge.ID", edgeID)
	err := r.NgrokClientset.TLSEdges().Delete(ctx, edgeID)
	if err == nil || ngrok.IsNotFound(err) {
		log.Info("Deleted TLSEdge", "edge.ID", edgeID)
		edge.Status.ID = ""
		if err := r.Client.Status().Update(ctx, edge); err != nil {
			return err
		}
	}

	return err
}

func (r *TLSEdgeReconciler) reconcileTunnelGroupBackend(ctx context.Context, edge *ingressv1alpha1.TLSEdge) error {
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

func (r *TLSEdgeReconciler) setMutualTLS(ctx context.Context, edge *ngrok.TLSEdge, mutualTls *ingressv1alpha1.EndpointMutualTLS) error {
	log := ctrl.LoggerFrom(ctx)

	client := r.NgrokClientset.EdgeModules().TLS().MutualTLS()
	if mutualTls == nil {
		if edge.MutualTls == nil {
			log.V(1).Info("Edge Mutual TLS matches spec")
			return nil
		}

		log.Info("Deleting Edge Mutual TLS")
		return client.Delete(ctx, edge.ID)
	}

	_, err := client.Replace(ctx, &ngrok.EdgeMutualTLSReplace{
		ID: edge.ID,
		Module: ngrok.EndpointMutualTLSMutate{
			CertificateAuthorityIDs: mutualTls.CertificateAuthorities,
		},
	})
	return err
}

func (r *TLSEdgeReconciler) setTLSTermination(ctx context.Context, edge *ngrok.TLSEdge, tlsTermination *ingressv1alpha1.EndpointTLSTermination) error {
	log := ctrl.LoggerFrom(ctx)

	client := r.NgrokClientset.EdgeModules().TLS().TLSTermination()
	if tlsTermination == nil {
		if edge.TlsTermination == nil {
			log.V(1).Info("Edge TLS termination matches spec")
			return nil
		}

		log.Info("Deleting Edge TLS termination")
		return client.Delete(ctx, edge.ID)
	}

	_, err := client.Replace(ctx, &ngrok.EdgeTLSTerminationReplace{
		ID: edge.ID,
		Module: ngrok.EndpointTLSTermination{
			TerminateAt: tlsTermination.TerminateAt,
			MinVersion:  tlsTermination.MinVersion,
		},
	})
	return err
}

func (r *TLSEdgeReconciler) findEdgeByBackendLabels(ctx context.Context, backendLabels map[string]string) (*ngrok.TLSEdge, error) {
	r.Log.Info("Searching for existing TLSEdge with backend labels", "labels", backendLabels)
	iter := r.NgrokClientset.TLSEdges().List(&ngrok.Paging{})
	for iter.Next(ctx) {
		edge := iter.Item()
		if edge.Backend == nil {
			continue
		}

		backend, err := r.NgrokClientset.TunnelGroupBackends().Get(ctx, edge.Backend.Backend.ID)
		if err != nil {
			// The backend ID on the edge is invalid and no longer exists in the ngrok API,
			// so we'll skip this edge check the next one.
			if ngrok.IsNotFound(err) {
				continue
			}

			// We've go an error besides not found. Return the error and
			// hopefully the next reconcile will fix it.
			return nil, err
		}
		if backend == nil {
			continue
		}

		if maps.Equal(backend.Labels, backendLabels) {
			r.Log.Info("Found existing TLSEdge with matching backend labels", "labels", backendLabels, "edge.ID", edge.ID)
			return edge, nil
		}
	}
	return nil, iter.Err()
}

func (r *TLSEdgeReconciler) updateEdgeStatus(ctx context.Context, edge *ingressv1alpha1.TLSEdge, remoteEdge *ngrok.TLSEdge) error {
	domains := &ingressv1alpha1.DomainList{}
	if err := r.Client.List(ctx, domains, client.InNamespace(edge.Namespace)); err != nil {
		return err
	}

	edgeDomainMap := make(map[string]bool)
	for _, hp := range remoteEdge.Hostports {
		host, _, err := parseHostAndPort(hp)
		if err != nil {
			return err
		}
		edgeDomainMap[host] = true
	}

	edge.Status.CNAMETargets = map[string]string{}
	for _, domain := range domains.Items {
		// We don't care about domains that aren't part of this edge
		if _, ok := edgeDomainMap[domain.Spec.Domain]; !ok {
			continue
		}

		if domain.Status.CNAMETarget != nil {
			edge.Status.CNAMETargets[domain.Spec.Domain] = *domain.Status.CNAMETarget
		}
	}
	edge.Status.ID = remoteEdge.ID
	edge.Status.URI = remoteEdge.URI
	edge.Status.Hostports = remoteEdge.Hostports
	edge.Status.Backend.ID = remoteEdge.Backend.Backend.ID

	return r.Status().Update(ctx, edge)
}

func (r *TLSEdgeReconciler) updateIPRestrictionModule(ctx context.Context, edge *ingressv1alpha1.TLSEdge, _ *ngrok.TLSEdge) error {
	if edge.Spec.IPRestriction == nil || len(edge.Spec.IPRestriction.IPPolicies) == 0 {
		return r.NgrokClientset.EdgeModules().TLS().IPRestriction().Delete(ctx, edge.Status.ID)
	}
	policyIds, err := r.IpPolicyResolver.ResolveIPPolicyNamesorIds(ctx, edge.Namespace, edge.Spec.IPRestriction.IPPolicies)
	if err != nil {
		return err
	}
	r.Log.Info("Resolved IP Policy NamesOrIDs to IDs", "policyIds", policyIds)

	_, err = r.NgrokClientset.EdgeModules().TLS().IPRestriction().Replace(ctx, &ngrok.EdgeIPRestrictionReplace{
		ID: edge.Status.ID,
		Module: ngrok.EndpointIPPolicyMutate{
			IPPolicyIDs: policyIds,
		},
	})
	return err
}

func (r *TLSEdgeReconciler) listTLSEdgesForIPPolicy(ctx context.Context, obj client.Object) []reconcile.Request {
	r.Log.Info("Listing TLSEdges for ip policy to determine if they need to be reconciled")
	policy, ok := obj.(*ingressv1alpha1.IPPolicy)
	if !ok {
		r.Log.Error(nil, "failed to convert object to IPPolicy", "object", obj)
		return []reconcile.Request{}
	}

	edges := &ingressv1alpha1.TLSEdgeList{}
	if err := r.Client.List(ctx, edges); err != nil {
		r.Log.Error(err, "failed to list TLSEdges for ippolicy", "name", policy.Name, "namespace", policy.Namespace)
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

	r.Log.Info("IPPolicy change triggered TLSEdge reconciliation", "count", len(recs), "policy", policy.Name, "namespace", policy.Namespace)
	return recs
}

func (r *TLSEdgeReconciler) listTLSEdgesForDomain(ctx context.Context, obj client.Object) []reconcile.Request {
	r.Log.Info("Listing TLSEdges for domain to determine if they need to be reconciled")
	domain, ok := obj.(*ingressv1alpha1.Domain)
	if !ok {
		r.Log.Error(nil, "failed to convert object to Domain", "object", obj)
		return []reconcile.Request{}
	}

	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain.Name, "namespace", domain.Namespace)

	edges := &ingressv1alpha1.TLSEdgeList{}
	if err := r.Client.List(ctx, edges); err != nil {
		log.Error(err, "failed to list TLSEdges for domain")
		return []reconcile.Request{}
	}

	recs := []reconcile.Request{}

	for _, edge := range edges.Items {
		for _, hostport := range edge.Spec.Hostports {
			host, _, err := parseHostAndPort(hostport)
			if err != nil {
				log.Error(err, "failed to parse host and port", "hostport", hostport)
				continue
			}
			if host == domain.Spec.Domain {
				log.V(1).Info("Found edge with matching hostport to reconcile for domain change", "edge", edge.Name)
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

	log.V(1).Info("Domain change triggered TLSEdge reconciliation", "reconcile_requests", recs)
	return recs
}

func (r *TLSEdgeReconciler) updatePolicyModule(ctx context.Context, edge *ingressv1alpha1.TLSEdge, remoteEdge *ngrok.TLSEdge) error {
	policy := edge.Spec.Policy
	client := r.NgrokClientset.EdgeModules().TLS().RawPolicy()

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
	_, err := client.Replace(ctx, &ngrokapi.EdgeRawTLSPolicyReplace{
		ID:     remoteEdge.ID,
		Module: policy,
	})
	return err
}

func (r *TLSEdgeReconciler) reconcileDomains(ctx context.Context, edge *ingressv1alpha1.TLSEdge) error {
	log := ctrl.LoggerFrom(ctx)

	existing := make(map[string]bool)
	domainList := &ingressv1alpha1.DomainList{}
	if err := r.Client.List(ctx, domainList, client.InNamespace(edge.Namespace)); err != nil {
		return err
	}

	for _, domain := range domainList.Items {
		existing[domain.Spec.Domain] = true
	}

	// Get the desired domains
	desiredDomains, err := r.getDesiredDomains(ctx, edge)
	if err != nil {
		return err
	}

	for _, domain := range desiredDomains {
		// Already exists, skip
		if _, ok := existing[domain.Spec.Domain]; ok {
			continue
		}

		// Doesn't exist, create
		log.Info("Creating domain", "name", domain.Name, "namespace", domain.Namespace)
		if err := r.Client.Create(ctx, &domain); err != nil {
			return err
		}
	}
	return nil
}

func (r *TLSEdgeReconciler) getDesiredDomains(ctx context.Context, edge *ingressv1alpha1.TLSEdge) ([]ingressv1alpha1.Domain, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Calculating desired domains")
	desired := []ingressv1alpha1.Domain{}
	for _, hostport := range edge.Spec.Hostports {
		host, _, err := parseHostAndPort(hostport)
		if err != nil {
			return nil, err
		}

		domain := ingressv1alpha1.Domain{
			ObjectMeta: metav1.ObjectMeta{
				Name:        strings.Replace(host, ".", "-", -1),
				Namespace:   edge.Namespace,
				Annotations: map[string]string{},
			},
			Spec: ingressv1alpha1.DomainSpec{
				Domain: host,
			},
		}
		desired = append(desired, domain)
	}
	return desired, nil
}

// parses the ngrok hostport string into a hostname and port
func parseHostAndPort(hostport string) (string, int32, error) {
	pieces := strings.SplitN(hostport, ":", 2)
	if len(pieces) != 2 {
		return "", 0, fmt.Errorf("invalid hostport")
	}

	port, err := strconv.ParseInt(pieces[1], 10, 32)
	if err != nil {
		return "", 0, err
	}

	return pieces[0], int32(port), nil
}
