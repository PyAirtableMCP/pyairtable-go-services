#!/bin/bash

# Migration runner for Phase 1 services (unified database)
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Default database connection parameters
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres_dev_password}"
DB_NAME="${DB_NAME:-pyairtable}"

echo "🔄 Running database migrations for Phase 1 services..."
echo "Using unified database: $DB_NAME"
echo ""

# Function to run migrations for a service schema
run_migration() {
    local service=$1
    local schema_name=$2
    local migration_dir=$3
    
    echo -e "${YELLOW}Setting up schema for $service ($schema_name)...${NC}"
    
    # Create schema if it doesn't exist
    PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "CREATE SCHEMA IF NOT EXISTS $schema_name;"
    
    # Run each SQL file in order, setting search_path first
    for sql_file in $(ls $migration_dir/*.sql | sort); do
        echo "  Applying $(basename $sql_file) to schema $schema_name..."
        
        # Create a temporary file with schema prefix
        temp_file="/tmp/$(basename $sql_file)"
        echo "SET search_path TO $schema_name, public;" > $temp_file
        cat $sql_file >> $temp_file
        
        PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f $temp_file
        if [ $? -eq 0 ]; then
            echo -e "  ${GREEN}✓ Success${NC}"
        else
            echo -e "  ${RED}✗ Failed${NC}"
            rm -f $temp_file
            exit 1
        fi
        
        rm -f $temp_file
    done
    
    echo ""
}

# Check if database exists, create if not
echo "Checking database existence..."
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1 || \
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d postgres -c "CREATE DATABASE $DB_NAME;"

# Create required extensions
echo "Creating required extensions..."
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";"
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "CREATE EXTENSION IF NOT EXISTS \"pgcrypto\";"

# Run migrations for each service schema
run_migration "Auth Service" "auth" "./auth"
run_migration "User Service" "users" "./users"
run_migration "API Gateway" "gateway" "./gateway"

echo -e "${GREEN}✅ All migrations completed successfully!${NC}"

# Show database status
echo ""
echo "📊 Database Status:"
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "\dn+" | grep -E "(auth|users|gateway)"

# Show table counts
echo ""
echo "📋 Table Counts by Schema:"
for schema in auth users gateway; do
    echo -e "\n${YELLOW}$schema schema:${NC}"
    PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SET search_path TO $schema; \dt" 2>/dev/null || echo "  No tables yet"
done

echo ""
echo "🔍 All schemas in database:"
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "\dn"