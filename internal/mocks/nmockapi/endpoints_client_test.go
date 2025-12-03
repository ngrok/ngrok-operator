package nmockapi_test

import (
	context "context"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

var _ = Describe("EndpointsClient", func() {
	const ()

	var (
		endpointsClient *nmockapi.EndpointsClient
		ctx             context.Context
	)

	BeforeEach(func() {
		endpointsClient = nmockapi.NewEndpointsClient()
		ctx = GinkgoT().Context()
	})

	Describe("Get()", func() {
		var (
			id       string
			endpoint *ngrok.Endpoint
			err      error
		)

		JustBeforeEach(func() {
			endpoint, err = endpointsClient.Get(ctx, id)
		})

		When("the endpoint exists", func() {
			BeforeEach(func() {
				endpoint, err := endpointsClient.Create(ctx, &ngrok.EndpointCreate{
					Description: ptr.To("test-endpoint"),
				})
				Expect(err).NotTo(HaveOccurred())
				id = endpoint.ID
			})

			It("should return the endpoint", func() {
				Expect(err).To(BeNil())
				Expect(endpoint.Description).To(Equal("test-endpoint"))
				Expect(endpoint.ID).To(MatchRegexp("^ep_"))
			})
		})

		When("the endpoint does not exist", func() {
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
			endpoints []*ngrok.Endpoint
			err       error
		)

		JustBeforeEach(func() {
			endpoints = make([]*ngrok.Endpoint, 0)
			iter := endpointsClient.List(nil)
			for iter.Next(ctx) {
				endpoints = append(endpoints, iter.Item())
			}
			err = iter.Err()
		})

		When("there are multiple endpoints", func() {
			BeforeEach(func() {
				_, err := endpointsClient.Create(ctx, &ngrok.EndpointCreate{
					URL:         "http://example1.com",
					Description: ptr.To("test-endpoint-1"),
				})
				Expect(err).NotTo(HaveOccurred())
				_, err = endpointsClient.Create(ctx, &ngrok.EndpointCreate{
					URL:         "http://example2.com",
					Description: ptr.To("test-endpoint-2"),
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return all endpoints", func() {
				Expect(err).To(BeNil())
				Expect(endpoints).To(HaveLen(2))
			})
		})

		When("there are no endpoints", func() {
			It("should return an empty list", func() {
				Expect(err).To(BeNil())
				Expect(endpoints).To(BeEmpty())
			})
		})
	})

	Describe("Create()", func() {
		var (
			endpointCreate  *ngrok.EndpointCreate
			createdEndpoint *ngrok.Endpoint
			err             error
		)

		JustBeforeEach(func() {
			createdEndpoint, err = endpointsClient.Create(ctx, endpointCreate)
		})

		When("the endpoint create request is valid", func() {
			BeforeEach(func() {
				endpointCreate = &ngrok.EndpointCreate{
					URL:         "http://example.com",
					Description: ptr.To("valid-endpoint"),
				}
			})

			It("should create and return the endpoint", func() {
				Expect(err).To(BeNil())
				Expect(createdEndpoint).NotTo(BeNil())
				Expect(createdEndpoint.Description).To(Equal("valid-endpoint"))
				Expect(createdEndpoint.ID).To(MatchRegexp("^ep_"))
			})
		})

		When("the endpoint already exists", func() {
			BeforeEach(func(ctx SpecContext) {
				endpointCreate = &ngrok.EndpointCreate{
					URL:         "http://example.com",
					Description: ptr.To("endpoint-1"),
				}
				_, err := endpointsClient.Create(ctx, endpointCreate)
				Expect(err).NotTo(HaveOccurred())
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
			err = endpointsClient.Delete(ctx, id)
		})

		When("the endpoint exists", func() {
			BeforeEach(func() {
				endpoint, err := endpointsClient.Create(ctx, &ngrok.EndpointCreate{
					Description: ptr.To("endpoint-to-delete"),
				})
				Expect(err).NotTo(HaveOccurred())
				id = endpoint.ID
			})

			It("should delete the endpoint", func() {
				Expect(err).To(BeNil())
				_, err = endpointsClient.Get(ctx, id)
				Expect(ngrok.IsNotFound(err)).To(BeTrue())
			})
		})

		When("the endpoint does not exist", func() {
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
