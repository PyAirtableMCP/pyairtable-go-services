package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// JWTConfig holds JWT validation configuration
type JWTConfig struct {
	Issuer          string        `json:"issuer"`
	Audience        string        `json:"audience"`
	JWKSEndpoint    string        `json:"jwks_endpoint"`
	Algorithm       string        `json:"algorithm"`
	ClockSkew       time.Duration `json:"clock_skew"`
	CacheTTL        time.Duration `json:"cache_ttl"`
	RequiredClaims  []string      `json:"required_claims"`
	CustomValidator func(claims jwt.MapClaims) error `json:"-"`
}

// OAuth2Config holds OAuth2/OIDC configuration
type OAuth2Config struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSEndpoint          string `json:"jwks_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	Scopes                []string `json:"scopes"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	X5c []string `json:"x5c"`
	X5t string `json:"x5t"`
}

// JWKSet represents a set of JSON Web Keys
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// EnhancedJWTValidator provides advanced JWT validation with OIDC support
type EnhancedJWTValidator struct {
	config      *JWTConfig
	oauth2Config *OAuth2Config
	redis       *redis.Client
	keyCache    map[string]*rsa.PublicKey
	cacheMu     sync.RWMutex
	httpClient  *http.Client
}

// UserClaims represents extracted user claims from JWT
type UserClaims struct {
	UserID      string                 `json:"user_id"`
	TenantID    string                 `json:"tenant_id"`
	Email       string                 `json:"email"`
	Username    string                 `json:"username"`
	Roles       []string               `json:"roles"`
	Permissions []string               `json:"permissions"`
	Scopes      []string               `json:"scopes"`
	Custom      map[string]interface{} `json:"custom"`
	
	// Standard JWT claims
	jwt.RegisteredClaims
}

// ValidationResult contains the result of JWT validation
type ValidationResult struct {
	Valid    bool        `json:"valid"`
	Claims   *UserClaims `json:"claims"`
	Token    *jwt.Token  `json:"-"`
	Error    string      `json:"error,omitempty"`
}

// NewEnhancedJWTValidator creates a new enhanced JWT validator
func NewEnhancedJWTValidator(config *JWTConfig, oauth2Config *OAuth2Config, redis *redis.Client) *EnhancedJWTValidator {
	return &EnhancedJWTValidator{
		config:       config,
		oauth2Config: oauth2Config,
		redis:        redis,
		keyCache:     make(map[string]*rsa.PublicKey),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ValidateToken validates a JWT token with enhanced features
func (v *EnhancedJWTValidator) ValidateToken(tokenString string) (*ValidationResult, error) {
	// Parse token without verification first to get the header
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Sprintf("failed to parse token: %v", err),
		}, nil
	}
	
	// Get the key ID from token header
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return &ValidationResult{
			Valid: false,
			Error: "missing key ID in token header",
		}, nil
	}
	
	// Get the public key for verification
	publicKey, err := v.getPublicKey(kid)
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Sprintf("failed to get public key: %v", err),
		}, nil
	}
	
	// Parse and validate token with proper key
	validatedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})
	
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Sprintf("token validation failed: %v", err),
		}, nil
	}
	
	// Extract and validate claims
	claims, ok := validatedToken.Claims.(jwt.MapClaims)
	if !ok || !validatedToken.Valid {
		return &ValidationResult{
			Valid: false,
			Error: "invalid token claims",
		}, nil
	}
	
	// Validate standard claims
	if err := v.validateStandardClaims(claims); err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Sprintf("standard claims validation failed: %v", err),
		}, nil
	}
	
	// Validate required claims
	if err := v.validateRequiredClaims(claims); err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Sprintf("required claims validation failed: %v", err),
		}, nil
	}
	
	// Apply custom validation if configured
	if v.config.CustomValidator != nil {
		if err := v.config.CustomValidator(claims); err != nil {
			return &ValidationResult{
				Valid: false,
				Error: fmt.Sprintf("custom validation failed: %v", err),
			}, nil
		}
	}
	
	// Extract user claims
	userClaims := v.extractUserClaims(claims)
	
	return &ValidationResult{
		Valid:  true,
		Claims: userClaims,
		Token:  validatedToken,
	}, nil
}

// Middleware creates a Fiber middleware for JWT validation
func (v *EnhancedJWTValidator) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract token from request
		token := v.extractTokenFromRequest(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing or invalid authorization token",
			})
		}
		
		// Validate token
		result, err := v.ValidateToken(token)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "token validation error",
			})
		}
		
		if !result.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": result.Error,
			})
		}
		
		// Store claims in context
		c.Locals("user_claims", result.Claims)
		c.Locals("jwt_token", result.Token)
		
		return c.Next()
	}
}

// RequireScopes creates middleware that requires specific scopes
func (v *EnhancedJWTValidator) RequireScopes(requiredScopes ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userClaims, ok := c.Locals("user_claims").(*UserClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "no user claims found",
			})
		}
		
		// Check if user has required scopes
		if !v.hasRequiredScopes(userClaims.Scopes, requiredScopes) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "insufficient scopes",
				"required_scopes": requiredScopes,
				"user_scopes": userClaims.Scopes,
			})
		}
		
		return c.Next()
	}
}

// RequireRoles creates middleware that requires specific roles
func (v *EnhancedJWTValidator) RequireRoles(requiredRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userClaims, ok := c.Locals("user_claims").(*UserClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "no user claims found",
			})
		}
		
		// Check if user has required roles
		if !v.hasRequiredRoles(userClaims.Roles, requiredRoles) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "insufficient roles",
				"required_roles": requiredRoles,
				"user_roles": userClaims.Roles,
			})
		}
		
		return c.Next()
	}
}

// RequirePermissions creates middleware that requires specific permissions
func (v *EnhancedJWTValidator) RequirePermissions(requiredPermissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userClaims, ok := c.Locals("user_claims").(*UserClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "no user claims found",
			})
		}
		
		// Check if user has required permissions
		if !v.hasRequiredPermissions(userClaims.Permissions, requiredPermissions) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "insufficient permissions",
				"required_permissions": requiredPermissions,
				"user_permissions": userClaims.Permissions,
			})
		}
		
		return c.Next()
	}
}

// getPublicKey retrieves and caches public keys from JWKS endpoint
func (v *EnhancedJWTValidator) getPublicKey(kid string) (*rsa.PublicKey, error) {
	// Check cache first
	v.cacheMu.RLock()
	if key, exists := v.keyCache[kid]; exists {
		v.cacheMu.RUnlock()
		return key, nil
	}
	v.cacheMu.RUnlock()
	
	// Check Redis cache
	if v.redis != nil {
		cacheKey := fmt.Sprintf("jwks:key:%s", kid)
		keyData, err := v.redis.Get(context.Background(), cacheKey).Result()
		if err == nil {
			// Deserialize key from cache
			key, err := v.deserializePublicKey(keyData)
			if err == nil {
				v.cacheMu.Lock()
				v.keyCache[kid] = key
				v.cacheMu.Unlock()
				return key, nil
			}
		}
	}
	
	// Fetch from JWKS endpoint
	jwks, err := v.fetchJWKS()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	
	// Find the key by kid
	var targetJWK *JWK
	for _, jwk := range jwks.Keys {
		if jwk.Kid == kid {
			targetJWK = &jwk
			break
		}
	}
	
	if targetJWK == nil {
		return nil, fmt.Errorf("key with kid %s not found", kid)
	}
	
	// Convert JWK to RSA public key
	publicKey, err := v.jwkToRSAPublicKey(targetJWK)
	if err != nil {
		return nil, fmt.Errorf("failed to convert JWK to RSA public key: %w", err)
	}
	
	// Cache the key
	v.cacheMu.Lock()
	v.keyCache[kid] = publicKey
	v.cacheMu.Unlock()
	
	// Cache in Redis
	if v.redis != nil {
		cacheKey := fmt.Sprintf("jwks:key:%s", kid)
		keyData := v.serializePublicKey(publicKey)
		v.redis.Set(context.Background(), cacheKey, keyData, v.config.CacheTTL)
	}
	
	return publicKey, nil
}

// fetchJWKS fetches the JWKS from the configured endpoint
func (v *EnhancedJWTValidator) fetchJWKS() (*JWKSet, error) {
	resp, err := v.httpClient.Get(v.config.JWKSEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS response: %w", err)
	}
	
	var jwks JWKSet
	err = json.Unmarshal(body, &jwks)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWKS: %w", err)
	}
	
	return &jwks, nil
}

// jwkToRSAPublicKey converts a JWK to RSA public key
func (v *EnhancedJWTValidator) jwkToRSAPublicKey(jwk *JWK) (*rsa.PublicKey, error) {
	// This is a simplified implementation
	// In production, use a proper JWK library like "github.com/lestrrat-go/jwx"
	return nil, fmt.Errorf("JWK to RSA conversion not implemented - use proper JWK library")
}

// validateStandardClaims validates standard JWT claims
func (v *EnhancedJWTValidator) validateStandardClaims(claims jwt.MapClaims) error {
	now := time.Now()
	
	// Validate issuer
	if v.config.Issuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != v.config.Issuer {
			return fmt.Errorf("invalid issuer")
		}
	}
	
	// Validate audience
	if v.config.Audience != "" {
		if aud, ok := claims["aud"].(string); !ok || aud != v.config.Audience {
			// Also check if audience is an array
			if audArray, ok := claims["aud"].([]interface{}); ok {
				found := false
				for _, a := range audArray {
					if audStr, ok := a.(string); ok && audStr == v.config.Audience {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("invalid audience")
				}
			} else {
				return fmt.Errorf("invalid audience")
			}
		}
	}
	
	// Validate expiration with clock skew
	if exp, ok := claims["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		if now.After(expTime.Add(v.config.ClockSkew)) {
			return fmt.Errorf("token has expired")
		}
	}
	
	// Validate not before with clock skew
	if nbf, ok := claims["nbf"].(float64); ok {
		nbfTime := time.Unix(int64(nbf), 0)
		if now.Before(nbfTime.Add(-v.config.ClockSkew)) {
			return fmt.Errorf("token not yet valid")
		}
	}
	
	// Validate issued at with clock skew
	if iat, ok := claims["iat"].(float64); ok {
		iatTime := time.Unix(int64(iat), 0)
		if now.Before(iatTime.Add(-v.config.ClockSkew)) {
			return fmt.Errorf("token issued in the future")
		}
	}
	
	return nil
}

// validateRequiredClaims validates that required claims are present
func (v *EnhancedJWTValidator) validateRequiredClaims(claims jwt.MapClaims) error {
	for _, requiredClaim := range v.config.RequiredClaims {
		if _, exists := claims[requiredClaim]; !exists {
			return fmt.Errorf("missing required claim: %s", requiredClaim)
		}
	}
	return nil
}

// extractUserClaims extracts user-specific claims from JWT
func (v *EnhancedJWTValidator) extractUserClaims(claims jwt.MapClaims) *UserClaims {
	userClaims := &UserClaims{
		Custom: make(map[string]interface{}),
	}
	
	// Extract standard claims
	if sub, ok := claims["sub"].(string); ok {
		userClaims.UserID = sub
	}
	
	if email, ok := claims["email"].(string); ok {
		userClaims.Email = email
	}
	
	if username, ok := claims["preferred_username"].(string); ok {
		userClaims.Username = username
	} else if username, ok := claims["username"].(string); ok {
		userClaims.Username = username
	}
	
	// Extract tenant ID (custom claim)
	if tenantID, ok := claims["tenant_id"].(string); ok {
		userClaims.TenantID = tenantID
	}
	
	// Extract roles
	if rolesInterface, ok := claims["roles"]; ok {
		userClaims.Roles = v.extractStringArray(rolesInterface)
	}
	
	// Extract permissions
	if permissionsInterface, ok := claims["permissions"]; ok {
		userClaims.Permissions = v.extractStringArray(permissionsInterface)
	}
	
	// Extract scopes
	if scopesInterface, ok := claims["scope"]; ok {
		if scopeString, ok := scopesInterface.(string); ok {
			userClaims.Scopes = strings.Split(scopeString, " ")
		}
	} else if scopesInterface, ok := claims["scopes"]; ok {
		userClaims.Scopes = v.extractStringArray(scopesInterface)
	}
	
	// Extract custom claims
	for key, value := range claims {
		if !v.isStandardClaim(key) {
			userClaims.Custom[key] = value
		}
	}
	
	return userClaims
}

// extractTokenFromRequest extracts JWT token from HTTP request
func (v *EnhancedJWTValidator) extractTokenFromRequest(c *fiber.Ctx) string {
	// Check Authorization header
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}
	
	// Check query parameter
	if token := c.Query("access_token"); token != "" {
		return token
	}
	
	// Check cookie
	if token := c.Cookies("access_token"); token != "" {
		return token
	}
	
	return ""
}

// Helper functions
func (v *EnhancedJWTValidator) extractStringArray(value interface{}) []string {
	switch v := value.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return v
	case string:
		return []string{v}
	default:
		return []string{}
	}
}

func (v *EnhancedJWTValidator) isStandardClaim(claim string) bool {
	standardClaims := map[string]bool{
		"iss": true, "sub": true, "aud": true, "exp": true, "nbf": true,
		"iat": true, "jti": true, "email": true, "preferred_username": true,
		"username": true, "scope": true, "scopes": true, "roles": true,
		"permissions": true, "tenant_id": true,
	}
	return standardClaims[claim]
}

func (v *EnhancedJWTValidator) hasRequiredScopes(userScopes, requiredScopes []string) bool {
	scopeSet := make(map[string]bool)
	for _, scope := range userScopes {
		scopeSet[scope] = true
	}
	
	for _, required := range requiredScopes {
		if !scopeSet[required] {
			return false
		}
	}
	
	return true
}

func (v *EnhancedJWTValidator) hasRequiredRoles(userRoles, requiredRoles []string) bool {
	roleSet := make(map[string]bool)
	for _, role := range userRoles {
		roleSet[role] = true
	}
	
	for _, required := range requiredRoles {
		if !roleSet[required] {
			return false
		}
	}
	
	return true
}

func (v *EnhancedJWTValidator) hasRequiredPermissions(userPermissions, requiredPermissions []string) bool {
	permissionSet := make(map[string]bool)
	for _, permission := range userPermissions {
		permissionSet[permission] = true
	}
	
	for _, required := range requiredPermissions {
		if !permissionSet[required] {
			return false
		}
	}
	
	return true
}

// Placeholder methods for key serialization (implement based on your needs)
func (v *EnhancedJWTValidator) serializePublicKey(key *rsa.PublicKey) string {
	// Implement proper serialization
	return ""
}

func (v *EnhancedJWTValidator) deserializePublicKey(data string) (*rsa.PublicKey, error) {
	// Implement proper deserialization
	return nil, fmt.Errorf("not implemented")
}