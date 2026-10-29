package privateendpoints

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// allocate is pure, so these call it directly instead of going through a
// running manager.
var _ = Describe("PrivateEndpoint port allocation", func() {
	const portMin, portMax = 100, 103

	DescribeTable("allocate",
		func(lastPort int32, used []int32, want int32, wantOK bool, wantLast int32) {
			r := &PrivateEndpointReconciler{PortMin: portMin, PortMax: portMax, lastPort: lastPort}
			usedSet := map[int32]bool{}
			for _, p := range used {
				usedSet[p] = true
			}

			got, ok := r.allocate(usedSet)

			Expect(ok).To(Equal(wantOK))
			Expect(got).To(Equal(want))
			Expect(r.lastPort).To(Equal(wantLast))
		},
		Entry("starts at PortMin when nothing is used", int32(0), nil, int32(100), true, int32(100)),
		Entry("seeds from the highest used port", int32(0), []int32{100, 102}, int32(103), true, int32(103)),
		Entry("ignores used ports outside the range when seeding", int32(0), []int32{99, 200}, int32(100), true, int32(100)),
		Entry("doesn't reuse a freed port below the last one", int32(101), nil, int32(102), true, int32(102)),
		Entry("wraps to PortMin after PortMax", int32(103), nil, int32(100), true, int32(100)),
		Entry("skips used ports after wrapping", int32(103), []int32{100, 101}, int32(102), true, int32(102)),
		Entry("hands out the last port again when it's the only free one", int32(102), []int32{100, 101, 103}, int32(102), true, int32(102)),
		Entry("fails when the range is exhausted", int32(101), []int32{100, 101, 102, 103}, int32(0), false, int32(101)),
	)

	It("advances across calls without reusing a port", func() {
		r := &PrivateEndpointReconciler{PortMin: portMin, PortMax: portMax}
		used := map[int32]bool{}
		var got []int32
		for range portMax - portMin + 1 {
			p, ok := r.allocate(used)
			Expect(ok).To(BeTrue())
			used[p] = true
			got = append(got, p)
		}
		Expect(got).To(Equal([]int32{100, 101, 102, 103}))

		_, ok := r.allocate(used)
		Expect(ok).To(BeFalse())
	})
})
