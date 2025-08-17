-- Migration: 001_initial_schema.down.sql
-- Description: Rollback initial schema for auth service
-- Created: 2025-08-12
-- Author: Database Administrator
-- WARNING: This will permanently delete all authentication data!

-- =============================================
-- ROLLBACK VERIFICATION
-- =============================================

-- Log the rollback operation
DO $$
BEGIN
    -- Only proceed if audit_logs table exists
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'audit_logs') THEN
        INSERT INTO audit_logs (
            action,
            resource_type,
            description,
            ip_address,
            metadata
        ) VALUES (
            'delete',
            'schema',
            'Rolling back initial schema migration 001',
            '127.0.0.1',
            '{"migration": "001_initial_schema", "operation": "rollback", "timestamp": "' || CURRENT_TIMESTAMP || '"}'
        );
    END IF;
END $$;

-- =============================================
-- DROP VIEWS (must drop before tables they reference)
-- =============================================

DROP VIEW IF EXISTS recent_audit_events CASCADE;
DROP VIEW IF EXISTS active_sessions_with_users CASCADE;
DROP VIEW IF EXISTS active_users CASCADE;

-- =============================================
-- DROP FUNCTIONS
-- =============================================

DROP FUNCTION IF EXISTS lock_user_account(VARCHAR, INTEGER) CASCADE;
DROP FUNCTION IF EXISTS cleanup_old_audit_logs(INTEGER) CASCADE;
DROP FUNCTION IF EXISTS expire_old_sessions() CASCADE;
DROP FUNCTION IF EXISTS update_session_activity() CASCADE;
DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;

-- =============================================
-- DROP TRIGGERS (automatically dropped with functions, but explicit for clarity)
-- =============================================

DROP TRIGGER IF EXISTS update_session_last_activity ON user_sessions;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

-- =============================================
-- DROP INDEXES (automatically dropped with tables, but explicit for clarity)
-- =============================================

-- Audit logs indexes
DROP INDEX IF EXISTS idx_audit_metadata_gin;
DROP INDEX IF EXISTS idx_audit_success;
DROP INDEX IF EXISTS idx_audit_request_id;
DROP INDEX IF EXISTS idx_audit_ip_address;
DROP INDEX IF EXISTS idx_audit_created_at;
DROP INDEX IF EXISTS idx_audit_resource;
DROP INDEX IF EXISTS idx_audit_action;
DROP INDEX IF EXISTS idx_audit_session_id;
DROP INDEX IF EXISTS idx_audit_user_id;
DROP INDEX IF EXISTS idx_audit_user_action_time;

-- User sessions indexes
DROP INDEX IF EXISTS idx_sessions_user_active;
DROP INDEX IF EXISTS idx_sessions_user_status;
DROP INDEX IF EXISTS idx_sessions_device_id;
DROP INDEX IF EXISTS idx_sessions_ip_address;
DROP INDEX IF EXISTS idx_sessions_last_activity;
DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_status;
DROP INDEX IF EXISTS idx_sessions_refresh_token;
DROP INDEX IF EXISTS idx_sessions_token;
DROP INDEX IF EXISTS idx_sessions_user_id;

-- Users indexes
DROP INDEX IF EXISTS idx_users_email_status;
DROP INDEX IF EXISTS idx_users_metadata_gin;
DROP INDEX IF EXISTS idx_users_last_login;
DROP INDEX IF EXISTS idx_users_created_at;
DROP INDEX IF EXISTS idx_users_email_verification_token;
DROP INDEX IF EXISTS idx_users_password_reset_token;
DROP INDEX IF EXISTS idx_users_email_verified;
DROP INDEX IF EXISTS idx_users_status;
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_email;

-- =============================================
-- DROP TABLES (in reverse dependency order)
-- =============================================

-- Drop audit_logs table (references users and user_sessions)
DROP TABLE IF EXISTS audit_logs CASCADE;

-- Drop user_sessions table (references users)
DROP TABLE IF EXISTS user_sessions CASCADE;

-- Drop users table (main table)
DROP TABLE IF EXISTS users CASCADE;

-- =============================================
-- DROP CUSTOM TYPES
-- =============================================

DROP TYPE IF EXISTS audit_action CASCADE;
DROP TYPE IF EXISTS session_status CASCADE;
DROP TYPE IF EXISTS user_status CASCADE;

-- =============================================
-- DROP EXTENSIONS (only if they were created by this migration)
-- =============================================

-- Note: We don't drop extensions as they might be used by other parts of the system
-- If you need to drop them, uncomment the following lines:
-- DROP EXTENSION IF EXISTS "pgcrypto";
-- DROP EXTENSION IF EXISTS "uuid-ossp";

-- =============================================
-- VERIFICATION
-- =============================================

-- Verify all objects have been dropped
DO $$
DECLARE
    remaining_tables INTEGER;
    remaining_types INTEGER;
    remaining_functions INTEGER;
BEGIN
    -- Check for remaining tables
    SELECT COUNT(*) INTO remaining_tables
    FROM information_schema.tables
    WHERE table_schema = 'public'
    AND table_name IN ('users', 'user_sessions', 'audit_logs');
    
    -- Check for remaining types
    SELECT COUNT(*) INTO remaining_types
    FROM pg_type
    WHERE typname IN ('user_status', 'session_status', 'audit_action');
    
    -- Check for remaining functions
    SELECT COUNT(*) INTO remaining_functions
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE n.nspname = 'public'
    AND p.proname IN (
        'update_updated_at_column',
        'update_session_activity',
        'expire_old_sessions',
        'cleanup_old_audit_logs',
        'lock_user_account'
    );
    
    -- Raise notice about rollback status
    IF remaining_tables = 0 AND remaining_types = 0 AND remaining_functions = 0 THEN
        RAISE NOTICE 'Migration 001_initial_schema successfully rolled back. All objects dropped.';
    ELSE
        RAISE WARNING 'Migration rollback incomplete. Remaining objects: % tables, % types, % functions', 
            remaining_tables, remaining_types, remaining_functions;
    END IF;
END $$;

-- =============================================
-- ROLLBACK COMPLETE
-- =============================================

-- Final confirmation message
SELECT 'Migration 001_initial_schema.down.sql completed' AS rollback_status,
       CURRENT_TIMESTAMP AS completed_at;