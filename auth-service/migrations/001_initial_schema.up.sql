-- Migration: 001_initial_schema.up.sql
-- Description: Initial schema for auth service with user authentication, sessions, and audit logging
-- Created: 2025-08-12
-- Author: Database Administrator

-- Enable UUID extension for unique identifiers
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Enable pgcrypto for password hashing
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create custom types for better data integrity
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended', 'pending_verification');
CREATE TYPE session_status AS ENUM ('active', 'expired', 'revoked', 'invalid');
CREATE TYPE audit_action AS ENUM ('create', 'read', 'update', 'delete', 'login', 'logout', 'password_change', 'password_reset', 'account_lock', 'account_unlock');

-- =============================================
-- USERS TABLE - Core user authentication data
-- =============================================
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(320) NOT NULL UNIQUE, -- RFC 5321 max email length
    username VARCHAR(50) UNIQUE,
    password_hash VARCHAR(255) NOT NULL, -- bcrypt hash with salt
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    phone VARCHAR(20),
    
    -- Status and verification
    status user_status NOT NULL DEFAULT 'pending_verification',
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    email_verification_token VARCHAR(255),
    email_verification_expires_at TIMESTAMP WITH TIME ZONE,
    
    -- Password management
    password_reset_token VARCHAR(255) UNIQUE,
    password_reset_expires_at TIMESTAMP WITH TIME ZONE,
    password_changed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Security tracking
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    account_locked_until TIMESTAMP WITH TIME ZONE,
    last_login_at TIMESTAMP WITH TIME ZONE,
    last_login_ip INET,
    
    -- Multi-factor authentication
    mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_secret VARCHAR(32), -- TOTP secret
    mfa_backup_codes TEXT[], -- Array of backup codes
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE, -- Soft delete
    
    -- JSON metadata for extensibility
    metadata JSONB DEFAULT '{}',
    
    -- Constraints
    CONSTRAINT check_email_format CHECK (email ~* '^[A-Za-z0-9._%-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$'),
    CONSTRAINT check_phone_format CHECK (phone IS NULL OR phone ~* '^\+?[1-9]\d{1,14}$'),
    CONSTRAINT check_failed_attempts CHECK (failed_login_attempts >= 0),
    CONSTRAINT check_password_reset_token_expiry CHECK (
        (password_reset_token IS NULL AND password_reset_expires_at IS NULL) OR
        (password_reset_token IS NOT NULL AND password_reset_expires_at IS NOT NULL)
    ),
    CONSTRAINT check_email_verification_token_expiry CHECK (
        (email_verification_token IS NULL AND email_verification_expires_at IS NULL) OR
        (email_verification_token IS NOT NULL AND email_verification_expires_at IS NOT NULL)
    )
);

-- =============================================
-- USER SESSIONS TABLE - Active user sessions
-- =============================================
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Session identification
    session_token VARCHAR(255) NOT NULL UNIQUE,
    refresh_token VARCHAR(255) UNIQUE,
    device_id VARCHAR(255), -- Device fingerprint
    
    -- Session metadata
    ip_address INET NOT NULL,
    user_agent TEXT,
    device_type VARCHAR(50), -- 'desktop', 'mobile', 'tablet', 'api'
    platform VARCHAR(50), -- 'web', 'ios', 'android', 'api'
    
    -- Session lifecycle
    status session_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_activity_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoked_by UUID REFERENCES users(id), -- Admin who revoked the session
    revoke_reason VARCHAR(255),
    
    -- Security features
    is_persistent BOOLEAN NOT NULL DEFAULT FALSE, -- Remember me functionality
    location_data JSONB, -- Geolocation data
    
    -- Constraints
    CONSTRAINT check_expires_at_future CHECK (expires_at > created_at),
    CONSTRAINT check_revoked_consistency CHECK (
        (status = 'revoked' AND revoked_at IS NOT NULL) OR
        (status != 'revoked' AND revoked_at IS NULL)
    )
);

-- =============================================
-- AUDIT LOG TABLE - Security and compliance tracking
-- =============================================
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Actor information
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    session_id UUID REFERENCES user_sessions(id) ON DELETE SET NULL,
    
    -- Action details
    action audit_action NOT NULL,
    resource_type VARCHAR(100) NOT NULL, -- 'user', 'session', 'permission', etc.
    resource_id VARCHAR(255), -- ID of the affected resource
    
    -- Request metadata
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(255), -- Correlation ID for request tracing
    
    -- Audit details
    description TEXT,
    old_values JSONB, -- Previous state (for updates)
    new_values JSONB, -- New state (for creates/updates)
    
    -- Success/failure tracking
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT,
    
    -- Timing
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Additional context
    metadata JSONB DEFAULT '{}'
);

-- =============================================
-- INDEXES FOR PERFORMANCE
-- =============================================

-- Users table indexes (optimized for performance)
CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_status ON users(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_password_reset_token ON users(password_reset_token) WHERE password_reset_token IS NOT NULL;
CREATE INDEX idx_users_email_verification_token ON users(email_verification_token) WHERE email_verification_token IS NOT NULL;

-- User sessions table indexes (optimized for core queries)
CREATE INDEX idx_sessions_user_id ON user_sessions(user_id);
CREATE INDEX idx_sessions_token ON user_sessions(session_token);
CREATE INDEX idx_sessions_refresh_token ON user_sessions(refresh_token) WHERE refresh_token IS NOT NULL;
CREATE INDEX idx_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX idx_sessions_user_status ON user_sessions(user_id, status) WHERE status = 'active';

-- Audit logs table indexes (essential queries only)
CREATE INDEX idx_audit_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_created_at ON audit_logs(created_at);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_request_id ON audit_logs(request_id) WHERE request_id IS NOT NULL;

-- Composite indexes for critical queries
CREATE INDEX idx_users_email_status ON users(email, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_sessions_user_active ON user_sessions(user_id, status, expires_at) WHERE status = 'active';

-- =============================================
-- TRIGGERS FOR AUTOMATIC UPDATES
-- =============================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger for users table
CREATE TRIGGER update_users_updated_at 
    BEFORE UPDATE ON users 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Function to update last_activity_at on session access
CREATE OR REPLACE FUNCTION update_session_activity()
RETURNS TRIGGER AS $$
BEGIN
    NEW.last_activity_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger for session activity tracking
CREATE TRIGGER update_session_last_activity 
    BEFORE UPDATE ON user_sessions 
    FOR EACH ROW 
    EXECUTE FUNCTION update_session_activity();

-- =============================================
-- FUNCTIONS FOR COMMON OPERATIONS
-- =============================================

-- Function to expire old sessions
CREATE OR REPLACE FUNCTION expire_old_sessions()
RETURNS INTEGER AS $$
DECLARE
    expired_count INTEGER;
BEGIN
    UPDATE user_sessions 
    SET status = 'expired'
    WHERE status = 'active' 
    AND expires_at < CURRENT_TIMESTAMP;
    
    GET DIAGNOSTICS expired_count = ROW_COUNT;
    RETURN expired_count;
END;
$$ LANGUAGE plpgsql;

-- Function to clean up old audit logs (for retention policy)
CREATE OR REPLACE FUNCTION cleanup_old_audit_logs(retention_days INTEGER DEFAULT 365)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM audit_logs 
    WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '1 day' * retention_days;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to lock user account after failed attempts
CREATE OR REPLACE FUNCTION lock_user_account(user_email VARCHAR, lock_duration_minutes INTEGER DEFAULT 30)
RETURNS BOOLEAN AS $$
DECLARE
    user_found BOOLEAN;
BEGIN
    UPDATE users 
    SET account_locked_until = CURRENT_TIMESTAMP + INTERVAL '1 minute' * lock_duration_minutes
    WHERE email = user_email AND deleted_at IS NULL;
    
    GET DIAGNOSTICS user_found = FOUND;
    RETURN user_found;
END;
$$ LANGUAGE plpgsql;

-- =============================================
-- VIEWS FOR COMMON QUERIES
-- =============================================

-- Active users view
CREATE VIEW active_users AS
SELECT 
    id, email, username, first_name, last_name, 
    status, email_verified, mfa_enabled,
    last_login_at, created_at, updated_at
FROM users 
WHERE deleted_at IS NULL 
AND status = 'active';

-- Active sessions with user info
CREATE VIEW active_sessions_with_users AS
SELECT 
    s.id as session_id,
    s.session_token,
    s.device_type,
    s.platform,
    s.ip_address,
    s.created_at as session_created_at,
    s.expires_at,
    s.last_activity_at,
    u.id as user_id,
    u.email,
    u.username,
    u.first_name,
    u.last_name
FROM user_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.status = 'active' 
AND s.expires_at > CURRENT_TIMESTAMP
AND u.deleted_at IS NULL;

-- Recent audit events view
CREATE VIEW recent_audit_events AS
SELECT 
    a.id,
    a.action,
    a.resource_type,
    a.resource_id,
    a.description,
    a.success,
    a.ip_address,
    a.created_at,
    u.email as user_email,
    u.username
FROM audit_logs a
LEFT JOIN users u ON a.user_id = u.id
WHERE a.created_at > CURRENT_TIMESTAMP - INTERVAL '7 days'
ORDER BY a.created_at DESC;

-- =============================================
-- SECURITY NOTICE
-- =============================================
-- IMPORTANT: This migration does NOT create default admin users for security reasons.
-- Admin users should be created manually after deployment using secure procedures.
-- Refer to the operational documentation for admin user creation procedures.

-- =============================================
-- COMMENTS FOR DOCUMENTATION
-- =============================================

COMMENT ON TABLE users IS 'Core user authentication and profile data with security features';
COMMENT ON TABLE user_sessions IS 'Active user sessions with device tracking and security metadata';
COMMENT ON TABLE audit_logs IS 'Security audit trail for compliance and monitoring';

COMMENT ON COLUMN users.email IS 'Primary email address for authentication and communication';
COMMENT ON COLUMN users.password_hash IS 'bcrypt hashed password with salt';
COMMENT ON COLUMN users.failed_login_attempts IS 'Counter for failed login attempts (reset on successful login)';
COMMENT ON COLUMN users.account_locked_until IS 'Timestamp until which account is locked due to failed attempts';
COMMENT ON COLUMN users.metadata IS 'Extensible JSON field for additional user properties';

COMMENT ON COLUMN user_sessions.session_token IS 'Primary session identifier (JWT or random token)';
COMMENT ON COLUMN user_sessions.refresh_token IS 'Token used to refresh expired session tokens';
COMMENT ON COLUMN user_sessions.device_id IS 'Unique device fingerprint for security tracking';
COMMENT ON COLUMN user_sessions.is_persistent IS 'Whether session should survive browser restart';

COMMENT ON COLUMN audit_logs.request_id IS 'Correlation ID for tracking requests across services';
COMMENT ON COLUMN audit_logs.old_values IS 'Previous state of resource before change';
COMMENT ON COLUMN audit_logs.new_values IS 'New state of resource after change';