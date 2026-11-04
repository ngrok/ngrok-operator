/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	clientCertificateRefsIndex = "spec.clientCertificateRefs"
	tlsTerminationSecretsIndex = "spec.tlsTermination.secrets"

	// ReasonSupersededByV1 is the Ready reason on a legacy AgentEndpoint whose
	// ngrok.com/v1 twin now drives the agent endpoint.
	//
	// LEGACY-agentendpoint-kind: delete at cleanup.
	ReasonSupersededByV1 = "SupersededByV1"
)

// agentEndpoint is an AgentEndpoint of either served API group.
type agentEndpoint interface {
	ngrokv1.EndpointWithDomain
	GetTrafficPolicyCfg() *ngrokv1.TrafficPolicyCfg
	SetObservedGeneration(generation int64)
}

// endpointParts is the group-independent view of an AgentEndpoint that the
// reconcile works from.
type endpointParts struct {
	driverSpec            agent.EndpointSpec
	clientCertificateRefs []ngrokv1.K8sObjectRef
	tlsTermination        *ngrokv1.EndpointTLSTermination
	status                *ngrokv1.AgentEndpointStatus
}

// legacyMetadataString is the metadata a legacy AgentEndpoint sends. String
// metadata holding a JSON object of strings is sent the way its v1 twin's map
// is (compact, sorted keys), so migrating doesn't reconnect the endpoint just
// because the string was formatted differently.
//
// LEGACY-agentendpoint-kind: delete at cleanup.
func legacyMetadataString(raw json.RawMessage) string {
	s := common.MetadataAPIString(raw)
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		return s
	}
	return common.MetadataAPIString(json.RawMessage(s))
}

func partsOf(ep agentEndpoint) endpointParts {
	switch o := ep.(type) {
	case *ngrokv1.AgentEndpoint:
		return endpointParts{
			driverSpec: agent.EndpointSpec{
				URL:         o.Spec.URL,
				Upstream:    o.Spec.Upstream,
				Description: o.Spec.Description,
				Metadata:    common.MetadataAPIString(common.MetadataFromMap(o.Spec.Metadata)),
				Bindings:    o.Spec.Bindings,
			},
			clientCertificateRefs: o.Spec.ClientCertificateRefs,
			tlsTermination:        o.Spec.TLSTermination,
			status:                &o.Status,
		}
	case *ngrokv1alpha1.AgentEndpoint: // LEGACY-agentendpoint-kind: delete this case at cleanup.
		return endpointParts{
			driverSpec: agent.EndpointSpec{
				URL:         o.Spec.URL,
				Upstream:    o.Spec.Upstream,
				Description: o.Spec.Description,
				Metadata:    legacyMetadataString(o.Spec.Metadata),
				Bindings:    o.Spec.Bindings,
			},
			clientCertificateRefs: o.Spec.ClientCertificateRefs,
			tlsTermination:        o.Spec.TLSTermination,
			status:                &o.Status,
		}
	default:
		panic(fmt.Sprintf("unsupported AgentEndpoint type %T", ep))
	}
}

// indexClientCertificateRefs extracts client certificate reference keys for indexing
func indexClientCertificateRefs(o client.Object) []string {
	ep, ok := o.(agentEndpoint)
	if !ok {
		return nil
	}
	var keys []string
	for _, ref := range partsOf(ep).clientCertificateRefs {
		keys = append(keys, secretIndexKey(ep.GetNamespace(), ref))
	}
	return keys
}

// indexTLSTerminationSecrets extracts Secret reference keys used by spec.tlsTermination
// (server certificate and optional mTLS client-CA bundle) for indexing. These
// refs are same-namespace only, so they always resolve against the endpoint's namespace.
func indexTLSTerminationSecrets(o client.Object) []string {
	ep, ok := o.(agentEndpoint)
	if !ok {
		return nil
	}
	tlsTermination := partsOf(ep).tlsTermination
	if tlsTermination == nil {
		return nil
	}
	keys := []string{ep.GetNamespace() + "/" + tlsTermination.ServerCertificateRef.Name}
	if tlsTermination.MutualTLS != nil {
		keys = append(keys, ep.GetNamespace()+"/"+tlsTermination.MutualTLS.ClientCAsRef.Name)
	}
	return keys
}

// secretIndexKey returns the "namespace/name" key used by Secret-watch indexes.
func secretIndexKey(namespace string, ref ngrokv1.K8sObjectRef) string {
	return namespace + "/" + ref.Name
}

// AgentEndpointReconciler reconciles AgentEndpoints of both served kinds: the
// canonical ngrok.com/v1 AgentEndpoint and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 AgentEndpoint.
//
// The two kinds are reconciled together, keyed by the namespace/name they
// share. A legacy and a v1 AgentEndpoint with the same name are "twins". The
// agent driver keys its tunnels by namespace/name too, so both twins address
// the same tunnel; an AgentEndpoint has no remote ID to hand over. Because
// both kinds enqueue the same request key, controller-runtime never
// reconciles a pair concurrently.
//
// This implements the twin rules in specs/crd-group-migration.md:
//  1. While a v1 object is alive, it is the only one that drives the tunnel.
//     The legacy twin is passive: Ready with reason SupersededByV1.
//  2. The tunnel is closed only when no surviving object (one that exists and
//     isn't being deleted) would still use it, and the check fails closed
//     (see liveTwinCheck).
//  3. Deleting the v1 object hands the tunnel back to its legacy twin in the
//     same pass.
//
// LEGACY-agentendpoint-kind: at cleanup, drop the legacy watch, indexes and
// list halves, the legacy half of agentEndpointPair, and the passive and
// twin-check branches. What remains is a single-kind reconcile.
type AgentEndpointReconciler struct {
	client.Client

	// APIReader reads from the API server, bypassing the cache. It is used
	// only to re-check the twin right before closing a tunnel. Defaults to the
	// manager's API reader.
	APIReader client.Reader

	Log         logr.Logger
	Scheme      *runtime.Scheme
	Recorder    events.EventRecorder
	AgentDriver agent.Driver

	ControllerLabels           labels.ControllerLabelValues
	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
	DomainManager              *domainpkg.Manager
	TrafficPolicyManager       *trafficpolicypkg.Manager

	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

// SetupWithManager sets up the controller with the Manager

func (r *AgentEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerNamed(mgr, "agentendpoint")
}

// SetupWithManagerNamed sets up the controller with the Manager using a custom controller name.
// This is useful for tests that need to run multiple controllers.
func (r *AgentEndpointReconciler) SetupWithManagerNamed(mgr ctrl.Manager, controllerName string) error {
	if r.AgentDriver == nil {
		return errors.New("AgentDriver is nil")
	}
	if r.APIReader == nil {
		r.APIReader = mgr.GetAPIReader()
	}

	// Initialize domain manager if not already set
	if r.DomainManager == nil {
		if err := labels.ValidateControllerLabelValues(r.ControllerLabels); err != nil {
			return err
		}

		opts := []domainpkg.ManagerOption{
			domainpkg.WithControllerLabels(r.ControllerLabels),
		}

		if r.DefaultDomainReclaimPolicy != nil {
			opts = append(opts, domainpkg.WithDefaultDomainReclaimPolicy(*r.DefaultDomainReclaimPolicy))
		}

		dm, err := domainpkg.NewManager(r.Client, r.Recorder, opts...)
		if err != nil {
			return err
		}
		r.DomainManager = dm
	}

	if r.TrafficPolicyManager == nil {
		r.TrafficPolicyManager = trafficpolicypkg.NewManager(r.Client, r.Recorder)
	}

	kinds := []client.Object{
		&ngrokv1.AgentEndpoint{},
		&ngrokv1alpha1.AgentEndpoint{}, // LEGACY-agentendpoint-kind: delete at cleanup.
	}
	for _, kind := range kinds {
		for index, extract := range map[string]client.IndexerFunc{
			trafficpolicypkg.RefIndex:  trafficpolicypkg.IndexKeyForObject,
			clientCertificateRefsIndex: indexClientCertificateRefs,
			tlsTerminationSecretsIndex: indexTLSTerminationSecrets,
		} {
			if err := mgr.GetFieldIndexer().IndexField(context.Background(), kind, index, extract); err != nil {
				return err
			}
		}
	}

	changed := builder.WithPredicates(
		predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&ngrokv1.AgentEndpoint{}, changed).
		// LEGACY-agentendpoint-kind: delete this watch at cleanup. It enqueues
		// the same namespace/name key as the v1 twin, which is what makes the
		// two reconcile as a pair.
		Watches(&ngrokv1alpha1.AgentEndpoint{}, &handler.EnqueueRequestForObject{}, changed).
		// LEGACY-trafficpolicy-kind: BEGIN
		// Watch the deprecated ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy
		// so endpoints that still resolve through the fallback path
		// re-enqueue on legacy-kind updates. Delete this Watches call at
		// cleanup; the canonical Watches below stays.
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.enqueueFor(r.findAgentEndpointForTrafficPolicy),
		).
		// LEGACY-trafficpolicy-kind: END
		Watches(
			// Canonical ngrok.com/v1 TrafficPolicy watches — the endpoint's
			// spec.trafficPolicy.targetRef is untyped-by-kind, so the same
			// mapper enqueues on either kind by namespace/name key.
			&ngrokv1.TrafficPolicy{},
			r.enqueueFor(r.findAgentEndpointForTrafficPolicy),
		).
		Watches(
			&v1.Secret{},
			r.enqueueFor(r.findAgentEndpointForSecret),
		).
		Watches(
			&ingressv1alpha1.Domain{},
			r.enqueueFor(r.findAgentEndpointsForDomain),
		).
		Complete(r)
}

// enqueueFor wraps a map function so it gets the controller's logger from its
// context.
func (r *AgentEndpointReconciler) enqueueFor(f handler.MapFunc) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		return f(ctrl.LoggerInto(ctx, r.Log), obj)
	})
}

// Reconcile runs one pass over the pair of AgentEndpoints named by req.
func (r *AgentEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("agentendpoint", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	pair, err := loadAgentEndpointPair(ctx, r.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(pair.all()) == 0 {
		return ctrl.Result{}, r.closeOrphanedTunnel(ctx, req.NamespacedName)
	}

	// Like BaseController: while draining, only deletes are processed, so no
	// new finalizers get added.
	draining := controller.IsDraining(ctx, r.DrainState)
	if draining && len(pair.deleting()) == 0 {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	if !draining {
		for _, ep := range pair.surviving() {
			if err := util.RegisterAndSyncFinalizer(ctx, r.Client, ep); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if res, err := r.processDeletes(ctx, pair); err != nil || !res.IsZero() {
		return res, err
	}

	if draining {
		return ctrl.Result{}, nil
	}
	return r.syncSurvivors(ctx, pair)
}

// processDeletes handles every object in the pair that is being deleted. The
// tunnel is closed only when no surviving twin would still use it.
func (r *AgentEndpointReconciler) processDeletes(ctx context.Context, pair *agentEndpointPair) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	tunnelName := r.statusID(pair.key)

	for _, ep := range pair.deleting() {
		if !util.HasFinalizer(ep) {
			continue
		}

		keep := len(pair.surviving()) > 0
		if !keep {
			var err error
			if keep, err = r.liveTwinCheck(ctx, ep); err != nil {
				return ctrl.Result{}, err
			}
		}

		if keep {
			log.Info("A live twin uses this agent endpoint; releasing the finalizer without closing it", "kind", util.ObjToHumanGvkName(ep))
		} else {
			name := util.ObjToHumanName(ep)
			r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Deleting", "Delete", fmt.Sprintf("Deleting %s", name))
			if err := r.AgentDriver.DeleteAgentEndpoint(ctx, tunnelName); err != nil {
				r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "DeleteError", "Delete", fmt.Sprintf("Failed to delete %s: %s", name, err.Error()))
				return r.errResult(ep, err)
			}
			r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Deleted", "Delete", fmt.Sprintf("Deleted %s", name))
		}

		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, ep); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// closeOrphanedTunnel closes the agent endpoint of a key with no object left.
// Every agent replica binds every AgentEndpoint, but only the replica that
// removes the finalizer closes its tunnel in processDeletes; the others get
// here. Both kinds are re-read from the API server first, since one may be
// newer than the cache, and the tunnel is kept if either still exists.
//
// LEGACY-agentendpoint-kind: at cleanup, re-read only the v1 kind.
func (r *AgentEndpointReconciler) closeOrphanedTunnel(ctx context.Context, key client.ObjectKey) error {
	for _, obj := range []client.Object{&ngrokv1.AgentEndpoint{}, &ngrokv1alpha1.AgentEndpoint{}} {
		keep, err := keepTunnelForTwinRead(obj, r.APIReader.Get(ctx, key, obj))
		if err != nil {
			return fmt.Errorf("re-reading %s before closing its agent endpoint: %w", key, err)
		}
		if keep {
			return nil
		}
	}
	return r.AgentDriver.DeleteAgentEndpoint(ctx, r.statusID(key))
}

// liveTwinCheck re-reads ep's twin from the API server, bypassing the cache,
// right before the tunnel is closed. The two kinds come from separate
// informers, so the cache can be stale. It reports whether the tunnel must be
// kept; see keepTunnelForTwinRead.
//
// LEGACY-agentendpoint-kind: delete at cleanup.
func (r *AgentEndpointReconciler) liveTwinCheck(ctx context.Context, ep agentEndpoint) (bool, error) {
	var twin client.Object = &ngrokv1.AgentEndpoint{}
	if _, ok := ep.(*ngrokv1.AgentEndpoint); ok {
		twin = &ngrokv1alpha1.AgentEndpoint{}
	}

	keep, err := keepTunnelForTwinRead(twin, r.APIReader.Get(ctx, client.ObjectKeyFromObject(ep), twin))
	if err != nil {
		return false, fmt.Errorf("re-reading the twin of %s before closing its agent endpoint: %w", ep.GetName(), err)
	}
	return keep, nil
}

// keepTunnelForTwinRead interprets the result of reading a deleted
// AgentEndpoint's twin strictly (fail closed):
//   - NotFound, or the twin's kind isn't served (NoMatch): no twin; close.
//   - Forbidden: treat the twin as present and keep the tunnel. On rollback,
//     RBAC reverts before the CRDs do, so this happens in practice.
//   - Any other error: returned, so the pass requeues.
//   - Otherwise keep the tunnel unless the twin is being deleted too.
//
// LEGACY-agentendpoint-kind: delete at cleanup.
func keepTunnelForTwinRead(twin client.Object, err error) (bool, error) {
	switch {
	case apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		return false, nil
	case apierrors.IsForbidden(err):
		return true, nil
	case err != nil:
		return false, err
	}
	return controller.IsUpsert(twin), nil
}

// syncSurvivors drives the tunnel from the surviving writer: v1 while it's
// alive, otherwise legacy. A legacy object with a live v1 twin is marked
// passive.
func (r *AgentEndpointReconciler) syncSurvivors(ctx context.Context, pair *agentEndpointPair) (ctrl.Result, error) {
	canonical, legacy := pair.survivingV1(), pair.survivingLegacy()
	if canonical == nil {
		if legacy == nil {
			return ctrl.Result{}, nil
		}
		return r.sync(ctx, legacy)
	}

	res, err := r.sync(ctx, canonical)

	// LEGACY-agentendpoint-kind: delete this block at cleanup.
	if legacy != nil {
		// Skip the write once the passive status is settled: every v1
		// reconcile on every replica gets here, and each legacy request draws
		// a deprecation warning from the API server.
		before := legacy.Status.DeepCopy()
		r.setPassive(legacy, canonical)
		legacy.Status.ObservedGeneration = legacy.Generation
		if equality.Semantic.DeepEqual(before, &legacy.Status) {
			return res, err
		}
		if serr := r.writeStatus(ctx, legacy, nil); serr != nil && err == nil {
			return controller.CtrlResultForErr(serr)
		}
	}
	return res, err
}

// setPassive marks a legacy AgentEndpoint whose v1 twin drives the agent
// endpoint. It stays Ready, since the endpoint is served, so alerts on legacy
// Ready=False don't fire mid-migration. The rest of its status is left as the
// legacy object last wrote it.
//
// LEGACY-agentendpoint-kind: delete at cleanup.
func (r *AgentEndpointReconciler) setPassive(legacy *ngrokv1alpha1.AgentEndpoint, canonical *ngrokv1.AgentEndpoint) {
	ready := meta.FindStatusCondition(legacy.Status.Conditions, ConditionReady)
	becomingPassive := ready == nil || ready.Reason != ReasonSupersededByV1

	setReadyCondition(legacy, true, ReasonSupersededByV1, fmt.Sprintf(
		"Superseded by agentendpoints.ngrok.com %s/%s, which now drives this agent endpoint. Changes to this object have no effect; delete it once you've migrated.",
		canonical.GetNamespace(), canonical.GetName()))

	// Operator-generated objects are migrated by the operator; the user can't
	// act on a warning about them.
	if r.isOperatorManaged(legacy) {
		return
	}
	if becomingPassive || legacy.GetGeneration() != legacy.Status.ObservedGeneration {
		r.Recorder.Eventf(legacy, canonical, v1.EventTypeWarning, trafficpolicypkg.EventDeprecatedAPIGroup, "Reconcile",
			"ngrok.k8s.ngrok.com/v1alpha1 AgentEndpoint is deprecated and superseded by agentendpoints.ngrok.com %s; edit that object instead",
			canonical.GetName())
	}
}

// isOperatorManaged reports whether the operator generated ep, either from an
// Ingress or Gateway (a controller-name label) or from a Service (controller
// owner reference). The label carries the api-manager's name, not this
// agent's, so any value counts, matching the upgrade guide's selector.
//
// LEGACY-agentendpoint-kind: delete at cleanup.
func (r *AgentEndpointReconciler) isOperatorManaged(ep client.Object) bool {
	l := ep.GetLabels()
	if l[labels.ControllerName] != "" || l[labels.LegacyControllerName] != "" {
		return true
	}
	owner := metav1.GetControllerOf(ep)
	return owner != nil && owner.Kind == "Service"
}

// sync creates or updates the agent endpoint from ep and writes ep's status.
func (r *AgentEndpointReconciler) sync(ctx context.Context, ep agentEndpoint) (ctrl.Result, error) {
	name := util.ObjToHumanName(ep)
	r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Updating", "Update", fmt.Sprintf("Updating %s", name))
	if err := r.update(ctx, ep); err != nil {
		r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "UpdateError", "Update", fmt.Sprintf("Failed to update %s: %s", name, err.Error()))
		return r.errResult(ep, err)
	}
	r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Updated", "Update", fmt.Sprintf("Updated %s", name))
	return ctrl.Result{}, nil
}

func (r *AgentEndpointReconciler) errResult(ep agentEndpoint, err error) (ctrl.Result, error) {
	if errors.Is(err, domainpkg.ErrDomainNotReady) {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if errors.Is(err, trafficpolicypkg.ErrInvalidConfig) || errors.Is(err, trafficpolicypkg.ErrInvalidPolicyJSON) {
		r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "ConfigError", "Reconcile", err.Error())
		r.Log.Error(err, "invalid TrafficPolicy configuration", "name", ep.GetName(), "namespace", ep.GetNamespace())
		return ctrl.Result{}, nil // Do not requeue
	}
	if errors.Is(err, trafficpolicypkg.ErrTrafficPolicyNotFound) {
		// Terminal: the condition is already False and an event was
		// emitted during Resolve. Don't requeue — the TrafficPolicy
		// watch re-enqueues this endpoint when the policy is (re)created.
		r.Log.Info("referenced TrafficPolicy not found; awaiting (re)creation", "name", ep.GetName(), "namespace", ep.GetNamespace())
		return ctrl.Result{}, nil
	}
	if ngrokapi.IsTrafficPolicyError(err.Error()) {
		// Terminal: CreateAgentEndpoint rejected the policy itself and
		// TrafficPolicyManager.SetError already recorded it on the
		// TrafficPolicyApplied condition. This error comes from the
		// agent SDK (golang.ngrok.com/ngrok/v2), not ngrok-api-go, so
		// CtrlResultForErr's *ngrok.Error unwrap below can't catch it —
		// without this it would requeue a permanent user-config error
		// forever.
		return ctrl.Result{}, nil
	}
	return controller.CtrlResultForErr(err)
}

func (r *AgentEndpointReconciler) update(ctx context.Context, endpoint agentEndpoint) error {
	parts := partsOf(endpoint)

	// EnsureDomainExists checks if the domain exists, creates it if needed, and sets conditions/domainRef
	domainResult, err := r.DomainManager.EnsureDomainExists(ctx, endpoint)
	if err != nil {
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	// Populate status.trafficPolicy from the spec up front so a failure path
	// reflects what the user is trying to attach rather than a stale success
	// value from a prior generation. Resolve manages the condition's False
	// path; we MarkApplied below only after the downstream create succeeds.
	parts.status.AttachedTrafficPolicy = trafficpolicypkg.IntendedSource(endpoint.GetTrafficPolicyCfg())
	tpResult, err := r.TrafficPolicyManager.Resolve(ctx, endpoint)
	if err != nil {
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}
	parts.status.AttachedTrafficPolicy = tpResult.Source

	clientCerts, err := r.getClientCerts(ctx, endpoint, parts.clientCertificateRefs)
	if err != nil {
		setEndpointCreatedCondition(endpoint, false, ReasonConfigError, fmt.Sprintf("Failed to get client certificates: %v", err))
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	agentTLS, err := r.getAgentTLSTermination(ctx, endpoint, parts.tlsTermination)
	if err != nil {
		setEndpointCreatedCondition(endpoint, false, ReasonConfigError, fmt.Sprintf("Failed to get TLS termination config: %v", err))
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	// Create the endpoint
	tunnelName := r.statusID(client.ObjectKeyFromObject(endpoint))
	result, err := r.AgentDriver.CreateAgentEndpoint(ctx, tunnelName, parts.driverSpec, tpResult.Policy, clientCerts, agentTLS)
	if err != nil {
		// Mark the endpoint as failed creation
		setEndpointCreatedCondition(endpoint, false, ReasonNgrokAPIError, fmt.Sprintf("Failed to create endpoint: %v", err))
		// If error indicates traffic policy issue, surface it via the shared condition too.
		if tpResult.Policy != "" && ngrokapi.IsTrafficPolicyError(err.Error()) {
			r.TrafficPolicyManager.SetError(endpoint, ngrokapi.SanitizeErrorMessage(err.Error()))
		}
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	// Downstream create succeeded — record the policy as applied (true)
	// and mark the endpoint as created.
	r.TrafficPolicyManager.MarkApplied(endpoint)
	setEndpointCreatedCondition(endpoint, true, ReasonEndpointCreated, "Endpoint successfully created")

	return r.updateStatus(ctx, endpoint, result, domainResult, nil)
}

// statusID is the agent driver's tunnel name for the AgentEndpoints at key.
// Both twins map to the same tunnel.
func (r *AgentEndpointReconciler) statusID(key client.ObjectKey) string {
	return fmt.Sprintf("%s/%s", key.Namespace, key.Name)
}

// listAgentEndpoints lists the AgentEndpoints of both kinds matching opts.
func (r *AgentEndpointReconciler) listAgentEndpoints(ctx context.Context, opts ...client.ListOption) ([]agentEndpoint, error) {
	var out []agentEndpoint

	var v1List ngrokv1.AgentEndpointList
	if err := r.Client.List(ctx, &v1List, opts...); err != nil {
		return nil, err
	}
	for i := range v1List.Items {
		out = append(out, &v1List.Items[i])
	}

	// LEGACY-agentendpoint-kind: BEGIN
	var legacyList ngrokv1alpha1.AgentEndpointList
	if err := r.Client.List(ctx, &legacyList, opts...); err != nil {
		return nil, err
	}
	for i := range legacyList.Items {
		out = append(out, &legacyList.Items[i])
	}
	// LEGACY-agentendpoint-kind: END

	return out, nil
}

// findAgentEndpointForTrafficPolicy searches for any AgentEndpoint CRs that
// reference a particular TrafficPolicy in the policy's namespace.
//
// The mapper is kind-agnostic by construction: it accepts anything satisfying
// trafficpolicypkg.TrafficPolicyResource, which today means both the canonical
// ngrok.com/v1 TrafficPolicy and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy. That works because the
// endpoint index key is composite namespace/name only, with no kind component.
// Testing against the interface rather than a list of concrete types means
// this function needs no edit when the legacy kind is dropped.
func (r *AgentEndpointReconciler) findAgentEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	if _, ok := o.(trafficpolicypkg.TrafficPolicyResource); !ok {
		return nil
	}

	// Use the shared composite-key index to find AgentEndpoints that reference
	// this TrafficPolicy in its namespace.
	endpoints, err := r.listAgentEndpoints(ctx, client.MatchingFields{trafficpolicypkg.RefIndex: trafficpolicypkg.LookupKey(o)})
	if err != nil {
		r.Log.Error(err, "failed to list AgentEndpoints using index")
		return nil
	}

	var requests []ctrl.Request
	for _, ep := range endpoints {
		requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(ep)})
	}
	return requests
}

func (r *AgentEndpointReconciler) findAgentEndpointForSecret(ctx context.Context, o client.Object) []ctrl.Request {
	secret, ok := o.(*v1.Secret)
	if !ok {
		return nil
	}

	secretKey := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)

	// An AgentEndpoint can reference a Secret via either ClientCertificateRefs
	// (upstream client certs) or TLSTermination (agent-side server cert / mTLS CAs),
	// each backed by a separate field index. Query both, then dedupe by NamespacedName.
	seen := map[client.ObjectKey]struct{}{}
	var requests []ctrl.Request

	for _, indexName := range []string{clientCertificateRefsIndex, tlsTerminationSecretsIndex} {
		endpoints, err := r.listAgentEndpoints(ctx, client.MatchingFields{indexName: secretKey})
		if err != nil {
			r.Log.Error(err, "failed to list AgentEndpoints using index", "index", indexName)
			continue
		}
		for _, ep := range endpoints {
			key := client.ObjectKeyFromObject(ep)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			requests = append(requests, ctrl.Request{NamespacedName: key})
		}
	}

	return requests
}

// findAgentEndpointsForDomain searches for any AgentEndpoint CRs that reference a particular Domain
func (r *AgentEndpointReconciler) findAgentEndpointsForDomain(ctx context.Context, o client.Object) []ctrl.Request {
	domain, ok := o.(*ingressv1alpha1.Domain)
	if !ok {
		return nil
	}

	endpoints, err := r.listAgentEndpoints(ctx, client.InNamespace(domain.Namespace))
	if err != nil {
		return nil
	}

	// Get the domain name from the Domain CR
	domainName := domain.Spec.Domain
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domainName)

	var requests []ctrl.Request
	// First match by domainRef
	for _, ep := range endpoints {
		if ep.GetDomainRef().Matches(domain) {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(ep),
			})
			continue
		}

		// ALSO match by URL - critical for catching domains created by old pods during rolling updates
		// When old pod creates domain, domainRef might not be set yet in the cached view
		if ep.GetURL() != "" {
			parsedURL, err := util.ParseAndSanitizeEndpointURL(ep.GetURL(), true)
			if err == nil && ingressv1alpha1.HyphenatedDomainNameFromURL(parsedURL.Hostname()) == hyphenatedDomain {
				requests = append(requests, ctrl.Request{
					NamespacedName: client.ObjectKeyFromObject(ep),
				})
			}
		}
	}
	return requests
}

// getClientCerts retrieves client certificates for upstream TLS connections.
// Updates the passed in AgentEndpoint with the status conditions based on the results.
func (r *AgentEndpointReconciler) getClientCerts(ctx context.Context, aep agentEndpoint, refs []ngrokv1.K8sObjectRef) ([]tls.Certificate, error) {
	if refs == nil {
		return nil, nil // Nothing to fetch
	}

	ret := []tls.Certificate{}
	for _, clientCertRef := range refs {
		key := client.ObjectKey{Name: clientCertRef.Name, Namespace: aep.GetNamespace()}

		// Attempt to get the Secret from the API server
		certSecret := &v1.Secret{}
		if err := r.Client.Get(ctx, key, certSecret); err != nil {
			r.Recorder.Eventf(aep, nil, v1.EventTypeWarning, "SecretNotFound", "Reconcile", fmt.Sprintf("Failed to find Secret %s/%s for clientCertificateRef: %v", key.Namespace, key.Name, err))
			return nil, err
		}

		certData, exists := certSecret.Data["tls.crt"]
		if !exists {
			return nil, fmt.Errorf("tls.crt data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
		}
		keyData, exists := certSecret.Data["tls.key"]
		if !exists {
			return nil, fmt.Errorf("tls.key data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
		}

		cert, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse TLS certificate AgentEndpoint clientCertRef %q: %w", fmt.Sprintf("%s.%s", key.Name, key.Namespace), err)
		}

		ret = append(ret, cert)
	}
	return ret, nil
}

// getAgentTLSTermination resolves spec.tlsTermination into the driver-facing
// AgentTLSTermination struct. Returns nil when tlsTermination is not configured.
// Caller is responsible for surfacing errors via ReasonConfigError.
func (r *AgentEndpointReconciler) getAgentTLSTermination(ctx context.Context, aep agentEndpoint, tlsTermination *ngrokv1.EndpointTLSTermination) (*agent.AgentTLSTermination, error) {
	if tlsTermination == nil {
		return nil, nil
	}

	serverCert, err := r.getServerCert(ctx, aep, tlsTermination.ServerCertificateRef)
	if err != nil {
		return nil, err
	}

	out := &agent.AgentTLSTermination{ServerCert: serverCert}

	if tlsTermination.MutualTLS != nil {
		clientCAs, err := r.getClientCAs(ctx, aep, tlsTermination.MutualTLS.ClientCAsRef)
		if err != nil {
			return nil, err
		}
		out.ClientCAs = clientCAs
		out.ClientAuth = clientAuthForMode(ctx, tlsTermination.MutualTLS.Mode)
	}

	return out, nil
}

// getServerCert fetches the agent's server certificate (tls.crt + tls.key) for
// TLS termination from the referenced kubernetes.io/tls Secret.
func (r *AgentEndpointReconciler) getServerCert(ctx context.Context, aep agentEndpoint, ref ngrokv1.K8sObjectRef) (*tls.Certificate, error) {
	key := client.ObjectKey{Name: ref.Name, Namespace: aep.GetNamespace()}

	secret := &v1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		r.Recorder.Eventf(aep, nil, v1.EventTypeWarning, "SecretNotFound", "Reconcile", fmt.Sprintf("Failed to find Secret %s/%s for tlsTermination.serverCertificateRef: %v", key.Namespace, key.Name, err))
		return nil, err
	}

	certData, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf("tls.crt data is missing from AgentEndpoint tlsTermination.serverCertificateRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}
	keyData, ok := secret.Data["tls.key"]
	if !ok {
		return nil, fmt.Errorf("tls.key data is missing from AgentEndpoint tlsTermination.serverCertificateRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS certificate AgentEndpoint tlsTermination.serverCertificateRef %q: %w", fmt.Sprintf("%s.%s", key.Name, key.Namespace), err)
	}
	return &cert, nil
}

// getClientCAs builds the mTLS client-CA pool from the referenced Secret's ca.crt key.
func (r *AgentEndpointReconciler) getClientCAs(ctx context.Context, aep agentEndpoint, ref ngrokv1.K8sObjectRef) (*x509.CertPool, error) {
	key := client.ObjectKey{Name: ref.Name, Namespace: aep.GetNamespace()}

	secret := &v1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		r.Recorder.Eventf(aep, nil, v1.EventTypeWarning, "SecretNotFound", "Reconcile", fmt.Sprintf("Failed to find Secret %s/%s for tlsTermination.mutualTLS.clientCAsRef: %v", key.Namespace, key.Name, err))
		return nil, err
	}

	caData, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt data is missing from AgentEndpoint tlsTermination.mutualTLS.clientCAsRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("no PEM-encoded certificates found in AgentEndpoint tlsTermination.mutualTLS.clientCAsRef %q ca.crt", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}
	return pool, nil
}

// clientAuthForMode maps the CRD mTLS mode to tls.ClientAuthType. Defaults to
// RequireAndVerifyClientCert when the mode is empty (matches the CRD default).
func clientAuthForMode(ctx context.Context, mode ngrokv1.EndpointMutualTLSMode) tls.ClientAuthType {
	switch mode {
	case ngrokv1.EndpointMutualTLSModeRequest:
		return tls.VerifyClientCertIfGiven
	case ngrokv1.EndpointMutualTLSModeRequire, "":
		return tls.RequireAndVerifyClientCert
	default:
		ctrl.LoggerFrom(ctx).Info("unknown mutualTLS mode, defaulting to RequireAndVerifyClientCert", "mode", mode)
		return tls.RequireAndVerifyClientCert
	}
}

// updateStatus updates the endpoint status fields, calculates Ready condition, and writes to k8s API.
// The TrafficPolicyApplied condition is maintained by the trafficpolicy.Manager
// during Resolve; the caller writes status.trafficPolicy from Result.Source.
func (r *AgentEndpointReconciler) updateStatus(ctx context.Context, endpoint agentEndpoint, result *agent.EndpointResult, domainResult *domainpkg.DomainResult, statusErr error) error {
	// Update status fields if we have a result
	if result != nil {
		partsOf(endpoint).status.AssignedURL = result.URL
	}

	// Calculate overall Ready condition based on other conditions and domain status
	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	// Write status to k8s API
	if err := r.writeStatus(ctx, endpoint, statusErr); err != nil {
		return err
	}

	// Requeue if domain is not ready (fallback to watch for convergence)
	if domainResult != nil {
		return domainResult.RequeueError()
	}
	return nil
}

// writeStatus writes ep's status with BaseController's retry-on-conflict
// logic, and returns origErr (or a StatusError if the write failed).
func (r *AgentEndpointReconciler) writeStatus(ctx context.Context, ep agentEndpoint, origErr error) error {
	switch obj := ep.(type) {
	case *ngrokv1.AgentEndpoint:
		return (&controller.BaseController[*ngrokv1.AgentEndpoint]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	case *ngrokv1alpha1.AgentEndpoint: // LEGACY-agentendpoint-kind: delete this case at cleanup.
		return (&controller.BaseController[*ngrokv1alpha1.AgentEndpoint]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	default:
		return fmt.Errorf("unsupported AgentEndpoint type %T", ep)
	}
}

// agentEndpointPair holds the AgentEndpoints of both kinds that share one
// namespace/name. Either may be nil.
type agentEndpointPair struct {
	key client.ObjectKey
	v1  *ngrokv1.AgentEndpoint
	// LEGACY-agentendpoint-kind: delete at cleanup.
	legacy *ngrokv1alpha1.AgentEndpoint
}

func loadAgentEndpointPair(ctx context.Context, c client.Reader, key client.ObjectKey) (*agentEndpointPair, error) {
	pair := &agentEndpointPair{key: key}

	canonical := &ngrokv1.AgentEndpoint{}
	if err := c.Get(ctx, key, canonical); err == nil {
		pair.v1 = canonical
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	legacy := &ngrokv1alpha1.AgentEndpoint{}
	if err := c.Get(ctx, key, legacy); err == nil {
		pair.legacy = legacy
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	return pair, nil
}

// all returns the objects that exist, v1 first.
func (p *agentEndpointPair) all() []agentEndpoint {
	var out []agentEndpoint
	if p.v1 != nil {
		out = append(out, p.v1)
	}
	if p.legacy != nil {
		out = append(out, p.legacy)
	}
	return out
}

// surviving returns the objects that exist and aren't being deleted.
func (p *agentEndpointPair) surviving() []agentEndpoint {
	var out []agentEndpoint
	for _, o := range p.all() {
		if controller.IsUpsert(o) {
			out = append(out, o)
		}
	}
	return out
}

// deleting returns the objects that are being deleted.
func (p *agentEndpointPair) deleting() []agentEndpoint {
	var out []agentEndpoint
	for _, o := range p.all() {
		if controller.IsDelete(o) {
			out = append(out, o)
		}
	}
	return out
}

func (p *agentEndpointPair) survivingV1() *ngrokv1.AgentEndpoint {
	if p.v1 != nil && controller.IsUpsert(p.v1) {
		return p.v1
	}
	return nil
}

func (p *agentEndpointPair) survivingLegacy() *ngrokv1alpha1.AgentEndpoint {
	if p.legacy != nil && controller.IsUpsert(p.legacy) {
		return p.legacy
	}
	return nil
}
