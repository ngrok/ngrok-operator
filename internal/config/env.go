package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// EnvPrefix starts the name of every operator configuration variable.
const EnvPrefix = "NGROK_OPERATOR_"

var wordBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// EnvName returns the environment variable for a dotted config path:
// ngrok.rootCAs is read from NGROK_OPERATOR_NGROK_ROOT_CAS.
//
// The chart derives names with the same expression (ngrok-operator.envName),
// so the two cannot disagree.
func EnvName(path string) string {
	name := wordBoundary.ReplaceAllString(strings.ReplaceAll(path, ".", "_"), "${1}_${2}")
	return EnvPrefix + strings.ToUpper(name)
}

// EnvNames lists the variable for every individual setting in Config.
func EnvNames() []string {
	var names []string
	walk(reflect.ValueOf(Default()).Elem(), "", func(path string, field reflect.Value) {
		if field.Kind() != reflect.Struct {
			names = append(names, EnvName(path))
		}
	})
	return names
}

// Load returns Default() with the NGROK_OPERATOR_* variables in the
// environment applied over it.
func Load() (*Config, error) {
	return loadFrom(os.LookupEnv)
}

// loadFrom reads a variable for every section and every setting in Config.
//
// A section variable such as NGROK_OPERATOR_FEATURES holds that section as
// JSON; keys it leaves out keep their defaults, and unknown keys are an error.
// The chart renders one per top-level section. A setting variable such as
// NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED sets one value and wins over its
// section, which is the convenient form for local development.
//
// Setting values that are strings are taken as-is; booleans, lists and maps
// are parsed as YAML, which also accepts JSON. An empty variable counts as
// unset.
func loadFrom(lookup func(string) (string, bool)) (*Config, error) {
	cfg := Default()

	var errs []error
	walk(reflect.ValueOf(cfg).Elem(), "", func(path string, field reflect.Value) {
		name := EnvName(path)
		value, ok := lookup(name)
		if !ok || value == "" {
			return
		}
		if err := set(field, value); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	})

	return cfg, errors.Join(errs...)
}

// walk calls fn for every field of v, parents before their children, named by
// its dotted JSON path.
func walk(v reflect.Value, prefix string, fn func(path string, field reflect.Value)) {
	t := v.Type()
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		field := v.Field(i)
		fn(prefix+name, field)
		if field.Kind() == reflect.Struct {
			walk(field, prefix+name+".", fn)
		}
	}
}

func set(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil
	case reflect.Struct:
		// Decoded onto the field in place, so keys the value leaves out keep
		// their current values.
		return yaml.UnmarshalStrict([]byte(value), field.Addr().Interface())
	}

	parsed := reflect.New(field.Type())
	if err := yaml.Unmarshal([]byte(value), parsed.Interface()); err != nil {
		return err
	}
	field.Set(parsed.Elem())
	return nil
}
