package repository

import (
	"context"
	"github.com/Reg-Kris/pyairtable-user-service/internal/models"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	// CRUD operations
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id string) error
	SoftDelete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	
	// Query operations
	FindByID(ctx context.Context, id string) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByIDs(ctx context.Context, ids []string) ([]*models.User, error)
	List(ctx context.Context, filter *models.UserFilter) ([]*models.User, int64, error)
	
	// Tenant operations
	ListByTenant(ctx context.Context, tenantID string, filter *models.UserFilter) ([]*models.User, int64, error)
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
	
	// Stats operations
	GetStats(ctx context.Context, tenantID string) (*models.UserStats, error)
	
	// Bulk operations
	BulkCreate(ctx context.Context, users []*models.User) error
	BulkUpdate(ctx context.Context, users []*models.User) error
	BulkDelete(ctx context.Context, ids []string) error
}

// CacheRepository defines the interface for caching
type CacheRepository interface {
	// User caching
	GetUser(ctx context.Context, id string) (*models.User, error)
	SetUser(ctx context.Context, user *models.User) error
	DeleteUser(ctx context.Context, id string) error
	
	// User list caching
	GetUserList(ctx context.Context, key string) ([]*models.User, error)
	SetUserList(ctx context.Context, key string, users []*models.User, ttl int) error
	DeleteUserList(ctx context.Context, key string) error
	
	// Stats caching
	GetStats(ctx context.Context, key string) (*models.UserStats, error)
	SetStats(ctx context.Context, key string, stats *models.UserStats, ttl int) error
	DeleteStats(ctx context.Context, key string) error
	
	// Cache invalidation
	InvalidateUserCache(ctx context.Context, userID string) error
	InvalidateTenantCache(ctx context.Context, tenantID string) error
	FlushAll(ctx context.Context) error
}