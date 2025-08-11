# Permission Service API Documentation

## Overview

The Permission Service provides Role-Based Access Control (RBAC) and hierarchical permissions management for the PyAirtable platform. It supports fine-grained access control across workspaces, projects, and Airtable bases.

## Features

- **Hierarchical Permissions**: Workspace → Project → Airtable Base inheritance
- **Role-Based Access Control**: System and tenant-specific roles
- **Direct Resource Permissions**: Explicit allow/deny permissions on specific resources
- **Caching**: Redis-based permission caching for performance
- **Audit Logging**: Complete audit trail of permission changes
- **Batch Operations**: Efficient batch permission checking
- **Token Validation**: Integration with Auth Service for token validation

## Service Information

- **Base URL**: `http://localhost:8085` (development)
- **Health Check**: `GET /health`
- **Readiness Check**: `GET /ready`
- **Service Info**: `GET /api/v1/info`

## Authentication

All API endpoints (except health checks and info) require authentication via Bearer token:

```
Authorization: Bearer <jwt-token>
```

The service validates tokens by calling the Auth Service.

## API Endpoints

### Permission Checking

#### Check Single Permission
```http
POST /api/v1/permissions/check
```

Check if a user has permission to perform an action on a resource.

**Request Body:**
```json
{
  "user_id": "uuid",
  "resource_type": "workspace|project|airtable_base|user|file|api",
  "resource_id": "uuid",
  "action": "read|create|update|delete|manage|admin|invite|remove|execute|download|upload|share|list|view",
  "tenant_id": "uuid"
}
```

**Response:**
```json
{
  "allowed": true,
  "reason": "role permission: workspace_admin"
}
```

#### Batch Check Permissions
```http
POST /api/v1/permissions/batch-check
```

Check multiple permissions efficiently in a single request.

**Request Body:**
```json
{
  "user_id": "uuid",
  "tenant_id": "uuid",
  "checks": [
    {
      "resource_type": "workspace",
      "resource_id": "uuid",
      "action": "read",
      "key": "workspace_read"
    },
    {
      "resource_type": "project",
      "resource_id": "uuid", 
      "action": "create",
      "key": "project_create"
    }
  ]
}
```

**Response:**
```json
{
  "results": {
    "workspace_read": {
      "allowed": true,
      "reason": "role permission: member"
    },
    "project_create": {
      "allowed": false,
      "reason": "no matching permissions found"
    }
  }
}
```

### User Permissions

#### Get User Permissions
```http
GET /api/v1/users/{userId}/permissions?resource_type=workspace&resource_id=uuid
```

Retrieve all permissions for a user, optionally filtered by resource.

**Response:**
```json
{
  "user_id": "uuid",
  "tenant_id": "uuid",
  "roles": [
    {
      "user_id": "uuid",
      "role_id": "uuid", 
      "resource_type": "workspace",
      "resource_id": "uuid",
      "tenant_id": "uuid",
      "granted_by": "uuid",
      "granted_at": "2025-01-08T12:00:00Z",
      "expires_at": null,
      "is_active": true,
      "role": {
        "id": "uuid",
        "name": "workspace_admin",
        "description": "Workspace administrator"
      }
    }
  ],
  "direct_permissions": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "resource_type": "project",
      "resource_id": "uuid", 
      "action": "delete",
      "permission": "allow",
      "granted_at": "2025-01-08T12:00:00Z",
      "reason": "Special access for migration"
    }
  ],
  "effective_permissions": {
    "workspace:uuid": ["read", "update", "manage", "invite"],
    "project:uuid": ["read", "create", "update", "delete"]
  }
}
```

### Permission Management

#### Grant Direct Permission
```http
POST /api/v1/permissions/grant
```

Grant a direct permission to a user on a specific resource.

**Request Body:**
```json
{
  "user_id": "uuid",
  "resource_type": "project",
  "resource_id": "uuid",
  "action": "delete",
  "permission": "allow",
  "expires_at": "2025-02-08T12:00:00Z",
  "reason": "Temporary access for data migration"
}
```

**Response:**
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "resource_type": "project",
  "resource_id": "uuid",
  "action": "delete", 
  "permission": "allow",
  "tenant_id": "uuid",
  "granted_by": "uuid",
  "granted_at": "2025-01-08T12:00:00Z",
  "expires_at": "2025-02-08T12:00:00Z",
  "reason": "Temporary access for data migration"
}
```

#### Revoke Permission
```http
DELETE /api/v1/permissions/{permissionId}
```

Revoke a direct permission.

**Response:**
```json
{
  "message": "Permission revoked successfully"
}
```

### Role Management

#### List Roles
```http
GET /api/v1/roles?search=admin&is_active=true&page=1&page_size=20
```

List roles with optional filtering.

**Query Parameters:**
- `search`: Filter by name or description
- `is_system_role`: Filter system vs tenant roles
- `is_active`: Filter active/inactive roles
- `page`: Page number (default: 1)
- `page_size`: Items per page (default: 20, max: 100)
- `sort_by`: Sort field (default: name)
- `sort_order`: asc/desc (default: asc)

**Response:**
```json
{
  "roles": [
    {
      "id": "uuid",
      "name": "workspace_admin",
      "description": "Workspace administrator",
      "is_system_role": true,
      "is_active": true,
      "tenant_id": null,
      "permissions": [
        {
          "name": "workspace:manage",
          "description": "Full workspace management",
          "resource_type": "workspace",
          "action": "manage"
        }
      ]
    }
  ],
  "total": 5,
  "page": 1,
  "page_size": 20,
  "total_pages": 1
}
```

#### Create Role
```http
POST /api/v1/roles
```

Create a new role (tenant-specific).

**Request Body:**
```json
{
  "name": "project_reviewer",
  "description": "Can review projects but not modify",
  "permissions": [
    "project:read",
    "airtable_base:read",
    "user:view"
  ]
}
```

**Response:**
```json
{
  "id": "uuid",
  "name": "project_reviewer", 
  "description": "Can review projects but not modify",
  "is_system_role": false,
  "is_active": true,
  "tenant_id": "uuid"
}
```

#### Assign Role
```http
POST /api/v1/roles/assign
```

Assign a role to a user.

**Request Body:**
```json
{
  "user_id": "uuid",
  "role_id": "uuid",
  "resource_type": "workspace",
  "resource_id": "uuid",
  "expires_at": "2025-12-31T23:59:59Z"
}
```

**Response:**
```json
{
  "user_id": "uuid",
  "role_id": "uuid",
  "resource_type": "workspace", 
  "resource_id": "uuid",
  "tenant_id": "uuid",
  "granted_by": "uuid",
  "granted_at": "2025-01-08T12:00:00Z",
  "expires_at": "2025-12-31T23:59:59Z",
  "is_active": true
}
```

### Audit Logs

#### Get Audit Logs
```http
GET /api/v1/audit-logs?action=assign_role&page=1&page_size=20
```

Retrieve permission change audit logs.

**Query Parameters:**
- `user_id`: Filter by acting user
- `target_user_id`: Filter by affected user
- `action`: Filter by action type
- `resource_type`: Filter by resource type
- `resource_id`: Filter by specific resource
- `page`: Page number
- `page_size`: Items per page
- `sort_by`: Sort field (default: created_at)
- `sort_order`: asc/desc (default: desc)

**Response:**
```json
{
  "logs": [
    {
      "id": "uuid",
      "tenant_id": "uuid",
      "user_id": "uuid",
      "target_user_id": "uuid",
      "action": "assign_role",
      "resource_type": "workspace",
      "resource_id": "uuid",
      "changes": {
        "role_id": "uuid",
        "resource_type": "workspace",
        "expires_at": "2025-12-31T23:59:59Z"
      },
      "ip_address": "192.168.1.100",
      "user_agent": "Mozilla/5.0...",
      "created_at": "2025-01-08T12:00:00Z"
    }
  ],
  "total": 150,
  "page": 1,
  "page_size": 20,
  "total_pages": 8
}
```

## Permission Model

### Resource Types
- `workspace`: Workspace-level resources
- `project`: Project-level resources
- `airtable_base`: Airtable base connections
- `user`: User management operations
- `file`: File operations
- `api`: API-level operations (roles, audit logs, etc.)

### Actions
- `read`: Read resource information
- `create`: Create new resources
- `update`: Update existing resources
- `delete`: Delete resources
- `manage`: Full management (includes all above)
- `admin`: Administrative access
- `invite`: Invite users
- `remove`: Remove users
- `execute`: Execute operations
- `download`: Download files
- `upload`: Upload files
- `share`: Share resources
- `list`: List resources
- `view`: View details

### Default Roles

#### System Roles
- `system_admin`: Full system access
- `workspace_admin`: Workspace administration
- `project_admin`: Project administration
- `member`: Standard member access
- `viewer`: Read-only access

### Permission Hierarchy

1. **Direct Resource Permissions**: Highest priority, explicit allow/deny
2. **Role-Based Permissions**: Role assignments at various levels
3. **Inheritance**: Permissions flow down the hierarchy:
   - Workspace → Project → Airtable Base
   - Global → Resource Type → Specific Resource

## Error Responses

All endpoints return standard HTTP status codes:

- `200`: Success
- `201`: Created
- `400`: Bad Request
- `401`: Unauthorized
- `403`: Forbidden
- `404`: Not Found
- `409`: Conflict
- `500`: Internal Server Error

Error response format:
```json
{
  "error": "Error message description"
}
```

## Rate Limiting

The service implements rate limiting to prevent abuse. Default limits are configurable via environment variables.

## Caching

Permission checks are cached in Redis for 5 minutes to improve performance. Cache is automatically invalidated when permissions change.

## Development

### Environment Variables

```bash
# Server
PORT=8085
HOST=0.0.0.0

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=password
DB_NAME=pyairtable_permissions
DB_SSLMODE=disable

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=2

# Auth Service
AUTH_SERVICE_URL=http://localhost:8082

# Logging
LOG_LEVEL=info
```

### Running Locally

```bash
# Start dependencies
docker-compose up postgres redis

# Run migrations
make migrate

# Start service
make run
```

### Testing

```bash
# Run unit tests
make test

# Run integration tests
make test-integration

# Check permission
curl -X POST http://localhost:8085/api/v1/permissions/check \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "uuid",
    "resource_type": "workspace", 
    "resource_id": "uuid",
    "action": "read",
    "tenant_id": "uuid"
  }'
```