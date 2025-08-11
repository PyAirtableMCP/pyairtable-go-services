# PyAirtable Platform API Design

## Overview

The PyAirtable Platform API provides a unified interface for user management, workspace management, tenant management, and notifications. This RESTful API is designed with consistency, scalability, and developer experience in mind.

## API Principles

### Design Principles
1. **RESTful**: Following REST conventions for resource-based URLs
2. **Consistent**: Uniform response formats and error handling
3. **Versioned**: API versioning for backward compatibility
4. **Secure**: JWT-based authentication with proper authorization
5. **Tenant-Aware**: Multi-tenant architecture with proper data isolation

### Base URL
```
https://api.pyairtable.com/api/v1
```

### Authentication
All protected endpoints require JWT authentication:
```
Authorization: Bearer <jwt_token>
```

## API Endpoints

### Authentication & Public Endpoints

#### User Login
```http
POST /api/v1/public/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "email": "user@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "is_active": true,
    "tenant_id": 1,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

#### Validate Tenant Slug
```http
GET /api/v1/public/tenants/validate-slug?slug=my-company
```

**Response:**
```json
{
  "slug": "my-company",
  "available": true
}
```

### User Management

#### List Users
```http
GET /api/v1/users?limit=10&offset=0
Authorization: Bearer <token>
```

**Response:**
```json
{
  "users": [
    {
      "id": 1,
      "email": "user@example.com",
      "first_name": "John",
      "last_name": "Doe",
      "is_active": true,
      "tenant_id": 1,
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "offset": 0
}
```

#### Create User
```http
POST /api/v1/users
Authorization: Bearer <token>
Content-Type: application/json

{
  "email": "newuser@example.com",
  "password": "securepassword",
  "first_name": "Jane",
  "last_name": "Smith"
}
```

**Response:**
```json
{
  "id": 2,
  "email": "newuser@example.com",
  "first_name": "Jane",
  "last_name": "Smith",
  "is_active": true,
  "tenant_id": 1,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

#### Get User
```http
GET /api/v1/users/{id}
Authorization: Bearer <token>
```

#### Update User
```http
PUT /api/v1/users/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "first_name": "John",
  "last_name": "Updated"
}
```

#### Delete User
```http
DELETE /api/v1/users/{id}
Authorization: Bearer <token>
```

**Response:** `204 No Content`

### Workspace Management

#### List Workspaces
```http
GET /api/v1/workspaces?limit=10&offset=0
Authorization: Bearer <token>
```

**Response:**
```json
{
  "workspaces": [
    {
      "id": 1,
      "name": "My Workspace",
      "description": "A sample workspace",
      "owner_id": 1,
      "tenant_id": 1,
      "is_active": true,
      "settings": {
        "timezone": "UTC",
        "language": "en"
      },
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "offset": 0
}
```

#### Create Workspace
```http
POST /api/v1/workspaces
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "New Workspace",
  "description": "A new workspace for our team",
  "settings": {
    "timezone": "UTC",
    "language": "en"
  }
}
```

#### Get User's Workspaces
```http
GET /api/v1/workspaces/user
Authorization: Bearer <token>
```

**Response:**
```json
{
  "workspaces": [
    {
      "id": 1,
      "name": "My Workspace",
      "description": "A sample workspace",
      "owner_id": 1,
      "tenant_id": 1,
      "is_active": true,
      "settings": {},
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Get Workspace
```http
GET /api/v1/workspaces/{id}
Authorization: Bearer <token>
```

#### Update Workspace
```http
PUT /api/v1/workspaces/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Updated Workspace Name",
  "description": "Updated description"
}
```

#### Delete Workspace
```http
DELETE /api/v1/workspaces/{id}
Authorization: Bearer <token>
```

#### Add Workspace Member
```http
POST /api/v1/workspaces/{id}/members
Authorization: Bearer <token>
Content-Type: application/json

{
  "user_id": 2,
  "role": "member",
  "permissions": {
    "can_edit": true,
    "can_delete": false
  }
}
```

**Response:**
```json
{
  "id": 1,
  "workspace_id": 1,
  "user_id": 2,
  "role": "member",
  "permissions": {
    "can_edit": true,
    "can_delete": false
  },
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

#### List Workspace Members
```http
GET /api/v1/workspaces/{id}/members
Authorization: Bearer <token>
```

**Response:**
```json
{
  "members": [
    {
      "id": 1,
      "workspace_id": 1,
      "user_id": 1,
      "role": "owner",
      "permissions": {},
      "user": {
        "id": 1,
        "email": "owner@example.com",
        "first_name": "John",
        "last_name": "Doe"
      },
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Tenant Management

#### List Tenants (Admin Only)
```http
GET /api/v1/tenants?limit=10&offset=0
Authorization: Bearer <admin_token>
```

**Response:**
```json
{
  "tenants": [
    {
      "id": 1,
      "name": "Acme Corp",
      "slug": "acme-corp",
      "domain": "acme.com",
      "plan": "pro",
      "is_active": true,
      "settings": {
        "max_users": 100,
        "features": ["workspaces", "notifications"]
      },
      "subscription": {
        "status": "active",
        "expires_at": "2024-12-31T23:59:59Z"
      },
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "offset": 0
}
```

#### Create Tenant
```http
POST /api/v1/tenants
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "New Company",
  "slug": "new-company",
  "domain": "newcompany.com",
  "plan": "starter",
  "settings": {
    "max_users": 10
  }
}
```

#### Get Tenant
```http
GET /api/v1/tenants/{id}
Authorization: Bearer <token>
```

#### Get Tenant by Slug
```http
GET /api/v1/tenants/slug/{slug}
Authorization: Bearer <token>
```

#### Update Tenant
```http
PUT /api/v1/tenants/{id}
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "Updated Company Name",
  "plan": "pro"
}
```

#### Delete Tenant
```http
DELETE /api/v1/tenants/{id}
Authorization: Bearer <admin_token>
```

### Notification Management

#### Get User Notifications
```http
GET /api/v1/notifications?limit=10&offset=0
Authorization: Bearer <token>
```

**Response:**
```json
{
  "notifications": [
    {
      "id": 1,
      "user_id": 1,
      "tenant_id": 1,
      "type": "workspace_invitation",
      "title": "Workspace Invitation",
      "message": "You've been invited to join 'Project Alpha' workspace",
      "data": {
        "workspace_id": 2,
        "invited_by": "jane@example.com"
      },
      "is_read": false,
      "read_at": null,
      "expires_at": "2024-02-01T00:00:00Z",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "offset": 0
}
```

#### Create Notification
```http
POST /api/v1/notifications
Authorization: Bearer <token>
Content-Type: application/json

{
  "user_id": 2,
  "type": "system_update",
  "title": "System Maintenance",
  "message": "System will be under maintenance from 2 AM to 4 AM UTC",
  "data": {
    "maintenance_window": "2024-01-15T02:00:00Z"
  },
  "expires_at": "2024-01-15T04:00:00Z"
}
```

#### Mark Notification as Read
```http
PUT /api/v1/notifications/{id}/read
Authorization: Bearer <token>
```

**Response:** `204 No Content`

#### Get Unread Notification Count
```http
GET /api/v1/notifications/unread-count
Authorization: Bearer <token>
```

**Response:**
```json
{
  "unread_count": 5
}
```

## Response Formats

### Success Response
All successful responses follow this structure:
```json
{
  "data": {}, // or [] for arrays
  "meta": {   // optional, for pagination
    "limit": 10,
    "offset": 0,
    "total": 100
  }
}
```

### Error Response
All error responses follow this structure:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed",
    "details": [
      {
        "field": "email",
        "message": "Email is required"
      }
    ]
  }
}
```

### HTTP Status Codes
- `200 OK` - Successful GET, PUT requests
- `201 Created` - Successful POST requests
- `204 No Content` - Successful DELETE requests
- `400 Bad Request` - Invalid request data
- `401 Unauthorized` - Authentication required
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `409 Conflict` - Resource already exists
- `422 Unprocessable Entity` - Validation errors
- `500 Internal Server Error` - Server error

## Authentication & Authorization

### JWT Token Structure
```json
{
  "user_id": 1,
  "tenant_id": 1,
  "exp": 1640995200,
  "iat": 1640908800
}
```

### Permission Levels
1. **Public** - No authentication required
2. **User** - Authenticated user, access to own data
3. **Admin** - Tenant admin, access to tenant data
4. **Super Admin** - System admin, access to all data

### Tenant Isolation
- All authenticated requests are scoped to the user's tenant
- Users can only access data within their tenant
- Admin users can manage their tenant's data
- Super admin users can access cross-tenant data

## Rate Limiting

### Rate Limits
- **Authentication endpoints**: 5 requests per minute per IP
- **User endpoints**: 100 requests per minute per user
- **Workspace endpoints**: 50 requests per minute per user
- **Notification endpoints**: 200 requests per minute per user

### Rate Limit Headers
```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 99
X-RateLimit-Reset: 1640908800
```

## Pagination

### Query Parameters
- `limit` - Number of items per page (default: 10, max: 100)
- `offset` - Number of items to skip (default: 0)

### Example
```http
GET /api/v1/users?limit=20&offset=40
```

### Response
```json
{
  "users": [...],
  "limit": 20,
  "offset": 40,
  "total": 150
}
```

## Filtering and Sorting

### Filtering
```http
GET /api/v1/users?is_active=true&created_after=2024-01-01
```

### Sorting
```http
GET /api/v1/users?sort=created_at&order=desc
```

## Webhooks (Future)

### Webhook Events
- `user.created`
- `user.updated`
- `user.deleted`
- `workspace.created`
- `workspace.member.added`
- `notification.created`

### Webhook Payload
```json
{
  "event": "user.created",
  "data": {
    "user": {...}
  },
  "timestamp": "2024-01-01T00:00:00Z",
  "tenant_id": 1
}
```

## API Versioning

### Versioning Strategy
- URL path versioning: `/api/v1/`, `/api/v2/`
- Backward compatibility maintained for at least 2 versions
- Deprecation notices provided 6 months before removal

### Version Support
- **v1** - Current stable version
- **v2** - Future version (not yet implemented)

## Client Libraries

### Go Client
```go
import "github.com/Reg-Kris/pyairtable-platform/pkg/client"

client := client.New("https://api.pyairtable.com", "your-token")
users, err := client.Users().List()
```

### JavaScript/TypeScript Client (Future)
```javascript
import { PyAirtableClient } from '@pyairtable/client';

const client = new PyAirtableClient('https://api.pyairtable.com', 'your-token');
const users = await client.users.list();
```

## OpenAPI Specification

The complete API specification is available as an OpenAPI 3.0 document:
- **Swagger UI**: https://api.pyairtable.com/docs
- **OpenAPI JSON**: https://api.pyairtable.com/openapi.json

## Examples and SDKs

### Authentication Example
```bash
# Login
curl -X POST https://api.pyairtable.com/api/v1/public/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "password"}'

# Use token
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
curl -H "Authorization: Bearer $TOKEN" \
  https://api.pyairtable.com/api/v1/users
```

### Complete Workflow Example
```bash
# 1. Login
TOKEN=$(curl -s -X POST https://api.pyairtable.com/api/v1/public/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "password"}' \
  | jq -r '.token')

# 2. Create workspace
WORKSPACE_ID=$(curl -s -X POST https://api.pyairtable.com/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My New Workspace", "description": "A test workspace"}' \
  | jq -r '.id')

# 3. Add member to workspace
curl -X POST https://api.pyairtable.com/api/v1/workspaces/$WORKSPACE_ID/members \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": 2, "role": "member"}'

# 4. List workspace members
curl -H "Authorization: Bearer $TOKEN" \
  https://api.pyairtable.com/api/v1/workspaces/$WORKSPACE_ID/members
```

This API design provides a comprehensive, RESTful interface for the PyAirtable Platform with proper authentication, authorization, and multi-tenant support.