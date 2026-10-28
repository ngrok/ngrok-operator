package flags

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

// yamlValue is a flag holding a list or map, written as YAML or JSON, such as
// '["a == \'x,y\'", "true"]' or '{env: dev}'. Unlike pflag's StringSlice it
// does not split on commas, which CEL expressions contain.
type yamlValue[T any] struct {
	p   *T
	typ string
}

func (v *yamlValue[T]) Set(s string) error {
	var x T
	if err := yaml.Unmarshal([]byte(s), &x); err != nil {
		return fmt.Errorf("want a YAML or JSON %s of strings: %w", v.typ, err)
	}
	*v.p = x
	return nil
}

func (v *yamlValue[T]) String() string {
	b, _ := json.Marshal(*v.p)
	return string(b)
}

func (v *yamlValue[T]) Type() string { return v.typ }
