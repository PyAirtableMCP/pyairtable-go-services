# File Processing Service Makefile

.PHONY: help build test clean run docker dev deps lint format vet security audit docker-build docker-run docker-push deploy benchmark

# Variables
APP_NAME := file-processing-service
GO_VERSION := 1.21
DOCKER_IMAGE := $(APP_NAME)
DOCKER_TAG := latest
REGISTRY := your-registry.com
NAMESPACE := file-processing

# Colors for output
RED := \033[31m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
RESET := \033[0m

help: ## Show this help message
	@echo "$(BLUE)File Processing Service$(RESET)"
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	@echo "$(BLUE)Building $(APP_NAME)...$(RESET)"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags='-w -s -extldflags "-static"' \
		-a -installsuffix cgo \
		-o bin/$(APP_NAME) \
		./cmd/$(APP_NAME)
	@echo "$(GREEN)Build completed successfully$(RESET)"

build-local: ## Build for local development
	@echo "$(BLUE)Building $(APP_NAME) for local development...$(RESET)"
	@go build -o bin/$(APP_NAME) ./cmd/$(APP_NAME)
	@echo "$(GREEN)Local build completed$(RESET)"

test: ## Run tests
	@echo "$(BLUE)Running tests...$(RESET)"
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Tests completed$(RESET)"
	@echo "Coverage report generated: coverage.html"

test-short: ## Run short tests
	@echo "$(BLUE)Running short tests...$(RESET)"
	@go test -short -race ./...

benchmark: ## Run benchmarks
	@echo "$(BLUE)Running benchmarks...$(RESET)"
	@go test -bench=. -benchmem ./test/benchmarks/...
	@echo "$(GREEN)Benchmarks completed$(RESET)"

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(RESET)"
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@rm -rf temp/
	@rm -rf logs/
	@docker system prune -f
	@echo "$(GREEN)Clean completed$(RESET)"

run: build-local ## Run the application locally
	@echo "$(BLUE)Starting $(APP_NAME)...$(RESET)"
	@./bin/$(APP_NAME)

run-dev: ## Run with live reload (requires air)
	@echo "$(BLUE)Starting development server with live reload...$(RESET)"
	@air -c .air.toml

dev: ## Start development environment
	@echo "$(BLUE)Starting development environment...$(RESET)"
	@docker-compose -f docker-compose.yml up -d postgres redis minio
	@echo "$(GREEN)Development environment started$(RESET)"
	@echo "PostgreSQL: localhost:5432"
	@echo "Redis: localhost:6379"
	@echo "MinIO: localhost:9000 (Console: localhost:9001)"

dev-stop: ## Stop development environment
	@echo "$(YELLOW)Stopping development environment...$(RESET)"
	@docker-compose down
	@echo "$(GREEN)Development environment stopped$(RESET)"

deps: ## Install dependencies
	@echo "$(BLUE)Installing dependencies...$(RESET)"
	@go mod download
	@go mod tidy
	@echo "$(GREEN)Dependencies installed$(RESET)"

deps-update: ## Update dependencies
	@echo "$(BLUE)Updating dependencies...$(RESET)"
	@go get -u ./...
	@go mod tidy
	@echo "$(GREEN)Dependencies updated$(RESET)"

lint: ## Run linters
	@echo "$(BLUE)Running linters...$(RESET)"
	@golangci-lint run ./...
	@echo "$(GREEN)Linting completed$(RESET)"

format: ## Format code
	@echo "$(BLUE)Formatting code...$(RESET)"
	@gofmt -s -w .
	@goimports -w .
	@echo "$(GREEN)Code formatted$(RESET)"

vet: ## Run go vet
	@echo "$(BLUE)Running go vet...$(RESET)"
	@go vet ./...
	@echo "$(GREEN)Vet completed$(RESET)"

security: ## Run security checks
	@echo "$(BLUE)Running security checks...$(RESET)"
	@gosec ./...
	@echo "$(GREEN)Security checks completed$(RESET)"

audit: ## Run dependency audit
	@echo "$(BLUE)Running dependency audit...$(RESET)"
	@go list -json -deps ./... | nancy sleuth
	@echo "$(GREEN)Audit completed$(RESET)"

docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(RESET)"
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "$(GREEN)Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(RESET)"

docker-build-no-cache: ## Build Docker image without cache
	@echo "$(BLUE)Building Docker image without cache...$(RESET)"
	@docker build --no-cache -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "$(GREEN)Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(RESET)"

docker-run: ## Run Docker container
	@echo "$(BLUE)Running Docker container...$(RESET)"
	@docker run -p 8080:8080 -p 9090:9090 $(DOCKER_IMAGE):$(DOCKER_TAG)

docker-compose-up: ## Start services with docker-compose
	@echo "$(BLUE)Starting services with docker-compose...$(RESET)"
	@docker-compose up -d
	@echo "$(GREEN)Services started$(RESET)"

docker-compose-down: ## Stop services with docker-compose
	@echo "$(YELLOW)Stopping services with docker-compose...$(RESET)"
	@docker-compose down
	@echo "$(GREEN)Services stopped$(RESET)"

docker-compose-logs: ## View docker-compose logs
	@docker-compose logs -f

docker-push: docker-build ## Push Docker image to registry
	@echo "$(BLUE)Pushing Docker image to registry...$(RESET)"
	@docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker push $(REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "$(GREEN)Docker image pushed$(RESET)"

migrate-up: ## Run database migrations up
	@echo "$(BLUE)Running database migrations up...$(RESET)"
	@migrate -path migrations -database "postgres://postgres:password@localhost:5432/file_processing?sslmode=disable" up
	@echo "$(GREEN)Migrations completed$(RESET)"

migrate-down: ## Run database migrations down
	@echo "$(YELLOW)Running database migrations down...$(RESET)"
	@migrate -path migrations -database "postgres://postgres:password@localhost:5432/file_processing?sslmode=disable" down
	@echo "$(GREEN)Migrations rolled back$(RESET)"

migrate-create: ## Create new migration (usage: make migrate-create NAME=migration_name)
	@echo "$(BLUE)Creating new migration: $(NAME)...$(RESET)"
	@migrate create -ext sql -dir migrations $(NAME)
	@echo "$(GREEN)Migration created$(RESET)"

generate: ## Generate code (mocks, etc.)
	@echo "$(BLUE)Generating code...$(RESET)"
	@go generate ./...
	@echo "$(GREEN)Code generation completed$(RESET)"

mock: ## Generate mocks
	@echo "$(BLUE)Generating mocks...$(RESET)"
	@mockgen -source=internal/storage/interface.go -destination=test/mocks/storage_mock.go
	@mockgen -source=internal/processors/interface.go -destination=test/mocks/processor_mock.go
	@echo "$(GREEN)Mocks generated$(RESET)"

docs: ## Generate documentation
	@echo "$(BLUE)Generating documentation...$(RESET)"
	@godoc -http=:6060
	@echo "$(GREEN)Documentation server started at http://localhost:6060$(RESET)"

load-test: ## Run load tests
	@echo "$(BLUE)Running load tests...$(RESET)"
	@k6 run test/load/upload_test.js
	@echo "$(GREEN)Load tests completed$(RESET)"

performance-test: ## Run performance tests
	@echo "$(BLUE)Running performance tests...$(RESET)"
	@go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof ./test/benchmarks/...
	@echo "$(GREEN)Performance tests completed$(RESET)"
	@echo "CPU Profile: cpu.prof"
	@echo "Memory Profile: mem.prof"

stress-test: ## Run stress tests
	@echo "$(BLUE)Running stress tests...$(RESET)"
	@go test -stress -timeout=10m ./test/stress/...
	@echo "$(GREEN)Stress tests completed$(RESET)"

integration-test: ## Run integration tests
	@echo "$(BLUE)Running integration tests...$(RESET)"
	@docker-compose -f docker-compose.test.yml up -d
	@sleep 30  # Wait for services to be ready
	@go test -tags=integration ./test/integration/...
	@docker-compose -f docker-compose.test.yml down
	@echo "$(GREEN)Integration tests completed$(RESET)"

deploy-dev: ## Deploy to development environment
	@echo "$(BLUE)Deploying to development environment...$(RESET)"
	@kubectl apply -f deployments/k8s/namespace.yaml
	@kubectl apply -f deployments/k8s/configmap.yaml
	@kubectl apply -f deployments/k8s/deployment.yaml
	@kubectl apply -f deployments/k8s/service.yaml
	@echo "$(GREEN)Deployed to development$(RESET)"

deploy-prod: ## Deploy to production environment
	@echo "$(BLUE)Deploying to production environment...$(RESET)"
	@kubectl apply -f deployments/k8s/production/
	@echo "$(GREEN)Deployed to production$(RESET)"

monitoring: ## Start monitoring stack
	@echo "$(BLUE)Starting monitoring stack...$(RESET)"
	@docker-compose --profile monitoring up -d
	@echo "$(GREEN)Monitoring stack started$(RESET)"
	@echo "Prometheus: http://localhost:9091"
	@echo "Grafana: http://localhost:3000 (admin/admin)"

security-scan: ## Run security scanning
	@echo "$(BLUE)Running security scan...$(RESET)"
	@docker-compose --profile security up -d
	@echo "$(GREEN)Security services started$(RESET)"

init: deps ## Initialize project for development
	@echo "$(BLUE)Initializing project...$(RESET)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/cosmtrek/air@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/tools/cmd/godoc@latest
	@go install github.com/golang/mock/mockgen@latest
	@mkdir -p bin temp logs
	@echo "$(GREEN)Project initialized$(RESET)"

check: lint vet test ## Run all checks (lint, vet, test)
	@echo "$(GREEN)All checks passed$(RESET)"

ci: deps check build ## Run CI pipeline
	@echo "$(GREEN)CI pipeline completed$(RESET)"

release: ## Create a new release
	@echo "$(BLUE)Creating release...$(RESET)"
	@./scripts/release.sh
	@echo "$(GREEN)Release created$(RESET)"

health-check: ## Check service health
	@echo "$(BLUE)Checking service health...$(RESET)"
	@curl -f http://localhost:8080/health || (echo "$(RED)Health check failed$(RESET)" && exit 1)
	@curl -f http://localhost:8080/ready || (echo "$(RED)Readiness check failed$(RESET)" && exit 1)
	@echo "$(GREEN)Service is healthy$(RESET)"

logs: ## View service logs
	@docker-compose logs -f $(APP_NAME)

shell: ## Open shell in running container
	@docker-compose exec $(APP_NAME) sh

backup: ## Backup database
	@echo "$(BLUE)Creating database backup...$(RESET)"
	@docker-compose exec postgres pg_dump -U postgres file_processing > backup_$(shell date +%Y%m%d_%H%M%S).sql
	@echo "$(GREEN)Backup created$(RESET)"

restore: ## Restore database (usage: make restore BACKUP=backup_file.sql)
	@echo "$(BLUE)Restoring database from $(BACKUP)...$(RESET)"
	@docker-compose exec -T postgres psql -U postgres file_processing < $(BACKUP)
	@echo "$(GREEN)Database restored$(RESET)"

setup-git-hooks: ## Setup git hooks
	@echo "$(BLUE)Setting up git hooks...$(RESET)"
	@cp scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "$(GREEN)Git hooks setup completed$(RESET)"