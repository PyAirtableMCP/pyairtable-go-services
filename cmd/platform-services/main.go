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
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"pyairtable-go/pkg/config"
	"pyairtable-go/pkg/database"
	"pyairtable-go/pkg/middleware"
	"pyairtable-go/pkg/observability"
	"pyairtable-go/pkg/redis"
	"pyairtable-go/pkg/services/analytics"
	"pyairtable-go/pkg/services/auth"
)

type PlatformService struct {
	app            *fiber.App
	config         *config.Config
	logger         *observability.Logger
	db             *database.DB
	redisClient    *redis.Client
	authService    *auth.Service
	analyticsService *analytics.Service
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize logger
	logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)

	// Initialize database
	db, err := database.New(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to database", "error", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		logger.Fatal("Failed to run migrations", "error", err)
	}

	// Initialize Redis
	redisClient, err := redis.New(cfg.Redis.URL, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", "error", err)
	}
	defer redisClient.Close()

	// Initialize services
	authService := auth.NewService(db, redisClient, cfg)
	analyticsService := analytics.NewService(db, redisClient, cfg)

	// Create platform service instance
	service := NewPlatformService(cfg, logger, db, redisClient, authService, analyticsService)

	// Setup routes
	service.setupRoutes()

	// Start server
	logger.Info("Starting Platform Services", "port", cfg.Server.Port)

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
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := service.app.Shutdown(); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exited")
}

func NewPlatformService(
	cfg *config.Config,
	logger *observability.Logger,
	db *database.DB,
	redisClient *redis.Client,
	authService *auth.Service,
	analyticsService *analytics.Service,
) *PlatformService {
	// Configure Fiber app
	app := fiber.New(fiber.Config{
		ServerHeader:          "PyAirtable-Platform",
		StrictRouting:         true,
		CaseSensitive:         true,
		DisableStartupMessage: true,
		ReduceMemoryUsage:     true,
		ErrorHandler:          errorHandler,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           120 * time.Second,
	})

	return &PlatformService{
		app:              app,
		config:           cfg,
		logger:           logger,
		db:               db,
		redisClient:      redisClient,
		authService:      authService,
		analyticsService: analyticsService,
	}
}

func (s *PlatformService) setupRoutes() {
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

	// Health check
	s.app.Get("/health", s.healthCheck)

	// Metrics endpoint
	if s.config.Metrics.Enabled {
		s.app.Get(s.config.Metrics.Path, fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))
	}

	// API routes
	api := s.app.Group("/api/v1")

	// Authentication routes (no auth required)
	auth := api.Group("/auth")
	auth.Post("/register", s.register)
	auth.Post("/login", s.login)
	auth.Post("/refresh", s.refreshToken)
	auth.Post("/logout", middleware.JWTAuth(s.config.JWT.Secret), s.logout)

	// User routes (auth required)
	users := api.Group("/users")
	users.Use(middleware.JWTAuth(s.config.JWT.Secret))
	users.Get("/profile", s.getProfile)
	users.Put("/profile", s.updateProfile)
	users.Delete("/profile", s.deleteProfile)

	// Analytics routes (auth required)
	analyticsGroup := api.Group("/analytics")
	analyticsGroup.Use(middleware.JWTAuth(s.config.JWT.Secret))
	analyticsGroup.Post("/events", s.trackEvent)
	analyticsGroup.Get("/events", s.getEvents)
	analyticsGroup.Get("/dashboard", s.getDashboard)

	// Admin routes (admin auth required)
	admin := api.Group("/admin")
	admin.Use(middleware.JWTAuth(s.config.JWT.Secret))
	admin.Use(s.requireAdmin)
	admin.Get("/users", s.listUsers)
	admin.Get("/analytics/summary", s.getAnalyticsSummary)
}

// Authentication handlers
func (s *PlatformService) register(c *fiber.Ctx) error {
	var req auth.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	user, err := s.authService.Register(c.Context(), &req)
	if err != nil {
		s.logger.Error("Registration failed", "error", err, "email", req.Email)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Track registration event
	go s.analyticsService.Track(c.Context(), "user_registered", user.ID, map[string]interface{}{
		"timestamp": time.Now(),
		"ip":        c.IP(),
		"user_agent": c.Get("User-Agent"),
	})

	s.logger.Info("User registered successfully", "user_id", user.ID, "email", user.Email)

	return c.Status(201).JSON(fiber.Map{
		"message": "User registered successfully",
		"user":    user,
	})
}

func (s *PlatformService) login(c *fiber.Ctx) error {
	var req auth.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	result, err := s.authService.Login(c.Context(), &req)
	if err != nil {
		s.logger.Error("Login failed", "error", err, "email", req.Email)
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	// Track login event
	go s.analyticsService.Track(c.Context(), "user_login", result.User.ID, map[string]interface{}{
		"timestamp":  time.Now(),
		"ip":         c.IP(),
		"user_agent": c.Get("User-Agent"),
	})

	s.logger.Info("User logged in successfully", "user_id", result.User.ID, "email", result.User.Email)

	return c.JSON(result)
}

func (s *PlatformService) refreshToken(c *fiber.Ctx) error {
	var req auth.RefreshTokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	result, err := s.authService.RefreshToken(c.Context(), &req)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (s *PlatformService) logout(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	if err := s.authService.Logout(c.Context(), userID); err != nil {
		s.logger.Error("Logout failed", "error", err, "user_id", userID)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to logout"})
	}

	s.logger.Info("User logged out successfully", "user_id", userID)

	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

// User profile handlers
func (s *PlatformService) getProfile(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	user, err := s.authService.GetUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	return c.JSON(user)
}

func (s *PlatformService) updateProfile(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	var req auth.UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	user, err := s.authService.UpdateProfile(c.Context(), userID, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(user)
}

func (s *PlatformService) deleteProfile(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	if err := s.authService.DeleteUser(c.Context(), userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete user"})
	}

	return c.JSON(fiber.Map{"message": "User deleted successfully"})
}

// Analytics handlers
func (s *PlatformService) trackEvent(c *fiber.Ctx) error {
	userID, _ := middleware.GetUserID(c) // Optional for analytics

	var req analytics.TrackEventRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	event, err := s.analyticsService.Track(c.Context(), req.EventName, userID, req.Properties)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to track event"})
	}

	return c.Status(201).JSON(event)
}

func (s *PlatformService) getEvents(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	events, err := s.analyticsService.GetUserEvents(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get events"})
	}

	return c.JSON(events)
}

func (s *PlatformService) getDashboard(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	dashboard, err := s.analyticsService.GetUserDashboard(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get dashboard"})
	}

	return c.JSON(dashboard)
}

// Admin handlers
func (s *PlatformService) listUsers(c *fiber.Ctx) error {
	users, err := s.authService.ListUsers(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list users"})
	}

	return c.JSON(users)
}

func (s *PlatformService) getAnalyticsSummary(c *fiber.Ctx) error {
	summary, err := s.analyticsService.GetSummary(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get analytics summary"})
	}

	return c.JSON(summary)
}

// Middleware
func (s *PlatformService) requireAdmin(c *fiber.Ctx) error {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		return c.Status(401).JSON(fiber.Map{"error": "User not authenticated"})
	}

	isAdmin, err := s.authService.IsAdmin(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to check admin status"})
	}

	if !isAdmin {
		return c.Status(403).JSON(fiber.Map{"error": "Admin access required"})
	}

	return c.Next()
}

// Health check
func (s *PlatformService) healthCheck(c *fiber.Ctx) error {
	// Check database connectivity
	if err := s.db.Ping(c.Context()); err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  "Database connection failed",
		})
	}

	// Check Redis connectivity
	if err := s.redisClient.Ping(c.Context()); err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  "Redis connection failed",
		})
	}

	return c.JSON(fiber.Map{
		"status":    "healthy",
		"service":   "PyAirtable Platform Services",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC(),
	})
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