package shared

import "errors"

// Domain-specific errors
var (
	// Common validation errors
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrInvalidID       = errors.New("invalid identifier")
	ErrEmptyValue      = errors.New("value cannot be empty")
	ErrInvalidStatus   = errors.New("invalid status")
	
	// Aggregate errors
	ErrAggregateNotFound    = errors.New("aggregate not found")
	ErrAggregateExists      = errors.New("aggregate already exists")
	ErrConcurrencyConflict  = errors.New("concurrency conflict - entity was modified")
	ErrInvalidVersion       = errors.New("invalid entity version")
	
	// Business rule errors
	ErrBusinessRuleViolation = errors.New("business rule violation")
	ErrInvalidOperation      = errors.New("invalid operation")
	ErrAccessDenied          = errors.New("access denied")
	ErrTenantMismatch        = errors.New("tenant mismatch")
	
	// Event errors
	ErrInvalidEvent    = errors.New("invalid domain event")
	ErrEventNotHandled = errors.New("event handler not found")
)

// DomainError represents a domain-specific error with context
type DomainError struct {
	Code    string
	Message string
	Cause   error
	Context map[string]interface{}
}

// Error implements the error interface
func (e *DomainError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// NewDomainError creates a new domain error
func NewDomainError(code, message string, cause error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context to the error
func (e *DomainError) WithContext(key string, value interface{}) *DomainError {
	e.Context[key] = value
	return e
}

// Error codes
const (
	ErrCodeValidation     = "VALIDATION_ERROR"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeAlreadyExists  = "ALREADY_EXISTS"
	ErrCodeConcurrency    = "CONCURRENCY_CONFLICT"
	ErrCodeConflict       = "CONFLICT"
	ErrCodeBusinessRule   = "BUSINESS_RULE_VIOLATION"
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeInternal       = "INTERNAL_ERROR"
)