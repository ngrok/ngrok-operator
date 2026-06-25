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

// Package trafficpolicy provides shared traffic-policy resolution and
// condition management for endpoint controllers. It mirrors the layout of
// internal/domain: a single Manager that both CloudEndpoint and AgentEndpoint
// controllers call to look up the canonical policy referenced by their spec
// (inline or targetRef) and to populate the TrafficPolicyApplied status
// condition. The manager owns the condition only; per-endpoint status fields
// that summarize the attached policy (e.g. AgentEndpointStatus.AttachedTrafficPolicy)
// are written by the calling controller from Result.Source.
//
// Legacy field handling (CloudEndpoint's deprecated spec.trafficPolicyName and
// nested spec.trafficPolicy.policy) lives in the CloudEndpoint controller;
// this package only sees the canonical shape via EndpointWithTrafficPolicy.
package trafficpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

const (
	// ConditionTrafficPolicy is the condition type recorded on endpoints to
	// reflect the result of resolving their referenced TrafficPolicy.
	ConditionTrafficPolicy = "TrafficPolicyApplied"
)

const (
	ReasonTrafficPolicyApplied = "TrafficPolicyApplied"
	ReasonTrafficPolicyError   = "TrafficPolicyError"
)

const (
	// SourceNone is the status value when no traffic policy is configured.
	SourceNone = "none"
	// SourceInline is the status value when an inline policy is used.
	SourceInline = "inline"
)

// ErrInvalidConfig is returned when the TrafficPolicyCfg union is in an
// invalid state. The CRD's CEL rule should prevent this in practice; the
// runtime check exists as a defense in depth and to make tests easier.
var ErrInvalidConfig = errors.New("invalid TrafficPolicy configuration: exactly one of inline or targetRef must be set")

// ErrTrafficPolicyNotFound is returned when a referenced NgrokTrafficPolicy
// does not exist. It is a terminal (non-retryable) error: callers set the
// TrafficPolicyApplied condition to False and do not requeue, relying on the
// TrafficPolicy watch to re-enqueue the endpoint when the policy is
// (re)created. This avoids the exponential-backoff error loop that requeuing
// a NotFound would otherwise cause.
var ErrTrafficPolicyNotFound = errors.New("referenced TrafficPolicy not found")

// ErrInvalidPolicyJSON is returned when a policy's bytes are not valid JSON.
// Like ErrInvalidConfig this is terminal: a malformed policy cannot be fixed
// by retrying, so callers surface it and do not requeue. It is reachable only
// by clients that bypass CRD admission validation.
var ErrInvalidPolicyJSON = errors.New("TrafficPolicy contains invalid JSON")

// Result is what the Manager returns to callers after resolving a policy.
// Policy is the JSON string handed to the ngrok API / agent SDK. Source
// identifies the resolved attachment ("inline", "none", or the referenced
// TrafficPolicy "name" / "namespace/name") and is suitable for writing
// into an endpoint kind's status summary field when that kind has one.
type Result struct {
	Policy string
	Source string
}

// Manager resolves TrafficPolicyCfg values into JSON strings while keeping
// the endpoint's TrafficPolicyApplied condition and attached-policy status
// in sync. A single Manager instance is safe for concurrent use.
type Manager struct {
	Client   client.Client
	Recorder events.EventRecorder
}

// NewManager returns a Manager that resolves traffic policies via the
// supplied client and emits events through the supplied recorder.
func NewManager(c client.Client, r events.EventRecorder) *Manager {
	return &Manager{Client: c, Recorder: r}
}

// Resolve produces the policy JSON for the endpoint. The Result.Source value
// identifies the resolved attachment ("inline", "none", or the referenced
// TrafficPolicy name); callers that maintain a status field summarizing the
// attachment write it themselves from Result.Source.
//
// Resolve only manages the negative side of the TrafficPolicyApplied
// condition. When the endpoint has no policy configured, Resolve removes any
// prior condition and returns Result{Source: "none"}. On any failure (CEL
// mismatch, missing referenced TrafficPolicy, JSON marshal error, etc.)
// Resolve sets the condition to False with the matching reason and returns
// a non-nil error. The positive case (condition=True) is the caller's
// responsibility — call MarkApplied after the downstream Create/Update
// succeeds so the condition reflects "really applied" rather than "resolved
// and about to be applied".
func (m *Manager) Resolve(ctx context.Context, ep ngrokv1alpha1.EndpointWithTrafficPolicy) (*Result, error) {
	cfg := ep.GetTrafficPolicyCfg()
	if cfg == nil {
		meta.RemoveStatusCondition(ep.GetConditions(), ConditionTrafficPolicy)
		return &Result{Source: SourceNone}, nil
	}

	if !exactlyOneSet(cfg) {
		m.setCondition(ep, false, ReasonTrafficPolicyError, ErrInvalidConfig.Error())
		return nil, ErrInvalidConfig
	}

	switch cfg.Type() {
	case ngrokv1alpha1.TrafficPolicyCfgType_Inline:
		policy, err := marshalInline(cfg.Inline)
		if err != nil {
			m.setCondition(ep, false, ReasonTrafficPolicyError, err.Error())
			return nil, err
		}
		return &Result{Policy: policy, Source: SourceInline}, nil

	case ngrokv1alpha1.TrafficPolicyCfgType_K8sRef:
		policy, err := m.resolveRef(ctx, ep, cfg.Reference)
		if err != nil {
			m.setCondition(ep, false, ReasonTrafficPolicyError, err.Error())
			return nil, err
		}
		return &Result{Policy: policy, Source: IntendedSource(cfg)}, nil

	default:
		m.setCondition(ep, false, ReasonTrafficPolicyError, ErrInvalidConfig.Error())
		return nil, ErrInvalidConfig
	}
}

// MarkApplied flips the TrafficPolicyApplied condition to True. Callers
// invoke this after a successful downstream Create/Update so the condition
// truly reflects "applied" rather than the looser "resolved" state Resolve
// alone can guarantee.
func (m *Manager) MarkApplied(ep ngrokv1alpha1.EndpointWithTrafficPolicy) {
	if ep.GetTrafficPolicyCfg() == nil {
		// No policy configured; Resolve already removed the condition.
		return
	}
	m.setCondition(ep, true, ReasonTrafficPolicyApplied, "Traffic policy successfully applied")
}

// IntendedSource returns the value the controller should write into the
// endpoint's status summary field ("inline", "none", or the referenced
// policy's name) given the canonical config. It mirrors the Source value
// Resolve returns so controllers can populate the status field up-front,
// before resolution may fail, without duplicating logic.
func IntendedSource(cfg *ngrokv1alpha1.TrafficPolicyCfg) string {
	if cfg == nil {
		return SourceNone
	}
	if cfg.Reference != nil {
		return cfg.Reference.Name
	}
	if cfg.Inline != nil {
		return SourceInline
	}
	return SourceNone
}

// resolveRef fetches the referenced NgrokTrafficPolicy and marshals its
// policy JSON. The policy is always read from the endpoint's own namespace;
// cross-namespace references are not supported.
func (m *Manager) resolveRef(ctx context.Context, ep ngrokv1alpha1.EndpointWithTrafficPolicy, ref *ngrokv1alpha1.K8sObjectRef) (string, error) {
	key := client.ObjectKey{Namespace: ep.GetNamespace(), Name: ref.Name}
	log := ctrl.LoggerFrom(ctx).WithValues("trafficPolicy", key)

	tp := &ngrokv1alpha1.NgrokTrafficPolicy{}
	if err := m.Client.Get(ctx, key, tp); err != nil {
		if apierrors.IsNotFound(err) {
			if m.Recorder != nil {
				m.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "TrafficPolicyNotFound", "Reconcile",
					fmt.Sprintf("Failed to find TrafficPolicy %s/%s: %v", key.Namespace, key.Name, err))
			}
			return "", fmt.Errorf("%w: %s", ErrTrafficPolicyNotFound, key)
		}
		// Transient/unexpected error — let the caller requeue with backoff.
		return "", err
	}

	policy, err := marshalInline(tp.Spec.Policy)
	if err != nil {
		log.Error(err, "failed to marshal TrafficPolicy JSON")
		return "", err
	}
	return policy, nil
}

// marshalInline returns the JSON string form of a json.RawMessage, validating
// that the contents are well-formed JSON. CRD admission accepts arbitrary
// objects under the policy field (it's PreserveUnknownFields + schemaless),
// but malformed raw bytes can still reach the controller from clients that
// bypass admission validation; we surface that here rather than passing
// garbage to the ngrok API.
func marshalInline(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	if !json.Valid(raw) {
		return "", ErrInvalidPolicyJSON
	}
	return string(raw), nil
}

// exactlyOneSet returns true when exactly one of the union fields is set.
// Used as a defense-in-depth check; the CRD's CEL rule enforces the same
// invariant at admission time.
func exactlyOneSet(cfg *ngrokv1alpha1.TrafficPolicyCfg) bool {
	count := 0
	if cfg.Inline != nil {
		count++
	}
	if cfg.Reference != nil {
		count++
	}
	return count == 1
}

// SetError marks the TrafficPolicyApplied condition as False with the
// provided message. Use after a downstream call (e.g. the ngrok API) reports
// that the policy itself caused the failure, so the condition reflects the
// real reason rather than a stale "applied" state from Resolve.
func (m *Manager) SetError(ep ngrokv1alpha1.EndpointWithTrafficPolicy, message string) {
	m.setCondition(ep, false, ReasonTrafficPolicyError, message)
}

// setCondition writes the TrafficPolicyApplied condition on the endpoint.
func (m *Manager) setCondition(ep ngrokv1alpha1.EndpointWithTrafficPolicy, applied bool, reason, message string) {
	status := metav1.ConditionTrue
	if !applied {
		status = metav1.ConditionFalse
	}

	meta.SetStatusCondition(ep.GetConditions(), metav1.Condition{
		Type:               ConditionTrafficPolicy,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ep.GetGeneration(),
	})
}
