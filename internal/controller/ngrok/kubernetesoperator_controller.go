/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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
package ngrok

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
)

var featureMap = map[string]string{
	ngrokv1alpha1.KubernetesOperatorFeatureBindings: "bindings",
	ngrokv1alpha1.KubernetesOperatorFeatureIngress:  "ingress",
	ngrokv1alpha1.KubernetesOperatorFeatureGateway:  "gateway",
}

const (
	NgrokErrorFailedToCreateCSR = "ERR_NGROK_20006"
)

// KubernetesOperatorReconciler reconciles a KubernetesOperator object
type KubernetesOperatorReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*ngrokv1alpha1.KubernetesOperator]

	Log            logr.Logger
	Recorder       record.EventRecorder
	NgrokClientset ngrokapi.Clientset

	// Namespace where the ngrok-operator is managing its resources
	Namespace string

	// DrainState is the shared drain state checker. The controller uses IsDraining()
	// to detect drain mode and SetDraining(true) to trigger it.
	DrainState *drain.StateChecker

	// WatchNamespace limits draining to resources in this namespace (empty = all namespaces)
	WatchNamespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {

	if r.NgrokClientset == nil {
		return errors.New("NgrokClientset is required")
	}

	r.controller = &controller.BaseController[*ngrokv1alpha1.KubernetesOperator]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		Namespace: &r.Namespace,

		StatusID: func(obj *ngrokv1alpha1.KubernetesOperator) string { return obj.Status.ID },
		Create:   r.create,
		Update:   r.update,
		Delete:   r.delete,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1alpha1.KubernetesOperator{}).
		WithEventFilter(
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		).
		Complete(r)
}

// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=kubernetesoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=kubernetesoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=kubernetesoperators/finalizers,verbs=update

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *KubernetesOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("kubernetesoperator", req.NamespacedName)

	ko := &ngrokv1alpha1.KubernetesOperator{}
	if err := r.Client.Get(ctx, req.NamespacedName, ko); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Use the shared DrainState as the single source of truth for drain detection
	if r.DrainState != nil && r.DrainState.IsDraining(ctx) {
		return r.handleDrain(ctx, ko, log)
	}

	return r.controller.Reconcile(ctx, req, new(ngrokv1alpha1.KubernetesOperator))
}

func (r *KubernetesOperatorReconciler) handleDrain(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator, log logr.Logger) (ctrl.Result, error) {
	// Ensure the drain state is set so all other controllers immediately see it
	if r.DrainState != nil {
		r.DrainState.SetDraining(true)
	}
	log.Info("Starting drain process")

	if ko.Status.DrainStatus != ngrokv1alpha1.DrainStatusDraining {
		ko.Status.DrainStatus = ngrokv1alpha1.DrainStatusDraining
		ko.Status.DrainMessage = "Drain in progress"
		if err := r.Client.Status().Update(ctx, ko); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(ko, v1.EventTypeNormal, "DrainStarted", "Starting drain of all managed resources")
	}

	policy := ko.GetDrainPolicy()

	drainer := &drain.Drainer{
		Client:         r.Client,
		Log:            log,
		Policy:         policy,
		WatchNamespace: r.WatchNamespace,
	}

	result, err := drainer.DrainAll(ctx)
	if err != nil {
		ko.Status.DrainStatus = ngrokv1alpha1.DrainStatusFailed
		ko.Status.DrainMessage = fmt.Sprintf("Drain failed: %v", err)
		ko.Status.DrainProgress = result.Progress()
		if statusErr := r.Client.Status().Update(ctx, ko); statusErr != nil {
			log.Error(statusErr, "Failed to update drain status")
		}
		r.Recorder.Event(ko, v1.EventTypeWarning, "DrainFailed", fmt.Sprintf("Drain failed: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	ko.Status.DrainProgress = result.Progress()
	ko.Status.DrainErrors = result.ErrorStrings()

	if result.HasErrors() {
		ko.Status.DrainMessage = fmt.Sprintf("Drain completed with %d errors", result.Failed)
		for _, err := range result.Errors {
			log.Error(err, "Drain error")
		}
		if statusErr := r.Client.Status().Update(ctx, ko); statusErr != nil {
			log.Error(statusErr, "Failed to update drain status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if !result.IsComplete() {
		ko.Status.DrainMessage = "Drain in progress"
		if statusErr := r.Client.Status().Update(ctx, ko); statusErr != nil {
			log.Error(statusErr, "Failed to update drain status")
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	ko.Status.DrainStatus = ngrokv1alpha1.DrainStatusCompleted
	ko.Status.DrainMessage = "Drain completed successfully"
	ko.Status.DrainErrors = nil
	if err := r.Client.Status().Update(ctx, ko); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(ko, v1.EventTypeNormal, "DrainCompleted", "All managed resources have been drained")
	log.Info("Drain completed successfully", "progress", result.Progress())

	if !ko.DeletionTimestamp.IsZero() {
		log.Info("Deleting KubernetesOperator from ngrok API")
		if ko.Status.ID != "" {
			if err := r.NgrokClientset.KubernetesOperators().Delete(ctx, ko.Status.ID); err != nil {
				if !ngrok.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}

		if util.RemoveFinalizer(ko) {
			if err := r.Client.Update(ctx, ko); err != nil {
				return ctrl.Result{}, err
			}
		}
		log.Info("Finalizer removed, KubernetesOperator will be deleted")
	}

	return ctrl.Result{}, nil
}

func (r *KubernetesOperatorReconciler) create(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) (err error) {
	var k8sOp *ngrok.KubernetesOperator
	k8sOp, err = r.findExisting(ctx, ko)
	if err != nil {
		return r.updateStatus(ctx, ko, nil, err)
	}

	// Already exists, so update the status to match the existing KubernetesOperator
	// and make sure it is up to date
	if k8sOp != nil {
		return r._update(ctx, ko, k8sOp)
	}

	// In our spec, this is possibly nil, so we need to guard against it.
	deployment := ngrok.KubernetesOperatorDeployment{}
	if ko.Spec.Deployment != nil {
		deployment.Name = ko.Spec.Deployment.Name
		deployment.Namespace = ko.Spec.Deployment.Namespace
		deployment.Version = ko.Spec.Deployment.Version
	}

	// Not found, so we'll create the KubernetesOperator
	createParams := &ngrok.KubernetesOperatorCreate{
		Metadata:        r.tryMergeMetadata(ctx, ko),
		Description:     ko.Spec.Description,
		EnabledFeatures: calculateFeaturesEnabled(ko),
		Region:          ko.Spec.Region,
		Deployment:      deployment,
	}

	bindingsEnabled := slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings)
	var tlsSecret *v1.Secret

	if bindingsEnabled {
		tlsSecret, err = r.findOrCreateTLSSecret(ctx, ko)
		if err != nil {
			return ngrokapi.NewNgrokError(err, ngrokapi.NgrokOpErrFailedToCreateCSR, "failed to create TLS secret for CSR")
		}

		createParams.Binding = &ngrok.KubernetesOperatorBindingCreate{
			EndpointSelectors: ko.Spec.Binding.EndpointSelectors,
			CSR:               string(tlsSecret.Data["tls.csr"]),
		}
	}

	// Create the KubernetesOperator in the ngrok API
	k8sOp, err = r.NgrokClientset.KubernetesOperators().Create(ctx, createParams)
	if err != nil {
		return r.updateStatus(ctx, ko, nil, err)
	}

	if bindingsEnabled {
		err = r.updateTLSSecretCert(ctx, tlsSecret, k8sOp)
	}

	return r.updateStatus(ctx, ko, k8sOp, err)
}

func (r *KubernetesOperatorReconciler) update(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) error {
	log := ctrl.LoggerFrom(ctx).WithValues("id", ko.Status.ID)

	log.V(3).Info("fetching KubernetesOperator from ngrok API")
	ngrokKo, err := r.NgrokClientset.KubernetesOperators().Get(ctx, ko.Status.ID)
	if err != nil {
		return r.updateStatus(ctx, ko, nil, err)
	}

	return r._update(ctx, ko, ngrokKo)
}

func (r *KubernetesOperatorReconciler) delete(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) error {
	err := r.NgrokClientset.KubernetesOperators().Delete(ctx, ko.Status.ID)
	if err == nil || ngrok.IsNotFound(err) {
		ko.Status.ID = ""
	}
	return err
}

// updateStatus fills in the status fields of the KubernetesOperator CRD based on the current state of the ngrok API and updates the status in k8s
func (r *KubernetesOperatorReconciler) updateStatus(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator, ngrokKo *ngrok.KubernetesOperator, err error) error {
	existsInNgrokAPI := ngrokKo != nil && ngrokKo.ID != ""

	if existsInNgrokAPI {
		ko.Status.ID = ngrokKo.ID
		ko.Status.URI = ngrokKo.URI

		if ngrokKo.EnabledFeatures != nil {
			ko.Status.EnabledFeatures = strings.Join(ngrokKo.EnabledFeatures, ",")
		}
		if ngrokKo.Binding != nil {
			ko.Status.BindingsIngressEndpoint = ngrokKo.Binding.IngressEndpoint
		}

		ko.Status.RegistrationStatus = ngrokv1alpha1.KubernetesOperatorRegistrationStatusSuccess
	} else {
		if err != nil {
			ko.Status.RegistrationStatus = ngrokv1alpha1.KubernetesOperatorRegistrationStatusError
		}
		ko.Status.RegistrationStatus = ngrokv1alpha1.KubernetesOperatorRegistrationStatusPending
	}

	errorCode := ""
	errorMessage := ""

	// Handle errors
	if err != nil {
		errorMessage := err.Error() // default to the error message

		var ngrokErr *ngrok.Error
		if errors.As(err, &ngrokErr) {
			errorCode = ngrokErr.ErrorCode
			errorMessage = ngrokErr.Msg
		}

		ko.Status.RegistrationErrorCode = errorCode
		ko.Status.RegistrationErrorMessage = errorMessage

		// Special case for NotFound errors, we'll clear the ID and URI so we can re-queue the reconciliation
		if ngrok.IsNotFound(err) {
			ko.Status.ID = ""
			ko.Status.URI = ""
		}
	}

	ko.Status.RegistrationErrorCode = errorCode
	ko.Status.RegistrationErrorMessage = errorMessage

	return r.controller.ReconcileStatus(ctx, ko, err)
}

func (r *KubernetesOperatorReconciler) _update(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator, ngrokKo *ngrok.KubernetesOperator) (err error) {
	log := ctrl.LoggerFrom(ctx)

	// In our spec, this is possibly nil, so we need to guard against it.
	var deployment *ngrok.KubernetesOperatorDeploymentUpdate
	if ko.Spec.Deployment != nil {
		deployment = &ngrok.KubernetesOperatorDeploymentUpdate{
			Name:    &ko.Spec.Deployment.Name,
			Version: &ko.Spec.Deployment.Version,
		}
	}

	// Update the KubernetesOperator in the ngrok API
	updateParams := &ngrok.KubernetesOperatorUpdate{
		ID:              ngrokKo.ID,
		Description:     ptr.To(ko.Spec.Description),
		Metadata:        ptr.To(r.tryMergeMetadata(ctx, ko)),
		EnabledFeatures: calculateFeaturesEnabled(ko),
		Region:          ptr.To(ko.Spec.Region),
		Deployment:      deployment,
	}

	bindingsEnabled := slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings)
	var tlsSecret *v1.Secret

	if bindingsEnabled {
		tlsSecret, err = r.findOrCreateTLSSecret(ctx, ko)
		if err != nil {
			return r.updateStatus(ctx, ko, nil, err)
		}

		updateParams.Binding = &ngrok.KubernetesOperatorBindingUpdate{
			EndpointSelectors: ko.Spec.Binding.EndpointSelectors,
			CSR:               ptr.To(string(tlsSecret.Data["tls.csr"])),
		}
	}

	log.V(1).Info("updating KubernetesOperator in ngrok API", "id", ngrokKo.ID)
	ngrokKo, err = r.NgrokClientset.KubernetesOperators().Update(ctx, updateParams)
	if err != nil {
		log.Error(err, "failed to update KubernetesOperator in ngrok API")
		return r.updateStatus(ctx, ko, nil, err)
	}

	log.V(1).Info("successfully updated KubernetesOperator in ngrok API", "id", ngrokKo.ID)

	if bindingsEnabled {
		err = r.updateTLSSecretCert(ctx, tlsSecret, ngrokKo)
	}
	return r.updateStatus(ctx, ko, ngrokKo, err)
}

// fuzzy match against the ngrok API to find an existing KubernetesOperator
func (r *KubernetesOperatorReconciler) findExisting(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) (*ngrok.KubernetesOperator, error) {
	log := ctrl.LoggerFrom(ctx)

	namespaceUID, err := getNamespaceUID(ctx, r.Client, ko.GetNamespace())
	if err != nil {
		return nil, nil
	}

	iter := r.NgrokClientset.KubernetesOperators().List(&ngrok.Paging{})
	for iter.Next(ctx) {
		item := iter.Item()
		iterLogger := log.WithValues(
			"id", item.ID,
			"deployment", item.Deployment.Name,
			"namespace", item.Deployment.Namespace,
			"metadata", item.Metadata,
		)

		iterLogger.V(5).Info("checking if KubernetesOperator matches")

		if item.Deployment.Name != ko.Spec.Deployment.Name {
			continue
		}
		if item.Deployment.Namespace != ko.GetNamespace() {
			continue
		}

		// In case the KubernetesOperator already exists in the ngrok API, check if it's the namespace
		// UID is the same as the one we are trying to create. If it is, use the existing one since we
		// get conflicts if we try to create a new one.
		metadata := item.Metadata
		if metadata != "" {
			uid, err := extractNamespaceUIDFromMetadata(metadata)
			// In case the metadata is not a JSON object or we can't extract it,
			// we'll ignore it and continue our search
			if err != nil || uid == "" {
				continue
			}
			if uid != namespaceUID {
				continue
			}
		}

		iterLogger.V(3).Info("found matching KubernetesOperator")
		return item, nil
	}

	return nil, iter.Err()
}

func calculateFeaturesEnabled(ko *ngrokv1alpha1.KubernetesOperator) []string {
	features := []string{}

	for _, f := range ko.Spec.EnabledFeatures {
		if v, ok := featureMap[f]; ok {
			features = append(features, v)
		}
	}
	return features
}

func (r *KubernetesOperatorReconciler) findOrCreateTLSSecret(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) (secret *v1.Secret, err error) {
	secret = &v1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Namespace: ko.GetNamespace(), Name: ko.Spec.Binding.TlsSecretName}, secret)
	if !apierrors.IsNotFound(err) {
		return
	}

	if err == nil {
		isValid := secret.Type == v1.SecretTypeTLS &&
			// tls.crt is managed by updateTLSSecretCert
			// secret.Data["tls.crt"] != nil &&
			secret.Data["tls.key"] != nil &&
			secret.Data["tls.csr"] != nil

		if isValid {
			return
		}

		// otherwise fallthrough to generate the CSR
	}

	// If the secret doesn't exist, create it with a new private key and a CSR
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}

	// Create the CSR
	var csr []byte
	csr, err = generateCSR(privKey)
	if err != nil {
		return
	}

	privateKeyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return
	}

	secret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ko.Spec.Binding.TlsSecretName,
			Namespace: r.Namespace,
		},
		Type: v1.SecretTypeTLS,
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Data = map[string][]byte{
			"tls.key": pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: privateKeyBytes}),
			"tls.crt": {},
			"tls.csr": csr,
		}

		return nil
	})

	return
}

// Update the KubernetesOperator with the latest TLS certificate from the ngrok API
func (r *KubernetesOperatorReconciler) updateTLSSecretCert(ctx context.Context, secret *v1.Secret, ngrokKo *ngrok.KubernetesOperator) error {
	if ngrokKo == nil || ngrokKo.Binding == nil || secret == nil {
		return nil
	}

	// If the certificate hasn't changed, return early
	if string(secret.Data["tls.crt"]) == ngrokKo.Binding.Cert.Cert {
		return nil
	}

	newSecret := secret.DeepCopy()
	newSecret.Data["tls.crt"] = []byte(ngrokKo.Binding.Cert.Cert)

	return r.Client.Patch(ctx, newSecret, client.MergeFrom(secret))
}

// Try merging the user-provided metadata in the KubernetesOperator spec with the namespace UID.
// This is done to see if we can adopt an existing KubernetesOperator in the ngrok API going forward.
// If there are any errors, the original metadata is returned.
func (r *KubernetesOperatorReconciler) tryMergeMetadata(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) string {
	namespaceUID, err := getNamespaceUID(ctx, r.Client, ko.GetNamespace())
	if err != nil {
		return ko.Spec.Metadata
	}

	metadata, err := mergeMetadata(ko.Spec.Metadata, namespaceUID)
	if err != nil {
		return ko.Spec.Metadata
	}

	return metadata
}

const UIDNamespaceMetadataKey = "namespace.uid"

// mergeMetadata merges the UID of the namespace of the kubernetes operator with the metadata
// provided by the user.
func mergeMetadata(metadata string, namespaceUID string) (string, error) {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return "", err
	}
	m[UIDNamespaceMetadataKey] = namespaceUID
	metadataBytes, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(metadataBytes), nil
}

func extractNamespaceUIDFromMetadata(metadata string) (string, error) {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return "", err
	}
	uid, ok := m[UIDNamespaceMetadataKey].(string)
	if !ok {
		return "", nil
	}
	return uid, nil
}

// nolint:unused
func generateCSR(privKey *ecdsa.PrivateKey) ([]byte, error) {
	subj := pkix.Name{}

	template := x509.CertificateRequest{
		Subject:            subj,
		SignatureAlgorithm: x509.ECDSAWithSHA512,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getNamespaceUID(ctx context.Context, r client.Reader, namespace string) (string, error) {
	ns := &v1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: namespace}, ns)
	if err != nil {
		return "", err
	}
	return string(ns.UID), nil
}
