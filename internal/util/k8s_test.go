package util

import (
	"context"
	"slices"
	"testing"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestToClientObjects(t *testing.T) {
	s := []ingressv1alpha1.Domain{}
	assert.Empty(t, ToClientObjects(s))

	s = []ingressv1alpha1.Domain{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: ingressv1alpha1.DomainSpec{
				Domain: "test.ngrok.io",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test2",
				Namespace: "other",
			},
			Spec: ingressv1alpha1.DomainSpec{
				Domain: "test.ngrok.io",
			},
		},
	}

	objs := ToClientObjects(s)
	assert.Len(t, objs, 2)

	// Test some client.Object methods on our objects
	assert.Equal(t, "test", objs[0].GetName())
	assert.Equal(t, "default", objs[0].GetNamespace())
	assert.Equal(t, "test2", objs[1].GetName())
	assert.Equal(t, "other", objs[1].GetNamespace())

	assert.Equal(t, &s[0], objs[0])
	assert.Equal(t, &s[1], objs[1])
}

func Test_ObjNameFuncs(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	type fnTest struct {
		fn   func(client.Object) string
		want string
	}

	tests := []struct {
		name  string
		obj   client.Object
		wants []fnTest
	}{
		{
			name: "nil",
			obj:  nil,
			wants: []fnTest{
				{fn: ObjToName, want: ""},
				{fn: ObjToKind, want: ""},
				{fn: ObjToGVK, want: ""},
				{fn: ObjToHumanName, want: ""},
				{fn: ObjToHumanGvkName, want: ""},
			},
		},
		{
			name: "empty",
			obj:  &v1.ConfigMap{},
			wants: []fnTest{
				{fn: ObjToName, want: ""},
				{fn: ObjToKind, want: ""},
				{fn: ObjToGVK, want: ""},
				{fn: ObjToHumanName, want: ""},
				{fn: ObjToHumanGvkName, want: ""},
			},
		},
		{
			name: "configmap",
			obj: &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-cm",
				},
			},
			wants: []fnTest{
				{fn: ObjToName, want: "my-cm"},
				{fn: ObjToKind, want: "ConfigMap"},
				{fn: ObjToGVK, want: "/v1, Kind=ConfigMap"},
				{fn: ObjToHumanName, want: "ConfigMap/my-cm"},
				{fn: ObjToHumanGvkName, want: "/v1, Kind=ConfigMap Name=my-cm"},
			},
		},
		{
			name: "job",
			obj: &batchv1.Job{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Job",
					APIVersion: "batch/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-job",
				},
			},
			wants: []fnTest{
				{fn: ObjToName, want: "my-job"},
				{fn: ObjToKind, want: "Job"},
				{fn: ObjToGVK, want: "batch/v1, Kind=Job"},
				{fn: ObjToHumanName, want: "Job/my-job"},
				{fn: ObjToHumanGvkName, want: "batch/v1, Kind=Job Name=my-job"},
			},
		},
		{
			name: "custom",
			obj: &v1.ConfigMap{ // use a configmap, but change the type meta
				TypeMeta: metav1.TypeMeta{
					Kind:       "CustomObject",
					APIVersion: "k8s.ngrok.com/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-obj",
				},
			},
			wants: []fnTest{
				{fn: ObjToName, want: "my-obj"},
				{fn: ObjToKind, want: "CustomObject"},
				{fn: ObjToGVK, want: "k8s.ngrok.com/v1beta1, Kind=CustomObject"},
				{fn: ObjToHumanName, want: "CustomObject/my-obj"},
				{fn: ObjToHumanGvkName, want: "k8s.ngrok.com/v1beta1, Kind=CustomObject Name=my-obj"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, want := range test.wants {
				got := want.fn(test.obj)
				assert.Equal(want.want, got)
			}
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
			name: "has new finalizer",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName},
				},
			},
			want: true,
		},
		{
			name: "has legacy finalizer",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{LegacyFinalizerName},
				},
			},
			want: true,
		},
		{
			name: "has both finalizers",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName, LegacyFinalizerName},
				},
			},
			want: true,
		},
		{
			name: "has new finalizer among others",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer", FinalizerName, "another.finalizer"},
				},
			},
			want: true,
		},
		{
			name: "has legacy finalizer among others",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer", LegacyFinalizerName, "another.finalizer"},
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

// TestAddFinalizer asserts the R1 behavior: AddFinalizer writes the legacy
// key only. In R2 this test must be updated to assert FinalizerName is the
// written key and LegacyFinalizerName has been stripped.
func TestAddFinalizer(t *testing.T) {
	tests := []struct {
		name              string
		obj               client.Object
		wantAdded         bool
		wantLegacyPresent bool
		wantNewPresent    bool
	}{
		{
			name:              "add to empty",
			obj:               &netv1.Ingress{},
			wantAdded:         true,
			wantLegacyPresent: true,
			wantNewPresent:    false,
		},
		{
			name: "add to existing",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer"},
				},
			},
			wantAdded:         true,
			wantLegacyPresent: true,
			wantNewPresent:    false,
		},
		{
			name: "legacy already present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{LegacyFinalizerName},
				},
			},
			wantAdded:         false,
			wantLegacyPresent: true,
			wantNewPresent:    false,
		},
		{
			name: "new finalizer already present from prior R2 reconcile",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName},
				},
			},
			// R1 still wants the legacy key on the object, so the add
			// returns true even though HasFinalizer was already true.
			wantAdded:         true,
			wantLegacyPresent: true,
			wantNewPresent:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added := AddFinalizer(tt.obj)
			assert.Equal(t, tt.wantAdded, added)
			assert.Equal(t, tt.wantLegacyPresent, hasRawFinalizer(tt.obj, LegacyFinalizerName))
			assert.Equal(t, tt.wantNewPresent, hasRawFinalizer(tt.obj, FinalizerName))
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
			name: "remove when new present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName},
				},
			},
			wantRemoved: true,
			wantPresent: false,
		},
		{
			name: "remove when legacy present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{LegacyFinalizerName},
				},
			},
			wantRemoved: true,
			wantPresent: false,
		},
		{
			name: "remove when both present",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{FinalizerName, LegacyFinalizerName},
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
			name: "remove from multiple with new",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer", FinalizerName, "another.finalizer"},
				},
			},
			wantRemoved: true,
			wantPresent: false,
		},
		{
			name: "remove from multiple with legacy",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other.finalizer", LegacyFinalizerName, "another.finalizer"},
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
			name: "object already has legacy finalizer",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ingress",
					Namespace:  "default",
					Finalizers: []string{LegacyFinalizerName},
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
			name: "remove new finalizer from object with one",
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
			name: "remove legacy finalizer from object with one",
			obj: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ingress",
					Namespace:  "default",
					Finalizers: []string{LegacyFinalizerName},
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

// hasRawFinalizer reports whether the object carries the given finalizer
// literal, bypassing the dual-key HasFinalizer helper. Used by tests that
// need to assert the specific key written during the R1 migration window.
func hasRawFinalizer(o client.Object, name string) bool {
	return slices.Contains(o.GetFinalizers(), name)
}
