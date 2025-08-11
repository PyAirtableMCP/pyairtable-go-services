package main

import (
	"context"
	"database/sql"
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
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"github.com/pyairtable-compose/go-services/pkg/cache"
	"github.com/pyairtable-compose/go-services/pkg/config"
	"github.com/pyairtable-compose/go-services/pkg/cqrs"
	"github.com/pyairtable-compose/go-services/pkg/database"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
	"github.com/pyairtable-compose/go-services/pkg/middleware"
	"github.com/pyairtable-compose/go-services/pkg/observability"
)

type CQRSPlatformService struct {
	app            *fiber.App
	config         *config.Config
	logger         *observability.Logger
	
	// Database connections
	writeDB        *sql.DB
	readDB         *sql.DB
	
	// CQRS components
	cqrsService    *cqrs.CQRSService
	queryService   *cqrs.QueryService
	eventStore     *eventstore.PostgresEventStore
	
	// Cache
	redisClient    *cache.RedisClient
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize logger
	logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)

	// Initialize Redis client
	redisClient, err := cache.NewRedisClient(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", "error", err)
	}
	defer redisClient.Close()

	// Initialize CQRS platform service
	service, err := NewCQRSPlatformService(cfg, logger, redisClient)
	if err != nil {
		logger.Fatal("Failed to initialize CQRS platform service", "error", err)
	}
	defer service.Close()

	// Setup routes
	service.setupRoutes()

	// Start server
	logger.Info("Starting CQRS Platform Services", "port", cfg.Server.Port)

	// Graceful shutdown
	go func() {
		if err := service.app.Listen(":" + cfg.Server.Port); err != nil {
			logger.Error("Server failed to start", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server is shutting down...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := service.app.ShutdownWithContext(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exited")
}

func NewCQRSPlatformService(
	cfg *config.Config,
	logger *observability.Logger,
	redisClient *cache.RedisClient,
) (*CQRSPlatformService, error) {
	// Initialize CQRS configuration
	cqrsConfig := cqrs.DefaultCQRSServiceConfig()
	cqrsConfig.Database.WriteConnectionStr = cfg.Database.URL
	cqrsConfig.Database.ReadConnectionStr = cfg.Database.URL // Same DB for now, different schemas

	// Initialize CQRS service
	cqrsService, err := cqrs.NewCQRSService(cqrsConfig, redisClient)
	if err != nil {
		return nil, err
	}

	// Initialize event store
	eventStore := eventstore.NewPostgresEventStore(cqrsService.DatabaseConfig.GetWriteDB())

	// Initialize query service
	queryService := cqrs.NewQueryService(
		cqrsService.DatabaseConfig.GetReadDB(),
		cqrsService.CacheManager,
		cqrsService.QueryOptimizer,
	)

	// Register projection handlers
	userHandler := cqrs.NewUserProjectionHandler(cqrsService.DatabaseConfig.GetReadDB())
	workspaceHandler := cqrs.NewWorkspaceProjectionHandler(cqrsService.DatabaseConfig.GetReadDB())
	
	cqrsService.ProjectionManager.RegisterProjection(userHandler)
	cqrsService.ProjectionManager.RegisterProjection(workspaceHandler)

	// Configure Fiber app
	app := fiber.New(fiber.Config{
		ServerHeader:          "PyAirtable-CQRS-Platform",
		StrictRouting:         true,
		CaseSensitive:         true,
		DisableStartupMessage: true,
		ReduceMemoryUsage:     true,
		ErrorHandler:          errorHandler,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           120 * time.Second,
	})

	// Warm up cache on startup
	go func() {
		ctx := context.Background()
		if err := cqrsService.CacheManager.WarmupCache(ctx); err != nil {
			logger.Error("Failed to warm up cache", "error", err)
		}
	}()

	logger.Info("CQRS Platform Service initialized successfully",
		"write_db", "write_schema",
		"read_db", "read_schema",
		"cache_strategy", "multi-level",
		"projection_strategy", "asynchronous",
	)

	return &CQRSPlatformService{
		app:          app,
		config:       cfg,
		logger:       logger,
		writeDB:      cqrsService.DatabaseConfig.GetWriteDB(),
		readDB:       cqrsService.DatabaseConfig.GetReadDB(),
		cqrsService:  cqrsService,
		queryService: queryService,
		eventStore:   eventStore,
		redisClient:  redisClient,
	}, nil
}

func (s *CQRSPlatformService) setupRoutes() {
	// Global middleware
	s.app.Use(recover.New())
	s.app.Use(helmet.New())
	s.app.Use(compress.New())

	// CORS middleware
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: joinStrings(s.config.CORS.Origins, ","),
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-API-Key",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Logging middleware
	s.app.Use(logger.New())

	// Rate limiting
	s.app.Use(limiter.New(limiter.Config{
		Max:        s.config.RateLimit.RequestsPerMinute,
		Expiration: 1 * time.Minute,
	}))

	// Custom middleware
	s.app.Use(middleware.RequestID())
	s.app.Use(middleware.Metrics())
	s.app.Use(middleware.TenantContext())

	// Health check with CQRS status
	s.app.Get("/health", s.healthCheck)
	s.app.Get("/health/cqrs", s.cqrsHealthCheck)

	// Metrics endpoint
	if s.config.Metrics.Enabled {
		s.app.Get(s.config.Metrics.Path, fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))
	}

	// API routes
	api := s.app.Group("/api/v1")

	// User routes (CQRS queries)
	users := api.Group("/users")
	users.Use(middleware.JWTAuth(s.config.JWT.Secret))
	users.Get("/profile", s.getUserProfile)
	users.Get("/tenant/:tenantId", s.getUsersByTenant)
	users.Get("/:userId", s.getUserByID)

	// Workspace routes (CQRS queries)
	workspaces := api.Group("/workspaces")
	workspaces.Use(middleware.JWTAuth(s.config.JWT.Secret))
	workspaces.Get("/tenant/:tenantId", s.getWorkspacesByTenant)
	workspaces.Get("/:workspaceId", s.getWorkspaceByID)
	workspaces.Get("/:workspaceId/members", s.getWorkspaceMembers)

	// Tenant routes (CQRS queries)
	tenants := api.Group("/tenants")
	tenants.Use(middleware.JWTAuth(s.config.JWT.Secret))
	tenants.Get("/:tenantId/summary", s.getTenantSummary)
	tenants.Get("/:tenantId/dashboard", s.getTenantDashboard)

	// Admin routes for CQRS management
	admin := api.Group("/admin/cqrs")
	admin.Use(middleware.JWTAuth(s.config.JWT.Secret))
	admin.Use(s.requireAdmin)
	admin.Get("/projections/status", s.getProjectionStatus)
	admin.Get("/cache/stats", s.getCacheStats)
	admin.Post("/cache/invalidate", s.invalidateCache)
	admin.Post("/cache/warmup", s.warmupCache)
}

// CQRS Query Handlers

func (s *CQRSPlatformService) getUserProfile(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	tenantID, exists := middleware.GetTenantID(c)
	if !exists {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID required"})
	}

	user, err := s.queryService.GetUserByID(c.Context(), userID, tenantID)
	if err != nil {
		s.logger.Error("Failed to get user profile", "error", err, "user_id", userID)
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	s.logger.Info("User profile retrieved via CQRS", "user_id", userID, "cache_hit", true)
	return c.JSON(user)
}

func (s *CQRSPlatformService) getUserByID(c *fiber.Ctx) error {
	userID := c.Params("userId")
	tenantID, exists := middleware.GetTenantID(c)
	if !exists {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID required"})
	}

	user, err := s.queryService.GetUserByID(c.Context(), userID, tenantID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	return c.JSON(user)
}

func (s *CQRSPlatformService) getUsersByTenant(c *fiber.Ctx) error {
	tenantID := c.Params("tenantId")
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	activeOnly := c.QueryBool("active_only", false)

	if limit > 100 {
		limit = 100 // Max limit for performance
	}

	users, err := s.queryService.GetUsersByTenant(c.Context(), tenantID, limit, offset, activeOnly)
	if err != nil {
		s.logger.Error("Failed to get users by tenant", "error", err, "tenant_id", tenantID)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve users"})
	}

	return c.JSON(fiber.Map{
		"users": users,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
			"count":  len(users),
		},
	})
}

func (s *CQRSPlatformService) getWorkspaceByID(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	tenantID, exists := middleware.GetTenantID(c)
	if !exists {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID required"})
	}

	workspace, err := s.queryService.GetWorkspaceByID(c.Context(), workspaceID, tenantID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Workspace not found"})
	}

	return c.JSON(workspace)
}

func (s *CQRSPlatformService) getWorkspacesByTenant(c *fiber.Ctx) error {
	tenantID := c.Params("tenantId")
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	if limit > 100 {
		limit = 100
	}

	workspaces, err := s.queryService.GetWorkspacesByTenant(c.Context(), tenantID, limit, offset)
	if err != nil {
		s.logger.Error("Failed to get workspaces by tenant", "error", err, "tenant_id", tenantID)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve workspaces"})
	}

	return c.JSON(fiber.Map{
		"workspaces": workspaces,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
			"count":  len(workspaces),
		},
	})
}

func (s *CQRSPlatformService) getWorkspaceMembers(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	limit := c.QueryInt("limit", 100)
	offset := c.QueryInt("offset", 0)

	members, err := s.queryService.GetWorkspaceMembers(c.Context(), workspaceID, limit, offset)
	if err != nil {
		s.logger.Error("Failed to get workspace members", "error", err, "workspace_id", workspaceID)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve workspace members"})
	}

	return c.JSON(fiber.Map{
		"members": members,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
			"count":  len(members),
		},
	})
}

func (s *CQRSPlatformService) getTenantSummary(c *fiber.Ctx) error {
	tenantID := c.Params("tenantId")

	summary, err := s.queryService.GetTenantSummary(c.Context(), tenantID)
	if err != nil {
		s.logger.Error("Failed to get tenant summary", "error", err, "tenant_id", tenantID)
		return c.Status(404).JSON(fiber.Map{"error": "Tenant summary not found"})
	}

	return c.JSON(summary)
}

func (s *CQRSPlatformService) getTenantDashboard(c *fiber.Ctx) error {
	tenantID := c.Params("tenantId")

	// Get tenant summary
	summary, err := s.queryService.GetTenantSummary(c.Context(), tenantID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Get recent users (last 10)
	recentUsers, err := s.queryService.GetUsersByTenant(c.Context(), tenantID, 10, 0, false)
	if err != nil {
		s.logger.Error("Failed to get recent users for dashboard", "error", err)
		recentUsers = []cqrs.UserProjection{}
	}

	// Get recent workspaces (last 10)
	recentWorkspaces, err := s.queryService.GetWorkspacesByTenant(c.Context(), tenantID, 10, 0)
	if err != nil {
		s.logger.Error("Failed to get recent workspaces for dashboard", "error", err)
		recentWorkspaces = []cqrs.WorkspaceProjection{}
	}

	dashboard := fiber.Map{
		"summary":           summary,
		"recent_users":      recentUsers,
		"recent_workspaces": recentWorkspaces,
		"performance": fiber.Map{
			"cache_hit_ratio": s.cqrsService.CacheManager.GetHitRatio(),
			"cache_stats":     s.cqrsService.CacheManager.GetStats(),
		},
	}

	return c.JSON(dashboard)
}

// Admin Handlers for CQRS Management

func (s *CQRSPlatformService) getProjectionStatus(c *fiber.Ctx) error {
	status := s.cqrsService.ProjectionManager.GetProjectionStatus()
	return c.JSON(status)
}

func (s *CQRSPlatformService) getCacheStats(c *fiber.Ctx) error {
	stats := s.cqrsService.CacheManager.GetStats()
	hitRatio := s.cqrsService.CacheManager.GetHitRatio()
	
	return c.JSON(fiber.Map{
		"stats":     stats,
		"hit_ratio": hitRatio,
		"redis_stats": s.redisClient.GetConnectionStats(),
	})
}

func (s *CQRSPlatformService) invalidateCache(c *fiber.Ctx) error {
	var req struct {
		Pattern string `json:"pattern"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Pattern == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Pattern is required"})
	}

	err := s.cqrsService.CacheManager.InvalidatePattern(c.Context(), req.Pattern)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to invalidate cache"})
	}

	return c.JSON(fiber.Map{"message": "Cache invalidated successfully"})
}

func (s *CQRSPlatformService) warmupCache(c *fiber.Ctx) error {
	go func() {
		ctx := context.Background()
		if err := s.cqrsService.CacheManager.WarmupCache(ctx); err != nil {
			s.logger.Error("Failed to warm up cache", "error", err)
		}
	}()

	return c.JSON(fiber.Map{"message": "Cache warmup started"})
}

// Health and Utility Handlers

func (s *CQRSPlatformService) healthCheck(c *fiber.Ctx) error {
	// Basic health check
	if err := s.writeDB.Ping(); err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  "Write database connection failed",
		})
	}

	if err := s.readDB.Ping(); err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  "Read database connection failed",
		})
	}

	if err := s.redisClient.Ping(); err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  "Redis connection failed",
		})
	}

	return c.JSON(fiber.Map{
		"status":    "healthy",
		"service":   "PyAirtable CQRS Platform Services",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC(),
	})
}

func (s *CQRSPlatformService) cqrsHealthCheck(c *fiber.Ctx) error {
	health := s.cqrsService.HealthCheck()
	return c.JSON(health)
}

func (s *CQRSPlatformService) requireAdmin(c *fiber.Ctx) error {
	// TODO: Implement admin check logic
	return c.Next()
}

func (s *CQRSPlatformService) Close() error {
	if s.cqrsService != nil {
		return s.cqrsService.Close()
	}
	return nil
}

// Error handler
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

// Helper function
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