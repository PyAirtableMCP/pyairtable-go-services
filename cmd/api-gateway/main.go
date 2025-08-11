package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"pyairtable-go/pkg/config"
	"pyairtable-go/pkg/middleware"
	"pyairtable-go/pkg/observability"
)

type APIGateway struct {
	app    *fiber.App
	config *config.Config
	logger *observability.Logger
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize logger
	logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)

	// Create API Gateway instance
	gateway := NewAPIGateway(cfg, logger)

	// Setup routes
	gateway.setupRoutes()

	// Start server
	logger.Info("Starting API Gateway", "port", cfg.Server.Port)

	// Graceful shutdown
	go func() {
		if err := gateway.app.Listen(":" + cfg.Server.Port); err != nil {
			logger.Error("Server failed to start", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server is shutting down...")

	// Graceful shutdown with timeout
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := gateway.app.Shutdown(); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exited")
}

func NewAPIGateway(cfg *config.Config, logger *observability.Logger) *APIGateway {
	// Configure Fiber app
	app := fiber.New(fiber.Config{
		Prefork:                 false, // Enable for production
		ServerHeader:            "PyAirtable-Gateway",
		StrictRouting:           true,
		CaseSensitive:           true,
		DisableStartupMessage:   true,
		ReduceMemoryUsage:       true,
		ErrorHandler:            errorHandler,
		BodyLimit:               int(cfg.FileProcessing.MaxFileSize),
		ReadTimeout:             30 * time.Second,
		WriteTimeout:            30 * time.Second,
		IdleTimeout:             120 * time.Second,
	})

	return &APIGateway{
		app:    app,
		config: cfg,
		logger: logger,
	}
}

func (g *APIGateway) setupRoutes() {
	// Global middleware
	g.app.Use(recover.New())
	g.app.Use(helmet.New())
	g.app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	// CORS middleware
	g.app.Use(cors.New(cors.Config{
		AllowOrigins: joinStrings(g.config.CORS.Origins, ","),
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-API-Key",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Logging middleware
	g.app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))

	// Rate limiting
	g.app.Use(limiter.New(limiter.Config{
		Max:        g.config.RateLimit.RequestsPerMinute,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.Get("X-Forwarded-For", c.IP())
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{
				"error": "Rate limit exceeded",
			})
		},
	}))

	// Custom middleware
	g.app.Use(middleware.RequestID())
	g.app.Use(middleware.Metrics())

	// Health check endpoint
	g.app.Get("/health", g.healthCheck)

	// Metrics endpoint
	if g.config.Metrics.Enabled {
		g.app.Get(g.config.Metrics.Path, fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))
	}

	// API routes with authentication
	api := g.app.Group("/api")
	api.Use(middleware.APIKeyAuth(g.config.Auth.APIKey, g.config.Auth.RequireAPIKey))

	// Proxy routes to services
	g.setupProxyRoutes(api)
}

func (g *APIGateway) setupProxyRoutes(api fiber.Router) {
	// LLM Orchestrator routes
	api.All("/llm/*", proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Services.LLMOrchestratorURL},
		ModifyRequest: func(c *fiber.Ctx) error {
			c.Request().URI().SetPath(c.Params("*"))
			return nil
		},
	}))

	// MCP Server routes
	api.All("/mcp/*", proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Services.MCPServerURL},
		ModifyRequest: func(c *fiber.Ctx) error {
			c.Request().URI().SetPath(c.Params("*"))
			return nil
		},
	}))

	// Airtable Gateway routes
	api.All("/airtable/*", proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Services.AirtableGatewayURL},
		ModifyRequest: func(c *fiber.Ctx) error {
			c.Request().URI().SetPath(c.Params("*"))
			return nil
		},
	}))

	// Platform Services routes
	api.All("/platform/*", proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Services.PlatformServicesURL},
		ModifyRequest: func(c *fiber.Ctx) error {
			c.Request().URI().SetPath(c.Params("*"))
			return nil
		},
	}))

	// Automation Services routes
	api.All("/automation/*", proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Services.AutomationServicesURL},
		ModifyRequest: func(c *fiber.Ctx) error {
			c.Request().URI().SetPath(c.Params("*"))
			return nil
		},
	}))
}

func (g *APIGateway) healthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "healthy",
		"service":   "PyAirtable API Gateway",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(startTime).String(),
	})
}

var startTime = time.Now()

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"error":     err.Error(),
		"timestamp": time.Now().UTC(),
		"path":      c.Path(),
		"method":    c.Method(),
	})
}

func joinStrings(slice []string, sep string) string {
	if len(slice) == 0 {
		return ""
	}
	if len(slice) == 1 {
		return slice[0]
	}

	result := slice[0]
	for _, s := range slice[1:] {
		result += sep + s
	}
	return result
}