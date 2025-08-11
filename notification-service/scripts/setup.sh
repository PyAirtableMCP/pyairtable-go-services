#!/bin/bash

# {{SERVICE_NAME}} Setup Script
# This script sets up the development environment

set -e

echo "🚀 Setting up {{SERVICE_NAME}} development environment..."

# Check if required tools are installed
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo "❌ $1 is not installed. Please install it first."
        exit 1
    fi
    echo "✅ $1 is available"
}

echo "Checking required tools..."
check_tool go
check_tool docker
check_tool docker-compose

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file from template..."
    cp .env.example .env
    echo "✅ .env file created. Please review and update as needed."
else
    echo "✅ .env file already exists"
fi

# Download Go dependencies
echo "📦 Downloading Go dependencies..."
go mod download
go mod tidy

# Install development tools
echo "🔧 Installing development tools..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest

# Start development services
echo "🐳 Starting development services (PostgreSQL, Redis)..."
docker-compose up -d postgres redis

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 10

# Run database migrations (if any)
echo "🗄️  Running database migrations..."
make migrate-up || echo "ℹ️  No migrations to run"

# Run tests to verify setup
echo "🧪 Running tests to verify setup..."
make test || echo "⚠️  Some tests failed, but setup is complete"

echo ""
echo "🎉 Setup complete!"
echo ""
echo "Next steps:"
echo "  1. Review and update .env file if needed"
echo "  2. Run 'make run' to start the service"
echo "  3. Visit http://localhost:{{PORT}}/health to verify it's running"
echo ""
echo "Available commands:"
echo "  make help     - Show all available commands"
echo "  make run      - Start the service"
echo "  make test     - Run tests"
echo "  make dev      - Start development environment"
echo "  make dev-stop - Stop development environment"