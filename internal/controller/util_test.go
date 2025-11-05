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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestHasFinalizer(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{
			name: "no finalizers",
			obj:  &netv1.Ingress{},
			want: false,
		},
		{
			name: "different finalizer",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer"},
				},
			},
			want: false,
		},
		{
			name: "has finalizer",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName},
				},
			},
			want: true,
		},
		{
			name: "has finalizer among others",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer", FinalizerName, "another.finalizer"},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasFinalizer(tt.obj))
		})
	}
}

func TestAddFinalizer(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		wantAdded   bool
		wantPresent bool
	}{
		{
			name:        "add to empty",
			obj:         &netv1.Ingress{},
			wantAdded:   true,
			wantPresent: true,
		},
		{
			name: "add to existing",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer"},
				},
			},
			wantAdded:   true,
			wantPresent: true,
		},
		{
			name: "already present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName},
				},
			},
			wantAdded:   false,
			wantPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added := AddFinalizer(tt.obj)
			assert.Equal(t, tt.wantAdded, added)
			assert.Equal(t, tt.wantPresent, HasFinalizer(tt.obj))
		})
	}
}

func TestRemoveFinalizer(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		wantRemoved bool
		wantPresent bool
	}{
		{
			name:        "remove from empty",
			obj:         &netv1.Ingress{},
			wantRemoved: false,
			wantPresent: false,
		},
		{
			name: "remove when present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName},
				},
			},
			wantRemoved: true,
			wantPresent: false,
		},
		{
			name: "remove when not present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer"},
				},
			},
			wantRemoved: false,
			wantPresent: false,
		},
		{
			name: "remove from multiple",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer", FinalizerName, "another.finalizer"},
				},
			},
			wantRemoved: true,
			wantPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removed := RemoveFinalizer(tt.obj)
			assert.Equal(t, tt.wantRemoved, removed)
			assert.Equal(t, tt.wantPresent, HasFinalizer(tt.obj))
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

func TestRegisterAndSyncFinalizer(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		obj           client.Object
		wantErr       bool
		wantFinalizer bool
		wantUpdated   bool
	}{
		{
			name: "add finalizer to object without one",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
			wantErr:       false,
			wantFinalizer: true,
			wantUpdated:   true,
		},
		{
			name: "object already has finalizer",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ingress",
					Namespace:  "default",
					Finalizers: []string{FinalizerName},
				},
			},
			wantErr:       false,
			wantFinalizer: true,
			wantUpdated:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := runtime.NewScheme()
			err := scheme.AddToScheme(s)
			require.NoError(t, err)
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.obj).Build()

			err = RegisterAndSyncFinalizer(ctx, c, tt.obj)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantFinalizer, HasFinalizer(tt.obj))

			var updated netv1.Ingress
			err = c.Get(ctx, client.ObjectKeyFromObject(tt.obj), &updated)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFinalizer, HasFinalizer(&updated))
		})
	}
}

func TestRemoveAndSyncFinalizer(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		obj           client.Object
		wantErr       bool
		wantFinalizer bool
	}{
		{
			name: "remove finalizer from object with one",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ingress",
					Namespace:  "default",
					Finalizers: []string{FinalizerName},
				},
			},
			wantErr:       false,
			wantFinalizer: false,
		},
		{
			name: "remove finalizer from object without one",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
			wantErr:       false,
			wantFinalizer: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := runtime.NewScheme()
			err := scheme.AddToScheme(s)
			require.NoError(t, err)
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.obj).Build()

			err = RemoveAndSyncFinalizer(ctx, c, tt.obj)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantFinalizer, HasFinalizer(tt.obj))

			var updated netv1.Ingress
			err = c.Get(ctx, client.ObjectKeyFromObject(tt.obj), &updated)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFinalizer, HasFinalizer(&updated))
		})
	}
}
