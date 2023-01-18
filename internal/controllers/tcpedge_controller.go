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
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/backends/tunnel_group"
	"github.com/ngrok/ngrok-api-go/v5/edges/tcp"
)

// TCPEdgeReconciler reconciles a TCPEdge object
type TCPEdgeReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	TCPEdgeClient            *tcp.Client
	TunnelGroupBackendClient *tunnel_group.Client
}

// SetupWithManager sets up the controller with the Manager.
func (r *TCPEdgeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.TCPEdge{}).
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
	log := r.Log.WithValues("V1Alpha1TCPEdge", req.NamespacedName)

	edge := new(ingressv1alpha1.TCPEdge)
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
				if err := r.TCPEdgeClient.Delete(ctx, edge.Status.ID); err != nil {
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
		return ctrl.Result{}, nil
	}

	if err := r.reconcileTunnelGroupBackend(ctx, edge); err != nil {
		log.Error(err, "unable to ensure tunnel group backend", err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.reconcileEdge(ctx, edge)
}

func (r *TCPEdgeReconciler) reconcileTunnelGroupBackend(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	specBackend := edge.Spec.Backend
	// First make sure the tunnel group backend matches
	if edge.Status.Backend.ID != "" {
		// A backend has already been created for this edge, make sure the labels match
		backend, err := r.TunnelGroupBackendClient.Get(ctx, edge.Status.Backend.ID)
		if err != nil {
			return err
		}

		// If the labels don't match, update the backend with the desired labels
		if !reflect.DeepEqual(backend.Labels, specBackend.Labels) {
			backend, err = r.TunnelGroupBackendClient.Update(ctx, &ngrok.TunnelGroupBackendUpdate{
				ID:          backend.ID,
				Metadata:    pointer.String(specBackend.Metadata),
				Description: pointer.String(specBackend.Description),
				Labels:      specBackend.Labels,
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	// No backend has been created for this edge, create one
	backend, err := r.TunnelGroupBackendClient.Create(ctx, &ngrok.TunnelGroupBackendCreate{
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

func (r *TCPEdgeReconciler) reconcileEdge(ctx context.Context, edge *ingressv1alpha1.TCPEdge) error {
	if edge.Status.ID != "" {
		// An edge already exists, make sure everything matches
		resp, err := r.TCPEdgeClient.Get(ctx, edge.Status.ID)
		if err != nil {
			return err
		}

		// If the backend doesn't match, update the edge with the desired backend

		if resp.Backend.Backend.ID != edge.Status.Backend.ID {
			resp, err = r.TCPEdgeClient.Update(ctx, &ngrok.TCPEdgeUpdate{
				ID:          resp.ID,
				Description: pointer.String(edge.Spec.Description),
				Metadata:    pointer.String(edge.Spec.Metadata),
				Hostports:   edge.Status.Hostports,
				Backend: &ngrok.EndpointBackendMutate{
					BackendID: edge.Status.Backend.ID,
				},
			})
			if err != nil {
				return err
			}
		}

		edge.Status.ID = resp.ID
		edge.Status.URI = resp.URI
		edge.Status.Hostports = resp.Hostports
		edge.Status.Backend.ID = resp.Backend.Backend.ID
		return r.Status().Update(ctx, edge)
	}

	// No edge has been created for this edge, create one
	resp, err := r.TCPEdgeClient.Create(ctx, &ngrok.TCPEdgeCreate{
		Description: edge.Spec.Description,
		Metadata:    edge.Spec.Metadata,
		Backend: &ngrok.EndpointBackendMutate{
			BackendID: edge.Status.Backend.ID,
		},
	})
	if err != nil {
		return err
	}

	edge.Status.ID = resp.ID
	edge.Status.URI = resp.URI
	edge.Status.Hostports = resp.Hostports
	edge.Status.Backend.ID = resp.Backend.Backend.ID
	return r.Status().Update(ctx, edge)
}

func (r *TCPEdgeReconciler) findEdgeByHostports(ctx context.Context, hostports []string) (*ngrok.TCPEdge, error) {
	iter := r.TCPEdgeClient.List(&ngrok.Paging{})
	for iter.Next(ctx) {
		edge := iter.Item()
		if reflect.DeepEqual(edge.Hostports, hostports) {
			return edge, nil
		}
	}
	return nil, iter.Err()
}
