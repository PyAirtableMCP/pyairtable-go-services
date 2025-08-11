package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// MFAService handles Multi-Factor Authentication operations
type MFAService struct {
	logger       *zap.Logger
	issuer       string
	backupCodes  *BackupCodeService
}

// MFAConfig holds MFA configuration
type MFAConfig struct {
	Issuer          string
	Period          uint   // TOTP period in seconds (default: 30)
	SecretLength    int    // Secret length in bytes (default: 20)
	BackupCodeCount int    // Number of backup codes to generate (default: 10)
	BackupCodeLength int   // Length of each backup code (default: 8)
}

// MFASecret represents a generated MFA secret
type MFASecret struct {
	Secret      string `json:"secret"`
	QRCodeURL   string `json:"qr_code_url"`
	BackupCodes []string `json:"backup_codes"`
}

// MFAVerification represents MFA verification result
type MFAVerification struct {
	Valid       bool   `json:"valid"`
	UsedBackup  bool   `json:"used_backup"`
	BackupCode  string `json:"backup_code,omitempty"`
	LastUsed    time.Time `json:"last_used"`
}

// BackupCodeService manages backup codes
type BackupCodeService struct {
	logger *zap.Logger
}

// NewMFAService creates a new MFA service
func NewMFAService(config *MFAConfig, logger *zap.Logger) *MFAService {
	if config == nil {
		config = &MFAConfig{
			Issuer:           "PyAirtable",
			Period:           30,
			SecretLength:     20,
			BackupCodeCount:  10,
			BackupCodeLength: 8,
		}
	}
	
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MFAService{
		logger:      logger,
		issuer:      config.Issuer,
		backupCodes: &BackupCodeService{logger: logger},
	}
}

// GenerateSecret generates a new MFA secret for a user
func (m *MFAService) GenerateSecret(userEmail string) (*MFASecret, error) {
	// Generate secret
	secret := make([]byte, 20) // 160-bit secret
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	secretBase32 := base32.StdEncoding.EncodeToString(secret)

	// Generate TOTP key
	key, err := otp.NewKeyFromURL(fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s",
		m.issuer, userEmail, secretBase32, m.issuer,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create TOTP key: %w", err)
	}

	// Generate QR code URL
	qrURL := key.URL()

	// Generate backup codes
	backupCodes, err := m.backupCodes.Generate(10, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	m.logger.Info("MFA secret generated",
		zap.String("user_email", userEmail),
		zap.Int("backup_codes_count", len(backupCodes)),
	)

	return &MFASecret{
		Secret:      secretBase32,
		QRCodeURL:   qrURL,
		BackupCodes: backupCodes,
	}, nil
}

// VerifyTOTP verifies a TOTP code
func (m *MFAService) VerifyTOTP(secret, code string) (*MFAVerification, error) {
	// Clean up the code (remove spaces, convert to uppercase)
	cleanCode := strings.ReplaceAll(strings.ToUpper(code), " ", "")

	// Verify TOTP
	valid := totp.Validate(cleanCode, secret, time.Now())
	
	result := &MFAVerification{
		Valid:    valid,
		LastUsed: time.Now(),
	}

	if valid {
		m.logger.Info("TOTP verification successful")
	} else {
		m.logger.Warn("TOTP verification failed", zap.String("code", cleanCode))
	}

	return result, nil
}

// VerifyWithBackup verifies using a backup code
func (m *MFAService) VerifyWithBackup(userID string, code string, hashedBackupCodes []string) (*MFAVerification, error) {
	// Clean up the code
	cleanCode := strings.ReplaceAll(strings.ToUpper(code), " ", "")

	for i, hashedCode := range hashedBackupCodes {
		if m.backupCodes.Verify(cleanCode, hashedCode) {
			m.logger.Info("Backup code verification successful",
				zap.String("user_id", userID),
				zap.Int("code_index", i),
			)

			return &MFAVerification{
				Valid:      true,
				UsedBackup: true,
				BackupCode: cleanCode,
				LastUsed:   time.Now(),
			}, nil
		}
	}

	m.logger.Warn("Backup code verification failed",
		zap.String("user_id", userID),
		zap.String("code", cleanCode),
	)

	return &MFAVerification{
		Valid:    false,
		LastUsed: time.Now(),
	}, nil
}

// VerifyMFA verifies MFA using either TOTP or backup code
func (m *MFAService) VerifyMFA(userID, secret, code string, hashedBackupCodes []string) (*MFAVerification, error) {
	// Try TOTP first
	if len(code) == 6 && isNumeric(code) {
		result, err := m.VerifyTOTP(secret, code)
		if err != nil {
			return nil, err
		}
		if result.Valid {
			return result, nil
		}
	}

	// Try backup codes if TOTP fails or code looks like backup code
	if len(hashedBackupCodes) > 0 {
		return m.VerifyWithBackup(userID, code, hashedBackupCodes)
	}

	return &MFAVerification{
		Valid:    false,
		LastUsed: time.Now(),
	}, nil
}

// Generate creates new backup codes
func (b *BackupCodeService) Generate(count, length int) ([]string, error) {
	codes := make([]string, count)
	
	for i := 0; i < count; i++ {
		code, err := b.generateSingleCode(length)
		if err != nil {
			return nil, fmt.Errorf("failed to generate backup code %d: %w", i, err)
		}
		codes[i] = code
	}

	return codes, nil
}

// generateSingleCode generates a single backup code
func (b *BackupCodeService) generateSingleCode(length int) (string, error) {
	// Use alphanumeric characters (excluding confusing ones like 0, O, 1, I, l)
	const charset = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}

	return string(bytes), nil
}

// Hash creates a bcrypt hash of backup codes for storage
func (b *BackupCodeService) Hash(codes []string) ([]string, error) {
	hashedCodes := make([]string, len(codes))
	
	for i, code := range codes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash backup code %d: %w", i, err)
		}
		hashedCodes[i] = string(hash)
	}

	return hashedCodes, nil
}

// Verify checks if a backup code matches its hash
func (b *BackupCodeService) Verify(code, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(code))
	return err == nil
}

// MFAEnforcementPolicy defines MFA enforcement rules
type MFAEnforcementPolicy struct {
	EnforceForRoles    []string      `json:"enforce_for_roles"`
	EnforceAfterDays   int           `json:"enforce_after_days"`
	GracePeriodDays    int           `json:"grace_period_days"`
	RequireForAdmins   bool          `json:"require_for_admins"`
	RequireForAPI      bool          `json:"require_for_api"`
	ExemptIPs          []string      `json:"exempt_ips"`
	ExemptUserAgents   []string      `json:"exempt_user_agents"`
}

// MFAEnforcer manages MFA enforcement policies
type MFAEnforcer struct {
	policy *MFAEnforcementPolicy
	logger *zap.Logger
}

// NewMFAEnforcer creates a new MFA enforcer
func NewMFAEnforcer(policy *MFAEnforcementPolicy, logger *zap.Logger) *MFAEnforcer {
	if policy == nil {
		policy = &MFAEnforcementPolicy{
			EnforceForRoles:  []string{"admin", "owner"},
			EnforceAfterDays: 30,
			GracePeriodDays:  7,
			RequireForAdmins: true,
			RequireForAPI:    false,
		}
	}

	return &MFAEnforcer{
		policy: policy,
		logger: logger,
	}
}

// ShouldEnforceMFA determines if MFA should be enforced for a user
func (e *MFAEnforcer) ShouldEnforceMFA(userRole, userIP, userAgent string, accountAge time.Duration) bool {
	// Always enforce for admins if policy requires
	if e.policy.RequireForAdmins && (userRole == "admin" || userRole == "owner") {
		return true
	}

	// Check role-based enforcement
	for _, role := range e.policy.EnforceForRoles {
		if userRole == role {
			return true
		}
	}

	// Check account age enforcement
	if e.policy.EnforceAfterDays > 0 {
		enforcementAge := time.Duration(e.policy.EnforceAfterDays) * 24 * time.Hour
		gracePeriod := time.Duration(e.policy.GracePeriodDays) * 24 * time.Hour
		
		if accountAge > enforcementAge+gracePeriod {
			return true
		}
	}

	// Check IP exemptions
	for _, exemptIP := range e.policy.ExemptIPs {
		if userIP == exemptIP {
			return false
		}
	}

	return false
}

// MFASession manages MFA verification state in user sessions
type MFASession struct {
	UserID         string    `json:"user_id"`
	MFAVerified    bool      `json:"mfa_verified"`
	VerifiedAt     time.Time `json:"verified_at"`
	RequiresMFA    bool      `json:"requires_mfa"`
	BackupUsed     bool      `json:"backup_used"`
	BackupCode     string    `json:"backup_code,omitempty"`
}

// IsValid checks if MFA session is still valid
func (s *MFASession) IsValid(maxAge time.Duration) bool {
	if !s.MFAVerified {
		return false
	}
	
	return time.Since(s.VerifiedAt) <= maxAge
}

// GenerateRecoveryCodes generates new recovery codes for a user
func (m *MFAService) GenerateRecoveryCodes(userID string) ([]string, error) {
	codes, err := m.backupCodes.Generate(10, 8)
	if err != nil {
		return nil, err
	}

	m.logger.Info("Recovery codes generated",
		zap.String("user_id", userID),
		zap.Int("count", len(codes)),
	)

	return codes, nil
}

// ValidateSecretStrength validates MFA secret strength
func (m *MFAService) ValidateSecretStrength(secret string) error {
	if len(secret) < 32 { // Base32 encoded 160-bit secret
		return fmt.Errorf("secret too short: minimum 32 characters required")
	}

	// Decode and check entropy
	decoded, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return fmt.Errorf("invalid secret format: %w", err)
	}

	if len(decoded) < 20 { // 160 bits
		return fmt.Errorf("insufficient entropy: minimum 160 bits required")
	}

	return nil
}

// Utility functions

// isNumeric checks if string contains only digits
func isNumeric(s string) bool {
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// HashSecret creates a hash of the MFA secret for secure storage
func HashSecret(secret string) string {
	hash := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}

// Example integration with user service
type UserMFA struct {
	UserID         string    `json:"user_id"`
	Secret         string    `json:"secret"`         // Encrypted/hashed
	BackupCodes    []string  `json:"backup_codes"`   // Hashed
	Enabled        bool      `json:"enabled"`
	EnabledAt      time.Time `json:"enabled_at"`
	LastUsed       time.Time `json:"last_used"`
	LastBackupUsed string    `json:"last_backup_used"` // Hash of last used backup code
}

// EnableMFA enables MFA for a user
func (m *MFAService) EnableMFA(userID, secret string, backupCodes []string) error {
	// Validate secret strength
	if err := m.ValidateSecretStrength(secret); err != nil {
		return fmt.Errorf("invalid secret: %w", err)
	}

	// Hash backup codes for storage
	hashedCodes, err := m.backupCodes.Hash(backupCodes)
	if err != nil {
		return fmt.Errorf("failed to hash backup codes: %w", err)
	}

	m.logger.Info("MFA enabled for user",
		zap.String("user_id", userID),
		zap.Int("backup_codes", len(hashedCodes)),
	)

	// Here you would save to database:
	// - Encrypted secret
	// - Hashed backup codes
	// - Enabled timestamp

	return nil
}

// DisableMFA disables MFA for a user
func (m *MFAService) DisableMFA(userID string) error {
	m.logger.Info("MFA disabled for user", zap.String("user_id", userID))
	
	// Here you would:
	// - Remove MFA secret from database
	// - Invalidate backup codes
	// - Log security event

	return nil
}