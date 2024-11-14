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
	"encoding/pem"
	"errors"
	"slices"
	"strings"

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
	"github.com/ngrok/ngrok-api-go/v6"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// TODO: features need to be capitalized in the ngrok API currently, this is subject to change
var featureMap = map[string]string{
	ngrokv1alpha1.KubernetesOperatorFeatureBindings: "Bindings",
	ngrokv1alpha1.KubernetesOperatorFeatureIngress:  "Ingress",
	ngrokv1alpha1.KubernetesOperatorFeatureGateway:  "Gateway",
}

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
	return r.controller.Reconcile(ctx, req, new(ngrokv1alpha1.KubernetesOperator))
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

	// Not found, so we'll create the KubernetesOperator
	createParams := &ngrok.KubernetesOperatorCreate{
		Metadata:        ko.Spec.Metadata,
		Description:     ko.Spec.Description,
		EnabledFeatures: calculateFeaturesEnabled(ko),
		Region:          ko.Spec.Region,
		Deployment: ngrok.KubernetesOperatorDeployment{
			// TODO(hkatz) clusterId
			// Cluster: ko.Spec.Deployment.Cluster,
			Name:      ko.Spec.Deployment.Name,
			Namespace: ko.Spec.Deployment.Namespace,
			Version:   ko.Spec.Deployment.Version,
		},
	}

	bindingsEnabled := slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings)
	var tlsSecret *v1.Secret

	if bindingsEnabled {
		tlsSecret, err := r.findOrCreateTLSSecret(ctx, ko)
		if err != nil {
			return err
		}

		createParams.Binding = &ngrok.KubernetesOperatorBindingCreate{
			Name:        ko.Spec.Binding.Name,
			AllowedURLs: ko.Spec.Binding.AllowedURLs,
			CSR:         string(tlsSecret.Data["tls.csr"]),
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

	// confirm that the ngrokKo we recieve matches our given ko we're updating
	// otherwise we need to create a new ngrokKo with the new information and ID
	if !ngrokK8sopMatchesKubernetesOperator(ngrokKo, ko) {
		log.V(3).Info("existing KubernetesOperator does not match, creating new k8sop")
		return r.create(ctx, ko) // create will find or create
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

	// Update the KubernetesOperator in the ngrok API
	updateParams := &ngrok.KubernetesOperatorUpdate{
		ID:              ngrokKo.ID,
		Description:     ptr.To(ko.Spec.Description),
		Metadata:        ptr.To(ko.Spec.Metadata),
		EnabledFeatures: calculateFeaturesEnabled(ko),
		Region:          ptr.To(ko.Spec.Region),
	}

	bindingsEnabled := slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings)
	var tlsSecret *v1.Secret

	if bindingsEnabled {
		tlsSecret, err = r.findOrCreateTLSSecret(ctx, ko)
		if err != nil {
			return r.updateStatus(ctx, ko, nil, err)
		}

		updateParams.Binding = &ngrok.KubernetesOperatorBindingUpdate{
			Name:        ptr.To(ko.Spec.Binding.Name),
			AllowedURLs: ko.Spec.Binding.AllowedURLs,
			CSR:         ptr.To(string(tlsSecret.Data["tls.csr"])),
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
		if !ngrokK8sopMatchesKubernetesOperator(item, ko) {
			iterLogger.V(5).Info("KubernetesOperator does not match")
			continue
		}

		iterLogger.V(3).Info("found matching KubernetesOperator", "id", item.ID)
		return item, nil
	}

	log.V(3).Info("no matching KubernetesOperator found")
	return nil, iter.Err()
}

// ngrokK8sopMatchesKubernetesOperator checks if the KubernetesOperator in the ngrok API matches the KubernetesOperator CRD
func ngrokK8sopMatchesKubernetesOperator(k8sop *ngrok.KubernetesOperator, ko *ngrokv1alpha1.KubernetesOperator) bool {
	if k8sop == nil || ko == nil {
		return false
	}

	// TODO(hkatz) clusterId
	// if item.Deployment.Cluster != ko.Spec.Deployment.Cluster {
	// 	continue
	// }

	if k8sop.Deployment.Name != ko.Spec.Deployment.Name {
		return false
	}

	if k8sop.Deployment.Namespace != ko.Spec.Deployment.Namespace {
		return false
	}

	// bindings enabled on the CRD
	if slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings) {
		// bindings enabled in the API
		if slices.Contains(k8sop.EnabledFeatures, featureMap[ngrokv1alpha1.KubernetesOperatorFeatureBindings]) {
			// names must match
			if k8sop.Binding.Name != ko.Spec.Binding.Name {
				return false
			}
		}
	}

	return true
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
