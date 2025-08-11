package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP request metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyairtable",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pyairtable",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "pyairtable",
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being processed",
		},
	)
)

// RequestID adds a unique request ID to each request
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check if request ID already exists in header
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			// Generate new UUID for request ID
			requestID = uuid.New().String()
		}

		// Set request ID in response header
		c.Set("X-Request-ID", requestID)

		// Store request ID in context for logging
		c.Locals("request_id", requestID)

		return c.Next()
	}
}

// Metrics collects HTTP metrics using Prometheus
func Metrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Increment in-flight requests
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		// Process request
		err := c.Next()

		// Calculate duration
		duration := time.Since(start).Seconds()

		// Get labels
		method := c.Method()
		path := c.Route().Path
		statusCode := string(rune(c.Response().StatusCode()))

		// Update metrics
		httpRequestsTotal.WithLabelValues(method, path, statusCode).Inc()
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)

		return err
	}
}

// Timing adds timing information to response headers
func Timing() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		// Add timing header
		duration := time.Since(start)
		c.Set("X-Response-Time", duration.String())

		return err
	}
}

// SecurityHeaders adds common security headers
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Security headers
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		return c.Next()
	}
}

// TenantContext extracts tenant information from request
func TenantContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract tenant ID from various sources
		tenantID := c.Get("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.Query("tenant_id")
		}
		if tenantID == "" {
			// Extract from subdomain if using subdomain-based tenancy
			host := c.Get("Host")
			if host != "" {
				// Simple subdomain extraction (extend as needed)
				parts := splitString(host, ".")
				if len(parts) > 2 {
					tenantID = parts[0]
				}
			}
		}

		// Store tenant ID in context
		if tenantID != "" {
			c.Locals("tenant_id", tenantID)
		}

		return c.Next()
	}
}

// splitString is a simple string split function
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}

	var result []string
	var current string

	for _, char := range s {
		if string(char) == sep {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}