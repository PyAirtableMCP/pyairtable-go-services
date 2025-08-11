-- User projections table for read models
-- This table provides optimized queries for user data without needing to replay events

CREATE TABLE IF NOT EXISTS user_projections (
    id VARCHAR(255) PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    avatar VARCHAR(500),
    bio TEXT,
    phone VARCHAR(50),
    tenant_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    email_verified BOOLEAN DEFAULT FALSE,
    email_verified_at TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT FALSE,
    last_login_at TIMESTAMP WITH TIME ZONE,
    roles JSONB DEFAULT '[]',
    preferences JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    timezone VARCHAR(100) DEFAULT 'UTC',
    language VARCHAR(10) DEFAULT 'en',
    deactivation_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    event_version BIGINT NOT NULL DEFAULT 1
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_user_projections_tenant_id ON user_projections(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_projections_email ON user_projections(email);
CREATE INDEX IF NOT EXISTS idx_user_projections_status ON user_projections(status);
CREATE INDEX IF NOT EXISTS idx_user_projections_is_active ON user_projections(is_active);
CREATE INDEX IF NOT EXISTS idx_user_projections_email_verified ON user_projections(email_verified);
CREATE INDEX IF NOT EXISTS idx_user_projections_created_at ON user_projections(created_at);
CREATE INDEX IF NOT EXISTS idx_user_projections_updated_at ON user_projections(updated_at);
CREATE INDEX IF NOT EXISTS idx_user_projections_event_version ON user_projections(event_version);

-- Create GIN index for JSONB fields
CREATE INDEX IF NOT EXISTS idx_user_projections_roles_gin ON user_projections USING GIN(roles);
CREATE INDEX IF NOT EXISTS idx_user_projections_preferences_gin ON user_projections USING GIN(preferences);
CREATE INDEX IF NOT EXISTS idx_user_projections_metadata_gin ON user_projections USING GIN(metadata);

-- Create composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_user_projections_tenant_status ON user_projections(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_user_projections_tenant_active ON user_projections(tenant_id, is_active);

-- Add table comment
COMMENT ON TABLE user_projections IS 'Read model projection for user data optimized for queries';

-- Create workspace projections table
CREATE TABLE IF NOT EXISTS workspace_projections (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    slug VARCHAR(255),
    owner_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    is_public BOOLEAN DEFAULT FALSE,
    allow_invitations BOOLEAN DEFAULT TRUE,
    default_member_role VARCHAR(50) DEFAULT 'member',
    max_members INTEGER DEFAULT 100,
    member_count INTEGER DEFAULT 0,
    project_count INTEGER DEFAULT 0,
    custom_fields JSONB DEFAULT '{}',
    integrations JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    event_version BIGINT NOT NULL DEFAULT 1
);

-- Create indexes for workspace projections
CREATE INDEX IF NOT EXISTS idx_workspace_projections_tenant_id ON workspace_projections(tenant_id);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_owner_id ON workspace_projections(owner_id);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_status ON workspace_projections(status);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_slug ON workspace_projections(slug);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_created_at ON workspace_projections(created_at);

-- Create workspace members projection table
CREATE TABLE IF NOT EXISTS workspace_member_projections (
    id VARCHAR(255) PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    permissions JSONB DEFAULT '[]',
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    added_by VARCHAR(255),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    event_version BIGINT NOT NULL DEFAULT 1,
    UNIQUE(workspace_id, user_id)
);

-- Create indexes for workspace members
CREATE INDEX IF NOT EXISTS idx_workspace_member_projections_workspace_id ON workspace_member_projections(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_member_projections_user_id ON workspace_member_projections(user_id);
CREATE INDEX IF NOT EXISTS idx_workspace_member_projections_role ON workspace_member_projections(role);
CREATE INDEX IF NOT EXISTS idx_workspace_member_projections_status ON workspace_member_projections(status);

-- Create tenant projections table
CREATE TABLE IF NOT EXISTS tenant_projections (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    domain VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    subscription_plan VARCHAR(50) DEFAULT 'free',
    subscription_status VARCHAR(50) DEFAULT 'trial',
    trial_ends_at TIMESTAMP WITH TIME ZONE,
    current_users INTEGER DEFAULT 0,
    current_workspaces INTEGER DEFAULT 0,
    current_projects INTEGER DEFAULT 0,
    current_storage_gb DECIMAL(10,2) DEFAULT 0,
    current_api_calls INTEGER DEFAULT 0,
    max_users INTEGER DEFAULT 5,
    max_workspaces INTEGER DEFAULT 3,
    max_projects INTEGER DEFAULT 10,
    max_storage_gb INTEGER DEFAULT 1,
    max_api_calls INTEGER DEFAULT 1000,
    billing_email VARCHAR(255),
    billing_currency VARCHAR(3) DEFAULT 'USD',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    event_version BIGINT NOT NULL DEFAULT 1
);

-- Create indexes for tenant projections
CREATE INDEX IF NOT EXISTS idx_tenant_projections_slug ON tenant_projections(slug);
CREATE INDEX IF NOT EXISTS idx_tenant_projections_domain ON tenant_projections(domain);
CREATE INDEX IF NOT EXISTS idx_tenant_projections_status ON tenant_projections(status);
CREATE INDEX IF NOT EXISTS idx_tenant_projections_subscription_plan ON tenant_projections(subscription_plan);
CREATE INDEX IF NOT EXISTS idx_tenant_projections_subscription_status ON tenant_projections(subscription_status);
CREATE INDEX IF NOT EXISTS idx_tenant_projections_trial_ends_at ON tenant_projections(trial_ends_at);

-- Create event handler tracking table
CREATE TABLE IF NOT EXISTS event_handler_checkpoints (
    handler_name VARCHAR(255) PRIMARY KEY,
    last_processed_event_id VARCHAR(255),
    last_processed_at TIMESTAMP WITH TIME ZONE,
    last_processed_version BIGINT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    error_count INTEGER DEFAULT 0,
    last_error TEXT,
    last_error_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create function to update workspace member count
CREATE OR REPLACE FUNCTION update_workspace_member_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- Increment member count
        UPDATE workspace_projections 
        SET member_count = member_count + 1, updated_at = NOW()
        WHERE id = NEW.workspace_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        -- Decrement member count
        UPDATE workspace_projections 
        SET member_count = member_count - 1, updated_at = NOW()
        WHERE id = OLD.workspace_id;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Create triggers for workspace member count
DROP TRIGGER IF EXISTS trigger_workspace_member_count_insert ON workspace_member_projections;
CREATE TRIGGER trigger_workspace_member_count_insert
    AFTER INSERT ON workspace_member_projections
    FOR EACH ROW EXECUTE FUNCTION update_workspace_member_count();

DROP TRIGGER IF EXISTS trigger_workspace_member_count_delete ON workspace_member_projections;
CREATE TRIGGER trigger_workspace_member_count_delete
    AFTER DELETE ON workspace_member_projections
    FOR EACH ROW EXECUTE FUNCTION update_workspace_member_count();

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO postgres;