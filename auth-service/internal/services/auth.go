package services

import (
    "crypto/rand"
    "encoding/base64"
    "errors"
    "time"
    
    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
    "github.com/pyairtable-compose/auth-service/internal/models"
    "github.com/pyairtable-compose/auth-service/internal/repository"
    "go.uber.org/zap"
)

var (
    ErrInvalidCredentials = errors.New("invalid email or password")
    ErrUserNotActive     = errors.New("user account is not active")
    ErrInvalidToken      = errors.New("invalid token")
    ErrTokenExpired      = errors.New("token has expired")
)

// UserContext represents the authenticated user context
type UserContext struct {
    UserID   string
    Email    string
    Role     string
    TenantID string
}

// AuthService handles authentication logic
type AuthService struct {
    logger     *zap.Logger
    userRepo   repository.UserRepository
    tokenRepo  repository.TokenRepository
    jwtSecret  string
    tokenTTL   time.Duration
    refreshTTL time.Duration
}

// NewAuthService creates a new auth service
func NewAuthService(logger *zap.Logger, userRepo repository.UserRepository, tokenRepo repository.TokenRepository, jwtSecret string) *AuthService {
    return &AuthService{
        logger:     logger,
        userRepo:   userRepo,
        tokenRepo:  tokenRepo,
        jwtSecret:  jwtSecret,
        tokenTTL:   24 * time.Hour,      // Access token: 24 hours
        refreshTTL: 7 * 24 * time.Hour,  // Refresh token: 7 days
    }
}

// Login authenticates a user and returns tokens
func (s *AuthService) Login(req *models.LoginRequest) (*models.TokenResponse, error) {
    // Get identifier (email or username)
    identifier := req.GetIdentifier()
    if identifier == "" {
        return nil, ErrInvalidCredentials
    }
    
    // Find user by email (treating username as email for flexibility)
    user, err := s.userRepo.FindByEmail(identifier)
    if err != nil {
        s.logger.Error("Failed to find user", zap.Error(err), zap.String("identifier", identifier))
        return nil, ErrInvalidCredentials
    }
    
    // Check password
    if !models.CheckPassword(req.Password, user.PasswordHash) {
        return nil, ErrInvalidCredentials
    }
    
    // Check if user is active
    if !user.IsActive {
        return nil, ErrUserNotActive
    }
    
    // Generate tokens
    accessToken, err := s.generateAccessToken(user)
    if err != nil {
        s.logger.Error("Failed to generate access token", zap.Error(err))
        return nil, err
    }
    
    refreshToken, err := s.generateRefreshToken(user.ID)
    if err != nil {
        s.logger.Error("Failed to generate refresh token", zap.Error(err))
        return nil, err
    }
    
    // Update last login
    now := time.Now()
    user.LastLoginAt = &now
    if err := s.userRepo.Update(user); err != nil {
        s.logger.Error("Failed to update last login", zap.Error(err))
    }
    
    return &models.TokenResponse{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        TokenType:    "Bearer",
        ExpiresIn:    int(s.tokenTTL.Seconds()),
    }, nil
}

// Register creates a new user account
func (s *AuthService) Register(req *models.RegisterRequest) (*models.User, error) {
    // Check if user exists
    existing, _ := s.userRepo.FindByEmail(req.Email)
    if existing != nil {
        return nil, errors.New("email already registered")
    }
    
    // Hash password
    passwordHash, err := models.HashPassword(req.Password)
    if err != nil {
        return nil, err
    }
    
    // Generate tenant ID if not provided
    tenantID := req.TenantID
    if tenantID == "" {
        // Generate a new UUID for tenant_id
        tenantID = uuid.New().String()
    }
    
    // Create user
    user := &models.User{
        Email:         req.Email,
        PasswordHash:  passwordHash,
        FirstName:     req.FirstName,
        LastName:      req.LastName,
        TenantID:      tenantID,
        Role:          "user", // Default role
        IsActive:      true,
        EmailVerified: false,
        CreatedAt:     time.Now(),
        UpdatedAt:     time.Now(),
    }
    
    if err := s.userRepo.Create(user); err != nil {
        s.logger.Error("Failed to create user", zap.Error(err))
        return nil, err
    }
    
    return user, nil
}

// RefreshToken generates new tokens using a refresh token
func (s *AuthService) RefreshToken(refreshToken string) (*models.TokenResponse, error) {
    // Validate refresh token
    userID, err := s.tokenRepo.ValidateRefreshToken(refreshToken)
    if err != nil {
        return nil, ErrInvalidToken
    }
    
    // Get user
    user, err := s.userRepo.FindByID(userID)
    if err != nil {
        return nil, err
    }
    
    if !user.IsActive {
        return nil, ErrUserNotActive
    }
    
    // Generate new tokens
    accessToken, err := s.generateAccessToken(user)
    if err != nil {
        return nil, err
    }
    
    newRefreshToken, err := s.generateRefreshToken(user.ID)
    if err != nil {
        return nil, err
    }
    
    // Invalidate old refresh token
    s.tokenRepo.InvalidateRefreshToken(refreshToken)
    
    return &models.TokenResponse{
        AccessToken:  accessToken,
        RefreshToken: newRefreshToken,
        TokenType:    "Bearer",
        ExpiresIn:    int(s.tokenTTL.Seconds()),
    }, nil
}

// ValidateToken validates an access token and returns claims
func (s *AuthService) ValidateToken(tokenString string) (*models.Claims, error) {
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        // Explicitly validate algorithm to prevent algorithm confusion attacks
        if token.Method != jwt.SigningMethodHS256 {
            return nil, errors.New("unexpected signing method: only HS256 allowed")
        }
        return []byte(s.jwtSecret), nil
    })
    
    if err != nil {
        return nil, err
    }
    
    if !token.Valid {
        return nil, ErrInvalidToken
    }
    
    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return nil, ErrInvalidToken
    }
    
    // Check expiration
    exp, ok := claims["exp"].(float64)
    if !ok || time.Now().Unix() > int64(exp) {
        return nil, ErrTokenExpired
    }
    
    return &models.Claims{
        UserID:   claims["user_id"].(string),
        Email:    claims["email"].(string),
        Role:     claims["role"].(string),
        TenantID: claims["tenant_id"].(string),
        Exp:      int64(exp),
        Iat:      int64(claims["iat"].(float64)),
    }, nil
}

// generateAccessToken creates a new JWT access token
func (s *AuthService) generateAccessToken(user *models.User) (string, error) {
    claims := jwt.MapClaims{
        "user_id":   user.ID,
        "email":     user.Email,
        "role":      user.Role,
        "tenant_id": user.TenantID,
        "iat":       time.Now().Unix(),
        "exp":       time.Now().Add(s.tokenTTL).Unix(),
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(s.jwtSecret))
}

// generateRefreshToken creates a new refresh token
func (s *AuthService) generateRefreshToken(userID string) (string, error) {
    // Generate random token
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    
    token := base64.URLEncoding.EncodeToString(b)
    
    // Store in repository
    if err := s.tokenRepo.StoreRefreshToken(token, userID, s.refreshTTL); err != nil {
        return "", err
    }
    
    return token, nil
}

// Logout invalidates tokens
func (s *AuthService) Logout(refreshToken string) error {
    return s.tokenRepo.InvalidateRefreshToken(refreshToken)
}

// GetUserByID gets a user by their ID
func (s *AuthService) GetUserByID(userID string) (*models.User, error) {
    return s.userRepo.FindByID(userID)
}

// UpdateUserProfile updates a user's profile information
func (s *AuthService) UpdateUserProfile(userID string, req *models.UpdateProfileRequest) (*models.User, error) {
    user, err := s.userRepo.FindByID(userID)
    if err != nil {
        return nil, err
    }
    
    // Update fields if provided
    if req.FirstName != "" {
        user.FirstName = req.FirstName
    }
    if req.LastName != "" {
        user.LastName = req.LastName
    }
    
    user.UpdatedAt = time.Now()
    
    if err := s.userRepo.Update(user); err != nil {
        return nil, err
    }
    
    return user, nil
}

// ChangePassword changes a user's password
func (s *AuthService) ChangePassword(userID string, req *models.ChangePasswordRequest) error {
    user, err := s.userRepo.FindByID(userID)
    if err != nil {
        return err
    }
    
    // Verify current password
    if !models.CheckPassword(req.CurrentPassword, user.PasswordHash) {
        return errors.New("current password is incorrect")
    }
    
    // Hash new password
    newHash, err := models.HashPassword(req.NewPassword)
    if err != nil {
        return err
    }
    
    // Update password
    user.PasswordHash = newHash
    user.UpdatedAt = time.Now()
    
    return s.userRepo.Update(user)
}