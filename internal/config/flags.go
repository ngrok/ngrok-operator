package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// The components that load Config, as named in its components tags.
const (
	APIManager        = "apiManager"
	Agent             = "agent"
	BindingsForwarder = "bindingsForwarder"
)

// FlagName returns the command-line flag for a dotted config path:
// ngrok.rootCAs is --ngrok-root-cas. It is the same name as EnvName's, in
// kebab case.
func FlagName(path string) string {
	name := wordBoundary.ReplaceAllString(strings.ReplaceAll(path, ".", "-"), "${1}-${2}")
	return strings.ToLower(name)
}

// Flags holds the command-line values for one component's settings until
// they are applied over the loaded config.
type Flags struct {
	values map[string]*flagValue
}

// RegisterFlags registers a flag for every setting whose components tag
// names component. Each flag shows its Default() value and help tag in
// --help, and takes the same encoding as the setting's environment variable.
func RegisterFlags(fs *pflag.FlagSet, component string) *Flags {
	flags := &Flags{values: map[string]*flagValue{}}

	walk(reflect.ValueOf(Default()).Elem(), "", func(path string, sf reflect.StructField, field reflect.Value) {
		if field.Kind() == reflect.Struct {
			return
		}
		if !slices.Contains(strings.Split(sf.Tag.Get("components"), ","), component) {
			return
		}

		value := &flagValue{kind: field.Kind(), display: display(field)}
		flag := fs.VarPF(value, FlagName(path), "", sf.Tag.Get("help"))
		if field.Kind() == reflect.Bool {
			flag.NoOptDefVal = "true"
		}
		flags.values[path] = value
	})

	return flags
}

// Apply sets every flag passed on the command line over cfg, so a flag wins
// over the environment.
func (f *Flags) Apply(cfg *Config) error {
	var errs []error
	walk(reflect.ValueOf(cfg).Elem(), "", func(path string, _ reflect.StructField, field reflect.Value) {
		value, ok := f.values[path]
		if !ok || !value.changed {
			return
		}
		if err := set(field, value.raw); err != nil {
			errs = append(errs, fmt.Errorf("--%s: %w", FlagName(path), err))
		}
	})
	return errors.Join(errs...)
}

// flagValue records the raw string passed for a setting. Parsing is left to
// Apply, which uses the same rules as the environment.
type flagValue struct {
	kind    reflect.Kind
	display string
	raw     string
	changed bool
}

func (v *flagValue) String() string {
	if v.changed {
		return v.raw
	}
	return v.display
}

func (v *flagValue) Set(raw string) error {
	v.raw, v.changed = raw, true
	return nil
}

func (v *flagValue) Type() string {
	switch v.kind {
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		return "json-list"
	case reflect.Map:
		return "json-map"
	}
	return "string"
}

// display renders a default for --help: strings and booleans as-is, lists and
// maps as JSON. Empty values render as "", which pflag omits.
func display(field reflect.Value) string {
	switch field.Kind() {
	case reflect.String:
		return field.String()
	case reflect.Bool:
		return strconv.FormatBool(field.Bool())
	}
	if field.Len() == 0 {
		return ""
	}
	raw, _ := json.Marshal(field.Interface())
	return string(raw)
}
