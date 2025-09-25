package service

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func newTestService(isLoadBalancer bool, isOurLoadBalancerClass bool, annotations map[string]string) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-service",
			Namespace:   "test-namespace",
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "tcp",
					Protocol: corev1.ProtocolTCP,
					Port:     80,
				},
			},
		},
	}
	if isLoadBalancer {
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	}
	if isOurLoadBalancerClass {
		svc.Spec.LoadBalancerClass = ptr.To(NgrokLoadBalancerClass)
	} else {
		svc.Spec.LoadBalancerClass = ptr.To("not-ngrok")
	}

	return svc
}

var _ = Describe("ServiceController", func() {
	DescribeTable("shouldHandleService", func(svc *corev1.Service, expected bool) {
		Expect(shouldHandleService(svc)).To(Equal(expected))
	},
		Entry("Non-LoadBalancer service", newTestService(false, false, nil), false),
		Entry("LoadBalancer service, but not our class", newTestService(true, false, nil), false),
		Entry("LoadBalancer service, our class, but no annotations", newTestService(true, true, nil), true),
	)
})
