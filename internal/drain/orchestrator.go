/*
MIT License

Copyright (c) 2025 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package drain

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Outcome represents the result of a drain operation
type Outcome int

const (
	// OutcomeRetry indicates drain had transient errors and should be retried
	OutcomeRetry Outcome = iota
	// OutcomeComplete indicates drain finished successfully
	OutcomeComplete
	// OutcomeFailed indicates drain encountered a fatal error
	OutcomeFailed
)

// OrchestratorConfig contains the configuration for creating an Orchestrator
type OrchestratorConfig struct {
	// Client is the Kubernetes client (should be the manager's cached client, which
	// respects cache.Options.DefaultNamespaces if configured)
	Client client.Client
	// Recorder is the event recorder for drain-related events
	Recorder record.EventRecorder
	// Log is the logger for drain operations
	Log logr.Logger
	// K8sOpNamespace is the operator's namespace (used for StateChecker)
	K8sOpNamespace string
	// K8sOpName is the KubernetesOperator CR name (used for StateChecker)
	K8sOpName string
}

// Orchestrator manages the complete drain workflow including state checking,
// status updates, and resource cleanup. It provides a clean separation between
// the drain workflow and the KubernetesOperator lifecycle management.
type Orchestrator struct {
	client       client.Client
	recorder     record.EventRecorder
	log          logr.Logger
	stateChecker *StateChecker
}

// NewOrchestrator creates a new Orchestrator with the given configuration.
func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	return &Orchestrator{
		client:       cfg.Client,
		recorder:     cfg.Recorder,
		log:          cfg.Log,
		stateChecker: NewStateChecker(cfg.Client, cfg.K8sOpNamespace, cfg.K8sOpName),
	}
}

// State returns the read-only drain state interface for other controllers to check
// if the operator is draining. This should be passed to all controllers that need
// to skip non-delete reconciles during drain.
func (o *Orchestrator) State() State {
	return o.stateChecker
}

// HandleDrain performs the complete drain workflow for the given KubernetesOperator.
// It handles:
// - Setting the in-memory drain flag to notify other controllers
// - Updating the KO status to reflect drain progress
// - Running the drainer to remove finalizers from all managed resources
// - Recording events for observability
//
// The caller (KubernetesOperatorReconciler) is responsible for:
// - Detecting when drain should start (KO deletion or status)
// - Removing the KO finalizer after drain completes
// - Deleting the KubernetesOperator from the ngrok API
func (o *Orchestrator) HandleDrain(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) (Outcome, error) {
	log := o.log.WithValues("namespace", ko.Namespace, "name", ko.Name)

	// Set in-memory flag for fast propagation to other controllers in this pod.
	// The controller already set status.drainStatus which handles cross-pod visibility.
	o.stateChecker.SetDraining()

	// If drain already completed, don't re-run the drainer
	if ko.Status.DrainStatus == ngrokv1alpha1.DrainStatusCompleted {
		log.V(1).Info("Drain already completed, skipping")
		return OutcomeComplete, nil
	}

	log.Info("Drain started, sleeping to allow other controllers to observe drain state")
	time.Sleep(2 * time.Second)

	log.Info("Running drain process")
	o.recorder.Event(ko, v1.EventTypeNormal, "DrainStarted", "Starting drain of all managed resources")

	// Create and run the drainer
	drainer := &Drainer{
		Client: o.client,
		Log:    log,
		Policy: ko.GetDrainPolicy(),
	}

	result, err := drainer.DrainAll(ctx)
	if err != nil {
		ko.Status.DrainStatus = ngrokv1alpha1.DrainStatusFailed
		ko.Status.DrainMessage = fmt.Sprintf("Drain failed: %v", err)
		ko.Status.DrainProgress = result.Progress()
		if statusErr := o.client.Status().Update(ctx, ko); statusErr != nil {
			log.Error(statusErr, "Failed to update drain status after error")
		}
		o.recorder.Event(ko, v1.EventTypeWarning, "DrainFailed", fmt.Sprintf("Drain failed: %v", err))
		return OutcomeFailed, err
	}

	// Update progress
	ko.Status.DrainProgress = result.Progress()
	ko.Status.DrainErrors = result.ErrorStrings()

	// If there were transient errors (e.g., conflict updating a resource), retry
	if result.HasErrors() {
		ko.Status.DrainMessage = fmt.Sprintf("Drain encountered %d errors, will retry", result.Failed)
		for _, drainErr := range result.Errors {
			log.Error(drainErr, "Drain error")
		}
		if statusErr := o.client.Status().Update(ctx, ko); statusErr != nil {
			log.Error(statusErr, "Failed to update drain status")
		}
		return OutcomeRetry, nil
	}

	// Drain completed successfully (no errors means all resources processed)
	ko.Status.DrainStatus = ngrokv1alpha1.DrainStatusCompleted
	ko.Status.DrainMessage = "Drain completed successfully"
	ko.Status.DrainErrors = nil
	if err := o.client.Status().Update(ctx, ko); err != nil {
		return OutcomeFailed, fmt.Errorf("failed to update drain completed status: %w", err)
	}
	o.recorder.Event(ko, v1.EventTypeNormal, "DrainCompleted", "All managed resources have been drained")
	log.Info("Drain completed successfully", "progress", result.Progress())

	return OutcomeComplete, nil
}
