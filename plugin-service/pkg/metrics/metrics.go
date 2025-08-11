package metrics

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/expfmt"
)

// Collector provides metrics collection for the plugin service
type Collector struct {
	registry  *prometheus.Registry
	serviceName string
	
	// Plugin metrics
	pluginsTotal           prometheus.Gauge
	installationsTotal     prometheus.Gauge
	executionsTotal        *prometheus.CounterVec
	executionDuration      *prometheus.HistogramVec
	executionErrors        *prometheus.CounterVec
	
	// Runtime metrics
	runtimeInstancesActive prometheus.Gauge
	runtimeMemoryUsage     *prometheus.GaugeVec
	runtimeCPUUsage        *prometheus.GaugeVec
	
	// Security metrics
	securityValidations    *prometheus.CounterVec
	securityViolations     *prometheus.CounterVec
	quarantinedPlugins     prometheus.Gauge
	
	// Registry metrics
	registrySyncDuration   *prometheus.HistogramVec
	registrySyncErrors     *prometheus.CounterVec
	registryPluginCount    *prometheus.GaugeVec
	
	// HTTP metrics
	httpRequestsTotal      *prometheus.CounterVec
	httpRequestDuration    *prometheus.HistogramVec
	httpResponseSize       *prometheus.HistogramVec
	
	// Resource metrics
	resourceMemoryUsage    prometheus.Gauge
	resourceCPUUsage       prometheus.Gauge
	resourceDiskUsage      prometheus.Gauge
	
	mu sync.RWMutex
}

// NewCollector creates a new metrics collector
func NewCollector(serviceName string) *Collector {
	registry := prometheus.NewRegistry()
	
	c := &Collector{
		registry:    registry,
		serviceName: serviceName,
	}
	
	c.initMetrics()
	c.registerMetrics()
	
	return c
}

// initMetrics initializes all Prometheus metrics
func (c *Collector) initMetrics() {
	// Plugin metrics
	c.pluginsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_plugins_total",
		Help: "Total number of plugins",
	})
	
	c.installationsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_installations_total",
		Help: "Total number of plugin installations",
	})
	
	c.executionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plugin_service_executions_total",
		Help: "Total number of plugin executions",
	}, []string{"plugin_id", "status", "function"})
	
	c.executionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "plugin_service_execution_duration_seconds",
		Help:    "Duration of plugin executions",
		Buckets: prometheus.DefBuckets,
	}, []string{"plugin_id", "function"})
	
	c.executionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plugin_service_execution_errors_total",
		Help: "Total number of plugin execution errors",
	}, []string{"plugin_id", "error_type"})
	
	// Runtime metrics
	c.runtimeInstancesActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_runtime_instances_active",
		Help: "Number of active plugin runtime instances",
	})
	
	c.runtimeMemoryUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "plugin_service_runtime_memory_usage_bytes",
		Help: "Memory usage by plugin runtime instances",
	}, []string{"plugin_id", "instance_id"})
	
	c.runtimeCPUUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "plugin_service_runtime_cpu_usage_seconds",
		Help: "CPU usage by plugin runtime instances",
	}, []string{"plugin_id", "instance_id"})
	
	// Security metrics
	c.securityValidations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plugin_service_security_validations_total",
		Help: "Total number of security validations performed",
	}, []string{"result"})
	
	c.securityViolations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plugin_service_security_violations_total",
		Help: "Total number of security violations detected",
	}, []string{"violation_type", "severity"})
	
	c.quarantinedPlugins = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_quarantined_plugins_total",
		Help: "Number of plugins currently quarantined",
	})
	
	// Registry metrics
	c.registrySyncDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "plugin_service_registry_sync_duration_seconds",
		Help:    "Duration of registry synchronization operations",
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 25, 60},
	}, []string{"registry_name", "result"})
	
	c.registrySyncErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plugin_service_registry_sync_errors_total",
		Help: "Total number of registry synchronization errors",
	}, []string{"registry_name", "error_type"})
	
	c.registryPluginCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "plugin_service_registry_plugin_count",
		Help: "Number of plugins available in each registry",
	}, []string{"registry_name"})
	
	// HTTP metrics
	c.httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "plugin_service_http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status_code"})
	
	c.httpRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "plugin_service_http_request_duration_seconds",
		Help:    "Duration of HTTP requests",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
	
	c.httpResponseSize = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "plugin_service_http_response_size_bytes",
		Help:    "Size of HTTP responses",
		Buckets: []float64{100, 1000, 10000, 100000, 1000000},
	}, []string{"method", "path"})
	
	// Resource metrics
	c.resourceMemoryUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_resource_memory_usage_bytes",
		Help: "Current memory usage of the service",
	})
	
	c.resourceCPUUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_resource_cpu_usage_percent",
		Help: "Current CPU usage of the service",
	})
	
	c.resourceDiskUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "plugin_service_resource_disk_usage_bytes",
		Help: "Current disk usage of the service",
	})
}

// registerMetrics registers all metrics with the Prometheus registry
func (c *Collector) registerMetrics() {
	// Plugin metrics
	c.registry.MustRegister(c.pluginsTotal)
	c.registry.MustRegister(c.installationsTotal)
	c.registry.MustRegister(c.executionsTotal)
	c.registry.MustRegister(c.executionDuration)
	c.registry.MustRegister(c.executionErrors)
	
	// Runtime metrics
	c.registry.MustRegister(c.runtimeInstancesActive)
	c.registry.MustRegister(c.runtimeMemoryUsage)
	c.registry.MustRegister(c.runtimeCPUUsage)
	
	// Security metrics
	c.registry.MustRegister(c.securityValidations)
	c.registry.MustRegister(c.securityViolations)
	c.registry.MustRegister(c.quarantinedPlugins)
	
	// Registry metrics
	c.registry.MustRegister(c.registrySyncDuration)
	c.registry.MustRegister(c.registrySyncErrors)
	c.registry.MustRegister(c.registryPluginCount)
	
	// HTTP metrics
	c.registry.MustRegister(c.httpRequestsTotal)
	c.registry.MustRegister(c.httpRequestDuration)
	c.registry.MustRegister(c.httpResponseSize)
	
	// Resource metrics
	c.registry.MustRegister(c.resourceMemoryUsage)
	c.registry.MustRegister(c.resourceCPUUsage)
	c.registry.MustRegister(c.resourceDiskUsage)
}

// Plugin metrics methods
func (c *Collector) SetPluginCount(count float64) {
	c.pluginsTotal.Set(count)
}

func (c *Collector) SetInstallationCount(count float64) {
	c.installationsTotal.Set(count)
}

func (c *Collector) IncPluginExecution(pluginID, status, function string) {
	c.executionsTotal.WithLabelValues(pluginID, status, function).Inc()
}

func (c *Collector) ObserveExecutionDuration(pluginID, function string, duration time.Duration) {
	c.executionDuration.WithLabelValues(pluginID, function).Observe(duration.Seconds())
}

func (c *Collector) IncExecutionError(pluginID, errorType string) {
	c.executionErrors.WithLabelValues(pluginID, errorType).Inc()
}

// Runtime metrics methods
func (c *Collector) SetActiveInstances(count float64) {
	c.runtimeInstancesActive.Set(count)
}

func (c *Collector) SetRuntimeMemoryUsage(pluginID, instanceID string, bytes float64) {
	c.runtimeMemoryUsage.WithLabelValues(pluginID, instanceID).Set(bytes)
}

func (c *Collector) SetRuntimeCPUUsage(pluginID, instanceID string, seconds float64) {
	c.runtimeCPUUsage.WithLabelValues(pluginID, instanceID).Set(seconds)
}

// Security metrics methods
func (c *Collector) IncSecurityValidation(result string) {
	c.securityValidations.WithLabelValues(result).Inc()
}

func (c *Collector) IncSecurityViolation(violationType, severity string) {
	c.securityViolations.WithLabelValues(violationType, severity).Inc()
}

func (c *Collector) SetQuarantinedPlugins(count float64) {
	c.quarantinedPlugins.Set(count)
}

// Registry metrics methods
func (c *Collector) ObserveRegistrySync(registryName, result string, duration time.Duration) {
	c.registrySyncDuration.WithLabelValues(registryName, result).Observe(duration.Seconds())
}

func (c *Collector) IncRegistrySyncError(registryName, errorType string) {
	c.registrySyncErrors.WithLabelValues(registryName, errorType).Inc()
}

func (c *Collector) SetRegistryPluginCount(registryName string, count float64) {
	c.registryPluginCount.WithLabelValues(registryName).Set(count)
}

// HTTP metrics methods
func (c *Collector) IncHTTPRequest(method, path, statusCode string) {
	c.httpRequestsTotal.WithLabelValues(method, path, statusCode).Inc()
}

func (c *Collector) ObserveHTTPDuration(method, path string, duration time.Duration) {
	c.httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func (c *Collector) ObserveHTTPResponseSize(method, path string, size float64) {
	c.httpResponseSize.WithLabelValues(method, path).Observe(size)
}

// Resource metrics methods
func (c *Collector) SetMemoryUsage(bytes float64) {
	c.resourceMemoryUsage.Set(bytes)
}

func (c *Collector) SetCPUUsage(percent float64) {
	c.resourceCPUUsage.Set(percent)
}

func (c *Collector) SetDiskUsage(bytes float64) {
	c.resourceDiskUsage.Set(bytes)
}

// Gather returns the metrics in Prometheus format
func (c *Collector) Gather() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	metricFamilies, err := c.registry.Gather()
	if err != nil {
		return ""
	}
	
	var buf strings.Builder
	for _, mf := range metricFamilies {
		if _, err := expfmt.MetricFamilyToText(&buf, mf); err != nil {
			continue
		}
	}
	
	return buf.String()
}

// GetRegistry returns the Prometheus registry
func (c *Collector) GetRegistry() *prometheus.Registry {
	return c.registry
}

// HTTPMiddleware creates a middleware for collecting HTTP metrics
func (c *Collector) HTTPMiddleware() func(next func()) func() {
	return func(next func()) func() {
		return func() {
			start := time.Now()
			
			// This would be integrated with the actual HTTP framework
			// For now, it's a placeholder
			
			next()
			
			duration := time.Since(start)
			
			// These would be extracted from the actual HTTP context
			method := "GET"
			path := "/api/v1/plugins"
			statusCode := "200"
			responseSize := 1024.0
			
			c.IncHTTPRequest(method, path, statusCode)
			c.ObserveHTTPDuration(method, path, duration)
			c.ObserveHTTPResponseSize(method, path, responseSize)
		}
	}
}

// StartResourceMonitoring starts monitoring system resources
func (c *Collector) StartResourceMonitoring() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			c.updateResourceMetrics()
		}
	}()
}

// updateResourceMetrics updates system resource metrics
func (c *Collector) updateResourceMetrics() {
	// In a real implementation, these would be actual system metrics
	// For now, we'll use placeholder values
	
	// Memory usage (in bytes)
	c.SetMemoryUsage(100 * 1024 * 1024) // 100MB
	
	// CPU usage (percentage)
	c.SetCPUUsage(25.5)
	
	// Disk usage (in bytes)
	c.SetDiskUsage(1 * 1024 * 1024 * 1024) // 1GB
}

// Custom metric types for specific use cases

// Timer is a helper for timing operations
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// Elapsed returns the elapsed time since the timer was created
func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}

// ObserveAndReset observes the elapsed time and resets the timer
func (t *Timer) ObserveAndReset(observer prometheus.Observer) {
	elapsed := t.Elapsed()
	observer.Observe(elapsed.Seconds())
	t.start = time.Now()
}

// Counter wrapper for easier use
type Counter struct {
	counter prometheus.Counter
}

// NewCounter creates a new counter
func NewCounter(name, help string) *Counter {
	counter := promauto.NewCounter(prometheus.CounterOpts{
		Name: name,
		Help: help,
	})
	
	return &Counter{counter: counter}
}

// Inc increments the counter
func (c *Counter) Inc() {
	c.counter.Inc()
}

// Add adds a value to the counter
func (c *Counter) Add(value float64) {
	c.counter.Add(value)
}

// Gauge wrapper for easier use
type Gauge struct {
	gauge prometheus.Gauge
}

// NewGauge creates a new gauge
func NewGauge(name, help string) *Gauge {
	gauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	})
	
	return &Gauge{gauge: gauge}
}

// Set sets the gauge value
func (g *Gauge) Set(value float64) {
	g.gauge.Set(value)
}

// Inc increments the gauge
func (g *Gauge) Inc() {
	g.gauge.Inc()
}

// Dec decrements the gauge
func (g *Gauge) Dec() {
	g.gauge.Dec()
}

// Add adds a value to the gauge
func (g *Gauge) Add(value float64) {
	g.gauge.Add(value)
}

// Sub subtracts a value from the gauge
func (g *Gauge) Sub(value float64) {
	g.gauge.Sub(value)
}