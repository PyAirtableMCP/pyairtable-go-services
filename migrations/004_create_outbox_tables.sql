-- Outbox Pattern Migration for PyAirtable
-- This migration creates the necessary tables for implementing the transactional outbox pattern
-- to ensure reliable event publishing and message delivery.

-- Main outbox table for storing events before publishing
CREATE TABLE IF NOT EXISTS event_outbox (
    id VARCHAR(255) PRIMARY KEY,
    aggregate_id VARCHAR(255) NOT NULL,
    aggregate_type VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    event_version BIGINT NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending, processing, published, failed, skipped
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,
    published_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    tenant_id VARCHAR(255),
    correlation_id VARCHAR(255),
    causation_id VARCHAR(255),
    
    -- Partitioning by tenant for better performance in multi-tenant scenarios
    CONSTRAINT chk_outbox_status CHECK (status IN ('pending', 'processing', 'published', 'failed', 'skipped'))
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_outbox_status ON event_outbox(status);
CREATE INDEX IF NOT EXISTS idx_outbox_status_next_retry ON event_outbox(status, next_retry_at) WHERE status = 'failed';
CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON event_outbox(created_at);
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON event_outbox(aggregate_id, aggregate_type);
CREATE INDEX IF NOT EXISTS idx_outbox_event_type ON event_outbox(event_type);
CREATE INDEX IF NOT EXISTS idx_outbox_tenant_id ON event_outbox(tenant_id);
CREATE INDEX IF NOT EXISTS idx_outbox_correlation_id ON event_outbox(correlation_id);

-- Partial index for pending events (most frequent query)
CREATE INDEX IF NOT EXISTS idx_outbox_pending_events ON event_outbox(created_at) WHERE status = 'pending';

-- Partial index for failed events ready for retry
CREATE INDEX IF NOT EXISTS idx_outbox_retry_ready ON event_outbox(next_retry_at) 
    WHERE status = 'failed' AND next_retry_at IS NOT NULL;

-- Outbox processors table for tracking different publisher instances
CREATE TABLE IF NOT EXISTS outbox_processors (
    id VARCHAR(255) PRIMARY KEY,
    processor_name VARCHAR(255) NOT NULL UNIQUE,
    processor_type VARCHAR(100) NOT NULL, -- event_bus, kafka, redis_stream, webhook, etc.
    configuration JSONB DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    heartbeat_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_processed_at TIMESTAMP WITH TIME ZONE,
    last_processed_event_id VARCHAR(255),
    metrics JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for outbox_processors
CREATE INDEX IF NOT EXISTS idx_processors_active ON outbox_processors(is_active);
CREATE INDEX IF NOT EXISTS idx_processors_type ON outbox_processors(processor_type);
CREATE INDEX IF NOT EXISTS idx_processors_heartbeat ON outbox_processors(heartbeat_at);

-- Outbox delivery tracking table for monitoring and observability
CREATE TABLE IF NOT EXISTS outbox_delivery_log (
    id VARCHAR(255) PRIMARY KEY,
    event_id VARCHAR(255) NOT NULL REFERENCES event_outbox(id),
    processor_id VARCHAR(255) NOT NULL REFERENCES outbox_processors(id),
    attempt_number INTEGER NOT NULL DEFAULT 1,
    status VARCHAR(50) NOT NULL, -- success, failure, timeout
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms INTEGER,
    error_message TEXT,
    response_data JSONB,
    
    CONSTRAINT chk_delivery_status CHECK (status IN ('success', 'failure', 'timeout'))
);

-- Create indexes for delivery log
CREATE INDEX IF NOT EXISTS idx_delivery_log_event_id ON outbox_delivery_log(event_id);
CREATE INDEX IF NOT EXISTS idx_delivery_log_processor_id ON outbox_delivery_log(processor_id);
CREATE INDEX IF NOT EXISTS idx_delivery_log_status ON outbox_delivery_log(status);
CREATE INDEX IF NOT EXISTS idx_delivery_log_started_at ON outbox_delivery_log(started_at);

-- Outbox metrics aggregation table for performance monitoring
CREATE TABLE IF NOT EXISTS outbox_metrics (
    id VARCHAR(255) PRIMARY KEY,
    processor_id VARCHAR(255) NOT NULL REFERENCES outbox_processors(id),
    metric_type VARCHAR(100) NOT NULL, -- throughput, latency, error_rate, retry_rate
    time_window TIMESTAMP WITH TIME ZONE NOT NULL, -- hourly aggregation window
    value DECIMAL(15,4) NOT NULL,
    unit VARCHAR(50) NOT NULL, -- events/sec, ms, percentage
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(processor_id, metric_type, time_window)
);

-- Create indexes for metrics
CREATE INDEX IF NOT EXISTS idx_metrics_processor_type ON outbox_metrics(processor_id, metric_type);
CREATE INDEX IF NOT EXISTS idx_metrics_time_window ON outbox_metrics(time_window);

-- Create a function to automatically update the updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for outbox_processors
CREATE TRIGGER trg_processors_updated_at
    BEFORE UPDATE ON outbox_processors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to get next batch of events for processing
CREATE OR REPLACE FUNCTION get_next_outbox_batch(
    p_batch_size INTEGER DEFAULT 100,
    p_processor_id VARCHAR(255) DEFAULT NULL
)
RETURNS TABLE (
    id VARCHAR(255),
    aggregate_id VARCHAR(255),
    aggregate_type VARCHAR(255),
    event_type VARCHAR(255),
    event_version BIGINT,
    payload JSONB,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER,
    tenant_id VARCHAR(255)
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        e.id, e.aggregate_id, e.aggregate_type, e.event_type, e.event_version,
        e.payload, e.metadata, e.created_at, e.retry_count, e.tenant_id
    FROM event_outbox e
    WHERE (e.status = 'pending' OR (e.status = 'failed' AND e.next_retry_at <= NOW()))
    ORDER BY e.created_at ASC
    LIMIT p_batch_size
    FOR UPDATE SKIP LOCKED;
END;
$$ LANGUAGE plpgsql;

-- Function to mark events as processing
CREATE OR REPLACE FUNCTION mark_outbox_events_processing(
    p_event_ids VARCHAR(255)[],
    p_processor_id VARCHAR(255)
)
RETURNS INTEGER AS $$
DECLARE
    updated_count INTEGER;
BEGIN
    UPDATE event_outbox 
    SET 
        status = 'processing',
        processed_at = NOW(),
        metadata = jsonb_set(
            COALESCE(metadata, '{}'), 
            '{processor_id}', 
            to_jsonb(p_processor_id)
        )
    WHERE id = ANY(p_event_ids)
    AND status IN ('pending', 'failed');
    
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql;

-- Function to mark events as published
CREATE OR REPLACE FUNCTION mark_outbox_events_published(
    p_event_ids VARCHAR(255)[],
    p_processor_id VARCHAR(255)
)
RETURNS INTEGER AS $$
DECLARE
    updated_count INTEGER;
BEGIN
    UPDATE event_outbox 
    SET 
        status = 'published',
        published_at = NOW(),
        error_message = NULL
    WHERE id = ANY(p_event_ids)
    AND status = 'processing';
    
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql;

-- Function to mark events as failed with retry logic
CREATE OR REPLACE FUNCTION mark_outbox_events_failed(
    p_event_ids VARCHAR(255)[],
    p_error_messages TEXT[],
    p_processor_id VARCHAR(255),
    p_retry_backoff_base_seconds INTEGER DEFAULT 60
)
RETURNS INTEGER AS $$
DECLARE
    updated_count INTEGER;
BEGIN
    UPDATE event_outbox 
    SET 
        status = CASE 
            WHEN retry_count >= max_retries THEN 'failed'
            ELSE 'failed'
        END,
        retry_count = retry_count + 1,
        error_message = p_error_messages[array_position(p_event_ids, id)],
        next_retry_at = CASE 
            WHEN retry_count + 1 < max_retries THEN 
                NOW() + INTERVAL '1 second' * (p_retry_backoff_base_seconds * POWER(2, retry_count))
            ELSE NULL
        END
    WHERE id = ANY(p_event_ids)
    AND status = 'processing';
    
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql;

-- Function to cleanup old processed events
CREATE OR REPLACE FUNCTION cleanup_processed_outbox_events(
    p_retention_days INTEGER DEFAULT 7
)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM event_outbox 
    WHERE status = 'published' 
    AND published_at < NOW() - INTERVAL '1 day' * p_retention_days;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to get outbox statistics
CREATE OR REPLACE FUNCTION get_outbox_statistics()
RETURNS TABLE (
    status VARCHAR(50),
    count BIGINT,
    oldest_event TIMESTAMP WITH TIME ZONE,
    newest_event TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        e.status,
        COUNT(*) as count,
        MIN(e.created_at) as oldest_event,
        MAX(e.created_at) as newest_event
    FROM event_outbox e
    GROUP BY e.status
    ORDER BY e.status;
END;
$$ LANGUAGE plpgsql;

-- Function to update processor heartbeat
CREATE OR REPLACE FUNCTION update_processor_heartbeat(
    p_processor_id VARCHAR(255),
    p_last_processed_event_id VARCHAR(255) DEFAULT NULL
)
RETURNS VOID AS $$
BEGIN
    UPDATE outbox_processors 
    SET 
        heartbeat_at = NOW(),
        last_processed_at = CASE 
            WHEN p_last_processed_event_id IS NOT NULL THEN NOW()
            ELSE last_processed_at
        END,
        last_processed_event_id = COALESCE(p_last_processed_event_id, last_processed_event_id)
    WHERE id = p_processor_id;
END;
$$ LANGUAGE plpgsql;

-- Add table comments for documentation
COMMENT ON TABLE event_outbox IS 'Transactional outbox for reliable event publishing';
COMMENT ON TABLE outbox_processors IS 'Registry of outbox event processors and publishers';
COMMENT ON TABLE outbox_delivery_log IS 'Audit log of event delivery attempts';
COMMENT ON TABLE outbox_metrics IS 'Aggregated metrics for outbox performance monitoring';

-- Grant permissions
GRANT ALL PRIVILEGES ON TABLE event_outbox TO postgres;
GRANT ALL PRIVILEGES ON TABLE outbox_processors TO postgres;
GRANT ALL PRIVILEGES ON TABLE outbox_delivery_log TO postgres;
GRANT ALL PRIVILEGES ON TABLE outbox_metrics TO postgres;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO postgres;

-- Create sample outbox processors for common use cases
INSERT INTO outbox_processors (id, processor_name, processor_type, configuration, is_active) VALUES 
    ('event-bus-processor', 'Event Bus Publisher', 'event_bus', '{"topic_prefix": "pyairtable", "batch_size": 50}', true),
    ('kafka-processor', 'Kafka Publisher', 'kafka', '{"brokers": ["localhost:9092"], "topic": "pyairtable-events", "batch_size": 100}', false),
    ('redis-stream-processor', 'Redis Stream Publisher', 'redis_stream', '{"stream": "pyairtable:events", "max_length": 10000}', false),
    ('webhook-processor', 'Webhook Publisher', 'webhook', '{"url": "http://localhost:3000/webhooks/events", "timeout_ms": 5000}', false)
ON CONFLICT (processor_name) DO NOTHING;