# PyAirtable Plugin Service

A comprehensive plugin system for PyAirtable that enables secure third-party extensibility through WebAssembly-based plugins.

## Features

### 🔒 Security-First Architecture
- **WebAssembly Sandbox**: Plugins run in isolated WASM environments
- **Code Signing**: Digital signature verification for plugin authenticity
- **Permission System**: Granular access control for plugin capabilities
- **Resource Limits**: CPU, memory, and network usage constraints
- **Quarantine System**: Automatic plugin isolation for security violations

### ⚡ High-Performance Runtime
- **WASM Runtime**: Fast and secure plugin execution with wazero/wasmtime
- **Hot Reload**: Development-time plugin reloading
- **Resource Monitoring**: Real-time tracking of plugin resource usage
- **Instance Pooling**: Efficient plugin instance management
- **Precompilation**: Optimized plugin loading and execution

### 🔧 Developer Experience
- **TypeScript SDK**: Rich SDK for plugin development
- **CLI Tools**: Comprehensive toolchain for plugin creation and management
- **Development Server**: Local testing environment with live reload
- **Plugin Templates**: Ready-to-use templates for common plugin types
- **Documentation Generator**: Automatic API documentation

### 📦 Plugin Registry
- **Multi-Registry Support**: Official, community, and private registries
- **Plugin Discovery**: Search and browse available plugins
- **Version Management**: Semantic versioning and dependency resolution
- **Metadata Sync**: Automatic registry synchronization
- **Installation Management**: Plugin lifecycle management

### 📊 Monitoring & Observability
- **Prometheus Metrics**: Comprehensive metrics collection
- **Distributed Tracing**: Request tracing with OpenTelemetry
- **Audit Logging**: Complete audit trail of plugin operations
- **Performance Analytics**: Plugin execution statistics
- **Health Monitoring**: Service and plugin health checks

## Quick Start

### Prerequisites
- Go 1.21+
- PostgreSQL 13+
- Redis 6+
- Docker (optional)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/pyairtable/plugin-service.git
   cd plugin-service
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Set up the database**
   ```bash
   # Create database
   createdb plugin_service
   
   # Run migrations
   go run cmd/migrate/main.go up
   ```

4. **Configure the service**
   ```bash
   cp configs/config.yaml.example configs/config.yaml
   # Edit config.yaml with your settings
   ```

5. **Start the service**
   ```bash
   go run cmd/plugin-service/main.go
   ```

### Using Docker

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f plugin-service

# Stop services
docker-compose down
```

### Development with monitoring

```bash
# Start with monitoring stack
docker-compose --profile monitoring up -d

# Access services:
# - Plugin Service: http://localhost:8080
# - Prometheus: http://localhost:9090
# - Grafana: http://localhost:3001 (admin/admin)
```

## Plugin Development

### Install the CLI

```bash
# Install the plugin CLI
go install ./tools/cli

# Verify installation
pyairtable-plugin --version
```

### Create a New Plugin

```bash
# Create a TypeScript plugin
pyairtable-plugin new my-plugin --template typescript

# Create a formula plugin
pyairtable-plugin new my-formula --template formula

# Create a UI plugin
pyairtable-plugin new my-ui --template ui
```

### Plugin Structure

```
my-plugin/
├── plugin.yaml          # Plugin manifest
├── src/
│   ├── index.ts         # Main plugin code
│   ├── types.ts         # Type definitions
│   └── utils.ts         # Utility functions
├── tests/
│   └── index.test.ts    # Plugin tests
├── package.json         # Dependencies
└── README.md           # Plugin documentation
```

### Build and Test

```bash
cd my-plugin

# Build the plugin
pyairtable-plugin build

# Run tests
pyairtable-plugin test

# Validate the plugin
pyairtable-plugin validate

# Start development server
pyairtable-plugin dev
```

### Plugin Types

#### Formula Plugin
```typescript
import { createPlugin, defineFormula } from '@pyairtable/plugin-sdk';

const sumFormula = defineFormula(
  'SUM',
  'Calculates the sum of numbers',
  'math',
  [
    { name: 'numbers', type: 'array', description: 'Array of numbers', required: true }
  ],
  'number',
  (numbers: number[]) => numbers.reduce((a, b) => a + b, 0)
);

export default createPlugin({
  name: 'math-formulas',
  version: '1.0.0',
  type: 'formula',
  description: 'Mathematical formula functions',
  author: 'Your Name',
  entryPoint: 'index',
}, {
  initialize() {
    this.api.formulas.functions.SUM = sumFormula;
  }
});
```

#### UI Plugin
```typescript
import { createPlugin, defineUIComponent } from '@pyairtable/plugin-sdk';

const CustomButton = defineUIComponent('button', {
  text: 'Click me',
  onClick: () => console.log('Button clicked!'),
  variant: 'primary'
});

export default createPlugin({
  name: 'custom-ui',
  version: '1.0.0',
  type: 'ui',
  description: 'Custom UI components',
  author: 'Your Name',
  entryPoint: 'index',
  permissions: [
    { resource: 'ui', actions: ['render', 'update'] }
  ]
}, {
  initialize() {
    // Register UI components
  },
  
  execute(functionName, input, context) {
    if (functionName === 'renderButton') {
      return { success: true, result: CustomButton };
    }
  }
});
```

#### Webhook Plugin
```typescript
import { createPlugin } from '@pyairtable/plugin-sdk';

export default createPlugin({
  name: 'webhook-handler',
  version: '1.0.0',
  type: 'webhook',
  description: 'Custom webhook processing',
  author: 'Your Name',
  entryPoint: 'index',
  permissions: [
    { resource: 'webhooks', actions: ['create', 'delete'] },
    { resource: 'records', actions: ['read', 'write'] }
  ]
}, {
  async execute(functionName, input, context) {
    if (functionName === 'processWebhook') {
      const { payload, headers } = input;
      
      // Process the webhook payload
      const record = await context.api.records.create('table_id', {
        'Name': payload.name,
        'Status': 'processed'
      });
      
      return { success: true, result: { recordId: record.id } };
    }
  }
});
```

## API Reference

### REST API

The plugin service exposes a comprehensive REST API:

#### Plugins
- `GET /api/v1/plugins` - List plugins
- `POST /api/v1/plugins` - Create plugin
- `GET /api/v1/plugins/{id}` - Get plugin
- `PUT /api/v1/plugins/{id}` - Update plugin
- `DELETE /api/v1/plugins/{id}` - Delete plugin
- `POST /api/v1/plugins/{id}/validate` - Validate plugin

#### Installations
- `GET /api/v1/installations` - List installations
- `POST /api/v1/installations` - Install plugin
- `GET /api/v1/installations/{id}` - Get installation
- `PUT /api/v1/installations/{id}` - Update installation
- `DELETE /api/v1/installations/{id}` - Uninstall plugin
- `POST /api/v1/installations/{id}/execute` - Execute plugin

#### Registry
- `GET /api/v1/registry` - Search registry
- `GET /api/v1/registry/{name}` - Get plugin from registry
- `POST /api/v1/registry/sync` - Sync registries

### SDK Reference

The TypeScript SDK provides a rich set of APIs:

#### Core APIs
- `api.records` - Record operations (CRUD)
- `api.fields` - Field management
- `api.tables` - Table operations
- `api.bases` - Base management
- `api.attachments` - File attachments
- `api.comments` - Comments system
- `api.webhooks` - Webhook management
- `api.formulas` - Formula functions

#### Storage API
```typescript
// Plugin-scoped storage
await context.storage.set('key', { data: 'value' });
const data = await context.storage.get('key');
await context.storage.delete('key');
```

#### Events API
```typescript
// Listen to events
context.on('record.created', (data) => {
  console.log('New record:', data);
});

// Emit events
context.emit('custom.event', { message: 'Hello' });
```

## Configuration

### Environment Variables

All configuration can be overridden using environment variables with the `PLUGIN_SERVICE_` prefix:

```bash
PLUGIN_SERVICE_DATABASE_HOST=localhost
PLUGIN_SERVICE_DATABASE_PORT=5432
PLUGIN_SERVICE_REDIS_HOST=localhost
PLUGIN_SERVICE_SECURITY_JWT_SECRET=your-secret
```

### Resource Limits

Configure default and maximum resource limits:

```yaml
runtime:
  default_limits:
    max_memory_mb: 64
    max_cpu_percent: 50
    max_execution_ms: 5000
    max_storage_mb: 10
    max_network_reqs: 10
  
  max_limits:
    max_memory_mb: 256
    max_cpu_percent: 100
    max_execution_ms: 30000
    max_storage_mb: 100
    max_network_reqs: 100
```

### Security Configuration

```yaml
security:
  code_signing_enabled: true
  trusted_signers:
    - "-----BEGIN PUBLIC KEY-----\n..."
  sandbox_enabled: true
  permission_check_enabled: true
  quarantine_enabled: true
  quarantine_time: 24h
```

## Deployment

### Kubernetes

Deploy to Kubernetes using the provided manifests:

```bash
# Apply namespace
kubectl apply -f deployments/k8s/namespace.yaml

# Apply configurations and secrets
kubectl apply -f deployments/k8s/configmap.yaml
kubectl apply -f deployments/k8s/secrets.yaml

# Deploy the service
kubectl apply -f deployments/k8s/deployment.yaml

# Verify deployment
kubectl get pods -n pyairtable -l app=plugin-service
```

### Docker Swarm

```bash
# Deploy to Docker Swarm
docker stack deploy -c docker-compose.prod.yml plugin-service
```

### Monitoring

The service includes comprehensive monitoring:

#### Prometheus Metrics
- Plugin execution metrics
- Runtime performance metrics  
- Security violation metrics
- Registry synchronization metrics
- HTTP request metrics
- System resource metrics

#### Grafana Dashboards
- Plugin Service Overview
- Plugin Performance Analysis
- Security Monitoring
- Resource Usage Analysis

#### Alerts
- High plugin execution failure rate
- Resource limit violations
- Security violations detected
- Service unavailability

## Security

### Plugin Security Model

1. **Sandboxing**: All plugins run in isolated WebAssembly environments
2. **Permissions**: Plugins must declare required permissions
3. **Resource Limits**: CPU, memory, and network usage constraints
4. **Code Signing**: Plugins can be digitally signed for authenticity
5. **Quarantine**: Malicious plugins are automatically quarantined

### Best Practices

1. **Always verify plugin signatures** in production
2. **Set appropriate resource limits** based on your infrastructure
3. **Monitor plugin execution metrics** for anomalies
4. **Keep the service updated** with security patches
5. **Use private registries** for internal plugins

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run the test suite
6. Submit a pull request

### Development Setup

```bash
# Install development dependencies
make dev-deps

# Run tests
make test

# Run linting
make lint

# Build the service
make build

# Start development environment
make dev-up
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- Documentation: https://docs.pyairtable.com/plugins
- Issues: https://github.com/pyairtable/plugin-service/issues
- Discussions: https://github.com/pyairtable/plugin-service/discussions
- Email: plugins@pyairtable.com