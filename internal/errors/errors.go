package errors

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
