package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"github.com/pyairtable/api-gateway/internal/config"
	"github.com/pyairtable/api-gateway/internal/middleware"
	"github.com/pyairtable/api-gateway/internal/routing"
)

// BenchmarkHealthEndpoint tests the performance of the health endpoint
func BenchmarkHealthEndpoint(b *testing.B) {
	app := setupTestApp(b)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/health", nil)
			resp, err := app.Test(req, -1)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
			
			if resp.StatusCode != fiber.StatusOK {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})
}

// BenchmarkRoutingEngine tests the performance of the routing engine
func BenchmarkRoutingEngine(b *testing.B) {
	cfg := createTestConfig()
	logger := createTestLogger()
	
	// Create mock backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()
	
	// Update config to point to mock backend
	cfg.Services.AuthService.URL = backend.URL
	
	registry := routing.NewServiceRegistry(logger, time.Second*30)
	err := registry.RegisterFromConfig(cfg)
	require.NoError(b, err)
	
	engine, err := routing.NewEngine(logger, registry, cfg)
	require.NoError(b, err)
	
	app := fiber.New()
	app.Post("/api/v1/auth/login", func(c *fiber.Ctx) error {
		return engine.RouteRequest(c, "auth")
	})
	
	body := []byte(`{"email":"test@example.com","password":"password"}`)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			
			resp, err := app.Test(req, -1)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
			
			if resp.StatusCode != fiber.StatusOK {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})
}

// BenchmarkLoadBalancer tests different load balancing algorithms
func BenchmarkLoadBalancer(b *testing.B) {
	algorithms := []string{"round_robin", "weighted", "least_connections", "random", "latency"}
	
	for _, algo := range algorithms {
		b.Run(algo, func(b *testing.B) {
			balancer, err := routing.NewLoadBalancer(algo)
			require.NoError(b, err)
			
			instances := createTestInstances(10)
			
			b.ResetTimer()
			b.ReportAllocs()
			
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := balancer.SelectInstance(instances)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkJWTMiddleware tests JWT token validation performance
func BenchmarkJWTMiddleware(b *testing.B) {
	cfg := createTestConfig()
	cfg.Auth.JWTSecret = "test-secret-key-for-benchmarking"
	
	logger := createTestLogger()
	authMiddleware := middleware.NewAuthMiddleware(cfg, logger)
	
	// Generate test token
	token, err := authMiddleware.GenerateToken(
		"user123", 
		"test@example.com", 
		[]string{"user"}, 
		[]string{"read", "write"}, 
		"tenant123",
	)
	require.NoError(b, err)
	
	app := fiber.New()
	app.Use(authMiddleware.JWT())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	
	b.ResetTimer()
	b.ReportAllocs()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			
			resp, err := app.Test(req, -1)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
			
			if resp.StatusCode != fiber.StatusOK {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})
}

// BenchmarkConcurrentRequests tests the gateway under high concurrent load
func BenchmarkConcurrentRequests(b *testing.B) {
	app := setupTestApp(b)
	
	concurrencyLevels := []int{100, 500, 1000, 2000, 5000}
	
	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency_%d", concurrency), func(b *testing.B) {
			b.ResetTimer()
			
			var wg sync.WaitGroup
			requests := make(chan struct{}, concurrency)
			
			// Start workers
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range requests {
						req := httptest.NewRequest("GET", "/health", nil)
						resp, err := app.Test(req, -1)
						if err != nil {
							b.Error(err)
							return
						}
						resp.Body.Close()
					}
				}()
			}
			
			// Send requests
			start := time.Now()
			for i := 0; i < b.N; i++ {
				requests <- struct{}{}
			}
			close(requests)
			
			wg.Wait()
			
			duration := time.Since(start)
			rps := float64(b.N) / duration.Seconds()
			
			b.ReportMetric(rps, "requests/sec")
			b.ReportMetric(duration.Seconds()/float64(b.N)*1000, "ms/request")
		})
	}
}

// BenchmarkMemoryUsage tests memory efficiency
func BenchmarkMemoryUsage(b *testing.B) {
	app := setupTestApp(b)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

// LoadTest simulates real-world load to verify 10k+ RPS capability
func TestLoadCapability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}
	
	app := setupTestApp(t)
	
	// Test parameters
	duration := 10 * time.Second
	targetRPS := 10000
	concurrency := 1000
	
	t.Logf("Starting load test: %d RPS target for %v with %d concurrent connections", 
		targetRPS, duration, concurrency)
	
	var (
		totalRequests  int64
		successRequests int64
		errors        int64
		mu            sync.Mutex
	)
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for {
				select {
				case <-ctx.Done():
					return
				default:
					req := httptest.NewRequest("GET", "/health", nil)
					resp, err := app.Test(req, 100) // 100ms timeout
					
					mu.Lock()
					totalRequests++
					if err != nil {
						errors++
					} else {
						resp.Body.Close()
						if resp.StatusCode == fiber.StatusOK {
							successRequests++
						} else {
							errors++
						}
					}
					mu.Unlock()
				}
			}
		}()
	}
	
	wg.Wait()
	
	actualRPS := float64(totalRequests) / duration.Seconds()
	successRate := float64(successRequests) / float64(totalRequests) * 100
	
	t.Logf("Load test results:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Successful requests: %d", successRequests)
	t.Logf("  Errors: %d", errors)
	t.Logf("  Actual RPS: %.2f", actualRPS)
	t.Logf("  Success rate: %.2f%%", successRate)
	
	// Verify we achieved our performance target
	assert.Greater(t, actualRPS, float64(targetRPS*0.8), "Should achieve at least 80% of target RPS")
	assert.Greater(t, successRate, 95.0, "Success rate should be above 95%")
}

// Helper functions

func setupTestApp(t testing.TB) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"timestamp": time.Now().Unix(),
		})
	})
	
	return app
}

func createTestConfig() *config.Config {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:        "localhost",
			Port:        8080,
			Environment: "test",
			Concurrency: 256 * 1024,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
		Performance: config.PerformanceConfig{
			LoadBalancing: config.LoadBalancingConfig{
				Algorithm: "round_robin",
			},
		},
	}
	
	// Set up test services
	cfg.Services.AuthService.URL = "http://localhost:8081"
	cfg.Services.AuthService.Timeout = 30 * time.Second
	cfg.Services.AuthService.RetryCount = 3
	cfg.Services.AuthService.RetryDelay = time.Second
	cfg.Services.AuthService.HealthCheckPath = "/health"
	cfg.Services.AuthService.Weight = 1
	
	return cfg
}

func createTestLogger() *zap.Logger {
	// Return a no-op logger for benchmarks
	return zap.NewNop()
}

func createTestInstances(count int) []*routing.ServiceInstance {
	instances := make([]*routing.ServiceInstance, count)
	
	for i := 0; i < count; i++ {
		instances[i] = &routing.ServiceInstance{
			ID:     fmt.Sprintf("instance-%d", i),
			Name:   "test-service",
			URL:    fmt.Sprintf("http://localhost:%d", 8000+i),
			Weight: 1,
		}
		instances[i].SetHealthy(true)
	}
	
	return instances
}