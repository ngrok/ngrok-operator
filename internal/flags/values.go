package flags

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

// listValue is a list flag that takes YAML or JSON, such as
// '["a == \'x,y\'", "true"]'. Unlike StringSlice it does not split on commas,
// which CEL expressions contain.
type listValue struct{ p *[]string }

func (v *listValue) Set(s string) error {
	var list []string
	if err := yaml.Unmarshal([]byte(s), &list); err != nil {
		return fmt.Errorf("want a YAML or JSON list of strings: %w", err)
	}
	*v.p = list
	return nil
}

func (v *listValue) String() string {
	b, _ := json.Marshal(*v.p)
	return string(b)
}

func (v *listValue) Type() string { return "list" }

// mapValue is a string map flag that takes YAML or JSON, such as
// '{env: dev, "example.com/team": k8s}'.
type mapValue struct{ p *map[string]string }

func (v *mapValue) Set(s string) error {
	m := map[string]string{}
	if err := yaml.Unmarshal([]byte(s), &m); err != nil {
		return fmt.Errorf("want a YAML or JSON map of strings: %w", err)
	}
	*v.p = m
	return nil
}

func (v *mapValue) String() string {
	b, _ := json.Marshal(*v.p)
	return string(b)
}

func (v *mapValue) Type() string { return "map" }
