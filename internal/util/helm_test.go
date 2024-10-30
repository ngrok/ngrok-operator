package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseHelmDictionary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dict string
		want map[string]string
		err  bool
	}{
		{
			name: "empty",
			dict: "",
			want: map[string]string{},
			err:  false,
		},
		{
			name: "single",
			dict: "key=val",
			want: map[string]string{"key": "val"},
			err:  false,
		},
		{
			name: "multiple",
			dict: "key1=val1,key2=val2",
			want: map[string]string{"key1": "val1", "key2": "val2"},
			err:  false,
		},
		{
			name: "invalid",
			dict: "key1=val1,key2",
			want: nil,
			err:  true,
		},
		{
			name: "invalid2",
			dict: "key1-no-equal-sign",
			want: nil,
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)
			got, err := ParseHelmDictionary(tt.dict)
			if tt.err {
				assert.Error(err)
			} else {
				assert.NoError(err)
				assert.Equal(tt.want, got)
			}
		})
	}
}
