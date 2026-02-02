/*
MIT License

Copyright (c) 2025 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIsUpsert(t *testing.T) {
	now := metav1.NewTime(time.Now())
	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{
			name: "no deletion timestamp",
			obj:  &netv1.Ingress{},
			want: true,
		},
		{
			name: "with deletion timestamp",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsUpsert(tt.obj))
		})
	}
}

func TestIsDelete(t *testing.T) {
	now := metav1.NewTime(time.Now())
	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{
			name: "no deletion timestamp",
			obj:  &netv1.Ingress{},
			want: false,
		},
		{
			name: "with deletion timestamp",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDelete(tt.obj))
		})
	}
}

func TestAddAnnotations(t *testing.T) {
	tests := []struct {
		name            string
		obj             client.Object
		annotations     map[string]string
		wantAnnotations map[string]string
	}{
		{
			name:            "nil object",
			obj:             nil,
			annotations:     map[string]string{"key": "value"},
			wantAnnotations: nil,
		},
		{
			name:            "nil annotations",
			obj:             &netv1.Ingress{},
			annotations:     nil,
			wantAnnotations: nil,
		},
		{
			name:            "add to empty",
			obj:             &netv1.Ingress{},
			annotations:     map[string]string{"key": "value"},
			wantAnnotations: map[string]string{"key": "value"},
		},
		{
			name: "add to existing",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"existing": "annotation"},
				},
			},
			annotations:     map[string]string{"key": "value"},
			wantAnnotations: map[string]string{"existing": "annotation", "key": "value"},
		},
		{
			name: "overwrite existing",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"key": "old"},
				},
			},
			annotations:     map[string]string{"key": "new"},
			wantAnnotations: map[string]string{"key": "new"},
		},
		{
			name:            "add multiple",
			obj:             &netv1.Ingress{},
			annotations:     map[string]string{"key1": "value1", "key2": "value2"},
			wantAnnotations: map[string]string{"key1": "value1", "key2": "value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AddAnnotations(tt.obj, tt.annotations)
			if tt.obj != nil {
				assert.Equal(t, tt.wantAnnotations, tt.obj.GetAnnotations())
			}
		})
	}
}
