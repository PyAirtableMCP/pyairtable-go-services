-- Create CQRS schemas for read/write separation
-- This migration sets up separate schemas for write models (commands/events) and read models (queries/projections)

-- Create write schema for command handling and event storage
CREATE SCHEMA IF NOT EXISTS write_schema;

-- Create read schema for projections and query models
CREATE SCHEMA IF NOT EXISTS read_schema;

-- Grant necessary permissions
GRANT USAGE ON SCHEMA write_schema TO pyairtable_user;
GRANT USAGE ON SCHEMA read_schema TO pyairtable_user;
GRANT CREATE ON SCHEMA write_schema TO pyairtable_user;
GRANT CREATE ON SCHEMA read_schema TO pyairtable_user;

-- Move existing event store table to write schema
DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'domain_events' AND table_schema = 'public') THEN
        ALTER TABLE public.domain_events SET SCHEMA write_schema;
    END IF;
END $$;

-- Create event store table in write schema if it doesn't exist
CREATE TABLE IF NOT EXISTS write_schema.domain_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(255) NOT NULL,
    aggregate_id UUID NOT NULL,
    aggregate_type VARCHAR(255) NOT NULL,
    version BIGINT NOT NULL,
    tenant_id UUID NOT NULL,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payload JSONB NOT NULL,
    metadata JSONB,
    processed BOOLEAN DEFAULT FALSE,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Ensure version uniqueness per aggregate
    CONSTRAINT unique_version_per_aggregate UNIQUE (aggregate_id, version)
);

-- Create indexes for write schema event store
CREATE INDEX IF NOT EXISTS idx_domain_events_aggregate_id ON write_schema.domain_events (aggregate_id);
CREATE INDEX IF NOT EXISTS idx_domain_events_event_type ON write_schema.domain_events (event_type);
CREATE INDEX IF NOT EXISTS idx_domain_events_tenant_id ON write_schema.domain_events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_domain_events_occurred_at ON write_schema.domain_events (occurred_at);
CREATE INDEX IF NOT EXISTS idx_domain_events_processed ON write_schema.domain_events (processed) WHERE processed = FALSE;
CREATE INDEX IF NOT EXISTS idx_domain_events_aggregate_version ON write_schema.domain_events (aggregate_id, version);

-- Create user projections table in read schema
CREATE TABLE IF NOT EXISTS read_schema.user_projections (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    display_name VARCHAR(510), -- Computed field: first_name + last_name
    avatar TEXT,
    tenant_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    email_verified BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT FALSE,
    timezone VARCHAR(100),
    language VARCHAR(10) DEFAULT 'en',
    roles JSONB DEFAULT '[]'::jsonb,
    deactivation_reason TEXT,
    last_login_at TIMESTAMP WITH TIME ZONE,
    login_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    event_version BIGINT NOT NULL DEFAULT 0,
    
    -- Optimistic concurrency control
    CONSTRAINT check_event_version CHECK (event_version >= 0)
);

-- Create workspace projections table in read schema
CREATE TABLE IF NOT EXISTS read_schema.workspace_projections (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    tenant_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    is_public BOOLEAN DEFAULT FALSE,
    member_count INTEGER DEFAULT 0,
    base_count INTEGER DEFAULT 0,
    settings JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    event_version BIGINT NOT NULL DEFAULT 0,
    
    -- Optimistic concurrency control
    CONSTRAINT check_workspace_event_version CHECK (event_version >= 0)
);

-- Create workspace member projections for efficient queries
CREATE TABLE IF NOT EXISTS read_schema.workspace_member_projections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    permissions JSONB DEFAULT '[]'::jsonb,
    joined_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    event_version BIGINT NOT NULL DEFAULT 0,
    
    -- Ensure unique user per workspace
    CONSTRAINT unique_user_per_workspace UNIQUE (workspace_id, user_id),
    CONSTRAINT check_member_event_version CHECK (event_version >= 0)
);

-- Create tenant summary projections for dashboard queries
CREATE TABLE IF NOT EXISTS read_schema.tenant_summary_projections (
    tenant_id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    total_users INTEGER DEFAULT 0,
    active_users INTEGER DEFAULT 0,
    total_workspaces INTEGER DEFAULT 0,
    total_bases INTEGER DEFAULT 0,
    plan_type VARCHAR(50) DEFAULT 'free',
    storage_used_bytes BIGINT DEFAULT 0,
    api_calls_month INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    event_version BIGINT NOT NULL DEFAULT 0,
    
    CONSTRAINT check_tenant_event_version CHECK (event_version >= 0)
);

-- Create projection checkpoint table for tracking processed events
CREATE TABLE IF NOT EXISTS read_schema.projection_checkpoints (
    projection_name VARCHAR(255) PRIMARY KEY,
    last_processed_event_id UUID,
    last_processed_at TIMESTAMP WITH TIME ZONE,
    events_processed_count BIGINT DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for read schema projections
-- User projection indexes for fast queries
CREATE INDEX IF NOT EXISTS idx_user_projections_tenant_id ON read_schema.user_projections (tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_projections_email ON read_schema.user_projections (email);
CREATE INDEX IF NOT EXISTS idx_user_projections_status ON read_schema.user_projections (status);
CREATE INDEX IF NOT EXISTS idx_user_projections_active ON read_schema.user_projections (is_active);
CREATE INDEX IF NOT EXISTS idx_user_projections_updated_at ON read_schema.user_projections (updated_at);

-- Workspace projection indexes
CREATE INDEX IF NOT EXISTS idx_workspace_projections_tenant_id ON read_schema.workspace_projections (tenant_id);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_owner_id ON read_schema.workspace_projections (owner_id);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_status ON read_schema.workspace_projections (status);
CREATE INDEX IF NOT EXISTS idx_workspace_projections_updated_at ON read_schema.workspace_projections (updated_at);

-- Workspace member projection indexes
CREATE INDEX IF NOT EXISTS idx_workspace_member_workspace_id ON read_schema.workspace_member_projections (workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_member_user_id ON read_schema.workspace_member_projections (user_id);
CREATE INDEX IF NOT EXISTS idx_workspace_member_role ON read_schema.workspace_member_projections (role);

-- Tenant summary indexes
CREATE INDEX IF NOT EXISTS idx_tenant_summary_plan_type ON read_schema.tenant_summary_projections (plan_type);
CREATE INDEX IF NOT EXISTS idx_tenant_summary_updated_at ON read_schema.tenant_summary_projections (updated_at);

-- Create functions for automatic timestamp updates
CREATE OR REPLACE FUNCTION read_schema.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for automatic updated_at timestamp
CREATE TRIGGER update_user_projections_updated_at
    BEFORE UPDATE ON read_schema.user_projections
    FOR EACH ROW EXECUTE FUNCTION read_schema.update_updated_at_column();

CREATE TRIGGER update_workspace_projections_updated_at
    BEFORE UPDATE ON read_schema.workspace_projections
    FOR EACH ROW EXECUTE FUNCTION read_schema.update_updated_at_column();

CREATE TRIGGER update_workspace_member_projections_updated_at
    BEFORE UPDATE ON read_schema.workspace_member_projections
    FOR EACH ROW EXECUTE FUNCTION read_schema.update_updated_at_column();

CREATE TRIGGER update_tenant_summary_projections_updated_at
    BEFORE UPDATE ON read_schema.tenant_summary_projections
    FOR EACH ROW EXECUTE FUNCTION read_schema.update_updated_at_column();

CREATE TRIGGER update_projection_checkpoints_updated_at
    BEFORE UPDATE ON read_schema.projection_checkpoints
    FOR EACH ROW EXECUTE FUNCTION read_schema.update_updated_at_column();

-- Grant permissions on all tables
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA write_schema TO pyairtable_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA read_schema TO pyairtable_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA write_schema TO pyairtable_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA read_schema TO pyairtable_user;

-- Set default permissions for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA write_schema GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pyairtable_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA read_schema GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pyairtable_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA write_schema GRANT USAGE, SELECT ON SEQUENCES TO pyairtable_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA read_schema GRANT USAGE, SELECT ON SEQUENCES TO pyairtable_user;