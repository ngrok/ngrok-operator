package forwarder

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestForwarder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Forwarder Controller Suite")
}

func TestGetIngressEndpointWithFallback(t *testing.T) {
	cases := []struct {
		input            string
		expectedEndpoint string
		shouldErr        bool
	}{
		{
			"",
			"",
			true,
		},
		{
			"foo.example.com",
			"foo.example.com:443",
			false,
		},
		{
			"foo.example.com:443",
			"foo.example.com:443",
			false,
		},
		{
			"foo.example.com:443:1234",
			"",
			true,
		},
	}

	for _, c := range cases {
		ingressEndpoint, err := getIngressEndpointWithFallback(c.input, logr.Discard())
		assert.Equal(t, c.expectedEndpoint, ingressEndpoint)
		if c.shouldErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

var _ = Describe("podIdentityFromPod", func() {
	var (
		pod *v1.Pod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				UID:         "uid123",
				Name:        "pod1",
				Namespace:   "default",
				Annotations: map[string]string{},
			},
		}
	)

	It("prunes non-prefixed annotations and returns PodIdentity", func() {
		pod.Annotations = map[string]string{
			"k8s.ngrok.com/keep": "yes",
			"some.other/strip":   "no",
		}

		pid := podIdentityFromPod(pod)
		Expect(pid).To(Not(BeNil()))
		Expect(pid.Uid).To(Equal("uid123"))
		Expect(pid.Name).To(Equal("pod1"))
		Expect(pid.Namespace).To(Equal("default"))
		Expect(pid.Annotations).To(Not(BeNil()))
		Expect(pid.Annotations).To(HaveKey("k8s.ngrok.com/keep"))
		Expect(pid.Annotations).To(Not(HaveKey("some.other/strip")))
	})
})
