-- Event Store table for Domain-Driven Design (DDD) Event Sourcing
-- Based on the Python event sourcing architecture in event-sourcing/event-bus-architecture.py

-- Create the event_store table
CREATE TABLE IF NOT EXISTS event_store (
    id VARCHAR(255) PRIMARY KEY,
    stream_id VARCHAR(255) NOT NULL,
    version INTEGER NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    event_data JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    correlation_id VARCHAR(255),
    UNIQUE(stream_id, version)
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_event_store_stream_id ON event_store(stream_id);
CREATE INDEX IF NOT EXISTS idx_event_store_version ON event_store(stream_id, version);
CREATE INDEX IF NOT EXISTS idx_event_store_event_type ON event_store(event_type);
CREATE INDEX IF NOT EXISTS idx_event_store_created_at ON event_store(created_at);
CREATE INDEX IF NOT EXISTS idx_event_store_correlation_id ON event_store(correlation_id);

-- Create domain_events table for additional event metadata and projections
CREATE TABLE IF NOT EXISTS domain_events (
    id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    aggregate_type VARCHAR(255) NOT NULL,
    version BIGINT NOT NULL,
    tenant_id VARCHAR(255),
    occurred_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    payload JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    correlation_id VARCHAR(255),
    causation_id VARCHAR(255),
    processed BOOLEAN DEFAULT FALSE,
    retry_count INTEGER DEFAULT 0,
    UNIQUE(aggregate_id, version)
);

-- Create indexes for domain_events
CREATE INDEX IF NOT EXISTS idx_domain_events_aggregate_id ON domain_events(aggregate_id);
CREATE INDEX IF NOT EXISTS idx_domain_events_aggregate_type ON domain_events(aggregate_type);
CREATE INDEX IF NOT EXISTS idx_domain_events_event_type ON domain_events(event_type);
CREATE INDEX IF NOT EXISTS idx_domain_events_tenant_id ON domain_events(tenant_id);
CREATE INDEX IF NOT EXISTS idx_domain_events_occurred_at ON domain_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_domain_events_correlation_id ON domain_events(correlation_id);
CREATE INDEX IF NOT EXISTS idx_domain_events_processed ON domain_events(processed);

-- Create saga_instances table for SAGA orchestration
CREATE TABLE IF NOT EXISTS saga_instances (
    id VARCHAR(255) PRIMARY KEY,
    saga_type VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending, running, completed, compensating, failed, compensated
    current_step INTEGER DEFAULT 0,
    input_data JSONB,
    output_data JSONB,
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    correlation_id VARCHAR(255),
    tenant_id VARCHAR(255)
);

-- Create indexes for saga_instances
CREATE INDEX IF NOT EXISTS idx_saga_instances_status ON saga_instances(status);
CREATE INDEX IF NOT EXISTS idx_saga_instances_saga_type ON saga_instances(saga_type);
CREATE INDEX IF NOT EXISTS idx_saga_instances_correlation_id ON saga_instances(correlation_id);
CREATE INDEX IF NOT EXISTS idx_saga_instances_tenant_id ON saga_instances(tenant_id);

-- Create saga_steps table for individual SAGA steps
CREATE TABLE IF NOT EXISTS saga_steps (
    id VARCHAR(255) PRIMARY KEY,
    saga_id VARCHAR(255) NOT NULL REFERENCES saga_instances(id),
    step_order INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    service VARCHAR(255) NOT NULL,
    command JSONB NOT NULL,
    compensation_command JSONB,
    status VARCHAR(50) DEFAULT 'pending', -- pending, running, completed, failed
    timeout_seconds INTEGER DEFAULT 300,
    retry_attempts INTEGER DEFAULT 3,
    current_attempts INTEGER DEFAULT 0,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT
);

-- Create indexes for saga_steps
CREATE INDEX IF NOT EXISTS idx_saga_steps_saga_id ON saga_steps(saga_id);
CREATE INDEX IF NOT EXISTS idx_saga_steps_status ON saga_steps(status);
CREATE INDEX IF NOT EXISTS idx_saga_steps_service ON saga_steps(service);

-- Create event_handlers table for tracking event processing
CREATE TABLE IF NOT EXISTS event_handlers (
    id VARCHAR(255) PRIMARY KEY,
    handler_name VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    last_processed_event_id VARCHAR(255),
    last_processed_at TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT TRUE,
    UNIQUE(handler_name, event_type)
);

-- Create indexes for event_handlers
CREATE INDEX IF NOT EXISTS idx_event_handlers_event_type ON event_handlers(event_type);
CREATE INDEX IF NOT EXISTS idx_event_handlers_active ON event_handlers(is_active);

-- Create snapshots table for aggregate snapshots (performance optimization)
CREATE TABLE IF NOT EXISTS aggregate_snapshots (
    id VARCHAR(255) PRIMARY KEY,
    aggregate_id VARCHAR(255) NOT NULL,
    aggregate_type VARCHAR(255) NOT NULL,
    version BIGINT NOT NULL,
    snapshot_data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    tenant_id VARCHAR(255),
    UNIQUE(aggregate_id, version)
);

-- Create indexes for aggregate_snapshots
CREATE INDEX IF NOT EXISTS idx_snapshots_aggregate_id ON aggregate_snapshots(aggregate_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_aggregate_type ON aggregate_snapshots(aggregate_type);
CREATE INDEX IF NOT EXISTS idx_snapshots_tenant_id ON aggregate_snapshots(tenant_id);

-- Add table comments for documentation
COMMENT ON TABLE event_store IS 'Main event store for event sourcing - stores all domain events';
COMMENT ON TABLE domain_events IS 'Domain events table for DDD patterns with additional metadata';
COMMENT ON TABLE saga_instances IS 'SAGA orchestration instances for distributed transactions';
COMMENT ON TABLE saga_steps IS 'Individual steps within SAGA instances';
COMMENT ON TABLE event_handlers IS 'Tracking table for event handlers and processors';
COMMENT ON TABLE aggregate_snapshots IS 'Snapshots of aggregates for performance optimization';

-- Create a function to get next version for an aggregate
CREATE OR REPLACE FUNCTION get_next_aggregate_version(p_aggregate_id VARCHAR(255))
RETURNS BIGINT AS $$
BEGIN
    RETURN COALESCE(
        (SELECT MAX(version) + 1 FROM domain_events WHERE aggregate_id = p_aggregate_id),
        1
    );
END;
$$ LANGUAGE plpgsql;

-- Create a function to check optimistic concurrency for events
CREATE OR REPLACE FUNCTION check_event_concurrency(
    p_aggregate_id VARCHAR(255),
    p_expected_version BIGINT
) RETURNS BOOLEAN AS $$
DECLARE
    current_version BIGINT;
BEGIN
    SELECT COALESCE(MAX(version), 0) INTO current_version
    FROM domain_events 
    WHERE aggregate_id = p_aggregate_id;
    
    RETURN current_version = p_expected_version;
END;
$$ LANGUAGE plpgsql;

-- Grant permissions to postgres user (could be more specific in production)
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO postgres;