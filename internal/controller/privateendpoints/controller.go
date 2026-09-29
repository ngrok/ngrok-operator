package privateendpoints

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconciler is keyed by hostname (req.Name is a host key), because DNS
// returns one IP per name: every endpoint on a hostname must share it.
type Reconciler struct {
	client.Client
	Log               logr.Logger
	Namespace         string
	SharedServiceName string
	ForwarderSelector map[string]string
	PortMin, PortMax  int32
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	hostKey := req.Name
	var all ngrokv1.PrivateEndpointList
	if err := r.List(ctx, &all, client.InNamespace(r.Namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	var mine []*ngrokv1.PrivateEndpoint
	used := map[int32]bool{}
	for i := range all.Items {
		cr := &all.Items[i]
		if cr.DeletionTimestamp != nil {
			continue
		}
		if cr.Labels[pe.HostLabel] == hostKey {
			mine = append(mine, cr)
		}
		if cr.Status.ForwarderPort != 0 {
			used[cr.Status.ForwarderPort] = true
		}
	}
	svcName := pe.HostServiceName(hostKey)
	if len(mine) == 0 {
		return ctrl.Result{}, r.deleteService(ctx, svcName)
	}
	for _, cr := range mine {
		if !pe.SharedEligible(cr.Spec) {
			return r.reconcileDedicated(ctx, hostKey, svcName, mine, used)
		}
	}
	return r.reconcileShared(ctx, svcName, mine)
}

func (r *Reconciler) reconcileShared(ctx context.Context, svcName string, crs []*ngrokv1.PrivateEndpoint) (ctrl.Result, error) {
	if err := r.deleteService(ctx, svcName); err != nil {
		return ctrl.Result{}, err
	}
	var shared corev1.Service
	err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: r.SharedServiceName}, &shared)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("getting shared Service: %w", err)
	}
	ip := shared.Spec.ClusterIP
	var errs []error
	for _, cr := range crs {
		errs = append(errs, r.setStatus(ctx, cr, ip, 0, "SharedServiceNotReady", "waiting for Service "+r.SharedServiceName))
	}
	if ip == "" {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, errors.Join(errs...)
	}
	return ctrl.Result{}, errors.Join(errs...)
}

func (r *Reconciler) reconcileDedicated(ctx context.Context, hostKey, svcName string, crs []*ngrokv1.PrivateEndpoint, used map[int32]bool) (ctrl.Result, error) {
	sort.Slice(crs, func(i, j int) bool {
		if crs[i].Spec.Port != crs[j].Spec.Port {
			return crs[i].Spec.Port < crs[j].Spec.Port
		}
		return crs[i].Name < crs[j].Name
	})
	fwdPorts := map[string]int32{}
	byPort := map[int32]string{}
	var ports []corev1.ServicePort
	var conflicts []*ngrokv1.PrivateEndpoint
	for _, cr := range crs {
		if _, taken := byPort[cr.Spec.Port]; taken {
			conflicts = append(conflicts, cr)
			continue
		}
		byPort[cr.Spec.Port] = cr.Name
		fp := cr.Status.ForwarderPort
		if fp < r.PortMin || fp > r.PortMax {
			var err error
			if fp, err = r.allocate(used); err != nil {
				return ctrl.Result{}, err
			}
			used[fp] = true
		}
		fwdPorts[cr.Name] = fp
		ports = append(ports, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", cr.Spec.Port),
			Protocol:   corev1.ProtocolTCP,
			Port:       cr.Spec.Port,
			TargetPort: intstr.FromInt32(fp),
		})
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: r.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: hostKey}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = r.ForwarderSelector
		svc.Spec.Ports = ports
		return nil
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying Service %s: %w", svcName, err)
	}

	var errs []error
	for _, cr := range crs {
		if fp, ok := fwdPorts[cr.Name]; ok {
			errs = append(errs, r.setStatus(ctx, cr, svc.Spec.ClusterIP, fp, "ServiceNotReady", ""))
		}
	}
	for _, cr := range conflicts {
		errs = append(errs, r.setStatus(ctx, cr, "", 0, "PortConflict",
			fmt.Sprintf("PrivateEndpoint %s already serves port %d on this hostname", byPort[cr.Spec.Port], cr.Spec.Port)))
	}
	if svc.Spec.ClusterIP == "" {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, errors.Join(errs...)
	}
	return ctrl.Result{}, errors.Join(errs...)
}

func (r *Reconciler) allocate(used map[int32]bool) (int32, error) {
	for p := r.PortMin; p <= r.PortMax; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free forwarder ports in %d-%d", r.PortMin, r.PortMax)
}

// setStatus writes ip/port and the Ready condition. An empty ip means not
// ready, with notReadyReason and msg explaining why.
func (r *Reconciler) setStatus(ctx context.Context, cr *ngrokv1.PrivateEndpoint, ip string, port int32, notReadyReason, msg string) error {
	orig := cr.Status.DeepCopy()
	cr.Status.ClusterIP = ip
	cr.Status.ForwarderPort = port
	cr.Status.ObservedGeneration = cr.Generation
	cond := metav1.Condition{Type: ngrokv1.PrivateEndpointConditionReady, Status: metav1.ConditionTrue, Reason: "Ready", ObservedGeneration: cr.Generation}
	if ip == "" {
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, notReadyReason, msg
	}
	meta.SetStatusCondition(&cr.Status.Conditions, cond)
	if equality.Semantic.DeepEqual(orig, &cr.Status) {
		return nil
	}
	if err := r.Status().Update(ctx, cr); err != nil {
		return fmt.Errorf("updating PrivateEndpoint %s status: %w", cr.Name, err)
	}
	return nil
}

func (r *Reconciler) deleteService(ctx context.Context, name string) error {
	err := r.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace}})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting Service %s: %w", name, err)
	}
	return nil
}

func (r *Reconciler) hostRequest(key string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: key}}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	byHost := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		if o.GetNamespace() != r.Namespace || o.GetLabels()[pe.HostLabel] == "" {
			return nil
		}
		return []reconcile.Request{r.hostRequest(o.GetLabels()[pe.HostLabel])}
	})
	services := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		if o.GetNamespace() != r.Namespace {
			return nil
		}
		if o.GetName() != r.SharedServiceName {
			if key := o.GetLabels()[pe.HostLabel]; key != "" {
				return []reconcile.Request{r.hostRequest(key)}
			}
			return nil
		}
		var list ngrokv1.PrivateEndpointList
		if err := r.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
			r.Log.Error(err, "listing PrivateEndpoints for shared Service change")
			return nil
		}
		seen := map[string]bool{}
		var reqs []reconcile.Request
		for _, cr := range list.Items {
			if key := cr.Labels[pe.HostLabel]; key != "" && !seen[key] {
				seen[key] = true
				reqs = append(reqs, r.hostRequest(key))
			}
		}
		return reqs
	})
	return ctrl.NewControllerManagedBy(mgr).
		Named("privateendpoint").
		Watches(&ngrokv1.PrivateEndpoint{}, byHost, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Service{}, services).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). // port allocation assumes one worker
		Complete(r)
}
