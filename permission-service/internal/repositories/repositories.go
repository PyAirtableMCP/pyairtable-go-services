package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/Reg-Kris/pyairtable-permission-service/internal/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Repository interfaces
type PermissionRepository interface {
	GetByNames(ctx context.Context, names []string) ([]*models.Permission, error)
	List(ctx context.Context, filter *models.PermissionFilter) ([]*models.Permission, int64, error)
	Create(ctx context.Context, permission *models.Permission) error
	GetByID(ctx context.Context, id string) (*models.Permission, error)
}

type RoleRepository interface {
	GetByID(ctx context.Context, id string) (*models.Role, error)
	GetByName(ctx context.Context, name, tenantID string) (*models.Role, error)
	List(ctx context.Context, filter *models.RoleFilter) ([]*models.Role, int64, error)
	Create(ctx context.Context, role *models.Role) error
	Update(ctx context.Context, role *models.Role) error
	Delete(ctx context.Context, id string) error
	AssignPermissions(ctx context.Context, roleID string, permissions []*models.Permission) error
}

type UserRoleRepository interface {
	GetUserRoles(ctx context.Context, userID, tenantID, resourceType, resourceID string) ([]*models.UserRole, error)
	GetUserRole(ctx context.Context, userID, roleID, tenantID string, resourceType, resourceID *string) (*models.UserRole, error)
	Create(ctx context.Context, userRole *models.UserRole) error
	Delete(ctx context.Context, userID, roleID, tenantID string, resourceType, resourceID *string) error
	List(ctx context.Context, filter *models.UserRoleFilter) ([]*models.UserRole, int64, error)
}

type ResourcePermissionRepository interface {
	GetUserResourcePermissions(ctx context.Context, userID, tenantID, resourceType, resourceID string) ([]*models.ResourcePermission, error)
	GetByID(ctx context.Context, id string) (*models.ResourcePermission, error)
	Create(ctx context.Context, permission *models.ResourcePermission) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter interface{}) ([]*models.ResourcePermission, int64, error)
}

type AuditRepository interface {
	Create(ctx context.Context, log *models.PermissionAuditLog) error
	List(ctx context.Context, filter *models.AuditLogFilter) ([]*models.PermissionAuditLog, int64, error)
}

type CacheRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, expiry time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePattern(ctx context.Context, pattern string) error
}

// Concrete implementations

// PermissionRepositoryImpl implements PermissionRepository
type PermissionRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewPermissionRepository(db *gorm.DB, logger *zap.Logger) *PermissionRepositoryImpl {
	return &PermissionRepositoryImpl{db: db, logger: logger}
}

func (r *PermissionRepositoryImpl) GetByNames(ctx context.Context, names []string) ([]*models.Permission, error) {
	var permissions []*models.Permission
	err := r.db.WithContext(ctx).Where("name IN ? AND is_active = ?", names, true).Find(&permissions).Error
	return permissions, err
}

func (r *PermissionRepositoryImpl) List(ctx context.Context, filter *models.PermissionFilter) ([]*models.Permission, int64, error) {
	var permissions []*models.Permission
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Permission{})

	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Search != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	if err := query.Offset(offset).Limit(filter.PageSize).Find(&permissions).Error; err != nil {
		return nil, 0, err
	}

	return permissions, total, nil
}

func (r *PermissionRepositoryImpl) Create(ctx context.Context, permission *models.Permission) error {
	return r.db.WithContext(ctx).Create(permission).Error
}

func (r *PermissionRepositoryImpl) GetByID(ctx context.Context, id string) (*models.Permission, error) {
	var permission models.Permission
	err := r.db.WithContext(ctx).First(&permission, "id = ?", id).Error
	return &permission, err
}

// RoleRepositoryImpl implements RoleRepository
type RoleRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewRoleRepository(db *gorm.DB, logger *zap.Logger) *RoleRepositoryImpl {
	return &RoleRepositoryImpl{db: db, logger: logger}
}

func (r *RoleRepositoryImpl) GetByID(ctx context.Context, id string) (*models.Role, error) {
	var role models.Role
	err := r.db.WithContext(ctx).Preload("Permissions").First(&role, "id = ?", id).Error
	return &role, err
}

func (r *RoleRepositoryImpl) GetByName(ctx context.Context, name, tenantID string) (*models.Role, error) {
	var role models.Role
	query := r.db.WithContext(ctx).Where("name = ?", name)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	} else {
		query = query.Where("tenant_id IS NULL")
	}
	err := query.First(&role).Error
	return &role, err
}

func (r *RoleRepositoryImpl) List(ctx context.Context, filter *models.RoleFilter) ([]*models.Role, int64, error) {
	var roles []*models.Role
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Role{})

	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.Search != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}
	if filter.IsSystemRole != nil {
		query = query.Where("is_system_role = ?", *filter.IsSystemRole)
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	sortBy := filter.SortBy
	if sortBy == "" {
		sortBy = "name"
	}
	sortOrder := filter.SortOrder
	if sortOrder == "" {
		sortOrder = "asc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	offset := (filter.Page - 1) * filter.PageSize
	if err := query.Preload("Permissions").Offset(offset).Limit(filter.PageSize).Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

func (r *RoleRepositoryImpl) Create(ctx context.Context, role *models.Role) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *RoleRepositoryImpl) Update(ctx context.Context, role *models.Role) error {
	return r.db.WithContext(ctx).Save(role).Error
}

func (r *RoleRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Role{}, "id = ?", id).Error
}

func (r *RoleRepositoryImpl) AssignPermissions(ctx context.Context, roleID string, permissions []*models.Permission) error {
	var role models.Role
	if err := r.db.WithContext(ctx).First(&role, "id = ?", roleID).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&role).Association("Permissions").Replace(permissions)
}

// UserRoleRepositoryImpl implements UserRoleRepository
type UserRoleRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewUserRoleRepository(db *gorm.DB, logger *zap.Logger) *UserRoleRepositoryImpl {
	return &UserRoleRepositoryImpl{db: db, logger: logger}
}

func (r *UserRoleRepositoryImpl) GetUserRoles(ctx context.Context, userID, tenantID, resourceType, resourceID string) ([]*models.UserRole, error) {
	var userRoles []*models.UserRole
	query := r.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND is_active = ?", userID, tenantID, true)

	if resourceType != "" {
		if resourceID != "" {
			// Exact resource match
			query = query.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID)
		} else {
			// Resource type match
			query = query.Where("resource_type = ?", resourceType)
		}
	} else {
		// Global roles (no resource specified)
		query = query.Where("resource_type IS NULL AND resource_id IS NULL")
	}

	err := query.Preload("Role").Find(&userRoles).Error
	return userRoles, err
}

func (r *UserRoleRepositoryImpl) GetUserRole(ctx context.Context, userID, roleID, tenantID string, resourceType, resourceID *string) (*models.UserRole, error) {
	var userRole models.UserRole
	query := r.db.WithContext(ctx).Where("user_id = ? AND role_id = ? AND tenant_id = ?", userID, roleID, tenantID)

	if resourceType != nil {
		query = query.Where("resource_type = ?", *resourceType)
	} else {
		query = query.Where("resource_type IS NULL")
	}

	if resourceID != nil {
		query = query.Where("resource_id = ?", *resourceID)
	} else {
		query = query.Where("resource_id IS NULL")
	}

	err := query.First(&userRole).Error
	return &userRole, err
}

func (r *UserRoleRepositoryImpl) Create(ctx context.Context, userRole *models.UserRole) error {
	return r.db.WithContext(ctx).Create(userRole).Error
}

func (r *UserRoleRepositoryImpl) Delete(ctx context.Context, userID, roleID, tenantID string, resourceType, resourceID *string) error {
	query := r.db.WithContext(ctx).Where("user_id = ? AND role_id = ? AND tenant_id = ?", userID, roleID, tenantID)

	if resourceType != nil {
		query = query.Where("resource_type = ?", *resourceType)
	} else {
		query = query.Where("resource_type IS NULL")
	}

	if resourceID != nil {
		query = query.Where("resource_id = ?", *resourceID)
	} else {
		query = query.Where("resource_id IS NULL")
	}

	return query.Delete(&models.UserRole{}).Error
}

func (r *UserRoleRepositoryImpl) List(ctx context.Context, filter *models.UserRoleFilter) ([]*models.UserRole, int64, error) {
	var userRoles []*models.UserRole
	var total int64

	query := r.db.WithContext(ctx).Model(&models.UserRole{})

	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.RoleID != "" {
		query = query.Where("role_id = ?", filter.RoleID)
	}
	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		query = query.Where("resource_id = ?", filter.ResourceID)
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	if err := query.Preload("Role").Offset(offset).Limit(filter.PageSize).Find(&userRoles).Error; err != nil {
		return nil, 0, err
	}

	return userRoles, total, nil
}

// ResourcePermissionRepositoryImpl implements ResourcePermissionRepository
type ResourcePermissionRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewResourcePermissionRepository(db *gorm.DB, logger *zap.Logger) *ResourcePermissionRepositoryImpl {
	return &ResourcePermissionRepositoryImpl{db: db, logger: logger}
}

func (r *ResourcePermissionRepositoryImpl) GetUserResourcePermissions(ctx context.Context, userID, tenantID, resourceType, resourceID string) ([]*models.ResourcePermission, error) {
	var permissions []*models.ResourcePermission
	query := r.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID)

	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if resourceID != "" {
		query = query.Where("resource_id = ?", resourceID)
	}

	// Include permissions that haven't expired
	query = query.Where("expires_at IS NULL OR expires_at > ?", time.Now())

	err := query.Find(&permissions).Error
	return permissions, err
}

func (r *ResourcePermissionRepositoryImpl) GetByID(ctx context.Context, id string) (*models.ResourcePermission, error) {
	var permission models.ResourcePermission
	err := r.db.WithContext(ctx).First(&permission, "id = ?", id).Error
	return &permission, err
}

func (r *ResourcePermissionRepositoryImpl) Create(ctx context.Context, permission *models.ResourcePermission) error {
	return r.db.WithContext(ctx).Create(permission).Error
}

func (r *ResourcePermissionRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.ResourcePermission{}, "id = ?", id).Error
}

func (r *ResourcePermissionRepositoryImpl) List(ctx context.Context, filter interface{}) ([]*models.ResourcePermission, int64, error) {
	// Implementation for listing resource permissions with filters
	var permissions []*models.ResourcePermission
	var total int64

	query := r.db.WithContext(ctx).Model(&models.ResourcePermission{})
	
	// Apply filters based on filter type (would need proper filter struct)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Find(&permissions).Error; err != nil {
		return nil, 0, err
	}

	return permissions, total, nil
}

// AuditRepositoryImpl implements AuditRepository
type AuditRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewAuditRepository(db *gorm.DB, logger *zap.Logger) *AuditRepositoryImpl {
	return &AuditRepositoryImpl{db: db, logger: logger}
}

func (r *AuditRepositoryImpl) Create(ctx context.Context, log *models.PermissionAuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *AuditRepositoryImpl) List(ctx context.Context, filter *models.AuditLogFilter) ([]*models.PermissionAuditLog, int64, error) {
	var logs []*models.PermissionAuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.PermissionAuditLog{})

	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.TargetUserID != "" {
		query = query.Where("target_user_id = ?", filter.TargetUserID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		query = query.Where("resource_id = ?", filter.ResourceID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	sortBy := filter.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := filter.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	offset := (filter.Page - 1) * filter.PageSize
	if err := query.Offset(offset).Limit(filter.PageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// CacheRepositoryImpl implements CacheRepository
type CacheRepositoryImpl struct {
	redis  *redis.Client
	logger *zap.Logger
}

func NewCacheRepository(redis *redis.Client, logger *zap.Logger) *CacheRepositoryImpl {
	return &CacheRepositoryImpl{redis: redis, logger: logger}
}

func (r *CacheRepositoryImpl) Get(ctx context.Context, key string) (string, error) {
	return r.redis.Get(ctx, key).Result()
}

func (r *CacheRepositoryImpl) Set(ctx context.Context, key, value string, expiry time.Duration) error {
	return r.redis.Set(ctx, key, value, expiry).Err()
}

func (r *CacheRepositoryImpl) Delete(ctx context.Context, key string) error {
	return r.redis.Del(ctx, key).Err()
}

func (r *CacheRepositoryImpl) DeletePattern(ctx context.Context, pattern string) error {
	keys, err := r.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return r.redis.Del(ctx, keys...).Err()
	}
	return nil
}

// Repository factory
type Repositories struct {
	PermissionRepo        PermissionRepository
	RoleRepo              RoleRepository
	UserRoleRepo          UserRoleRepository
	ResourcePermissionRepo ResourcePermissionRepository
	AuditRepo             AuditRepository
	CacheRepo             CacheRepository

	db     *gorm.DB
	redis  *redis.Client
	logger *zap.Logger
}

func New(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *Repositories {
	return &Repositories{
		PermissionRepo:         NewPermissionRepository(db, logger),
		RoleRepo:               NewRoleRepository(db, logger),
		UserRoleRepo:           NewUserRoleRepository(db, logger),
		ResourcePermissionRepo: NewResourcePermissionRepository(db, logger),
		AuditRepo:              NewAuditRepository(db, logger),
		CacheRepo:              NewCacheRepository(redis, logger),
		db:                     db,
		redis:                  redis,
		logger:                 logger,
	}
}