-- Migration: Create user profiles table
-- Version: 001  
-- Description: Extended user profile information

-- Up Migration
CREATE TABLE IF NOT EXISTS user_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID UNIQUE NOT NULL, -- References auth service user
    email VARCHAR(255) UNIQUE NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    avatar TEXT,
    bio TEXT,
    phone VARCHAR(50),
    tenant_id UUID NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    is_active BOOLEAN DEFAULT true,
    email_verified BOOLEAN DEFAULT false,
    phone_verified BOOLEAN DEFAULT false,
    preferences JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    last_login_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX idx_user_profiles_email ON user_profiles(email);
CREATE INDEX idx_user_profiles_tenant_id ON user_profiles(tenant_id);
CREATE INDEX idx_user_profiles_role ON user_profiles(role);
CREATE INDEX idx_user_profiles_active ON user_profiles(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_profiles_created_at ON user_profiles(created_at DESC);
CREATE INDEX idx_user_profiles_search ON user_profiles USING gin(
    to_tsvector('english', coalesce(first_name, '') || ' ' || 
                          coalesce(last_name, '') || ' ' || 
                          coalesce(display_name, '') || ' ' ||
                          coalesce(email, ''))
);

-- Updated at trigger
CREATE TRIGGER update_user_profiles_updated_at BEFORE UPDATE
    ON user_profiles FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE user_profiles ENABLE ROW LEVEL SECURITY;

-- Policy: Users can view their own profile
CREATE POLICY user_view_own_profile ON user_profiles
    FOR SELECT
    USING (user_id::text = current_setting('app.current_user_id', true));

-- Policy: Users can view profiles in their tenant
CREATE POLICY user_view_tenant_profiles ON user_profiles
    FOR SELECT
    USING (tenant_id::text = current_setting('app.current_tenant_id', true));

-- Policy: Users can update their own profile
CREATE POLICY user_update_own_profile ON user_profiles
    FOR UPDATE
    USING (user_id::text = current_setting('app.current_user_id', true));

-- Comments
COMMENT ON TABLE user_profiles IS 'Extended user profile information';
COMMENT ON COLUMN user_profiles.user_id IS 'Reference to auth service user ID';
COMMENT ON COLUMN user_profiles.preferences IS 'User preferences (theme, language, etc)';
COMMENT ON COLUMN user_profiles.metadata IS 'Additional metadata for extensibility';

-- Down Migration (commented out for safety)
-- DROP TABLE IF EXISTS user_profiles CASCADE;