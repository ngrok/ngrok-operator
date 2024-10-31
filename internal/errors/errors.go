package errors

import (
	"fmt"
	"strings"

	netv1 "k8s.io/api/networking/v1"
)

// Not all domains are reconciled yet and have a domain in their status
type NotAllDomainsReadyYetError struct{}

// Error returns the error message
func (e *NotAllDomainsReadyYetError) Error() string {
	return "not all domains ready yet"
}

// NewNotAllDomainsReadyYetError returns a new NotAllDomainsReadyYetError
func NewNotAllDomainsReadyYetError() error {
	return &NotAllDomainsReadyYetError{}
}

// IsNotAllDomainsReadyYet returns true if the error is a NotAllDomainsReadyYetError
func IsNotAllDomainsReadyYet(err error) bool {
	_, ok := err.(*NotAllDomainsReadyYetError)
	return ok
}

// ErrNotFoundInStore is meant to be used when an object is not found in the store so
// that the caller can decide what to do with it.
type ErrNotFoundInStore struct {
	message string
}

// NewErrorNotFound returns a new ErrNotFoundInStore
func NewErrorNotFound(message string) ErrNotFoundInStore {
	return ErrNotFoundInStore{message: message}
}

// Error: Stringer: returns the error message
func (e ErrNotFoundInStore) Error() string {
	return e.message
}

// IsErrorNotFound: Reflect: returns true if the error is a ErrNotFoundInStore
func IsErrorNotFound(err error) bool {
	_, ok := err.(ErrNotFoundInStore)
	return ok
}

// ErrIngressClassNotFoundInStore is meant to be used when an object is not found in the store so
// that the caller can decide what to do with it.
type ErrIngressClassNotFoundInStore struct {
	message string
}

// NewErrorNotFound returns a new ErrIngressClassNotFoundInStore
func NewIngressClassErrorNotFound(message string) ErrIngressClassNotFoundInStore {
	return ErrIngressClassNotFoundInStore{message: message}
}

// Error: Stringer: returns the error message
func (e ErrIngressClassNotFoundInStore) Error() string {
	return e.message
}

// IsErrorIngressClassNotFound: Reflect: returns true if the error is a ErrIngressClassNotFoundInStore
func IsErrIngressClassNotFound(err error) bool {
	_, ok := err.(ErrIngressClassNotFoundInStore)
	return ok
}

// ErrInvalidIngressClass is meant to be used when an ingress object has an invalid ingress class
type ErrDifferentIngressClass struct {
	message string
}

// NewErrDifferentIngressClass returns a new ErrDifferentIngressClass
func NewErrDifferentIngressClass(ourIngressClasses []*netv1.IngressClass, foundIngressClass *string) ErrDifferentIngressClass {
	msg := []string{"The controller will not reconcile this ingress object due to the ingress class mismatching."}
	if foundIngressClass == nil {
		msg = append(msg, "The ingress object does not have an ingress class set.")
	} else {
		msg = append(msg, fmt.Sprintf("The ingress object has an ingress class set to %s.", *foundIngressClass))
	}
	for _, ingressClass := range ourIngressClasses {
		if ingressClass.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			msg = append(msg, fmt.Sprintf("This controller is the default ingress controller ingress class %s.", ingressClass.Name))
		}
		msg = append(msg, fmt.Sprintf("This controller is watching for the class %s", ingressClass.Name))
	}
	if len(ourIngressClasses) == 0 {
		msg = append(msg, "There are no ngrok ingress classes registered in the cluster.")
	}
	return ErrDifferentIngressClass{message: strings.Join(msg, "\n")}
}

// Error: Stringer: returns the error message
func (e ErrDifferentIngressClass) Error() string {
	if e.message == "" {
		return "different ingress class"
	}
	return e.message
}

// IsErrDifferentIngressClass: Reflect: returns true if the error is a ErrDifferentIngressClass
func IsErrDifferentIngressClass(err error) bool {
	_, ok := err.(ErrDifferentIngressClass)
	return ok
}

type ErrorNoDefaultIngressClassFound struct{}

// NoDefaultIngressClassFound is meant to be used when no default ingress class is found
func NewNoDefaultIngressClassFound() error {
	return ErrorNoDefaultIngressClassFound{}
}

func (e ErrorNoDefaultIngressClassFound) Error() string {
	return "no default ingress class found"
}

func IsErrorNoDefaultIngressClassFound(err error) bool {
	_, ok := err.(ErrorNoDefaultIngressClassFound)
	return ok
}

// ErrInvalidIngressSpec is meant to be used when an ingress object has an invalid spec
type ErrInvalidIngressSpec struct {
	errors []string
}

// NewErrInvalidIngressSpec returns a new ErrInvalidIngressSpec
func NewErrInvalidIngressSpec() ErrInvalidIngressSpec {
	return ErrInvalidIngressSpec{}
}

// AddError adds an error to the list of errors
func (e *ErrInvalidIngressSpec) AddError(err string) {
	e.errors = append(e.errors, err)
}

// HasErrors returns true if there are errors
func (e ErrInvalidIngressSpec) HasErrors() bool {
	return len(e.errors) > 0
}

// Error: Stringer: returns the error message
func (e ErrInvalidIngressSpec) Error() string {
	return fmt.Sprintf("invalid ingress spec: %s", e.errors)
}

// IsErrInvalidIngressSpec: Reflect: returns true if the error is a ErrInvalidIngressSpec
func IsErrInvalidIngressSpec(err error) bool {
	_, ok := err.(ErrInvalidIngressSpec)
	return ok
}

type ErrMissingRequiredSecret struct {
	message string
}

func NewErrMissingRequiredSecret(message string) ErrMissingRequiredSecret {
	return ErrMissingRequiredSecret{message: message}
}

func (e ErrMissingRequiredSecret) Error() string {
	return fmt.Sprintf("missing required secret: %s", e.message)
}

// IsErrMissingRequiredSecret: Reflect: returns true if the error is a ErrMissingRequiredSecret
func IsErrMissingRequiredSecret(err error) bool {
	_, ok := err.(ErrMissingRequiredSecret)
	return ok
}

type ErrInvalidConfiguration struct {
	cause error
}

func NewErrInvalidConfiguration(cause error) error {
	return ErrInvalidConfiguration{cause: cause}
}

func (e ErrInvalidConfiguration) Error() string {
	return fmt.Sprintf("invalid configuration: %s", e.cause.Error())
}

func (e ErrInvalidConfiguration) Unwrap() error {
	return e.cause
}
