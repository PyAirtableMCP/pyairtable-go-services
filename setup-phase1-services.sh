#!/bin/bash

# Setup Phase 1 Go microservices with proper structure

echo "🚀 Setting up Phase 1 Go microservices..."

# Create a proper Go workspace structure
cd /Users/kg/IdeaProjects/pyairtable-compose/go-services

# Phase 1 Services
SERVICES=(
    "api-gateway"
    "auth-service"
    "user-service"
)

for service in "${SERVICES[@]}"; do
    echo "📦 Setting up $service..."
    
    # Ensure service directory exists
    mkdir -p $service
    cd $service
    
    # Initialize Go module with proper versioning
    go mod init github.com/pyairtable/$service
    
    # Update to use Fiber v2 and other stable dependencies
    go get github.com/gofiber/fiber/v2@latest
    go get github.com/golang-jwt/jwt/v5@latest
    go get gorm.io/gorm@latest
    go get gorm.io/driver/postgres@latest
    go get github.com/redis/go-redis/v9@latest
    go get github.com/prometheus/client_golang@latest
    go get go.uber.org/zap@latest
    go get github.com/stretchr/testify@latest
    go get github.com/joho/godotenv@latest
    
    # Create proper service structure
    mkdir -p cmd/$service
    mkdir -p internal/{config,handlers,middleware,models,repository,service}
    mkdir -p pkg/{logger,database,cache,metrics}
    mkdir -p api/v1
    mkdir -p deployments/kubernetes
    mkdir -p scripts
    mkdir -p tests/{unit,integration,e2e}
    
    # Create main.go with proper structure
    cat > cmd/$service/main.go << 'EOF'
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/joho/godotenv"
    "go.uber.org/zap"
)

func main() {
    // Load environment variables
    if err := godotenv.Load(); err != nil {
        log.Println("No .env file found")
    }

    // Initialize structured logger
    zapLogger, _ := zap.NewProduction()
    defer zapLogger.Sync()
    sugar := zapLogger.Sugar()

    // Create Fiber app with proper configuration
    app := fiber.New(fiber.Config{
        ServerHeader:          "PyAirtable",
        DisableStartupMessage: false,
        AppName:               "SERVICE_NAME v1.0.0",
        ReadTimeout:           10 * time.Second,
        WriteTimeout:          10 * time.Second,
        IdleTimeout:           120 * time.Second,
    })

    // Middleware
    app.Use(recover.New())
    app.Use(logger.New())
    app.Use(cors.New(cors.Config{
        AllowOrigins: os.Getenv("CORS_ORIGINS"),
        AllowHeaders: "Origin, Content-Type, Accept, Authorization",
    }))

    // Health check
    app.Get("/health", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{
            "status": "healthy",
            "service": "SERVICE_NAME",
            "version": "1.0.0",
        })
    })

    // Graceful shutdown
    go func() {
        port := os.Getenv("PORT")
        if port == "" {
            port = "8080"
        }
        if err := app.Listen(":" + port); err != nil {
            sugar.Fatalf("Failed to start server: %v", err)
        }
    }()

    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    sugar.Info("Shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := app.ShutdownWithContext(ctx); err != nil {
        sugar.Fatalf("Server forced to shutdown: %v", err)
    }
    
    sugar.Info("Server exited")
}
EOF
    
    # Replace SERVICE_NAME with actual service name
    sed -i "" "s/SERVICE_NAME/$service/g" cmd/$service/main.go
    
    # Create Dockerfile
    cat > Dockerfile << 'EOF'
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/SERVICE_NAME/main.go

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080
CMD ["./main"]
EOF
    
    sed -i "" "s/SERVICE_NAME/$service/g" Dockerfile
    
    # Create Kubernetes deployment
    cat > deployments/kubernetes/deployment.yaml << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $service
  namespace: pyairtable
spec:
  replicas: 2
  selector:
    matchLabels:
      app: $service
  template:
    metadata:
      labels:
        app: $service
    spec:
      containers:
      - name: $service
        image: pyairtable/$service:latest
        ports:
        - containerPort: 8080
        env:
        - name: PORT
          value: "8080"
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: $service-secrets
              key: database-url
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: $service
  namespace: pyairtable
spec:
  selector:
    app: $service
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
EOF
    
    # Go back to services directory
    cd ..
done

echo "✅ Phase 1 services structure created!"

# Create shared packages
echo "📚 Creating shared packages..."
mkdir -p shared/{auth,database,cache,middleware,models}

cd shared
go mod init github.com/pyairtable/shared

# Add shared dependencies
go get github.com/gofiber/fiber/v2@latest
go get github.com/golang-jwt/jwt/v5@latest
go get gorm.io/gorm@latest
go get github.com/redis/go-redis/v9@latest

echo "✅ Shared packages created!"

echo "🎉 Phase 1 setup complete! Next steps:"
echo "1. Implement service-specific business logic"
echo "2. Set up inter-service communication"
echo "3. Configure databases using the schemas created"
echo "4. Deploy to Kubernetes"