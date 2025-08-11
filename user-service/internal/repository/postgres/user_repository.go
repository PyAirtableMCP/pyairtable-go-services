package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	
	"github.com/Reg-Kris/pyairtable-user-service/internal/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	
	preferencesJSON, _ := json.Marshal(user.Preferences)
	metadataJSON, _ := json.Marshal(user.Metadata)
	
	query := `
		INSERT INTO users (id, email, first_name, last_name, display_name, avatar, bio, phone,
		                   tenant_id, role, is_active, email_verified, phone_verified,
		                   preferences, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	
	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Email, user.FirstName, user.LastName, user.DisplayName,
		user.Avatar, user.Bio, user.Phone, user.TenantID, user.Role,
		user.IsActive, user.EmailVerified, user.PhoneVerified,
		preferencesJSON, metadataJSON, user.CreatedAt, user.UpdatedAt,
	)
	
	return err
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	user.UpdatedAt = time.Now()
	
	preferencesJSON, _ := json.Marshal(user.Preferences)
	metadataJSON, _ := json.Marshal(user.Metadata)
	
	query := `
		UPDATE users 
		SET email = $2, first_name = $3, last_name = $4, display_name = $5,
		    avatar = $6, bio = $7, phone = $8, role = $9,
		    is_active = $10, email_verified = $11, phone_verified = $12,
		    preferences = $13, metadata = $14, updated_at = $15, last_login_at = $16
		WHERE id = $1 AND deleted_at IS NULL
	`
	
	result, err := r.db.ExecContext(ctx, query,
		user.ID, user.Email, user.FirstName, user.LastName, user.DisplayName,
		user.Avatar, user.Bio, user.Phone, user.Role,
		user.IsActive, user.EmailVerified, user.PhoneVerified,
		preferencesJSON, metadataJSON, user.UpdatedAt, user.LastLoginAt,
	)
	
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("user not found")
	}
	
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("user not found")
	}
	
	return nil
}

func (r *UserRepository) SoftDelete(ctx context.Context, id string) error {
	query := `UPDATE users SET deleted_at = $2 WHERE id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now())
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("user not found")
	}
	
	return nil
}

func (r *UserRepository) Restore(ctx context.Context, id string) error {
	query := `UPDATE users SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("user not found or not deleted")
	}
	
	return nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	user := &models.User{}
	var preferencesJSON, metadataJSON sql.NullString
	
	query := `
		SELECT id, email, first_name, last_name, display_name, avatar, bio, phone,
		       tenant_id, role, is_active, email_verified, phone_verified,
		       preferences, metadata, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE id = $1 AND deleted_at IS NULL
	`
	
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
		&user.Avatar, &user.Bio, &user.Phone, &user.TenantID, &user.Role,
		&user.IsActive, &user.EmailVerified, &user.PhoneVerified,
		&preferencesJSON, &metadataJSON, &user.LastLoginAt,
		&user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	
	if err != nil {
		return nil, err
	}
	
	if preferencesJSON.Valid {
		json.Unmarshal([]byte(preferencesJSON.String), &user.Preferences)
	}
	if metadataJSON.Valid {
		json.Unmarshal([]byte(metadataJSON.String), &user.Metadata)
	}
	
	return user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	user := &models.User{}
	var preferencesJSON, metadataJSON sql.NullString
	
	query := `
		SELECT id, email, first_name, last_name, display_name, avatar, bio, phone,
		       tenant_id, role, is_active, email_verified, phone_verified,
		       preferences, metadata, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE email = $1 AND deleted_at IS NULL
	`
	
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
		&user.Avatar, &user.Bio, &user.Phone, &user.TenantID, &user.Role,
		&user.IsActive, &user.EmailVerified, &user.PhoneVerified,
		&preferencesJSON, &metadataJSON, &user.LastLoginAt,
		&user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	
	if err != nil {
		return nil, err
	}
	
	if preferencesJSON.Valid {
		json.Unmarshal([]byte(preferencesJSON.String), &user.Preferences)
	}
	if metadataJSON.Valid {
		json.Unmarshal([]byte(metadataJSON.String), &user.Metadata)
	}
	
	return user, nil
}

func (r *UserRepository) FindByIDs(ctx context.Context, ids []string) ([]*models.User, error) {
	if len(ids) == 0 {
		return []*models.User{}, nil
	}
	
	query := `
		SELECT id, email, first_name, last_name, display_name, avatar, bio, phone,
		       tenant_id, role, is_active, email_verified, phone_verified,
		       preferences, metadata, last_login_at, created_at, updated_at, deleted_at
		FROM users WHERE id = ANY($1) AND deleted_at IS NULL
		ORDER BY created_at DESC
	`
	
	rows, err := r.db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	return r.scanUsers(rows)
}

func (r *UserRepository) List(ctx context.Context, filter *models.UserFilter) ([]*models.User, int64, error) {
	// Build WHERE clause
	whereClause := []string{}
	args := []interface{}{}
	argCount := 0
	
	if !filter.IncludeDeleted {
		whereClause = append(whereClause, "deleted_at IS NULL")
	}
	
	if filter.TenantID != "" {
		argCount++
		whereClause = append(whereClause, fmt.Sprintf("tenant_id = $%d", argCount))
		args = append(args, filter.TenantID)
	}
	
	if filter.Role != "" {
		argCount++
		whereClause = append(whereClause, fmt.Sprintf("role = $%d", argCount))
		args = append(args, filter.Role)
	}
	
	if filter.IsActive != nil {
		argCount++
		whereClause = append(whereClause, fmt.Sprintf("is_active = $%d", argCount))
		args = append(args, *filter.IsActive)
	}
	
	if filter.EmailVerified != nil {
		argCount++
		whereClause = append(whereClause, fmt.Sprintf("email_verified = $%d", argCount))
		args = append(args, *filter.EmailVerified)
	}
	
	if filter.Search != "" {
		argCount++
		whereClause = append(whereClause, fmt.Sprintf(
			"(email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d OR display_name ILIKE $%d)",
			argCount, argCount, argCount, argCount,
		))
		searchTerm := "%" + filter.Search + "%"
		args = append(args, searchTerm)
	}
	
	where := ""
	if len(whereClause) > 0 {
		where = "WHERE " + strings.Join(whereClause, " AND ")
	}
	
	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users %s", where)
	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	
	// Build ORDER BY clause
	orderBy := "created_at DESC"
	if filter.SortBy != "" {
		validSortFields := map[string]bool{
			"email": true, "first_name": true, "last_name": true,
			"created_at": true, "updated_at": true, "last_login_at": true,
		}
		if validSortFields[filter.SortBy] {
			orderBy = filter.SortBy
			if filter.SortOrder == "asc" {
				orderBy += " ASC"
			} else {
				orderBy += " DESC"
			}
		}
	}
	
	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize
	
	// Query users
	query := fmt.Sprintf(`
		SELECT id, email, first_name, last_name, display_name, avatar, bio, phone,
		       tenant_id, role, is_active, email_verified, phone_verified,
		       preferences, metadata, last_login_at, created_at, updated_at, deleted_at
		FROM users %s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, where, orderBy, filter.PageSize, offset)
	
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	
	users, err := r.scanUsers(rows)
	if err != nil {
		return nil, 0, err
	}
	
	return users, total, nil
}

func (r *UserRepository) ListByTenant(ctx context.Context, tenantID string, filter *models.UserFilter) ([]*models.User, int64, error) {
	filter.TenantID = tenantID
	return r.List(ctx, filter)
}

func (r *UserRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	query := `SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND deleted_at IS NULL`
	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	return count, err
}

func (r *UserRepository) GetStats(ctx context.Context, tenantID string) (*models.UserStats, error) {
	stats := &models.UserStats{
		UsersByRole:   make(map[string]int64),
		UsersByTenant: make(map[string]int64),
		LastUpdated:   time.Now(),
	}
	
	whereClause := "deleted_at IS NULL"
	args := []interface{}{}
	if tenantID != "" {
		whereClause += " AND tenant_id = $1"
		args = append(args, tenantID)
	}
	
	// Total users
	err := r.db.QueryRowContext(ctx, 
		fmt.Sprintf("SELECT COUNT(*) FROM users WHERE %s", whereClause),
		args...,
	).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, err
	}
	
	// Active users
	err = r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM users WHERE %s AND is_active = true", whereClause),
		args...,
	).Scan(&stats.ActiveUsers)
	if err != nil {
		return nil, err
	}
	
	// Verified users
	err = r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM users WHERE %s AND email_verified = true", whereClause),
		args...,
	).Scan(&stats.VerifiedUsers)
	if err != nil {
		return nil, err
	}
	
	// Users by role
	roleQuery := fmt.Sprintf(`
		SELECT role, COUNT(*) 
		FROM users 
		WHERE %s 
		GROUP BY role
	`, whereClause)
	rows, err := r.db.QueryContext(ctx, roleQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	for rows.Next() {
		var role string
		var count int64
		if err := rows.Scan(&role, &count); err != nil {
			return nil, err
		}
		stats.UsersByRole[role] = count
	}
	
	// Recent signups (last 30 days)
	recentQuery := fmt.Sprintf(`
		SELECT COUNT(*) 
		FROM users 
		WHERE %s AND created_at > NOW() - INTERVAL '30 days'
	`, whereClause)
	err = r.db.QueryRowContext(ctx, recentQuery, args...).Scan(&stats.RecentSignups)
	if err != nil {
		return nil, err
	}
	
	return stats, nil
}

func (r *UserRepository) BulkCreate(ctx context.Context, users []*models.User) error {
	// Implementation would use COPY or multi-value INSERT
	// For now, using transaction with individual inserts
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	for _, user := range users {
		if err := r.Create(ctx, user); err != nil {
			return err
		}
	}
	
	return tx.Commit()
}

func (r *UserRepository) BulkUpdate(ctx context.Context, users []*models.User) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	for _, user := range users {
		if err := r.Update(ctx, user); err != nil {
			return err
		}
	}
	
	return tx.Commit()
}

func (r *UserRepository) BulkDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	
	query := `UPDATE users SET deleted_at = $1 WHERE id = ANY($2)`
	_, err := r.db.ExecContext(ctx, query, time.Now(), pq.Array(ids))
	return err
}

func (r *UserRepository) scanUsers(rows *sql.Rows) ([]*models.User, error) {
	var users []*models.User
	
	for rows.Next() {
		user := &models.User{}
		var preferencesJSON, metadataJSON sql.NullString
		
		err := rows.Scan(
			&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
			&user.Avatar, &user.Bio, &user.Phone, &user.TenantID, &user.Role,
			&user.IsActive, &user.EmailVerified, &user.PhoneVerified,
			&preferencesJSON, &metadataJSON, &user.LastLoginAt,
			&user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		
		if preferencesJSON.Valid {
			json.Unmarshal([]byte(preferencesJSON.String), &user.Preferences)
		}
		if metadataJSON.Valid {
			json.Unmarshal([]byte(metadataJSON.String), &user.Metadata)
		}
		
		users = append(users, user)
	}
	
	return users, nil
}