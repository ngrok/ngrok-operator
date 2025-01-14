package util

import (
	"context"
	"encoding/json"
	"time"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/resolvers"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"k8s.io/utils/ptr"
)

type ActionType string

const (
	PhaseOnHttpRequest  = "on_http_request"
	PhaseOnHttpResponse = "on_http_response"

	LegacyPhaseInbound  = "inbound"
	LegacyPhaseOutbound = "outbound"

	// backwardsCompatEnabledKey exists because previous implementations would store this value in the CRD and might exist
	// in traffic policy JSON.
	backwardsCompatEnabledKey = "enabled"
)

// These types are different than the v1alpha1 types because:
//    1. These are more generic types, allowing "generated" policy to more easily interact with unstructured user-provided policy.
//    2. Gateway code is now decoupled from module code, which is now deprecated.

type Actions struct {
	EndpointActions []RawAction
}

type EndpointAction struct {
	Type   string    `json:"type"`
	Config RawConfig `json:"config"`
}

type EndpointRule struct {
	Name    string      `json:"name"`
	Actions []RawAction `json:"actions"`
}

// RawRule exists to make generic raw json/map manipulation more legible. It adds no additional functionality on top of RawMessage.
type RawRule = json.RawMessage

// RawAction exists to make generic raw json/map manipulation more legible. It adds no additional functionality on top of RawMessage.
type RawAction = json.RawMessage

// RawConfig exists to make generic raw json/map manipulation more legible. It adds no additional functionality on top of RawMessage.
type RawConfig = json.RawMessage

type TrafficPolicy interface {
	// Merge takes another instance of TrafficPolicy and merges them.
	Merge(TrafficPolicy)
	// MergeRawRule merges the rule into the list of existing rules in the specified phase.
	MergeEndpointRule(rule EndpointRule, phase string) error
	// Deconstruct must be implemented such that two differing underlying implementations can be represented as the same
	// format during merges.
	Deconstruct() map[string][]RawRule
	// ToCRDJson returns a raw json version of the underlying traffic policy document to be saved to an Edge CRD.
	ToCRDJson() (json.RawMessage, error)
	// ToAPIJson() returns a raw json version of the underlying traffic policy document to be sent to the backend API.
	ToAPIJson() (json.RawMessage, error)
	// IsLegacyPolicy determines if the specified policy contains legacy phases.
	IsLegacyPolicy() bool
	// Enabled returns the value if it was present in a traffic policy document, nil if not.
	Enabled() *bool
	// ConvertLegacyPhases explicitly maps old phases to new phases. This doesn't guarantee in a resulting "valid" phase set,
	// so users will need to check the implementation used for this interface.
	ConvertLegacyDirectionsToPhases()
}

func NewTrafficPolicy() TrafficPolicy {
	return &trafficPolicyImpl{
		trafficPolicy: map[string][]RawRule{},
	}
}

func NewTrafficPolicyFromJson(msg json.RawMessage) (TrafficPolicy, error) {
	strippedMsg, enabled, err := filterEnabled(msg)
	if err != nil {
		return nil, err
	}

	var trafficPolicy map[string][]RawRule
	if err := json.Unmarshal(strippedMsg, &trafficPolicy); err != nil {
		return nil, err
	}

	return &trafficPolicyImpl{
		trafficPolicy: trafficPolicy,
		enabled:       enabled,
	}, nil
}

type trafficPolicyImpl struct {
	trafficPolicy map[string][]RawRule
	enabled       *bool
}

// MergeEndpointRule marshals the rule, then merges it into the correct phase within the traffic policy document.
func (t *trafficPolicyImpl) MergeEndpointRule(rule EndpointRule, phase string) error {
	rawRule, err := json.Marshal(&rule)
	if err != nil {
		return err
	}

	mergeSinglePhase(t.trafficPolicy, []RawRule{rawRule}, phase)

	return nil
}

func (t *trafficPolicyImpl) Deconstruct() map[string][]json.RawMessage {
	return t.trafficPolicy
}

// Merge merges addedTP traffic policy into that of the receivers traffic policy. If a phase from the incoming traffic policy
// already exists in the original, the associated rules are appended. If not, the phase is added to the original traffic
// policy.
func (t *trafficPolicyImpl) Merge(addedTP TrafficPolicy) {
	t.mergeEnabled(addedTP.Enabled())

	deconAddedTP := addedTP.Deconstruct()
	originalTP := t.trafficPolicy

	for phase, rules := range deconAddedTP {
		mergeSinglePhase(originalTP, rules, phase)
	}
}

// ToCRDJson creates a json from the traffic policy, but embeds the "enabled" field at the top level. This is necessary
// for backwards compatability where we let users set this.
func (t *trafficPolicyImpl) ToCRDJson() (json.RawMessage, error) {
	// no special processing needed if enabled wasn't set.
	if t.enabled == nil {
		return t.ToAPIJson()
	}

	output := map[string]any{}

	output[backwardsCompatEnabledKey] = *t.enabled

	for k, v := range t.trafficPolicy {
		output[k] = v
	}

	return json.Marshal(&output)
}

// ToCRDJson creates a json blob that is compatible with the `traffic_policy` API. This should be used when sending traffic
// policy to the backend. Unlike ToCRDJson, this does not embed "enabled" data.
func (t *trafficPolicyImpl) ToAPIJson() (json.RawMessage, error) {
	tp := t.trafficPolicy

	return json.Marshal(&tp)
}

// IsLegacyPolicy determines if the configured policy matches the old inbound/outbound format as opposed to the
// current phase-based format.
func (t *trafficPolicyImpl) IsLegacyPolicy() bool {
	if t.trafficPolicy == nil {
		return false
	}

	if _, ok := t.trafficPolicy[LegacyPhaseInbound]; ok {
		return true
	}

	if _, ok := t.trafficPolicy[LegacyPhaseOutbound]; ok {
		return true
	}

	return false
}

func (t *trafficPolicyImpl) Enabled() *bool {
	return t.enabled
}

// ConvertLegacyDirectionsToPhases converts inbound to on_http_request and outbound to on_http_response. This conversion is
// only necessary for the Gateway API, which only supported HTTP when inbound/outbound were valid. If the new phases already
// exist in the traffic policy, the rules are merged into that phase.
func (t *trafficPolicyImpl) ConvertLegacyDirectionsToPhases() {
	if len(t.trafficPolicy) == 0 {
		return
	}

	newMap := map[string][]RawRule{}

	for k, v := range t.trafficPolicy {
		switch k {
		case LegacyPhaseInbound:
			mergeSinglePhase(newMap, v, PhaseOnHttpRequest)
		case LegacyPhaseOutbound:
			mergeSinglePhase(newMap, v, PhaseOnHttpResponse)
		default:
			mergeSinglePhase(newMap, v, k)
		}
	}

	t.trafficPolicy = newMap
}

// mergeEnabled applies the supplied "enabled" value to the traffic policy. If there is a value set for both,
// we set to "false" is present in either.
func (t *trafficPolicyImpl) mergeEnabled(incomingEnabled *bool) {
	if incomingEnabled == nil {
		return
	}

	// original traffic policy had no enabled set, take on new value
	if t.enabled == nil {
		// don't want to copy the pointer, as the value will still be linked to a different policy
		temp := *incomingEnabled
		t.enabled = &temp
	}

	// both being set likely won't happen. However, if we have a mismatch, we should keep it enabled. Otherwise,
	// we might accidentally turn off something important like auth.
	resolvedEnabled := *t.enabled || *incomingEnabled
	t.enabled = &resolvedEnabled
}

// filterEnabled looks for a top level "enabled" in the json, removes it, and returns the value. For the module and gateway
// configuration paths, this value may be present. We need to respect this value and also can't include it in the policy payload.
//
// If the "enabled" field is not present in the message, the message is returned unmodified.
func filterEnabled(msg json.RawMessage) (json.RawMessage, *bool, error) {
	if msg == nil {
		return nil, nil, nil
	}

	var trafficPolicy map[string]any

	if err := json.Unmarshal(msg, &trafficPolicy); err != nil {
		return nil, nil, err
	}

	enabled, err := extractEnabledField(trafficPolicy)
	if err != nil {
		return nil, nil, err
	}

	rawTrafficPolicy, err := json.Marshal(trafficPolicy)
	if err != nil {
		return nil, nil, err
	}

	return rawTrafficPolicy, enabled, nil
}

// extractEnabledField removes the "enabled" field from the map, if present. The value is returned provided the associated value
// is a boolean.
func extractEnabledField(trafficPolicy map[string]any) (*bool, error) {
	if len(trafficPolicy) == 0 {
		return nil, nil
	}

	if enabledSetVal, ok := trafficPolicy["enabled"]; ok {
		delete(trafficPolicy, "enabled")
		var setVal *bool
		if enabled, ok := enabledSetVal.(bool); ok {
			setVal = &enabled
		}

		return setVal, nil
	}

	return nil, nil
}

// mergeEndpointRule adds the rules to the specified phase. If the phase isn't already present, it's added.
func mergeSinglePhase(originalTP map[string][]RawRule, addedRules []RawRule, phase string) {
	if len(addedRules) == 0 {
		return
	}

	if rules, ok := originalTP[phase]; ok {
		originalTP[phase] = append(rules, addedRules...)
		return
	}

	originalTP[phase] = addedRules
}

func NewTrafficPolicyFromModuleset(ctx context.Context, ms *ingressv1alpha1.NgrokModuleSet, secretResolver resolvers.SecretResolver, ipPolicyResolver resolvers.IPPolicyResolver) (*trafficpolicy.TrafficPolicy, error) {
	if ms == nil {
		return nil, nil
	}

	tp := trafficpolicy.NewTrafficPolicy()

	converters := []modulesetConverterFunc{
		// On TCP Connect Rules (IP Restriction & Mutual TLS)
		convertModuleSetIPRestriction(ipPolicyResolver),
		convertModuleSetTLS,
		// On HTTP Request Rules
		convertModuleSetCompression,
		convertModuleSetCircuitBreaker,
		convertModuleSetHeaders,
		convertModuleSetWebhookVerification(secretResolver),
		convertModuleSetSAML,
		convertModuleSetOIDC,
		convertModuleSetOAuth,
	}

	for _, converter := range converters {
		if err := converter(ctx, *ms, tp); err != nil {
			return nil, err
		}
	}

	if tp.IsEmpty() {
		return nil, nil
	}

	return tp, nil
}

type modulesetConverterFunc func(ctx context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error

func convertModuleSetTLS(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.TLSTermination == nil && ms.Modules.MutualTLS == nil {
		return nil
	}

	tlsConfig := trafficpolicy.TLSTerminationConfig{}

	if ms.Modules.TLSTermination != nil {
		tlsConfig.MinVersion = ms.Modules.TLSTermination.MinVersion
	}

	if ms.Modules.MutualTLS != nil {
		tlsConfig.MutualTLSCertificateAuthorities = ms.Modules.MutualTLS.CertificateAuthorities
	}

	rule := trafficpolicy.Rule{
		Actions: []trafficpolicy.Action{
			trafficpolicy.NewTerminateTLSAction(tlsConfig),
		},
	}
	tp.AddRuleOnTCPConnect(rule)

	return nil
}

func convertModuleSetIPRestriction(ipPolicyResolver resolvers.IPPolicyResolver) modulesetConverterFunc {
	return func(ctx context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
		if ms.Modules.IPRestriction == nil {
			return nil
		}

		ipPolicies, err := ipPolicyResolver.ResolveIPPolicyNamesorIds(ctx, ms.Namespace, ms.Modules.IPRestriction.IPPolicies)
		if err != nil {
			return err
		}

		tp.AddRuleOnTCPConnect(
			trafficpolicy.Rule{
				Actions: []trafficpolicy.Action{
					trafficpolicy.NewRestricIPsActionFromIPPolicies(ipPolicies),
				},
			},
		)

		return nil
	}
}

func convertModuleSetCompression(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.Compression == nil {
		return nil
	}

	tp.AddRuleOnHTTPResponse(
		trafficpolicy.Rule{
			Actions: []trafficpolicy.Action{
				trafficpolicy.NewCompressResponseAction(nil),
			},
		},
	)
	return nil
}

func convertModuleSetCircuitBreaker(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.CircuitBreaker == nil {
		return nil
	}

	var volumeThreshold *uint32
	if ms.Modules.CircuitBreaker.VolumeThreshold != 0 {
		volumeThreshold = ptr.To(ms.Modules.CircuitBreaker.VolumeThreshold)
	}

	var windowDuration *time.Duration
	if ms.Modules.CircuitBreaker.RollingWindow.Duration != 0 {
		windowDuration = ptr.To(ms.Modules.CircuitBreaker.RollingWindow.Duration)
	}

	var trippedDuration *time.Duration
	if ms.Modules.CircuitBreaker.TrippedDuration.Duration != 0 {
		trippedDuration = ptr.To(ms.Modules.CircuitBreaker.TrippedDuration.Duration)
	}

	tp.AddRuleOnHTTPRequest(
		trafficpolicy.Rule{
			Actions: []trafficpolicy.Action{
				trafficpolicy.NewCircuitBreakerAction(
					ms.Modules.CircuitBreaker.ErrorThresholdPercentage.AsApproximateFloat64(),
					volumeThreshold,
					windowDuration,
					trippedDuration,
				),
			},
		},
	)

	return nil
}

func convertModuleSetHeaders(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.Headers == nil {
		return nil
	}

	if ms.Modules.Headers.Request != nil {
		actions := []trafficpolicy.Action{}

		if len(ms.Modules.Headers.Request.Remove) > 0 {
			actions = append(actions, trafficpolicy.NewRemoveHeadersAction(ms.Modules.Headers.Request.Remove))
		}

		if len(ms.Modules.Headers.Request.Add) > 0 {
			actions = append(actions, trafficpolicy.NewAddHeadersAction(ms.Modules.Headers.Request.Add))
		}

		if len(actions) > 0 {
			tp.AddRuleOnHTTPRequest(trafficpolicy.Rule{Actions: actions})
		}
	}

	if ms.Modules.Headers.Response != nil {
		actions := []trafficpolicy.Action{}
		if len(ms.Modules.Headers.Response.Remove) > 0 {
			actions = append(actions, trafficpolicy.NewRemoveHeadersAction(ms.Modules.Headers.Response.Remove))
		}

		if len(ms.Modules.Headers.Response.Add) > 0 {
			actions = append(actions, trafficpolicy.NewAddHeadersAction(ms.Modules.Headers.Response.Add))
		}

		if len(actions) > 0 {
			tp.AddRuleOnHTTPResponse(trafficpolicy.Rule{Actions: actions})
		}
	}

	return nil
}
func convertModuleSetWebhookVerification(secretResolver resolvers.SecretResolver) modulesetConverterFunc {
	return func(ctx context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
		if ms.Modules.WebhookVerification == nil {
			return nil
		}

		provider := ms.Modules.WebhookVerification.Provider
		secretRef := ms.Modules.WebhookVerification.SecretRef

		// resolve the secretRef to a secret
		secret, err := secretResolver.GetSecret(ctx, ms.Namespace, secretRef.Name, secretRef.Key)
		if err != nil {
			return err
		}

		tp.AddRuleOnHTTPRequest(
			trafficpolicy.Rule{
				Actions: []trafficpolicy.Action{
					trafficpolicy.NewWebhookVerificationAction(provider, secret),
				},
			},
		)

		return nil
	}
}

func convertModuleSetSAML(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.SAML == nil {
		return nil
	}

	return errors.NewErrModulesetNotConvertibleToTrafficPolicy("SAML module is not supported at this time")
}

func convertModuleSetOIDC(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.OIDC == nil {
		return nil
	}

	return errors.NewErrModulesetNotConvertibleToTrafficPolicy("OIDC module is not supported at this time")
}

func convertModuleSetOAuth(_ context.Context, ms ingressv1alpha1.NgrokModuleSet, tp *trafficpolicy.TrafficPolicy) error {
	if ms.Modules.OAuth == nil {
		return nil
	}

	return errors.NewErrModulesetNotConvertibleToTrafficPolicy("OAuth module is not supported at this time")
}
