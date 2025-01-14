package trafficpolicy

import (
	"encoding/json"
	"time"
)

// ActionType is a type of action that can be taken. Ref: https://ngrok.com/docs/traffic-policy/actions/
type ActionType string

// Expression is a string that represents a traffic policy expression.
type Expression string

const (
	ActionType_AddHeaders       ActionType = "add-headers"
	ActionType_BasicAuth        ActionType = "basic-auth"
	ActionType_CircuitBreaker   ActionType = "circuit-breaker"
	ActionType_CompressResponse ActionType = "compress-response"
	ActionType_CustomResponse   ActionType = "custom-response"
	ActionType_Deny             ActionType = "deny"
	ActionType_ForwardInternal  ActionType = "forward-internal"
	ActionType_JWTValidation    ActionType = "jwt-validation"
	ActionType_Log              ActionType = "log"
	ActionType_RateLimit        ActionType = "rate-limit"
	ActionType_Redirect         ActionType = "redirect"
	ActionType_RemoveHeaders    ActionType = "remove-headers"
	ActionType_RestrictIPs      ActionType = "restrict-ips"
	ActionType_TerminateTLS     ActionType = "terminate-tls"
	ActionType_URLRewrite       ActionType = "url-rewrite"
	ActionType_VerifyWebhook    ActionType = "verify-webhook"
)

// TrafficPolicy is the configuration language for handling traffic received by ngrok
// for Edges and Endpoints. Specifically, it allows you to define rules for each
// phase in a connections lifecycle (on_http_request, on_http_response, and tcp_connect). These
// rules containin expressessions that match traffc and actions to take when those expressions match.
//
// Ref: https://ngrok.com/docs/traffic-policy/
type TrafficPolicy struct {
	OnHTTPRequest  []Rule `json:"on_http_request,omitempty"`
	OnHTTPResponse []Rule `json:"on_http_response,omitempty"`
	OnTCPConnect   []Rule `json:"on_tcp_connect,omitempty"`
}

// NewTrafficPolicy creates a new TrafficPolicy with empty rules.
func NewTrafficPolicy() *TrafficPolicy {
	return &TrafficPolicy{
		OnHTTPRequest:  []Rule{},
		OnHTTPResponse: []Rule{},
		OnTCPConnect:   []Rule{},
	}
}

// NewTrafficPolicyFromJSON creates a new TrafficPolicy from a JSON byte array.
func NewTrafficPolicyFromJSON(data []byte) (*TrafficPolicy, error) {
	tp := NewTrafficPolicy()
	err := json.Unmarshal(data, tp)
	return tp, err
}

// AddRuleOnHTTPRequest adds a rule to the OnHTTPRequest phase of the TrafficPolicy.
func (tp *TrafficPolicy) AddRuleOnHTTPRequest(rule Rule) {
	tp.OnHTTPRequest = append(tp.OnHTTPRequest, rule)
}

// AddRuleOnHTTPResponse adds a rule to the OnHTTPResponse phase of the TrafficPolicy.
func (tp *TrafficPolicy) AddRuleOnHTTPResponse(rule Rule) {
	tp.OnHTTPResponse = append(tp.OnHTTPResponse, rule)
}

// AddRuleOnTCPConnect adds a rule to the OnTCPConnect phase of the TrafficPolicy.
func (tp *TrafficPolicy) AddRuleOnTCPConnect(rule Rule) {
	tp.OnTCPConnect = append(tp.OnTCPConnect, rule)
}

func (tp *TrafficPolicy) ContainsAction(actionType ActionType) bool {
	for _, rule := range tp.OnHTTPRequest {
		for _, action := range rule.Actions {
			if action.Type == actionType {
				return true
			}
		}
	}

	for _, rule := range tp.OnHTTPResponse {
		for _, action := range rule.Actions {
			if action.Type == actionType {
				return true
			}
		}
	}

	for _, rule := range tp.OnTCPConnect {
		for _, action := range rule.Actions {
			if action.Type == actionType {
				return true
			}
		}
	}

	return false
}

func (tp *TrafficPolicy) Merge(other *TrafficPolicy) {
	if other == nil || other.IsEmpty() {
		return
	}
	tp.OnHTTPRequest = append(tp.OnHTTPRequest, other.OnHTTPRequest...)
	tp.OnHTTPResponse = append(tp.OnHTTPResponse, other.OnHTTPResponse...)
	tp.OnTCPConnect = append(tp.OnTCPConnect, other.OnTCPConnect...)
}

// IsEmpty returns true if the TrafficPolicy has no rules.
func (tp TrafficPolicy) IsEmpty() bool {
	return len(tp.OnHTTPRequest) == 0 &&
		len(tp.OnHTTPResponse) == 0 &&
		len(tp.OnTCPConnect) == 0
}

// DeepCopy creates a deep copy of a TrafficPolicy.
func (tp *TrafficPolicy) DeepCopy() (*TrafficPolicy, error) {
	// Serialize the original TrafficPolicy to JSON.
	data, err := json.Marshal(tp)
	if err != nil {
		return nil, err
	}

	// Deserialize the JSON back into a new TrafficPolicy.
	var copy TrafficPolicy
	err = json.Unmarshal(data, &copy)
	if err != nil {
		return nil, err
	}

	// Ensure slices are initialized
	if copy.OnHTTPRequest == nil {
		copy.OnHTTPRequest = []Rule{}
	}
	if copy.OnHTTPResponse == nil {
		copy.OnHTTPResponse = []Rule{}
	}
	if copy.OnTCPConnect == nil {
		copy.OnTCPConnect = []Rule{}
	}

	return &copy, nil
}

// A Rule allows you to define how traffic is filtered and processed within a phase. Rules
// consist of expressions and actions. Ref: https://ngrok.com/docs/traffic-policy/concepts/phase-rules/
type Rule struct {
	// I think on the server side, this is always handled as a string, but I set it to any because otherwise, json like this:
	// `name: 404` will fail to marshall/unmarshall even though the server side accepts it.
	Name        any      `json:"name,omitempty"`
	Expressions []string `json:"expressions,omitempty"`
	Actions     []Action `json:"actions"`
}

// An action allows you to manipulate, route, or manage traffic on an endpoint.
// Ref: https://ngrok.com/docs/traffic-policy/actions/
type Action struct {
	Type   ActionType `json:"type"`
	Config any        `json:"config"`
}

// NewAddHeadersAction creates a new action that adds headers to the request(OnHTTPRequest phase) or
// response(OnHTTPResponse phase).
func NewAddHeadersAction(headers map[string]string) Action {
	config := struct {
		Headers map[string]string `json:"headers"`
	}{
		Headers: headers,
	}

	return Action{
		Type:   ActionType_AddHeaders,
		Config: config,
	}
}

// NewRemoveHeadersAction creates a new action that removes headers from the request(OnHTTPRequest phase) or
// response(OnHTTPResponse phase).
func NewRemoveHeadersAction(headers []string) Action {
	config := struct {
		Headers []string `json:"headers"`
	}{
		Headers: headers,
	}

	return Action{
		Type:   ActionType_RemoveHeaders,
		Config: config,
	}
}

// NewCompressResponseAction creates a new action that compresses the response. Can only be used
// in the OnHTTPResponse phase.
func NewCompressResponseAction(algorithms []string) Action {
	config := struct {
		Algorithms []string `json:"algorithms,omitempty"`
	}{
		Algorithms: algorithms,
	}

	return Action{
		Type:   ActionType_CompressResponse,
		Config: config,
	}
}

// NewCicuitBreakerAction creates a new action that rejects requests when the error rate and request volume within a rolling
// window exceeds defined thresholds. Can only be used in the OnHTTPRequest phase.
func NewCircuitBreakerAction(errorThreshold float64, volumeThreshold *uint32, windowDuration *time.Duration, trippedDuration *time.Duration) Action {
	config := struct {
		ErrorThreshold  float64        `json:"error_threshold"`
		VolumeThreshold *uint32        `json:"volume_threshold,omitempty"`
		WindowDuration  *time.Duration `json:"window_duration,omitempty"`
		TrippedDuration *time.Duration `json:"tripped_duration,omitempty"`
	}{
		ErrorThreshold:  errorThreshold,
		VolumeThreshold: volumeThreshold,
		WindowDuration:  windowDuration,
		TrippedDuration: trippedDuration,
	}
	return Action{
		Type:   ActionType_CircuitBreaker,
		Config: config,
	}
}

// NewRestrictIPsActionFromIPPolicies creates a new action that restricts access to a set of IP policies.
// Supported on OnHTTPRequest, OnTCPConnect, and OnHTTPResponse phases.
func NewRestricIPsActionFromIPPolicies(policies []string) Action {
	config := struct {
		IPPolicies []string `json:"ip_policies"`
	}{
		IPPolicies: policies,
	}

	return Action{
		Type:   ActionType_RestrictIPs,
		Config: config,
	}
}

// TLSTerminationConfig is the configuration for terminating TLS on an endpoint.
type TLSTerminationConfig struct {
	MinVersion                      *string  `json:"min_version,omitempty"`
	MaxVersion                      *string  `json:"max_version,omitempty"`
	ServerPrivateKey                *string  `json:"server_private_key,omitempty"`
	ServerCertificate               *string  `json:"server_certificate,omitempty"`
	MutualTLSCertificateAuthorities []string `json:"mutual_tls_certificate_authorities,omitempty"`
	MutualTLSVerificationStrategy   *string  `json:"mutual_tls_verification_strategy,omitempty"`
}

// NewTerminateTLSAction creates a new action that configures how TLS is terminated on the endpoint.
func NewTerminateTLSAction(config TLSTerminationConfig) Action {
	return Action{
		Type:   ActionType_TerminateTLS,
		Config: config,
	}
}

// NewWebhookVerificationAction creates a new action that verifies a webhook request.
func NewWebhookVerificationAction(provider, secret string) Action {
	config := struct {
		Provider string `json:"provider"`
		Secret   string `json:"secret"`
	}{
		Provider: provider,
		Secret:   secret,
	}

	return Action{
		Type:   ActionType_VerifyWebhook,
		Config: config,
	}
}

// NewForwardInternalAction creates a new action that forwards the traffic to an internal endpoint.
func NewForwardInternalAction(url string) Action {
	config := struct {
		URL string `json:"url"`
	}{
		URL: url,
	}

	return Action{
		Type:   ActionType_ForwardInternal,
		Config: config,
	}
}
