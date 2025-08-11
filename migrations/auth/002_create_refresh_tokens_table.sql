-- Migration: Create refresh tokens table
-- Version: 002
-- Description: Table for storing refresh tokens

-- Up Migration
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);
CREATE INDEX idx_refresh_tokens_active ON refresh_tokens(expires_at, revoked_at) 
    WHERE revoked_at IS NULL;

-- Cleanup function for expired tokens
CREATE OR REPLACE FUNCTION cleanup_expired_tokens() RETURNS void AS $$
BEGIN
    DELETE FROM refresh_tokens 
    WHERE expires_at < CURRENT_TIMESTAMP 
    OR revoked_at IS NOT NULL;
END;
$$ LANGUAGE plpgsql;

-- Comments
COMMENT ON TABLE refresh_tokens IS 'JWT refresh tokens for persistent sessions';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'SHA-256 hash of the refresh token';
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'Timestamp when token was revoked (null if active)';

-- Down Migration (commented out for safety)
-- DROP TABLE IF EXISTS refresh_tokens CASCADE;
-- DROP FUNCTION IF EXISTS cleanup_expired_tokens();