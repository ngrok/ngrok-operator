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
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EndpointBindingPoller is a process to poll the ngrok API for binding_endpoints and reconcile the desired state with the cluster state of EndpointBindings
type EndpointBindingPoller struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder

	// Namespace is the namespace to manage for EndpointBindings
	Namespace string

	// PollingInterval is how often to poll the ngrok API for reconciling the BindingEndpoints
	PollingInterval time.Duration

	// Channel to stop the API polling goroutine
	stopCh chan struct{}
}

// Start implements the manager.Runnable interface.
func (r *EndpointBindingPoller) Start(ctx context.Context) error {
	r.Log.Info("Starting the BindingConfiguration polling routine")
	r.stopCh = make(chan struct{})
	defer close(r.stopCh)
	go r.startPollingAPI(ctx)
	<-ctx.Done()
	r.Log.Info("Stopping the BindingConfiguration polling routine")
	return nil
}

// startPollingAPI polls a mock API every over a polling interval and updates the BindingConfiguration's status.
func (r *EndpointBindingPoller) startPollingAPI(ctx context.Context) {
	ticker := time.NewTicker(r.PollingInterval)
	defer ticker.Stop()

	// Reconcile on startup
	if err := r.reconcileEndpointBindingsFromAPI(ctx); err != nil {
		r.Log.Error(err, "Failed to update binding_endpoints from API")
	}

	for {
		select {
		case <-ticker.C:
			r.Log.Info("Polling API for binding_endpoints")
			if err := r.reconcileEndpointBindingsFromAPI(ctx); err != nil {
				r.Log.Error(err, "Failed to update binding_endpoints from API")
			}
		case <-r.stopCh:
			r.Log.Info("Stopping API polling")
			return
		}
	}
}

// reconcileEndpointBindingsFromAPI fetches the desired binding_endpoints for this kubernetes operator binding
// then creates, updates, or deletes the EndpointBindings in-cluster
func (r *EndpointBindingPoller) reconcileEndpointBindingsFromAPI(ctx context.Context) error {
	// Fetch the mock endpoint data from the API
	resp, err := fetchEndpoints()
	if err != nil {
		return err
	}

	var apiBindingEndpoints []v6.Endpoint
	if resp.BindingEndpoints == nil {
		apiBindingEndpoints = []v6.Endpoint{} // empty
	} else {
		apiBindingEndpoints = resp.BindingEndpoints.Endpoints
	}

	desiredEndpointBindings, err := ngrokapi.AggregateBindingEndpoints(apiBindingEndpoints)
	if err != nil {
		return err
	}

	// Get all current EndpointBinding resources in the cluster.
	var epbList bindingsv1alpha1.EndpointBindingList
	if err := r.List(ctx, &epbList); err != nil {
		return err
	}
	existingEndpointBindings := epbList.Items

	toCreate, toUpdate, toDelete := filterEndpointBindingActions(existingEndpointBindings, desiredEndpointBindings)

	for _, binding := range toCreate {
		if err := r.createBinding(ctx, binding); err != nil {
			r.Log.Error(err, "Failed to create EndpointBinding", "name", binding.Name, "uri", binding.Spec.EndpointURI)
			// continue on purpose
		}
	}

	for _, binding := range toUpdate {
		if err := r.updateBinding(ctx, binding); err != nil {
			r.Log.Error(err, "Failed to update EndpointBinding", "name", binding.Name, "uri", binding.Spec.EndpointURI)
			// continue on purpose
		}
	}

	for _, binding := range toDelete {
		// best effort
		r.deleteBinding(ctx, binding)
	}

	return nil
}

// filterEndpointBindingActions takse 2 sets of existing and desired EndpointBindings
// and returns 3 lists: toCreate, toUpdate, toDelete
// representing the actions needed to reconcile the existing set with the desired set
func filterEndpointBindingActions(existingEndpointBindings []bindingsv1alpha1.EndpointBinding, desiredEndpoints ngrokapi.AggregatedEndpoints) (toCreate []bindingsv1alpha1.EndpointBinding, toUpdate []bindingsv1alpha1.EndpointBinding, toDelete []bindingsv1alpha1.EndpointBinding) {
	toCreate = []bindingsv1alpha1.EndpointBinding{}
	toUpdate = []bindingsv1alpha1.EndpointBinding{}
	toDelete = []bindingsv1alpha1.EndpointBinding{}

	for _, endpointBinding := range existingEndpointBindings {
		uri := endpointBinding.Spec.EndpointURI

		if desiredEndpointBinding, ok := desiredEndpoints[uri]; ok {
			// if the names match, then they are the same resource and we can update it
			if endpointBinding.Name == desiredEndpointBinding.Name {
				// existing endpoint is in our desired set
				// update this EndpointBinding
				toUpdate = append(toUpdate, desiredEndpointBinding)
			} else {
				// otherwise, we need a delete + create, rather than an update
				toDelete = append(toDelete, endpointBinding)
				toCreate = append(toCreate, desiredEndpointBinding)
			}
		} else {
			// existing endpoint is not in our desired set
			// delete this EndpointBinding
			toDelete = append(toDelete, endpointBinding)
		}

		// remove the desired endpoint from the set
		// so we can see which endpoints are net-new
		delete(desiredEndpoints, uri)
	}

	for _, desiredEndpointBinding := range desiredEndpoints {
		// desired endpoint is not in our existing set
		// create this EndpointBinding
		toCreate = append(toCreate, desiredEndpointBinding)
	}

	return toCreate, toUpdate, toDelete
}

// TODO: Port
// TODO: Metadata
func (r *EndpointBindingPoller) createBinding(ctx context.Context, desired bindingsv1alpha1.EndpointBinding) error {
	name := hashURI(desired.Spec.EndpointURI)

	toCreate := &bindingsv1alpha1.EndpointBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bindings.ngrok.com/v1alpha1",
			Kind:       "EndpointBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
		},
		Spec: bindingsv1alpha1.EndpointBindingSpec{
			EndpointURI: desired.Spec.EndpointURI,
			Scheme:      desired.Spec.Scheme,
			Port:        1111,
			Target: bindingsv1alpha1.EndpointTarget{
				Protocol:  desired.Spec.Target.Protocol,
				Namespace: desired.Spec.Target.Namespace,
				Service:   desired.Spec.Target.Service,
				Port:      desired.Spec.Target.Port,
			},
		},
	}

	r.Log.Info("Creating new EndpointBinding", "name", name, "uri", toCreate.Spec.EndpointURI)
	if err := r.Create(ctx, toCreate); err != nil {
		r.Log.Error(err, "Failed to create EndpointBinding", "name", name, "uri", toCreate.Spec.EndpointURI)
		r.Recorder.Event(toCreate, v1.EventTypeWarning, "Created", fmt.Sprintf("Failed to create EndpointBinding: %v", err))
		return err
	}

	// now fill in the status into the returned resource

	toCreateStatus := bindingsv1alpha1.EndpointBindingStatus{
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

	if err := r.updateBindingStatus(ctx, *toCreate); err != nil {
		return err
	}

	r.Recorder.Event(toCreate, v1.EventTypeNormal, "Created", "EndpointBinding created successfully")
	return nil
}

func (r *EndpointBindingPoller) updateBinding(ctx context.Context, desired bindingsv1alpha1.EndpointBinding) error {
	desiredName := hashURI(desired.Spec.EndpointURI)

	var existing bindingsv1alpha1.EndpointBinding
	err := r.Get(ctx, client.ObjectKey{Namespace: r.Namespace, Name: desiredName}, &existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// EndpointBinding doesn't exist, create it on the next polling loop
			r.Log.Info("Unable to find existing EndpointBinding, skipping update...", "name", desired.Name, "uri", desired.Spec.EndpointURI)
			return nil // not an error
		} else {
			// real error
			r.Log.Error(err, "Failed to find existing EndpointBinding", "name", desired.Name, "uri", desired.Spec.EndpointURI)
			return err
		}
	}

	if !endpointBindingNeedsUpdate(existing, desired) {
		r.Log.Info("EndpointBinding already matches existing state, skipping update...", "name", desired.Name, "uri", desired.Spec.EndpointURI)
		return nil
	}

	// found existing endpoint
	// now let's merge them together
	toUpdate := &existing
	toUpdate.Spec.Scheme = desired.Spec.Scheme
	toUpdate.Spec.Target = desired.Spec.Target
	toUpdate.Spec.EndpointURI = desired.Spec.EndpointURI

	r.Log.Info("Updating EndpointBinding", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
	if err := r.Update(ctx, toUpdate); err != nil {
		r.Log.Error(err, "Failed updating EndpointBinding", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
		r.Recorder.Event(toUpdate, v1.EventTypeWarning, "Updated", fmt.Sprintf("Failed to update EndpointBinding: %v", err))
		return err
	}

	// now fill in the status into the returned resource

	toUpdateStatus := bindingsv1alpha1.EndpointBindingStatus{
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

	if err := r.updateBindingStatus(ctx, *toUpdate); err != nil {
		return err
	}

	r.Recorder.Event(toUpdate, v1.EventTypeNormal, "Updated", "EndpointBinding updated successfully")
	return nil
}

func (r *EndpointBindingPoller) deleteBinding(ctx context.Context, endpointBinding bindingsv1alpha1.EndpointBinding) {
	if err := r.Delete(ctx, &endpointBinding); err != nil {
		r.Log.Error(err, "Failed to delete EndpointBinding", "name", endpointBinding.Name, "uri", endpointBinding.Spec.EndpointURI)
	} else {
		r.Log.Info("Deleted EndpointBinding", "name", endpointBinding.Name, "uri", endpointBinding.Spec.EndpointURI)
	}
}

func (r *EndpointBindingPoller) updateBindingStatus(ctx context.Context, desired bindingsv1alpha1.EndpointBinding) error {
	toUpdate := &desired
	toUpdate.Status = desired.Status

	if err := r.Status().Update(ctx, toUpdate); err != nil {
		r.Log.Error(err, "Failed to update EndpointBinding status", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
		return err
	}

	r.Log.Info("Updated EndpointBinding status", "name", toUpdate.Name, "uri", toUpdate.Spec.EndpointURI)
	return nil
}

// endpointBindingNeedsUpdate returns true if the data in desired does not match existing, and therefore existing needs updating to match desired
func endpointBindingNeedsUpdate(existing bindingsv1alpha1.EndpointBinding, desired bindingsv1alpha1.EndpointBinding) bool {
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

// hashURI hashes a URI to a unique string that can be used as EndpointBinding.metadata.name
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
			},
		},
	}, nil
}

// MockApiResponse represents a mock response from the API.
type MockApiResponse struct {
	BindingEndpoints *v6.EndpointList
}
