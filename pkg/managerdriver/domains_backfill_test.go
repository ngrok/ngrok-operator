package managerdriver

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
)

// LEGACY-PREFIX-MIGRATION: an existing Domain stamped with only the legacy
// controller labels by a previous operator version must get the new label pair
// backfilled by applyDomains. CreateOrPatch's Get overwrites the object
// initializer, so the backfill has to happen inside the mutate function.
func TestApplyDomains_BackfillsLegacyOnlyDomain(t *testing.T) {
	const (
		controllerNamespace = "ngrok-system"
		controllerName      = "ngrok-operator"
		domainName          = "example-com"
	)

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))

	existing := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      domainName,
			Namespace: "default",
			Labels: map[string]string{
				labels.LegacyControllerName:      controllerName,
				labels.LegacyControllerNamespace: controllerNamespace,
			},
		},
		Spec: ingressv1alpha1.DomainSpec{Domain: "example.com"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	d := NewDriver(
		logr.Discard(),
		scheme,
		controllerName,
		types.NamespacedName{Namespace: controllerNamespace, Name: controllerName},
	)

	desired := map[string]ingressv1alpha1.Domain{
		"example.com": {
			ObjectMeta: metav1.ObjectMeta{Name: domainName, Namespace: "default"},
			Spec:       ingressv1alpha1.DomainSpec{Domain: "example.com"},
		},
	}

	require.NoError(t, d.applyDomains(context.Background(), c, desired))

	got := &ingressv1alpha1.Domain{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(existing), got))

	gotLabels := got.GetLabels()
	assert.Equal(t, controllerName, gotLabels[labels.ControllerName], "new controller-name label backfilled")
	assert.Equal(t, controllerNamespace, gotLabels[labels.ControllerNamespace], "new controller-namespace label backfilled")
	assert.Equal(t, controllerName, gotLabels[labels.LegacyControllerName], "legacy controller-name label retained")
	assert.Equal(t, controllerNamespace, gotLabels[labels.LegacyControllerNamespace], "legacy controller-namespace label retained")
}
