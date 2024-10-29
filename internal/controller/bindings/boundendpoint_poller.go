package bindings

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	v6 "github.com/ngrok/ngrok-api-go/v6"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PortRangeConfig is a configuration for a port range
// Note: PortRange is inclusive: `[Min, Max]`
type PortRangeConfig struct {
	// Start is the minimum port number
	Min uint16

	// Max is the maximum port number
	Max uint16
}

// BoundEndpointPoller is a process to poll the ngrok API for binding_endpoints and reconcile the desired state with the cluster state of BoundEndpoints
type BoundEndpointPoller struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder

	// Namespace is the namespace to manage for BoundEndpoints
	Namespace string

	// KubernetesOperatorConfigName is the expected name of the KubernetesOperator that we should poll
	KubernetesOperatorConfigName string

	// PollingInterval is how often to poll the ngrok API for reconciling the BindingEndpoints
	PollingInterval time.Duration

	// PortRange is the allocatable port range for the Service definitions to Pod Forwarders
	PortRange PortRangeConfig

	// portAllocator manages the unique port allocations
	portAllocator *portBitmap

	// Channel to stop the API polling goroutine
	stopCh chan struct{}

	// reconcilingCancel is the active context's cancel function that is managing the reconciling goroutines
	// this context should be canceled and recreated during each reconcile loop
	reconcilingCancel context.CancelFunc

	// koId is the KubernetesOperator ID from the ngrok API
	koId string
}

// Start implements the manager.Runnable interface.
func (r *BoundEndpointPoller) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	// retrieve k8sop ID
	r.koId = r.getKubernetesOperatorId(ctx)

	log.Info("Starting the BindingConfiguration polling routine")
	r.stopCh = make(chan struct{})
	defer close(r.stopCh)

	// background polling
	go r.startPollingAPI(ctx)

	// handle cancellations
	<-ctx.Done()
	log.Info("Stopping the BindingConfiguration polling routine")
	return nil
}

// getKubernetesOperatorId waits to retrieve the k8sop ID from the KubernetesOperator resource, post-registration
func (r *BoundEndpointPoller) getKubernetesOperatorId(ctx context.Context) string {
	log := ctrl.LoggerFrom(ctx)

	log.V(1).Info("Waiting for KubernetesOperator to be registered and ID returned")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var ko ngrokv1alpha1.KubernetesOperator
			err := r.Client.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: r.KubernetesOperatorConfigName}, &ko)
			if err != nil {
				log.Error(err, "Failed to get KubernetesOperator", "name", r.KubernetesOperatorConfigName)
				continue
			}

			if ko.Status.RegistrationStatus != ngrokv1alpha1.KubernetesOperatorRegistrationStatusSuccess {
				log.V(1).Info("KubernetesOperator not yet registered, waiting...")
				continue
			}

			if ko.Status.ID == "" {
				log.V(1).Info("KubernetesOperator registered with missing ID, waiting...")
				continue
			}

			log.Info("KubernetesOperator registered successfully", "id", ko.Status.ID)
			return ko.Status.ID
		case <-ctx.Done():
			log.Info("Context canceled, stopping KubernetesOperator ID retrieval")
			return ""
		}
	}
}

// startPollingAPI polls a mock API every over a polling interval and updates the BindingConfiguration's status.
func (r *BoundEndpointPoller) startPollingAPI(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)

	ticker := time.NewTicker(r.PollingInterval)
	defer ticker.Stop()

	// Reconcile on startup
	if err := r.reconcileBoundEndpointsFromAPI(ctx); err != nil {
		log.Error(err, "Failed to update binding_endpoints from API")
	}

	for {
		select {
		case <-ticker.C:
			log.Info("Polling API for binding_endpoints")
			if err := r.reconcileBoundEndpointsFromAPI(ctx); err != nil {
				log.Error(err, "Failed to update binding_endpoints from API")
			}
		case <-r.stopCh:
			log.Info("Stopping API polling")
			return
		}
	}
}

// reconcileBoundEndpointsFromAPI fetches the desired binding_endpoints for this kubernetes operator binding
// then creates, updates, or deletes the BoundEndpoints in-cluster
func (r *BoundEndpointPoller) reconcileBoundEndpointsFromAPI(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	if r.reconcilingCancel != nil {
		r.reconcilingCancel() // cancel the previous reconcile loop
	}

	// Fetch the mock endpoint data from the API
	resp, err := fetchEndpoints()
	if err != nil {
		log.Error(err, "Failed to fetch binding_endpoints from API")
		return err
	}

	var apiBindingEndpoints []v6.Endpoint
	if resp.BindingEndpoints == nil {
		apiBindingEndpoints = []v6.Endpoint{} // empty
	} else {
		apiBindingEndpoints = resp.BindingEndpoints.Endpoints
	}

	desiredBoundEndpoints, err := ngrokapi.AggregateBindingEndpoints(apiBindingEndpoints)
	if err != nil {
		return err
	}

	// Get all current BoundEndpoint resources in the cluster.
	var epbList bindingsv1alpha1.BoundEndpointList
	if err := r.List(ctx, &epbList); err != nil {
		return err
	}
	existingBoundEndpoints := epbList.Items

	// since we have the existing BoundEndpoints and their Ports
	// let's use this opportunity to refresh the port allocater's state
	// NOTE: This range must stay static using this implementation.
	currentPortAllocations := newPortBitmap(r.PortRange.Min, r.PortRange.Max)
	for _, existingBoundEndpoint := range existingBoundEndpoints {
		if err := currentPortAllocations.Set(existingBoundEndpoint.Spec.Port); err != nil {
			r.Log.Error(err, "Failed to refresh port allocation", "port", existingBoundEndpoint.Spec.Port, "name", existingBoundEndpoint.Name)
			return err
		}
	}

	// reassign port allocations
	r.portAllocator = currentPortAllocations

	toCreate, toUpdate, toDelete := r.filterBoundEndpointActions(ctx, existingBoundEndpoints, desiredBoundEndpoints)

	// create context + errgroup for managing/closing the future goroutine in the reconcile actions loops
	errGroup, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancel(ctx)
	r.reconcilingCancel = cancel

	// launch goroutines to reconcile the BoundEndpoints' actions in the background until the next polling loop

	r.reconcileBoundEndpointAction(ctx, errGroup, toCreate, "create", func(ctx context.Context, binding bindingsv1alpha1.BoundEndpoint) error {
		return r.createBinding(ctx, binding)
	})

	r.reconcileBoundEndpointAction(ctx, errGroup, toUpdate, "update", func(ctx context.Context, binding bindingsv1alpha1.BoundEndpoint) error {
		return r.updateBinding(ctx, binding)
	})

	r.reconcileBoundEndpointAction(ctx, errGroup, toDelete, "delete", func(ctx context.Context, binding bindingsv1alpha1.BoundEndpoint) error {
		return r.deleteBinding(ctx, binding)
	})

	return nil
}

// boundEndpointActionFn reprents an action to take on an BoundEndpoint during reconciliation
type boundEndpointActionFn func(context.Context, bindingsv1alpha1.BoundEndpoint) error

// reconcileBoundEndpointAction runs a goroutine to try and process a list of BoundEndpoints
// for their desired action over and over again until stopChan is closed or receives a value
func (r *BoundEndpointPoller) reconcileBoundEndpointAction(ctx context.Context, errGroup *errgroup.Group, boundEndpoints []bindingsv1alpha1.BoundEndpoint, actionMsg string, action boundEndpointActionFn) {
	log := ctrl.LoggerFrom(ctx)

	errGroup.Go(func() error {
		// attempt reconciliation actions every so often
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// remainingBindings is the list of BoundEndpoints that still need to be actioned upon
		remainingBindings := boundEndpoints

		for {
			select {
			// stop go routine and return, there is a new reconcile poll happening actively
			case <-ctx.Done():
				log.Error(ctx.Err(), "Reconcile context canceled, stopping BoundEndpoint reconcile loop early", "action", actionMsg)
				return nil
			case <-ticker.C:
				log.V(9).Info("Received tick", "action", actionMsg, "remaining", remainingBindings)
				if len(remainingBindings) == 0 {
					return nil // all bindings have been processed
				}

				failedBindings := []bindingsv1alpha1.BoundEndpoint{}

				// process from list
				for _, binding := range remainingBindings {
					if err := action(ctx, binding); err != nil {
						name := hashURI(binding.Spec.EndpointURI)
						log.Error(err, "Failed to reconcile BoundEndpoint", "action", actionMsg, "name", name, "uri", binding.Spec.EndpointURI)
						failedBindings = append(failedBindings, binding)
					}
				}

				// update the remaining list with the failed bindings
				remainingBindings = failedBindings
			}
		}
	})
}

// filterBoundEndpointActions takse 2 sets of existing and desired BoundEndpoints
// and returns 3 lists: toCreate, toUpdate, toDelete
// representing the actions needed to reconcile the existing set with the desired set
func (r *BoundEndpointPoller) filterBoundEndpointActions(ctx context.Context, existingBoundEndpoints []bindingsv1alpha1.BoundEndpoint, desiredEndpoints ngrokapi.AggregatedEndpoints) (toCreate []bindingsv1alpha1.BoundEndpoint, toUpdate []bindingsv1alpha1.BoundEndpoint, toDelete []bindingsv1alpha1.BoundEndpoint) {
	log := ctrl.LoggerFrom(ctx)

	toCreate = []bindingsv1alpha1.BoundEndpoint{}
	toUpdate = []bindingsv1alpha1.BoundEndpoint{}
	toDelete = []bindingsv1alpha1.BoundEndpoint{}

	log.V(9).Info("Filtering BoundEndpoints", "existing", existingBoundEndpoints, "desired", desiredEndpoints)

	for _, existingBoundEndpoint := range existingBoundEndpoints {
		uri := existingBoundEndpoint.Spec.EndpointURI

		if desiredBoundEndpoint, ok := desiredEndpoints[uri]; ok {
			expectedName := hashURI(desiredBoundEndpoint.Spec.EndpointURI)

			// if the names match, then they are the same resource and we can update it
			if existingBoundEndpoint.Name == expectedName {
				// existing endpoint is in our desired set
				// update this BoundEndpoint
				toUpdate = append(toUpdate, desiredBoundEndpoint)
			} else {
				// otherwise, we need a delete + create, rather than an update
				toDelete = append(toDelete, existingBoundEndpoint)
				toCreate = append(toCreate, desiredBoundEndpoint)
			}
		} else {
			// existing endpoint is not in our desired set
			// delete this BoundEndpoint
			toDelete = append(toDelete, existingBoundEndpoint)
		}

		// remove the desired endpoint from the set
		// so we can see which endpoints are net-new
		delete(desiredEndpoints, uri)
	}

	for _, desiredBoundEndpoint := range desiredEndpoints {
		// desired endpoint is not in our existing set
		// create this BoundEndpoint
		toCreate = append(toCreate, desiredBoundEndpoint)
	}

	return toCreate, toUpdate, toDelete
}

// TODO: Metadata
func (r *BoundEndpointPoller) createBinding(ctx context.Context, desired bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx)

	name := hashURI(desired.Spec.EndpointURI)

	// allocate a port
	port, err := r.portAllocator.SetAny()
	if err != nil {
		r.Log.Error(err, "Failed to allocate port for BoundEndpoint", "name", name, "uri", desired.Spec.EndpointURI)
		return err
	}

	toCreate := &bindingsv1alpha1.BoundEndpoint{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bindings.ngrok.com/v1alpha1",
			Kind:       "BoundEndpoint",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURI: desired.Spec.EndpointURI,
			Scheme:      desired.Spec.Scheme,
			Port:        port,
			Target: bindingsv1alpha1.EndpointTarget{
				Protocol:  desired.Spec.Target.Protocol,
				Namespace: desired.Spec.Target.Namespace,
				Service:   desired.Spec.Target.Service,
				Port:      desired.Spec.Target.Port,
			},
		},
	}

	log.Info("Creating new BoundEndpoint", "name", name, "uri", toCreate.Spec.EndpointURI)
	if err := r.Create(ctx, toCreate); err != nil {
		if client.IgnoreAlreadyExists(err) == nil {
			log.Info("BoundEndpoint already exists, skipping create...", "name", name, "uri", toCreate.Spec.EndpointURI)

			if toCreate.Status.HashedName != "" && len(toCreate.Status.Endpoints) > 0 {
				// Status is filled, no need to update
				return nil
			} else {
				// intentionally blonk
				// we want to fall through and fill in the status
				log.Info("BoundEndpoint already exists, but status is empty, filling in status...", "name", name, "uri", toCreate.Spec.EndpointURI, "toCreate", toCreate)

				// refresh the toCreate object with existing data
				if err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: name}, toCreate); err != nil {
					log.Error(err, "Failed to get existing BoundEndpoint, skipping status update...", "name", name, "uri", toCreate.Spec.EndpointURI)
					return nil
				}
			}
		} else {
			log.Error(err, "Failed to create BoundEndpoint", "name", name, "uri", toCreate.Spec.EndpointURI)
			r.Recorder.Event(toCreate, v1.EventTypeWarning, "Created", fmt.Sprintf("Failed to create BoundEndpoint: %v", err))
			return err
		}
	}

	// now fill in the status into the returned resource

	toCreateStatus := bindingsv1alpha1.BoundEndpointStatus{
		HashedName: name,
		Endpoints:  []bindingsv1alpha1.BindingEndpoint{}, // empty for now, will be filled in just below
	}

	// attach the endpoints to the status
	for _, desiredEndpoint := range desired.Status.Endpoints {
		endpoint := desiredEndpoint
		endpoint.Status = bindingsv1alpha1.StatusProvisioning
		endpoint.ErrorCode = ""
		endpoint.ErrorMessage = ""

		toCreateStatus.Endpoints = append(toCreateStatus.Endpoints, endpoint)
	}

	toCreate.Status = toCreateStatus

	if err := r.updateBindingStatus(ctx, toCreate); err != nil {
		return err
	}

	r.Recorder.Event(toCreate, v1.EventTypeNormal, "Created", "BoundEndpoint created successfully")
	return nil
}

func (r *BoundEndpointPoller) updateBinding(ctx context.Context, desired bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx)

	desiredName := hashURI(desired.Spec.EndpointURI)

	var existing bindingsv1alpha1.BoundEndpoint
	err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: desiredName}, &existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// BoundEndpoint doesn't exist, create it on the next polling loop
			log.Info("Unable to find existing BoundEndpoint, skipping update...", "name", desiredName, "uri", desired.Spec.EndpointURI)
			return nil // not an error
		} else {
			// real error
			log.Error(err, "Failed to find existing BoundEndpoint", "name", desiredName, "uri", desired.Spec.EndpointURI)
			return err
		}
	}

	if !boundEndpointNeedsUpdate(existing, desired) {
		log.Info("BoundEndpoint already matches existing state, skipping update...", "name", desiredName, "uri", desired.Spec.EndpointURI)
		return nil
	}

	// found existing endpoint
	// now let's merge them together
	toUpdate := &existing
	toUpdate.Spec.Port = existing.Spec.Port // keep the same port
	toUpdate.Spec.Scheme = desired.Spec.Scheme
	toUpdate.Spec.Target = desired.Spec.Target
	toUpdate.Spec.EndpointURI = desired.Spec.EndpointURI

	log.Info("Updating BoundEndpoint", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
	if err := r.Update(ctx, toUpdate); err != nil {
		log.Error(err, "Failed updating BoundEndpoint", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
		r.Recorder.Event(toUpdate, v1.EventTypeWarning, "Updated", fmt.Sprintf("Failed to update BoundEndpoint: %v", err))
		return err
	}

	// now fill in the status into the returned resource

	toUpdateStatus := bindingsv1alpha1.BoundEndpointStatus{
		HashedName: desiredName,
		Endpoints:  []bindingsv1alpha1.BindingEndpoint{}, // empty for now, will be filled in just below
	}

	// attach the endpoints to the status
	for _, desiredEndpoint := range desired.Status.Endpoints {
		endpoint := desiredEndpoint
		endpoint.Status = bindingsv1alpha1.StatusProvisioning
		endpoint.ErrorCode = ""
		endpoint.ErrorMessage = ""

		toUpdateStatus.Endpoints = append(toUpdateStatus.Endpoints, endpoint)
	}

	toUpdate.Status = toUpdateStatus

	if err := r.updateBindingStatus(ctx, toUpdate); err != nil {
		return err
	}

	r.Recorder.Event(toUpdate, v1.EventTypeNormal, "Updated", "BoundEndpoint updated successfully")
	return nil
}

func (r *BoundEndpointPoller) deleteBinding(ctx context.Context, boundEndpoint bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.Delete(ctx, &boundEndpoint); err != nil {
		log.Error(err, "Failed to delete BoundEndpoint", "name", boundEndpoint.Name, "uri", boundEndpoint.Spec.EndpointURI)
		return err
	} else {
		log.Info("Deleted BoundEndpoint", "name", boundEndpoint.Name, "uri", boundEndpoint.Spec.EndpointURI)

		// unset the port allocation
		r.portAllocator.Unset(boundEndpoint.Spec.Port)
	}

	return nil
}

func (r *BoundEndpointPoller) updateBindingStatus(ctx context.Context, desired *bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx)

	toUpdate := desired
	toUpdate.Status = desired.Status

	if err := r.Status().Update(ctx, toUpdate); err != nil {
		log.Error(err, "Failed to update BoundEndpoint status", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
		return err
	}

	log.Info("Updated BoundEndpoint status", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
	return nil
}

// boundEndpointNeedsUpdate returns true if the data in desired does not match existing, and therefore existing needs updating to match desired
func boundEndpointNeedsUpdate(existing bindingsv1alpha1.BoundEndpoint, desired bindingsv1alpha1.BoundEndpoint) bool {
	hasSpecChanged := existing.Spec.Scheme != desired.Spec.Scheme ||
		!reflect.DeepEqual(existing.Spec.Target, desired.Spec.Target)

	if hasSpecChanged {
		return true
	}

	// compare the list of endpoints in the status
	if len(existing.Status.Endpoints) != len(desired.Status.Endpoints) {
		return true
	}

	existingEndpoints := map[string]bindingsv1alpha1.BindingEndpoint{}
	for _, existingEndpoint := range existing.Status.Endpoints {
		existingEndpoints[existingEndpoint.Ref.ID] = existingEndpoint
	}

	for _, desiredEndpoint := range desired.Status.Endpoints {
		if _, ok := existingEndpoints[desiredEndpoint.Ref.ID]; !ok {
			return true // at least one endpoint has changed
		}
	}

	return false

	// TODO: Compare Metadata labels and annotations between configured values and existing values
}

// hashURI hashes a URI to a unique string that can be used as BoundEndpoint.metadata.name
func hashURI(uri string) string {
	uid := uuid.NewSHA1(uuid.NameSpaceURL, []byte(uri))
	return "ngrok-" + uid.String()
}

// fetchEndpoints mocks an API call to retrieve a list of endpoint bindings.
func fetchEndpoints() (*MockApiResponse, error) {
	// TODO(hkatz): Implement the actual API call to fetch the binding_epndoints
	// Mock response with sample data.
	return &MockApiResponse{
		BindingEndpoints: &v6.EndpointList{
			Endpoints: []v6.Endpoint{
				{ID: "ep_100", PublicURL: "https://service1.namespace1"},
				{ID: "ep_101", PublicURL: "https://service1.namespace1"},
				{ID: "ep_102", PublicURL: "https://service1.namespace1"},
				{ID: "ep_200", PublicURL: "tcp://service2.namespace2:2020"},
				{ID: "ep_201", PublicURL: "tcp://service2.namespace2:2020"},
				{ID: "ep_300", PublicURL: "service3.namespace3"},
				{ID: "ep_400", PublicURL: "http://service4.namespace4:8080"},
				{ID: "ep_500", PublicURL: "http://example-files.example:80"},
			},
		},
	}, nil
}

// MockApiResponse represents a mock response from the API.
type MockApiResponse struct {
	BindingEndpoints *v6.EndpointList
}