// LEGACY-PREFIX-MIGRATION: BEGIN (file scope — read-side cleanup deletes this whole file)
// listAgentEndpointsForController / listCloudEndpointsForController exist
// only because the Driver needs to dual-match controller labels under the
// legacy `k8s.ngrok.com/` prefix during the migration window. Once legacy
// labels are gone the two helpers collapse back to a single
//
//	c.List(ctx, &out, d.controllerLabels.Selector())
//
// call at each callsite. In the read-side cleanup delete this file and
// re-inline the single List in driver.go::Sync and endpoints.go::SyncEndpoints.

package managerdriver

import (
	"context"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listAgentEndpointsForController lists AgentEndpoints matching the operator's
// controller labels under both the new (ngrok.com/...) and legacy
// (k8s.ngrok.com/...) prefixes, deduping by UID. The legacy fallback covers
// objects stamped by a previous version of the operator that has not yet
// reconciled them under the new prefix.
func (d *Driver) listAgentEndpointsForController(ctx context.Context, c client.Client) ([]ngrokv1alpha1.AgentEndpoint, error) {
	seen := map[types.UID]struct{}{}
	out := []ngrokv1alpha1.AgentEndpoint{}
	for _, sel := range d.controllerLabels.Selectors() {
		var l ngrokv1alpha1.AgentEndpointList
		if err := c.List(ctx, &l, sel); err != nil {
			return nil, err
		}
		for _, item := range l.Items {
			if _, ok := seen[item.UID]; ok {
				continue
			}
			seen[item.UID] = struct{}{}
			out = append(out, item)
		}
	}
	return out, nil
}

// listCloudEndpointsForController is the CloudEndpoint counterpart to
// listAgentEndpointsForController.
func (d *Driver) listCloudEndpointsForController(ctx context.Context, c client.Client) ([]ngrokv1alpha1.CloudEndpoint, error) {
	seen := map[types.UID]struct{}{}
	out := []ngrokv1alpha1.CloudEndpoint{}
	for _, sel := range d.controllerLabels.Selectors() {
		var l ngrokv1alpha1.CloudEndpointList
		if err := c.List(ctx, &l, sel); err != nil {
			return nil, err
		}
		for _, item := range l.Items {
			if _, ok := seen[item.UID]; ok {
				continue
			}
			seen[item.UID] = struct{}{}
			out = append(out, item)
		}
	}
	return out, nil
}

// LEGACY-PREFIX-MIGRATION: END
