/*
MIT License

Copyright (c) 2026 ngrok, Inc.

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

// LEGACY-domain-kind: delete this file at cleanup.

package ngrok

import (
	"context"
	"fmt"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

var _ = Describe("Domain migration to ngrok.com/v1", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		ctx      context.Context
		key      client.ObjectKey
		hostname string
		legacy   *ingressv1alpha1.Domain
		legacyID string
	)

	reservations := func() []*ngrok.ReservedDomain {
		var out []*ngrok.ReservedDomain
		iter := domainClient.List(&ngrok.FilteredPaging{})
		for iter.Next(ctx) {
			if iter.Item().Domain == hostname {
				out = append(out, iter.Item())
			}
		}
		Expect(iter.Err()).ToNot(HaveOccurred())
		return out
	}

	newV1 := func() *ngrokv1.Domain {
		return &ngrokv1.Domain{
			Name:      key.Name,
			Namespace: key.Namespace,
			Spec: ngrokv1.DomainSpec{
				Domain:      hostname,
				Description: "re-stamped",
			},
		}
	}

	eventuallyGone := func(obj client.Object) {
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, key, obj)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	}

	BeforeEach(func() {
		ctx = GinkgoT().Context()
		hostname = fmt.Sprintf("migrate-%s.ngrok.app", rand.String(8))
		key = client.ObjectKey{Namespace: domainTestNamespace, Name: ngrokv1.HyphenatedDomainNameFromURL(hostname)}

		legacy = &ingressv1alpha1.Domain{
			Name:      key.Name,
			Namespace: key.Namespace,
			Spec: ingressv1alpha1.DomainSpec{
				Domain:      hostname,
				Description: "legacy",
			},
		}
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &ingressv1alpha1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
			g.Expect(got.Status.ID).ToNot(BeEmpty())
			legacyID = got.Status.ID
		}, timeout, interval).Should(Succeed())

		DeferCleanup(func() {
			for _, obj := range []client.Object{&ngrokv1.Domain{}, &ingressv1alpha1.Domain{}} {
				if err := k8sClient.Get(ctx, key, obj); err == nil {
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed())
				}
				eventuallyGone(obj)
			}
		})
	})

	It("adopts the legacy reservation and supersedes the legacy Domain", func() {
		Expect(k8sClient.Create(ctx, newV1())).To(Succeed())

		Eventually(func(g Gomega) {
			v1 := &ngrokv1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, v1)).To(Succeed())
			g.Expect(v1.Status.ID).To(Equal(legacyID))

			got := &ingressv1alpha1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
			ready := meta.FindStatusCondition(got.Status.Conditions, domainpkg.ConditionReady)
			g.Expect(ready).ToNot(BeNil())
			g.Expect(ready.Reason).To(Equal(domainpkg.ReasonSupersededByV1))
			g.Expect(got.Finalizers).ToNot(BeEmpty(), "the legacy Domain keeps its finalizer until it is deleted")
		}, timeout, interval).Should(Succeed())

		Expect(reservations()).To(HaveLen(1))
		Eventually(func(g Gomega) {
			g.Expect(reservations()[0].Description).To(Equal("re-stamped"), "v1 pushes its spec")
		}, timeout, interval).Should(Succeed())
	})

	It("keeps the reservation when the legacy Domain is deleted after the re-stamp", func() {
		Expect(k8sClient.Create(ctx, newV1())).To(Succeed())
		Eventually(func(g Gomega) {
			v1 := &ngrokv1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, v1)).To(Succeed())
			g.Expect(v1.Status.ID).To(Equal(legacyID))
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
		eventuallyGone(&ingressv1alpha1.Domain{})

		Consistently(func(g Gomega) {
			g.Expect(reservations()).To(HaveLen(1))
		}, time.Second, interval).Should(Succeed())
	})

	It("hands the reservation back when the v1 Domain is deleted", func() {
		v1 := newV1()
		Expect(k8sClient.Create(ctx, v1)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &ngrokv1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
			g.Expect(got.Status.ID).To(Equal(legacyID))
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Delete(ctx, v1)).To(Succeed())
		eventuallyGone(&ngrokv1.Domain{})

		Expect(reservations()).To(HaveLen(1))
		// The pass that released the v1 finalizer also made legacy the writer.
		Eventually(func(g Gomega) {
			got := &ingressv1alpha1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
			ready := meta.FindStatusCondition(got.Status.Conditions, domainpkg.ConditionReady)
			g.Expect(ready).ToNot(BeNil())
			g.Expect(ready.Reason).ToNot(Equal(domainpkg.ReasonSupersededByV1))
			g.Expect(reservations()[0].Description).To(Equal("legacy"), "the legacy Domain writes again")
		}, timeout, interval).Should(Succeed())
	})

	It("keeps the reservation when v1 is applied and legacy deleted back to back", func() {
		Expect(k8sClient.Create(ctx, newV1())).To(Succeed())
		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())

		eventuallyGone(&ingressv1alpha1.Domain{})
		Eventually(func(g Gomega) {
			v1 := &ngrokv1.Domain{}
			g.Expect(k8sClient.Get(ctx, key, v1)).To(Succeed())
			g.Expect(v1.Status.ID).To(Equal(legacyID))
		}, timeout, interval).Should(Succeed())
		Expect(reservations()).To(HaveLen(1))
	})
})
