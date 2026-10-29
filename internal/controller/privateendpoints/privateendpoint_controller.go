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

// PrivateEndpointReconciler is keyed by hostname (req.Name is the normalized hostname). Each hostname
// gets one Service, named so the CoreDNS rewrite for foo.ngrok.direct lands on
// it, with one port per endpoint on that hostname.
type PrivateEndpointReconciler struct {
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

func (r *PrivateEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	host := req.Name
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
		// A deleting CR is treated as gone: its port frees up and it drops
		// out of its host's Service.
		if cr.DeletionTimestamp != nil {
			continue
		}
		if pe.NormalizeHost(cr.Spec.Hostname) == host {
			mine = append(mine, cr)
			continue
		}
		if fp := cr.Status.ForwarderPort; fp != 0 {
			if cur, ok := others[fp]; !ok || cr.Name < cur {
				others[fp] = cr.Name
			}
		}
	}
	// The last endpoint on this hostname is gone, so its Service goes too.
	// A hostname that can't form a Service name can't be reached through
	// the DNS rewrite, so none of its endpoints are wired, and it never had
	// a Service to clean up.
	svcName, err := pe.ServiceName(host)
	if err != nil {
		var errs []error
		for _, cr := range mine {
			errs = append(errs, r.setStatus(ctx, cr, "", 0, "UnsupportedHostname", err.Error()))
		}
		return ctrl.Result{}, errors.Join(errs...)
	}
	if len(mine) == 0 {
		return ctrl.Result{}, r.deleteService(ctx, svcName)
	}
	return r.reconcileService(ctx, svcName, mine, others)
}

func (r *PrivateEndpointReconciler) reconcileService(ctx context.Context, svcName string, crs []*ngrokv1.PrivateEndpoint, others map[int32]string) (ctrl.Result, error) {
	used, routed, err := r.portsInUse(ctx, svcName, crs, others)
	if err != nil {
		return ctrl.Result{}, err
	}
	a := r.assignPorts(crs, others, used, routed)

	var errs []error
	for _, cr := range a.conflicts {
		errs = append(errs, r.setStatus(ctx, cr, "", 0, "PortConflict",
			fmt.Sprintf("PrivateEndpoint %s already serves port %d on this hostname", a.byPort[cr.Spec.Port], cr.Spec.Port)))
	}
	for _, cr := range a.exhausted {
		errs = append(errs, r.setStatus(ctx, cr, "", 0, "PortsExhausted",
			fmt.Sprintf("no free forwarder ports in %d-%d", r.PortMin, r.PortMax)))
	}
	res := ctrl.Result{}
	// Ports free up only when another hostname's endpoints go away, which
	// doesn't enqueue this one, so poll.
	if len(a.exhausted) > 0 {
		res.RequeueAfter = retryAfter
	}
	if len(a.ports) == 0 {
		errs = append(errs, r.deleteService(ctx, svcName))
		return res, errors.Join(errs...)
	}

	svc, err := r.applyService(ctx, svcName, a.ports)
	if errors.Is(err, errServiceConflict) {
		// The Service watch only maps labeled Services, so nothing enqueues
		// us when the foreign Service goes away: poll instead.
		msg := fmt.Sprintf("Service %s/%s already exists and is not managed for this hostname", r.Namespace, svcName)
		for _, cr := range crs {
			if _, ok := a.fwdPorts[cr.Name]; ok {
				errs = append(errs, r.setStatus(ctx, cr, "", 0, "ServiceConflict", msg))
			}
		}
		return ctrl.Result{RequeueAfter: retryAfter}, errors.Join(errs...)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("applying Service %s: %w", svcName, err)
	}

	for _, cr := range crs {
		if fp, ok := a.fwdPorts[cr.Name]; ok {
			errs = append(errs, r.setStatus(ctx, cr, svc.Spec.ClusterIP, fp, "ServiceNotReady", ""))
		}
	}
	// The API server normally assigns the ClusterIP on create; if it hasn't,
	// retry soon rather than rely on the next Service event.
	if svc.Spec.ClusterIP == "" {
		res.RequeueAfter = 2 * time.Second
	}
	return res, errors.Join(errs...)
}

// keepsPort reports whether cr keeps the forwarder port in its status. Ports
// stay put across reconciles so forwarders and existing connections aren't
// disturbed; a port is given up only if it is out of range or a lower-named
// CR on another hostname also claims it.
func (r *PrivateEndpointReconciler) keepsPort(cr *ngrokv1.PrivateEndpoint, others map[int32]string) bool {
	fp := cr.Status.ForwarderPort
	if fp < r.PortMin || fp > r.PortMax {
		return false
	}
	owner, taken := others[fp]
	return !taken || cr.Name < owner
}

// portsInUse returns the forwarder ports no new allocation may take (used)
// and, for this host's existing Service, each port's current target
// (routed).
func (r *PrivateEndpointReconciler) portsInUse(ctx context.Context, svcName string, crs []*ngrokv1.PrivateEndpoint, others map[int32]string) (used map[int32]bool, routed map[int32]int32, err error) {
	used = map[int32]bool{}
	for p := range others {
		used[p] = true
	}
	// A port stays used while another host's Service routes to it, even if
	// no status claims it: the status write may have failed after the
	// Service write, or the CR may be gone before its Service.
	var svcs corev1.ServiceList
	if err := r.APIReader.List(ctx, &svcs, client.InNamespace(r.Namespace), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue}); err != nil {
		return nil, nil, fmt.Errorf("listing managed Services: %w", err)
	}
	// routed lets an endpoint whose status write failed after the Service
	// write get the port its Service already routes instead of a new one on
	// every retry.
	routed = map[int32]int32{}
	for _, svc := range svcs.Items {
		if svc.Name == svcName {
			for _, p := range svc.Spec.Ports {
				routed[p.Port] = p.TargetPort.IntVal
			}
			continue
		}
		for _, p := range svc.Spec.Ports {
			used[p.TargetPort.IntVal] = true
		}
	}
	for _, cr := range crs {
		if r.keepsPort(cr, others) {
			used[cr.Status.ForwarderPort] = true
		}
	}
	return used, routed, nil
}

// portAssignment is the outcome of assignPorts for one hostname.
type portAssignment struct {
	ports     []corev1.ServicePort // the host Service's ports
	fwdPorts  map[string]int32     // CR name -> forwarder port, for CRs served
	byPort    map[int32]string     // Service port -> CR name serving it
	conflicts []*ngrokv1.PrivateEndpoint
	exhausted []*ngrokv1.PrivateEndpoint
}

// assignPorts gives each CR a forwarder port, keeping the one it has where
// it can, and builds the host Service's ports from them. It marks used every
// port it hands out.
func (r *PrivateEndpointReconciler) assignPorts(crs []*ngrokv1.PrivateEndpoint, others map[int32]string, used map[int32]bool, routed map[int32]int32) portAssignment {
	// A stable order keeps the Service's port list from reordering (and
	// rewriting) between reconciles, and makes the lowest-named CR win a
	// port conflict below.
	sort.Slice(crs, func(i, j int) bool {
		if crs[i].Spec.Port != crs[j].Spec.Port {
			return crs[i].Spec.Port < crs[j].Spec.Port
		}
		return crs[i].Name < crs[j].Name
	})
	a := portAssignment{fwdPorts: map[string]int32{}, byPort: map[int32]string{}}
	for _, cr := range crs {
		// A Service port number can appear once, so two endpoints on the
		// same hostname and port (e.g. http:// and tcp://...:80) can't both
		// be served.
		if _, taken := a.byPort[cr.Spec.Port]; taken {
			a.conflicts = append(a.conflicts, cr)
			continue
		}
		a.byPort[cr.Spec.Port] = cr.Name
		fp := cr.Status.ForwarderPort
		if !r.keepsPort(cr, others) {
			if p := routed[cr.Spec.Port]; p >= r.PortMin && p <= r.PortMax && !used[p] {
				fp = p
			} else {
				var ok bool
				if fp, ok = r.allocate(used); !ok {
					a.exhausted = append(a.exhausted, cr)
					continue
				}
			}
			used[fp] = true
		}
		a.fwdPorts[cr.Name] = fp
		a.ports = append(a.ports, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", cr.Spec.Port),
			Protocol:   corev1.ProtocolTCP,
			Port:       cr.Spec.Port,
			TargetPort: intstr.FromInt32(fp),
		})
	}
	return a
}

// applyService creates or updates the host's Service with ports. It returns
// errServiceConflict if a Service by that name exists that this controller
// didn't create.
func (r *PrivateEndpointReconciler) applyService(ctx context.Context, svcName string, ports []corev1.ServicePort) (*corev1.Service, error) {
	svc := &corev1.Service{Name: svcName, Namespace: r.Namespace}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		// Never take over an existing Service we didn't create; it may
		// belong to the user.
		if !svc.CreationTimestamp.IsZero() && svc.Labels[pe.ManagedByLabel] != pe.ManagedByValue {
			return errServiceConflict
		}
		svc.Labels = map[string]string{pe.ManagedByLabel: pe.ManagedByValue}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = r.ForwarderSelector
		svc.Spec.Ports = ports
		return nil
	})
	return svc, err
}

// allocate hands out the next free port after the last one allocated,
// wrapping to PortMin, so a freed port is reused only after the rest of the
// range has been cycled through. That gives forwarder replicas still mapping
// the freed port to its old host time to catch up before it is reassigned.
//
// lastPort lives in memory and is seeded from the highest port in use, so
// after a restart or leader change a port freed above every remaining one
// can still be handed out again immediately.
func (r *PrivateEndpointReconciler) allocate(used map[int32]bool) (int32, bool) {
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
func (r *PrivateEndpointReconciler) setStatus(ctx context.Context, cr *ngrokv1.PrivateEndpoint, ip string, port int32, notReadyReason, msg string) error {
	orig := cr.Status.DeepCopy()
	cr.Status.ClusterIP = ip
	cr.Status.ForwarderPort = port
	cr.Status.ObservedGeneration = cr.Generation
	cond := metav1.Condition{Type: ngrokv1.PrivateEndpointConditionReady, Status: metav1.ConditionTrue, Reason: "Ready", ObservedGeneration: cr.Generation}
	if ip == "" {
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, notReadyReason, msg
	}
	meta.SetStatusCondition(&cr.Status.Conditions, cond)
	// Skip no-op writes: every reconcile sets the status of every endpoint
	// on the hostname, and most of them haven't changed.
	if equality.Semantic.DeepEqual(orig, &cr.Status) {
		return nil
	}
	if err := r.Status().Update(ctx, cr); err != nil {
		return fmt.Errorf("updating PrivateEndpoint %s status: %w", cr.Name, err)
	}
	return nil
}

// deleteService deletes the named Service if this controller created it.
// The cached read is fine here: a Service it misses still has its
// managed-by label, so its own watch event enqueues this host again.
func (r *PrivateEndpointReconciler) deleteService(ctx context.Context, svcName string) error {
	svc := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: svcName}, svc); err != nil {
		return client.IgnoreNotFound(err)
	}
	if svc.Labels[pe.ManagedByLabel] != pe.ManagedByValue {
		return nil
	}
	if err := r.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting Service %s: %w", svcName, err)
	}
	return nil
}

func (r *PrivateEndpointReconciler) hostRequest(host string) []reconcile.Request {
	return []reconcile.Request{{Namespace: r.Namespace, Name: host}}
}

// SetupWithManager maps PrivateEndpoints, by spec.hostname, and the Services
// this controller created, by name, to their hostname, since one reconcile
// handles a whole hostname. Services without the managed-by label are
// ignored.
func (r *PrivateEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	endpointHost := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		cr, ok := o.(*ngrokv1.PrivateEndpoint)
		if !ok || cr.Namespace != r.Namespace || cr.Spec.Hostname == "" {
			return nil
		}
		return r.hostRequest(pe.NormalizeHost(cr.Spec.Hostname))
	})
	serviceHost := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		if o.GetNamespace() != r.Namespace || o.GetLabels()[pe.ManagedByLabel] != pe.ManagedByValue {
			return nil
		}
		host, ok := pe.HostnameForService(o.GetName())
		if !ok {
			return nil
		}
		return r.hostRequest(host)
	})
	return ctrl.NewControllerManagedBy(mgr).
		Named("privateendpoint").
		// Status writes don't bump generation, so this controller's own
		// writes don't requeue it; spec changes, creates and deletes do.
		Watches(&ngrokv1.PrivateEndpoint{}, endpointHost, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Service events restore a managed Service that was edited or deleted.
		Watches(&corev1.Service{}, serviceHost).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). // port allocation assumes one worker
		Complete(r)
}
