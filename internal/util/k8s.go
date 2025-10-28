package util

import (
	"context"
	"fmt"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// ToClientObjects converts a slice of objects whose pointer implements client.Object
// to a slice of client.Objects
func ToClientObjects[T any, PT interface {
	*T
	client.Object
}](s []T) []client.Object {
	objs := make([]client.Object, len(s))
	for i, obj := range s {
		var p PT = &obj
		objs[i] = p
	}
	return objs
}

// ObjectsToName converts a client.Object to its name
func ObjToName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return obj.GetName()
}

// ObjToKind converts a client.Object to its kind
func ObjToKind(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.Kind
}

// ObjToGroupVersionKind converts a client.Object to its GVK
func ObjToGVK(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.String()
}

// ObjToHumanName converts a client.Object to a human-readable name
func ObjToHumanName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.Kind + "/" + obj.GetName()
}

// ObjToHumanGvkName converts a client.Object to a human-readable full name including GroupVersionKind
func ObjToHumanGvkName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.String() + " Name=" + obj.GetName()
}

func ListObjectsForType(ctx context.Context, client client.Reader, v any) ([]client.Object, error) {
	switch v.(type) {

	// ----------------------------------------------------------------------------
	// Kubernetes Core API Support
	// ----------------------------------------------------------------------------
	case *corev1.Service:
		services := &corev1.ServiceList{}
		err := client.List(ctx, services)
		return ToClientObjects(services.Items), err
	case *corev1.Secret:
		secrets := &corev1.SecretList{}
		err := client.List(ctx, secrets)
		return ToClientObjects(secrets.Items), err
	case *corev1.ConfigMap:
		configmaps := &corev1.ConfigMapList{}
		err := client.List(ctx, configmaps)
		return ToClientObjects(configmaps.Items), err
	case *corev1.Namespace:
		namespaces := &corev1.NamespaceList{}
		err := client.List(ctx, namespaces)
		return ToClientObjects(namespaces.Items), err
	case *netv1.Ingress:
		ingresses := &netv1.IngressList{}
		err := client.List(ctx, ingresses)
		return ToClientObjects(ingresses.Items), err
	case *netv1.IngressClass:
		ingressClasses := &netv1.IngressClassList{}
		err := client.List(ctx, ingressClasses)
		return ToClientObjects(ingressClasses.Items), err

	// ----------------------------------------------------------------------------
	// Kubernetes Gateway API Support
	// ----------------------------------------------------------------------------
	case *gatewayv1.GatewayClass:
		gatewayClasses := &gatewayv1.GatewayClassList{}
		err := client.List(ctx, gatewayClasses)
		return ToClientObjects(gatewayClasses.Items), err
	case *gatewayv1.Gateway:
		gateways := &gatewayv1.GatewayList{}
		err := client.List(ctx, gateways)
		return ToClientObjects(gateways.Items), err
	case *gatewayv1.HTTPRoute:
		httproutes := &gatewayv1.HTTPRouteList{}
		err := client.List(ctx, httproutes)
		return ToClientObjects(httproutes.Items), err
	case *gatewayv1alpha2.TCPRoute:
		tcpRoutes := &gatewayv1alpha2.TCPRouteList{}
		err := client.List(ctx, tcpRoutes)
		return ToClientObjects(tcpRoutes.Items), err
	case *gatewayv1alpha2.TLSRoute:
		tlsRoutes := &gatewayv1alpha2.TLSRouteList{}
		err := client.List(ctx, tlsRoutes)
		return ToClientObjects(tlsRoutes.Items), err
	case *gatewayv1beta1.ReferenceGrant:
		referenceGrants := &gatewayv1beta1.ReferenceGrantList{}
		err := client.List(ctx, referenceGrants)
		return ToClientObjects(referenceGrants.Items), err

	// ----------------------------------------------------------------------------
	// Ngrok API Support
	// ----------------------------------------------------------------------------
	case *ingressv1alpha1.Domain:
		domains := &ingressv1alpha1.DomainList{}
		err := client.List(ctx, domains)
		return ToClientObjects(domains.Items), err
	case *ingressv1alpha1.IPPolicy:
		ipPolicies := &ingressv1alpha1.IPPolicyList{}
		err := client.List(ctx, ipPolicies)
		return ToClientObjects(ipPolicies.Items), err
	case *ngrokv1alpha1.AgentEndpoint:
		agentEndpoints := &ngrokv1alpha1.AgentEndpointList{}
		err := client.List(ctx, agentEndpoints)
		return ToClientObjects(agentEndpoints.Items), err
	case *ngrokv1alpha1.CloudEndpoint:
		cloudEndpoints := &ngrokv1alpha1.CloudEndpointList{}
		err := client.List(ctx, cloudEndpoints)
		return ToClientObjects(cloudEndpoints.Items), err
	case *ngrokv1alpha1.KubernetesOperator:
		operators := &ngrokv1alpha1.KubernetesOperatorList{}
		err := client.List(ctx, operators)
		return ToClientObjects(operators.Items), err
	case *ngrokv1alpha1.NgrokTrafficPolicy:
		policies := &ngrokv1alpha1.NgrokTrafficPolicyList{}
		err := client.List(ctx, policies)
		return ToClientObjects(policies.Items), err
	}
	return nil, fmt.Errorf("unsupported type %T", v)
}
