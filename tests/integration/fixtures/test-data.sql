-- Test Data Fixtures for Integration Tests
-- This file populates the test database with seed data for comprehensive testing

-- Create test tenants
INSERT INTO tenants (id, name, domain, status, plan_type, max_users, created_at, updated_at) VALUES
('550e8400-e29b-41d4-a716-446655440001', 'Test Tenant Alpha', 'alpha.test.com', 'active', 'enterprise', 100, NOW(), NOW()),
('550e8400-e29b-41d4-a716-446655440002', 'Test Tenant Beta', 'beta.test.com', 'active', 'pro', 50, NOW(), NOW()),
('550e8400-e29b-41d4-a716-446655440003', 'Test Tenant Gamma', 'gamma.test.com', 'suspended', 'basic', 10, NOW(), NOW());

-- Create test users
INSERT INTO users (id, tenant_id, email, username, password_hash, first_name, last_name, status, is_verified, last_login_at, created_at, updated_at) VALUES
-- Tenant Alpha users
('660e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'admin@alpha.test.com', 'alpha_admin', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewtziaP/wqOQSa.a', 'Alpha', 'Admin', 'active', true, NOW(), NOW(), NOW()),
('660e8400-e29b-41d4-a716-446655440002', '550e8400-e29b-41d4-a716-446655440001', 'user1@alpha.test.com', 'alpha_user1', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewtziaP/wqOQSa.a', 'Alice', 'Alpha', 'active', true, NOW(), NOW(), NOW()),
('660e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440001', 'user2@alpha.test.com', 'alpha_user2', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewtziaP/wqOQSa.a', 'Bob', 'Alpha', 'active', true, NULL, NOW(), NOW()),
-- Tenant Beta users
('660e8400-e29b-41d4-a716-446655440004', '550e8400-e29b-41d4-a716-446655440002', 'admin@beta.test.com', 'beta_admin', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewtziaP/wqOQSa.a', 'Beta', 'Admin', 'active', true, NOW(), NOW(), NOW()),
('660e8400-e29b-41d4-a716-446655440005', '550e8400-e29b-41d4-a716-446655440002', 'user1@beta.test.com', 'beta_user1', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewtziaP/wqOQSa.a', 'Charlie', 'Beta', 'active', false, NULL, NOW(), NOW()),
-- Tenant Gamma users (suspended tenant)
('660e8400-e29b-41d4-a716-446655440006', '550e8400-e29b-41d4-a716-446655440003', 'admin@gamma.test.com', 'gamma_admin', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewtziaP/wqOQSa.a', 'Gamma', 'Admin', 'suspended', true, NULL, NOW(), NOW());

-- Create test workspaces
INSERT INTO workspaces (id, tenant_id, name, description, owner_id, status, settings, created_at, updated_at) VALUES
-- Alpha workspaces
('770e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'Alpha Main Workspace', 'Primary workspace for Tenant Alpha', '660e8400-e29b-41d4-a716-446655440001', 'active', '{"default_permissions": "read"}', NOW(), NOW()),
('770e8400-e29b-41d4-a716-446655440002', '550e8400-e29b-41d4-a716-446655440001', 'Alpha Dev Workspace', 'Development workspace for Tenant Alpha', '660e8400-e29b-41d4-a716-446655440002', 'active', '{"default_permissions": "write"}', NOW(), NOW()),
-- Beta workspaces
('770e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440002', 'Beta Workspace', 'Workspace for Tenant Beta', '660e8400-e29b-41d4-a716-446655440004', 'active', '{"default_permissions": "read"}', NOW(), NOW()),
-- Gamma workspaces (should be inaccessible due to suspended tenant)
('770e8400-e29b-41d4-a716-446655440004', '550e8400-e29b-41d4-a716-446655440003', 'Gamma Workspace', 'Workspace for suspended Tenant Gamma', '660e8400-e29b-41d4-a716-446655440006', 'suspended', '{}', NOW(), NOW());

-- Create test projects
INSERT INTO projects (id, workspace_id, tenant_id, name, description, owner_id, status, settings, created_at, updated_at) VALUES
-- Alpha projects
('880e8400-e29b-41d4-a716-446655440001', '770e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'Alpha Project 1', 'First project in Alpha main workspace', '660e8400-e29b-41d4-a716-446655440001', 'active', '{"auto_sync": true}', NOW(), NOW()),
('880e8400-e29b-41d4-a716-446655440002', '770e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'Alpha Project 2', 'Second project in Alpha main workspace', '660e8400-e29b-41d4-a716-446655440002', 'active', '{"auto_sync": false}', NOW(), NOW()),
('880e8400-e29b-41d4-a716-446655440003', '770e8400-e29b-41d4-a716-446655440002', '550e8400-e29b-41d4-a716-446655440001', 'Alpha Dev Project', 'Development project', '660e8400-e29b-41d4-a716-446655440002', 'active', '{"auto_sync": true}', NOW(), NOW()),
-- Beta projects
('880e8400-e29b-41d4-a716-446655440004', '770e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440002', 'Beta Project 1', 'Project in Beta workspace', '660e8400-e29b-41d4-a716-446655440004', 'active', '{"auto_sync": true}', NOW(), NOW());

-- Create test airtable bases
INSERT INTO airtable_bases (id, project_id, tenant_id, airtable_base_id, name, description, api_key_id, status, last_sync_at, created_at, updated_at) VALUES
-- Alpha bases
('990e8400-e29b-41d4-a716-446655440001', '880e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'appTestAlpha001', 'Alpha Test Base 1', 'Test Airtable base for Alpha project', 'key001', 'connected', NOW(), NOW(), NOW()),
('990e8400-e29b-41d4-a716-446655440002', '880e8400-e29b-41d4-a716-446655440002', '550e8400-e29b-41d4-a716-446655440001', 'appTestAlpha002', 'Alpha Test Base 2', 'Second test base for Alpha', 'key002', 'connected', NOW(), NOW(), NOW()),
-- Beta bases
('990e8400-e29b-41d4-a716-446655440003', '880e8400-e29b-41d4-a716-446655440004', '550e8400-e29b-41d4-a716-446655440002', 'appTestBeta001', 'Beta Test Base 1', 'Test Airtable base for Beta project', 'key003', 'connected', NOW(), NOW(), NOW());

-- Create permission roles if they don't exist
INSERT INTO roles (id, name, description, is_system_role, is_active, tenant_id, created_at, updated_at) VALUES
-- System roles
('role-system-admin', 'system_admin', 'Full system administration', true, true, NULL, NOW(), NOW()),
('role-workspace-admin', 'workspace_admin', 'Workspace administration', true, true, NULL, NOW(), NOW()),
('role-project-admin', 'project_admin', 'Project administration', true, true, NULL, NOW(), NOW()),
('role-member', 'member', 'Standard member access', true, true, NULL, NOW(), NOW()),
('role-viewer', 'viewer', 'Read-only access', true, true, NULL, NOW(), NOW()),
-- Tenant-specific roles
('role-alpha-custom1', 'alpha_project_lead', 'Alpha project leadership role', false, true, '550e8400-e29b-41d4-a716-446655440001', NOW(), NOW()),
('role-beta-custom1', 'beta_reviewer', 'Beta project reviewer role', false, true, '550e8400-e29b-41d4-a716-446655440002', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- Assign roles to users
INSERT INTO user_roles (user_id, role_id, resource_type, resource_id, tenant_id, granted_by, granted_at, expires_at, is_active) VALUES
-- System admin for alpha admin
('660e8400-e29b-41d4-a716-446655440001', 'role-workspace-admin', 'workspace', '770e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', NOW(), NULL, true),
-- Project admin for alpha user1
('660e8400-e29b-41d4-a716-446655440002', 'role-project-admin', 'project', '880e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', NOW(), NULL, true),
-- Member access for alpha user2
('660e8400-e29b-41d4-a716-446655440003', 'role-member', 'workspace', '770e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', NOW(), NULL, true),
-- Beta admin
('660e8400-e29b-41d4-a716-446655440004', 'role-workspace-admin', 'workspace', '770e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440002', '660e8400-e29b-41d4-a716-446655440004', NOW(), NULL, true),
-- Beta user with limited access
('660e8400-e29b-41d4-a716-446655440005', 'role-viewer', 'workspace', '770e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440002', '660e8400-e29b-41d4-a716-446655440004', NOW(), NULL, true),
-- Gamma admin (should be inactive due to suspended tenant)
('660e8400-e29b-41d4-a716-446655440006', 'role-workspace-admin', 'workspace', '770e8400-e29b-41d4-a716-446655440004', '550e8400-e29b-41d4-a716-446655440003', '660e8400-e29b-41d4-a716-446655440006', NOW(), NULL, false);

-- Create some direct permissions for testing
INSERT INTO direct_permissions (id, user_id, resource_type, resource_id, action, permission, tenant_id, granted_by, granted_at, expires_at, reason) VALUES
-- Give alpha user2 delete permission on specific project (overrides member role)
('perm-001', '660e8400-e29b-41d4-a716-446655440003', 'project', '880e8400-e29b-41d4-a716-446655440002', 'delete', 'allow', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', NOW(), NOW() + INTERVAL '30 days', 'Temporary deletion access for cleanup'),
-- Explicitly deny beta user access to specific base
('perm-002', '660e8400-e29b-41d4-a716-446655440005', 'airtable_base', '990e8400-e29b-41d4-a716-446655440003', 'read', 'deny', '550e8400-e29b-41d4-a716-446655440002', '660e8400-e29b-41d4-a716-446655440004', NOW(), NULL, 'Security restriction for sensitive data');

-- Create test files for file permission testing
INSERT INTO files (id, tenant_id, uploader_id, filename, original_filename, file_size, mime_type, status, metadata, created_at, updated_at) VALUES
('file-001', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', 'test-document-1.txt', 'test-document-1.txt', 1024, 'text/plain', 'uploaded', '{"description": "Test document for Alpha"}', NOW(), NOW()),
('file-002', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440002', 'project-data.csv', 'project-data.csv', 2048, 'text/csv', 'processed', '{"project_id": "880e8400-e29b-41d4-a716-446655440001"}', NOW(), NOW()),
('file-003', '550e8400-e29b-41d4-a716-446655440002', '660e8400-e29b-41d4-a716-446655440004', 'beta-report.json', 'beta-report.json', 512, 'application/json', 'uploaded', '{"report_type": "monthly"}', NOW(), NOW());

-- Create test audit logs
INSERT INTO audit_logs (id, tenant_id, user_id, target_user_id, action, resource_type, resource_id, changes, ip_address, user_agent, created_at) VALUES
('audit-001', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440002', 'assign_role', 'project', '880e8400-e29b-41d4-a716-446655440001', '{"role_id": "role-project-admin", "resource_type": "project"}', '192.168.1.100', 'Test-Agent/1.0', NOW() - INTERVAL '1 hour'),
('audit-002', '550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440003', 'grant_permission', 'project', '880e8400-e29b-41d4-a716-446655440002', '{"action": "delete", "permission": "allow", "expires_at": "2025-02-08T12:00:00Z"}', '192.168.1.100', 'Test-Agent/1.0', NOW() - INTERVAL '30 minutes'),
('audit-003', '550e8400-e29b-41d4-a716-446655440002', '660e8400-e29b-41d4-a716-446655440004', '660e8400-e29b-41d4-a716-446655440005', 'assign_role', 'workspace', '770e8400-e29b-41d4-a716-446655440003', '{"role_id": "role-viewer", "resource_type": "workspace"}', '10.0.0.50', 'Test-Agent/1.0', NOW() - INTERVAL '15 minutes');

-- Test credentials (all use password: "testpass123")
-- Note: These are pre-hashed with bcrypt rounds=4 for faster test execution
-- The original password is "testpass123" for all test users

COMMIT;