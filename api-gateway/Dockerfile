# Build stage
FROM golang:1.23-alpine AS builder

# Install dependencies for building
RUN apk add --no-cache git ca-certificates tzdata

# Set the working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o api-gateway \
    ./cmd/api-gateway/main.go

# Runtime stage
FROM scratch

# Copy timezone data and certificates from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/api-gateway /api-gateway

# Copy configuration files
COPY --from=builder /app/configs /configs

# Create a non-root user (using numeric IDs for scratch image)
USER 65534:65534

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/api-gateway", "--health-check"] || exit 1

# Set environment variables
ENV ENVIRONMENT=production
ENV LOG_LEVEL=info
ENV PORT=8080

# Run the application
ENTRYPOINT ["/api-gateway"]
