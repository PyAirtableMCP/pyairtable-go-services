// Package shared contains common domain types and interfaces used across all bounded contexts
package shared

import (
	"time"
	"github.com/google/uuid"
)

// ID represents a domain entity identifier
type ID struct {
	value string
}

// NewID creates a new unique identifier
func NewID() ID {
	return ID{value: uuid.New().String()}
}

// NewIDFromString creates an ID from an existing string
func NewIDFromString(value string) ID {
	return ID{value: value}
}

// String returns the string representation of the ID
func (id ID) String() string {
	return id.value
}

// IsEmpty checks if the ID is empty
func (id ID) IsEmpty() bool {
	return id.value == ""
}

// Equals checks if two IDs are equal
func (id ID) Equals(other ID) bool {
	return id.value == other.value
}

// TenantID represents a tenant identifier with validation
type TenantID struct {
	ID
}

// NewTenantID creates a new tenant ID
func NewTenantID() TenantID {
	return TenantID{ID: NewID()}
}

// NewTenantIDFromString creates a tenant ID from string
func NewTenantIDFromString(value string) TenantID {
	return TenantID{ID: NewIDFromString(value)}
}

// UserID represents a user identifier
type UserID struct {
	ID
}

// NewUserID creates a new user ID
func NewUserID() UserID {
	return UserID{ID: NewID()}
}

// NewUserIDFromString creates a user ID from string
func NewUserIDFromString(value string) UserID {
	return UserID{ID: NewIDFromString(value)}
}

// WorkspaceID represents a workspace identifier
type WorkspaceID struct {
	ID
}

// NewWorkspaceID creates a new workspace ID
func NewWorkspaceID() WorkspaceID {
	return WorkspaceID{ID: NewID()}
}

// NewWorkspaceIDFromString creates a workspace ID from string
func NewWorkspaceIDFromString(value string) WorkspaceID {
	return WorkspaceID{ID: NewIDFromString(value)}
}

// Timestamp represents a domain timestamp with business logic
type Timestamp struct {
	value time.Time
}

// NewTimestamp creates a new timestamp with current time
func NewTimestamp() Timestamp {
	return Timestamp{value: time.Now().UTC()}
}

// NewTimestampFromTime creates a timestamp from time.Time
func NewTimestampFromTime(t time.Time) Timestamp {
	return Timestamp{value: t.UTC()}
}

// Time returns the underlying time.Time
func (ts Timestamp) Time() time.Time {
	return ts.value
}

// IsZero checks if timestamp is zero
func (ts Timestamp) IsZero() bool {
	return ts.value.IsZero()
}

// Before checks if this timestamp is before another
func (ts Timestamp) Before(other Timestamp) bool {
	return ts.value.Before(other.value)
}

// After checks if this timestamp is after another
func (ts Timestamp) After(other Timestamp) bool {
	return ts.value.After(other.value)
}

// AddDuration adds a duration to the timestamp
func (ts Timestamp) AddDuration(d time.Duration) Timestamp {
	return Timestamp{value: ts.value.Add(d)}
}

// Version represents an entity version for optimistic locking
type Version struct {
	value int64
}

// NewVersion creates a new version starting at 1
func NewVersion() Version {
	return Version{value: 1}
}

// NewVersionFromInt creates a version from an integer
func NewVersionFromInt(v int64) Version {
	return Version{value: v}
}

// Int returns the version as an integer
func (v Version) Int() int64 {
	return v.value
}

// Next returns the next version
func (v Version) Next() Version {
	return Version{value: v.value + 1}
}

// Equals checks if two versions are equal
func (v Version) Equals(other Version) bool {
	return v.value == other.value
}

// Status represents a generic status enum pattern
type Status string

// Common status values
const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusPending  Status = "pending"
	StatusDeleted  Status = "deleted"
)

// IsValid checks if the status is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusPending, StatusDeleted:
		return true
	default:
		return false
	}
}

// String returns the string representation
func (s Status) String() string {
	return string(s)
}

// Email represents a validated email address
type Email struct {
	value string
}

// NewEmail creates a new email with validation
func NewEmail(email string) (Email, error) {
	if !isValidEmail(email) {
		return Email{}, ErrInvalidEmail
	}
	return Email{value: email}, nil
}

// String returns the email address
func (e Email) String() string {
	return e.value
}

// IsEmpty checks if email is empty
func (e Email) IsEmpty() bool {
	return e.value == ""
}

// Equals checks if two emails are equal
func (e Email) Equals(other Email) bool {
	return e.value == other.value
}

// isValidEmail performs basic email validation
func isValidEmail(email string) bool {
	// Basic validation - in production, use a proper email validation library
	return len(email) > 0 && 
		   len(email) <= 254 && 
		   containsAt(email) && 
		   !startsOrEndsWithSpecialChar(email)
}

func containsAt(s string) bool {
	count := 0
	for _, char := range s {
		if char == '@' {
			count++
		}
	}
	return count == 1
}

func startsOrEndsWithSpecialChar(s string) bool {
	if len(s) == 0 {
		return false
	}
	first := s[0]
	last := s[len(s)-1]
	return first == '.' || first == '@' || last == '.' || last == '@'
}