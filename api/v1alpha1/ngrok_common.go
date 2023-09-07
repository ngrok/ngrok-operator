package v1alpha1

import (
	"github.com/ngrok/ngrok-api-go/v5"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// common ngrok API/Dashboard fields
type ngrokAPICommon struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by kubernetes-ingress-controller`
	Description string `json:"description,omitempty"`
	// Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`{"owned-by":"kubernetes-ingress-controller"}`
	Metadata string `json:"metadata,omitempty"`
}

// Route Module Types

type EndpointUserAgentFilter struct {
	// a list of regexular expressions that will be used to allow traffic from HTTP Requests
	Allow []string `json:"allow,omitempty"`
	// a list of regexular expressions that will be used to deny traffic from HTTP Requests
	Deny []string `json:"deny,omitempty"`
}

type EndpointCompression struct {
	// Enabled is whether or not to enable compression for this endpoint
	Enabled bool `json:"enabled,omitempty"`
}

type EndpointIPPolicy struct {
	IPPolicies []string `json:"policies,omitempty"`
}

// EndpointRequestHeaders is the configuration for a HTTPSEdgeRoute's request headers
// to be added or removed from the request before it is sent to the backend service.
type EndpointRequestHeaders struct {
	// a map of header key to header value that will be injected into the HTTP Request
	// before being sent to the upstream application server
	Add map[string]string `json:"add,omitempty"`
	// a list of header names that will be removed from the HTTP Request before being
	// sent to the upstream application server
	Remove []string `json:"remove,omitempty"`
}

// EndpointResponseHeaders is the configuration for a HTTPSEdgeRoute's response headers
// to be added or removed from the response before it is sent to the client.
type EndpointResponseHeaders struct {
	// a map of header key to header value that will be injected into the HTTP Response
	// returned to the HTTP client
	Add map[string]string `json:"add,omitempty"`
	// a list of header names that will be removed from the HTTP Response returned to
	// the HTTP client
	Remove []string `json:"remove,omitempty"`
}

type EndpointHeaders struct {
	// Request headers are the request headers module configuration or null
	Request *EndpointRequestHeaders `json:"request,omitempty"`
	// Response headers are the response headers module configuration or null
	Response *EndpointResponseHeaders `json:"response,omitempty"`
}

type EndpointTLSTerminationAtEdge struct {
	// MinVersion is the minimum TLS version to allow for connections to the edge
	MinVersion string `json:"minVersion,omitempty"`
}

type SecretKeyRef struct {
	// Name of the Kubernetes secret
	Name string `json:"name,omitempty"`
	// Key in the secret to use
	Key string `json:"key,omitempty"`
}

type EndpointWebhookVerification struct {
	// a string indicating which webhook provider will be sending webhooks to this
	// endpoint. Value must be one of the supported providers defined at
	// https://ngrok.com/docs/cloud-edge#webhook-verification
	Provider string `json:"provider,omitempty"`
	// SecretRef is a reference to a secret containing the secret used to validate
	// requests from the given provider. All providers except AWS SNS require a secret
	SecretRef *SecretKeyRef `json:"secret,omitempty"`
}

type EndpointCircuitBreaker struct {
	// Duration after which the circuit is tripped to wait before re-evaluating upstream health
	//+kubebuilder:validation:Format=duration
	TrippedDuration v1.Duration `json:"trippedDuration,omitempty"`

	// Statistical rolling window duration that metrics are retained for.
	//+kubebuilder:validation:Format=duration
	RollingWindow v1.Duration `json:"rollingWindow,omitempty"`

	// Integer number of buckets into which metrics are retained. Max 128.
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=128
	NumBuckets uint32 `json:"numBuckets,omitempty"`

	// Integer number of requests in a rolling window that will trip the circuit.
	// Helpful if traffic volume is low.
	VolumeThreshold uint32 `json:"volumeThreshold,omitempty"`

	// Error threshold percentage should be between 0 - 1.0, not 0-100.0
	ErrorThresholdPercentage resource.Quantity `json:"errorThresholdPercentage,omitempty"`
}

type EndpointOIDC struct {
	// Do not enforce authentication on HTTP OPTIONS requests. necessary if you are
	// supporting CORS.
	OptionsPassthrough bool `json:"optionsPassthrough,omitempty"`
	// the prefix of the session cookie that ngrok sets on the http client to cache
	// authentication. default is 'ngrok.'
	CookiePrefix string `json:"cookiePrefix,omitempty"`
	// Duration of inactivity after which if the user has not accessed
	// the endpoint, their session will time out and they will be forced to
	// reauthenticate.
	//+kubebuilder:validation:Format=duration
	InactivityTimeout v1.Duration `json:"inactivityTimeout,omitempty"`
	// The maximum duration of an authenticated session.
	// After this period is exceeded, a user must reauthenticate.
	//+kubebuilder:validation:Format=duration
	MaximumDuration v1.Duration `json:"maximumDuration,omitempty"`
	// URL of the OIDC "OpenID provider". This is the base URL used for discovery.
	Issuer string `json:"issuer,omitempty"`
	// The OIDC app's client ID and OIDC audience.
	ClientID string `json:"clientId,omitempty"`
	// The OIDC app's client secret.
	ClientSecret SecretKeyRef `json:"clientSecret,omitempty"`
	// The set of scopes to request from the OIDC identity provider.
	Scopes []string `json:"scopes,omitempty"`
}

type EndpointSAML struct {
	// Do not enforce authentication on HTTP OPTIONS requests. necessary if you are
	// supporting CORS.
	OptionsPassthrough bool `json:"optionsPassthrough,omitempty"`
	// the prefix of the session cookie that ngrok sets on the http client to cache
	// authentication. default is 'ngrok.'
	CookiePrefix string `json:"cookiePrefix,omitempty"`
	// Duration of inactivity after which if the user has not accessed
	// the endpoint, their session will time out and they will be forced to
	// reauthenticate.
	//+kubebuilder:validation:Format=duration
	InactivityTimeout v1.Duration `json:"inactivityTimeout,omitempty"`
	// The maximum duration of an authenticated session.
	// After this period is exceeded, a user must reauthenticate.
	//+kubebuilder:validation:Format=duration
	MaximumDuration v1.Duration `json:"maximumDuration,omitempty"`
	// The full XML IdP EntityDescriptor. Your IdP may provide this to you as a a file
	// to download or as a URL.
	IdPMetadata string `json:"idpMetadata,omitempty"`
	// If true, indicates that whenever we redirect a user to the IdP for
	// authentication that the IdP must prompt the user for authentication credentials
	// even if the user already has a valid session with the IdP.
	ForceAuthn bool `json:"forceAuthn,omitempty"`
	// If true, the IdP may initiate a login directly (e.g. the user does not need to
	// visit the endpoint first and then be redirected). The IdP should set the
	// RelayState parameter to the target URL of the resource they want the user to be
	// redirected to after the SAML login assertion has been processed.
	AllowIdPInitiated *bool `json:"allowIdpInitiated,omitempty"`
	// If present, only users who are a member of one of the listed groups may access
	// the target endpoint.
	AuthorizedGroups []string `json:"authorizedGroups,omitempty"`
	// Defines the name identifier format the SP expects the IdP to use in its
	// assertions to identify subjects. If unspecified, a default value of
	// urn:oasis:names:tc:SAML:2.0:nameid-format:persistent will be used. A subset of
	// the allowed values enumerated by the SAML specification are supported.
	NameIDFormat string `json:"nameidFormat,omitempty"`
}

type OAuthProviderCommon struct {
	// Do not enforce authentication on HTTP OPTIONS requests. necessary if you are
	// supporting CORS.
	OptionsPassthrough bool `json:"optionsPassthrough,omitempty"`
	// the prefix of the session cookie that ngrok sets on the http client to cache
	// authentication. default is 'ngrok.'
	CookiePrefix string `json:"cookiePrefix,omitempty"`
	// Duration of inactivity after which if the user has not accessed
	// the endpoint, their session will time out and they will be forced to
	// reauthenticate.
	//+kubebuilder:validation:Format=duration
	InactivityTimeout v1.Duration `json:"inactivityTimeout,omitempty"`
	// Integer number of seconds of the maximum duration of an authenticated session.
	// After this period is exceeded, a user must reauthenticate.
	//+kubebuilder:validation:Format=duration
	MaximumDuration v1.Duration `json:"maximumDuration,omitempty"`
	// Duration after which ngrok guarantees it will refresh user
	// state from the identity provider and recheck whether the user is still
	// authorized to access the endpoint. This is the preferred tunable to use to
	// enforce a minimum amount of time after which a revoked user will no longer be
	// able to access the resource.
	//+kubebuilder:validation:Format=duration
	AuthCheckInterval v1.Duration `json:"authCheckInterval,omitempty"`
	// the OAuth app client ID. retrieve it from the identity provider's dashboard
	// where you created your own OAuth app. optional. if unspecified, ngrok will use
	// its own managed oauth application which has additional restrictions. see the
	// OAuth module docs for more details. if present, clientSecret must be present as
	// well.
	ClientID *string `json:"clientId,omitempty"`
	// the OAuth app client secret. retrieve if from the identity provider's dashboard
	// where you created your own OAuth app. optional, see all of the caveats in the
	// docs for clientId.
	ClientSecret *SecretKeyRef `json:"clientSecret,omitempty"`
	// a list of provider-specific OAuth scopes with the permissions your OAuth app
	// would like to ask for. these may not be set if you are using the ngrok-managed
	// oauth app (i.e. you must pass both client_id and client_secret to set scopes)
	Scopes []string `json:"scopes,omitempty"`
	// a list of email addresses of users authenticated by identity provider who are
	// allowed access to the endpoint
	EmailAddresses []string `json:"emailAddresses,omitempty"`
	// a list of email domains of users authenticated by identity provider who are
	// allowed access to the endpoint
	EmailDomains []string `json:"emailDomains,omitempty"`
}

func (opc OAuthProviderCommon) toNgrokEndpointOauth() *ngrok.EndpointOAuth {
	return &ngrok.EndpointOAuth{
		OptionsPassthrough: opc.OptionsPassthrough,
		CookiePrefix:       opc.CookiePrefix,
		InactivityTimeout:  uint32(opc.InactivityTimeout.Duration.Seconds()),
		MaximumDuration:    uint32(opc.MaximumDuration.Duration.Seconds()),
		AuthCheckInterval:  uint32(opc.AuthCheckInterval.Duration.Seconds()),
		Provider:           ngrok.EndpointOAuthProvider{},
	}
}

func (opc OAuthProviderCommon) ClientSecretKeyRef() *SecretKeyRef {
	return opc.ClientSecret
}

type EndpointOAuth struct {
	// configuration for using github as the identity provider
	Github *EndpointOAuthGitHub `json:"github,omitempty"`
	// configuration for using facebook as the identity provider
	Facebook *EndpointOAuthFacebook `json:"facebook,omitempty"`
	// configuration for using microsoft as the identity provider
	Microsoft *EndpointOAuthMicrosoft `json:"microsoft,omitempty"`
	// configuration for using google as the identity provider
	Google *EndpointOAuthGoogle `json:"google,omitempty"`
	// configuration for using linkedin as the identity provider
	Linkedin *EndpointOAuthLinkedIn `json:"linkedin,omitempty"`
	// configuration for using gitlab as the identity provider
	Gitlab *EndpointOAuthGitLab `json:"gitlab,omitempty"`
	// configuration for using twitch as the identity provider
	Twitch *EndpointOAuthTwitch `json:"twitch,omitempty"`
	// configuration for using amazon as the identity provider
	Amazon *EndpointOAuthAmazon `json:"amazon,omitempty"`
}

type EndpointOAuthGitHub struct {
	OAuthProviderCommon `json:",inline"`
	// a list of github teams identifiers. users will be allowed access to the endpoint
	// if they are a member of any of these teams. identifiers should be in the 'slug'
	// format qualified with the org name, e.g. org-name/team-name
	Teams []string `json:"teams,omitempty"`
	// a list of github org identifiers. users who are members of any of the listed
	// organizations will be allowed access. identifiers should be the organization's
	// 'slug'
	Organizations []string `json:"organizations,omitempty"`
}

func (github *EndpointOAuthGitHub) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if github == nil {
		return nil
	}

	mod := github.toNgrokEndpointOauth()
	mod.Provider.Github = &ngrok.EndpointOAuthGitHub{
		ClientID:       github.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         github.Scopes,
		EmailAddresses: github.EmailAddresses,
		EmailDomains:   github.EmailDomains,
		Teams:          github.Teams,
		Organizations:  github.Organizations,
	}
	return mod
}

type EndpointOAuthFacebook struct {
	OAuthProviderCommon `json:",inline"`
}

func (facebook *EndpointOAuthFacebook) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if facebook == nil {
		return nil
	}

	mod := facebook.toNgrokEndpointOauth()
	mod.Provider.Facebook = &ngrok.EndpointOAuthFacebook{
		ClientID:       facebook.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         facebook.Scopes,
		EmailAddresses: facebook.EmailAddresses,
		EmailDomains:   facebook.EmailDomains,
	}
	return mod
}

type EndpointOAuthMicrosoft struct {
	OAuthProviderCommon `json:",inline"`
}

func (microsoft *EndpointOAuthMicrosoft) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if microsoft == nil {
		return nil
	}

	mod := microsoft.toNgrokEndpointOauth()
	mod.Provider.Microsoft = &ngrok.EndpointOAuthMicrosoft{
		ClientID:       microsoft.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         microsoft.Scopes,
		EmailAddresses: microsoft.EmailAddresses,
		EmailDomains:   microsoft.EmailDomains,
	}
	return mod
}

type EndpointOAuthGoogle struct {
	OAuthProviderCommon `json:",inline"`
}

func (google *EndpointOAuthGoogle) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if google == nil {
		return nil
	}

	mod := google.toNgrokEndpointOauth()
	mod.Provider.Google = &ngrok.EndpointOAuthGoogle{
		ClientID:       google.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         google.Scopes,
		EmailAddresses: google.EmailAddresses,
		EmailDomains:   google.EmailDomains,
	}
	return mod
}

type EndpointOAuthLinkedIn struct {
	OAuthProviderCommon `json:",inline"`
}

func (linkedin *EndpointOAuthLinkedIn) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if linkedin == nil {
		return nil
	}

	mod := linkedin.toNgrokEndpointOauth()
	mod.Provider.Linkedin = &ngrok.EndpointOAuthLinkedIn{
		ClientID:       linkedin.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         linkedin.Scopes,
		EmailAddresses: linkedin.EmailAddresses,
		EmailDomains:   linkedin.EmailDomains,
	}
	return mod
}

type EndpointOAuthGitLab struct {
	OAuthProviderCommon `json:",inline"`
}

func (gitlab *EndpointOAuthGitLab) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if gitlab == nil {
		return nil
	}

	mod := gitlab.toNgrokEndpointOauth()
	mod.Provider.Gitlab = &ngrok.EndpointOAuthGitLab{
		ClientID:       gitlab.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         gitlab.Scopes,
		EmailAddresses: gitlab.EmailAddresses,
		EmailDomains:   gitlab.EmailDomains,
	}
	return mod
}

type EndpointOAuthTwitch struct {
	OAuthProviderCommon `json:",inline"`
}

func (twitch *EndpointOAuthTwitch) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if twitch == nil {
		return nil
	}

	mod := twitch.toNgrokEndpointOauth()
	mod.Provider.Twitch = &ngrok.EndpointOAuthTwitch{
		ClientID:       twitch.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         twitch.Scopes,
		EmailAddresses: twitch.EmailAddresses,
		EmailDomains:   twitch.EmailDomains,
	}
	return mod
}

type EndpointOAuthAmazon struct {
	OAuthProviderCommon `json:",inline"`
}

func (amazon *EndpointOAuthAmazon) ToNgrok(clientSecret *string) *ngrok.EndpointOAuth {
	if amazon == nil {
		return nil
	}

	mod := amazon.toNgrokEndpointOauth()
	mod.Provider.Amazon = &ngrok.EndpointOAuthAmazon{
		ClientID:       amazon.ClientID,
		ClientSecret:   clientSecret,
		Scopes:         amazon.Scopes,
		EmailAddresses: amazon.EmailAddresses,
		EmailDomains:   amazon.EmailDomains,
	}
	return mod
}
