#!/bin/bash

# PyAirtable Enhanced Monitoring Stack Deployment Script
# Deploys Prometheus, Grafana, and all monitoring components

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="docker-compose.monitoring.yml"
NETWORK_NAME="go-services_pyairtable-network"
MONITORING_NETWORK="pyairtable-monitoring"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  PyAirtable Monitoring Stack Setup${NC}"
echo -e "${BLUE}========================================${NC}"

# Function to print colored output
print_step() {
    echo -e "${GREEN}[STEP]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null && ! command -v docker &> /dev/null; then
    print_error "Docker and docker-compose are required but not installed."
    exit 1
fi

# Use docker compose (newer) or docker-compose (legacy)
if docker compose version &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

print_info "Using: $DOCKER_COMPOSE"

# Check if monitoring compose file exists
if [ ! -f "$COMPOSE_FILE" ]; then
    print_error "Monitoring compose file not found: $COMPOSE_FILE"
    exit 1
fi

# Create monitoring directories if they don't exist
print_step "Creating monitoring directories..."
mkdir -p monitoring/prometheus/rules
mkdir -p monitoring/grafana/{provisioning/{datasources,dashboards},dashboards/{infrastructure,services}}
mkdir -p monitoring/alertmanager
mkdir -p monitoring/blackbox

# Check if main PyAirtable network exists
print_step "Checking PyAirtable network..."
if ! docker network ls | grep -q "$NETWORK_NAME"; then
    print_warning "Main PyAirtable network '$NETWORK_NAME' not found."
    print_info "Creating external network..."
    docker network create "$NETWORK_NAME" || true
fi

# Function to wait for service to be healthy
wait_for_service() {
    local service_name=$1
    local max_attempts=${2:-30}
    local attempt=1

    print_step "Waiting for $service_name to be healthy..."
    
    while [ $attempt -le $max_attempts ]; do
        if $DOCKER_COMPOSE -f $COMPOSE_FILE ps | grep -q "$service_name.*healthy"; then
            print_info "$service_name is healthy!"
            return 0
        fi
        
        if [ $attempt -eq 1 ]; then
            print_info "Waiting for $service_name to start..."
        fi
        
        sleep 2
        attempt=$((attempt + 1))
    done
    
    print_warning "$service_name did not become healthy within expected time"
    return 1
}

# Function to check service URL
check_service_url() {
    local url=$1
    local service_name=$2
    local max_attempts=${3:-10}
    local attempt=1

    print_step "Checking $service_name at $url..."
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s -f "$url" > /dev/null 2>&1; then
            print_info "$service_name is responding at $url"
            return 0
        fi
        
        sleep 3
        attempt=$((attempt + 1))
    done
    
    print_warning "$service_name is not responding at $url"
    return 1
}

# Stop any existing monitoring stack
print_step "Stopping any existing monitoring stack..."
$DOCKER_COMPOSE -f $COMPOSE_FILE down --remove-orphans 2>/dev/null || true

# Pull latest images
print_step "Pulling latest monitoring images..."
$DOCKER_COMPOSE -f $COMPOSE_FILE pull

# Start the monitoring stack
print_step "Starting monitoring stack..."
$DOCKER_COMPOSE -f $COMPOSE_FILE up -d

# Wait for core services to be healthy
print_step "Waiting for services to start..."
sleep 10

# Check Prometheus
wait_for_service "prometheus" 20
check_service_url "http://localhost:9090/-/healthy" "Prometheus"

# Check Grafana
wait_for_service "grafana" 30
check_service_url "http://localhost:3001/api/health" "Grafana"

# Check Node Exporter
wait_for_service "node-exporter" 15
check_service_url "http://localhost:9100/metrics" "Node Exporter"

# Check other exporters
check_service_url "http://localhost:9187/metrics" "PostgreSQL Exporter"
check_service_url "http://localhost:9121/metrics" "Redis Exporter"
check_service_url "http://localhost:8080/metrics" "cAdvisor"
check_service_url "http://localhost:9115/metrics" "Blackbox Exporter"
check_service_url "http://localhost:9093/-/healthy" "Alertmanager"

# Display service status
print_step "Checking monitoring stack status..."
echo
$DOCKER_COMPOSE -f $COMPOSE_FILE ps

echo
print_step "Monitoring stack deployment completed!"
echo
echo -e "${GREEN}Access URLs:${NC}"
echo -e "  Grafana:     ${BLUE}http://localhost:3001${NC} (admin/admin)"
echo -e "  Prometheus:  ${BLUE}http://localhost:9090${NC}"
echo -e "  Alertmanager: ${BLUE}http://localhost:9093${NC}"
echo -e "  cAdvisor:    ${BLUE}http://localhost:8080${NC}"
echo
echo -e "${GREEN}Exporters:${NC}"
echo -e "  Node Exporter:       ${BLUE}http://localhost:9100/metrics${NC}"
echo -e "  PostgreSQL Exporter: ${BLUE}http://localhost:9187/metrics${NC}"
echo -e "  Redis Exporter:      ${BLUE}http://localhost:9121/metrics${NC}"
echo -e "  Blackbox Exporter:   ${BLUE}http://localhost:9115/metrics${NC}"
echo
echo -e "${YELLOW}Next Steps:${NC}"
echo "1. Access Grafana at http://localhost:3001 (admin/admin)"
echo "2. Import dashboards or use the pre-configured ones"
echo "3. Configure alert notification channels in Alertmanager"
echo "4. Review Prometheus targets at http://localhost:9090/targets"
echo "5. Check alerts at http://localhost:9090/alerts"
echo
echo -e "${GREEN}To stop the monitoring stack:${NC}"
echo "  $DOCKER_COMPOSE -f $COMPOSE_FILE down"
echo
echo -e "${GREEN}To view logs:${NC}"
echo "  $DOCKER_COMPOSE -f $COMPOSE_FILE logs -f [service-name]"
echo

# Check if main services are running for metrics collection
print_step "Checking if main PyAirtable services are running for metrics collection..."
if ! docker network ls | grep -q "$NETWORK_NAME"; then
    print_warning "Main PyAirtable services may not be running."
    print_info "To get full metrics, start them with:"
    print_info "  docker-compose up -d  # for main services"
    print_info "  or"
    print_info "  docker-compose -f docker-compose.phase1.yml up -d  # for phase 1 services"
fi

echo -e "${GREEN}Monitoring deployment completed successfully!${NC}"