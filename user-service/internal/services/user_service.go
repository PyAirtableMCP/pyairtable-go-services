package services

import (
	"context"
	"fmt"
	"strings"
	
	"github.com/Reg-Kris/pyairtable-user-service/internal/models"
	"github.com/Reg-Kris/pyairtable-user-service/internal/repository"
	"go.uber.org/zap"
)

type UserService struct {
	logger    *zap.Logger
	userRepo  repository.UserRepository
	cacheRepo repository.CacheRepository
}

func NewUserService(logger *zap.Logger, userRepo repository.UserRepository, cacheRepo repository.CacheRepository) *UserService {
	return &UserService{
		logger:    logger,
		userRepo:  userRepo,
		cacheRepo: cacheRepo,
	}
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(ctx context.Context, userID string) (*models.User, error) {
	// Check cache first
	if s.cacheRepo != nil {
		cached, err := s.cacheRepo.GetUser(ctx, userID)
		if err != nil {
			s.logger.Warn("Cache get error", zap.Error(err))
		} else if cached != nil {
			return cached, nil
		}
	}
	
	// Get from database
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if s.cacheRepo != nil {
		if err := s.cacheRepo.SetUser(ctx, user); err != nil {
			s.logger.Warn("Cache set error", zap.Error(err))
		}
	}
	
	return user, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return s.userRepo.FindByEmail(ctx, email)
}

// CreateUser creates a new user (usually called by auth service)
func (s *UserService) CreateUser(ctx context.Context, req *models.CreateUserRequest) (*models.User, error) {
	// Check if user already exists
	existing, _ := s.userRepo.FindByEmail(ctx, req.Email)
	if existing != nil {
		return nil, fmt.Errorf("user with email %s already exists", req.Email)
	}
	
	// Create user
	user := &models.User{
		ID:            req.ID,
		Email:         req.Email,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		DisplayName:   fmt.Sprintf("%s %s", req.FirstName, req.LastName),
		TenantID:      req.TenantID,
		Role:          req.Role,
		IsActive:      req.IsActive,
		EmailVerified: req.EmailVerified,
		Metadata:      req.Metadata,
		Preferences:   make(map[string]interface{}),
	}
	
	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logger.Error("Failed to create user", zap.Error(err))
		return nil, err
	}
	
	// Invalidate cache
	if s.cacheRepo != nil {
		s.cacheRepo.InvalidateTenantCache(ctx, user.TenantID)
	}
	
	return user, nil
}

// UpdateUser updates a user's profile
func (s *UserService) UpdateUser(ctx context.Context, userID string, req *models.UpdateUserRequest) (*models.User, error) {
	// Get existing user
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	
	// Update fields
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.Avatar != nil {
		user.Avatar = *req.Avatar
	}
	if req.Bio != nil {
		user.Bio = *req.Bio
	}
	if req.Phone != nil {
		user.Phone = *req.Phone
	}
	if req.Preferences != nil {
		user.Preferences = req.Preferences
	}
	if req.Metadata != nil {
		user.Metadata = req.Metadata
	}
	
	// Save updates
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update user", zap.Error(err))
		return nil, err
	}
	
	// Invalidate cache
	if s.cacheRepo != nil {
		s.cacheRepo.InvalidateUserCache(ctx, userID)
	}
	
	return user, nil
}

// DeleteUser soft deletes a user
func (s *UserService) DeleteUser(ctx context.Context, userID string) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	
	if err := s.userRepo.SoftDelete(ctx, userID); err != nil {
		s.logger.Error("Failed to delete user", zap.Error(err))
		return err
	}
	
	// Invalidate cache
	if s.cacheRepo != nil {
		s.cacheRepo.InvalidateUserCache(ctx, userID)
		s.cacheRepo.InvalidateTenantCache(ctx, user.TenantID)
	}
	
	return nil
}

// ListUsers retrieves a paginated list of users
func (s *UserService) ListUsers(ctx context.Context, filter *models.UserFilter) (*models.UserListResponse, error) {
	// Generate cache key
	cacheKey := s.generateListCacheKey(filter)
	
	// Check cache for list
	if s.cacheRepo != nil && filter.Page == 1 { // Only cache first page
		cached, err := s.cacheRepo.GetUserList(ctx, cacheKey)
		if err != nil {
			s.logger.Warn("Cache get error", zap.Error(err))
		} else if cached != nil {
			// Still need to get total count
			_, total, _ := s.userRepo.List(ctx, &models.UserFilter{
				TenantID: filter.TenantID,
				PageSize: 1,
			})
			
			return &models.UserListResponse{
				Users:      cached,
				Total:      total,
				Page:       filter.Page,
				PageSize:   filter.PageSize,
				TotalPages: int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize)),
			}, nil
		}
	}
	
	// Get from database
	users, total, err := s.userRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	
	// Cache first page results
	if s.cacheRepo != nil && filter.Page == 1 && len(users) > 0 {
		if err := s.cacheRepo.SetUserList(ctx, cacheKey, users, 300); err != nil {
			s.logger.Warn("Cache set error", zap.Error(err))
		}
	}
	
	return &models.UserListResponse{
		Users:      users,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize)),
	}, nil
}

// GetUserStats retrieves user statistics
func (s *UserService) GetUserStats(ctx context.Context, tenantID string) (*models.UserStats, error) {
	cacheKey := fmt.Sprintf("tenant:%s", tenantID)
	if tenantID == "" {
		cacheKey = "global"
	}
	
	// Check cache
	if s.cacheRepo != nil {
		cached, err := s.cacheRepo.GetStats(ctx, cacheKey)
		if err != nil {
			s.logger.Warn("Cache get error", zap.Error(err))
		} else if cached != nil {
			return cached, nil
		}
	}
	
	// Get from database
	stats, err := s.userRepo.GetStats(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if s.cacheRepo != nil {
		if err := s.cacheRepo.SetStats(ctx, cacheKey, stats, 3600); err != nil {
			s.logger.Warn("Cache set error", zap.Error(err))
		}
	}
	
	return stats, nil
}

// BulkCreateUsers creates multiple users
func (s *UserService) BulkCreateUsers(ctx context.Context, users []*models.User) error {
	if err := s.userRepo.BulkCreate(ctx, users); err != nil {
		return err
	}
	
	// Invalidate cache
	if s.cacheRepo != nil {
		tenants := make(map[string]bool)
		for _, user := range users {
			tenants[user.TenantID] = true
		}
		for tenantID := range tenants {
			s.cacheRepo.InvalidateTenantCache(ctx, tenantID)
		}
	}
	
	return nil
}

// GetUsersByIDs retrieves multiple users by their IDs
func (s *UserService) GetUsersByIDs(ctx context.Context, ids []string) ([]*models.User, error) {
	return s.userRepo.FindByIDs(ctx, ids)
}

// Helper function to generate cache key for list operations
func (s *UserService) generateListCacheKey(filter *models.UserFilter) string {
	parts := []string{}
	
	if filter.TenantID != "" {
		parts = append(parts, fmt.Sprintf("tenant:%s", filter.TenantID))
	}
	if filter.Role != "" {
		parts = append(parts, fmt.Sprintf("role:%s", filter.Role))
	}
	if filter.IsActive != nil {
		parts = append(parts, fmt.Sprintf("active:%t", *filter.IsActive))
	}
	if filter.EmailVerified != nil {
		parts = append(parts, fmt.Sprintf("verified:%t", *filter.EmailVerified))
	}
	if filter.Search != "" {
		parts = append(parts, fmt.Sprintf("search:%s", filter.Search))
	}
	parts = append(parts, fmt.Sprintf("sort:%s:%s", filter.SortBy, filter.SortOrder))
	
	return strings.Join(parts, ":")
}