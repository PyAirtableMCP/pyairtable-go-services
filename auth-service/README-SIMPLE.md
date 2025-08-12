# Simple Auth Service

A minimal Go-based authentication service with hardcoded credentials for development and testing.

## Features

- Single login endpoint with hardcoded credentials
- JWT token generation and signing
- CORS support for frontend integration
- Health check endpoint
- Zero database dependencies

## Quick Start

### Build and Run

```bash
cd /Users/kg/IdeaProjects/pyairtable-go-services/auth-service
go build -o bin/auth-service ./cmd/auth-service
./bin/auth-service
```

The service starts on port 8080 by default. Use `PORT=9000 ./bin/auth-service` to specify a different port.

### Hardcoded Credentials

- **Email**: `admin@test.com`
- **Password**: `admin123`

## API Endpoints

### Health Check
```bash
GET /health
```
Response:
```json
{
  "status": "ok",
  "service": "auth-service",
  "timestamp": 1755000086
}
```

### Login
```bash
POST /auth/login
Content-Type: application/json

{
  "email": "admin@test.com",
  "password": "admin123"
}
```

Success Response (200):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "1",
    "email": "admin@test.com"
  }
}
```

Error Response (401):
```json
{
  "error": "Invalid credentials"
}
```

## JWT Token Details

- **Algorithm**: HS256
- **Expiration**: 24 hours
- **Secret**: `your-super-secret-jwt-key` (hardcoded)
- **Claims**: email, user_id, standard JWT claims

## CORS Configuration

- **Allow Origins**: `*` (all origins)
- **Allow Methods**: GET, POST, PUT, DELETE, OPTIONS, PATCH
- **Allow Headers**: Origin, Content-Type, Accept, Authorization
- **Allow Credentials**: false

## Usage with Frontend

```javascript
// Login request
const response = await fetch('http://localhost:8080/auth/login', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    email: 'admin@test.com',
    password: 'admin123'
  })
});

const data = await response.json();
if (response.ok) {
  const { token, user } = data;
  // Store token for subsequent requests
  localStorage.setItem('authToken', token);
}
```

## Development Notes

- No database required
- No external services needed
- Single binary deployment
- ~150 lines of Go code
- JWT secret is hardcoded (change for production)

## File Structure

```
auth-service/
├── cmd/auth-service/main.go    # Main application file (~150 lines)
├── go.mod                      # Minimal dependencies
├── bin/auth-service           # Built binary
└── README-SIMPLE.md           # This file
```

## Dependencies

- `github.com/gofiber/fiber/v2` - HTTP framework
- `github.com/golang-jwt/jwt/v5` - JWT library

Total: 2 direct dependencies, ~10 transitive dependencies.