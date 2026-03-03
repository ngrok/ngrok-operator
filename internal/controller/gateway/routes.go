package gateway

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Returns true if the parentRef is a reference to a Gateway. If the parentRef does not specify a Group or Kind, it is assumed to be a reference to a Gateway.
func parentRefIsGateway(parentRef gatewayv1.ParentReference) bool {
	group := ptr.Deref(parentRef.Group, gatewayv1.GroupName)
	kind := ptr.Deref(parentRef.Kind, gatewayv1.Kind("Gateway"))
	return group == gatewayv1.GroupName && kind == "Gateway"
}

// routeReferencesNgrokGateway returns true if at least one parentRef targets a
// Gateway whose GatewayClass is managed by this controller.
func routeReferencesNgrokGateway(ctx context.Context, c client.Client, namespace string, parentRefs []gatewayv1.ParentReference) (bool, error) {
	for _, parentRef := range parentRefs {
		if !parentRefIsGateway(parentRef) {
			continue
		}

		ns := string(ptr.Deref(parentRef.Namespace, gatewayv1.Namespace(namespace)))
		gw := &gatewayv1.Gateway{}
		if err := c.Get(ctx, types.NamespacedName{Name: string(parentRef.Name), Namespace: ns}, gw); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return false, err
			}
			continue // gateway not found, cannot confirm ownership
		}

		gwc := &gatewayv1.GatewayClass{}
		if err := c.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwc); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return false, err
			}
			continue
		}

		if ShouldHandleGatewayClass(gwc) {
			return true, nil
		}
	}
	return false, nil
}
