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

// LEGACY-domain-kind: delete this file at cleanup. The single-kind cases it
// leans on (v1 only, reclaim policies) live in domain_controller_test.go.

package ngrok

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

var _ = Describe("Domain migration to ngrok.com/v1", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
		// settle is how long a Consistently check waits for a pass that must
		// not change anything.
		settle = time.Second
	)

	var (
		ctx      context.Context
		key      client.ObjectKey
		hostname string
	)

	reservationsFor := func(host string) []*ngrok.ReservedDomain {
		var out []*ngrok.ReservedDomain
		iter := domainClient.List(&ngrok.FilteredPaging{})
		for iter.Next(ctx) {
			if iter.Item().Domain == host {
				out = append(out, iter.Item())
			}
		}
		Expect(iter.Err()).ToNot(HaveOccurred())
		return out
	}
	reservations := func() []*ngrok.ReservedDomain { return reservationsFor(hostname) }

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

	newLegacy := func() *ingressv1alpha1.Domain {
		return &ingressv1alpha1.Domain{
			Name:      key.Name,
			Namespace: key.Namespace,
			Spec: ingressv1alpha1.DomainSpec{
				Domain:      hostname,
				Description: "legacy",
			},
		}
	}

	getV1 := func(g Gomega) *ngrokv1.Domain {
		got := &ngrokv1.Domain{}
		g.Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
		return got
	}

	getLegacy := func(g Gomega) *ingressv1alpha1.Domain {
		got := &ingressv1alpha1.Domain{}
		g.Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
		return got
	}

	readyOf := func(g Gomega, conditions []metav1.Condition) *metav1.Condition {
		ready := meta.FindStatusCondition(conditions, domainpkg.ConditionReady)
		g.Expect(ready).ToNot(BeNil(), "Ready condition")
		return ready
	}

	eventuallyGone := func(obj client.Object) {
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, key, obj)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	}

	// eventuallyEvent waits for an event about obj (matched by UID) with the
	// given type and reason whose note contains note.
	eventuallyEvent := func(obj client.Object, eventType, reason, note string) {
		Eventually(func(g Gomega) {
			list := &eventsv1.EventList{}
			g.Expect(k8sClient.List(ctx, list, client.InNamespace(key.Namespace))).To(Succeed())
			g.Expect(list.Items).To(ContainElement(SatisfyAll(
				HaveField("Regarding.UID", obj.GetUID()),
				HaveField("Type", eventType),
				HaveField("Reason", reason),
				HaveField("Note", ContainSubstring(note)),
			)))
		}, timeout, interval).Should(Succeed())
	}

	BeforeEach(func() {
		ctx = GinkgoT().Context()
		hostname = fmt.Sprintf("migrate-%s.ngrok.app", rand.String(8))
		key = client.ObjectKey{Namespace: domainTestNamespace, Name: ngrokv1.HyphenatedDomainNameFromURL(hostname)}

		DeferCleanup(func() {
			for _, obj := range []client.Object{&ngrokv1.Domain{}, &ingressv1alpha1.Domain{}} {
				if err := k8sClient.Get(ctx, key, obj); err == nil {
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed())
				}
				eventuallyGone(obj)
			}
		})
	})

	When("a legacy Domain holds a reservation", func() {
		var (
			legacy   *ingressv1alpha1.Domain
			legacyID string
		)

		BeforeEach(func() {
			legacy = newLegacy()
			Expect(k8sClient.Create(ctx, legacy)).To(Succeed())

			Eventually(func(g Gomega) {
				got := getLegacy(g)
				g.Expect(got.Status.ID).ToNot(BeEmpty())
				g.Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
				g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, domainpkg.ConditionReady)).To(BeTrue())
				legacyID = got.Status.ID
			}, timeout, interval).Should(Succeed())
		})

		It("keeps the legacy Domain as the writer while it has no v1 twin", func() {
			Eventually(func(g Gomega) {
				got := getLegacy(g)
				g.Expect(readyOf(g, got.Status.Conditions).Reason).To(Equal(domainpkg.ReasonDomainActive))
				g.Expect(got.Finalizers).To(ContainElement(util.FinalizerName))
			}, timeout, interval).Should(Succeed())
			Expect(reservations()).To(HaveLen(1))
			Expect(reservations()[0].Description).To(Equal("legacy"))
			eventuallyEvent(legacy, "Warning", trafficpolicypkg.EventDeprecatedAPIGroup, "re-create it as an ngrok.com/v1 Domain")
		})

		It("adopts the legacy reservation and supersedes the legacy Domain", func() {
			// A sentinel: a superseded legacy Domain's other status fields stay
			// as they were when the v1 Domain took over.
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				got := &ingressv1alpha1.Domain{}
				if err := k8sClient.Get(ctx, key, got); err != nil {
					return err
				}
				got.Status.CNAMETarget = new("frozen.example.com")
				return k8sClient.Status().Update(ctx, got)
			})).To(Succeed())

			v1 := newV1()
			Expect(k8sClient.Create(ctx, v1)).To(Succeed())

			Eventually(func(g Gomega) {
				gotV1 := getV1(g)
				g.Expect(gotV1.Status.ID).To(Equal(legacyID))
				g.Expect(readyOf(g, gotV1.Status.Conditions).Reason).To(Equal(domainpkg.ReasonDomainActive))

				got := getLegacy(g)
				ready := readyOf(g, got.Status.Conditions)
				g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(ready.Reason).To(Equal(domainpkg.ReasonSupersededByV1))
				g.Expect(got.Status.CNAMETarget).To(HaveValue(Equal("frozen.example.com")), "other status fields stay frozen")
				g.Expect(got.Finalizers).To(ContainElement(util.FinalizerName), "the legacy Domain keeps its finalizer until it is deleted")
			}, timeout, interval).Should(Succeed())

			Expect(reservations()).To(HaveLen(1), "adoption never creates a second reservation")
			Expect(reservations()[0].Description).To(Equal("re-stamped"), "v1 pushes its spec")
			eventuallyEvent(legacy, "Warning", trafficpolicypkg.EventDeprecatedAPIGroup, "superseded by domains.ngrok.com")

			By("running another pass, which leaves the superseded Domain alone")
			superseded := &ingressv1alpha1.Domain{}
			Expect(k8sClient.Get(ctx, key, superseded)).To(Succeed())
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				got := &ngrokv1.Domain{}
				if err := k8sClient.Get(ctx, key, got); err != nil {
					return err
				}
				got.Annotations = map[string]string{"test.k8s.ngrok.com/touch": rand.String(5)}
				return k8sClient.Update(ctx, got)
			})).To(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(getLegacy(g).ResourceVersion).To(Equal(superseded.ResourceVersion))
			}, settle, interval).Should(Succeed())
		})

		It("ignores edits to a superseded legacy Domain", func() {
			Expect(k8sClient.Create(ctx, newV1())).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(readyOf(g, getLegacy(g).Status.Conditions).Reason).To(Equal(domainpkg.ReasonSupersededByV1))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(reservations()[0].Description).To(Equal("re-stamped"))
			}, timeout, interval).Should(Succeed())

			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				got := &ingressv1alpha1.Domain{}
				if err := k8sClient.Get(ctx, key, got); err != nil {
					return err
				}
				got.Spec.Description = "edited on the legacy object"
				return k8sClient.Update(ctx, got)
			})).To(Succeed())

			Eventually(func(g Gomega) {
				got := getLegacy(g)
				g.Expect(got.Status.ObservedGeneration).To(Equal(got.Generation), "the edit was seen")
				g.Expect(readyOf(g, got.Status.Conditions).Reason).To(Equal(domainpkg.ReasonSupersededByV1))
			}, timeout, interval).Should(Succeed())
			eventuallyEvent(legacy, "Warning", trafficpolicypkg.EventDeprecatedAPIGroup, "this change has no effect")
			Consistently(func(g Gomega) {
				g.Expect(reservations()[0].Description).To(Equal("re-stamped"), "a superseded legacy Domain never writes to the reservation")
			}, settle, interval).Should(Succeed())
		})

		It("keeps the reservation when the legacy Domain is deleted after the re-stamp", func() {
			Expect(k8sClient.Create(ctx, newV1())).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(getV1(g).Status.ID).To(Equal(legacyID))
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
			eventuallyGone(&ingressv1alpha1.Domain{})

			eventuallyEvent(legacy, "Normal", EventDomainReservationKept, legacyID)
			Consistently(func(g Gomega) {
				g.Expect(reservations()).To(HaveLen(1))
			}, settle, interval).Should(Succeed())
		})

		It("keeps the reservation when v1 is applied and legacy deleted back to back", func() {
			Expect(k8sClient.Create(ctx, newV1())).To(Succeed())
			Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())

			eventuallyGone(&ingressv1alpha1.Domain{})
			Eventually(func(g Gomega) {
				g.Expect(getV1(g).Status.ID).To(Equal(legacyID))
			}, timeout, interval).Should(Succeed())
			Expect(reservations()).To(HaveLen(1))
		})

		It("hands the reservation back when the v1 Domain is deleted", func() {
			v1 := newV1()
			Expect(k8sClient.Create(ctx, v1)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(getV1(g).Status.ID).To(Equal(legacyID))
				g.Expect(readyOf(g, getLegacy(g).Status.Conditions).Reason).To(Equal(domainpkg.ReasonSupersededByV1))
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, v1)).To(Succeed())
			eventuallyGone(&ngrokv1.Domain{})

			Expect(reservations()).To(HaveLen(1))
			// The pass that released the v1 finalizer also made legacy the writer.
			Eventually(func(g Gomega) {
				got := getLegacy(g)
				g.Expect(readyOf(g, got.Status.Conditions).Reason).To(Equal(domainpkg.ReasonDomainActive))
				g.Expect(got.Status.ID).To(Equal(legacyID))
				g.Expect(got.Finalizers).To(ContainElement(util.FinalizerName))
				g.Expect(reservations()[0].Description).To(Equal("legacy"), "the legacy Domain writes again")
			}, timeout, interval).Should(Succeed())

			By("writing later edits to the legacy Domain again")
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				got := &ingressv1alpha1.Domain{}
				if err := k8sClient.Get(ctx, key, got); err != nil {
					return err
				}
				got.Spec.Description = "edited after v1 was deleted"
				return k8sClient.Update(ctx, got)
			})).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(reservations()[0].Description).To(Equal("edited after v1 was deleted"))
			}, timeout, interval).Should(Succeed())
		})

		It("releases the reservation when both Domains are deleted", func() {
			v1 := newV1()
			Expect(k8sClient.Create(ctx, v1)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(getV1(g).Status.ID).To(Equal(legacyID))
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
			Expect(k8sClient.Delete(ctx, v1)).To(Succeed())

			eventuallyGone(&ngrokv1.Domain{})
			eventuallyGone(&ingressv1alpha1.Domain{})
			Eventually(func(g Gomega) {
				g.Expect(reservationsFor(hostname)).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("leaves readers on the legacy Domain when the v1 spec push fails on adoption", func() {
			domainClient.SetUpdateError(errors.New("injected update failure"))
			DeferCleanup(domainClient.ClearErrors)

			Expect(k8sClient.Create(ctx, newV1())).To(Succeed())

			Eventually(func(g Gomega) {
				got := getV1(g)
				g.Expect(got.Status.ID).To(Equal(legacyID))
				g.Expect(got.Status.Domain).To(Equal(hostname), "status is filled from the adopted reservation")
				g.Expect(meta.FindStatusCondition(got.Status.Conditions, domainpkg.ConditionReady)).ToNot(BeNil())
				g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, domainpkg.ConditionReady)).To(BeFalse())
			}, timeout, interval).Should(Succeed())

			Consistently(func(g Gomega) {
				g.Expect(readyOf(g, getLegacy(g).Status.Conditions).Reason).ToNot(Equal(domainpkg.ReasonSupersededByV1))

				pair, err := domainpkg.GetPair(ctx, k8sClient, key)
				g.Expect(err).ToNot(HaveOccurred())
				authoritative, err := pair.Authoritative()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(authoritative.IsLegacy()).To(BeTrue(), "readers stay on the working legacy Domain")
			}, settle, interval).Should(Succeed())
			Expect(reservations()[0].Description).To(Equal("legacy"))
		})

		// Twins are keyed by namespace/name, never by hostname: v1 is the only
		// writer and the legacy Domain goes passive. They don't share a
		// reservation, though, so v1 doesn't adopt the legacy ID, and deleting
		// the legacy Domain releases its own.
		It("treats a v1 Domain with a different hostname as a twin that shares nothing", func() {
			otherHostname := fmt.Sprintf("other-%s.ngrok.app", rand.String(8))
			v1 := newV1()
			v1.Spec.Domain = otherHostname
			Expect(k8sClient.Create(ctx, v1)).To(Succeed())

			Eventually(func(g Gomega) {
				gotV1 := getV1(g)
				g.Expect(gotV1.Status.ID).ToNot(BeEmpty())
				g.Expect(gotV1.Status.ID).ToNot(Equal(legacyID), "v1 reserves its own hostname instead of adopting")

				ready := readyOf(g, getLegacy(g).Status.Conditions)
				g.Expect(ready.Reason).To(Equal(domainpkg.ReasonSupersededByV1), "only v1 writes")
				g.Expect(ready.Message).To(ContainSubstring("different hostname"))
			}, timeout, interval).Should(Succeed())
			Expect(reservationsFor(otherHostname)).To(HaveLen(1))
			Expect(reservations()).To(HaveLen(1))
			eventuallyEvent(legacy, "Warning", EventDomainTwinMismatch, otherHostname)
			eventuallyEvent(v1, "Warning", EventDomainTwinMismatch, otherHostname)

			Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
			eventuallyGone(&ingressv1alpha1.Domain{})

			Eventually(func(g Gomega) {
				g.Expect(reservations()).To(BeEmpty(), "the v1 twin doesn't hold the legacy reservation")
			}, timeout, interval).Should(Succeed())
			Expect(reservationsFor(otherHostname)).To(HaveLen(1))
		})

		It("runs only deletes while draining", func() {
			domainDrain.Store(true)
			DeferCleanup(func() { domainDrain.Store(false) })

			Expect(k8sClient.Create(ctx, newV1())).To(Succeed())
			Consistently(func(g Gomega) {
				got := getV1(g)
				g.Expect(got.Status.ID).To(BeEmpty(), "no adoption while draining")
				g.Expect(got.Finalizers).To(BeEmpty(), "no new finalizers while draining")
			}, settle, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
			eventuallyGone(&ingressv1alpha1.Domain{})

			Expect(reservations()).To(HaveLen(1), "the v1 twin names the same hostname, so it still holds the reservation")
			Expect(reservations()[0].Description).To(Equal("legacy"), "no sync of the surviving twin while draining")
			Consistently(func(g Gomega) {
				g.Expect(getV1(g).Status.ID).To(BeEmpty())
			}, settle, interval).Should(Succeed())
		})
	})

	// A legacy Domain created after its v1 twin took over is superseded on
	// arrival and never gets a finalizer.
	It("hands the reservation to a finalizerless legacy twin when the v1 Domain is deleted", func() {
		v1 := newV1()
		Expect(k8sClient.Create(ctx, v1)).To(Succeed())
		var v1ID string
		Eventually(func(g Gomega) {
			v1ID = getV1(g).Status.ID
			g.Expect(v1ID).ToNot(BeEmpty())
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Create(ctx, newLegacy())).To(Succeed())
		Eventually(func(g Gomega) {
			got := getLegacy(g)
			g.Expect(readyOf(g, got.Status.Conditions).Reason).To(Equal(domainpkg.ReasonSupersededByV1))
			g.Expect(got.Finalizers).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Delete(ctx, v1)).To(Succeed())
		eventuallyGone(&ngrokv1.Domain{})

		Eventually(func(g Gomega) {
			got := getLegacy(g)
			g.Expect(got.Status.ID).To(Equal(v1ID), "the legacy Domain picks up the same reservation")
			g.Expect(readyOf(g, got.Status.Conditions).Reason).To(Equal(domainpkg.ReasonDomainActive))
			g.Expect(got.Finalizers).To(ContainElement(util.FinalizerName), "the legacy Domain is the writer again")
		}, timeout, interval).Should(Succeed())
		Expect(reservations()).To(HaveLen(1))
	})

	It("releases a legacy Domain for an internal hostname without reserving it", func() {
		internalHostname := fmt.Sprintf("svc-%s.ns.internal", rand.String(8))
		legacy := newLegacy()
		legacy.Spec.Domain = internalHostname
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())

		Eventually(func(g Gomega) {
			got := getLegacy(g)
			g.Expect(got.Finalizers).To(BeEmpty())
			ready := readyOf(g, got.Status.Conditions)
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Message).To(ContainSubstring(".internal domains do not need to be reserved"))
		}, timeout, interval).Should(Succeed())
		Expect(reservationsFor(internalHostname)).To(BeEmpty())
	})
})
