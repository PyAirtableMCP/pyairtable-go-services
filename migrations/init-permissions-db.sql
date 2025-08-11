-- Initialize permissions database
-- This script should be run after the main database is created

\c pyairtable_permissions;

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Import permission tables
\i /docker-entrypoint-initdb.d/migrations/permission/001_create_permission_tables.sql;

-- Seed default permissions
INSERT INTO permissions (name, description, resource_type, action) VALUES
    ('read_workspace', 'Read access to workspace data', 'workspace', 'read'),
    ('write_workspace', 'Write access to workspace data', 'workspace', 'write'),
    ('delete_workspace', 'Delete workspace data', 'workspace', 'delete'),
    ('admin_workspace', 'Full administrative access to workspace', 'workspace', 'admin'),
    ('read_table', 'Read access to table data', 'table', 'read'),
    ('write_table', 'Write access to table data', 'table', 'write'),
    ('delete_table', 'Delete table data', 'table', 'delete'),
    ('admin_table', 'Full administrative access to table', 'table', 'admin'),
    ('read_record', 'Read access to record data', 'record', 'read'),
    ('write_record', 'Write access to record data', 'record', 'write'),
    ('delete_record', 'Delete record data', 'record', 'delete'),
    ('read_user', 'Read access to user data', 'user', 'read'),
    ('write_user', 'Write access to user data', 'user', 'write'),
    ('admin_user', 'Administrative access to user management', 'user', 'admin')
ON CONFLICT (name) DO NOTHING;

-- Seed default roles
INSERT INTO roles (name, description, is_system_role, tenant_id) VALUES
    ('admin', 'System administrator with full access', true, null),
    ('workspace_admin', 'Workspace administrator', true, null),
    ('workspace_member', 'Workspace member with read/write access', true, null),
    ('workspace_viewer', 'Workspace viewer with read-only access', true, null),
    ('table_admin', 'Table administrator', true, null),
    ('table_editor', 'Table editor with read/write access', true, null),
    ('table_viewer', 'Table viewer with read-only access', true, null)
ON CONFLICT (name, tenant_id) DO NOTHING;

-- Create role-permission mappings
WITH role_permission_mappings AS (
    SELECT 
        r.id as role_id,
        p.id as permission_id
    FROM roles r
    CROSS JOIN permissions p
    WHERE 
        (r.name = 'admin') -- Admin gets all permissions
        OR 
        (r.name = 'workspace_admin' AND p.resource_type IN ('workspace', 'table', 'record', 'user'))
        OR
        (r.name = 'workspace_member' AND p.resource_type IN ('workspace', 'table', 'record') AND p.action IN ('read', 'write'))
        OR
        (r.name = 'workspace_viewer' AND p.resource_type IN ('workspace', 'table', 'record') AND p.action = 'read')
        OR
        (r.name = 'table_admin' AND p.resource_type IN ('table', 'record'))
        OR
        (r.name = 'table_editor' AND p.resource_type IN ('table', 'record') AND p.action IN ('read', 'write'))
        OR
        (r.name = 'table_viewer' AND p.resource_type IN ('table', 'record') AND p.action = 'read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id FROM role_permission_mappings
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_permissions_lookup ON permissions (resource_type, action) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_roles_lookup ON user_roles (user_id, tenant_id, is_active);
CREATE INDEX IF NOT EXISTS idx_resource_permissions_lookup ON resource_permissions (user_id, resource_type, resource_id) WHERE deleted_at IS NULL;