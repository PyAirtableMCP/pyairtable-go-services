# PyAirtable API Versioning System

A comprehensive, production-ready API versioning system designed for PyAirtable that supports both REST and GraphQL APIs with advanced lifecycle management, analytics, and developer tools.

## 🎯 Overview

This versioning system provides a complete solution for API evolution that ensures smooth transitions while maintaining backward compatibility. It's built to handle complex enterprise requirements while being simple enough for teams to adopt quickly.

## ✨ Key Features

### 🔀 Multiple Versioning Strategies
- **URL Path Versioning**: `/api/v1/users`, `/api/v2/users`
- **Header Versioning**: `API-Version: v2`
- **Query Parameter Versioning**: `/api/users?version=v2`
- **Content Negotiation**: `Accept: application/vnd.pyairtable.v2+json`
- **Combined Strategy**: Automatic fallback between strategies

### 🏗️ GraphQL Schema Versioning
- Schema evolution with backward compatibility
- Apollo Federation support
- Automated introspection generation
- Breaking change detection
- Field transformation between schema versions

### ⏳ Lifecycle Management
- Automated deprecation workflows
- Sunset monitoring and enforcement
- Grace period management
- Approval-based transitions
- Notification automation

### 🔄 Request/Response Transformation
- Automatic data transformation between versions
- Field mapping and renaming
- Type conversion
- Custom transformation functions
- Conditional transformations

### 📊 Real-time Analytics
- Usage tracking per version
- Performance monitoring
- Error rate analysis
- Client behavior analytics
- A/B testing support

### 🌅 Sunset Monitoring
- Client migration tracking
- Impact analysis
- Automated warnings
- Grace period enforcement
- Rollback capabilities

### 🧪 A/B Testing
- Version-based experiments
- Statistical analysis
- Automatic winner detection
- Gradual rollout support
- Performance comparison

### 🛠️ Developer Tools
- Migration guides generation
- Compatibility matrix
- Auto-generated documentation
- SDK version management
- Change impact analysis

## 🚀 Quick Start

### 1. Installation

```go
import "github.com/pyairtable/pyairtable-compose/go-services/pkg/versioning"
```

### 2. Basic Setup

```go
// Create version manager
config := &versioning.ManagerConfig{
    DefaultStrategy:    versioning.StrategyCombined,
    EnableAnalytics:    true,
    EnableTransforms:   true,
}

versionManager := versioning.NewVersionManager(config)
versionManager.Initialize()

// Add versioning middleware to your HTTP handler
handler := versionManager.GetMiddleware()(yourAPIHandler)
```

### 3. Register API Versions

```go
// Register v1
v1 := &versioning.Version{
    Major:       1,
    Minor:       0,
    Patch:       0,
    Status:      versioning.StatusStable,
    ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
    Features: []string{
        "user_management",
        "basic_auth",
    },
}
versionManager.RegisterVersion(v1)

// Register v2 with breaking changes
v2 := &versioning.Version{
    Major:       2,
    Minor:       0,
    Patch:       0,
    Status:      versioning.StatusStable,
    ReleaseDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
    Features: []string{
        "user_management",
        "oauth2_auth",
        "graphql_support",
    },
    BreakingChanges: []versioning.BreakingChange{
        {
            Description:   "Authentication changed to OAuth2",
            AffectedPaths: []string{"/auth/*"},
            MigrationType: versioning.MigrationBehavior,
            MigrationGuide: "Update to OAuth2 authentication",
        },
    },
}
versionManager.RegisterVersion(v2)
```

### 4. Set Up Transformations

```go
transformer := versionManager.GetTransformer()

// Transform user data from v1 to v2
transformer.RegisterRule(versioning.TransformationRule{
    Name:            "v1_to_v2_user_id",
    SourceVersion:   "v1",
    TargetVersion:   "v2",
    ApplyToResponse: true,
    FieldMappings: []versioning.FieldMapping{
        {
            Operation:   versioning.OpTransform,
            SourcePath:  "user_id",
            TargetPath:  "user_id",
            Transform:   "integer_to_uuid",
        },
    },
})
```

## 📖 Usage Examples

### Version Detection

The system automatically detects versions using multiple strategies:

```bash
# URL path versioning
curl http://localhost:8080/api/v2/users

# Header versioning
curl -H "API-Version: v2" http://localhost:8080/api/users

# Query parameter versioning
curl http://localhost:8080/api/users?version=v2

# Content type versioning
curl -H "Accept: application/vnd.pyairtable.v2+json" http://localhost:8080/api/users
```

### Request Context

Access version information in your handlers:

```go
func yourHandler(w http.ResponseWriter, r *http.Request) {
    vctx, ok := versioning.GetVersionFromContext(r.Context())
    if !ok {
        http.Error(w, "Version context not found", http.StatusInternalServerError)
        return
    }
    
    version := vctx.Version
    fmt.Printf("Handling request for version: %s\n", version.String())
    
    // Handle request based on version
    switch version.String() {
    case "v1":
        handleV1Request(w, r)
    case "v2":
        handleV2Request(w, r)
    }
}
```

### GraphQL Versioning

```go
// Register GraphQL schemas
gqlManager := versioning.NewGraphQLVersionManager()

v1Schema := &versioning.GraphQLSchema{
    Version: "v1",
    SDL: `
        type User {
            id: Int!
            name: String!
            email: String!
        }
        
        type Query {
            user(id: Int!): User
        }
    `,
    Status: versioning.StatusStable,
}

gqlManager.RegisterSchema(v1Schema)

// Use GraphQL versioning middleware
graphqlHandler := gqlManager.GraphQLVersionMiddleware()(yourGraphQLHandler)
```

### Analytics Tracking

```go
// Track custom events
analytics := versionManager.GetAnalytics()

event := &versioning.AnalyticsEvent{
    Type:         versioning.EventAPIRequest,
    Version:      "v2",
    ClientID:     "client-123",
    Method:       "GET",
    Path:         "/api/users",
    ResponseTime: 150 * time.Millisecond,
}

analytics.TrackEvent(context.Background(), event)
```

## 🏗️ Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    Version Manager                              │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Detector      │  │   Registry      │  │  Transformer    │  │
│  │                 │  │                 │  │                 │  │
│  │ • URL Path      │  │ • Version Store │  │ • Field Mapping │  │
│  │ • Headers       │  │ • Metadata      │  │ • Type Convert  │  │
│  │ • Query Params  │  │ • Features      │  │ • Custom Funcs  │  │
│  │ • Content-Type  │  │ • Lifecycle     │  │ • Validation    │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                  Lifecycle Management                          │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Scheduler     │  │  Notifications  │  │   Migration     │  │
│  │                 │  │                 │  │                 │  │
│  │ • Transitions   │  │ • Email         │  │ • Guides        │  │
│  │ • Approvals     │  │ • Webhooks      │  │ • Tools         │  │
│  │ • Automation    │  │ • Slack         │  │ • Validation    │  │
│  │ • Monitoring    │  │ • In-App        │  │ • Tracking      │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    Analytics & Monitoring                      │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Analytics     │  │    Sunset       │  │   A/B Testing   │  │
│  │                 │  │   Monitoring    │  │                 │  │
│  │ • Usage Stats   │  │ • Client Track  │  │ • Experiments   │  │
│  │ • Performance   │  │ • Migration     │  │ • Statistics    │  │
│  │ • Error Rates   │  │ • Impact        │  │ • Rollout       │  │
│  │ • Dashboards    │  │ • Enforcement   │  │ • Analysis      │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Request Flow

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Client    │───▶│  Gateway    │───▶│  Versioning │───▶│   Handler   │
│   Request   │    │             │    │ Middleware  │    │             │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                                            │
                                            ▼
                   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
                   │  Analytics  │◀───│   Version   │───▶│Transform    │
                   │  Tracking   │    │  Detection  │    │  Engine     │
                   └─────────────┘    └─────────────┘    └─────────────┘
```

## 🔧 Configuration

### Version Manager Configuration

```go
config := &versioning.ManagerConfig{
    // Version detection strategy
    DefaultStrategy:    versioning.StrategyCombined,
    FallbackStrategies: []versioning.VersioningStrategy{
        versioning.StrategyPath,
        versioning.StrategyHeader,
        versioning.StrategyQuery,
    },
    
    // Feature flags
    EnableAnalytics:  true,
    EnableTransforms: true,
    
    // Deprecation policies
    DeprecationPolicy: &versioning.DeprecationPolicy{
        WarningPeriod:       30 * 24 * time.Hour,
        DeprecationPeriod:   90 * 24 * time.Hour,
        NotificationMethods: []string{"email", "webhook"},
    },
    
    // Sunset policies
    SunsetPolicy: &versioning.SunsetPolicy{
        GracePeriod:      30 * 24 * time.Hour,
        ForceUpgrade:     false,
        MaintenanceMode:  true,
        RedirectToNewest: true,
    },
}
```

### Analytics Configuration

```go
analyticsConfig := &versioning.AnalyticsConfig{
    Enabled:             true,
    SamplingRate:        1.0, // 100% sampling
    RetentionPeriod:     30 * 24 * time.Hour,
    AggregationInterval: 5 * time.Minute,
    AlertThresholds: &versioning.AlertThresholds{
        ErrorRateThreshold:      0.05,  // 5%
        ResponseTimeP99:         2 * time.Second,
        LowUsageThreshold:       100,   // requests/day
        DeprecatedUsageWarning:  1000,  // requests/day
    },
}
```

## 📚 API Reference

### Core Types

#### Version
```go
type Version struct {
    Major          int
    Minor          int  
    Patch          int
    Label          string               // e.g., "beta", "alpha"
    ReleaseDate    time.Time
    DeprecatedDate *time.Time
    SunsetDate     *time.Time
    Status         VersionStatus
    Features       []string
    BreakingChanges []BreakingChange
}
```

#### VersionManager
```go
type VersionManager struct {
    // DetectVersion detects version from HTTP request
    DetectVersion(r *http.Request) (*Version, VersioningStrategy, error)
    
    // GetMiddleware returns versioning middleware
    GetMiddleware() func(http.Handler) http.Handler
    
    // RegisterVersion registers a new version
    RegisterVersion(version *Version)
    
    // DeprecateVersion marks version as deprecated
    DeprecateVersion(versionStr string, sunsetDate time.Time) error
}
```

### Developer Tools

#### Migration Guide
```go
type MigrationGuide struct {
    FromVersion   string
    ToVersion     string
    Title         string
    Complexity    MigrationComplexity
    EstimatedTime time.Duration
    Steps         []MigrationStep
    CodeExamples  []CodeExample
    Prerequisites []string
    Warnings      []string
}
```

#### Compatibility Matrix
```go
type CompatibilityInfo struct {
    Compatible      bool
    BreakingChanges []BreakingChange
    MigrationPath   []string
    Effort          MigrationEffort
    AutoMigration   bool
    Tools           []string
    EstimatedTime   time.Duration
}
```

## 🧪 Testing

### Unit Tests

```bash
# Run all versioning tests
go test ./pkg/versioning/...

# Run with coverage
go test -cover ./pkg/versioning/...

# Run specific test
go test -run TestVersionDetection ./pkg/versioning/
```

### Integration Tests

```bash
# Start test server
go run cmd/versioning-demo/main.go

# Run integration tests
curl http://localhost:8080/api/v1/users
curl http://localhost:8080/api/v2/users
curl -H "API-Version: v2" http://localhost:8080/api/users
```

### Load Testing

```bash
# Install k6
go install go.k6.io/k6@latest

# Run load test
k6 run --vus 10 --duration 30s scripts/load-test.js
```

## 🚀 Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o versioning-demo cmd/versioning-demo/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/versioning-demo .
EXPOSE 8080
CMD ["./versioning-demo"]
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pyairtable-api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: pyairtable-api
  template:
    metadata:
      labels:
        app: pyairtable-api
    spec:
      containers:
      - name: api
        image: pyairtable/api-versioning:latest
        ports:
        - containerPort: 8080
        env:
        - name: ENABLE_ANALYTICS
          value: "true"
        - name: DEFAULT_STRATEGY
          value: "combined"
---
apiVersion: v1
kind: Service
metadata:
  name: pyairtable-api-service
spec:
  selector:
    app: pyairtable-api
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

## 📊 Monitoring & Observability

### Metrics

The system exports metrics for monitoring:

- `api_requests_total{version, method, status}` - Total API requests
- `api_request_duration_seconds{version}` - Request duration histogram
- `api_version_usage{version}` - Version usage counter
- `api_errors_total{version, error_type}` - Error counter
- `deprecated_version_usage{version}` - Deprecated version usage

### Dashboards

Pre-built Grafana dashboards available:

- **Version Overview**: Usage distribution, error rates, performance
- **Lifecycle Management**: Deprecation status, migration progress
- **Client Analytics**: Client behavior, version adoption
- **Sunset Monitoring**: Migration tracking, impact analysis

### Alerts

Recommended alerts:

- High error rate on any version (>5%)
- Deprecated version usage above threshold
- New version adoption slower than expected
- Client failing to migrate before sunset

## 🤝 Contributing

### Development Setup

```bash
# Clone repository
git clone https://github.com/pyairtable/pyairtable-compose.git
cd pyairtable-compose/go-services

# Install dependencies
go mod download

# Run tests
go test ./pkg/versioning/...

# Start demo server
go run cmd/versioning-demo/main.go
```

### Code Style

- Follow Go best practices
- Use meaningful variable names
- Add comments for public APIs
- Include tests for new features
- Update documentation

### Pull Request Process

1. Fork the repository
2. Create feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Update documentation
6. Submit pull request

## 📄 License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## 🆘 Support

### Documentation

- [API Reference](docs/api-reference.md)
- [Migration Guides](docs/migration-guides/)
- [Best Practices](docs/best-practices.md)
- [Troubleshooting](docs/troubleshooting.md)

### Community

- [GitHub Issues](https://github.com/pyairtable/pyairtable-compose/issues)
- [Discussions](https://github.com/pyairtable/pyairtable-compose/discussions)
- [Slack Channel](https://pyairtable.slack.com/channels/api-versioning)

### Enterprise Support

For enterprise support, contact: support@pyairtable.com

## 🗺️ Roadmap

### v1.1 (Next Release)
- [ ] Enhanced GraphQL federation support
- [ ] Advanced A/B testing features
- [ ] Improved analytics dashboards
- [ ] SDK auto-generation

### v2.0 (Future)
- [ ] Machine learning-based migration recommendations
- [ ] Automated compatibility testing
- [ ] Advanced client segmentation
- [ ] Real-time collaboration features

### Long-term
- [ ] Multi-cloud deployment support
- [ ] Advanced AI-powered optimization
- [ ] Integration with popular API gateways
- [ ] Enterprise SSO integration

## 🏆 Acknowledgments

- Inspired by industry best practices from Google, Facebook, and GitHub APIs
- Built with the Go community's excellent libraries
- Thanks to all contributors and beta testers

---

**Built with ❤️ for the PyAirtable community**