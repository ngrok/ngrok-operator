package ngrokapidriver

type Edge struct {
	Id       string
	Hostport string // TODO: Support an array of hostports when we support multiple rules
	Routes   []Route
}

type Route struct {
	Id string
	// route to match on, i.e. /example/foo
	Match string
	// "exact_path" or "path_prefix"
	MatchType string
	Labels    map[string]string

	// TODO: This is a shortcut and should be replaced
	Compression bool
	GoogleOAuth OAuthGoogle
}

type OAuthGoogle struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	EmailDomains []string
}
