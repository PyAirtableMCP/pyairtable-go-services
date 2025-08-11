-- Workspace Service Database Schema
-- This service manages workspaces, projects, and Airtable base connections

-- Create database if not exists
CREATE DATABASE IF NOT EXISTS workspace_service_db;

-- Use the database
\c workspace_service_db;

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Workspaces table
CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    settings JSONB DEFAULT '{}' NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP,
    
    -- Indexes
    CONSTRAINT unique_workspace_name_per_tenant UNIQUE (tenant_id, name, deleted_at)
);

CREATE INDEX idx_workspaces_tenant_id ON workspaces(tenant_id);
CREATE INDEX idx_workspaces_created_by ON workspaces(created_by);
CREATE INDEX idx_workspaces_deleted_at ON workspaces(deleted_at);

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    settings JSONB DEFAULT '{}' NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP,
    
    -- Indexes
    CONSTRAINT unique_project_name_per_workspace UNIQUE (workspace_id, name, deleted_at),
    CHECK (status IN ('active', 'archived', 'deleted'))
);

CREATE INDEX idx_projects_workspace_id ON projects(workspace_id);
CREATE INDEX idx_projects_status ON projects(status);
CREATE INDEX idx_projects_created_by ON projects(created_by);
CREATE INDEX idx_projects_deleted_at ON projects(deleted_at);

-- Airtable bases table
CREATE TABLE IF NOT EXISTS airtable_bases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    base_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    sync_enabled BOOLEAN DEFAULT true,
    last_sync_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP,
    
    -- Indexes
    CONSTRAINT unique_base_per_project UNIQUE (project_id, base_id, deleted_at)
);

CREATE INDEX idx_airtable_bases_project_id ON airtable_bases(project_id);
CREATE INDEX idx_airtable_bases_base_id ON airtable_bases(base_id);
CREATE INDEX idx_airtable_bases_sync_enabled ON airtable_bases(sync_enabled);
CREATE INDEX idx_airtable_bases_deleted_at ON airtable_bases(deleted_at);

-- Workspace members table
CREATE TABLE IF NOT EXISTS workspace_members (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL,
    joined_at TIMESTAMP DEFAULT NOW(),
    
    PRIMARY KEY (workspace_id, user_id),
    CHECK (role IN ('owner', 'admin', 'member', 'viewer'))
);

CREATE INDEX idx_workspace_members_user_id ON workspace_members(user_id);
CREATE INDEX idx_workspace_members_role ON workspace_members(role);

-- Workspace audit logs table
CREATE TABLE IF NOT EXISTS workspace_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(255),
    changes JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_workspace_id ON workspace_audit_logs(workspace_id);
CREATE INDEX idx_audit_logs_user_id ON workspace_audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON workspace_audit_logs(action);
CREATE INDEX idx_audit_logs_resource_type ON workspace_audit_logs(resource_type);
CREATE INDEX idx_audit_logs_created_at ON workspace_audit_logs(created_at);

-- Partition audit logs by month for better performance
CREATE TABLE workspace_audit_logs_y2024m01 PARTITION OF workspace_audit_logs
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');

CREATE TABLE workspace_audit_logs_y2024m02 PARTITION OF workspace_audit_logs
    FOR VALUES FROM ('2024-02-01') TO ('2024-03-01');

-- Add more partitions as needed...

-- Functions and triggers

-- Update timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_workspaces_updated_at BEFORE UPDATE ON workspaces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_airtable_bases_updated_at BEFORE UPDATE ON airtable_bases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to check workspace member count
CREATE OR REPLACE FUNCTION check_workspace_quota()
RETURNS TRIGGER AS $$
DECLARE
    workspace_count INTEGER;
    max_workspaces INTEGER := 10; -- Default quota
BEGIN
    SELECT COUNT(*) INTO workspace_count
    FROM workspaces
    WHERE tenant_id = NEW.tenant_id
    AND deleted_at IS NULL;
    
    IF workspace_count >= max_workspaces THEN
        RAISE EXCEPTION 'Workspace quota exceeded for tenant %', NEW.tenant_id;
    END IF;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER check_workspace_quota_before_insert BEFORE INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION check_workspace_quota();

-- Function to ensure at least one owner per workspace
CREATE OR REPLACE FUNCTION ensure_workspace_owner()
RETURNS TRIGGER AS $$
DECLARE
    owner_count INTEGER;
BEGIN
    IF OLD.role = 'owner' OR (TG_OP = 'DELETE' AND OLD.role = 'owner') THEN
        SELECT COUNT(*) INTO owner_count
        FROM workspace_members
        WHERE workspace_id = OLD.workspace_id
        AND role = 'owner'
        AND user_id != OLD.user_id;
        
        IF owner_count = 0 THEN
            RAISE EXCEPTION 'Cannot remove the last owner from workspace';
        END IF;
    END IF;
    
    RETURN OLD;
END;
$$ language 'plpgsql';

CREATE TRIGGER ensure_workspace_owner_before_update BEFORE UPDATE ON workspace_members
    FOR EACH ROW EXECUTE FUNCTION ensure_workspace_owner();

CREATE TRIGGER ensure_workspace_owner_before_delete BEFORE DELETE ON workspace_members
    FOR EACH ROW EXECUTE FUNCTION ensure_workspace_owner();

-- Initial data (optional)

-- Permissions for service account (if using RLS)
-- GRANT ALL ON ALL TABLES IN SCHEMA public TO workspace_service;
-- GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO workspace_service;

-- Comments
COMMENT ON TABLE workspaces IS 'Stores workspace information for multi-tenant organization';
COMMENT ON TABLE projects IS 'Stores projects within workspaces';
COMMENT ON TABLE airtable_bases IS 'Stores Airtable base connections for projects';
COMMENT ON TABLE workspace_members IS 'Stores workspace membership and roles';
COMMENT ON TABLE workspace_audit_logs IS 'Stores audit logs for workspace activities';