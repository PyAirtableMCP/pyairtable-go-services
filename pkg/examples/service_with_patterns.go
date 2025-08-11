// Package examples demonstrates how to integrate graceful shutdown and circuit breaker patterns
// Tasks: immediate-11, immediate-12 - Example implementation of graceful shutdown and circuit breaker
package examples

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sirupsen/logrus"

	"github.com/Reg-Kris/pyairtable-platform/pkg/circuitbreaker"
	"github.com/Reg-Kris/pyairtable-platform/pkg/graceful"
)

// ExampleService demonstrates proper integration of graceful shutdown and circuit breaker patterns
type ExampleService struct {
	app    *fiber.App
	db     *sql.DB
	logger *logrus.Logger
	
	// Circuit breakers for external dependencies
	dbCircuitBreaker       *circuitbreaker.CircuitBreaker
	airtableCircuitBreaker *circuitbreaker.CircuitBreaker
	
	shutdownHandler *graceful.ShutdownHandler
}

// NewExampleService creates a new service with all patterns integrated
func NewExampleService() *ExampleService {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	service := &ExampleService{
		app:    fiber.New(),
		logger: logger,
	}

	// Initialize circuit breakers
	service.dbCircuitBreaker = circuitbreaker.NewDatabaseCircuitBreaker("database", logger)
	service.airtableCircuitBreaker = circuitbreaker.NewHTTPCircuitBreaker("airtable", logger)

	// Setup graceful shutdown
	service.shutdownHandler = graceful.NewShutdownHandler(
		graceful.WithLogger(logger),
		graceful.WithTimeout(30*time.Second),
	)

	// Register shutdown hooks
	service.registerShutdownHooks()
	
	// Setup routes
	service.setupRoutes()

	return service
}

// registerShutdownHooks adds all necessary shutdown hooks
func (s *ExampleService) registerShutdownHooks() {
	// HTTP server shutdown
	s.shutdownHandler.AddHook(graceful.HTTPServerShutdownHook(s.app))

	// Database connection shutdown
	if s.db != nil {
		s.shutdownHandler.AddHook(graceful.DatabaseConnectionShutdownHook(s.db))
	}

	// Custom cleanup hook
	s.shutdownHandler.AddHook(s.customCleanupHook())

	// Metrics flush hook
	s.shutdownHandler.AddHook(graceful.MetricsFlushHook(s.flushMetrics))
}

// customCleanupHook demonstrates custom cleanup logic
func (s *ExampleService) customCleanupHook() graceful.ShutdownHook {
	return func(ctx context.Context) error {
		s.logger.Info("Performing custom cleanup tasks")
		
		// Simulate cleanup work
		select {
		case <-time.After(2 * time.Second):
			s.logger.Info("Custom cleanup completed")
			return nil
		case <-ctx.Done():
			s.logger.Warn("Custom cleanup timed out")
			return ctx.Err()
		}
	}
}

// flushMetrics demonstrates metrics flushing during shutdown
func (s *ExampleService) flushMetrics() error {
	s.logger.Info("Flushing metrics before shutdown")
	// Simulate metrics flush
	time.Sleep(500 * time.Millisecond)
	return nil
}

// setupRoutes configures HTTP routes with circuit breaker integration
func (s *ExampleService) setupRoutes() {
	// Health check endpoint (no circuit breaker needed)
	s.app.Get("/health", s.healthHandler)

	// Database endpoint with circuit breaker
	s.app.Get("/users", s.getUsersHandler)

	// External API endpoint with circuit breaker
	s.app.Get("/airtable/data", s.getAirtableDataHandler)

	// Circuit breaker status endpoint
	s.app.Get("/circuit-breakers", s.circuitBreakersStatusHandler)
}

// healthHandler provides basic health check
func (s *ExampleService) healthHandler(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "example-service",
	})
}

// getUsersHandler demonstrates database access with circuit breaker
func (s *ExampleService) getUsersHandler(c fiber.Ctx) error {
	ctx := c.Context()

	// Use circuit breaker for database access
	result, err := s.dbCircuitBreaker.Call(func() (interface{}, error) {
		return s.fetchUsersFromDB(ctx)
	})

	if err != nil {
		if err == circuitbreaker.ErrOpenState {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Database service temporarily unavailable",
				"code":  "DB_CIRCUIT_OPEN",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch users",
		})
	}

	return c.JSON(result)
}

// getAirtableDataHandler demonstrates external API access with circuit breaker
func (s *ExampleService) getAirtableDataHandler(c fiber.Ctx) error {
	ctx := c.Context()

	// Use circuit breaker for external API calls
	result, err := s.airtableCircuitBreaker.Call(func() (interface{}, error) {
		return s.fetchAirtableData(ctx)
	})

	if err != nil {
		if err == circuitbreaker.ErrOpenState {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"error":   "Airtable service temporarily unavailable",
				"code":    "AIRTABLE_CIRCUIT_OPEN",
				"fallback": s.getFallbackData(),
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch Airtable data",
		})
	}

	return c.JSON(result)
}

// circuitBreakersStatusHandler provides circuit breaker status for monitoring
func (s *ExampleService) circuitBreakersStatusHandler(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"circuit_breakers": fiber.Map{
			"database": fiber.Map{
				"name":   s.dbCircuitBreaker.Name(),
				"state":  s.dbCircuitBreaker.State().String(),
				"counts": s.dbCircuitBreaker.Counts(),
			},
			"airtable": fiber.Map{
				"name":   s.airtableCircuitBreaker.Name(),
				"state":  s.airtableCircuitBreaker.State().String(),
				"counts": s.airtableCircuitBreaker.Counts(),
			},
		},
		"timestamp": time.Now(),
	})
}

// fetchUsersFromDB simulates database access that might fail
func (s *ExampleService) fetchUsersFromDB(ctx context.Context) (interface{}, error) {
	// Simulate database query with potential failure
	select {
	case <-time.After(100 * time.Millisecond):
		// Simulate success most of the time
		return []map[string]interface{}{
			{"id": 1, "name": "Alice", "email": "alice@example.com"},
			{"id": 2, "name": "Bob", "email": "bob@example.com"},
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fetchAirtableData simulates external API call that might fail
func (s *ExampleService) fetchAirtableData(ctx context.Context) (interface{}, error) {
	// Simulate HTTP call to Airtable with potential failure
	select {
	case <-time.After(200 * time.Millisecond):
		// Simulate success
		return map[string]interface{}{
			"records": []map[string]interface{}{
				{"id": "rec1", "fields": map[string]string{"Name": "Record 1"}},
				{"id": "rec2", "fields": map[string]string{"Name": "Record 2"}},
			},
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getFallbackData provides fallback data when Airtable circuit breaker is open
func (s *ExampleService) getFallbackData() interface{} {
	return map[string]interface{}{
		"records": []map[string]interface{}{
			{"id": "cached1", "fields": map[string]string{"Name": "Cached Record 1"}},
		},
		"source": "cache",
	}
}

// Start starts the service and waits for shutdown signals
func (s *ExampleService) Start(port int) {
	// Start the HTTP server in a goroutine
	go func() {
		addr := fmt.Sprintf(":%d", port)
		s.logger.WithField("port", port).Info("Starting HTTP server")
		
		if err := s.app.Listen(addr); err != nil {
			s.logger.WithError(err).Error("HTTP server failed to start")
		}
	}()

	// Wait for shutdown signal
	s.logger.Info("Service started, waiting for shutdown signal")
	s.shutdownHandler.Wait()
	s.logger.Info("Service shutdown completed")
}

// ExampleUsage demonstrates how to use the service
func ExampleUsage() {
	service := NewExampleService()
	service.Start(8080)
}

// Advanced circuit breaker usage with custom configuration
func AdvancedCircuitBreakerExample() {
	logger := logrus.New()

	// Custom circuit breaker for a specific external service
	customCB := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{
		Name:        "custom-api",
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts circuitbreaker.Counts) bool {
			// Custom trip condition: trip after 3 consecutive failures
			// or if failure rate exceeds 60% with at least 5 requests
			return counts.ConsecutiveFailures >= 3 ||
				(counts.Requests >= 5 && 
				 float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6)
		},
		IsSuccessful: func(err error) bool {
			// Custom success condition: only nil error is success
			return err == nil
		},
		OnStateChange: func(name string, from, to circuitbreaker.State) {
			logger.WithFields(logrus.Fields{
				"circuit_breaker": name,
				"from_state":     from.String(),
				"to_state":       to.String(),
			}).Warn("Circuit breaker state changed")
		},
		Logger: logger,
	})

	// Example usage with context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := customCB.ExecuteWithContext(ctx, func(ctx context.Context) error {
		// Simulate external API call
		select {
		case <-time.After(1 * time.Second):
			return nil // Success
		case <-ctx.Done():
			return ctx.Err() // Timeout or cancellation
		}
	})

	if err != nil {
		logger.WithError(err).Error("Circuit breaker execution failed")
	}
}

// Example of service initialization with proper error handling
func RobustServiceInitialization() (*ExampleService, error) {
	logger := logrus.New()
	
	service := &ExampleService{
		logger: logger,
	}

	// Initialize database with retry logic
	var err error
	for i := 0; i < 3; i++ {
		service.db, err = sql.Open("postgres", "connection_string")
		if err == nil {
			break
		}
		logger.WithError(err).Warnf("Database connection attempt %d failed, retrying...", i+1)
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after 3 attempts: %w", err)
	}

	// Initialize circuit breakers with custom configurations based on service requirements
	service.dbCircuitBreaker = circuitbreaker.NewDatabaseCircuitBreaker("database", logger)
	service.airtableCircuitBreaker = circuitbreaker.NewHTTPCircuitBreaker("airtable", logger)

	// Initialize graceful shutdown with custom timeout based on service needs
	service.shutdownHandler = graceful.NewShutdownHandler(
		graceful.WithLogger(logger),
		graceful.WithTimeout(45*time.Second), // Longer timeout for complex cleanup
	)

	service.registerShutdownHooks()
	service.setupRoutes()

	return service, nil
}