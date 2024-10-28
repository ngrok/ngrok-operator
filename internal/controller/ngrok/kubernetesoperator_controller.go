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
	"slices"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (r *KubernetesOperatorReconciler) create(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) error {
	var k8sOp *ngrok.KubernetesOperator
	k8sOp, err := r.findExisting(ctx, ko)
	r.fillInStatus(ctx, ko, nil, err)
	if err != nil {
		return err
	}

	// Already exists, so update the status to match the existing KubernetesOperator
	// and make sure it is up to date
	if k8sOp != nil {
		return r._update(ctx, ko, k8sOp)
	}

	// Not found, so we'll create the KubernetesOperator
	createParams := &ngrok.KubernetesOperatorCreate{
		Metadata:        r.tryMergeMetadata(ctx, ko),
		Description:     ko.Spec.Description,
		EnabledFeatures: calculateFeaturesEnabled(ko),
		Region:          ko.Spec.Region,
		Deployment: ngrok.KubernetesOperatorDeployment{
			Name:      ko.Spec.Deployment.Name,
			Namespace: ko.Spec.Deployment.Namespace,
			Version:   ko.Spec.Deployment.Version,
		},
	}

	var tlsSecret *v1.Secret
	if slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings) {
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
	r.fillInStatus(ctx, ko, k8sOp, err)
	if err != nil {
		return err
	}

	return r.updateTLSSecretCert(ctx, tlsSecret, k8sOp)
}

func (r *KubernetesOperatorReconciler) update(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator) error {
	log := ctrl.LoggerFrom(ctx).WithValues("id", ko.Status.ID)

	log.V(3).Info("fetching KubernetesOperator from ngrok API")
	ngrokKo, err := r.NgrokClientset.KubernetesOperators().Get(ctx, ko.Status.ID)

	r.fillInStatus(ctx, ko, nil, nil)
	if err != nil {
		// If we can't find it, we'll clear the ID and re-reconcile
		if ngrok.IsNotFound(err) {
			log.Error(err, "expected to find KubernetesOperator in ngrok API, but it was not found")
		}
		return err
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

// fillInStatus fills in the status fields of the KubernetesOperator CRD based on the current state of the ngrok API
func (r *KubernetesOperatorReconciler) fillInStatus(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator, ngrokKo *ngrok.KubernetesOperator, err error) {
	if err != nil {
		errorCode := ""    // default unset
		errorMessage := "" // default unset

		var ngrokErr ngrok.Error
		if errors.As(err, &ngrokErr) {
			errorCode = ngrokErr.ErrorCode
			errorMessage = ngrokErr.Msg
		} else {
			errorMessage = err.Error()
		}

		// registration failed
		ko.Status.ID = ""
		ko.Status.URI = ""
		ko.Status.RegistrationStatus = ngrokv1alpha1.KubernetesOperatorRegistrationStatusError
		ko.Status.RegistrationErrorCode = errorCode
		ko.Status.RegistrationErrorMessage = errorMessage
	} else {
		if ngrokKo == nil {
			// If the KubernetesOperator is not found, clear the status fields
			ko.Status.ID = ""
			ko.Status.URI = ""
			ko.Status.RegistrationStatus = ngrokv1alpha1.KubernetesOperatorRegistrationStatusPending
			ko.Status.RegistrationErrorCode = ""
			ko.Status.RegistrationErrorMessage = ""
		} else {
			// If the KubernetesOperator is found, update the status fields
			ko.Status.ID = ngrokKo.ID
			ko.Status.URI = ngrokKo.URI
			ko.Status.RegistrationStatus = ngrokv1alpha1.KubernetesOperatorRegistrationStatusSuccess
			ko.Status.RegistrationErrorCode = ""
			ko.Status.RegistrationErrorMessage = ""
		}
	}
}

func (r *KubernetesOperatorReconciler) _update(ctx context.Context, ko *ngrokv1alpha1.KubernetesOperator, ngrokKo *ngrok.KubernetesOperator) (err error) {
	log := ctrl.LoggerFrom(ctx)

	defer func() {
		r.fillInStatus(ctx, ko, ngrokKo, err)
	}()

	// Update the KubernetesOperator in the ngrok API
	updateParams := &ngrok.KubernetesOperatorUpdate{
		ID:              ngrokKo.ID,
		Description:     ptr.To(ko.Spec.Description),
		Metadata:        ptr.To(r.tryMergeMetadata(ctx, ko)),
		EnabledFeatures: calculateFeaturesEnabled(ko),
		Region:          ptr.To(ko.Spec.Region),
	}

	var tlsSecret *v1.Secret
	if slices.Contains(ko.Spec.EnabledFeatures, ngrokv1alpha1.KubernetesOperatorFeatureBindings) {
		tlsSecret, err = r.findOrCreateTLSSecret(ctx, ko)
		if err != nil {
			return err
		}

		updateParams.Binding = &ngrok.KubernetesOperatorBindingUpdate{
			Name:        ptr.To(ko.Spec.Binding.Name),
			AllowedURLs: ko.Spec.Binding.AllowedURLs,
			CSR:         ptr.To(string(tlsSecret.Data["tls.csr"])),
		}
	}

	ngrokKo, err = r.NgrokClientset.KubernetesOperators().Update(ctx, updateParams)
	if err != nil {
		log.Error(err, "failed to update KubernetesOperator in ngrok API")
		return err
	}

	return r.updateTLSSecretCert(ctx, tlsSecret, ngrokKo)
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
			if uid != string(namespaceUID) {
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
	if err == nil {
		return
	}

	if !apierrors.IsNotFound(err) {
		return
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
			Namespace: ko.GetNamespace(),
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.key": pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: privateKeyBytes}),
			"tls.crt": {},
			"tls.csr": csr,
		},
	}

	err = r.Client.Create(ctx, secret)
	return
}

// Update the KubernetesOperator with the latest TLS certificate from the ngrok API
func (r *KubernetesOperatorReconciler) updateTLSSecretCert(ctx context.Context, secret *v1.Secret, ngrokKo *ngrok.KubernetesOperator) error {
	if ngrokKo.Binding == nil {
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
