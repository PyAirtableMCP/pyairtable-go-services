package graphql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"github.com/sirupsen/logrus"

	"github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/models"
	"github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/services"
	"github.com/pyairtable/pyairtable-compose/go-services/shared/auth"
	"github.com/pyairtable/pyairtable-compose/go-services/shared/middleware"
)

// Resolver is the root GraphQL resolver
type Resolver struct {
	userService         services.UserService
	authService         services.AuthService
	permissionService   services.PermissionService
	notificationService services.NotificationService
	activityService     services.ActivityService
	logger              *logrus.Logger
}

// NewResolver creates a new GraphQL resolver
func NewResolver(
	userService services.UserService,
	authService services.AuthService,
	permissionService services.PermissionService,
	notificationService services.NotificationService,
	activityService services.ActivityService,
	logger *logrus.Logger,
) *Resolver {
	return &Resolver{
		userService:         userService,
		authService:         authService,
		permissionService:   permissionService,
		notificationService: notificationService,
		activityService:     activityService,
		logger:              logger,
	}
}

// Query resolvers
type queryResolver struct {
	*Resolver
}

func (r *Resolver) Query() *queryResolver {
	return &queryResolver{r}
}

// Me returns the current authenticated user
func (r *queryResolver) Me(ctx context.Context) (*userResolver, error) {
	userID := auth.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, errors.New("authentication required")
	}

	user, err := r.userService.GetUserByID(ctx, userID)
	if err != nil {
		r.logger.WithError(err).WithField("userID", userID).Error("Failed to get current user")
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	return &userResolver{user: user, resolver: r.Resolver}, nil
}

// User returns a user by ID
func (r *queryResolver) User(ctx context.Context, args struct{ ID graphql.ID }) (*userResolver, error) {
	// Check if user has permission to view this user
	currentUserID := auth.GetUserIDFromContext(ctx)
	if currentUserID != string(args.ID) {
		role := auth.GetUserRoleFromContext(ctx)
		if role != "ADMIN" {
			return nil, errors.New("insufficient permissions")
		}
	}

	user, err := r.userService.GetUserByID(ctx, string(args.ID))
	if err != nil {
		r.logger.WithError(err).WithField("userID", args.ID).Error("Failed to get user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if user == nil {
		return nil, nil
	}

	return &userResolver{user: user, resolver: r.Resolver}, nil
}

// Users returns multiple users by IDs (for DataLoader)
func (r *queryResolver) Users(ctx context.Context, args struct{ IDs []graphql.ID }) ([]*userResolver, error) {
	// Convert GraphQL IDs to strings
	ids := make([]string, len(args.IDs))
	for i, id := range args.IDs {
		ids[i] = string(id)
	}

	users, err := r.userService.GetUsersByIDs(ctx, ids)
	if err != nil {
		r.logger.WithError(err).WithField("ids", ids).Error("Failed to get users by IDs")
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	resolvers := make([]*userResolver, len(users))
	for i, user := range users {
		if user != nil {
			resolvers[i] = &userResolver{user: user, resolver: r.Resolver}
		}
	}

	return resolvers, nil
}

// SearchUsers searches for users with filters
func (r *queryResolver) SearchUsers(ctx context.Context, args struct {
	Query      *string
	Filters    *UserSearchFilters
	Pagination *PaginationInput
	Sort       *SortInput
}) (*paginatedUsersResolver, error) {
	// Build search parameters
	params := &services.UserSearchParams{
		Query: "",
		Limit: 20,
		Offset: 0,
	}

	if args.Query != nil {
		params.Query = *args.Query
	}

	if args.Pagination != nil {
		if args.Pagination.Limit != nil {
			params.Limit = int(*args.Pagination.Limit)
		}
		if args.Pagination.Offset != nil {
			params.Offset = int(*args.Pagination.Offset)
		}
	}

	if args.Filters != nil {
		if args.Filters.Role != nil {
			params.Role = (*string)(args.Filters.Role)
		}
		if args.Filters.Status != nil {
			params.Status = (*string)(args.Filters.Status)
		}
		if args.Filters.WorkspaceID != nil {
			params.WorkspaceID = (*string)(args.Filters.WorkspaceID)
		}
	}

	if args.Sort != nil {
		params.SortField = args.Sort.Field
		params.SortDirection = string(args.Sort.Direction)
	}

	result, err := r.userService.SearchUsers(ctx, params)
	if err != nil {
		r.logger.WithError(err).WithField("params", params).Error("Failed to search users")
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	return &paginatedUsersResolver{result: result, resolver: r.Resolver}, nil
}

// UserPermissions returns permissions for a user
func (r *queryResolver) UserPermissions(ctx context.Context, args struct{ UserID graphql.ID }) ([]*permissionResolver, error) {
	// Check authorization
	currentUserID := auth.GetUserIDFromContext(ctx)
	if currentUserID != string(args.UserID) {
		role := auth.GetUserRoleFromContext(ctx)
		if role != "ADMIN" {
			return nil, errors.New("insufficient permissions")
		}
	}

	permissions, err := r.permissionService.GetUserPermissions(ctx, string(args.UserID))
	if err != nil {
		r.logger.WithError(err).WithField("userID", args.UserID).Error("Failed to get user permissions")
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	resolvers := make([]*permissionResolver, len(permissions))
	for i, permission := range permissions {
		resolvers[i] = &permissionResolver{permission: permission, resolver: r.Resolver}
	}

	return resolvers, nil
}

// UserNotifications returns notifications for a user
func (r *queryResolver) UserNotifications(ctx context.Context, args struct {
	UserID     graphql.ID
	UnreadOnly *bool
	Pagination *PaginationInput
}) ([]*notificationResolver, error) {
	// Check authorization
	currentUserID := auth.GetUserIDFromContext(ctx)
	if currentUserID != string(args.UserID) {
		return nil, errors.New("can only view your own notifications")
	}

	params := &services.NotificationParams{
		UserID:     string(args.UserID),
		UnreadOnly: false,
		Limit:      20,
		Offset:     0,
	}

	if args.UnreadOnly != nil {
		params.UnreadOnly = *args.UnreadOnly
	}

	if args.Pagination != nil {
		if args.Pagination.Limit != nil {
			params.Limit = int(*args.Pagination.Limit)
		}
		if args.Pagination.Offset != nil {
			params.Offset = int(*args.Pagination.Offset)
		}
	}

	notifications, err := r.notificationService.GetUserNotifications(ctx, params)
	if err != nil {
		r.logger.WithError(err).WithField("params", params).Error("Failed to get user notifications")
		return nil, fmt.Errorf("failed to get user notifications: %w", err)
	}

	resolvers := make([]*notificationResolver, len(notifications))
	for i, notification := range notifications {
		resolvers[i] = &notificationResolver{notification: notification, resolver: r.Resolver}
	}

	return resolvers, nil
}

// IsUsernameAvailable checks if a username is available
func (r *queryResolver) IsUsernameAvailable(ctx context.Context, args struct{ Username string }) (bool, error) {
	available, err := r.userService.IsUsernameAvailable(ctx, args.Username)
	if err != nil {
		r.logger.WithError(err).WithField("username", args.Username).Error("Failed to check username availability")
		return false, fmt.Errorf("failed to check username availability: %w", err)
	}

	return available, nil
}

// IsEmailAvailable checks if an email is available
func (r *queryResolver) IsEmailAvailable(ctx context.Context, args struct{ Email string }) (bool, error) {
	available, err := r.userService.IsEmailAvailable(ctx, args.Email)
	if err != nil {
		r.logger.WithError(err).WithField("email", args.Email).Error("Failed to check email availability")
		return false, fmt.Errorf("failed to check email availability: %w", err)
	}

	return available, nil
}

// Mutation resolvers
type mutationResolver struct {
	*Resolver
}

func (r *Resolver) Mutation() *mutationResolver {
	return &mutationResolver{r}
}

// Register creates a new user account
func (r *mutationResolver) Register(ctx context.Context, args struct{ Input RegisterInput }) (*authResultResolver, error) {
	// Validate input
	if err := validateRegisterInput(args.Input); err != nil {
		return nil, err
	}

	// Create user
	user := &models.User{
		Email:       args.Input.Email,
		FirstName:   args.Input.FirstName,
		LastName:    args.Input.LastName,
		Username:    args.Input.Username,
		Role:        models.UserRoleFreeUser,
		Status:      models.UserStatusPendingVerification,
		Preferences: models.DefaultUserPreferences(),
		Profile:     &models.UserProfile{},
	}

	result, err := r.authService.Register(ctx, user, args.Input.Password)
	if err != nil {
		r.logger.WithError(err).WithField("email", args.Input.Email).Error("Failed to register user")
		return nil, fmt.Errorf("failed to register user: %w", err)
	}

	// Log activity
	r.activityService.LogActivity(ctx, &models.UserActivity{
		UserID:    user.ID,
		Action:    models.UserActionLogin,
		IP:        middleware.GetIPFromContext(ctx),
		UserAgent: middleware.GetUserAgentFromContext(ctx),
		Timestamp: time.Now(),
	})

	return &authResultResolver{result: result, resolver: r.Resolver}, nil
}

// Login authenticates a user
func (r *mutationResolver) Login(ctx context.Context, args struct{ Input LoginInput }) (*authResultResolver, error) {
	result, err := r.authService.Login(ctx, args.Input.Email, args.Input.Password)
	if err != nil {
		r.logger.WithError(err).WithField("email", args.Input.Email).Error("Failed to login user")
		return nil, fmt.Errorf("login failed: %w", err)
	}

	// Log activity
	if result.User != nil {
		r.activityService.LogActivity(ctx, &models.UserActivity{
			UserID:    result.User.ID,
			Action:    models.UserActionLogin,
			IP:        middleware.GetIPFromContext(ctx),
			UserAgent: middleware.GetUserAgentFromContext(ctx),
			Timestamp: time.Now(),
		})
	}

	return &authResultResolver{result: result, resolver: r.Resolver}, nil
}

// UpdateUser updates user information
func (r *mutationResolver) UpdateUser(ctx context.Context, args struct{ Input UpdateUserInput }) (*userResolver, error) {
	userID := auth.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, errors.New("authentication required")
	}

	// Get current user
	user, err := r.userService.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Update user fields
	if args.Input.FirstName != nil {
		user.FirstName = *args.Input.FirstName
	}
	if args.Input.LastName != nil {
		user.LastName = *args.Input.LastName
	}
	// ... update other fields

	updatedUser, err := r.userService.UpdateUser(ctx, user)
	if err != nil {
		r.logger.WithError(err).WithField("userID", userID).Error("Failed to update user")
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Log activity
	r.activityService.LogActivity(ctx, &models.UserActivity{
		UserID:    userID,
		Action:    models.UserActionProfileUpdate,
		IP:        middleware.GetIPFromContext(ctx),
		UserAgent: middleware.GetUserAgentFromContext(ctx),
		Timestamp: time.Now(),
	})

	return &userResolver{user: updatedUser, resolver: r.Resolver}, nil
}

// User resolver
type userResolver struct {
	user     *models.User
	resolver *Resolver
}

func (r *userResolver) ID() graphql.ID {
	return graphql.ID(r.user.ID)
}

func (r *userResolver) Email() string {
	return r.user.Email
}

func (r *userResolver) Username() *string {
	if r.user.Username == "" {
		return nil
	}
	return &r.user.Username
}

func (r *userResolver) FirstName() *string {
	if r.user.FirstName == "" {
		return nil
	}
	return &r.user.FirstName
}

func (r *userResolver) LastName() *string {
	if r.user.LastName == "" {
		return nil
	}
	return &r.user.LastName
}

func (r *userResolver) DisplayName() *string {
	displayName := r.user.GetDisplayName()
	if displayName == "" {
		return nil
	}
	return &displayName
}

func (r *userResolver) Role() UserRole {
	return UserRole(r.user.Role)
}

func (r *userResolver) Status() UserStatus {
	return UserStatus(r.user.Status)
}

func (r *userResolver) EmailVerified() bool {
	return r.user.EmailVerified
}

func (r *userResolver) CreatedAt() graphql.Time {
	return graphql.Time{Time: r.user.CreatedAt}
}

func (r *userResolver) UpdatedAt() graphql.Time {
	return graphql.Time{Time: r.user.UpdatedAt}
}

// Permission resolver
type permissionResolver struct {
	permission *models.Permission
	resolver   *Resolver
}

func (r *permissionResolver) ID() graphql.ID {
	return graphql.ID(r.permission.ID)
}

func (r *permissionResolver) Name() string {
	return r.permission.Name
}

func (r *permissionResolver) Description() *string {
	if r.permission.Description == "" {
		return nil
	}
	return &r.permission.Description
}

// Notification resolver
type notificationResolver struct {
	notification *models.Notification
	resolver     *Resolver
}

func (r *notificationResolver) ID() graphql.ID {
	return graphql.ID(r.notification.ID)
}

func (r *notificationResolver) Title() string {
	return r.notification.Title
}

func (r *notificationResolver) Message() string {
	return r.notification.Message
}

func (r *notificationResolver) Read() bool {
	return r.notification.Read
}

func (r *notificationResolver) CreatedAt() graphql.Time {
	return graphql.Time{Time: r.notification.CreatedAt}
}

// Auth result resolver
type authResultResolver struct {
	result   *models.AuthResult
	resolver *Resolver
}

func (r *authResultResolver) Success() bool {
	return r.result.Success
}

func (r *authResultResolver) Token() *string {
	if r.result.Token == "" {
		return nil
	}
	return &r.result.Token
}

func (r *authResultResolver) User() *userResolver {
	if r.result.User == nil {
		return nil
	}
	return &userResolver{user: r.result.User, resolver: r.resolver}
}

func (r *authResultResolver) Message() *string {
	if r.result.Message == "" {
		return nil
	}
	return &r.result.Message
}

// Paginated users resolver
type paginatedUsersResolver struct {
	result   *models.PaginatedUsers
	resolver *Resolver
}

func (r *paginatedUsersResolver) Users() []*userResolver {
	resolvers := make([]*userResolver, len(r.result.Users))
	for i, user := range r.result.Users {
		resolvers[i] = &userResolver{user: user, resolver: r.resolver}
	}
	return resolvers
}

func (r *paginatedUsersResolver) TotalCount() int32 {
	return int32(r.result.TotalCount)
}

func (r *paginatedUsersResolver) HasNextPage() bool {
	return r.result.HasNextPage
}

func (r *paginatedUsersResolver) HasPreviousPage() bool {
	return r.result.HasPreviousPage
}

// Input types
type RegisterInput struct {
	Email          string
	Password       string
	FirstName      string
	LastName       string
	Username       *string
	AcceptTerms    bool
	MarketingOptIn *bool
}

type LoginInput struct {
	Email      string
	Password   string
	RememberMe *bool
}

type UpdateUserInput struct {
	FirstName   *string
	LastName    *string
	Username    *string
	Avatar      *string
	PhoneNumber *string
	Timezone    *string
	Locale      *string
}

type UserSearchFilters struct {
	Role        *UserRole
	Status      *UserStatus
	WorkspaceID *graphql.ID
	Skills      *[]string
	Location    *string
	Company     *string
}

type PaginationInput struct {
	Limit  *int32
	Offset *int32
	Cursor *string
}

type SortInput struct {
	Field     string
	Direction SortDirection
}

// Enums
type UserRole string
type UserStatus string
type SortDirection string

// Validation functions
func validateRegisterInput(input RegisterInput) error {
	if input.Email == "" {
		return errors.New("email is required")
	}
	if input.Password == "" {
		return errors.New("password is required")
	}
	if len(input.Password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if input.FirstName == "" {
		return errors.New("first name is required")
	}
	if input.LastName == "" {
		return errors.New("last name is required")
	}
	if !input.AcceptTerms {
		return errors.New("must accept terms of service")
	}
	if !isValidEmail(input.Email) {
		return errors.New("invalid email format")
	}
	return nil
}

func isValidEmail(email string) bool {
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}