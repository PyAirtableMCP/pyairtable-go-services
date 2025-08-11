# PyAirtable GraphQL Federation

A production-ready GraphQL federation gateway that provides a unified API across all PyAirtable microservices using Apollo Federation v2.

## 🚀 Features

### Core GraphQL Federation
- **Apollo Federation v2** implementation with schema composition
- **Unified GraphQL API** across 8+ microservices
- **Schema stitching** with automatic service discovery
- **Query planning** and optimized execution
- **Real-time subscriptions** with WebSocket support

### Performance & Scalability
- **DataLoader pattern** for N+1 query prevention
- **Multi-level caching** with Redis integration
- **Query complexity analysis** and depth limiting
- **Response caching** with intelligent invalidation
- **Connection pooling** and request batching

### Security & Authorization
- **Field-level authorization** with role-based access control
- **JWT authentication** with token validation
- **Rate limiting** per user and operation type
- **Query whitelisting** for production security
- **Schema introspection control**

### Developer Experience
- **GraphQL Playground** for interactive development
- **Automatic type generation** from schemas
- **Comprehensive examples** and documentation
- **Client SDK generation** for multiple languages
- **Migration guides** from REST to GraphQL

### Observability
- **Distributed tracing** with Jaeger integration
- **Performance monitoring** and metrics collection
- **Error tracking** and alerting
- **Query analytics** and usage statistics
- **Health checks** and status monitoring

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    GraphQL Gateway                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │  Complexity │  │    Rate     │  │    Auth     │        │
│  │   Analysis  │  │  Limiting   │  │  & AuthZ    │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Caching   │  │   Tracing   │  │ DataLoaders │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────┘
                               │
                    ┌──────────┴──────────┐
                    │  Apollo Federation  │
                    │   Query Planner     │
                    └──────────┬──────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         │                     │                     │
    ┌────▼────┐         ┌─────▼─────┐         ┌─────▼─────┐
    │  User   │         │Workspace  │         │ Airtable  │
    │ Service │         │ Service   │         │ Service   │
    │ (Go)    │         │   (Go)    │         │ (Python)  │
    └─────────┘         └───────────┘         └───────────┘
         │                     │                     │
    ┌────▼────┐         ┌─────▼─────┐         ┌─────▼─────┐
    │  File   │         │Permission │         │Analytics  │
    │ Service │         │ Service   │         │ Service   │
    │ (Go)    │         │   (Go)    │         │ (Python)  │
    └─────────┘         └───────────┘         └───────────┘
```

## 📋 Services Overview

| Service | Technology | Port | Purpose |
|---------|------------|------|---------|
| User Service | Go | 8001 | User management, authentication, profiles |
| Workspace Service | Go | 8002 | Workspace management, projects, teams |
| Airtable Service | Python | 8003 | Airtable integration, data sync |
| File Service | Go | 8004 | File upload, storage, processing |
| Permission Service | Go | 8005 | Authorization, permissions, RBAC |
| Notification Service | Go | 8006 | Notifications, alerts, messaging |
| Analytics Service | Python | 8007 | Data analytics, reporting |
| AI Service | Python | 8008 | AI features, content generation |

## 🛠️ Quick Start

### Prerequisites
- Node.js 18+
- Redis 6+
- PostgreSQL 13+
- Docker & Docker Compose (optional)

### Development Setup

1. **Clone and install dependencies:**
```bash
git clone <repository>
cd go-services/graphql-gateway
npm install
```

2. **Configure environment:**
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. **Start services with Docker:**
```bash
docker-compose up -d
```

4. **Start the gateway:**
```bash
npm run dev
```

5. **Open GraphQL Playground:**
```
http://localhost:4000/graphql
```

### Manual Setup

1. **Start Redis:**
```bash
redis-server
```

2. **Start PostgreSQL and create databases:**
```bash
psql -U postgres -f scripts/init-databases.sql
```

3. **Start individual services:**
```bash
# Terminal 1 - User Service
cd ../user-service
go run cmd/graphql-server/main.go

# Terminal 2 - Workspace Service  
cd ../workspace-service
go run cmd/graphql-server/main.go

# Terminal 3 - Airtable Service
cd ../../python-services/airtable-gateway
python -m uvicorn main:app --port 8003

# Continue for other services...
```

4. **Start the gateway:**
```bash
npm run dev
```

## 📖 Usage Examples

### Authentication
```graphql
mutation Login {
  login(input: {
    email: "user@example.com"
    password: "password123"
  }) {
    success
    token
    user {
      id
      firstName
      lastName
      email
    }
  }
}
```

### Complex Cross-Service Query
```graphql
query Dashboard($workspaceId: ID!) {
  workspace(id: $workspaceId) {
    id
    name
    members {
      user {
        firstName
        lastName
      }
      role
    }
    projects {
      name
      status
      progress
    }
    airtableBases {
      name
      recordCount
      lastSyncAt
    }
  }
  
  workspaceFiles(workspaceId: $workspaceId, pagination: { limit: 5 }) {
    files {
      filename
      size
      uploader {
        firstName
        lastName
      }
    }
  }
}
```

### Real-time Subscriptions
```graphql
subscription WorkspaceUpdates($workspaceId: ID!) {
  workspaceUpdated(workspaceId: $workspaceId) {
    id
    name
    memberCount
    updatedAt
  }
  
  fileUploaded(workspaceId: $workspaceId) {
    filename
    uploader {
      firstName
      lastName
    }
  }
}
```

### File Upload with Processing
```graphql
mutation UploadFile($input: UploadFileInput!) {
  uploadFile(input: $input) {
    file {
      id
      filename
      size
      status
    }
    processingJobs {
      type
      status
      progress
    }
  }
}
```

## 🔧 Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NODE_ENV` | `development` | Environment mode |
| `PORT` | `4000` | Gateway port |
| `JWT_SECRET` | - | JWT signing secret |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `MAX_QUERY_COMPLEXITY` | `1000` | Maximum query complexity |
| `MAX_QUERY_DEPTH` | `10` | Maximum query depth |
| `CACHE_TTL` | `300` | Cache TTL in seconds |
| `ENABLE_INTROSPECTION` | `true` | Enable schema introspection |
| `ENABLE_PLAYGROUND` | `true` | Enable GraphQL Playground |

### Service URLs

Configure subgraph URLs in your environment:

```bash
USERS_SUBGRAPH_URL=http://localhost:8001/graphql
WORKSPACES_SUBGRAPH_URL=http://localhost:8002/graphql
AIRTABLE_SUBGRAPH_URL=http://localhost:8003/graphql
FILES_SUBGRAPH_URL=http://localhost:8004/graphql
# ... etc
```

## 🎯 Key Features Deep Dive

### DataLoader Pattern
Eliminates N+1 queries by batching requests:

```typescript
// Automatically batches user lookups
const users = await Promise.all([
  context.dataloaders.userLoader.load('user1'),
  context.dataloaders.userLoader.load('user2'),
  context.dataloaders.userLoader.load('user3')
]);
```

### Query Complexity Analysis
Prevents expensive queries from overwhelming services:

```typescript
// Query complexity calculated based on:
// - Field complexity scores
// - Nested object multipliers  
// - Pagination limits
// - Custom field estimators
```

### Field-Level Authorization
Granular permission control:

```graphql
type User {
  id: ID!
  email: String! @auth(requires: [ADMIN, SELF])
  permissions: [Permission!]! @auth(requires: [ADMIN])
}
```

### Response Caching
Intelligent caching with automatic invalidation:

```typescript
// Cache keys generated from:
// - Query hash
// - Variables hash
// - User context
// - Cache tags for invalidation
```

### Real-time Subscriptions
WebSocket-based subscriptions with Redis pub/sub:

```typescript
// Filtered subscriptions with access control
subscription {
  workspaceUpdated(workspaceId: "ws123") {
    // Only receives updates for workspaces user has access to
  }
}
```

## 📊 Monitoring & Observability

### Metrics Collection
- Request count and latency
- Error rates by service
- Cache hit/miss ratios
- Query complexity distribution
- Rate limit violations

### Distributed Tracing
- Request flow across services
- Performance bottlenecks
- Error propagation
- Service dependencies

### Health Checks
```bash
# Gateway health
curl http://localhost:4000/health

# Service health
curl http://localhost:8001/health  # User service
curl http://localhost:8002/health  # Workspace service
```

## 🔒 Security

### Authentication
- JWT token validation
- Token refresh mechanism
- Secure cookie handling
- Multi-factor authentication support

### Authorization
- Role-based access control (RBAC)
- Permission-based authorization
- Field-level security
- Resource ownership checks

### Rate Limiting
- Per-user rate limits
- Operation-specific limits
- Complexity-based limiting
- Burst protection

### Query Security
- Depth limiting (max 10 levels)
- Complexity analysis (max 1000 points)
- Query whitelisting for production
- Introspection control

## 🚀 Deployment

### Docker Deployment
```bash
# Build and deploy all services
docker-compose up -d

# Scale specific services
docker-compose up -d --scale user-service=3
```

### Kubernetes Deployment
```bash
# Apply manifests
kubectl apply -f k8s/

# Monitor deployment
kubectl get pods -l app=graphql-gateway
```

### Production Considerations
- Disable introspection and playground
- Enable query whitelisting
- Configure proper CORS origins
- Set up monitoring and alerting
- Use environment-specific secrets
- Enable compression and caching

## 🧪 Testing

### Unit Tests
```bash
npm test
```

### Integration Tests
```bash
npm run test:integration
```

### Load Testing
```bash
# Using K6
k6 run performance-testing/load-test.js
```

### End-to-End Tests
```bash
npm run test:e2e
```

## 📚 API Documentation

### Schema Documentation
- Browse the schema in GraphQL Playground
- Generate documentation: `npm run generate-docs`
- View type definitions: `npm run introspect`

### Example Operations
- See `examples/` directory for comprehensive examples
- Query examples: `examples/queries.graphql`
- Mutation examples: `examples/mutations.graphql`
- Subscription examples: `examples/subscriptions.graphql`

## 🤝 Contributing

1. Follow the GraphQL schema design patterns
2. Add comprehensive tests for new features
3. Update documentation and examples
4. Ensure security best practices
5. Add monitoring and observability

## 📄 License

This project is licensed under the MIT License.

## 🔗 Related Projects

- [PyAirtable User Service](../user-service/README.md)
- [PyAirtable Workspace Service](../workspace-service/README.md)
- [PyAirtable File Service](../file-service/README.md)
- [PyAirtable Frontend](../../frontend-services/README.md)

## 📞 Support

For questions and support:
- Create an issue in the repository
- Check the [troubleshooting guide](docs/troubleshooting.md)
- Review the [FAQ](docs/faq.md)