package domain

import "errors"

// Standard condition types for domain management
const (
	ConditionDomainReady = "DomainReady"
)

// Standard condition reasons for domain management
const (
	ReasonDomainReady    = "DomainReady"
	ReasonDomainCreating = "DomainCreating"
	ReasonNgrokAPIError  = "NgrokAPIError"
)

// Standard domain management errors
var (
	ErrDomainCreating = errors.New("domain is being created, requeue after delay")
)
