package routing

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyairtable/api-gateway/internal/config"
	"go.uber.org/zap"
)

// ServiceInstance represents a single instance of a service
type ServiceInstance struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Weight         int       `json:"weight"`
	Healthy        int32     `json:"healthy"` // atomic, 1 for healthy, 0 for unhealthy
	LastCheck      time.Time `json:"last_check"`
	ResponseTime   int64     `json:"response_time_ms"` // atomic
	RequestCount   int64     `json:"request_count"`    // atomic
	ErrorCount     int64     `json:"error_count"`      // atomic
	HealthCheckURL string    `json:"health_check_url"`
	Timeout        time.Duration
	RetryCount     int
	RetryDelay     time.Duration
}

func (s *ServiceInstance) IsHealthy() bool {
	return atomic.LoadInt32(&s.Healthy) == 1
}

func (s *ServiceInstance) SetHealthy(healthy bool) {
	if healthy {
		atomic.StoreInt32(&s.Healthy, 1)
	} else {
		atomic.StoreInt32(&s.Healthy, 0)
	}
}

func (s *ServiceInstance) IncrementRequests() {
	atomic.AddInt64(&s.RequestCount, 1)
}

func (s *ServiceInstance) IncrementErrors() {
	atomic.AddInt64(&s.ErrorCount, 1)
}

func (s *ServiceInstance) UpdateResponseTime(duration time.Duration) {
	atomic.StoreInt64(&s.ResponseTime, duration.Milliseconds())
}

func (s *ServiceInstance) GetErrorRate() float64 {
	requests := atomic.LoadInt64(&s.RequestCount)
	if requests == 0 {
		return 0
	}
	errors := atomic.LoadInt64(&s.ErrorCount)
	return float64(errors) / float64(requests)
}

// ServiceRegistry manages service instances and their health
type ServiceRegistry struct {
	services      map[string][]*ServiceInstance
	mu            sync.RWMutex
	logger        *zap.Logger
	httpClient    *http.Client
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewServiceRegistry(logger *zap.Logger, checkInterval time.Duration) *ServiceRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ServiceRegistry{
		services:      make(map[string][]*ServiceInstance),
		logger:        logger,
		checkInterval: checkInterval,
		ctx:           ctx,
		cancel:        cancel,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

func (sr *ServiceRegistry) RegisterFromConfig(cfg *config.Config) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Clear existing services
	sr.services = make(map[string][]*ServiceInstance)

	// Register all services from config
	services := map[string]config.ServiceEndpoint{
		"auth":       cfg.Services.AuthService,
		"user":       cfg.Services.UserService,
		"workspace":  cfg.Services.WorkspaceService,
		"base":       cfg.Services.BaseService,
		"table":      cfg.Services.TableService,
		"view":       cfg.Services.ViewService,
		"record":     cfg.Services.RecordService,
		"field":      cfg.Services.FieldService,
		"formula":    cfg.Services.FormulaService,
		"llm":        cfg.Services.LLMOrchestrator,
		"file":       cfg.Services.FileProcessor,
		"airtable":   cfg.Services.AirtableGateway,
		"analytics":  cfg.Services.AnalyticsService,
		"workflow":   cfg.Services.WorkflowEngine,
	}

	for name, serviceConfig := range services {
		instance := &ServiceInstance{
			ID:             fmt.Sprintf("%s-1", name),
			Name:           name,
			URL:            serviceConfig.URL,
			Weight:         serviceConfig.Weight,
			Healthy:        1, // Assume healthy initially
			LastCheck:      time.Now(),
			HealthCheckURL: serviceConfig.URL + serviceConfig.HealthCheckPath,
			Timeout:        serviceConfig.Timeout,
			RetryCount:     serviceConfig.RetryCount,
			RetryDelay:     serviceConfig.RetryDelay,
		}

		sr.services[name] = []*ServiceInstance{instance}
		sr.logger.Info("Registered service", 
			zap.String("name", name),
			zap.String("url", serviceConfig.URL),
			zap.Int("weight", serviceConfig.Weight))
	}

	return nil
}

func (sr *ServiceRegistry) RegisterService(name string, instance *ServiceInstance) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if _, exists := sr.services[name]; !exists {
		sr.services[name] = make([]*ServiceInstance, 0)
	}

	sr.services[name] = append(sr.services[name], instance)
	sr.logger.Info("Service registered", 
		zap.String("service", name),
		zap.String("instance_id", instance.ID),
		zap.String("url", instance.URL))
}

func (sr *ServiceRegistry) UnregisterService(name, instanceID string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	instances, exists := sr.services[name]
	if !exists {
		return
	}

	for i, instance := range instances {
		if instance.ID == instanceID {
			sr.services[name] = append(instances[:i], instances[i+1:]...)
			sr.logger.Info("Service unregistered",
				zap.String("service", name),
				zap.String("instance_id", instanceID))
			break
		}
	}

	// Remove service if no instances left
	if len(sr.services[name]) == 0 {
		delete(sr.services, name)
	}
}

func (sr *ServiceRegistry) GetHealthyInstances(serviceName string) []*ServiceInstance {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	instances, exists := sr.services[serviceName]
	if !exists {
		return nil
	}

	var healthy []*ServiceInstance
	for _, instance := range instances {
		if instance.IsHealthy() {
			healthy = append(healthy, instance)
		}
	}

	return healthy
}

func (sr *ServiceRegistry) GetAllInstances(serviceName string) []*ServiceInstance {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	instances, exists := sr.services[serviceName]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make([]*ServiceInstance, len(instances))
	copy(result, instances)
	return result
}

func (sr *ServiceRegistry) GetServiceNames() []string {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	names := make([]string, 0, len(sr.services))
	for name := range sr.services {
		names = append(names, name)
	}
	return names
}

func (sr *ServiceRegistry) StartHealthChecks() {
	go sr.healthCheckLoop()
}

func (sr *ServiceRegistry) Stop() {
	sr.cancel()
}

func (sr *ServiceRegistry) healthCheckLoop() {
	ticker := time.NewTicker(sr.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sr.ctx.Done():
			return
		case <-ticker.C:
			sr.performHealthChecks()
		}
	}
}

func (sr *ServiceRegistry) performHealthChecks() {
	sr.mu.RLock()
	
	// Collect all instances that need checking
	var allInstances []*ServiceInstance
	for _, instances := range sr.services {
		allInstances = append(allInstances, instances...)
	}
	
	sr.mu.RUnlock()

	// Perform health checks in parallel
	const maxConcurrentChecks = 10
	semaphore := make(chan struct{}, maxConcurrentChecks)
	
	var wg sync.WaitGroup
	for _, instance := range allInstances {
		wg.Add(1)
		go func(inst *ServiceInstance) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire
			defer func() { <-semaphore }() // Release
			
			sr.checkInstanceHealth(inst)
		}(instance)
	}
	
	wg.Wait()
}

func (sr *ServiceRegistry) checkInstanceHealth(instance *ServiceInstance) {
	start := time.Now()
	
	req, err := http.NewRequestWithContext(sr.ctx, "GET", instance.HealthCheckURL, nil)
	if err != nil {
		sr.logger.Warn("Failed to create health check request",
			zap.String("service", instance.Name),
			zap.String("url", instance.HealthCheckURL),
			zap.Error(err))
		instance.SetHealthy(false)
		instance.IncrementErrors()
		return
	}

	resp, err := sr.httpClient.Do(req)
	duration := time.Since(start)
	
	instance.LastCheck = time.Now()
	instance.UpdateResponseTime(duration)

	if err != nil {
		sr.logger.Debug("Health check failed",
			zap.String("service", instance.Name),
			zap.String("url", instance.HealthCheckURL),
			zap.Error(err))
		instance.SetHealthy(false)
		instance.IncrementErrors()
		return
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	instance.SetHealthy(healthy)

	if !healthy {
		instance.IncrementErrors()
		sr.logger.Warn("Service health check failed",
			zap.String("service", instance.Name),
			zap.String("url", instance.HealthCheckURL),
			zap.Int("status_code", resp.StatusCode),
			zap.Duration("response_time", duration))
	} else {
		sr.logger.Debug("Service health check passed",
			zap.String("service", instance.Name),
			zap.Duration("response_time", duration))
	}
}

func (sr *ServiceRegistry) GetStats() map[string]interface{} {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	stats := make(map[string]interface{})
	
	for serviceName, instances := range sr.services {
		serviceStats := make([]map[string]interface{}, 0, len(instances))
		
		for _, instance := range instances {
			instanceStats := map[string]interface{}{
				"id":            instance.ID,
				"url":           instance.URL,
				"healthy":       instance.IsHealthy(),
				"weight":        instance.Weight,
				"last_check":    instance.LastCheck,
				"response_time": atomic.LoadInt64(&instance.ResponseTime),
				"request_count": atomic.LoadInt64(&instance.RequestCount),
				"error_count":   atomic.LoadInt64(&instance.ErrorCount),
				"error_rate":    instance.GetErrorRate(),
			}
			serviceStats = append(serviceStats, instanceStats)
		}
		
		stats[serviceName] = serviceStats
	}
	
	return stats
}