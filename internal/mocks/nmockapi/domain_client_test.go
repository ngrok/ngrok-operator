package nmockapi_test

import (
	context "context"
	"fmt"
	"slices"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("DomainClient", func() {
	const (
		NgrokManagedDomainSuffix = "ngrok.app"
		CustomDomainSuffix       = "custom-domain.xyz"
	)

	var (
		domainClient *nmockapi.DomainClient
		ctx          context.Context
	)

	BeforeEach(func() {
		domainClient = nmockapi.NewDomainClient()
		ctx = GinkgoT().Context()
	})

	Describe("Get()", func() {
		var (
			id     string
			domain *ngrok.ReservedDomain
			err    error
		)

		JustBeforeEach(func() {
			domain, err = domainClient.Get(ctx, id)
		})

		When("the domain exists", func() {
			BeforeEach(func() {
				domain, err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
					Domain: "test-domain.ngrok.io",
				})
				Expect(err).NotTo(HaveOccurred())
				id = domain.ID
			})

			It("should return the domain", func() {
				Expect(err).To(BeNil())
				Expect(domain.Domain).To(Equal("test-domain.ngrok.io"))
				Expect(domain.ID).To(MatchRegexp("^rd_"))
			})
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				id = "non-existing-id"
			})

			It("should return an ngrok not found error", func() {
				Expect(err).To(HaveOccurred())
				Expect(ngrok.IsNotFound(err)).To(BeTrue())
			})
		})
	})

	Describe("List()", func() {
		var (
			domains []*ngrok.ReservedDomain
			err     error
		)

		JustBeforeEach(func() {
			iter := domainClient.List(nil)

			domains = make([]*ngrok.ReservedDomain, 0)
			for iter.Next(ctx) {
				domains = append(domains, iter.Item())
			}
			err = iter.Err()
		})

		When("there are no domains", func() {
			It("the iterator should return an empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(domains).To(BeEmpty())
			})
		})

		When("there are domains", func() {
			BeforeEach(func() {
				_, createDomain1Err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
					Domain: "test-domain-1.ngrok.io",
				})
				Expect(createDomain1Err).ToNot(HaveOccurred())
				_, createDomain2Err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
					Domain: "test-domain-2.ngrok.io",
				})
				Expect(createDomain2Err).ToNot(HaveOccurred())
			})

			It("the iterator should return the domains", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(domains).To(HaveLen(2))

				Expect(slices.ContainsFunc(domains, func(domain *ngrok.ReservedDomain) bool {
					return domain.Domain == "test-domain-1.ngrok.io"
				})).To(BeTrue())
				Expect(slices.ContainsFunc(domains, func(domain *ngrok.ReservedDomain) bool {
					return domain.Domain == "test-domain-2.ngrok.io"
				})).To(BeTrue())
			})
		})
	})

	Describe("Create()", func() {
		var (
			domain       *ngrok.ReservedDomain
			err          error
			domainSuffix string
			domainName   string
		)

		JustBeforeEach(func() {
			domain, err = domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
				Domain: domainName,
			})
		})

		When("the domain is a ngrok managed domain", func() {
			BeforeEach(func() {
				domainSuffix = NgrokManagedDomainSuffix
				domainName = fmt.Sprintf("test-domain-%s.%s", rand.String(10), domainSuffix)
			})

			It("should create and return the domain", func() {
				Expect(err).To(BeNil())
				Expect(domain.Domain).To(Equal(domainName))
			})

			It("should create the domain with an ID", func() {
				Expect(domain.ID).ToNot(BeEmpty())
				Expect(domain.ID).To(MatchRegexp("^rd_"))
			})

			It("should create the domain with a timestamp", func() {
				Expect(domain.CreatedAt).ToNot(BeEmpty())
			})

			It("should not create the domain with a CNAMETarget", func() {
				Expect(domain.CNAMETarget).To(BeNil())
			})

			When("the domain is already taken", func() {
				BeforeEach(func() {
					_, err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{Domain: domainName})
					Expect(err).ToNot(HaveOccurred())
				})

				It("should return an ngrok already exists error", func() {
					Expect(err).To(HaveOccurred())
					Expect(ngrok.IsErrorCode(err, 413)).To(BeTrue())
				})
			})
		})

		When("the domain is a custom domain", func() {
			BeforeEach(func() {
				domainSuffix = CustomDomainSuffix
				domainName = fmt.Sprintf("test-domain-%s.%s", rand.String(10), domainSuffix)
			})

			It("should create and return the domain", func() {
				Expect(err).To(BeNil())
				Expect(domain.Domain).To(Equal(domainName))
			})

			It("should create the domain with an ID", func() {
				Expect(domain.ID).ToNot(BeEmpty())
				Expect(domain.ID).To(MatchRegexp("^rd_"))
			})

			It("should create the domain with a timestamp", func() {
				Expect(domain.CreatedAt).ToNot(BeEmpty())
			})

			It("should create the domain with a CNAMETarget", func() {
				Expect(domain.CNAMETarget).ToNot(BeNil())
				Expect(*domain.CNAMETarget).To(MatchRegexp("^[a-zA-Z0-9]{17}\\.[a-zA-Z0-9]{17}\\.ngrok-cname\\.com$"))
			})

			When("the domain is already taken", func() {
				BeforeEach(func() {
					_, err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
						Domain: domainName,
					})
					Expect(err).ToNot(HaveOccurred())
				})

				It("should return an ngrok already exists error", func() {
					Expect(err).To(HaveOccurred())
					Expect(ngrok.IsErrorCode(err, 413)).To(BeTrue())
				})
			})
		})
	})

	Describe("Delete()", func() {
		var (
			id        string
			deleteErr error
			domain    *ngrok.ReservedDomain
			getErr    error
		)

		JustBeforeEach(func() {
			deleteErr = domainClient.Delete(ctx, id)
			domain, getErr = domainClient.Get(ctx, id)
		})

		When("the domain exists", func() {
			BeforeEach(func() {
				domain, err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
					Domain: "test-domain-4.ngrok.io",
				})
				Expect(err).ToNot(HaveOccurred())
				id = domain.ID
			})

			It("should delete the domain", func() {
				Expect(deleteErr).ToNot(HaveOccurred())

				Expect(getErr).To(HaveOccurred())
				Expect(ngrok.IsNotFound(getErr)).To(BeTrue())

				Expect(domain).To(BeNil())
			})
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				id = "non-existing-id"
			})

			It("Should return an ngrok not found error", func() {
				Expect(deleteErr).To(HaveOccurred())
				Expect(ngrok.IsNotFound(deleteErr)).To(BeTrue())
			})
		})
	})

	Describe("Update()", func() {
		var (
			createdDomain *ngrok.ReservedDomain
			createErr     error
			domainUpdate  *ngrok.ReservedDomainUpdate

			updatedDomain *ngrok.ReservedDomain
			updateErr     error
		)

		BeforeEach(func() {
			createdDomain, createErr = domainClient.Create(ctx, &ngrok.ReservedDomainCreate{
				Domain: "test-domain-5.ngrok.io",
			})
			Expect(createErr).ToNot(HaveOccurred())
		})

		JustBeforeEach(func() {
			updatedDomain, updateErr = domainClient.Update(ctx, domainUpdate)
		})

		When("the domain exists", func() {
			BeforeEach(func() {
				domainUpdate = &ngrok.ReservedDomainUpdate{
					ID:          createdDomain.ID,
					Metadata:    ngrok.String("new-metadata"),
					Description: ngrok.String("new-description"),
				}
			})

			It("should update the domain", func() {
				Expect(updateErr).ToNot(HaveOccurred())
				Expect(updatedDomain).ToNot(BeNil())
				Expect(updatedDomain.ID).To(Equal(createdDomain.ID))
				Expect(updatedDomain.Metadata).To(Equal("new-metadata"))
				Expect(updatedDomain.Description).To(Equal("new-description"))
			})
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				domainUpdate = &ngrok.ReservedDomainUpdate{
					ID:          "non-existing-id",
					Metadata:    ngrok.String("new-metadata"),
					Description: ngrok.String("new-description"),
				}
			})

			It("should return a not found error", func() {
				Expect(updateErr).To(HaveOccurred())
				Expect(ngrok.IsNotFound(updateErr)).To(BeTrue())
			})
		})
	})
})
