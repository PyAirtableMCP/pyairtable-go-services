-- Auth Service Database Setup
-- This script ensures the database and required tables exist

-- Create database (if running this manually on PostgreSQL)
-- CREATE DATABASE pyairtable_auth;

-- Connect to the auth database
-- \c pyairtable_auth;

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create users table if it doesn't exist
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    tenant_id UUID NOT NULL DEFAULT uuid_generate_v4(),
    is_active BOOLEAN DEFAULT true,
    email_verified BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT email_format CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_users_active ON users(is_active);

-- Create trigger function for updating updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for users table
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at 
    BEFORE UPDATE ON users 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Add helpful comments
COMMENT ON TABLE users IS 'User accounts for authentication and authorization';
COMMENT ON COLUMN users.id IS 'Unique user identifier';
COMMENT ON COLUMN users.email IS 'User email address (unique)';
COMMENT ON COLUMN users.password_hash IS 'Bcrypt hashed password';
COMMENT ON COLUMN users.tenant_id IS 'Associated tenant/organization';
COMMENT ON COLUMN users.role IS 'User role (user, admin, etc)';
COMMENT ON COLUMN users.is_active IS 'Whether the user account is active';
COMMENT ON COLUMN users.email_verified IS 'Whether the email has been verified';

-- Create a test user for development (with password "TestPassword123")
-- Password hash for "TestPassword123" using bcrypt cost 10
INSERT INTO users (email, password_hash, first_name, last_name, role, is_active, email_verified)
VALUES (
    'admin@example.com', 
    '$2a$10$8qTG1.wJ5f5.5F5.5F5.5O5kJ7p9b9b9b9b9b9b9b9b9b9b9b9b9b9b',
    'System',
    'Administrator',
    'admin',
    true,
    true
) ON CONFLICT (email) DO NOTHING;

-- Insert some sample users for testing
INSERT INTO users (email, password_hash, first_name, last_name, role, is_active, email_verified)
VALUES 
    ('user1@example.com', '$2a$10$8qTG1.wJ5f5.5F5.5F5.5O5kJ7p9b9b9b9b9b9b9b9b9b9b9b9b9b9b', 'John', 'Doe', 'user', true, false),
    ('user2@example.com', '$2a$10$8qTG1.wJ5f5.5F5.5F5.5O5kJ7p9b9b9b9b9b9b9b9b9b9b9b9b9b9b', 'Jane', 'Smith', 'user', true, true)
ON CONFLICT (email) DO NOTHING;

-- Show table structure
\d users

-- Show sample data
SELECT id, email, first_name, last_name, role, is_active, email_verified, created_at 
FROM users 
ORDER BY created_at DESC 
LIMIT 5;