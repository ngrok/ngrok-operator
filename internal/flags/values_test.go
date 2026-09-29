package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListValue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{name: "JSON", in: `["a","b"]`, want: []string{"a", "b"}},
		{name: "YAML", in: `[a, b]`, want: []string{"a", "b"}},
		{name: "commas inside items", in: `["a == 'x,y'", "true"]`, want: []string{"a == 'x,y'", "true"}},
		{name: "scalars become strings", in: `[1, true]`, want: []string{"1", "true"}},
		{name: "empty", in: `[]`, want: []string{}},
		{name: "not a list", in: `a`, wantErr: true},
		{name: "map", in: `{a: b}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			v := &yamlValue[[]string]{&got, "list"}
			err := v.Set(tc.in)
			if tc.wantErr {
				require.ErrorContains(t, err, "want a YAML or JSON list of strings")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, "list", v.Type())
		})
	}
}

func TestMapValue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		want    map[string]string
		wantErr bool
	}{
		{name: "JSON", in: `{"key1":"val1","key2":"val2"}`, want: map[string]string{"key1": "val1", "key2": "val2"}},
		{name: "YAML", in: `{env: dev, "example.com/team": k8s}`, want: map[string]string{"env": "dev", "example.com/team": "k8s"}},
		{name: "commas and equals in values", in: `{a: "x=1,y=2"}`, want: map[string]string{"a": "x=1,y=2"}},
		{name: "scalars become strings", in: `{a: 1, b: true}`, want: map[string]string{"a": "1", "b": "true"}},
		{name: "empty", in: `{}`, want: map[string]string{}},
		{name: "helm dictionary format", in: `key1=val1,key2=val2`, wantErr: true},
		{name: "list", in: `[a]`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]string
			v := &yamlValue[map[string]string]{&got, "map"}
			err := v.Set(tc.in)
			if tc.wantErr {
				require.ErrorContains(t, err, "want a YAML or JSON map of strings")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestYAMLValueString(t *testing.T) {
	list := []string{"a == 'x,y'"}
	assert.Equal(t, `["a == 'x,y'"]`, (&yamlValue[[]string]{&list, "list"}).String())
	m := map[string]string{"b": "2", "a": "1"}
	assert.Equal(t, `{"a":"1","b":"2"}`, (&yamlValue[map[string]string]{&m, "map"}).String())
}
