package services

import (
	"errors"
	"testing"
	"time"

	"github.com/pyairtable-compose/auth-service/internal/models"
	"go.uber.org/zap/zaptest"
)

// Mock UserRepository for testing
type mockUserRepository struct {
	users       map[string]*models.User
	emailLookup map[string]string // email -> id mapping
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users:       make(map[string]*models.User),
		emailLookup: make(map[string]string),
	}
}

func (m *mockUserRepository) Create(user *models.User) error {
	if user.ID == "" {
		user.ID = "test-user-id"
	}
	
	// Check if email already exists
	if _, exists := m.emailLookup[user.Email]; exists {
		return errors.New("email already exists")
	}
	
	m.users[user.ID] = user
	m.emailLookup[user.Email] = user.ID
	return nil
}

func (m *mockUserRepository) Update(user *models.User) error {
	if _, exists := m.users[user.ID]; !exists {
		return errors.New("user not found")
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepository) Delete(id string) error {
	user, exists := m.users[id]
	if !exists {
		return errors.New("user not found")
	}
	delete(m.emailLookup, user.Email)
	delete(m.users, id)
	return nil
}

func (m *mockUserRepository) FindByID(id string) (*models.User, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (m *mockUserRepository) FindByEmail(email string) (*models.User, error) {
	id, exists := m.emailLookup[email]
	if !exists {
		return nil, errors.New("user not found")
	}
	return m.users[id], nil
}

func (m *mockUserRepository) FindByTenant(tenantID string) ([]*models.User, error) {
	var users []*models.User
	for _, user := range m.users {
		if user.TenantID == tenantID {
			users = append(users, user)
		}
	}
	return users, nil
}

func (m *mockUserRepository) List(limit, offset int) ([]*models.User, error) {
	var users []*models.User
	for _, user := range m.users {
		users = append(users, user)
	}
	return users, nil
}

func (m *mockUserRepository) Count() (int64, error) {
	return int64(len(m.users)), nil
}

// Mock TokenRepository for testing
type mockTokenRepository struct {
	tokens map[string]string // token -> userID
}

func newMockTokenRepository() *mockTokenRepository {
	return &mockTokenRepository{
		tokens: make(map[string]string),
	}
}

func (m *mockTokenRepository) StoreRefreshToken(token, userID string, ttl time.Duration) error {
	m.tokens[token] = userID
	return nil
}

func (m *mockTokenRepository) ValidateRefreshToken(token string) (string, error) {
	userID, exists := m.tokens[token]
	if !exists {
		return "", errors.New("token not found")
	}
	return userID, nil
}

func (m *mockTokenRepository) InvalidateRefreshToken(token string) error {
	delete(m.tokens, token)
	return nil
}

func (m *mockTokenRepository) InvalidateAllUserTokens(userID string) error {
	for token, tokenUserID := range m.tokens {
		if tokenUserID == userID {
			delete(m.tokens, token)
		}
	}
	return nil
}

func (m *mockTokenRepository) CleanupExpiredTokens() error {
	return nil
}

// Helper function to create a test auth service
func createTestAuthService(t *testing.T) *AuthService {
	logger := zaptest.NewLogger(t)
	userRepo := newMockUserRepository()
	tokenRepo := newMockTokenRepository()
	jwtSecret := "test-secret-key-for-jwt-tokens"
	
	return NewAuthService(logger, userRepo, tokenRepo, jwtSecret)
}

func TestAuthService_Register(t *testing.T) {
	authService := createTestAuthService(t)
	
	req := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	user, err := authService.Register(req)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}
	
	if user.Email != req.Email {
		t.Errorf("Expected email %s, got %s", req.Email, user.Email)
	}
	
	if user.FirstName != req.FirstName {
		t.Errorf("Expected first name %s, got %s", req.FirstName, user.FirstName)
	}
	
	if user.LastName != req.LastName {
		t.Errorf("Expected last name %s, got %s", req.LastName, user.LastName)
	}
	
	if !user.IsActive {
		t.Error("New user should be active by default")
	}
	
	if user.EmailVerified {
		t.Error("New user should not have verified email by default")
	}
	
	// Test password hash is created correctly
	if !models.CheckPassword(req.Password, user.PasswordHash) {
		t.Error("Password hash should validate against original password")
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	authService := createTestAuthService(t)
	
	req := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	// First registration should succeed
	_, err := authService.Register(req)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}
	
	// Second registration with same email should fail
	_, err = authService.Register(req)
	if err == nil {
		t.Error("Expected error for duplicate email registration")
	}
	
	if err.Error() != "email already registered" {
		t.Errorf("Expected 'email already registered' error, got: %s", err.Error())
	}
}

func TestAuthService_Login(t *testing.T) {
	authService := createTestAuthService(t)
	
	// Register a user first
	registerReq := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	_, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	// Test login
	loginReq := &models.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	
	tokens, err := authService.Login(loginReq)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	if tokens.AccessToken == "" {
		t.Error("Access token should not be empty")
	}
	
	if tokens.RefreshToken == "" {
		t.Error("Refresh token should not be empty")
	}
	
	if tokens.TokenType != "Bearer" {
		t.Errorf("Expected token type 'Bearer', got %s", tokens.TokenType)
	}
	
	if tokens.ExpiresIn <= 0 {
		t.Error("ExpiresIn should be positive")
	}
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	authService := createTestAuthService(t)
	
	// Test login with non-existent user
	loginReq := &models.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "password123",
	}
	
	_, err := authService.Login(loginReq)
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
	
	// Register a user
	registerReq := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "correctpassword",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	_, err = authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	// Test login with wrong password
	loginReq = &models.LoginRequest{
		Email:    "test@example.com",
		Password: "wrongpassword",
	}
	
	_, err = authService.Login(loginReq)
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials for wrong password, got %v", err)
	}
}

func TestAuthService_ValidateToken(t *testing.T) {
	authService := createTestAuthService(t)
	
	// Register and login to get a token
	registerReq := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	loginReq := &models.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	
	tokens, err := authService.Login(loginReq)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	// Validate the token
	claims, err := authService.ValidateToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("Token validation failed: %v", err)
	}
	
	if claims.UserID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, claims.UserID)
	}
	
	if claims.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, claims.Email)
	}
	
	if claims.Role != user.Role {
		t.Errorf("Expected role %s, got %s", user.Role, claims.Role)
	}
}

func TestAuthService_ValidateToken_Invalid(t *testing.T) {
	authService := createTestAuthService(t)
	
	// Test with invalid token
	_, err := authService.ValidateToken("invalid.token.here")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
	
	// Test with empty token
	_, err = authService.ValidateToken("")
	if err == nil {
		t.Error("Expected error for empty token")
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	authService := createTestAuthService(t)
	
	// Register a user
	registerReq := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "oldpassword123",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	// Change password
	changeReq := &models.ChangePasswordRequest{
		CurrentPassword: "oldpassword123",
		NewPassword:     "newpassword456",
	}
	
	err = authService.ChangePassword(user.ID, changeReq)
	if err != nil {
		t.Fatalf("Password change failed: %v", err)
	}
	
	// Verify new password works
	loginReq := &models.LoginRequest{
		Email:    "test@example.com",
		Password: "newpassword456",
	}
	
	_, err = authService.Login(loginReq)
	if err != nil {
		t.Error("Login with new password should succeed")
	}
	
	// Verify old password no longer works
	loginReq.Password = "oldpassword123"
	_, err = authService.Login(loginReq)
	if err != ErrInvalidCredentials {
		t.Error("Login with old password should fail")
	}
}

func TestAuthService_ChangePassword_WrongCurrentPassword(t *testing.T) {
	authService := createTestAuthService(t)
	
	// Register a user
	registerReq := &models.RegisterRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}
	
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	// Try to change password with wrong current password
	changeReq := &models.ChangePasswordRequest{
		CurrentPassword: "wrongpassword",
		NewPassword:     "newpassword456",
	}
	
	err = authService.ChangePassword(user.ID, changeReq)
	if err == nil {
		t.Error("Expected error for wrong current password")
	}
	
	if err.Error() != "current password is incorrect" {
		t.Errorf("Expected 'current password is incorrect' error, got: %s", err.Error())
	}
}