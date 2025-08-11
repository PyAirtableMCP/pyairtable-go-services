package repository

import (
    "time"
    "github.com/pyairtable-compose/auth-service/internal/models"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
    Create(user *models.User) error
    Update(user *models.User) error
    Delete(id string) error
    FindByID(id string) (*models.User, error)
    FindByEmail(email string) (*models.User, error)
    FindByTenant(tenantID string) ([]*models.User, error)
    List(limit, offset int) ([]*models.User, error)
    Count() (int64, error)
}

// TokenRepository defines the interface for token management
type TokenRepository interface {
    StoreRefreshToken(token, userID string, ttl time.Duration) error
    ValidateRefreshToken(token string) (userID string, err error)
    InvalidateRefreshToken(token string) error
    InvalidateAllUserTokens(userID string) error
    CleanupExpiredTokens() error
}