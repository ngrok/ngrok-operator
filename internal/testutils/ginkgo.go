package testutils

import (
	"context"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExpectCreateNamespace returns a function that creates a namespace with the given name.
// The function uses Gomega's Expect internally, so it should only be used in Ginkgo tests.
//
// Example usage:
//
//	createNs := testutils.ExpectCreateNamespace(k8sClient)
//	createNs("test-namespace")
//	defer testutils.ExpectDeleteNamespace(k8sClient)("test-namespace")
func ExpectCreateNamespace(k8sClient client.Client) func(string) {
	return func(name string) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())
	}
}

// ExpectDeleteNamespace returns a function that deletes a namespace with the given name.
// The function uses Gomega's Expect internally and expects the delete to succeed or return NotFound.
// This is useful for cleaning up namespaces in defer statements.
//
// Example usage:
//
//	deleteNs := testutils.ExpectDeleteNamespace(k8sClient)
//	defer deleteNs("test-namespace")
func ExpectDeleteNamespace(k8sClient client.Client) func(string) {
	return func(name string) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		err := k8sClient.Delete(context.Background(), ns)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}
