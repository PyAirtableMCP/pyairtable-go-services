#!/bin/bash

# PyAirtable Platform Service Consolidation Script
# Consolidates user-service, workspace-service, tenant-service, and notification-service
# into a single pyairtable-platform service

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICES_DIR="$SCRIPT_DIR"
PLATFORM_DIR="$SERVICES_DIR/pyairtable-platform"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Services to consolidate
SERVICES_TO_CONSOLIDATE=(
    "user-service"
    "workspace-service" 
    "notification-service"
)

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Function to check if service exists
check_service_exists() {
    local service_name="$1"
    if [[ ! -d "$SERVICES_DIR/$service_name" ]]; then
        print_error "Service $service_name not found in $SERVICES_DIR"
        return 1
    fi
    return 0
}

# Function to create directory structure
create_platform_structure() {
    print_status "Creating pyairtable-platform directory structure..."
    
    mkdir -p "$PLATFORM_DIR"/{cmd/server,internal/{handlers,services,models,config,middleware,repositories},pkg/client,configs,deployments/{docker,k8s},scripts,test/{fixtures,integration,unit}}
    
    print_success "Directory structure created"
}

# Function to extract and consolidate models
consolidate_models() {
    print_status "Consolidating models from services..."
    
    # Create base models file with shared structs
    cat > "$PLATFORM_DIR/internal/models/base.go" << 'EOF'
package models

import (
	"time"
	"gorm.io/gorm"
)

// BaseModel contains common fields for all models
type BaseModel struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TenantModel extends BaseModel with tenant isolation
type TenantModel struct {
	BaseModel
	TenantID uint `gorm:"not null;index" json:"tenant_id"`
}
EOF

    # Create user models
    cat > "$PLATFORM_DIR/internal/models/user.go" << 'EOF'
package models

import (
	"time"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Email     string         `gorm:"unique;not null" json:"email"`
	Password  string         `gorm:"not null" json:"-"`
	FirstName string         `json:"first_name"`
	LastName  string         `json:"last_name"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	TenantID  uint           `gorm:"not null;index" json:"tenant_id"`
	Tenant    Tenant         `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name for User
func (User) TableName() string {
	return "users"
}

// UserSession represents a user session
type UserSession struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Token     string    `gorm:"unique;not null" json:"-"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the table name for UserSession
func (UserSession) TableName() string {
	return "user_sessions"
}

// IsExpired checks if the session is expired
func (s *UserSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// UserProfile extends user information
type UserProfile struct {
	BaseModel
	UserID      uint   `gorm:"unique;not null" json:"user_id"`
	User        User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Avatar      string `json:"avatar"`
	Timezone    string `json:"timezone"`
	Language    string `json:"language"`
	Preferences map[string]interface{} `gorm:"type:jsonb" json:"preferences"`
}

// TableName returns the table name for UserProfile
func (UserProfile) TableName() string {
	return "user_profiles"
}
EOF

    # Create workspace models
    cat > "$PLATFORM_DIR/internal/models/workspace.go" << 'EOF'
package models

// Workspace represents a workspace within a tenant
type Workspace struct {
	TenantModel
	Name        string `gorm:"not null" json:"name"`
	Description string `json:"description"`
	OwnerID     uint   `gorm:"not null;index" json:"owner_id"`
	Owner       User   `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	IsActive    bool   `gorm:"default:true" json:"is_active"`
	Settings    map[string]interface{} `gorm:"type:jsonb" json:"settings"`
}

// TableName returns the table name for Workspace
func (Workspace) TableName() string {
	return "workspaces"
}

// WorkspaceMember represents workspace membership
type WorkspaceMember struct {
	BaseModel
	WorkspaceID uint      `gorm:"not null;index" json:"workspace_id"`
	Workspace   Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	UserID      uint      `gorm:"not null;index" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role        string    `gorm:"not null;default:'member'" json:"role"` // owner, admin, member, viewer
	Permissions map[string]interface{} `gorm:"type:jsonb" json:"permissions"`
}

// TableName returns the table name for WorkspaceMember
func (WorkspaceMember) TableName() string {
	return "workspace_members"
}
EOF

    # Create tenant models
    cat > "$PLATFORM_DIR/internal/models/tenant.go" << 'EOF'
package models

// Tenant represents a tenant/organization in the system
type Tenant struct {
	BaseModel
	Name         string `gorm:"not null" json:"name"`
	Slug         string `gorm:"unique;not null" json:"slug"`
	Domain       string `gorm:"unique" json:"domain"`
	Plan         string `gorm:"not null;default:'free'" json:"plan"`
	IsActive     bool   `gorm:"default:true" json:"is_active"`
	Settings     map[string]interface{} `gorm:"type:jsonb" json:"settings"`
	Subscription map[string]interface{} `gorm:"type:jsonb" json:"subscription"`
}

// TableName returns the table name for Tenant
func (Tenant) TableName() string {
	return "tenants"
}
EOF

    # Create notification models (partial from notification service)
    cat > "$PLATFORM_DIR/internal/models/notification.go" << 'EOF'
package models

// Notification represents a notification in the system
type Notification struct {
	TenantModel
	UserID     uint                   `gorm:"not null;index" json:"user_id"`
	User       User                   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Type       string                 `gorm:"not null" json:"type"`
	Title      string                 `gorm:"not null" json:"title"`
	Message    string                 `gorm:"not null" json:"message"`
	Data       map[string]interface{} `gorm:"type:jsonb" json:"data"`
	IsRead     bool                   `gorm:"default:false" json:"is_read"`
	ReadAt     *time.Time             `json:"read_at"`
	ExpiresAt  *time.Time             `json:"expires_at"`
}

// TableName returns the table name for Notification
func (Notification) TableName() string {
	return "notifications"
}

// NotificationChannel represents notification delivery channels
type NotificationChannel struct {
	BaseModel
	UserID    uint   `gorm:"not null;index" json:"user_id"`
	User      User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Type      string `gorm:"not null" json:"type"` // email, sms, push, webhook
	Address   string `gorm:"not null" json:"address"`
	IsActive  bool   `gorm:"default:true" json:"is_active"`
	Verified  bool   `gorm:"default:false" json:"verified"`
	Settings  map[string]interface{} `gorm:"type:jsonb" json:"settings"`
}

// TableName returns the table name for NotificationChannel
func (NotificationChannel) TableName() string {
	return "notification_channels"
}
EOF

    print_success "Models consolidated"
}

# Function to consolidate services
consolidate_services() {
    print_status "Consolidating service layer..."
    
    # Create user service
    cat > "$PLATFORM_DIR/internal/services/user_service.go" << 'EOF'
package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"golang.org/x/crypto/bcrypt"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
)

type UserService struct {
	db     *gorm.DB
	jwtKey []byte
}

func NewUserService(db *gorm.DB, jwtKey []byte) *UserService {
	return &UserService{
		db:     db,
		jwtKey: jwtKey,
	}
}

// CreateUser creates a new user
func (s *UserService) CreateUser(ctx context.Context, user *models.User) error {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	user.Password = string(hashedPassword)

	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Preload("Tenant").First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// AuthenticateUser authenticates a user by email and password
func (s *UserService) AuthenticateUser(ctx context.Context, email, password string) (*models.User, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

// GenerateJWT generates a JWT token for a user
func (s *UserService) GenerateJWT(userID uint, tenantID uint) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":   userID,
		"tenant_id": tenantID,
		"exp":       time.Now().Add(time.Hour * 24).Unix(),
	})

	return token.SignedString(s.jwtKey)
}

// ValidateJWT validates a JWT token
func (s *UserService) ValidateJWT(tokenString string) (uint, uint, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return s.jwtKey, nil
	})

	if err != nil {
		return 0, 0, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID := uint(claims["user_id"].(float64))
		tenantID := uint(claims["tenant_id"].(float64))
		return userID, tenantID, nil
	}

	return 0, 0, fmt.Errorf("invalid token")
}

// UpdateUser updates a user
func (s *UserService) UpdateUser(ctx context.Context, user *models.User) error {
	if err := s.db.WithContext(ctx).Save(user).Error; err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// DeleteUser soft deletes a user
func (s *UserService) DeleteUser(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&models.User{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// ListUsers lists users with pagination
func (s *UserService) ListUsers(ctx context.Context, tenantID uint, limit, offset int) ([]*models.User, error) {
	var users []*models.User
	query := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	return users, nil
}
EOF

    # Create workspace service
    cat > "$PLATFORM_DIR/internal/services/workspace_service.go" << 'EOF'
package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
)

type WorkspaceService struct {
	db *gorm.DB
}

func NewWorkspaceService(db *gorm.DB) *WorkspaceService {
	return &WorkspaceService{db: db}
}

// CreateWorkspace creates a new workspace
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create workspace
	if err := tx.Create(workspace).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// Add owner as workspace member
	member := &models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      workspace.OwnerID,
		Role:        "owner",
	}

	if err := tx.Create(member).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to add owner as member: %w", err)
	}

	return tx.Commit().Error
}

// GetWorkspaceByID retrieves a workspace by ID
func (s *WorkspaceService) GetWorkspaceByID(ctx context.Context, id uint) (*models.Workspace, error) {
	var workspace models.Workspace
	if err := s.db.WithContext(ctx).Preload("Owner").First(&workspace, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workspace not found")
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	return &workspace, nil
}

// ListWorkspaces lists workspaces for a tenant
func (s *WorkspaceService) ListWorkspaces(ctx context.Context, tenantID uint, limit, offset int) ([]*models.Workspace, error) {
	var workspaces []*models.Workspace
	query := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Preload("Owner")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&workspaces).Error; err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	return workspaces, nil
}

// UpdateWorkspace updates a workspace
func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	if err := s.db.WithContext(ctx).Save(workspace).Error; err != nil {
		return fmt.Errorf("failed to update workspace: %w", err)
	}
	return nil
}

// DeleteWorkspace soft deletes a workspace
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&models.Workspace{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}
	return nil
}

// AddMember adds a user to a workspace
func (s *WorkspaceService) AddMember(ctx context.Context, member *models.WorkspaceMember) error {
	if err := s.db.WithContext(ctx).Create(member).Error; err != nil {
		return fmt.Errorf("failed to add workspace member: %w", err)
	}
	return nil
}

// RemoveMember removes a user from a workspace
func (s *WorkspaceService) RemoveMember(ctx context.Context, workspaceID, userID uint) error {
	if err := s.db.WithContext(ctx).Where("workspace_id = ? AND user_id = ?", workspaceID, userID).Delete(&models.WorkspaceMember{}).Error; err != nil {
		return fmt.Errorf("failed to remove workspace member: %w", err)
	}
	return nil
}

// ListMembers lists workspace members
func (s *WorkspaceService) ListMembers(ctx context.Context, workspaceID uint) ([]*models.WorkspaceMember, error) {
	var members []*models.WorkspaceMember
	if err := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Preload("User").Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to list workspace members: %w", err)
	}
	return members, nil
}

// GetUserWorkspaces gets all workspaces for a user
func (s *WorkspaceService) GetUserWorkspaces(ctx context.Context, userID uint) ([]*models.Workspace, error) {
	var workspaces []*models.Workspace
	if err := s.db.WithContext(ctx).
		Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.user_id = ?", userID).
		Preload("Owner").
		Find(&workspaces).Error; err != nil {
		return nil, fmt.Errorf("failed to get user workspaces: %w", err)
	}
	return workspaces, nil
}
EOF

    # Create tenant service
    cat > "$PLATFORM_DIR/internal/services/tenant_service.go" << 'EOF'
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
)

type TenantService struct {
	db *gorm.DB
}

func NewTenantService(db *gorm.DB) *TenantService {
	return &TenantService{db: db}
}

// CreateTenant creates a new tenant
func (s *TenantService) CreateTenant(ctx context.Context, tenant *models.Tenant) error {
	// Generate slug if not provided
	if tenant.Slug == "" {
		tenant.Slug = s.generateSlug(tenant.Name)
	}

	if err := s.db.WithContext(ctx).Create(tenant).Error; err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

// GetTenantByID retrieves a tenant by ID
func (s *TenantService) GetTenantByID(ctx context.Context, id uint) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := s.db.WithContext(ctx).First(&tenant, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// GetTenantBySlug retrieves a tenant by slug
func (s *TenantService) GetTenantBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// GetTenantByDomain retrieves a tenant by domain
func (s *TenantService) GetTenantByDomain(ctx context.Context, domain string) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := s.db.WithContext(ctx).Where("domain = ?", domain).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// UpdateTenant updates a tenant
func (s *TenantService) UpdateTenant(ctx context.Context, tenant *models.Tenant) error {
	if err := s.db.WithContext(ctx).Save(tenant).Error; err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}
	return nil
}

// DeleteTenant soft deletes a tenant
func (s *TenantService) DeleteTenant(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&models.Tenant{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	return nil
}

// ListTenants lists all tenants with pagination
func (s *TenantService) ListTenants(ctx context.Context, limit, offset int) ([]*models.Tenant, error) {
	var tenants []*models.Tenant
	query := s.db.WithContext(ctx)
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&tenants).Error; err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	return tenants, nil
}

// generateSlug generates a URL-friendly slug from tenant name
func (s *TenantService) generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}

// ValidateSlug checks if a slug is available
func (s *TenantService) ValidateSlug(ctx context.Context, slug string) bool {
	var count int64
	s.db.WithContext(ctx).Model(&models.Tenant{}).Where("slug = ?", slug).Count(&count)
	return count == 0
}
EOF

    # Create notification service (partial)
    cat > "$PLATFORM_DIR/internal/services/notification_service.go" << 'EOF'
package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
)

type NotificationService struct {
	db *gorm.DB
}

func NewNotificationService(db *gorm.DB) *NotificationService {
	return &NotificationService{db: db}
}

// CreateNotification creates a new notification
func (s *NotificationService) CreateNotification(ctx context.Context, notification *models.Notification) error {
	if err := s.db.WithContext(ctx).Create(notification).Error; err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}
	return nil
}

// GetUserNotifications gets notifications for a user
func (s *NotificationService) GetUserNotifications(ctx context.Context, userID uint, limit, offset int) ([]*models.Notification, error) {
	var notifications []*models.Notification
	query := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&notifications).Error; err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	return notifications, nil
}

// MarkAsRead marks a notification as read
func (s *NotificationService) MarkAsRead(ctx context.Context, notificationID uint) error {
	if err := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("id = ?", notificationID).
		Update("is_read", true).Error; err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}
	return nil
}

// GetUnreadCount gets the count of unread notifications for a user
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID uint) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to get unread count: %w", err)
	}
	return count, nil
}
EOF

    # Create service container
    cat > "$PLATFORM_DIR/internal/services/services.go" << 'EOF'
package services

import (
	"gorm.io/gorm"
)

// Services contains all service dependencies
type Services struct {
	User         *UserService
	Workspace    *WorkspaceService
	Tenant       *TenantService
	Notification *NotificationService
}

// NewServices creates a new service container
func NewServices(db *gorm.DB, jwtKey []byte) *Services {
	return &Services{
		User:         NewUserService(db, jwtKey),
		Workspace:    NewWorkspaceService(db),
		Tenant:       NewTenantService(db),
		Notification: NewNotificationService(db),
	}
}
EOF

    print_success "Services consolidated"
}

# Function to consolidate handlers
consolidate_handlers() {
    print_status "Consolidating handlers..."
    
    # Create user handlers
    cat > "$PLATFORM_DIR/internal/handlers/user.go" << 'EOF'
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

type UserHandler struct {
	userService *services.UserService
	logger      *slog.Logger
}

func NewUserHandler(userService *services.UserService, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

// CreateUser handles user creation
func (h *UserHandler) CreateUser(c fiber.Ctx) error {
	var user models.User
	if err := c.Bind().Body(&user); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Get tenant ID from context
	tenantID := c.Locals("tenantID").(uint)
	user.TenantID = tenantID

	if err := h.userService.CreateUser(c.Context(), &user); err != nil {
		h.logger.Error("Failed to create user", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
	}

	return c.Status(201).JSON(user)
}

// GetUser handles getting a user by ID
func (h *UserHandler) GetUser(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	user, err := h.userService.GetUserByID(c.Context(), uint(id))
	if err != nil {
		h.logger.Error("Failed to get user", "error", err)
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	return c.JSON(user)
}

// UpdateUser handles user updates
func (h *UserHandler) UpdateUser(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var user models.User
	if err := c.Bind().Body(&user); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	user.ID = uint(id)
	if err := h.userService.UpdateUser(c.Context(), &user); err != nil {
		h.logger.Error("Failed to update user", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update user"})
	}

	return c.JSON(user)
}

// DeleteUser handles user deletion
func (h *UserHandler) DeleteUser(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	if err := h.userService.DeleteUser(c.Context(), uint(id)); err != nil {
		h.logger.Error("Failed to delete user", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete user"})
	}

	return c.SendStatus(204)
}

// ListUsers handles listing users
func (h *UserHandler) ListUsers(c fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(uint)
	
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	users, err := h.userService.ListUsers(c.Context(), tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list users", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list users"})
	}

	return c.JSON(fiber.Map{
		"users":  users,
		"limit":  limit,
		"offset": offset,
	})
}

// Login handles user authentication
func (h *UserHandler) Login(c fiber.Ctx) error {
	var loginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.Bind().Body(&loginReq); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	user, err := h.userService.AuthenticateUser(c.Context(), loginReq.Email, loginReq.Password)
	if err != nil {
		h.logger.Error("Authentication failed", "error", err)
		return c.Status(401).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	token, err := h.userService.GenerateJWT(user.ID, user.TenantID)
	if err != nil {
		h.logger.Error("Failed to generate token", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	return c.JSON(fiber.Map{
		"user":  user,
		"token": token,
	})
}
EOF

    # Create workspace handlers
    cat > "$PLATFORM_DIR/internal/handlers/workspace.go" << 'EOF'
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

type WorkspaceHandler struct {
	workspaceService *services.WorkspaceService
	logger           *slog.Logger
}

func NewWorkspaceHandler(workspaceService *services.WorkspaceService, logger *slog.Logger) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService: workspaceService,
		logger:           logger,
	}
}

// CreateWorkspace handles workspace creation
func (h *WorkspaceHandler) CreateWorkspace(c fiber.Ctx) error {
	var workspace models.Workspace
	if err := c.Bind().Body(&workspace); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Get user and tenant from context
	userID := c.Locals("userID").(uint)
	tenantID := c.Locals("tenantID").(uint)
	
	workspace.OwnerID = userID
	workspace.TenantID = tenantID

	if err := h.workspaceService.CreateWorkspace(c.Context(), &workspace); err != nil {
		h.logger.Error("Failed to create workspace", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create workspace"})
	}

	return c.Status(201).JSON(workspace)
}

// GetWorkspace handles getting a workspace by ID
func (h *WorkspaceHandler) GetWorkspace(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	workspace, err := h.workspaceService.GetWorkspaceByID(c.Context(), uint(id))
	if err != nil {
		h.logger.Error("Failed to get workspace", "error", err)
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	return c.JSON(workspace)
}

// UpdateWorkspace handles workspace updates
func (h *WorkspaceHandler) UpdateWorkspace(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	var workspace models.Workspace
	if err := c.Bind().Body(&workspace); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	workspace.ID = uint(id)
	if err := h.workspaceService.UpdateWorkspace(c.Context(), &workspace); err != nil {
		h.logger.Error("Failed to update workspace", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update workspace"})
	}

	return c.JSON(workspace)
}

// DeleteWorkspace handles workspace deletion
func (h *WorkspaceHandler) DeleteWorkspace(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	if err := h.workspaceService.DeleteWorkspace(c.Context(), uint(id)); err != nil {
		h.logger.Error("Failed to delete workspace", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete workspace"})
	}

	return c.SendStatus(204)
}

// ListWorkspaces handles listing workspaces
func (h *WorkspaceHandler) ListWorkspaces(c fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(uint)
	
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	workspaces, err := h.workspaceService.ListWorkspaces(c.Context(), tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list workspaces", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list workspaces"})
	}

	return c.JSON(fiber.Map{
		"workspaces": workspaces,
		"limit":      limit,
		"offset":     offset,
	})
}

// GetUserWorkspaces handles getting workspaces for a user
func (h *WorkspaceHandler) GetUserWorkspaces(c fiber.Ctx) error {
	userID := c.Locals("userID").(uint)

	workspaces, err := h.workspaceService.GetUserWorkspaces(c.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user workspaces", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get user workspaces"})
	}

	return c.JSON(fiber.Map{"workspaces": workspaces})
}

// AddMember handles adding a member to workspace
func (h *WorkspaceHandler) AddMember(c fiber.Ctx) error {
	idStr := c.Params("id")
	workspaceID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	var member models.WorkspaceMember
	if err := c.Bind().Body(&member); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	member.WorkspaceID = uint(workspaceID)

	if err := h.workspaceService.AddMember(c.Context(), &member); err != nil {
		h.logger.Error("Failed to add workspace member", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to add workspace member"})
	}

	return c.Status(201).JSON(member)
}

// ListMembers handles listing workspace members
func (h *WorkspaceHandler) ListMembers(c fiber.Ctx) error {
	idStr := c.Params("id")
	workspaceID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid workspace ID"})
	}

	members, err := h.workspaceService.ListMembers(c.Context(), uint(workspaceID))
	if err != nil {
		h.logger.Error("Failed to list workspace members", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list workspace members"})
	}

	return c.JSON(fiber.Map{"members": members})
}
EOF

    # Create tenant handlers
    cat > "$PLATFORM_DIR/internal/handlers/tenant.go" << 'EOF'
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

type TenantHandler struct {
	tenantService *services.TenantService
	logger        *slog.Logger
}

func NewTenantHandler(tenantService *services.TenantService, logger *slog.Logger) *TenantHandler {
	return &TenantHandler{
		tenantService: tenantService,
		logger:        logger,
	}
}

// CreateTenant handles tenant creation
func (h *TenantHandler) CreateTenant(c fiber.Ctx) error {
	var tenant models.Tenant
	if err := c.Bind().Body(&tenant); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := h.tenantService.CreateTenant(c.Context(), &tenant); err != nil {
		h.logger.Error("Failed to create tenant", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create tenant"})
	}

	return c.Status(201).JSON(tenant)
}

// GetTenant handles getting a tenant by ID
func (h *TenantHandler) GetTenant(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	tenant, err := h.tenantService.GetTenantByID(c.Context(), uint(id))
	if err != nil {
		h.logger.Error("Failed to get tenant", "error", err)
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	return c.JSON(tenant)
}

// GetTenantBySlug handles getting a tenant by slug
func (h *TenantHandler) GetTenantBySlug(c fiber.Ctx) error {
	slug := c.Params("slug")

	tenant, err := h.tenantService.GetTenantBySlug(c.Context(), slug)
	if err != nil {
		h.logger.Error("Failed to get tenant by slug", "error", err)
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	return c.JSON(tenant)
}

// UpdateTenant handles tenant updates
func (h *TenantHandler) UpdateTenant(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var tenant models.Tenant
	if err := c.Bind().Body(&tenant); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	tenant.ID = uint(id)
	if err := h.tenantService.UpdateTenant(c.Context(), &tenant); err != nil {
		h.logger.Error("Failed to update tenant", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update tenant"})
	}

	return c.JSON(tenant)
}

// DeleteTenant handles tenant deletion
func (h *TenantHandler) DeleteTenant(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	if err := h.tenantService.DeleteTenant(c.Context(), uint(id)); err != nil {
		h.logger.Error("Failed to delete tenant", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete tenant"})
	}

	return c.SendStatus(204)
}

// ListTenants handles listing tenants
func (h *TenantHandler) ListTenants(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	tenants, err := h.tenantService.ListTenants(c.Context(), limit, offset)
	if err != nil {
		h.logger.Error("Failed to list tenants", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list tenants"})
	}

	return c.JSON(fiber.Map{
		"tenants": tenants,
		"limit":   limit,
		"offset":  offset,
	})
}

// ValidateSlug handles slug validation
func (h *TenantHandler) ValidateSlug(c fiber.Ctx) error {
	slug := c.Query("slug")
	if slug == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Slug parameter required"})
	}

	available := h.tenantService.ValidateSlug(c.Context(), slug)
	return c.JSON(fiber.Map{
		"slug":      slug,
		"available": available,
	})
}
EOF

    # Create notification handlers (partial)
    cat > "$PLATFORM_DIR/internal/handlers/notification.go" << 'EOF'
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-platform/internal/models"
	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

type NotificationHandler struct {
	notificationService *services.NotificationService
	logger              *slog.Logger
}

func NewNotificationHandler(notificationService *services.NotificationService, logger *slog.Logger) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
		logger:              logger,
	}
}

// CreateNotification handles notification creation
func (h *NotificationHandler) CreateNotification(c fiber.Ctx) error {
	var notification models.Notification
	if err := c.Bind().Body(&notification); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Get tenant ID from context
	tenantID := c.Locals("tenantID").(uint)
	notification.TenantID = tenantID

	if err := h.notificationService.CreateNotification(c.Context(), &notification); err != nil {
		h.logger.Error("Failed to create notification", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create notification"})
	}

	return c.Status(201).JSON(notification)
}

// GetUserNotifications handles getting notifications for a user
func (h *NotificationHandler) GetUserNotifications(c fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	notifications, err := h.notificationService.GetUserNotifications(c.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get notifications", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get notifications"})
	}

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"limit":         limit,
		"offset":        offset,
	})
}

// MarkNotificationAsRead handles marking a notification as read
func (h *NotificationHandler) MarkNotificationAsRead(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid notification ID"})
	}

	if err := h.notificationService.MarkAsRead(c.Context(), uint(id)); err != nil {
		h.logger.Error("Failed to mark notification as read", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to mark notification as read"})
	}

	return c.SendStatus(204)
}

// GetUnreadCount handles getting unread notification count
func (h *NotificationHandler) GetUnreadCount(c fiber.Ctx) error {
	userID := c.Locals("userID").(uint)

	count, err := h.notificationService.GetUnreadCount(c.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get unread count", "error", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get unread count"})
	}

	return c.JSON(fiber.Map{"unread_count": count})
}
EOF

    # Create main handler container
    cat > "$PLATFORM_DIR/internal/handlers/handlers.go" << 'EOF'
package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

type Handlers struct {
	User         *UserHandler
	Workspace    *WorkspaceHandler
	Tenant       *TenantHandler
	Notification *NotificationHandler
	logger       *slog.Logger
}

func NewHandlers(services *services.Services, logger *slog.Logger) *Handlers {
	return &Handlers{
		User:         NewUserHandler(services.User, logger),
		Workspace:    NewWorkspaceHandler(services.Workspace, logger),
		Tenant:       NewTenantHandler(services.Tenant, logger),
		Notification: NewNotificationHandler(services.Notification, logger),
		logger:       logger,
	}
}

// Health handles health check requests
func (h *Handlers) Health(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"service":   "pyairtable-platform",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	})
}
EOF

    print_success "Handlers consolidated"
}

# Function to create main server
create_main_server() {
    print_status "Creating main server..."
    
    cat > "$PLATFORM_DIR/cmd/server/main.go" << 'EOF'
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Reg-Kris/pyairtable-platform/internal/config"
	"github.com/Reg-Kris/pyairtable-platform/internal/handlers"
	"github.com/Reg-Kris/pyairtable-platform/internal/middleware"
	"github.com/Reg-Kris/pyairtable-platform/internal/models"
	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Connect to database
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}

	// Auto-migrate models
	if err := migrateModels(db); err != nil {
		logger.Error("Failed to migrate models", "error", err)
		os.Exit(1)
	}

	// Initialize services
	svcs := services.NewServices(db, []byte(cfg.JWTSecret))

	// Initialize handlers
	h := handlers.NewHandlers(svcs, logger)

	// Setup Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Setup routes
	setupRoutes(app, h, svcs.User)

	// Graceful shutdown
	go func() {
		sigterm := make(chan os.Signal, 1)
		signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)
		<-sigterm

		logger.Info("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := app.ShutdownWithContext(ctx); err != nil {
			logger.Error("Server shutdown error", "error", err)
		}
	}()

	// Start server
	logger.Info("Starting PyAirtable Platform Service", "port", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		logger.Error("Server failed to start", "error", err)
	}
}

func migrateModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.UserProfile{},
		&models.UserSession{},
		&models.Workspace{},
		&models.WorkspaceMember{},
		&models.Notification{},
		&models.NotificationChannel{},
	)
}

func setupRoutes(app *fiber.App, h *handlers.Handlers, userService *services.UserService) {
	// Health check
	app.Get("/health", h.Health)

	// API v1
	v1 := app.Group("/api/v1")

	// Public routes
	public := v1.Group("/public")
	public.Post("/login", h.User.Login)
	public.Get("/tenants/validate-slug", h.Tenant.ValidateSlug)

	// Protected routes (require authentication)
	protected := v1.Group("/")
	protected.Use(middleware.AuthMiddleware(userService))

	// User routes
	users := protected.Group("/users")
	users.Get("/", h.User.ListUsers)
	users.Post("/", h.User.CreateUser)
	users.Get("/:id", h.User.GetUser)
	users.Put("/:id", h.User.UpdateUser)
	users.Delete("/:id", h.User.DeleteUser)

	// Workspace routes
	workspaces := protected.Group("/workspaces")
	workspaces.Get("/", h.Workspace.ListWorkspaces)
	workspaces.Post("/", h.Workspace.CreateWorkspace)
	workspaces.Get("/user", h.Workspace.GetUserWorkspaces)
	workspaces.Get("/:id", h.Workspace.GetWorkspace)
	workspaces.Put("/:id", h.Workspace.UpdateWorkspace)
	workspaces.Delete("/:id", h.Workspace.DeleteWorkspace)
	workspaces.Post("/:id/members", h.Workspace.AddMember)
	workspaces.Get("/:id/members", h.Workspace.ListMembers)

	// Tenant routes (admin only - would add admin middleware)
	tenants := protected.Group("/tenants")
	tenants.Get("/", h.Tenant.ListTenants)
	tenants.Post("/", h.Tenant.CreateTenant)
	tenants.Get("/:id", h.Tenant.GetTenant)
	tenants.Get("/slug/:slug", h.Tenant.GetTenantBySlug)
	tenants.Put("/:id", h.Tenant.UpdateTenant)
	tenants.Delete("/:id", h.Tenant.DeleteTenant)

	// Notification routes
	notifications := protected.Group("/notifications")
	notifications.Get("/", h.Notification.GetUserNotifications)
	notifications.Post("/", h.Notification.CreateNotification)
	notifications.Put("/:id/read", h.Notification.MarkNotificationAsRead)
	notifications.Get("/unread-count", h.Notification.GetUnreadCount)
}
EOF

    print_success "Main server created"
}

# Function to create configuration
create_config() {
    print_status "Creating configuration..."
    
    cat > "$PLATFORM_DIR/internal/config/config.go" << 'EOF'
package config

import (
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	RedisURL    string
	LogLevel    string
}

func Load() (*Config, error) {
	return &Config{
		Port:        getEnv("PORT", "8007"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/pyairtable?sslmode=disable"),
		JWTSecret:   getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
EOF

    print_success "Configuration created"
}

# Function to create middleware
create_middleware() {
    print_status "Creating middleware..."
    
    cat > "$PLATFORM_DIR/internal/middleware/auth.go" << 'EOF'
package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/Reg-Kris/pyairtable-platform/internal/services"
)

func AuthMiddleware(userService *services.UserService) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Get Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "Authorization header required"})
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid authorization header format"})
		}

		token := parts[1]

		// Validate token
		userID, tenantID, err := userService.ValidateJWT(token)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid token"})
		}

		// Set user and tenant in context
		c.Locals("userID", userID)
		c.Locals("tenantID", tenantID)

		return c.Next()
	}
}

func TenantMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Extract tenant from subdomain or header
		tenant := c.Get("X-Tenant-ID")
		if tenant == "" {
			// Try to extract from subdomain
			host := c.Get("Host")
			parts := strings.Split(host, ".")
			if len(parts) > 1 {
				tenant = parts[0]
			}
		}

		if tenant != "" {
			c.Locals("tenant", tenant)
		}

		return c.Next()
	}
}
EOF

    print_success "Middleware created"
}

# Function to create go.mod
create_go_mod() {
    print_status "Creating go.mod..."
    
    cat > "$PLATFORM_DIR/go.mod" << 'EOF'
module github.com/Reg-Kris/pyairtable-platform

go 1.21

require (
	github.com/gofiber/fiber/v3 v3.0.0-beta.2
	github.com/golang-jwt/jwt/v5 v5.0.0
	golang.org/x/crypto v0.19.0
	gorm.io/driver/postgres v1.5.2
	gorm.io/gorm v1.30.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/bytedance/sonic v1.9.1 // indirect
	github.com/chenzhuoyu/base64x v0.0.0-20221115062448-fe3a3abad311 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.14.0 // indirect
	github.com/gofiber/utils/v2 v2.0.0-beta.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.4.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/leodido/go-urn v1.2.4 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.11 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.52.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	golang.org/x/arch v0.3.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.20.0 // indirect
)
EOF

    print_success "go.mod created"
}

# Function to create docker and deployment files
create_deployment_files() {
    print_status "Creating deployment files..."
    
    # Dockerfile
    cat > "$PLATFORM_DIR/Dockerfile" << 'EOF'
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/server/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Expose port
EXPOSE 8007

# Command to run
CMD ["./main"]
EOF

    # Docker Compose
    cat > "$PLATFORM_DIR/docker-compose.yml" << 'EOF'
version: '3.8'

services:
  platform:
    build: .
    ports:
      - "8007:8007"
    environment:
      - PORT=8007
      - DATABASE_URL=postgres://postgres:postgres@postgres:5432/pyairtable?sslmode=disable
      - JWT_SECRET=your-secret-key-change-in-production
      - REDIS_URL=redis://redis:6379
      - LOG_LEVEL=info
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_DB=pyairtable
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    restart: unless-stopped

volumes:
  postgres_data:
EOF

    # Makefile
    cat > "$PLATFORM_DIR/Makefile" << 'EOF'
.PHONY: build run test clean docker-build docker-run

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=pyairtable-platform
BINARY_UNIX=$(BINARY_NAME)_unix

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) cmd/server/main.go

# Run the application
run:
	$(GOBUILD) -o $(BINARY_NAME) cmd/server/main.go
	./$(BINARY_NAME)

# Test the application
test:
	$(GOTEST) -v ./...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) cmd/server/main.go

# Docker commands
docker-build:
	docker build -t pyairtable-platform .

docker-run:
	docker-compose up --build

docker-stop:
	docker-compose down

# Development commands
dev:
	go run cmd/server/main.go

deps:
	go mod download
	go mod tidy

# Database migrations (if using migrate tool)
migrate-up:
	migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/pyairtable?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/pyairtable?sslmode=disable" down

# Lint
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Generate mocks (if using mockery)
mocks:
	mockery --all --output=test/mocks
EOF

    print_success "Deployment files created"
}

# Function to create package client
create_package_client() {
    print_status "Creating package client..."
    
    cat > "$PLATFORM_DIR/pkg/client/client.go" << 'EOF'
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents the PyAirtable Platform API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// New creates a new client instance
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetToken sets the authentication token
func (c *Client) SetToken(token string) {
	c.token = token
}

// makeRequest makes an HTTP request to the API
func (c *Client) makeRequest(method, path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return c.httpClient.Do(req)
}

// UserService provides user-related API methods
type UserService struct {
	client *Client
}

// Users returns the user service
func (c *Client) Users() *UserService {
	return &UserService{client: c}
}

// User represents a user
type User struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	IsActive  bool   `json:"is_active"`
	TenantID  uint   `json:"tenant_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// List retrieves a list of users
func (s *UserService) List() ([]User, error) {
	resp, err := s.client.makeRequest("GET", "/api/v1/users", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Users []User `json:"users"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Users, nil
}

// Get retrieves a user by ID
func (s *UserService) Get(id uint) (*User, error) {
	resp, err := s.client.makeRequest("GET", fmt.Sprintf("/api/v1/users/%d", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &user, nil
}

// Create creates a new user
func (s *UserService) Create(user *User) error {
	resp, err := s.client.makeRequest("POST", "/api/v1/users", user)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create user: status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(user)
}

// Login authenticates a user
func (s *UserService) Login(email, password string) (string, *User, error) {
	loginReq := map[string]string{
		"email":    email,
		"password": password,
	}

	resp, err := s.client.makeRequest("POST", "/api/v1/public/login", loginReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("login failed: status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
		User  User   `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Token, &result.User, nil
}

// WorkspaceService provides workspace-related API methods
type WorkspaceService struct {
	client *Client
}

// Workspaces returns the workspace service
func (c *Client) Workspaces() *WorkspaceService {
	return &WorkspaceService{client: c}
}

// Workspace represents a workspace
type Workspace struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     uint   `json:"owner_id"`
	TenantID    uint   `json:"tenant_id"`
	IsActive    bool   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// List retrieves a list of workspaces
func (s *WorkspaceService) List() ([]Workspace, error) {
	resp, err := s.client.makeRequest("GET", "/api/v1/workspaces", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Workspaces []Workspace `json:"workspaces"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Workspaces, nil
}

// Get retrieves a workspace by ID
func (s *WorkspaceService) Get(id uint) (*Workspace, error) {
	resp, err := s.client.makeRequest("GET", fmt.Sprintf("/api/v1/workspaces/%d", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var workspace Workspace
	if err := json.NewDecoder(resp.Body).Decode(&workspace); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &workspace, nil
}

// Create creates a new workspace
func (s *WorkspaceService) Create(workspace *Workspace) error {
	resp, err := s.client.makeRequest("POST", "/api/v1/workspaces", workspace)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create workspace: status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(workspace)
}

// TenantService provides tenant-related API methods
type TenantService struct {
	client *Client
}

// Tenants returns the tenant service
func (c *Client) Tenants() *TenantService {
	return &TenantService{client: c}
}

// Tenant represents a tenant
type Tenant struct {
	ID           uint                   `json:"id"`
	Name         string                 `json:"name"`
	Slug         string                 `json:"slug"`
	Domain       string                 `json:"domain"`
	Plan         string                 `json:"plan"`
	IsActive     bool                   `json:"is_active"`
	Settings     map[string]interface{} `json:"settings"`
	Subscription map[string]interface{} `json:"subscription"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

// Get retrieves a tenant by ID
func (s *TenantService) Get(id uint) (*Tenant, error) {
	resp, err := s.client.makeRequest("GET", fmt.Sprintf("/api/v1/tenants/%d", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tenant Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tenant, nil
}

// GetBySlug retrieves a tenant by slug
func (s *TenantService) GetBySlug(slug string) (*Tenant, error) {
	resp, err := s.client.makeRequest("GET", fmt.Sprintf("/api/v1/tenants/slug/%s", slug), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tenant Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tenant, nil
}
EOF

    print_success "Package client created"
}

# Function to create README
create_readme() {
    print_status "Creating README..."
    
    cat > "$PLATFORM_DIR/README.md" << 'EOF'
# PyAirtable Platform Service

A consolidated Go service that combines user management, workspace management, tenant management, and notification services into a single, efficient platform service.

## Features

- **User Management**: Complete user lifecycle management with authentication
- **Workspace Management**: Multi-tenant workspace and membership management
- **Tenant Management**: Organization/tenant isolation and management
- **Notification System**: User notification delivery and management
- **JWT Authentication**: Secure token-based authentication
- **Database Integration**: PostgreSQL with GORM ORM
- **RESTful API**: Clean, consistent REST endpoints
- **Containerized**: Docker and Docker Compose support

## Architecture

```
pyairtable-platform/
├── cmd/server/           # Application entry point
├── internal/
│   ├── handlers/         # HTTP handlers for each domain
│   ├── services/         # Business logic layer
│   ├── models/          # Data models and database schemas
│   ├── config/          # Configuration management
│   └── middleware/      # HTTP middleware
├── pkg/client/          # Go client library for the API
├── configs/             # Configuration files
├── deployments/         # Deployment configurations
└── test/               # Test files and fixtures
```

## Quick Start

### Prerequisites

- Go 1.21 or later
- PostgreSQL 12 or later
- Redis (optional, for caching)
- Docker and Docker Compose (for containerized deployment)

### Environment Variables

```bash
PORT=8080
DATABASE_URL=postgres://postgres:postgres@localhost:5432/pyairtable?sslmode=disable
JWT_SECRET=your-secret-key-change-in-production
REDIS_URL=redis://localhost:6379
LOG_LEVEL=info
```

### Local Development

1. Clone and navigate to the project:
```bash
cd pyairtable-platform
```

2. Install dependencies:
```bash
go mod download
```

3. Set up your database and environment variables

4. Run the application:
```bash
make run
```

### Docker Development

```bash
make docker-run
```

This will start:
- PyAirtable Platform service on port 8080
- PostgreSQL on port 5432
- Redis on port 6379

## API Endpoints

### Authentication
- `POST /api/v1/public/login` - User login

### Users
- `GET /api/v1/users` - List users
- `POST /api/v1/users` - Create user
- `GET /api/v1/users/:id` - Get user
- `PUT /api/v1/users/:id` - Update user
- `DELETE /api/v1/users/:id` - Delete user

### Workspaces
- `GET /api/v1/workspaces` - List workspaces
- `POST /api/v1/workspaces` - Create workspace
- `GET /api/v1/workspaces/:id` - Get workspace
- `PUT /api/v1/workspaces/:id` - Update workspace
- `DELETE /api/v1/workspaces/:id` - Delete workspace
- `GET /api/v1/workspaces/user` - Get user's workspaces
- `POST /api/v1/workspaces/:id/members` - Add workspace member
- `GET /api/v1/workspaces/:id/members` - List workspace members

### Tenants
- `GET /api/v1/tenants` - List tenants
- `POST /api/v1/tenants` - Create tenant
- `GET /api/v1/tenants/:id` - Get tenant
- `GET /api/v1/tenants/slug/:slug` - Get tenant by slug
- `PUT /api/v1/tenants/:id` - Update tenant
- `DELETE /api/v1/tenants/:id` - Delete tenant
- `GET /api/v1/public/tenants/validate-slug` - Validate tenant slug

### Notifications
- `GET /api/v1/notifications` - Get user notifications
- `POST /api/v1/notifications` - Create notification
- `PUT /api/v1/notifications/:id/read` - Mark notification as read
- `GET /api/v1/notifications/unread-count` - Get unread count

## Using the Go Client

```go
package main

import (
    "log"
    "github.com/Reg-Kris/pyairtable-platform/pkg/client"
)

func main() {
    // Create client
    c := client.New("http://localhost:8080", "")
    
    // Login
    token, user, err := c.Users().Login("user@example.com", "password")
    if err != nil {
        log.Fatal(err)
    }
    
    // Set token for authenticated requests
    c.SetToken(token)
    
    // List workspaces
    workspaces, err := c.Workspaces().List()
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Found %d workspaces", len(workspaces))
}
```

## Database Schema

The service uses the following main entities:

- **Tenants**: Organizations/companies using the platform
- **Users**: Individual users belonging to tenants
- **Workspaces**: Project spaces within tenants
- **WorkspaceMembers**: User membership in workspaces
- **Notifications**: User notifications
- **UserSessions**: Active user sessions

## Development

### Running Tests
```bash
make test
```

### Building
```bash
make build
```

### Linting
```bash
make lint
```

### Formatting
```bash
make fmt
```

## Deployment

### Docker
```bash
make docker-build
docker run -p 8080:8080 pyairtable-platform
```

### Kubernetes
See the `deployments/k8s/` directory for Kubernetes manifests.

## Migration from Separate Services

This consolidated service replaces the following individual services:
- user-service
- workspace-service  
- tenant-service
- notification-service (partial)

See the migration guide for detailed steps on transitioning from the separate services to this consolidated platform.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run the test suite
6. Create a pull request

## License

This project is licensed under the MIT License.
EOF

    print_success "README created"
}

# Main execution
main() {
    print_status "Starting PyAirtable Platform Service Consolidation"
    
    # Check if services exist
    for service in "${SERVICES_TO_CONSOLIDATE[@]}"; do
        if ! check_service_exists "$service"; then
            print_error "Service $service not found. Exiting."
            exit 1
        fi
    done
    
    # Remove existing platform directory if it exists
    if [[ -d "$PLATFORM_DIR" ]]; then
        print_warning "Removing existing pyairtable-platform directory"
        rm -rf "$PLATFORM_DIR"
    fi
    
    # Create platform structure
    create_platform_structure
    
    # Consolidate components
    consolidate_models
    consolidate_services
    consolidate_handlers
    create_main_server
    create_config
    create_middleware
    create_go_mod
    create_deployment_files
    create_package_client
    create_readme
    
    print_success "PyAirtable Platform Service consolidation completed!"
    print_status "Next steps:"
    echo "1. cd pyairtable-platform"
    echo "2. go mod tidy"
    echo "3. Set up your environment variables"
    echo "4. Run: make docker-run"
    echo ""
    print_status "The consolidated service includes:"
    echo "- User management with authentication"
    echo "- Workspace and membership management"
    echo "- Tenant/organization management"
    echo "- Notification system (partial)"
    echo "- RESTful API with proper routing"
    echo "- Go client library"
    echo "- Docker deployment support"
}

main "$@"