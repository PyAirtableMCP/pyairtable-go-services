# PyAirtable Go Microservices

This directory contains the Go-based microservices for the PyAirtable platform, implementing a modern 22-service architecture.

## 🏗️ Architecture Overview

### Phase 1 Services (Core Infrastructure)
- **API Gateway** (Port 8080): Routes all external traffic, handles authentication, rate limiting
- **Auth Service** (Port 8001): JWT authentication, user registration, token management
- **User Service** (Port 8002): User profile management, CRUD operations
- **Airtable Gateway** (Port 8003): Proxy for Airtable API with caching and rate limiting

### Phase 2 Services (Business Logic)
- **Tenant Service**: Multi-tenancy management
- **Workspace Service**: Workspace organization
- **Permission Service**: RBAC and access control
- **Workflow Engine**: Automation and task scheduling
- **Analytics Service**: Metrics and reporting
- **File Service**: File upload and processing
- **Notification Service**: Email/SMS/Push notifications
- **Webhook Service**: External integrations

### Phase 3 Services (Advanced Features)
- **AI Service**: LLM integration and AI features
- **Search Service**: Full-text search with Elasticsearch
- **Export Service**: Data export in various formats
- **Import Service**: Bulk data import
- **Sync Service**: Real-time data synchronization
- **Audit Service**: Activity logging and compliance
- **Billing Service**: Subscription and payment processing
- **Admin Service**: Platform administration

## 🚀 Quick Start

### Prerequisites
- Go 1.23+
- Docker & Docker Compose
- PostgreSQL 16
- Redis 7
- Make

### Local Development

1. **Set up environment variables**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

2. **Start infrastructure**
   ```bash
   docker-compose -f docker-compose.phase1.yml up postgres redis
   ```

3. **Run a service locally**
   ```bash
   cd auth-service
   go run cmd/auth-service/main.go
   ```

4. **Run all Phase 1 services**
   ```bash
   docker-compose -f docker-compose.phase1.yml up
   ```

5. **Test the services**
   ```bash
   ./test-phase1.sh
   ```

## 📁 Service Structure

Each service follows a standard structure:

```
service-name/
├── cmd/
│   └── service-name/
│       └── main.go          # Entry point
├── internal/
│   ├── config/             # Configuration
│   ├── handlers/           # HTTP handlers
│   ├── models/             # Data models
│   ├── repository/         # Data access layer
│   ├── services/           # Business logic
│   └── middleware/         # HTTP middleware
├── pkg/                    # Shared packages
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

## 🔧 Common Operations

### Build a service
```bash
cd service-name
go build -o bin/service-name cmd/service-name/main.go
```

### Run tests
```bash
go test ./...
```

### Update dependencies
```bash
go mod tidy
```

### Generate mocks
```bash
go generate ./...
```

## 🌐 API Endpoints

### API Gateway Routes

#### Authentication
- `POST /api/v1/auth/register` - Register new user
- `POST /api/v1/auth/login` - User login
- `POST /api/v1/auth/refresh` - Refresh access token
- `POST /api/v1/auth/logout` - User logout

#### Users (Protected)
- `GET /api/v1/users/me` - Get current user
- `PUT /api/v1/users/me` - Update current user
- `GET /api/v1/users/:id` - Get user by ID
- `PUT /api/v1/users/:id` - Update user
- `DELETE /api/v1/users/:id` - Delete user
- `GET /api/v1/users` - List users

#### Airtable (Protected)
- `GET /api/v1/airtable/bases` - List bases
- `GET /api/v1/airtable/bases/:baseId` - Get base
- `GET /api/v1/airtable/bases/:baseId/tables` - List tables
- `GET /api/v1/airtable/bases/:baseId/tables/:tableId` - Get table
- `GET /api/v1/airtable/bases/:baseId/tables/:tableId/records` - List records
- `POST /api/v1/airtable/bases/:baseId/tables/:tableId/records` - Create record
- `GET /api/v1/airtable/bases/:baseId/tables/:tableId/records/:recordId` - Get record
- `PATCH /api/v1/airtable/bases/:baseId/tables/:tableId/records/:recordId` - Update record
- `DELETE /api/v1/airtable/bases/:baseId/tables/:tableId/records/:recordId` - Delete record

## 🔒 Security

- JWT authentication with access/refresh tokens
- Rate limiting per IP address
- CORS configuration
- Request ID tracking
- Secure password hashing with bcrypt
- SQL injection prevention
- Input validation

## 🗄️ Database Schema

Each service has its own database following the database-per-service pattern:

- **pyairtable_auth**: Users, sessions, tokens
- **pyairtable_users**: User profiles, preferences
- **pyairtable_tenants**: Organizations, subscriptions
- **pyairtable_workspaces**: Workspaces, projects
- **pyairtable_permissions**: Roles, permissions
- **pyairtable_workflows**: Automation rules, schedules
- **pyairtable_analytics**: Metrics, reports

## 🐳 Docker Support

Each service includes a multi-stage Dockerfile for optimal image size:

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder
# ... build steps ...

# Final stage
FROM alpine:latest
# ... runtime configuration ...
```

## 📊 Monitoring

Services expose metrics for Prometheus:
- Request count
- Request duration
- Error rate
- Custom business metrics

Health endpoints:
- `/health` - Basic health check
- `/ready` - Readiness probe
- `/metrics` - Prometheus metrics

## 🤝 Contributing

1. Follow the existing code structure
2. Write tests for new features
3. Update documentation
4. Use conventional commits
5. Run linters before committing

## 📝 License

Copyright (c) 2024 PyAirtable. All rights reserved.