package util

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
