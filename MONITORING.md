# PyAirtable Enhanced Monitoring Stack

This repository includes a comprehensive monitoring solution for the PyAirtable microservices architecture, providing observability, alerting, and performance insights.

## Overview

The monitoring stack includes:

- **Prometheus**: Metrics collection and storage with advanced configuration
- **Grafana**: Visualization dashboards with pre-built PyAirtable-specific views
- **Alertmanager**: Alert management and notification routing
- **Node Exporter**: System-level metrics (CPU, memory, disk, network)
- **PostgreSQL Exporter**: Database performance and health metrics
- **Redis Exporter**: Cache performance and usage metrics
- **cAdvisor**: Container-level metrics and resource usage
- **Blackbox Exporter**: External service health checks and uptime monitoring

## Quick Start

### 1. Deploy the Monitoring Stack

```bash
# Make the deployment script executable (if not already)
chmod +x deploy-monitoring.sh

# Deploy the complete monitoring stack
./deploy-monitoring.sh
```

### 2. Access the Dashboards

After deployment, access the monitoring interfaces:

- **Grafana**: http://localhost:3001 (admin/admin)
- **Prometheus**: http://localhost:9090
- **Alertmanager**: http://localhost:9093
- **cAdvisor**: http://localhost:8080

### 3. View Service Metrics

Individual exporter metrics are available at:

- **Node Exporter**: http://localhost:9100/metrics
- **PostgreSQL Exporter**: http://localhost:9187/metrics
- **Redis Exporter**: http://localhost:9121/metrics
- **Blackbox Exporter**: http://localhost:9115/metrics

## Architecture

### Services Monitored

The monitoring stack automatically discovers and monitors:

**Go Microservices:**
- API Gateway (port 8080) - Main entry point with proxy metrics
- Auth Service (port 8001) - Authentication and JWT metrics
- User Service (port 8002) - User management metrics
- Permission Service (port 8085) - RBAC and authorization metrics
- Platform Services (port 8081) - Consolidated platform metrics
- Automation Services (port 8082) - File processing and workflow metrics

**Python Services:**
- Airtable Gateway (port 8002) - Integration metrics
- LLM Orchestrator (port 8003) - AI/ML processing metrics
- MCP Server (port 8001) - Protocol server metrics

**Infrastructure:**
- PostgreSQL database
- Redis cache
- Docker containers
- System resources

### Metrics Collected

#### Application Metrics
- **Request Rate**: Requests per second by service and endpoint
- **Response Time**: P50, P95, P99 latency percentiles
- **Error Rate**: HTTP error percentages by status code
- **Database Connections**: Active connection pools
- **Cache Performance**: Hit/miss ratios and response times
- **Business Metrics**: Custom metrics like authentication attempts, file uploads

#### Infrastructure Metrics
- **CPU Usage**: Per-core and aggregate usage
- **Memory Usage**: Available, used, and cached memory
- **Disk Usage**: Filesystem usage and I/O statistics
- **Network**: Bandwidth, packets, and connection statistics
- **Container Metrics**: Resource usage per container

#### Database Metrics
- **Connection Pool**: Active connections and pool exhaustion
- **Query Performance**: Slow queries and execution times
- **Database Size**: Table sizes and growth trends
- **Lock Analysis**: Deadlocks and blocking queries

#### Cache Metrics
- **Hit Ratio**: Cache effectiveness
- **Memory Usage**: Redis memory consumption
- **Key Statistics**: Key count and expiration
- **Connection Stats**: Client connections and commands

## Dashboards

### Pre-built Dashboards

1. **PyAirtable Services Overview** (`pyairtable-overview`)
   - Service health status
   - Request rates and error rates
   - Response time percentiles
   - Database and cache metrics

2. **API Performance Dashboard** (`api-performance`)
   - Detailed endpoint performance
   - Request rate by endpoint
   - HTTP status code breakdown
   - Performance summary table

3. **System Metrics** (`system-metrics`)
   - CPU and memory usage
   - Disk usage by mount point
   - System load averages
   - Network statistics

4. **Infrastructure Overview**
   - Container resource usage
   - Database performance
   - Cache statistics
   - Service dependencies

### Dashboard Features

- **Real-time Updates**: 30-second refresh intervals
- **Time Range Selection**: Flexible time periods (15m, 1h, 6h, 24h, 7d)
- **Service Filtering**: Filter metrics by specific services
- **Alert Annotations**: Visual indicators of alert states
- **Drill-down Capabilities**: Click through to detailed views

## Alerting

### Alert Categories

**Critical Alerts** (Immediate Response Required):
- Service Down
- High Error Rate (>5%)
- Database Connection Pool Exhausted
- Very High Response Time (>5s)

**Warning Alerts** (Attention Required):
- High Response Time (>1s)
- High CPU/Memory Usage (>80%)
- Low Cache Hit Ratio (<80%)
- Database Slow Queries

**Infrastructure Alerts**:
- Low Disk Space (>85%)
- High System Load
- Container Restarts
- Network Issues

### Alert Routing

Alerts are routed based on:
- **Severity**: Critical vs Warning
- **Category**: Service, Infrastructure, Database, Security
- **Environment**: Development vs Production
- **Service**: Specific microservice alerts

### Notification Channels

Configure notification channels in `monitoring/alertmanager/alertmanager.yml`:

```yaml
receivers:
  - name: 'critical-alerts'
    email_configs:
      - to: 'alerts@pyairtable.com'
    slack_configs:
      - api_url: 'YOUR_SLACK_WEBHOOK'
        channel: '#alerts-critical'
```

## Configuration

### Prometheus Configuration

Main configuration: `monitoring/prometheus/prometheus.yml`

**Key Features:**
- 15-second scrape intervals for Go services
- 30-second intervals for infrastructure
- Service discovery and labeling
- Custom relabeling rules
- Alert rule integration

### Custom Metrics

To add custom metrics to your Go services:

```go
// Example: Custom business metric
var userRegistrations = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "user_registrations_total",
        Help: "Total number of user registrations",
    },
    []string{"source", "plan_type"},
)

// Register the metric
prometheus.MustRegister(userRegistrations)

// Use in your code
userRegistrations.WithLabelValues("web", "premium").Inc()
```

### Adding New Services

To monitor a new service:

1. Ensure the service exposes `/metrics` endpoint
2. Add to Prometheus configuration in `prometheus.yml`:

```yaml
- job_name: 'new-service'
  static_configs:
    - targets: ['new-service:8080']
  metrics_path: '/metrics'
  scrape_interval: 10s
```

3. Restart Prometheus: `docker-compose -f docker-compose.monitoring.yml restart prometheus`

## Troubleshooting

### Common Issues

**Prometheus Targets Down:**
```bash
# Check service is running
docker-compose ps

# Check service logs
docker-compose logs [service-name]

# Verify metrics endpoint
curl http://localhost:8080/metrics
```

**Grafana Dashboard Not Loading:**
```bash
# Check Grafana logs
docker-compose -f docker-compose.monitoring.yml logs grafana

# Verify datasource connection
# Go to Grafana > Configuration > Data Sources > Test
```

**High Memory Usage:**
```bash
# Check Prometheus retention settings
# Reduce retention time in prometheus.yml:
# --storage.tsdb.retention.time=15d
```

### Log Analysis

View logs for specific components:

```bash
# All monitoring services
docker-compose -f docker-compose.monitoring.yml logs -f

# Specific service
docker-compose -f docker-compose.monitoring.yml logs -f prometheus
docker-compose -f docker-compose.monitoring.yml logs -f grafana
```

### Performance Tuning

**Prometheus Optimization:**
- Adjust scrape intervals based on service criticality
- Use recording rules for complex queries
- Configure appropriate retention policies

**Grafana Optimization:**
- Use template variables for dynamic dashboards
- Implement query caching
- Optimize dashboard refresh intervals

## Maintenance

### Regular Tasks

1. **Dashboard Updates**: Keep dashboards updated with new metrics
2. **Alert Review**: Regularly review and tune alert thresholds
3. **Retention Management**: Monitor storage usage and adjust retention
4. **Performance Review**: Analyze dashboard performance and optimize queries

### Backup and Recovery

**Backup Grafana Dashboards:**
```bash
# Export dashboards via API
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:3001/api/dashboards/db/dashboard-name
```

**Backup Prometheus Data:**
```bash
# Create snapshot
curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot

# Backup volume
docker run --rm -v prometheus-data:/data -v $(pwd):/backup \
  alpine tar czf /backup/prometheus-backup.tar.gz /data
```

### Updates

**Update Monitoring Stack:**
```bash
# Pull latest images
docker-compose -f docker-compose.monitoring.yml pull

# Restart with new images
docker-compose -f docker-compose.monitoring.yml up -d
```

## Integration with CI/CD

### Automated Monitoring

Include monitoring deployment in CI/CD pipelines:

```yaml
# Example GitHub Actions workflow
- name: Deploy Monitoring
  run: |
    ./deploy-monitoring.sh
    # Wait for services to be healthy
    timeout 300 bash -c 'until curl -f http://localhost:9090/-/healthy; do sleep 5; done'
```

### Monitoring Deployments

Monitor deployment success through:
- Service health checks
- Error rate monitoring during deployments
- Performance regression detection
- Automated rollback triggers

## Advanced Configuration

### Custom Recording Rules

Add to `monitoring/prometheus/rules/`:

```yaml
groups:
  - name: pyairtable_sli
    rules:
      - record: pyairtable:service_availability
        expr: avg_over_time(up[5m])
      
      - record: pyairtable:error_budget
        expr: 1 - (rate(http_requests_total{status=~"5.."}[30d]) / rate(http_requests_total[30d]))
```

### Multi-Environment Setup

For production environments:
1. Update `prometheus.yml` with production service URLs
2. Configure secure authentication
3. Set up proper retention policies
4. Configure external alerting channels
5. Implement proper backup strategies

## Security Considerations

1. **Authentication**: Configure Grafana authentication (LDAP, OAuth, etc.)
2. **Network Security**: Restrict monitoring ports to internal networks
3. **Data Privacy**: Ensure metrics don't contain sensitive information
4. **Access Control**: Implement role-based access for dashboards
5. **Secrets Management**: Secure API keys and passwords

## Support and Resources

- **Prometheus Documentation**: https://prometheus.io/docs/
- **Grafana Documentation**: https://grafana.com/docs/
- **Go Metrics Best Practices**: https://prometheus.io/docs/guides/go-application/
- **Alert Manager Configuration**: https://prometheus.io/docs/alerting/latest/configuration/

For PyAirtable-specific monitoring questions, refer to the team documentation or create an issue in the repository.