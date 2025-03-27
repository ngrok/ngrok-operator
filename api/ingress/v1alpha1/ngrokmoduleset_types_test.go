package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNgrokModuleSetIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		ms   *NgrokModuleSet
		want bool
	}{
		{
			name: "nil",
			ms:   nil,
			want: true,
		},
		{
			name: "empty",
			ms:   &NgrokModuleSet{},
			want: true,
		},
		{
			name: "non-empty",
			ms: &NgrokModuleSet{
				Modules: NgrokModuleSetModules{
					Headers: &EndpointHeaders{
						Request: &EndpointRequestHeaders{
							Add: map[string]string{
								"key": "value",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ms.IsEmpty(); got != tt.want {
				t.Errorf("NgrokModuleSet.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNgrokModuleSetMerge(t *testing.T) {
	addHeaders := &EndpointHeaders{
		Request: &EndpointRequestHeaders{
			Add: map[string]string{
				"key": "value",
			},
		},
	}
	compression := &EndpointCompression{
		Enabled: true,
	}

	tests := []struct {
		name  string
		m     *NgrokModuleSet
		other *NgrokModuleSet
		want  *NgrokModuleSet
	}{
		{
			name:  "both_nil",
			m:     nil,
			other: nil,
			want:  nil,
		},
		{
			name: "b_nil",
			m: &NgrokModuleSet{
				Modules: NgrokModuleSetModules{
					Headers: addHeaders,
				},
			},
			other: nil,
			want: &NgrokModuleSet{
				Modules: NgrokModuleSetModules{
					Headers: addHeaders,
				},
			},
		},
		{
			name: "neither_nil",
			m: &NgrokModuleSet{
				Modules: NgrokModuleSetModules{
					Compression: compression,
				},
			},
			other: &NgrokModuleSet{
				Modules: NgrokModuleSetModules{
					Headers: addHeaders,
				},
			},
			want: &NgrokModuleSet{
				Modules: NgrokModuleSetModules{
					Headers:     addHeaders,
					Compression: compression,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Merge(tt.other)

			assert.Equal(t, tt.m, tt.want)
		})
	}
}
