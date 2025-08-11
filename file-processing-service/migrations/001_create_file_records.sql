-- Migration: Create file records table
-- Version: 001
-- Description: Create the main file records table for storing file metadata

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- File records table
CREATE TABLE file_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    filename VARCHAR(255) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    file_type VARCHAR(50) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL CHECK (size > 0),
    compressed_size BIGINT,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    progress FLOAT NOT NULL DEFAULT 0.0 CHECK (progress >= 0.0 AND progress <= 1.0),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,
    last_activity TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    error_message TEXT,
    processing_data JSONB,
    storage_path VARCHAR(500) NOT NULL,
    thumbnail_path VARCHAR(500),
    metadata JSONB NOT NULL DEFAULT '{}',
    virus_scanned BOOLEAN NOT NULL DEFAULT FALSE,
    virus_scan_result JSONB,
    
    -- Indexes
    CONSTRAINT valid_status CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'uploading')),
    CONSTRAINT valid_file_type CHECK (file_type IN ('csv', 'pdf', 'docx', 'xlsx', 'jpeg', 'png', 'webp'))
);

-- Create indexes for better query performance
CREATE INDEX idx_file_records_status ON file_records(status);
CREATE INDEX idx_file_records_file_type ON file_records(file_type);
CREATE INDEX idx_file_records_created_at ON file_records(created_at);
CREATE INDEX idx_file_records_processed_at ON file_records(processed_at);
CREATE INDEX idx_file_records_last_activity ON file_records(last_activity);
CREATE INDEX idx_file_records_storage_path ON file_records(storage_path);

-- GIN index for JSONB fields
CREATE INDEX idx_file_records_processing_data ON file_records USING GIN (processing_data);
CREATE INDEX idx_file_records_metadata ON file_records USING GIN (metadata);
CREATE INDEX idx_file_records_virus_scan_result ON file_records USING GIN (virus_scan_result);

-- Processing queue table for managing file processing tasks
CREATE TABLE processing_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL REFERENCES file_records(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 1,
    submitted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    worker_id VARCHAR(100),
    error_message TEXT,
    context JSONB,
    
    -- Ensure file is only queued once at a time
    UNIQUE(file_id)
);

-- Create indexes for processing queue
CREATE INDEX idx_processing_queue_priority ON processing_queue(priority DESC, submitted_at ASC);
CREATE INDEX idx_processing_queue_status ON processing_queue(started_at, completed_at);
CREATE INDEX idx_processing_queue_worker_id ON processing_queue(worker_id);
CREATE INDEX idx_processing_queue_file_id ON processing_queue(file_id);

-- Processing progress table for tracking detailed progress
CREATE TABLE processing_progress (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL REFERENCES file_records(id) ON DELETE CASCADE,
    step VARCHAR(50) NOT NULL,
    progress FLOAT NOT NULL CHECK (progress >= 0.0 AND progress <= 1.0),
    message TEXT,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    details JSONB,
    
    -- Ensure only one active step per file
    UNIQUE(file_id, step, started_at)
);

-- Create indexes for processing progress
CREATE INDEX idx_processing_progress_file_id ON processing_progress(file_id, started_at DESC);
CREATE INDEX idx_processing_progress_step ON processing_progress(step);
CREATE INDEX idx_processing_progress_started_at ON processing_progress(started_at);

-- File storage metadata table
CREATE TABLE file_storage (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL REFERENCES file_records(id) ON DELETE CASCADE,
    storage_provider VARCHAR(50) NOT NULL,
    bucket_name VARCHAR(100),
    storage_key VARCHAR(500) NOT NULL,
    storage_url TEXT,
    thumbnail_key VARCHAR(500),
    thumbnail_url TEXT,
    etag VARCHAR(100),
    version_id VARCHAR(100),
    encryption_key VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Ensure one storage record per file
    UNIQUE(file_id)
);

-- Create indexes for file storage
CREATE INDEX idx_file_storage_file_id ON file_storage(file_id);
CREATE INDEX idx_file_storage_provider ON file_storage(storage_provider);
CREATE INDEX idx_file_storage_bucket_key ON file_storage(bucket_name, storage_key);

-- File processing cache table for caching extracted data
CREATE TABLE processing_cache (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cache_key VARCHAR(255) NOT NULL UNIQUE,
    file_id UUID REFERENCES file_records(id) ON DELETE CASCADE,
    data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    hit_count INTEGER NOT NULL DEFAULT 0,
    last_accessed TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for processing cache
CREATE INDEX idx_processing_cache_key ON processing_cache(cache_key);
CREATE INDEX idx_processing_cache_file_id ON processing_cache(file_id);
CREATE INDEX idx_processing_cache_expires_at ON processing_cache(expires_at);
CREATE INDEX idx_processing_cache_last_accessed ON processing_cache(last_accessed);

-- Performance monitoring table
CREATE TABLE processing_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    metric_name VARCHAR(100) NOT NULL,
    metric_value NUMERIC NOT NULL,
    labels JSONB,
    recorded_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Partition by month for better performance
    PARTITION BY RANGE (recorded_at)
);

-- Create index for metrics
CREATE INDEX idx_processing_metrics_name_time ON processing_metrics(metric_name, recorded_at DESC);
CREATE INDEX idx_processing_metrics_labels ON processing_metrics USING GIN (labels);

-- Create monthly partitions for the current year
DO $$
DECLARE
    start_date DATE;
    end_date DATE;
    partition_name TEXT;
BEGIN
    FOR month_num IN 1..12 LOOP
        start_date := DATE(EXTRACT(YEAR FROM NOW()) || '-' || LPAD(month_num::TEXT, 2, '0') || '-01');
        end_date := start_date + INTERVAL '1 month';
        partition_name := 'processing_metrics_' || TO_CHAR(start_date, 'YYYY_MM');
        
        EXECUTE FORMAT('CREATE TABLE %I PARTITION OF processing_metrics FOR VALUES FROM (%L) TO (%L)',
                      partition_name, start_date, end_date);
    END LOOP;
END $$;

-- Triggers for updating timestamps
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to file_storage table
CREATE TRIGGER trigger_file_storage_updated_at
    BEFORE UPDATE ON file_storage
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- Function to clean up old records
CREATE OR REPLACE FUNCTION cleanup_old_records()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER := 0;
BEGIN
    -- Clean up old processing progress records (older than 30 days)
    DELETE FROM processing_progress 
    WHERE started_at < NOW() - INTERVAL '30 days';
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    -- Clean up expired cache entries
    DELETE FROM processing_cache 
    WHERE expires_at < NOW();
    
    -- Clean up old completed processing queue entries (older than 7 days)
    DELETE FROM processing_queue 
    WHERE completed_at IS NOT NULL 
    AND completed_at < NOW() - INTERVAL '7 days';
    
    -- Clean up old metrics (older than 90 days)
    DELETE FROM processing_metrics 
    WHERE recorded_at < NOW() - INTERVAL '90 days';
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Create indexes for cleanup function performance
CREATE INDEX idx_processing_progress_cleanup ON processing_progress(started_at) WHERE started_at < NOW() - INTERVAL '30 days';
CREATE INDEX idx_processing_cache_cleanup ON processing_cache(expires_at) WHERE expires_at < NOW();
CREATE INDEX idx_processing_queue_cleanup ON processing_queue(completed_at) WHERE completed_at IS NOT NULL;

-- Views for common queries
CREATE VIEW active_files AS
SELECT 
    fr.*,
    fs.storage_provider,
    fs.storage_url,
    fs.thumbnail_url,
    pq.priority,
    pq.worker_id,
    COALESCE(pq.retry_count, 0) as retry_count
FROM file_records fr
LEFT JOIN file_storage fs ON fr.id = fs.file_id
LEFT JOIN processing_queue pq ON fr.id = pq.file_id
WHERE fr.status IN ('pending', 'processing', 'uploading');

CREATE VIEW completed_files AS
SELECT 
    fr.*,
    fs.storage_provider,
    fs.storage_url,
    fs.thumbnail_url,
    fr.processed_at - fr.created_at as processing_duration
FROM file_records fr
LEFT JOIN file_storage fs ON fr.id = fs.file_id
WHERE fr.status = 'completed';

CREATE VIEW failed_files AS
SELECT 
    fr.*,
    pq.retry_count,
    pq.error_message as queue_error
FROM file_records fr
LEFT JOIN processing_queue pq ON fr.id = pq.file_id
WHERE fr.status = 'failed';

-- Grant permissions (adjust as needed for your security model)
-- These are basic permissions - in production, create specific roles
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO postgres;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO postgres;