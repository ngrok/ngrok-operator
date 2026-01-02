package trafficpolicy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	ActionType_SetVars          ActionType = "set-vars"
	ActionType_OAuth            ActionType = "oauth"
	ActionType_OIDC             ActionType = "openid-connect"
	ActionType_RateLimit        ActionType = "rate-limit"
	ActionType_Redirect         ActionType = "redirect"
	ActionType_RemoveHeaders    ActionType = "remove-headers"
	ActionType_RestrictIPs      ActionType = "restrict-ips"
	ActionType_TerminateTLS     ActionType = "terminate-tls"
	ActionType_URLRewrite       ActionType = "url-rewrite"
	ActionType_VerifyWebhook    ActionType = "verify-webhook"
)

func ActionTypes() []ActionType {
	return []ActionType{
		ActionType_AddHeaders,
		ActionType_BasicAuth,
		ActionType_CircuitBreaker,
		ActionType_CompressResponse,
		ActionType_CustomResponse,
		ActionType_Deny,
		ActionType_ForwardInternal,
		ActionType_JWTValidation,
		ActionType_Log,
		ActionType_SetVars,
		ActionType_OAuth,
		ActionType_OIDC,
		ActionType_RateLimit,
		ActionType_Redirect,
		ActionType_RemoveHeaders,
		ActionType_RestrictIPs,
		ActionType_TerminateTLS,
		ActionType_URLRewrite,
		ActionType_VerifyWebhook,
	}
}

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

// validTrafficPolicyKeys are the only keys allowed at the top level of a traffic policy document.
// Any other keys indicate a malformed policy that could lead to security issues if silently ignored.
var validTrafficPolicyKeys = map[string]bool{
	"on_http_request":  true,
	"on_http_response": true,
	"on_tcp_connect":   true,
}

// NewTrafficPolicyFromJSON creates a new TrafficPolicy from a JSON byte array.
// It validates that all top-level keys are known phase names and returns an error
// if unknown keys are present (which would otherwise be silently ignored).
func NewTrafficPolicyFromJSON(data []byte) (*TrafficPolicy, error) {
	if len(data) == 0 {
		return NewTrafficPolicy(), nil
	}

	// First unmarshal to a map to detect unknown keys
	var rawPolicy map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawPolicy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal traffic policy: %w. raw traffic policy: %v", err, string(data))
	}

	// Check for unknown keys that would be silently dropped
	var unknownKeys []string
	for key := range rawPolicy {
		if !validTrafficPolicyKeys[key] {
			unknownKeys = append(unknownKeys, key)
		}
	}

	if len(unknownKeys) > 0 {
		sort.Strings(unknownKeys)
		return nil, fmt.Errorf("traffic policy contains unknown keys that would be ignored: %s; valid keys are: on_http_request, on_http_response, on_tcp_connect",
			strings.Join(unknownKeys, ", "))
	}

	// Now unmarshal into the typed struct
	tp := NewTrafficPolicy()
	if err := json.Unmarshal(data, tp); err != nil {
		return nil, fmt.Errorf("failed to parse traffic policy: %w. traffic policy: %v", err, string(data))
	}
	return tp, nil
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

// NewCustomResponseAction creates a new action that enables you to return a hard-coded response back to the client that made a request to your endpoint.
func NewCustomResponseAction(statusCode int, content string, headers map[string]string) Action {
	config := struct {
		StatusCode int               `json:"status_code"`
		Content    string            `json:"content,omitempty"`
		Headers    map[string]string `json:"headers,omitempty"`
	}{
		StatusCode: statusCode,
		Content:    content,
		Headers:    headers,
	}
	return Action{
		Type:   ActionType_CustomResponse,
		Config: config,
	}
}

// OAuthConfig is the configuration for protecting an endpoint with OAuth.
type OAuthConfig struct {
	// The name of the OAuth identity provider to be used for authentication.
	Provider string `json:"provider,omitempty"`
	// Allow CORS preflight requests to bypass authentication checks. Enable if the endpoint needs to be accessible via CORS.
	AllowCORSPreflight *bool `json:"allow_cors_preflight,omitempty"`
	// Sets the allowed domain for the auth cookie.
	AuthCookieDomain *string `json:"auth_cookie_domain,omitempty"`
	// Unique authentication identifier for this provider. This value will be used for the cookie, redirect, authentication and logout purposes.
	AuthID *string `json:"auth_id,omitempty"`
	// A map of additional URL parameters to apply to the authorization endpoint URL.
	AuthzURLParams map[string]string `json:"authz_url_params,omitempty"`
	// Your OAuth app's client ID. Set to nil if you want to use ngrok’s managed application.
	ClientID *string `json:"client_id,omitempty"`
	// Your OAuth app's client secret. Set to nil if you want to use a managed application.
	ClientSecret *string `json:"client_secret,omitempty"`
	// Defines the period of inactivity after which a user's session is automatically ended, requiring re-authentication.
	IdleSessionTimeout *time.Duration `json:"idle_session_timeout,omitempty"`
	// Defines the maximum lifetime of a session regardless of activity.
	MaxSessionDuration *time.Duration `json:"max_session_duration,omitempty"`
	// A list of additional scopes to request when users authenticate with the identity provider.
	Scopes []string `json:"scopes,omitempty"`
	// How often should ngrok refresh data about the authenticated user from the identity provider.
	UserinfoRefreshInterval *time.Duration `json:"userinfo_refresh_interval,omitempty"`
}

// NewOAuthAction creates a new OAuth action that restricts access to only authorized users by enforcing OAuth
// through an identity provider of your choice.
func NewOAuthAction(config OAuthConfig) Action {
	return Action{
		Type:   ActionType_OAuth,
		Config: config,
	}
}

// OIDCConfig is the configuration for protecting an endpoint with OIDC.
type OIDCConfig struct {
	// The base URL of the Open ID provider that serves an OpenID Provider Configuration Document at /.well-known/openid-configuration.
	IssuerURL string `json:"issuer_url,omitempty"`
	// Allow CORS preflight requests to bypass authentication checks. Enable if the endpoint needs to be accessible via CORS.
	AllowCORSPreflight *bool `json:"allow_cors_preflight,omitempty"`
	// Sets the allowed domain for the auth cookie.
	AuthCookieDomain *string `json:"auth_cookie_domain,omitempty"`
	// Unique authentication identifier for this provider. This value will be used for the cookie, redirect, authentication and logout purposes.
	AuthID *string `json:"auth_id,omitempty"`
	// A map of additional URL parameters to apply to the authorization endpoint URL.
	AuthzURLParams map[string]string `json:"authz_url_params,omitempty"`
	// Your OAuth app's client ID. Set to nil if you want to use ngrok’s managed application.
	ClientID *string `json:"client_id,omitempty"`
	// Your OAuth app's client secret. Set to nil if you want to use a managed application.
	ClientSecret *string `json:"client_secret,omitempty"`
	// Defines the period of inactivity after which a user's session is automatically ended, requiring re-authentication.
	IdleSessionTimeout *time.Duration `json:"idle_session_timeout,omitempty"`
	// Defines the maximum lifetime of a session regardless of activity.
	MaxSessionDuration *time.Duration `json:"max_session_duration,omitempty"`
	// A list of additional scopes to request when users authenticate with the identity provider.
	Scopes []string `json:"scopes,omitempty"`
	// How often should ngrok refresh data about the authenticated user from the identity provider.
	UserinfoRefreshInterval *time.Duration `json:"userinfo_refresh_interval,omitempty"`
}

// NewOIDCAction creates a new OIDC action that restricts access to only authorized users by enforcing OIDC
// through an identity provider of your choice.
func NewOIDCAction(config OIDCConfig) Action {
	return Action{
		Type:   ActionType_OIDC,
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
