package ngrokapidriver

type Edge struct {
	Id       string
	Hostport string // TODO: Support an array of hostports when we support multiple rules
	Labels   map[string]string
	Routes   []Route
}

type Route struct {
	Id string
	// route to match on, i.e. /example/foo
	Match string
	// "exact_path" or "path_prefix"
	MatchType string
	Modules   []any
}

type Module interface {
}
