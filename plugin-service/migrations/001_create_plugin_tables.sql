-- Plugin Service Database Schema
-- This migration creates the core tables for the plugin system

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Developers table
CREATE TABLE developers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    website VARCHAR(500),
    verified BOOLEAN DEFAULT FALSE,
    public_key TEXT,
    total_downloads BIGINT DEFAULT 0,
    average_rating DECIMAL(3,2) DEFAULT 0.0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Plugin registries table
CREATE TABLE plugin_registries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL UNIQUE,
    url VARCHAR(500) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'community', -- official, community, private
    enabled BOOLEAN DEFAULT TRUE,
    trusted_keys TEXT[], -- Array of trusted public keys
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Plugins table
CREATE TABLE plugins (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    type VARCHAR(50) NOT NULL, -- formula, ui, webhook, automation, connector, view
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending, installed, active, suspended, deprecated, error
    developer_id UUID NOT NULL REFERENCES developers(id) ON DELETE CASCADE,
    workspace_id UUID, -- NULL for public plugins
    
    -- Metadata
    description TEXT NOT NULL,
    long_description TEXT,
    icon_url VARCHAR(500),
    tags TEXT[] DEFAULT '{}',
    categories TEXT[] DEFAULT '{}',
    
    -- Technical details
    entry_point VARCHAR(255) NOT NULL,
    wasm_binary BYTEA,
    wasm_hash VARCHAR(64),
    source_code TEXT,
    dependencies JSONB DEFAULT '[]',
    
    -- Permissions and security
    permissions JSONB DEFAULT '[]',
    signature TEXT,
    signed_by VARCHAR(100),
    
    -- Resource limits
    resource_limits JSONB DEFAULT '{}',
    
    -- Configuration
    config JSONB DEFAULT '{}',
    user_config JSONB,
    
    -- Statistics
    downloads BIGINT DEFAULT 0,
    rating DECIMAL(3,2) DEFAULT 0.0,
    review_count BIGINT DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    installed_at TIMESTAMP WITH TIME ZONE,
    last_active_at TIMESTAMP WITH TIME ZONE,
    
    UNIQUE(name, version, workspace_id)
);

-- Plugin installations table
CREATE TABLE plugin_installations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL,
    user_id UUID NOT NULL,
    version VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'installed', -- installed, active, suspended, error
    config JSONB DEFAULT '{}',
    
    -- Runtime state
    last_error TEXT,
    restart_count INTEGER DEFAULT 0,
    
    -- Timestamps
    installed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_active_at TIMESTAMP WITH TIME ZONE,
    
    UNIQUE(plugin_id, workspace_id)
);

-- Plugin executions table
CREATE TABLE plugin_executions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    installation_id UUID NOT NULL REFERENCES plugin_installations(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL,
    user_id UUID NOT NULL,
    
    -- Execution context
    trigger_type VARCHAR(50) NOT NULL, -- api, webhook, event, scheduled
    trigger_data JSONB,
    
    -- Resource usage
    memory_used_mb INTEGER DEFAULT 0,
    cpu_time_ms INTEGER DEFAULT 0,
    execution_time_ms INTEGER DEFAULT 0,
    
    -- Results
    status VARCHAR(50) NOT NULL, -- success, error, timeout
    result JSONB,
    error TEXT,
    
    -- Timestamps
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Plugin hooks table
CREATE TABLE plugin_hooks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    installation_id UUID NOT NULL REFERENCES plugin_installations(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    priority INTEGER DEFAULT 0,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Plugin reviews table
CREATE TABLE plugin_reviews (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title VARCHAR(255),
    content TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(plugin_id, user_id, workspace_id)
);

-- Plugin quarantine table
CREATE TABLE plugin_quarantine (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plugin_id UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    quarantined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    release_at TIMESTAMP WITH TIME ZONE NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_by UUID NOT NULL
);

-- Audit log table
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(100) NOT NULL,
    plugin_id UUID,
    user_id UUID,
    workspace_id UUID,
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(100) NOT NULL,
    ip_address INET,
    user_agent TEXT,
    details JSONB DEFAULT '{}',
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for better query performance

-- Plugins indexes
CREATE INDEX idx_plugins_developer_id ON plugins(developer_id);
CREATE INDEX idx_plugins_workspace_id ON plugins(workspace_id);
CREATE INDEX idx_plugins_type ON plugins(type);
CREATE INDEX idx_plugins_status ON plugins(status);
CREATE INDEX idx_plugins_name ON plugins(name);
CREATE INDEX idx_plugins_created_at ON plugins(created_at);
CREATE INDEX idx_plugins_downloads ON plugins(downloads DESC);
CREATE INDEX idx_plugins_rating ON plugins(rating DESC);

-- Use GIN index for JSON fields
CREATE INDEX idx_plugins_tags ON plugins USING GIN(tags);
CREATE INDEX idx_plugins_categories ON plugins USING GIN(categories);
CREATE INDEX idx_plugins_permissions ON plugins USING GIN(permissions);

-- Plugin installations indexes
CREATE INDEX idx_installations_plugin_id ON plugin_installations(plugin_id);
CREATE INDEX idx_installations_workspace_id ON plugin_installations(workspace_id);
CREATE INDEX idx_installations_user_id ON plugin_installations(user_id);
CREATE INDEX idx_installations_status ON plugin_installations(status);
CREATE INDEX idx_installations_installed_at ON plugin_installations(installed_at);

-- Plugin executions indexes
CREATE INDEX idx_executions_plugin_id ON plugin_executions(plugin_id);
CREATE INDEX idx_executions_installation_id ON plugin_executions(installation_id);
CREATE INDEX idx_executions_workspace_id ON plugin_executions(workspace_id);
CREATE INDEX idx_executions_user_id ON plugin_executions(user_id);
CREATE INDEX idx_executions_started_at ON plugin_executions(started_at);
CREATE INDEX idx_executions_status ON plugin_executions(status);

-- Plugin hooks indexes
CREATE INDEX idx_hooks_plugin_id ON plugin_hooks(plugin_id);
CREATE INDEX idx_hooks_installation_id ON plugin_hooks(installation_id);
CREATE INDEX idx_hooks_event_type ON plugin_hooks(event_type);

-- Plugin reviews indexes
CREATE INDEX idx_reviews_plugin_id ON plugin_reviews(plugin_id);
CREATE INDEX idx_reviews_user_id ON plugin_reviews(user_id);
CREATE INDEX idx_reviews_rating ON plugin_reviews(rating);
CREATE INDEX idx_reviews_created_at ON plugin_reviews(created_at);

-- Plugin quarantine indexes
CREATE INDEX idx_quarantine_plugin_id ON plugin_quarantine(plugin_id);
CREATE INDEX idx_quarantine_release_at ON plugin_quarantine(release_at);

-- Audit log indexes
CREATE INDEX idx_audit_log_event_type ON audit_log(event_type);
CREATE INDEX idx_audit_log_plugin_id ON audit_log(plugin_id);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_audit_log_workspace_id ON audit_log(workspace_id);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_log_action ON audit_log(action);

-- Create triggers for updated_at timestamps

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_developers_updated_at 
    BEFORE UPDATE ON developers 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_plugin_registries_updated_at 
    BEFORE UPDATE ON plugin_registries 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_plugins_updated_at 
    BEFORE UPDATE ON plugins 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_plugin_installations_updated_at 
    BEFORE UPDATE ON plugin_installations 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_plugin_reviews_updated_at 
    BEFORE UPDATE ON plugin_reviews 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create views for common queries

-- Active plugins view
CREATE VIEW active_plugins AS
SELECT 
    p.*,
    d.name as developer_name,
    d.verified as developer_verified,
    COUNT(pi.id) as installation_count
FROM plugins p
JOIN developers d ON p.developer_id = d.id
LEFT JOIN plugin_installations pi ON p.id = pi.plugin_id AND pi.status = 'active'
WHERE p.status IN ('active', 'installed')
GROUP BY p.id, d.id;

-- Plugin statistics view
CREATE VIEW plugin_statistics AS
SELECT 
    p.id,
    p.name,
    p.type,
    COUNT(DISTINCT pi.id) as total_installations,
    COUNT(DISTINCT CASE WHEN pi.status = 'active' THEN pi.id END) as active_installations,
    COUNT(DISTINCT pe.id) as total_executions,
    COUNT(DISTINCT CASE WHEN pe.status = 'success' THEN pe.id END) as successful_executions,
    AVG(pe.execution_time_ms) as avg_execution_time_ms,
    AVG(pr.rating) as average_rating,
    COUNT(DISTINCT pr.id) as review_count
FROM plugins p
LEFT JOIN plugin_installations pi ON p.id = pi.plugin_id
LEFT JOIN plugin_executions pe ON p.id = pe.plugin_id
LEFT JOIN plugin_reviews pr ON p.id = pr.plugin_id
GROUP BY p.id, p.name, p.type;

-- Recent executions view
CREATE VIEW recent_executions AS
SELECT 
    pe.*,
    p.name as plugin_name,
    p.type as plugin_type,
    pi.workspace_id
FROM plugin_executions pe
JOIN plugins p ON pe.plugin_id = p.id
JOIN plugin_installations pi ON pe.installation_id = pi.id
WHERE pe.started_at >= NOW() - INTERVAL '24 hours'
ORDER BY pe.started_at DESC;

-- Insert default data

-- Insert official registry
INSERT INTO plugin_registries (name, url, type, enabled) VALUES 
('official', 'https://plugins.pyairtable.com', 'official', true);

-- Create system developer for official plugins
INSERT INTO developers (name, email, verified) VALUES 
('PyAirtable Team', 'plugins@pyairtable.com', true);

-- Create some indexes for full-text search (if needed)
-- These would be used for plugin search functionality
CREATE INDEX idx_plugins_search ON plugins USING GIN(
    to_tsvector('english', name || ' ' || description || ' ' || array_to_string(tags, ' '))
);

-- Add constraints for data integrity
ALTER TABLE plugins ADD CONSTRAINT check_rating_range 
    CHECK (rating >= 0.0 AND rating <= 5.0);

ALTER TABLE plugins ADD CONSTRAINT check_downloads_positive 
    CHECK (downloads >= 0);

ALTER TABLE plugin_reviews ADD CONSTRAINT check_review_rating_range 
    CHECK (rating >= 1 AND rating <= 5);

-- Add partition for audit log (by month) for better performance
-- This would help with large audit log tables
CREATE TABLE audit_log_template (
    LIKE audit_log INCLUDING ALL
) PARTITION BY RANGE (timestamp);

-- Add check constraints for JSON fields
ALTER TABLE plugins ADD CONSTRAINT check_resource_limits_format
    CHECK (
        resource_limits IS NULL OR (
            jsonb_typeof(resource_limits) = 'object' AND
            (resource_limits ? 'maxMemoryMB' AND jsonb_typeof(resource_limits->'maxMemoryMB') = 'number') AND
            (resource_limits ? 'maxCPUPercent' AND jsonb_typeof(resource_limits->'maxCPUPercent') = 'number') AND
            (resource_limits ? 'maxExecutionMs' AND jsonb_typeof(resource_limits->'maxExecutionMs') = 'number')
        )
    );

-- Comment the tables and important columns
COMMENT ON TABLE plugins IS 'Core plugins table storing plugin metadata and binaries';
COMMENT ON TABLE plugin_installations IS 'Plugin installations per workspace';
COMMENT ON TABLE plugin_executions IS 'Log of all plugin executions with performance metrics';
COMMENT ON TABLE plugin_hooks IS 'Event hooks registered by plugins';
COMMENT ON TABLE plugin_reviews IS 'User reviews and ratings for plugins';
COMMENT ON TABLE plugin_quarantine IS 'Quarantined plugins that are temporarily blocked';
COMMENT ON TABLE audit_log IS 'Comprehensive audit log of all plugin-related actions';

COMMENT ON COLUMN plugins.wasm_binary IS 'Compiled WebAssembly binary of the plugin';
COMMENT ON COLUMN plugins.wasm_hash IS 'SHA-256 hash of the WASM binary for integrity verification';
COMMENT ON COLUMN plugins.permissions IS 'JSON array of required permissions';
COMMENT ON COLUMN plugins.resource_limits IS 'JSON object defining resource consumption limits';
COMMENT ON COLUMN plugins.signature IS 'Digital signature of the plugin for verification';

-- Grant permissions (these would be adjusted based on your user roles)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO plugin_service_user;
-- GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO plugin_service_user;