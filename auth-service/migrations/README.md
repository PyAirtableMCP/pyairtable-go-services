# Database Migrations for Auth Service

This directory contains database migration files for the Auth Service, implementing a production-ready migration system with operational excellence in mind.

## 📋 Table of Contents

- [Overview](#overview)
- [Migration System Features](#migration-system-features)
- [Quick Start](#quick-start)
- [Database Schema](#database-schema)
- [Migration Files](#migration-files)
- [Migration Script Usage](#migration-script-usage)
- [Operational Procedures](#operational-procedures)
- [Monitoring and Alerting](#monitoring-and-alerting)
- [Disaster Recovery](#disaster-recovery)
- [Troubleshooting](#troubleshooting)

## Overview

The Auth Service uses a robust migration system built for production environments with the following principles:

- **Safety First**: Automatic backups, rollback capability, and transaction safety
- **Operational Excellence**: Comprehensive logging, monitoring, and error handling
- **High Availability**: Minimal downtime with proper locking and validation
- **Compliance**: Audit trails and security tracking

## Migration System Features

### 🔒 Security & Compliance
- **Audit Logging**: All operations tracked in `audit_logs` table
- **User Authentication**: Multi-factor authentication support
- **Session Management**: Secure session tracking with device fingerprinting
- **Password Security**: bcrypt hashing with configurable cost factor
- **Account Protection**: Failed login attempt tracking and account locking

### 🛡️ Operational Safety
- **Automatic Backups**: Database backups before each migration
- **Transaction Safety**: All migrations run in transactions
- **Rollback Support**: Down migrations for safe rollbacks
- **Lock Management**: Prevents concurrent migration execution
- **Validation**: Pre-flight checks and post-migration verification

### 📊 Monitoring & Observability
- **Migration Tracking**: `schema_migrations` table tracks all applied migrations
- **Execution Metrics**: Migration execution time tracking
- **Comprehensive Logging**: Structured logs with timestamps and correlation IDs
- **Health Checks**: Built-in database connection testing
- **Status Reporting**: Detailed migration status reports

### 🚀 Performance Features
- **Optimized Indexes**: Strategic indexing for common query patterns
- **Connection Pooling**: Efficient database connection management
- **Maintenance Functions**: Built-in functions for cleanup and optimization
- **Views**: Pre-built views for common data access patterns

## Quick Start

### Prerequisites

1. **PostgreSQL Client Tools**
   ```bash
   # On macOS
   brew install postgresql
   
   # On Ubuntu/Debian
   sudo apt-get install postgresql-client
   
   # On CentOS/RHEL
   sudo yum install postgresql
   ```

2. **Database Setup**
   ```bash
   # Create database (if not exists)
   createdb auth_service
   
   # Set environment variables
   export DB_HOST=localhost
   export DB_PORT=5432
   export DB_NAME=auth_service
   export DB_USER=postgres
   export DB_PASSWORD=your_password
   ```

### Running Migrations

1. **Test Database Connection**
   ```bash
   ./scripts/migrate.sh test
   ```

2. **Check Migration Status**
   ```bash
   ./scripts/migrate.sh status
   ```

3. **Apply All Pending Migrations**
   ```bash
   ./scripts/migrate.sh up
   ```

4. **Create Backup Only**
   ```bash
   ./scripts/migrate.sh backup
   ```

## Database Schema

### Core Tables

#### `users` Table
Primary user authentication and profile data:

- **Authentication**: email, username, password_hash
- **Profile**: first_name, last_name, phone
- **Security**: failed_login_attempts, account_locked_until, mfa_enabled
- **Verification**: email_verified, email_verification_token
- **Audit**: created_at, updated_at, last_login_at, last_login_ip

#### `user_sessions` Table
Active user session tracking:

- **Session Management**: session_token, refresh_token, expires_at
- **Device Tracking**: device_id, device_type, platform, user_agent
- **Security**: ip_address, location_data, is_persistent
- **Lifecycle**: status, created_at, last_activity_at, revoked_at

#### `audit_logs` Table
Security and compliance audit trail:

- **Actor**: user_id, session_id, ip_address
- **Action**: action (enum), resource_type, resource_id
- **Changes**: old_values, new_values (JSONB)
- **Context**: description, metadata, request_id

### Database Functions

#### Session Management
```sql
-- Expire old sessions
SELECT expire_old_sessions();

-- Lock user account after failed attempts
SELECT lock_user_account('user@example.com', 30);
```

#### Maintenance
```sql
-- Clean up old audit logs (365 days retention)
SELECT cleanup_old_audit_logs(365);
```

### Useful Views

#### `active_users`
Shows all active, non-deleted users with essential information.

#### `active_sessions_with_users`
Combines active sessions with user information for monitoring.

#### `recent_audit_events`
Shows audit events from the last 7 days with user context.

## Migration Files

### Naming Convention

Migration files follow this naming pattern:
```
{version}_{description}.{direction}.sql
```

Examples:
- `001_initial_schema.up.sql` - Initial schema creation
- `001_initial_schema.down.sql` - Initial schema rollback
- `002_add_user_preferences.up.sql` - Add user preferences
- `002_add_user_preferences.down.sql` - Remove user preferences

### Migration Structure

Each UP migration should include:
1. **Header**: Description, author, date
2. **Extensions**: Required PostgreSQL extensions
3. **Types**: Custom enum types
4. **Tables**: Table definitions with constraints
5. **Indexes**: Performance optimization indexes
6. **Functions**: Utility functions
7. **Triggers**: Automatic data management
8. **Views**: Common query simplification
9. **Initial Data**: Default or system data
10. **Comments**: Documentation

Each DOWN migration should include:
1. **Verification**: Log rollback operation
2. **Reverse Order**: Drop in reverse dependency order
3. **Cleanup**: Remove all created objects
4. **Validation**: Verify successful rollback

## Migration Script Usage

### Basic Commands

```bash
# Apply all pending migrations
./scripts/migrate.sh up

# Rollback specific migration
./scripts/migrate.sh down 001

# Show current status
./scripts/migrate.sh status

# Test database connection
./scripts/migrate.sh test

# Create backup only
./scripts/migrate.sh backup
```

### Advanced Options

```bash
# Verbose logging
./scripts/migrate.sh -v up

# Dry run (show what would be done)
./scripts/migrate.sh --dry-run up

# Skip automatic backup (not recommended)
./scripts/migrate.sh --no-backup up

# Help
./scripts/migrate.sh --help
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_HOST` | Database host | localhost |
| `DB_PORT` | Database port | 5432 |
| `DB_NAME` | Database name | auth_service |
| `DB_USER` | Database user | postgres |
| `DB_PASSWORD` | Database password | (none) |
| `DB_SSL_MODE` | SSL mode | prefer |

## Operational Procedures

### Pre-Migration Checklist

1. **Environment Verification**
   - [ ] Database connection tested
   - [ ] Sufficient disk space for backup
   - [ ] No active long-running transactions
   - [ ] Application maintenance window scheduled

2. **Backup Verification**
   - [ ] Recent backup available
   - [ ] Backup restoration tested
   - [ ] Backup retention policy followed

3. **Migration Validation**
   - [ ] Migration tested in staging environment
   - [ ] Rollback procedure verified
   - [ ] Performance impact assessed

### Migration Execution

1. **Pre-Migration Steps**
   ```bash
   # Check current status
   ./scripts/migrate.sh status
   
   # Create manual backup
   ./scripts/migrate.sh backup
   
   # Test connection
   ./scripts/migrate.sh test
   ```

2. **Execute Migration**
   ```bash
   # Apply migrations
   ./scripts/migrate.sh up
   ```

3. **Post-Migration Verification**
   ```bash
   # Verify migration status
   ./scripts/migrate.sh status
   
   # Check application functionality
   # Run smoke tests
   # Monitor error logs
   ```

### Rollback Procedures

1. **Immediate Rollback**
   ```bash
   # Rollback specific migration
   ./scripts/migrate.sh down 001
   ```

2. **Emergency Rollback**
   ```bash
   # Restore from backup
   pg_restore -d auth_service backup_file.sql
   ```

### Maintenance Schedule

#### Daily Tasks
- Monitor migration logs
- Check backup success
- Review audit logs for anomalies

#### Weekly Tasks
- Clean up old backups
- Review migration performance metrics
- Update documentation

#### Monthly Tasks
- Test disaster recovery procedures
- Review and optimize database performance
- Update backup retention policies

## Monitoring and Alerting

### Key Metrics

1. **Migration Performance**
   - Migration execution time
   - Database backup size
   - Migration success/failure rate

2. **Database Health**
   - Connection pool utilization
   - Active session count
   - Failed login attempts
   - Account lockout events

3. **Security Events**
   - Authentication failures
   - Suspicious login patterns
   - MFA bypass attempts
   - Privilege escalation attempts

### Monitoring Queries

```sql
-- Check migration history
SELECT version, filename, applied_at, execution_time_ms 
FROM schema_migrations 
ORDER BY applied_at DESC 
LIMIT 10;

-- Monitor active sessions
SELECT COUNT(*) as active_sessions,
       COUNT(DISTINCT user_id) as unique_users
FROM user_sessions 
WHERE status = 'active' 
AND expires_at > CURRENT_TIMESTAMP;

-- Security monitoring
SELECT action, COUNT(*) as event_count
FROM audit_logs 
WHERE created_at > CURRENT_TIMESTAMP - INTERVAL '1 hour'
GROUP BY action 
ORDER BY event_count DESC;

-- Failed login monitoring
SELECT email, failed_login_attempts, account_locked_until
FROM users 
WHERE failed_login_attempts > 0 
OR account_locked_until > CURRENT_TIMESTAMP;
```

### Alert Thresholds

| Metric | Warning | Critical |
|--------|---------|----------|
| Migration execution time | > 5 minutes | > 15 minutes |
| Failed login rate | > 10/hour | > 50/hour |
| Active sessions | > 1000 | > 5000 |
| Database connections | > 80% pool | > 95% pool |
| Backup failure | 1 failure | 2 consecutive failures |

## Disaster Recovery

### Recovery Time Objective (RTO)
- **Target**: 15 minutes for database restoration
- **Maximum**: 1 hour for full service recovery

### Recovery Point Objective (RPO)
- **Target**: 5 minutes of data loss maximum
- **Backup frequency**: Every 6 hours minimum

### DR Procedures

#### 1. Database Corruption
```bash
# Stop application services
# Restore from latest backup
pg_restore -d auth_service latest_backup.sql
# Verify data integrity
# Restart application services
```

#### 2. Migration Failure
```bash
# Stop migration process
# Rollback problematic migration
./scripts/migrate.sh down {version}
# Verify rollback success
# Investigate and fix issues
```

#### 3. Data Center Failure
```bash
# Activate secondary database
# Update application configuration
# Verify service functionality
# Monitor for issues
```

### DR Testing Schedule

- **Monthly**: Test backup restoration
- **Quarterly**: Full DR simulation
- **Annually**: Complete infrastructure failover test

## Troubleshooting

### Common Issues

#### Migration Hangs
```bash
# Check for locks
SELECT * FROM pg_locks WHERE granted = false;

# Check long-running queries
SELECT pid, now() - pg_stat_activity.query_start AS duration, query 
FROM pg_stat_activity 
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes';

# Kill blocking queries (if safe)
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE ...;
```

#### Connection Issues
```bash
# Check connection limits
SELECT * FROM pg_stat_activity;

# Check database configuration
SHOW max_connections;
SHOW shared_buffers;
```

#### Performance Issues
```bash
# Check slow queries
SELECT query, mean_time, calls 
FROM pg_stat_statements 
ORDER BY mean_time DESC 
LIMIT 10;

# Check index usage
SELECT schemaname, tablename, attname, n_distinct, correlation
FROM pg_stats
WHERE tablename IN ('users', 'user_sessions', 'audit_logs');
```

### Log Files

- **Migration logs**: `auth-service/logs/migration_*.log`
- **Error logs**: `auth-service/logs/migration_errors_*.log`
- **Application logs**: Check application-specific log locations

### Support Contacts

- **Database Team**: dba@company.com
- **DevOps Team**: devops@company.com
- **Security Team**: security@company.com
- **On-call**: Use incident management system

---

## Best Practices

### Development
1. Always create both UP and DOWN migrations
2. Test migrations in staging before production
3. Keep migrations idempotent when possible
4. Document breaking changes thoroughly
5. Review migrations in code review process

### Operations
1. Always backup before migrations
2. Monitor applications during and after migrations
3. Have rollback plan ready
4. Communicate maintenance windows
5. Keep detailed incident logs

### Security
1. Use least privilege database users
2. Encrypt sensitive data at rest
3. Regular security audits
4. Monitor for suspicious activity
5. Keep audit logs for compliance requirements

### Performance
1. Analyze query performance regularly
2. Monitor index usage and effectiveness
3. Plan for data growth
4. Regular maintenance tasks
5. Capacity planning

---

*Last updated: 2025-08-12*  
*Version: 1.0.0*  
*Maintained by: Database Administration Team*