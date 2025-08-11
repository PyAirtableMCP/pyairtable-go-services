package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fiber/fiber/v2"
	"github.com/fiber/fiber/v2/middleware/cors"
	"github.com/fiber/fiber/v2/middleware/logger"
	"github.com/fiber/fiber/v2/middleware/recover"
	"github.com/fiber/fiber/v2/middleware/requestid"
	"go.uber.org/zap"

	"github.com/pyairtable/go-services/plugin-service/internal/config"
	"github.com/pyairtable/go-services/plugin-service/internal/handlers"
	"github.com/pyairtable/go-services/plugin-service/internal/runtime"
	"github.com/pyairtable/go-services/plugin-service/internal/security"
	"github.com/pyairtable/go-services/plugin-service/internal/services"
	"github.com/pyairtable/go-services/plugin-service/pkg/database"
	"github.com/pyairtable/go-services/plugin-service/pkg/logger"
	"github.com/pyairtable/go-services/plugin-service/pkg/metrics"
	"github.com/pyairtable/go-services/plugin-service/pkg/redis"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	zapLogger, err := logger.New(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer zapLogger.Sync()

	zapLogger.Info("Starting Plugin Service", 
		zap.String("version", "1.0.0"),
		zap.String("environment", os.Getenv("ENVIRONMENT")))

	// Initialize database
	db, err := database.NewPostgres(cfg.Database.GetDSN(), cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		zapLogger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Run database migrations
	if err := database.RunMigrations(db, cfg.Database.MigrationsPath); err != nil {
		zapLogger.Fatal("Failed to run database migrations", zap.Error(err))
	}

	// Initialize Redis
	redisClient, err := redis.NewClient(cfg.Redis.GetRedisAddr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		zapLogger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Initialize metrics
	metricsCollector := metrics.NewCollector("plugin_service")

	// Initialize security manager
	securityConfig := &security.SecurityConfig{
		CodeSigningEnabled:     cfg.Security.CodeSigningEnabled,
		TrustedSigners:         cfg.Security.TrustedSigners,
		SandboxEnabled:         cfg.Security.SandboxEnabled,
		PermissionCheckEnabled: cfg.Security.PermissionCheckEnabled,
		MaxPluginSize:          cfg.Security.MaxPluginSize,
		AllowedFileTypes:       cfg.Security.AllowedFileTypes,
		QuarantineEnabled:      cfg.Security.QuarantineEnabled,
		QuarantineTime:         cfg.Security.QuarantineTime,
	}

	auditLogger := &AuditLoggerImpl{logger: zapLogger}
	securityManager := security.NewSecurityManager(securityConfig, auditLogger)

	// Initialize WASM runtime
	ctx := context.Background()
	eventBus := &EventBusImpl{logger: zapLogger}
	wasmRuntime, err := runtime.NewWASMRuntime(ctx, &cfg.Runtime, eventBus)
	if err != nil {
		zapLogger.Fatal("Failed to initialize WASM runtime", zap.Error(err))
	}
	defer wasmRuntime.Shutdown()

	// Initialize repositories
	pluginRepo := NewPluginRepository(db)
	installationRepo := NewInstallationRepository(db)
	executionRepo := NewExecutionRepository(db)
	registryRepo := NewRegistryRepository(db)

	// Initialize cache
	registryCache := NewRegistryCache(redisClient)

	// Initialize services
	registryService := services.NewRegistryService(&cfg.Registry, zapLogger, registryRepo, registryCache)
	notificationService := &NotificationServiceImpl{logger: zapLogger}

	pluginService := services.NewPluginService(
		cfg,
		zapLogger,
		pluginRepo,
		installationRepo,
		executionRepo,
		wasmRuntime,
		securityManager,
		registryService,
		notificationService,
		auditLogger,
	)

	// Initialize HTTP server
	app := fiber.New(fiber.Config{
		ServerHeader: "PyAirtable Plugin Service",
		AppName:      "PyAirtable Plugin Service v1.0.0",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			zapLogger.Error("HTTP error", 
				zap.Error(err),
				zap.Int("status_code", code),
				zap.String("method", c.Method()),
				zap.String("path", c.Path()))
			
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} ${latency}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-User-ID,X-Workspace-ID",
	}))

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"timestamp": time.Now().UTC(),
			"version": "1.0.0",
		})
	})

	// Metrics endpoint
	if cfg.Monitoring.MetricsEnabled {
		app.Get(cfg.Monitoring.MetricsPath, func(c *fiber.Ctx) error {
			return c.SendString(metricsCollector.Gather())
		})
	}

	// Initialize handlers
	pluginHandler := handlers.NewPluginHandler(pluginService)
	pluginHandler.RegisterRoutes(app)

	// Start server
	address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	zapLogger.Info("Starting HTTP server", zap.String("address", address))

	// Graceful shutdown
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	// Start server in goroutine
	go func() {
		var err error
		if cfg.Server.TLS.Enabled {
			err = app.ListenTLS(address, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			err = app.Listen(address)
		}
		
		if err != nil {
			zapLogger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		zapLogger.Info("Received shutdown signal")
	case <-serverCtx.Done():
		zapLogger.Info("Server context cancelled")
	}

	// Graceful shutdown
	zapLogger.Info("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		zapLogger.Error("Server shutdown error", zap.Error(err))
	} else {
		zapLogger.Info("Server shutdown completed")
	}
}

// Implementation stubs - these would be in separate files

type AuditLoggerImpl struct {
	logger *zap.Logger
}

func (a *AuditLoggerImpl) LogSecurityEvent(event security.SecurityEvent) error {
	a.logger.Info("Security event",
		zap.String("type", event.Type),
		zap.String("plugin_id", event.PluginID.String()),
		zap.String("severity", event.Severity),
		zap.String("message", event.Message))
	return nil
}

func (a *AuditLoggerImpl) LogPluginEvent(ctx context.Context, event *services.AuditEvent) error {
	a.logger.Info("Plugin event",
		zap.String("type", event.Type),
		zap.String("plugin_id", event.PluginID.String()),
		zap.String("action", event.Action),
		zap.String("resource", event.Resource))
	return nil
}

type EventBusImpl struct {
	logger *zap.Logger
}

func (e *EventBusImpl) PublishPluginEvent(event runtime.PluginEvent) error {
	e.logger.Debug("Plugin runtime event",
		zap.String("type", event.Type),
		zap.String("plugin_id", event.PluginID.String()),
		zap.String("instance_id", event.InstanceID.String()))
	return nil
}

type NotificationServiceImpl struct {
	logger *zap.Logger
}

func (n *NotificationServiceImpl) SendPluginNotification(ctx context.Context, notification *services.PluginNotification) error {
	n.logger.Info("Plugin notification",
		zap.String("type", notification.Type),
		zap.String("plugin_id", notification.PluginID.String()),
		zap.String("message", notification.Message))
	return nil
}

// Repository implementations would be in separate files
func NewPluginRepository(db interface{}) services.PluginRepository {
	// Implementation
	return nil
}

func NewInstallationRepository(db interface{}) services.InstallationRepository {
	// Implementation
	return nil
}

func NewExecutionRepository(db interface{}) services.ExecutionRepository {
	// Implementation
	return nil
}

func NewRegistryRepository(db interface{}) services.RegistryRepository {
	// Implementation
	return nil
}

func NewRegistryCache(redis interface{}) services.RegistryCache {
	// Implementation
	return nil
}