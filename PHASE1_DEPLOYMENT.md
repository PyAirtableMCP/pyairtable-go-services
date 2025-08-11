# Phase 1 Services Deployment Guide

This guide covers the deployment and testing of the Phase 1 microservices for the PyAirtable platform.

## 🏗️ Phase 1 Services

1. **API Gateway** (Port 8080) - Central entry point for all API requests
2. **Auth Service** (Port 8001) - JWT authentication and user management  
3. **User Service** (Port 8002) - User profile management
4. **Airtable Gateway** (Port 8003) - Airtable API integration with caching

## 📋 Prerequisites

- Docker & Docker Compose
- PostgreSQL client (for migrations)
- Go 1.23+ (for local development)
- Python 3.11+ (for Airtable Gateway)
- Make
- jq (for testing scripts)

## 🚀 Quick Start

### 1. Set Environment Variables

```bash
# Copy example environment file
cp ../.env.example .env

# Edit .env with your values:
# - AIRTABLE_PAT: Your Airtable Personal Access Token
# - JWT_SECRET: A secure random string
# - INTERNAL_API_KEY: API key for service-to-service communication
```

### 2. Start Infrastructure

```bash
# Start PostgreSQL and Redis
docker-compose -f docker-compose.phase1.yml up -d postgres redis

# Wait for services to be healthy
docker-compose -f docker-compose.phase1.yml ps
```

### 3. Run Database Migrations

```bash
cd migrations
./run-migrations.sh
```

### 4. Build and Start Services

```bash
# Build all services
docker-compose -f docker-compose.phase1.yml build

# Start all services
docker-compose -f docker-compose.phase1.yml up -d

# Check service health
docker-compose -f docker-compose.phase1.yml ps
```

### 5. Test the Services

```bash
# Run the test script
./test-phase1.sh
```

## 🔧 Service Configuration

### API Gateway
- **Port**: 8080
- **Features**: Request routing, rate limiting, authentication middleware
- **Config**: Rate limit of 100 requests/minute per IP

### Auth Service
- **Port**: 8001
- **Features**: User registration, login, JWT tokens, refresh tokens
- **Database**: `pyairtable_auth`
- **Token TTL**: 24 hours (access), 7 days (refresh)

### User Service
- **Port**: 8002
- **Features**: User profile CRUD, tenant isolation, caching
- **Database**: `pyairtable_users`
- **Cache**: Redis with 15-minute TTL

### Airtable Gateway
- **Port**: 8003 (exposed as 8002 internally)
- **Features**: Airtable API proxy, response caching, rate limiting
- **Cache**: Redis with 1-hour TTL
- **Rate Limit**: 5 requests/second to Airtable API

## 📡 API Endpoints

### Authentication Flow

1. **Register a new user**
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePassword123!",
    "first_name": "John",
    "last_name": "Doe"
  }'
```

2. **Login**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePassword123!"
  }'
```

3. **Use the access token**
```bash
TOKEN="your-access-token"
curl -X GET http://localhost:8080/api/v1/users/me \
  -H "Authorization: Bearer $TOKEN"
```

### Airtable Integration

```bash
# List Airtable bases
curl -X GET http://localhost:8080/api/v1/airtable/bases \
  -H "Authorization: Bearer $TOKEN"

# Get records from a table
curl -X GET "http://localhost:8080/api/v1/airtable/bases/{baseId}/tables/{tableId}/records" \
  -H "Authorization: Bearer $TOKEN"
```

## 🐛 Troubleshooting

### Check Service Logs
```bash
# All services
docker-compose -f docker-compose.phase1.yml logs -f

# Specific service
docker-compose -f docker-compose.phase1.yml logs -f auth-service
```

### Database Connection Issues
```bash
# Test PostgreSQL connection
docker-compose -f docker-compose.phase1.yml exec postgres pg_isready

# Connect to database
docker-compose -f docker-compose.phase1.yml exec postgres psql -U postgres
```

### Redis Connection Issues
```bash
# Test Redis connection
docker-compose -f docker-compose.phase1.yml exec redis redis-cli ping
```

### Service Health Checks
```bash
# API Gateway
curl http://localhost:8080/health

# Auth Service
curl http://localhost:8001/health

# User Service  
curl http://localhost:8002/health

# Airtable Gateway
curl http://localhost:8003/health
```

## 🔄 Development Workflow

### Local Development

1. **Run a service locally**
```bash
# Auth Service
cd auth-service
go run cmd/auth-service/main.go

# User Service
cd user-service
go run cmd/user-service/main.go

# Airtable Gateway
cd ../python-services/airtable-gateway
python -m uvicorn src.main:app --reload
```

2. **Update dependencies**
```bash
# Go services
go mod tidy

# Python services
pip install -r requirements.txt
```

### Adding a New Endpoint

1. Update the service handler
2. Add route in service router
3. Update API Gateway proxy routes
4. Add tests
5. Update documentation

## 📊 Monitoring

### Service Metrics
- Request count per endpoint
- Response time percentiles
- Error rates
- Active connections

### Health Endpoints
- `/health` - Basic health check
- `/api/v1/info` - Service information

### Database Monitoring
```sql
-- Check connection count
SELECT count(*) FROM pg_stat_activity;

-- Check table sizes
SELECT relname, pg_size_pretty(pg_total_relation_size(relid))
FROM pg_catalog.pg_statio_user_tables
ORDER BY pg_total_relation_size(relid) DESC;
```

## 🚢 Production Deployment

### Environment Variables
```bash
# Production settings
ENVIRONMENT=production
LOG_LEVEL=INFO
CORS_ORIGINS=https://yourdomain.com

# Security
JWT_SECRET=<strong-random-string>
INTERNAL_API_KEY=<strong-random-string>

# Database
DATABASE_URL=postgres://user:pass@host:5432/dbname?sslmode=require

# Redis
REDIS_URL=redis://:password@host:6379
```

### Docker Compose Override
```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  api-gateway:
    restart: always
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
```

### Health Checks
All services include health check endpoints and Docker health checks for proper orchestration.

## 🔐 Security Considerations

1. **JWT Tokens**: Use strong secrets, implement token rotation
2. **Database**: Use SSL connections, implement RLS policies
3. **API Keys**: Rotate regularly, use environment variables
4. **CORS**: Configure specific origins for production
5. **Rate Limiting**: Adjust limits based on usage patterns

## 📝 Next Steps

1. Deploy remaining Phase 2 services
2. Implement service mesh (Istio/Linkerd)
3. Add distributed tracing (Jaeger)
4. Set up monitoring stack (Prometheus/Grafana)
5. Implement CI/CD pipeline