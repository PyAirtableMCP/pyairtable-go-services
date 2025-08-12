#!/bin/bash

# =============================================
# DATABASE MIGRATION SCRIPT FOR AUTH SERVICE
# =============================================
# Description: Production-ready migration runner with operational excellence
# Author: Database Administrator
# Created: 2025-08-12
# Version: 1.0.0

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# =============================================
# CONFIGURATION AND ENVIRONMENT
# =============================================

# Script directory and paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUTH_SERVICE_DIR="$(dirname "$SCRIPT_DIR")"
MIGRATIONS_DIR="$AUTH_SERVICE_DIR/migrations"
LOG_DIR="$AUTH_SERVICE_DIR/logs"
BACKUP_DIR="$AUTH_SERVICE_DIR/backups"

# Create necessary directories
mkdir -p "$LOG_DIR" "$BACKUP_DIR"

# Logging configuration
LOG_FILE="$LOG_DIR/migration_$(date +%Y%m%d_%H%M%S).log"
ERROR_LOG="$LOG_DIR/migration_errors_$(date +%Y%m%d_%H%M%S).log"

# Migration tracking
MIGRATION_TABLE="schema_migrations"
LOCK_FILE="/tmp/auth_service_migration.lock"
BACKUP_RETENTION_DAYS=30

# Default database configuration (override with environment variables)
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-auth_service}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_SSL_MODE="${DB_SSL_MODE:-prefer}"

# Connection string
if [ -n "$DB_PASSWORD" ]; then
    DB_URL="postgresql://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSL_MODE"
else
    DB_URL="postgresql://$DB_USER@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSL_MODE"
fi

# =============================================
# UTILITY FUNCTIONS
# =============================================

# Logging functions
log_info() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO: $*" | tee -a "$LOG_FILE"
}

log_warn() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: $*" | tee -a "$LOG_FILE" >&2
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" | tee -a "$LOG_FILE" | tee -a "$ERROR_LOG" >&2
}

log_success() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] SUCCESS: $*" | tee -a "$LOG_FILE"
}

# Cleanup function
cleanup() {
    local exit_code=$?
    if [ -f "$LOCK_FILE" ]; then
        rm -f "$LOCK_FILE"
        log_info "Released migration lock"
    fi
    
    if [ $exit_code -ne 0 ]; then
        log_error "Migration failed with exit code $exit_code"
        log_error "Check logs: $LOG_FILE and $ERROR_LOG"
    fi
    
    exit $exit_code
}

# Set up signal handlers
trap cleanup EXIT INT TERM

# =============================================
# DATABASE FUNCTIONS
# =============================================

# Test database connection
test_connection() {
    log_info "Testing database connection..."
    
    if ! psql "$DB_URL" -c "SELECT 1;" >/dev/null 2>&1; then
        log_error "Cannot connect to database. Check connection parameters."
        log_error "DB_HOST=$DB_HOST, DB_PORT=$DB_PORT, DB_NAME=$DB_NAME, DB_USER=$DB_USER"
        return 1
    fi
    
    log_success "Database connection successful"
    return 0
}

# Create migration tracking table
create_migration_table() {
    log_info "Creating migration tracking table..."
    
    psql "$DB_URL" <<EOF 2>>"$ERROR_LOG"
CREATE TABLE IF NOT EXISTS $MIGRATION_TABLE (
    id SERIAL PRIMARY KEY,
    version VARCHAR(255) NOT NULL UNIQUE,
    filename VARCHAR(255) NOT NULL,
    applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INTEGER,
    checksum VARCHAR(64),
    applied_by VARCHAR(100) DEFAULT CURRENT_USER
);

CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON $MIGRATION_TABLE(version);
CREATE INDEX IF NOT EXISTS idx_schema_migrations_applied_at ON $MIGRATION_TABLE(applied_at);

COMMENT ON TABLE $MIGRATION_TABLE IS 'Tracks applied database migrations for version control';
EOF

    if [ $? -eq 0 ]; then
        log_success "Migration tracking table ready"
    else
        log_error "Failed to create migration tracking table"
        return 1
    fi
}

# Get applied migrations
get_applied_migrations() {
    psql "$DB_URL" -t -c "SELECT version FROM $MIGRATION_TABLE ORDER BY version;" 2>/dev/null | sed '/^$/d' | tr -d ' '
}

# Check if migration is applied
is_migration_applied() {
    local version="$1"
    local applied_migrations
    applied_migrations=$(get_applied_migrations)
    echo "$applied_migrations" | grep -q "^$version$"
}

# Calculate file checksum
calculate_checksum() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file" | cut -d' ' -f1
    else
        md5 -q "$file" 2>/dev/null || md5sum "$file" | cut -d' ' -f1
    fi
}

# Create database backup
create_backup() {
    local backup_name="auth_service_backup_$(date +%Y%m%d_%H%M%S).sql"
    local backup_path="$BACKUP_DIR/$backup_name"
    
    log_info "Creating database backup: $backup_name"
    
    if pg_dump "$DB_URL" --no-owner --no-privileges > "$backup_path" 2>>"$ERROR_LOG"; then
        log_success "Backup created: $backup_path"
        
        # Compress backup
        if command -v gzip >/dev/null 2>&1; then
            gzip "$backup_path"
            log_info "Backup compressed: ${backup_path}.gz"
        fi
        
        return 0
    else
        log_error "Failed to create backup"
        return 1
    fi
}

# Clean up old backups
cleanup_old_backups() {
    log_info "Cleaning up backups older than $BACKUP_RETENTION_DAYS days..."
    
    find "$BACKUP_DIR" -name "auth_service_backup_*.sql*" -type f -mtime +$BACKUP_RETENTION_DAYS -delete 2>/dev/null || true
    
    local remaining_backups
    remaining_backups=$(find "$BACKUP_DIR" -name "auth_service_backup_*.sql*" -type f | wc -l)
    log_info "Backup cleanup complete. $remaining_backups backups remaining."
}

# =============================================
# MIGRATION FUNCTIONS
# =============================================

# Apply single migration
apply_migration() {
    local migration_file="$1"
    local filename
    filename=$(basename "$migration_file")
    local version
    version=$(echo "$filename" | sed 's/^\([0-9]\+\)_.*/\1/')
    
    if is_migration_applied "$version"; then
        log_info "Migration $version ($filename) already applied, skipping"
        return 0
    fi
    
    log_info "Applying migration $version: $filename"
    
    local start_time
    start_time=$(date +%s%3N)
    
    local checksum
    checksum=$(calculate_checksum "$migration_file")
    
    # Execute migration in a transaction
    if psql "$DB_URL" <<EOF 2>>"$ERROR_LOG"
BEGIN;

-- Execute migration
\i $migration_file

-- Record successful migration
INSERT INTO $MIGRATION_TABLE (version, filename, execution_time_ms, checksum)
VALUES ('$version', '$filename', $(date +%s%3N) - $start_time, '$checksum');

COMMIT;
EOF
    then
        local end_time
        end_time=$(date +%s%3N)
        local duration=$((end_time - start_time))
        
        log_success "Migration $version applied successfully in ${duration}ms"
        return 0
    else
        log_error "Migration $version failed"
        return 1
    fi
}

# Rollback single migration
rollback_migration() {
    local version="$1"
    
    if ! is_migration_applied "$version"; then
        log_warn "Migration $version is not applied, cannot rollback"
        return 1
    fi
    
    local down_file
    down_file=$(find "$MIGRATIONS_DIR" -name "${version}_*.down.sql" | head -1)
    
    if [ ! -f "$down_file" ]; then
        log_error "Rollback file not found for migration $version"
        return 1
    fi
    
    log_info "Rolling back migration $version"
    
    # Execute rollback in a transaction
    if psql "$DB_URL" <<EOF 2>>"$ERROR_LOG"
BEGIN;

-- Execute rollback
\i $down_file

-- Remove migration record
DELETE FROM $MIGRATION_TABLE WHERE version = '$version';

COMMIT;
EOF
    then
        log_success "Migration $version rolled back successfully"
        return 0
    else
        log_error "Rollback of migration $version failed"
        return 1
    fi
}

# Apply all pending migrations
apply_all_migrations() {
    local migration_files
    migration_files=$(find "$MIGRATIONS_DIR" -name "*.up.sql" | sort)
    
    if [ -z "$migration_files" ]; then
        log_info "No migration files found in $MIGRATIONS_DIR"
        return 0
    fi
    
    local applied_count=0
    local total_count=0
    
    for migration_file in $migration_files; do
        total_count=$((total_count + 1))
        if apply_migration "$migration_file"; then
            applied_count=$((applied_count + 1))
        else
            log_error "Migration failed, stopping execution"
            return 1
        fi
    done
    
    log_success "Applied $applied_count/$total_count migrations"
}

# Get migration status
show_status() {
    log_info "Migration Status Report"
    log_info "====================="
    
    # Database connection info
    log_info "Database: $DB_NAME@$DB_HOST:$DB_PORT"
    log_info "User: $DB_USER"
    
    # Available migrations
    local available_migrations
    available_migrations=$(find "$MIGRATIONS_DIR" -name "*.up.sql" | wc -l)
    log_info "Available migrations: $available_migrations"
    
    # Applied migrations
    local applied_migrations
    applied_migrations=$(get_applied_migrations | wc -l)
    log_info "Applied migrations: $applied_migrations"
    
    # Pending migrations
    local pending_count=$((available_migrations - applied_migrations))
    log_info "Pending migrations: $pending_count"
    
    # Show applied migrations
    if [ "$applied_migrations" -gt 0 ]; then
        log_info ""
        log_info "Applied Migrations:"
        psql "$DB_URL" -c "SELECT version, filename, applied_at, execution_time_ms FROM $MIGRATION_TABLE ORDER BY version;" 2>/dev/null || true
    fi
    
    # Show pending migrations
    if [ "$pending_count" -gt 0 ]; then
        log_info ""
        log_info "Pending Migrations:"
        find "$MIGRATIONS_DIR" -name "*.up.sql" | sort | while read -r migration_file; do
            local filename
            filename=$(basename "$migration_file")
            local version
            version=$(echo "$filename" | sed 's/^\([0-9]\+\)_.*/\1/')
            
            if ! is_migration_applied "$version"; then
                log_info "  $version: $filename"
            fi
        done
    fi
}

# =============================================
# MAIN FUNCTIONS
# =============================================

# Usage information
usage() {
    cat <<EOF
Database Migration Script for Auth Service

USAGE:
    $0 [OPTIONS] COMMAND

COMMANDS:
    up              Apply all pending migrations
    down VERSION    Rollback specific migration version
    status          Show migration status
    test            Test database connection
    backup          Create database backup only

OPTIONS:
    -h, --help      Show this help message
    -v, --verbose   Enable verbose logging
    --dry-run       Show what would be done without executing
    --no-backup     Skip automatic backup (not recommended)

ENVIRONMENT VARIABLES:
    DB_HOST         Database host (default: localhost)
    DB_PORT         Database port (default: 5432)
    DB_NAME         Database name (default: auth_service)
    DB_USER         Database user (default: postgres)
    DB_PASSWORD     Database password
    DB_SSL_MODE     SSL mode (default: prefer)

EXAMPLES:
    $0 up                    # Apply all pending migrations
    $0 down 001              # Rollback migration 001
    $0 status                # Show current migration status
    $0 test                  # Test database connection
    $0 backup                # Create backup only

LOGS:
    Migration logs: $LOG_FILE
    Error logs:     $ERROR_LOG

EOF
}

# Acquire migration lock
acquire_lock() {
    if [ -f "$LOCK_FILE" ]; then
        local lock_pid
        lock_pid=$(cat "$LOCK_FILE" 2>/dev/null || echo "")
        
        if [ -n "$lock_pid" ] && kill -0 "$lock_pid" 2>/dev/null; then
            log_error "Migration already in progress (PID: $lock_pid)"
            log_error "If you're sure no migration is running, remove: $LOCK_FILE"
            return 1
        else
            log_warn "Stale lock file found, removing"
            rm -f "$LOCK_FILE"
        fi
    fi
    
    echo $$ > "$LOCK_FILE"
    log_info "Acquired migration lock"
}

# Main execution function
main() {
    local command=""
    local version=""
    local dry_run=false
    local no_backup=false
    local verbose=false
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            up|down|status|test|backup)
                command="$1"
                if [ "$command" = "down" ] && [ $# -gt 1 ]; then
                    version="$2"
                    shift
                fi
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            -v|--verbose)
                verbose=true
                ;;
            --dry-run)
                dry_run=true
                ;;
            --no-backup)
                no_backup=true
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
        shift
    done
    
    # Validate command
    if [ -z "$command" ]; then
        log_error "No command specified"
        usage
        exit 1
    fi
    
    # Set verbose mode
    if [ "$verbose" = true ]; then
        set -x
    fi
    
    # Show configuration
    log_info "Starting migration script"
    log_info "Command: $command"
    log_info "Database: $DB_NAME@$DB_HOST:$DB_PORT"
    log_info "Migrations directory: $MIGRATIONS_DIR"
    
    if [ "$dry_run" = true ]; then
        log_info "DRY RUN MODE - No changes will be made"
    fi
    
    # Test database connection
    if ! test_connection; then
        exit 1
    fi
    
    # Handle commands that don't need lock
    case $command in
        test)
            log_success "Database connection test completed"
            exit 0
            ;;
        status)
            create_migration_table
            show_status
            exit 0
            ;;
    esac
    
    # Acquire lock for operations that modify database
    if ! acquire_lock; then
        exit 1
    fi
    
    # Create migration table
    if ! create_migration_table; then
        exit 1
    fi
    
    # Create backup (unless disabled or dry run)
    if [ "$no_backup" = false ] && [ "$dry_run" = false ]; then
        if ! create_backup; then
            log_warn "Backup failed, but continuing with migration"
        fi
    fi
    
    # Execute command
    case $command in
        up)
            if [ "$dry_run" = true ]; then
                log_info "Would apply pending migrations"
                show_status
            else
                apply_all_migrations
                cleanup_old_backups
            fi
            ;;
        down)
            if [ -z "$version" ]; then
                log_error "Version required for rollback command"
                exit 1
            fi
            
            if [ "$dry_run" = true ]; then
                log_info "Would rollback migration $version"
            else
                rollback_migration "$version"
            fi
            ;;
        backup)
            log_success "Backup completed"
            ;;
        *)
            log_error "Unknown command: $command"
            exit 1
            ;;
    esac
    
    log_success "Migration script completed successfully"
}

# =============================================
# SCRIPT EXECUTION
# =============================================

# Ensure we're in the right directory
cd "$SCRIPT_DIR"

# Check if migrations directory exists
if [ ! -d "$MIGRATIONS_DIR" ]; then
    log_error "Migrations directory not found: $MIGRATIONS_DIR"
    exit 1
fi

# Check for required tools
for tool in psql pg_dump; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        log_error "Required tool not found: $tool"
        log_error "Please install PostgreSQL client tools"
        exit 1
    fi
done

# Execute main function with all arguments
main "$@"