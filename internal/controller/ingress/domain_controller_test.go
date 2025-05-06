package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DomainReconciler", func() {
	const (
		timeout                  = 10 * time.Second
		duration                 = 10 * time.Second
		interval                 = 250 * time.Millisecond
		NgrokManagedDomainSuffix = "ngrok.app"
		CustomDomainSuffix       = "custom-domain.xyz"
	)

	var (
		ctx       context.Context
		namespace string = "default"
	)

	BeforeEach(func() {
		ctx = GinkgoT().Context()
	})

	AfterEach(func() {
		// List all domain CRs in the env test cluster
		domains := &ingressv1alpha1.DomainList{}
		err := k8sClient.List(ctx, domains)
		Expect(err).ToNot(HaveOccurred())
		// Delete all domain CRs in the env test cluster
		for _, d := range domains.Items {
			Expect(k8sClient.Delete(ctx, &d)).To(Succeed())
		}

		// Eventually, listing all the domain CRs should return an empty list because we
		// deleted all of them and the finalizer should have cleaned up the ngrok domains.
		Eventually(func(g Gomega) {
			domains := &ingressv1alpha1.DomainList{}
			err := k8sClient.List(ctx, domains)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(domains.Items).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		// Reset the internal state of the domain client between tests
		// to ensure that each test starts with a clean slate.
		domainClient.Reset()
	})

	Describe("CreateDomain", func() {
		var (
			createDomainErr error
			domainSuffix    string
			domainName      string
			domain          *ingressv1alpha1.Domain
		)

		JustBeforeEach(func() {
			domain = &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-domain",
					Namespace: namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: domainName,
				},
			}
			createDomainErr = k8sClient.Create(ctx, domain)
		})

		When("the domain is a ngrok managed domain", func() {
			BeforeEach(func() {
				domainSuffix = NgrokManagedDomainSuffix
				domainName = fmt.Sprintf("test-domain-%s.%s", rand.String(10), domainSuffix)
			})

			When("the domain does not exist in ngrok", func() {
				It("should create the domain in ngrok", func() {
					Expect(createDomainErr).ToNot(HaveOccurred())

					Eventually(func(g Gomega) {
						foundDomain := &ingressv1alpha1.Domain{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), foundDomain)
						g.Expect(err).ToNot(HaveOccurred())

						g.Expect(foundDomain.Status.ID).To(MatchRegexp("^rd"))
						g.Expect(foundDomain.Status.Domain).To(Equal(domainName))
						g.Expect(foundDomain.Status.CNAMETarget).To(BeNil())
					}, timeout, interval).Should(Succeed())
				})
			})

			When("the domain already exists in ngrok", func() {
				var (
					existingDomain     *ngrok.ReservedDomain
					preCreateDomainErr error
				)
				BeforeEach(func() {
					existingDomain, preCreateDomainErr = domainClient.Create(ctx, &ngrok.ReservedDomainCreate{Domain: domainName})
					Expect(preCreateDomainErr).ToNot(HaveOccurred())
				})

				It("should use the existing domain in ngrok", func() {
					Expect(createDomainErr).ToNot(HaveOccurred())

					Eventually(func(g Gomega) {
						foundDomain := &ingressv1alpha1.Domain{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), foundDomain)
						g.Expect(err).ToNot(HaveOccurred())

						g.Expect(foundDomain.Status.ID).To(Equal(existingDomain.ID))
						g.Expect(foundDomain.Status.Domain).To(Equal(domainName))
						g.Expect(foundDomain.Status.CNAMETarget).To(BeNil())
					}, timeout, interval).Should(Succeed())
				})
			})
		})

		When("the domain is a custom domain", func() {
			BeforeEach(func() {
				domainSuffix = CustomDomainSuffix
				domainName = fmt.Sprintf("test-domain-%s.%s", rand.String(10), domainSuffix)
			})

			When("the domain does not exist in ngrok", func() {
				It("should create the domain in ngrok", func() {
					Expect(createDomainErr).ToNot(HaveOccurred())

					Eventually(func(g Gomega) {
						foundDomain := &ingressv1alpha1.Domain{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), foundDomain)
						g.Expect(err).ToNot(HaveOccurred())

						g.Expect(foundDomain.Status.ID).To(MatchRegexp("^rd"))
						g.Expect(foundDomain.Status.Domain).To(Equal(domainName))
						g.Expect(foundDomain.Status.CNAMETarget).ToNot(BeNil())
						g.Expect(*foundDomain.Status.CNAMETarget).To(MatchRegexp("\\.ngrok-cname\\.com$"))
					}, timeout, interval).Should(Succeed())
				})
			})

			When("the domain already exists in ngrok", func() {
				var (
					existingDomain     *ngrok.ReservedDomain
					preCreateDomainErr error
				)

				BeforeEach(func() {
					existingDomain, preCreateDomainErr = domainClient.Create(ctx, &ngrok.ReservedDomainCreate{Domain: domainName})
					Expect(preCreateDomainErr).ToNot(HaveOccurred())

					GinkgoLogr.Info("Pre-created domain", "domain", existingDomain.Domain, "id", existingDomain.ID)
				})

				It("should use the existing domain in ngrok", func() {
					Expect(createDomainErr).ToNot(HaveOccurred())

					Eventually(func(g Gomega) {
						foundDomain := &ingressv1alpha1.Domain{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), foundDomain)
						g.Expect(err).ToNot(HaveOccurred())

						g.Expect(foundDomain.Status.ID).To(Equal(existingDomain.ID))
						g.Expect(foundDomain.Status.Domain).To(Equal(domainName))
						g.Expect(foundDomain.Status.CNAMETarget).ToNot(BeNil())
					}, timeout, interval).Should(Succeed())
				})
			})
		})
	})

	Describe("UpdateDomain", func() {
		var (
			domainName string
			domain     *ingressv1alpha1.Domain
			objKey     client.ObjectKey
		)

		BeforeEach(func() {
			name := fmt.Sprintf("test-domain-%s", rand.String(10))
			domainName = fmt.Sprintf("test-domain-%s.%s", rand.String(10), NgrokManagedDomainSuffix)
			objKey = client.ObjectKey{
				Name:      name,
				Namespace: namespace,
			}
			domain = &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: domainName,
				},
			}
			domain.Spec.Metadata = "starting metadata"
			domain.Spec.Description = "starting description"

			Expect(k8sClient.Create(ctx, domain)).To(Succeed())
		})

		It("updates the domain metadata", func() {
			patch := client.MergeFrom(domain.DeepCopy())
			domain.Spec.Metadata = "updated metadata"
			Expect(k8sClient.Patch(ctx, domain, patch)).To(Succeed())

			Eventually(func(g Gomega) {
				d := &ingressv1alpha1.Domain{}
				err := k8sClient.Get(ctx, objKey, d)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(d.Spec.Metadata).To(Equal("updated metadata"))
				g.Expect(d.Status.ID).ToNot(BeEmpty())
				g.Expect(d.Status.Domain).To(Equal(domainName))
			}, timeout, interval).Should(Succeed())
		})

		It("updates the domain description", func() {
			patch := client.MergeFrom(domain.DeepCopy())
			domain.Spec.Description = "updated description"
			Expect(k8sClient.Patch(ctx, domain, patch)).To(Succeed())

			Eventually(func(g Gomega) {
				d := &ingressv1alpha1.Domain{}
				err := k8sClient.Get(ctx, objKey, d)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(d.Spec.Description).To(Equal("updated description"))
				g.Expect(d.Status.ID).ToNot(BeEmpty())
				g.Expect(d.Status.Domain).To(Equal(domainName))
			}, timeout, interval).Should(Succeed())
		})

		When("the domain was manually deleted in ngrok", func() {
			var (
				previousID string
			)
			BeforeEach(func() {
				Eventually(func(g Gomega) {
					d := &ingressv1alpha1.Domain{}
					err := k8sClient.Get(ctx, objKey, d)
					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(d.Status.ID).ToNot(BeEmpty())
					g.Expect(d.Status.Domain).To(Equal(domainName))

					previousID = d.Status.ID
					g.Expect(domainClient.Delete(ctx, previousID)).To(Succeed())
				})
			})

			It("should create a new domain in ngrok", func() {
				// Simulate a manual reconcile by adding an annotation
				patch := client.MergeFrom(domain.DeepCopy())
				controller.AddAnnotations(domain, map[string]string{
					"manual-reconcile": "true",
				})
				Expect(k8sClient.Patch(ctx, domain, patch)).To(Succeed())

				Eventually(func(g Gomega) {
					d := &ingressv1alpha1.Domain{}
					err := k8sClient.Get(ctx, objKey, d)
					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(d.Status.ID).ToNot(BeEmpty())
					g.Expect(d.Status.ID).ToNot(Equal(previousID))
					g.Expect(d.Status.Domain).To(Equal(domainName))
					g.Expect(d.Status.CNAMETarget).To(BeNil())
				}, timeout, interval).Should(Succeed())
			})
		})
	})

	Describe("DeleteDomain", func() {
		var (
			reclaimPolicy ingressv1alpha1.DomainReclaimPolicy
			domain        *ingressv1alpha1.Domain

			ngrokDomainID                string
			deleteDomainInNgrokBeforeK8s bool
		)

		JustBeforeEach(func() {
			By("Creating the domain")
			domain = &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-domain",
					Namespace: namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					ReclaimPolicy: reclaimPolicy,
					Domain:        fmt.Sprintf("test-domain-%s.%s", rand.String(10), NgrokManagedDomainSuffix),
				},
			}
			Expect(k8sClient.Create(ctx, domain)).To(Succeed())

			By("Waiting for the domain to be created in ngrok")
			Eventually(func(g Gomega) {
				d := &ingressv1alpha1.Domain{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), d)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(d.Status.ID).ToNot(BeEmpty())
				g.Expect(d.Status.Domain).To(Equal(domain.Spec.Domain))

				ngrokDomainID = d.Status.ID
			}, timeout, interval).Should(Succeed())

			if deleteDomainInNgrokBeforeK8s {
				By("Deleting the domain in ngrok")
				// Simulate the domain being deleted in ngrok
				Expect(domainClient.Delete(ctx, ngrokDomainID)).To(Succeed())
			}

			By("Deleting the domain CR in k8s")
			Expect(k8sClient.Delete(ctx, domain)).To(Succeed())

			By("Waiting for the domain to be deleted in kubernetes")
			Eventually(func(g Gomega) {
				d := &ingressv1alpha1.Domain{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), d)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		When("The domain exists in ngrok", func() {
			When("The reclaim policy is set to delete", func() {
				BeforeEach(func() {
					reclaimPolicy = ingressv1alpha1.DomainReclaimPolicyDelete
				})

				It("should delete the domain in ngrok", func() {
					rd, err := domainClient.Get(ctx, ngrokDomainID)
					Expect(ngrok.IsNotFound(err)).To(BeTrue())
					Expect(rd).To(BeNil())
				})
			})

			When("The reclaim policy is set to retain", func() {
				BeforeEach(func() {
					reclaimPolicy = ingressv1alpha1.DomainReclaimPolicyRetain
				})

				It("should not delete the domain in ngrok", func() {
					rd, err := domainClient.Get(ctx, ngrokDomainID)
					Expect(err).ToNot(HaveOccurred())
					Expect(rd).ToNot(BeNil())
					Expect(rd.ID).To(Equal(ngrokDomainID))
				})
			})
		})

		When("The domain does not exist in ngrok", func() {
			BeforeEach(func() {
				deleteDomainInNgrokBeforeK8s = true
			})

			When("The reclaim policy is set to delete", func() {
				BeforeEach(func() {
					reclaimPolicy = ingressv1alpha1.DomainReclaimPolicyDelete
				})

				It("The domain should be deleted in ngrok", func() {
					rd, err := domainClient.Get(ctx, ngrokDomainID)
					Expect(ngrok.IsNotFound(err)).To(BeTrue())
					Expect(rd).To(BeNil())
				})
			})

			When("The reclaim policy is set to retain", func() {
				BeforeEach(func() {
					reclaimPolicy = ingressv1alpha1.DomainReclaimPolicyRetain
				})

				It("The domain is still missing in ngrok", func() {
					iter := domainClient.List(&ngrok.Paging{})
					for iter.Next(ctx) {
						d := iter.Item()
						if d.Domain == domain.Spec.Domain {
							Fail("Domain should not exist in ngrok")
						}
					}
					Expect(iter.Err()).To(BeNil())
				})
			})
		})
	})
})
