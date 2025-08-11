# api-gateway

Central API Gateway with routing and load balancing

## Port
- Default: 8080

## Development

```bash
# Install dependencies
go mod download

# Run locally
make run

# Run with hot reload
make dev

# Build Docker image
make docker-build

# Run tests
make test
```

## API Endpoints

- `GET /health` - Health check
- `GET /api/v1/info` - Service information

## Environment Variables

- `PORT` - Service port (default: 8080)
- `LOG_LEVEL` - Logging level (default: info)
