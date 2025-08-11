-- Create webhook management tables
-- Migration: 001_create_webhook_tables.sql

-- Create webhooks table
CREATE TABLE IF NOT EXISTS webhooks (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    workspace_id INTEGER,
    name VARCHAR(255) NOT NULL,
    url VARCHAR(2048) NOT NULL,
    secret VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    description TEXT,
    max_retries INTEGER DEFAULT 3,
    timeout_seconds INTEGER DEFAULT 30,
    rate_limit_rps INTEGER DEFAULT 10,
    last_delivery_at TIMESTAMP,
    last_failure_at TIMESTAMP,
    failure_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    headers JSONB,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Create indexes for webhooks table
CREATE INDEX IF NOT EXISTS idx_webhooks_user_id ON webhooks(user_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_workspace_id ON webhooks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_status ON webhooks(status);
CREATE INDEX IF NOT EXISTS idx_webhooks_deleted_at ON webhooks(deleted_at);
CREATE INDEX IF NOT EXISTS idx_webhooks_last_delivery_at ON webhooks(last_delivery_at);
CREATE INDEX IF NOT EXISTS idx_webhooks_last_failure_at ON webhooks(last_failure_at);

-- Add constraint for webhook status
ALTER TABLE webhooks ADD CONSTRAINT chk_webhook_status 
    CHECK (status IN ('active', 'inactive', 'paused', 'failed'));

-- Create webhook_subscriptions table
CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id SERIAL PRIMARY KEY,
    webhook_id INTEGER NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    filters JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE
);

-- Create indexes for webhook_subscriptions table
CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_webhook_id ON webhook_subscriptions(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_event_type ON webhook_subscriptions(event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_resource_id ON webhook_subscriptions(resource_id);
CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_is_active ON webhook_subscriptions(is_active);
CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_deleted_at ON webhook_subscriptions(deleted_at);

-- Add constraint for event types
ALTER TABLE webhook_subscriptions ADD CONSTRAINT chk_event_type 
    CHECK (event_type IN (
        'table.created', 'table.updated', 'table.deleted',
        'record.created', 'record.updated', 'record.deleted',
        'workspace.created', 'workspace.updated',
        'user.invited', 'user.joined'
    ));

-- Create webhook_delivery_attempts table
CREATE TABLE IF NOT EXISTS webhook_delivery_attempts (
    id SERIAL PRIMARY KEY,
    webhook_id INTEGER NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempt_number INTEGER DEFAULT 1,
    scheduled_at TIMESTAMP NOT NULL,
    attempted_at TIMESTAMP,
    delivered_at TIMESTAMP,
    response_status INTEGER,
    response_headers JSONB,
    response_body TEXT,
    error_message TEXT,
    duration_ms BIGINT,
    signature VARCHAR(255),
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    next_retry_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE
);

-- Create indexes for webhook_delivery_attempts table
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_webhook_id ON webhook_delivery_attempts(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_event_id ON webhook_delivery_attempts(event_id);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_event_type ON webhook_delivery_attempts(event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_status ON webhook_delivery_attempts(status);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_scheduled_at ON webhook_delivery_attempts(scheduled_at);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_next_retry_at ON webhook_delivery_attempts(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_deleted_at ON webhook_delivery_attempts(deleted_at);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_created_at ON webhook_delivery_attempts(created_at);

-- Add constraint for delivery status
ALTER TABLE webhook_delivery_attempts ADD CONSTRAINT chk_delivery_status 
    CHECK (status IN ('pending', 'delivered', 'failed', 'retrying', 'abandoned'));

-- Create webhook_dead_letters table
CREATE TABLE IF NOT EXISTS webhook_dead_letters (
    id SERIAL PRIMARY KEY,
    webhook_id INTEGER NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    failure_reason TEXT NOT NULL,
    last_attempt_at TIMESTAMP NOT NULL,
    attempt_count INTEGER NOT NULL,
    is_processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMP,
    original_headers JSONB,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE
);

-- Create indexes for webhook_dead_letters table
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_webhook_id ON webhook_dead_letters(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_event_id ON webhook_dead_letters(event_id);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_event_type ON webhook_dead_letters(event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_is_processed ON webhook_dead_letters(is_processed);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_last_attempt_at ON webhook_dead_letters(last_attempt_at);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_deleted_at ON webhook_dead_letters(deleted_at);

-- Create function to update updated_at column
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers to automatically update updated_at
CREATE TRIGGER update_webhooks_updated_at BEFORE UPDATE ON webhooks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_webhook_subscriptions_updated_at BEFORE UPDATE ON webhook_subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_webhook_delivery_attempts_updated_at BEFORE UPDATE ON webhook_delivery_attempts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_webhook_dead_letters_updated_at BEFORE UPDATE ON webhook_dead_letters
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_webhooks_user_workspace ON webhooks(user_id, workspace_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_webhook_event ON webhook_subscriptions(webhook_id, event_type) WHERE deleted_at IS NULL AND is_active = true;
CREATE INDEX IF NOT EXISTS idx_delivery_attempts_webhook_status_scheduled ON webhook_delivery_attempts(webhook_id, status, scheduled_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_delivery_attempts_retry_queue ON webhook_delivery_attempts(next_retry_at, status) WHERE deleted_at IS NULL AND status = 'retrying';
CREATE INDEX IF NOT EXISTS idx_dead_letters_unprocessed ON webhook_dead_letters(webhook_id, is_processed, created_at) WHERE deleted_at IS NULL AND is_processed = false;

-- Create partial indexes for performance
CREATE INDEX IF NOT EXISTS idx_webhooks_active ON webhooks(id, user_id) WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX IF NOT EXISTS idx_delivery_attempts_pending ON webhook_delivery_attempts(webhook_id, scheduled_at) WHERE deleted_at IS NULL AND status = 'pending';

-- Add comments for documentation
COMMENT ON TABLE webhooks IS 'Webhook endpoint configurations for users and workspaces';
COMMENT ON TABLE webhook_subscriptions IS 'Event subscriptions for webhooks with filtering capabilities';
COMMENT ON TABLE webhook_delivery_attempts IS 'Delivery attempts with retry tracking and response logging';
COMMENT ON TABLE webhook_dead_letters IS 'Failed deliveries that exhausted retry attempts';

COMMENT ON COLUMN webhooks.secret IS 'HMAC-SHA256 secret for payload signing - never expose in API responses';
COMMENT ON COLUMN webhooks.rate_limit_rps IS 'Requests per second limit for this webhook endpoint';
COMMENT ON COLUMN webhook_subscriptions.filters IS 'JSON filters for advanced event matching';
COMMENT ON COLUMN webhook_delivery_attempts.idempotency_key IS 'Unique key to prevent duplicate deliveries';
COMMENT ON COLUMN webhook_delivery_attempts.signature IS 'HMAC-SHA256 signature sent with the payload';