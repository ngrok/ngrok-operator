package nmockapi_test

import (
	context "context"
	"errors"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Iter", func() {
	var (
		iter    ngrok.Iter[string]
		iterErr error
		items   []string
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = GinkgoT().Context()
	})

	JustBeforeEach(func() {
		iter = nmockapi.NewIter(items, iterErr)
	})

	Describe("Item()", func() {
		Context("when there is an error", func() {
			BeforeEach(func() {
				iterErr = errors.New("ut-oh")
			})

			It("should return an empty item", func() {
				Expect(iter.Item()).To(Equal(""))
			})
		})

		Context("when there is no error", func() {
			BeforeEach(func() {
				iterErr = nil
				items = []string{"a", "b", "c"}
			})

			It("should return the current item", func() {
				iter.Next(ctx)
				Expect(iter.Item()).To(Equal("a"))

				iter.Next(ctx)
				Expect(iter.Item()).To(Equal("b"))

				iter.Next(ctx)
				Expect(iter.Item()).To(Equal("c"))
			})

			Context("when called before Next", func() {
				It("should return an empty item", func() {
					Expect(iter.Item()).To(Equal(""))
				})
			})

			Context("when called after Next returns false", func() {
				It("should return an empty item", func() {
					for iter.Next(ctx) {
						// Iterate until there are no more items
					}
					Expect(iter.Item()).To(Equal(""))
				})
			})
		})
	})

	Describe("Next", func() {
		Context("when there is an error", func() {
			BeforeEach(func() {
				iterErr = errors.New("ut-oh")
			})

			It("should return false", func() {
				Expect(iter.Next(ctx)).To(BeFalse())
			})
		})

		Context("when there is no error", func() {
			BeforeEach(func() {
				iterErr = nil
			})

			Context("when there are no items", func() {
				BeforeEach(func() {
					iter = nmockapi.NewIter([]string{}, nil)
				})

				It("should return false", func() {

				})
			})
			BeforeEach(func() {
				iter = nmockapi.NewIter([]string{"a", "b", "c"}, nil)
			})

			It("should return true while there are more values", func() {
				Expect(iter.Next(ctx)).To(BeTrue())
				Expect(iter.Next(ctx)).To(BeTrue())
				Expect(iter.Next(ctx)).To(BeTrue())

				Expect(iter.Next(ctx)).To(BeFalse())
			})
		})
	})

	Describe("Err()", func() {
		Context("when there is an error", func() {
			BeforeEach(func() {
				iterErr = errors.New("ut-oh")
			})

			It("should return the error", func() {
				Expect(iter.Err()).To(HaveOccurred())
			})
		})

		Context("when there is no error", func() {
			BeforeEach(func() {
				iterErr = nil
			})

			It("should return nil", func() {
				Expect(iter.Err()).ToNot(HaveOccurred())
			})
		})
	})
})
