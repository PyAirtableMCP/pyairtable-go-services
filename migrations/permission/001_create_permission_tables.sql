-- Permission Service Database Schema
-- Creates all tables for the RBAC permission system

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Roles table
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_system_role BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    tenant_id UUID NULL, -- NULL for system roles
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

-- Unique constraint on name per tenant
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_name_tenant ON roles (name, tenant_id) WHERE deleted_at IS NULL;
-- Index for tenant filtering
CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles (tenant_id) WHERE deleted_at IS NULL;
-- Index for system roles
CREATE INDEX IF NOT EXISTS idx_roles_system ON roles (is_system_role) WHERE deleted_at IS NULL;

-- Permissions table
CREATE TABLE IF NOT EXISTS permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(200) NOT NULL UNIQUE,
    description TEXT,
    resource_type VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

-- Indexes for permission lookups
CREATE INDEX IF NOT EXISTS idx_permissions_resource_type ON permissions (resource_type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_permissions_action ON permissions (action) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_permissions_resource_action ON permissions (resource_type, action) WHERE deleted_at IS NULL;

-- Role-Permission junction table
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- User Roles table (tracks role assignments to users)
CREATE TABLE IF NOT EXISTS user_roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    resource_type VARCHAR(50) NULL, -- NULL for global roles
    resource_id UUID NULL,          -- NULL for global roles
    tenant_id UUID NOT NULL,
    granted_by UUID NOT NULL,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL,
    is_active BOOLEAN DEFAULT TRUE
);

-- Indexes for user role lookups
CREATE INDEX IF NOT EXISTS idx_user_roles_user_tenant ON user_roles (user_id, tenant_id) WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_user_roles_resource ON user_roles (resource_type, resource_id) WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_user_roles_expires ON user_roles (expires_at) WHERE expires_at IS NOT NULL;

-- Resource Permissions table (direct permissions on resources)
CREATE TABLE IF NOT EXISTS resource_permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    permission VARCHAR(10) NOT NULL CHECK (permission IN ('allow', 'deny')),
    tenant_id UUID NOT NULL,
    granted_by UUID NOT NULL,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL,
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

-- Indexes for resource permission lookups
CREATE INDEX IF NOT EXISTS idx_resource_permissions_user_tenant ON resource_permissions (user_id, tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_resource_permissions_resource ON resource_permissions (resource_type, resource_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_resource_permissions_expires ON resource_permissions (expires_at) WHERE expires_at IS NOT NULL AND deleted_at IS NULL;

-- Permission Audit Log table
CREATE TABLE IF NOT EXISTS permission_audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,          -- Who performed the action
    target_user_id UUID NOT NULL,   -- Who was affected by the action
    action VARCHAR(100) NOT NULL,   -- What action was performed
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID NULL,
    changes JSONB,                  -- What changed
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for audit log queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created ON permission_audit_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON permission_audit_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target_user ON permission_audit_logs (target_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON permission_audit_logs (action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON permission_audit_logs (resource_type, resource_id, created_at DESC);

-- Add triggers for updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply triggers to tables with updated_at columns
DROP TRIGGER IF EXISTS update_roles_updated_at ON roles;
CREATE TRIGGER update_roles_updated_at BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_permissions_updated_at ON permissions;
CREATE TRIGGER update_permissions_updated_at BEFORE UPDATE ON permissions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_resource_permissions_updated_at ON resource_permissions;
CREATE TRIGGER update_resource_permissions_updated_at BEFORE UPDATE ON resource_permissions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE roles IS 'Roles that can be assigned to users, can be system-wide or tenant-specific';
COMMENT ON TABLE permissions IS 'System permissions that define what actions can be performed on resources';
COMMENT ON TABLE role_permissions IS 'Junction table linking roles to their permissions';
COMMENT ON TABLE user_roles IS 'Role assignments to users, can be global, resource-type specific, or resource-specific';
COMMENT ON TABLE resource_permissions IS 'Direct permission grants/denials on specific resources';
COMMENT ON TABLE permission_audit_logs IS 'Audit trail of all permission-related changes';

-- Add some constraints for data integrity
ALTER TABLE resource_permissions ADD CONSTRAINT check_expires_after_granted 
    CHECK (expires_at IS NULL OR expires_at > granted_at);