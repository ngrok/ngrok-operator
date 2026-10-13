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

// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconciler is keyed by hostname (req.Name is a host key). Each hostname
// gets one Service, named so the CoreDNS rewrite for foo.internal lands on
// it, with one port per endpoint on that hostname.
type Reconciler struct {
	client.Client
	// APIReader reads PrivateEndpoints uncached for port allocation.
	APIReader         client.Reader
	Log               logr.Logger
	Namespace         string
	ForwarderSelector map[string]string
	PortMin, PortMax  int32

	// lastPort is the most recently allocated forwarder port. See allocate.
	lastPort int32
}

// retryAfter is how long to wait before retrying a conflict that only a
// change outside this hostname (a freed port, a deleted Service) resolves.
const retryAfter = 30 * time.Second

var errServiceConflict = errors.New("service not managed by the private endpoint controller")

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	hostKey := req.Name
	// Uncached: the informer may not have seen the forwarder ports the
	// previous reconcile just wrote, and reusing one cross-routes traffic.
	var all ngrokv1.PrivateEndpointList
	if err := r.APIReader.List(ctx, &all, client.InNamespace(r.Namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	var mine []*ngrokv1.PrivateEndpoint
	// others maps each forwarder port held on another hostname to the
	// lowest-named CR holding it; that CR wins if two hosts claim the port.
	others := map[int32]string{}
	for i := range all.Items {
		cr := &all.Items[i]
		if cr.DeletionTimestamp != nil {
			continue
		}
		if cr.Labels[pe.HostLabel] == hostKey {
			mine = append(mine, cr)
			continue
		}
		if fp := cr.Status.ForwarderPort; fp != 0 {
			if cur, ok := others[fp]; !ok || cr.Name < cur {
				others[fp] = cr.Name
			}
		}
	}
	if len(mine) == 0 {
		return ctrl.Result{}, r.deleteHostServices(ctx, hostKey)
	}
	svcName, err := pe.ServiceName(mine[0].Spec.Hostname)
	if err != nil {
		var errs []error
		for _, cr := range mine {
			errs = append(errs, r.setStatus(ctx, cr, "", 0, "UnsupportedHostname", err.Error()))
		}
		errs = append(errs, r.deleteHostServices(ctx, hostKey))
		return ctrl.Result{}, errors.Join(errs...)
	}
	return r.reconcileService(ctx, hostKey, svcName, mine, others)
}

func (r *Reconciler) reconcileService(ctx context.Context, hostKey, svcName string, crs []*ngrokv1.PrivateEndpoint, others map[int32]string) (ctrl.Result, error) {
	keepPort := func(cr *ngrokv1.PrivateEndpoint) bool {
		fp := cr.Status.ForwarderPort
		if fp < r.PortMin || fp > r.PortMax {
			return false
		}
		owner, taken := others[fp]
		return !taken || cr.Name < owner
	}
	used := map[int32]bool{}
	for p := range others {
		used[p] = true
	}
	// A port stays used while another host's Service routes to it, even if
	// no status claims it: the status write may have failed after the
	// Service write, or the CR may be gone before its Service.
	var svcs corev1.ServiceList
	if err := r.APIReader.List(ctx, &svcs, client.InNamespace(r.Namespace), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing managed Services: %w", err)
	}
	for _, svc := range svcs.Items {
		if svc.Labels[pe.HostLabel] == hostKey {
			continue
		}
		for _, p := range svc.Spec.Ports {
			used[p.TargetPort.IntVal] = true
		}
	}
	for _, cr := range crs {
		if keepPort(cr) {
			used[cr.Status.ForwarderPort] = true
		}
	}
	sort.Slice(crs, func(i, j int) bool {
		if crs[i].Spec.Port != crs[j].Spec.Port {
			return crs[i].Spec.Port < crs[j].Spec.Port
		}
		return crs[i].Name < crs[j].Name
	})
	fwdPorts := map[string]int32{}
	byPort := map[int32]string{}
	var ports []corev1.ServicePort
	var conflicts, exhausted []*ngrokv1.PrivateEndpoint
	for _, cr := range crs {
		if _, taken := byPort[cr.Spec.Port]; taken {
			conflicts = append(conflicts, cr)
			continue
		}
		byPort[cr.Spec.Port] = cr.Name
		fp := cr.Status.ForwarderPort
		if !keepPort(cr) {
			var ok bool
			if fp, ok = r.allocate(used); !ok {
				exhausted = append(exhausted, cr)
				continue
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

	var errs []error
	for _, cr := range conflicts {
		errs = append(errs, r.setStatus(ctx, cr, "", 0, "PortConflict",
			fmt.Sprintf("PrivateEndpoint %s already serves port %d on this hostname", byPort[cr.Spec.Port], cr.Spec.Port)))
	}
	for _, cr := range exhausted {
		errs = append(errs, r.setStatus(ctx, cr, "", 0, "PortsExhausted",
			fmt.Sprintf("no free forwarder ports in %d-%d", r.PortMin, r.PortMax)))
	}
	res := ctrl.Result{}
	if len(exhausted) > 0 {
		res.RequeueAfter = retryAfter
	}
	if len(ports) == 0 {
		errs = append(errs, r.deleteHostServices(ctx, hostKey))
		return res, errors.Join(errs...)
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: r.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if !svc.CreationTimestamp.IsZero() && (svc.Labels[pe.ManagedByLabel] != pe.ManagedByValue || svc.Labels[pe.HostLabel] != hostKey) {
			return errServiceConflict
		}
		svc.Labels = map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: hostKey}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = r.ForwarderSelector
		svc.Spec.Ports = ports
		return nil
	})
	if errors.Is(err, errServiceConflict) {
		// The Service watch only maps labeled Services, so nothing enqueues
		// us when the foreign Service goes away: poll instead.
		msg := fmt.Sprintf("Service %s/%s already exists and is not managed for this hostname", r.Namespace, svcName)
		for _, cr := range crs {
			if _, ok := fwdPorts[cr.Name]; ok {
				errs = append(errs, r.setStatus(ctx, cr, "", 0, "ServiceConflict", msg))
			}
		}
		return ctrl.Result{RequeueAfter: retryAfter}, errors.Join(errs...)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("applying Service %s: %w", svcName, err)
	}

	for _, cr := range crs {
		if fp, ok := fwdPorts[cr.Name]; ok {
			errs = append(errs, r.setStatus(ctx, cr, svc.Spec.ClusterIP, fp, "ServiceNotReady", ""))
		}
	}
	if svc.Spec.ClusterIP == "" {
		res.RequeueAfter = 2 * time.Second
	}
	return res, errors.Join(errs...)
}

// allocate hands out the next free port after the last one allocated,
// wrapping to PortMin, so a freed port is reused only after the rest of the
// range has been cycled through. That gives forwarder replicas still mapping
// the freed port to its old host time to catch up before it is reassigned.
//
// lastPort lives in memory and is seeded from the highest port in use, so
// after a restart or leader change a port freed above every remaining one
// can still be handed out again immediately.
func (r *Reconciler) allocate(used map[int32]bool) (int32, bool) {
	if r.lastPort == 0 {
		r.lastPort = r.PortMin - 1
		for p := range used {
			if p > r.lastPort && p <= r.PortMax {
				r.lastPort = p
			}
		}
	}
	n := r.PortMax - r.PortMin + 1
	for i := int32(1); i <= n; i++ {
		p := r.PortMin + (r.lastPort-r.PortMin+i)%n
		if !used[p] {
			r.lastPort = p
			return p, true
		}
	}
	return 0, false
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

func (r *Reconciler) deleteHostServices(ctx context.Context, hostKey string) error {
	var svcs corev1.ServiceList
	if err := r.List(ctx, &svcs, client.InNamespace(r.Namespace), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: hostKey}); err != nil {
		return fmt.Errorf("listing Services for host %s: %w", hostKey, err)
	}
	var errs []error
	for i := range svcs.Items {
		if err := r.Delete(ctx, &svcs.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting Service %s: %w", svcs.Items[i].Name, err))
		}
	}
	return errors.Join(errs...)
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
	return ctrl.NewControllerManagedBy(mgr).
		Named("privateendpoint").
		Watches(&ngrokv1.PrivateEndpoint{}, byHost, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Service{}, byHost).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). // port allocation assumes one worker
		Complete(r)
}
