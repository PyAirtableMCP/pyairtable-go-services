-- Migration: Create API logs table for gateway
-- Version: 001
-- Description: Request/response logging for API Gateway

-- Create shared functions if they don't exist
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Up Migration - Non-partitioned version for simplicity
CREATE TABLE IF NOT EXISTS api_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id VARCHAR(100) NOT NULL,
    method VARCHAR(10) NOT NULL,
    path TEXT NOT NULL,
    query_params JSONB,
    headers JSONB,
    request_body JSONB,
    response_status INTEGER,
    response_body JSONB,
    response_time_ms INTEGER,
    user_id UUID,
    tenant_id UUID,
    ip_address INET,
    user_agent TEXT,
    service_name VARCHAR(100),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_api_logs_request_id ON api_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_api_logs_user_id ON api_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_api_logs_tenant_id ON api_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_api_logs_created_at ON api_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_api_logs_path_method ON api_logs(path, method);
CREATE INDEX IF NOT EXISTS idx_api_logs_status ON api_logs(response_status);
CREATE INDEX IF NOT EXISTS idx_api_logs_error ON api_logs(created_at DESC) WHERE error_message IS NOT NULL;

-- Trigger for updated_at
CREATE TRIGGER update_api_logs_updated_at BEFORE UPDATE
    ON api_logs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Rate limit tracking
CREATE TABLE IF NOT EXISTS rate_limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier VARCHAR(255) NOT NULL, -- IP or user_id
    endpoint VARCHAR(255) NOT NULL,
    window_start TIMESTAMP WITH TIME ZONE NOT NULL,
    request_count INTEGER DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(identifier, endpoint, window_start)
);

CREATE INDEX IF NOT EXISTS idx_rate_limits_identifier ON rate_limits(identifier);
CREATE INDEX IF NOT EXISTS idx_rate_limits_window ON rate_limits(window_start);

-- Trigger for rate_limits updated_at
CREATE TRIGGER update_rate_limits_updated_at BEFORE UPDATE
    ON rate_limits FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Circuit breaker state
CREATE TABLE IF NOT EXISTS circuit_breakers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_name VARCHAR(100) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'closed', -- closed, open, half_open
    failure_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    last_failure_time TIMESTAMP WITH TIME ZONE,
    next_retry_time TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(service_name, endpoint)
);

CREATE INDEX IF NOT EXISTS idx_circuit_breakers_service ON circuit_breakers(service_name);
CREATE INDEX IF NOT EXISTS idx_circuit_breakers_state ON circuit_breakers(state);

-- Trigger for circuit_breakers updated_at
CREATE TRIGGER update_circuit_breakers_updated_at BEFORE UPDATE
    ON circuit_breakers FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Comments
COMMENT ON TABLE api_logs IS 'API Gateway request/response logs (partitioned by month)';
COMMENT ON TABLE rate_limits IS 'Rate limiting tracking per identifier';
COMMENT ON TABLE circuit_breakers IS 'Circuit breaker state for downstream services';

-- Down Migration (commented out for safety)
-- DROP TABLE IF EXISTS api_logs CASCADE;
-- DROP TABLE IF EXISTS rate_limits CASCADE;
-- DROP TABLE IF EXISTS circuit_breakers CASCADE;
-- DROP FUNCTION IF EXISTS create_monthly_partition();