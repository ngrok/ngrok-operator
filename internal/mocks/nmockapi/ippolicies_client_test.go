package nmockapi_test

import (
	context "context"
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IPPolicyClient", func() {
	const ()

	var (
		ipPolicyClient *nmockapi.IPPolicyClient
		ctx            context.Context
	)

	BeforeEach(func() {
		ipPolicyClient = nmockapi.NewIPPolicyClient()
		ctx = GinkgoT().Context()
	})

	Describe("Get()", func() {
		var (
			id       string
			ipPolicy *ngrok.IPPolicy
			err      error
		)

		JustBeforeEach(func() {
			ipPolicy, err = ipPolicyClient.Get(ctx, id)
		})

		When("the IP policy exists", func() {
			BeforeEach(func() {
				ipPolicy, err := ipPolicyClient.Create(ctx, &ngrok.IPPolicyCreate{
					Description: "test-ip-policy",
				})
				Expect(err).NotTo(HaveOccurred())
				id = ipPolicy.ID
			})

			It("should return the IP policy", func() {
				Expect(err).To(BeNil())
				Expect(ipPolicy.Description).To(Equal("test-ip-policy"))
				Expect(ipPolicy.ID).To(MatchRegexp("^ipp_"))
			})
		})

		When("the IP policy does not exist", func() {
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
			ipPolicies []*ngrok.IPPolicy
			err        error
		)

		JustBeforeEach(func() {
			ipPolicies = make([]*ngrok.IPPolicy, 0)
			iter := ipPolicyClient.List(nil)
			for iter.Next(ctx) {
				ipPolicies = append(ipPolicies, iter.Item())
			}
			err = iter.Err()
		})

		When("there are multiple IP policies", func() {
			BeforeEach(func() {
				_, err := ipPolicyClient.Create(ctx, &ngrok.IPPolicyCreate{
					Description: "test-ip-policy-1",
				})
				Expect(err).NotTo(HaveOccurred())
				_, err = ipPolicyClient.Create(ctx, &ngrok.IPPolicyCreate{
					Description: "test-ip-policy-2",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return all IP policies", func() {
				Expect(err).To(BeNil())
				Expect(ipPolicies).To(HaveLen(2))
			})
		})

		When("there are no IP policies", func() {
			It("should return an empty list", func() {
				Expect(err).To(BeNil())
				Expect(ipPolicies).To(BeEmpty())
			})
		})
	})

	Describe("Create()", func() {
		var (
			ipPolicyCreate *ngrok.IPPolicyCreate
			createdPolicy  *ngrok.IPPolicy
			err            error
		)

		JustBeforeEach(func() {
			createdPolicy, err = ipPolicyClient.Create(ctx, ipPolicyCreate)
		})

		When("the IP policy create request is valid", func() {
			BeforeEach(func() {
				ipPolicyCreate = &ngrok.IPPolicyCreate{
					Description: "valid-ip-policy",
				}
			})

			It("should create and return the IP policy", func() {
				Expect(err).To(BeNil())
				Expect(createdPolicy).NotTo(BeNil())
				Expect(createdPolicy.Description).To(Equal("valid-ip-policy"))
				Expect(createdPolicy.ID).To(MatchRegexp("^ipp_"))
			})
		})

		When("the IP policy create request is invalid", func() {
			BeforeEach(func() {
				ipPolicyCreate = &ngrok.IPPolicyCreate{
					Description: "", // Assuming description is required
				}
			})

			It("should return a validation error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Delete()", func() {
		var (
			id  string
			err error
		)

		JustBeforeEach(func() {
			err = ipPolicyClient.Delete(ctx, id)
		})

		When("the IP policy exists", func() {
			BeforeEach(func() {
				ipPolicy, err := ipPolicyClient.Create(ctx, &ngrok.IPPolicyCreate{
					Description: "ip-policy-to-delete",
				})
				Expect(err).NotTo(HaveOccurred())
				id = ipPolicy.ID
			})

			It("should delete the IP policy", func() {
				Expect(err).To(BeNil())
				_, err = ipPolicyClient.Get(ctx, id)
				Expect(ngrok.IsNotFound(err)).To(BeTrue())
			})
		})

		When("the IP policy does not exist", func() {
			BeforeEach(func() {
				id = "non-existing-id"
			})

			It("should return an ngrok not found error", func() {
				Expect(err).To(HaveOccurred())
				Expect(ngrok.IsNotFound(err)).To(BeTrue())
			})
		})
	})
})
