package models

import (
    "time"
    "golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
    ID             string    `json:"id" db:"id"`
    Email          string    `json:"email" db:"email"`
    PasswordHash   string    `json:"-" db:"password_hash"`
    FirstName      string    `json:"first_name" db:"first_name"`
    LastName       string    `json:"last_name" db:"last_name"`
    Role           string    `json:"role" db:"role"`
    TenantID       string    `json:"tenant_id" db:"tenant_id"`
    IsActive       bool      `json:"is_active" db:"is_active"`
    EmailVerified  bool      `json:"email_verified" db:"email_verified"`
    CreatedAt      time.Time `json:"created_at" db:"created_at"`
    UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
    LastLoginAt    *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
}

// LoginRequest represents a login request
type LoginRequest struct {
    Email    string `json:"email" validate:"omitempty,email"`
    Username string `json:"username" validate:"omitempty"`
    Password string `json:"password" validate:"required,min=8"`
}

// GetIdentifier returns email if provided, otherwise username (for login flexibility)
func (lr *LoginRequest) GetIdentifier() string {
    if lr.Email != "" {
        return lr.Email
    }
    return lr.Username
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
    Email     string `json:"email" validate:"required,email"`
    Password  string `json:"password" validate:"required,min=8"`
    FirstName string `json:"first_name" validate:"required"`
    LastName  string `json:"last_name" validate:"required"`
    TenantID  string `json:"tenant_id,omitempty"`
}

// TokenResponse represents a JWT token response
type TokenResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    TokenType    string `json:"token_type"`
    ExpiresIn    int    `json:"expires_in"`
}

// RefreshRequest represents a token refresh request
type RefreshRequest struct {
    RefreshToken string `json:"refresh_token" validate:"required"`
}

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
    FirstName string `json:"first_name,omitempty" validate:"omitempty,min=1"`
    LastName  string `json:"last_name,omitempty" validate:"omitempty,min=1"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
    CurrentPassword string `json:"current_password" validate:"required"`
    NewPassword     string `json:"new_password" validate:"required,min=8"`
}

// Claims represents JWT claims
type Claims struct {
    UserID   string `json:"user_id"`
    Email    string `json:"email"`
    Role     string `json:"role"`
    TenantID string `json:"tenant_id"`
    Exp      int64  `json:"exp"`
    Iat      int64  `json:"iat"`
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    return string(bytes), err
}

// CheckPassword compares a password with a hash
func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}