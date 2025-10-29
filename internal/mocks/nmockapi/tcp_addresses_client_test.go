package nmockapi

import (
	context "context"

	"github.com/ngrok/ngrok-api-go/v7"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TCPAddressesClient", func() {
	var (
		client *TCPAddressesClient
		ctx    context.Context
	)

	BeforeEach(func() {
		client = NewTCPAddressClient()
		ctx = GinkgoT().Context()
	})

	Describe("Create", func() {
		var (
			addr *ngrok.ReservedAddr
			err  error
		)

		JustBeforeEach(func() {
			addr, err = client.Create(ctx, &ngrok.ReservedAddrCreate{
				Region:      "us",
				Description: "test address",
			})
		})

		It("should create a new reserved address", func() {
			Expect(err).To(BeNil())
			Expect(addr).ToNot(BeNil())
			Expect(addr.ID).ToNot(BeEmpty())
			Expect(addr.CreatedAt).ToNot(BeEmpty())
			Expect(addr.Region).To(Equal("us"))
			Expect(addr.Description).To(Equal("test address"))
			Expect(addr.URI).To(ContainSubstring(addr.ID))
			Expect(addr.Addr).To(MatchRegexp(`^[0-6]\.tcp\.ngrok\.io:[1-3]\d{4}$`))
		})
	})

	Describe("List", func() {
		var (
			addrs []*ngrok.ReservedAddr
			err   error
		)

		JustBeforeEach(func(ctx SpecContext) {
			addrs = []*ngrok.ReservedAddr{}
			iter := client.List(nil)
			for iter.Next(ctx) {
				addrs = append(addrs, iter.Item())
			}
			err = iter.Err()
		})

		When("there are no addresses", func() {
			It("the iterator should return an empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(addrs).To(BeEmpty())
			})
		})

		When("there are addresses", func() {
			BeforeEach(func() {
				_, createAddr1Err := client.Create(ctx, &ngrok.ReservedAddrCreate{
					Region:      "us",
					Description: "addr 1",
				})
				Expect(createAddr1Err).ToNot(HaveOccurred())

				_, createAddr2Err := client.Create(ctx, &ngrok.ReservedAddrCreate{
					Region:      "us",
					Description: "addr 2",
				})
				Expect(createAddr2Err).ToNot(HaveOccurred())
			})

			It("the iterator should return the list of addresses", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(addrs).To(HaveLen(2))
			})
		})
	})

	Describe("Delete", func() {
		var (
			id     string
			delErr error
		)

		Context("when the address exists", func() {
			BeforeEach(func() {
				addr, err := client.Create(ctx, &ngrok.ReservedAddrCreate{
					Region:      "us",
					Description: "to-delete",
				})
				Expect(err).ToNot(HaveOccurred())
				id = addr.ID
			})

			JustBeforeEach(func() {
				delErr = client.Delete(ctx, id)
			})

			It("should delete the address without error", func() {
				Expect(delErr).ToNot(HaveOccurred())
			})
		})

		Context("when the address does not exist", func() {
			BeforeEach(func() {
				id = "non-existent-id"
			})

			JustBeforeEach(func() {
				delErr = client.Delete(ctx, id)
			})

			It("should return a not found error", func() {
				Expect(delErr).To(HaveOccurred())
				Expect(ngrok.IsNotFound(delErr)).To(BeTrue())
			})
		})
	})
})
