package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"go.uber.org/zap"

	"github.com/Reg-Kris/pyairtable-permission-service/internal/models"
)

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

func New(cfg DatabaseConfig, zapLogger *zap.Logger) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.Name, cfg.Port, cfg.SSLMode,
	)

	// Configure GORM logger
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold: 200 * time.Millisecond,
			LogLevel:      logger.Info,
			Colorful:      false,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
		NamingStrategy: nil, // Use default naming strategy
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Set connection pool settings for permission service
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)  // Lower than main services since permissions are often cached
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	return db, nil
}

// AutoMigrate runs database migrations for permission service
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Role{},
		&models.Permission{},
		&models.UserRole{},
		&models.ResourcePermission{},
		&models.PermissionAuditLog{},
	)
}

// SeedDefaultPermissions creates default system permissions
func SeedDefaultPermissions(db *gorm.DB) error {
	defaultPermissions := []*models.Permission{
		// Workspace permissions
		{Name: "workspace:read", Description: "Read workspace information", ResourceType: models.ResourceTypeWorkspace, Action: models.ActionRead, IsActive: true},
		{Name: "workspace:update", Description: "Update workspace settings", ResourceType: models.ResourceTypeWorkspace, Action: models.ActionUpdate, IsActive: true},
		{Name: "workspace:delete", Description: "Delete workspace", ResourceType: models.ResourceTypeWorkspace, Action: models.ActionDelete, IsActive: true},
		{Name: "workspace:manage", Description: "Full workspace management", ResourceType: models.ResourceTypeWorkspace, Action: models.ActionManage, IsActive: true},
		{Name: "workspace:invite", Description: "Invite users to workspace", ResourceType: models.ResourceTypeWorkspace, Action: models.ActionInvite, IsActive: true},

		// Project permissions
		{Name: "project:read", Description: "Read project information", ResourceType: models.ResourceTypeProject, Action: models.ActionRead, IsActive: true},
		{Name: "project:create", Description: "Create new projects", ResourceType: models.ResourceTypeProject, Action: models.ActionCreate, IsActive: true},
		{Name: "project:update", Description: "Update project settings", ResourceType: models.ResourceTypeProject, Action: models.ActionUpdate, IsActive: true},
		{Name: "project:delete", Description: "Delete projects", ResourceType: models.ResourceTypeProject, Action: models.ActionDelete, IsActive: true},
		{Name: "project:manage", Description: "Full project management", ResourceType: models.ResourceTypeProject, Action: models.ActionManage, IsActive: true},

		// Airtable base permissions
		{Name: "airtable_base:read", Description: "Read Airtable base data", ResourceType: models.ResourceTypeAirtableBase, Action: models.ActionRead, IsActive: true},
		{Name: "airtable_base:create", Description: "Create Airtable base connections", ResourceType: models.ResourceTypeAirtableBase, Action: models.ActionCreate, IsActive: true},
		{Name: "airtable_base:update", Description: "Update Airtable base settings", ResourceType: models.ResourceTypeAirtableBase, Action: models.ActionUpdate, IsActive: true},
		{Name: "airtable_base:delete", Description: "Delete Airtable base connections", ResourceType: models.ResourceTypeAirtableBase, Action: models.ActionDelete, IsActive: true},
		{Name: "airtable_base:execute", Description: "Execute operations on Airtable bases", ResourceType: models.ResourceTypeAirtableBase, Action: models.ActionExecute, IsActive: true},

		// User permissions
		{Name: "user:read", Description: "Read user information", ResourceType: models.ResourceTypeUser, Action: models.ActionRead, IsActive: true},
		{Name: "user:view", Description: "View user profiles", ResourceType: models.ResourceTypeUser, Action: models.ActionView, IsActive: true},
		{Name: "user:update", Description: "Update user information", ResourceType: models.ResourceTypeUser, Action: models.ActionUpdate, IsActive: true},
		{Name: "user:manage", Description: "Full user management", ResourceType: models.ResourceTypeUser, Action: models.ActionManage, IsActive: true},

		// File permissions
		{Name: "file:read", Description: "Read file information", ResourceType: models.ResourceTypeFile, Action: models.ActionRead, IsActive: true},
		{Name: "file:upload", Description: "Upload files", ResourceType: models.ResourceTypeFile, Action: models.ActionUpload, IsActive: true},
		{Name: "file:download", Description: "Download files", ResourceType: models.ResourceTypeFile, Action: models.ActionDownload, IsActive: true},
		{Name: "file:delete", Description: "Delete files", ResourceType: models.ResourceTypeFile, Action: models.ActionDelete, IsActive: true},
		{Name: "file:share", Description: "Share files", ResourceType: models.ResourceTypeFile, Action: models.ActionShare, IsActive: true},

		// API permissions
		{Name: "api:roles:list", Description: "List roles", ResourceType: models.ResourceTypeAPI, Action: models.ActionList, IsActive: true},
		{Name: "api:roles:create", Description: "Create roles", ResourceType: models.ResourceTypeAPI, Action: models.ActionCreate, IsActive: true},
		{Name: "api:audit-logs:read", Description: "Read audit logs", ResourceType: models.ResourceTypeAPI, Action: models.ActionRead, IsActive: true},
	}

	for _, perm := range defaultPermissions {
		// Check if permission already exists
		var existing models.Permission
		result := db.Where("name = ?", perm.Name).First(&existing)
		if result.Error != nil {
			// Permission doesn't exist, create it
			if err := db.Create(perm).Error; err != nil {
				return fmt.Errorf("failed to create permission %s: %w", perm.Name, err)
			}
		}
	}

	return nil
}

// SeedDefaultRoles creates default system roles
func SeedDefaultRoles(db *gorm.DB) error {
	// Define default roles
	defaultRoles := []struct {
		Name        string
		Description string
		Permissions []string
	}{
		{
			Name:        "system_admin",
			Description: "System administrator with full access",
			Permissions: []string{
				"workspace:manage", "project:manage", "airtable_base:execute", "user:manage",
				"file:share", "api:roles:create", "api:audit-logs:read",
			},
		},
		{
			Name:        "workspace_admin",
			Description: "Workspace administrator",
			Permissions: []string{
				"workspace:manage", "workspace:invite", "project:manage", "airtable_base:execute", "user:view",
			},
		},
		{
			Name:        "project_admin",
			Description: "Project administrator",
			Permissions: []string{
				"project:manage", "airtable_base:execute", "file:share", "user:view",
			},
		},
		{
			Name:        "member",
			Description: "Standard member",
			Permissions: []string{
				"workspace:read", "project:read", "airtable_base:read", "file:download", "user:view",
			},
		},
		{
			Name:        "viewer",
			Description: "Read-only access",
			Permissions: []string{
				"workspace:read", "project:read", "airtable_base:read", "user:view",
			},
		},
	}

	for _, roleData := range defaultRoles {
		// Check if role already exists
		var existing models.Role
		result := db.Where("name = ? AND tenant_id IS NULL", roleData.Name).First(&existing)
		if result.Error != nil {
			// Role doesn't exist, create it
			role := &models.Role{
				Name:         roleData.Name,
				Description:  roleData.Description,
				IsSystemRole: true,
				IsActive:     true,
				TenantID:     nil, // System role
			}

			if err := db.Create(role).Error; err != nil {
				return fmt.Errorf("failed to create role %s: %w", roleData.Name, err)
			}

			// Assign permissions to role
			var permissions []*models.Permission
			db.Where("name IN ?", roleData.Permissions).Find(&permissions)
			
			if len(permissions) > 0 {
				db.Model(role).Association("Permissions").Replace(permissions)
			}
		}
	}

	return nil
}