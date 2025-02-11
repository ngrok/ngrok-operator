package managerdriver

import (
	"context"
	"reflect"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (d *Driver) SyncEndpoints(ctx context.Context, c client.Client) error {
	if !d.syncAllowConcurrent {
		if proceed, wait := d.syncStart(true); proceed {
			defer d.syncDone()
		} else {
			return wait(ctx)
		}
	}

	d.log.Info("syncing cloud and agent endpoints state!!")
	translator := NewTranslator(
		d.log,
		d.store,
		d.defaultManagedResourceLabels(),
		d.ingressNgrokMetadata,
		d.gatewayNgrokMetadata,
		d.clusterDomain,
	)
	translationResult := translator.Translate()

	currentAgentEndpoints := &ngrokv1alpha1.AgentEndpointList{}
	currentCloudEndpoints := &ngrokv1alpha1.CloudEndpointList{}

	if err := c.List(ctx, currentAgentEndpoints, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing agent endpoints")
		return err
	}

	if err := c.List(ctx, currentCloudEndpoints, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing cloud endpoints")
		return err
	}

	if err := d.applyAgentEndpoints(ctx, c, translationResult.AgentEndpoints, currentAgentEndpoints.Items); err != nil {
		d.log.Error(err, "applying agent endpoints")
		return err
	}
	if err := d.applyCloudEndpoints(ctx, c, translationResult.CloudEndpoints, currentCloudEndpoints.Items); err != nil {
		d.log.Error(err, "applying cloud endpoints")
		return err
	}

	return nil
}

func (d *Driver) applyAgentEndpoints(ctx context.Context, c client.Client, desired map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint, current []ngrokv1alpha1.AgentEndpoint) error {
	// update or delete agent endpoints we don't need anymore
	for _, currAEP := range current {

		// If this AgentEndpoint is created by the user and not owned/managed by the operator then ignore it
		if !hasDefaultManagedResourceLabels(currAEP.Labels, d.managerName.Name, d.managerName.Namespace) {
			continue
		}

		objectKey := types.NamespacedName{
			Name:      currAEP.Name,
			Namespace: currAEP.Namespace,
		}
		if desiredAEP, exists := desired[objectKey]; exists {
			needsUpdate := false

			if !reflect.DeepEqual(desiredAEP.Spec, currAEP.Spec) {
				currAEP.Spec = desiredAEP.Spec
				needsUpdate = true
			}

			if !reflect.DeepEqual(desiredAEP.Labels, currAEP.Labels) {
				currAEP.Labels = desiredAEP.Labels
				needsUpdate = true
			}
			if !reflect.DeepEqual(desiredAEP.Annotations, currAEP.Annotations) {
				currAEP.Annotations = desiredAEP.Annotations
				needsUpdate = true
			}

			if needsUpdate {
				if err := c.Update(ctx, &currAEP); err != nil {
					d.log.Error(err, "error updating agent endpoint", "desired", desiredAEP, "current", currAEP)
					return err
				}
			}

			// matched and updated the agent endpoint, no longer desired
			delete(desired, objectKey)
		} else {
			if err := c.Delete(ctx, &currAEP); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting agent endpoint", "current agent endpoints", currAEP)
				return err
			}
		}
	}

	// the set of desired agent endpoints now only contains new agent endpoints, create them
	for _, agentEndpoint := range desired {
		if err := c.Create(ctx, agentEndpoint); err != nil {
			d.log.Error(err, "error creating agent endpoint", "agent endpoint", agentEndpoint)
			return err
		}
	}

	return nil
}

func (d *Driver) applyCloudEndpoints(ctx context.Context, c client.Client, desired map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, current []ngrokv1alpha1.CloudEndpoint) error {
	// update or delete cloud endpoints we don't need anymore
	for _, currCLEP := range current {

		// If this CloudEndpoint is created by the user and not owned/managed by the operator then ignore it
		if !hasDefaultManagedResourceLabels(currCLEP.Labels, d.managerName.Name, d.managerName.Namespace) {
			continue
		}

		objectKey := types.NamespacedName{
			Name:      currCLEP.Name,
			Namespace: currCLEP.Namespace,
		}
		if desiredCLEP, exists := desired[objectKey]; exists {
			needsUpdate := false

			// Copy the ID in the status field from the existing cloud endpoint to the desired one.
			// The ID is set by controller in the operator-agent pod and so we don't want the controller from the
			// operator-manager pod (which this code runs in) to erase it
			desiredCLEP.Status.ID = currCLEP.Status.ID
			if !reflect.DeepEqual(desiredCLEP.Spec, currCLEP.Spec) {
				currCLEP.Spec = desiredCLEP.Spec
				needsUpdate = true
			}

			if !reflect.DeepEqual(desiredCLEP.Labels, currCLEP.Labels) {
				currCLEP.Labels = desiredCLEP.Labels
				needsUpdate = true
			}
			if !reflect.DeepEqual(desiredCLEP.Annotations, currCLEP.Annotations) {
				currCLEP.Annotations = desiredCLEP.Annotations
				needsUpdate = true
			}

			if needsUpdate {
				if err := c.Update(ctx, &currCLEP); err != nil {
					d.log.Error(err, "error updating cloud endpoint", "desired", desiredCLEP, "current", currCLEP)
					return err
				}
			}

			// matched and updated the cloud endpoint, no longer desired
			delete(desired, objectKey)
		} else {
			if err := c.Delete(ctx, &currCLEP); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting cloud endpoint", "cloud endpoint", currCLEP)
				return err
			}
		}
	}

	// the set of desired cloud endpoints now only contains new cloud endpoints, create them
	for _, cloudEndpoint := range desired {
		if err := c.Create(ctx, cloudEndpoint); err != nil {
			d.log.Error(err, "error creating cloud endpoint", "cloud endpoint", cloudEndpoint)
			return err
		}
	}

	return nil
}
