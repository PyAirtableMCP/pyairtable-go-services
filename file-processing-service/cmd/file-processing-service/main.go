package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pyairtable/go-services/file-processing-service/internal/config"
	"github.com/pyairtable/go-services/file-processing-service/internal/handlers"
	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"github.com/pyairtable/go-services/file-processing-service/internal/processors"
	"github.com/pyairtable/go-services/file-processing-service/internal/storage"
	"github.com/pyairtable/go-services/file-processing-service/internal/websocket"
	"github.com/pyairtable/go-services/file-processing-service/pkg/logger"
	"github.com/pyairtable/go-services/file-processing-service/pkg/metrics"
)

// Application represents the main application
type Application struct {
	config          *models.Config
	logger          *logger.Logger
	metrics         *metrics.Manager
	storage         storage.Interface
	processor       *processors.ProcessingEngine
	progressTracker *websocket.ProgressTracker
	server          *http.Server
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.LoadDefault()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger
	logger, err := logger.New(cfg.Monitoring.LogLevel, cfg.Monitoring.LogFormat)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Initialize metrics
	metricsManager := metrics.New(cfg.Monitoring)

	// Initialize storage backend
	storageBackend, err := initializeStorage(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize progress tracker
	var progressTracker *websocket.ProgressTracker
	if cfg.WebSocket.Enabled {
		progressTracker = websocket.NewProgressTracker(&cfg.WebSocket)
	}

	// Initialize processing engine
	processingEngine := processors.NewProcessingEngine(
		&cfg.Processing,
		storageBackend,
		progressTracker,
	)

	// Create application
	app := &Application{
		config:          cfg,
		logger:          logger,
		metrics:         metricsManager,
		storage:         storageBackend,
		processor:       processingEngine,
		progressTracker: progressTracker,
	}

	// Start the application
	return app.start()
}

func (app *Application) start() error {
	app.logger.Info("Starting File Processing Service")

	// Health check storage
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := app.storage.Health(ctx); err != nil {
		app.logger.Error("Storage health check failed", "error", err)
		return fmt.Errorf("storage is not available: %w", err)
	}

	// Start processing engine
	if err := app.processor.Start(); err != nil {
		return fmt.Errorf("failed to start processing engine: %w", err)
	}

	// Setup HTTP router
	router := app.setupRouter()

	// Create HTTP server
	app.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", app.config.Server.Host, app.config.Server.Port),
		Handler:        router,
		ReadTimeout:    app.config.Server.ReadTimeout,
		WriteTimeout:   app.config.Server.WriteTimeout,
		IdleTimeout:    app.config.Server.IdleTimeout,
		MaxHeaderBytes: app.config.Server.MaxHeaderBytes,
	}

	// Start metrics server if enabled
	if app.config.Monitoring.Enabled {
		go app.startMetricsServer()
	}

	// Start server in goroutine
	go func() {
		app.logger.Info("Server starting", 
			"host", app.config.Server.Host,
			"port", app.config.Server.Port,
		)
		
		if err := app.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.logger.Error("Server failed", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	app.logger.Info("Shutting down server...")

	// Graceful shutdown
	return app.shutdown()
}

func (app *Application) setupRouter() *gin.Engine {
	// Set Gin mode
	if app.config.Monitoring.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(app.loggingMiddleware())
	router.Use(app.metricsMiddleware())
	router.Use(app.corsMiddleware())

	if app.config.Server.EnableCompression {
		// Add compression middleware if needed
	}

	// Setup routes
	app.setupRoutes(router)

	return router
}

func (app *Application) setupRoutes(router *gin.Engine) {
	// Health check
	router.GET("/health", app.healthHandler)
	router.GET("/ready", app.readinessHandler)

	// File processing API
	api := router.Group("/api/v1")
	{
		// Upload handlers
		uploadHandler := handlers.NewUploadHandler(
			app.storage,
			app.progressTracker,
			app.config,
			app.processor,
		)

		api.POST("/files/upload", uploadHandler.StreamingUploadHandler)
		api.POST("/files/upload/init", uploadHandler.ChunkedUploadInitHandler)
		api.PUT("/files/upload/:file_id/chunk", uploadHandler.ChunkedUploadChunkHandler)
		api.POST("/files/upload/:file_id/complete", uploadHandler.ChunkedUploadCompleteHandler)

		// Processing handlers
		processingHandler := handlers.NewProcessingHandler(
			app.processor,
			app.storage,
			app.progressTracker,
		)

		api.POST("/files/:file_id/process", processingHandler.ProcessFileHandler)
		api.GET("/files/:file_id/status", processingHandler.GetFileStatusHandler)
		api.GET("/files/:file_id/result", processingHandler.GetProcessingResultHandler)
		api.GET("/files/:file_id/download", processingHandler.DownloadFileHandler)

		// File management
		fileHandler := handlers.NewFileHandler(app.storage)
		api.GET("/files", fileHandler.ListFilesHandler)
		api.GET("/files/:file_id", fileHandler.GetFileInfoHandler)
		api.DELETE("/files/:file_id", fileHandler.DeleteFileHandler)

		// Processing queue management
		queueHandler := handlers.NewQueueHandler(app.processor)
		api.GET("/queue/status", queueHandler.GetQueueStatusHandler)
		api.GET("/queue/metrics", queueHandler.GetQueueMetricsHandler)
	}

	// WebSocket endpoint
	if app.config.WebSocket.Enabled {
		router.GET("/ws", gin.WrapH(http.HandlerFunc(app.progressTracker.HandleWebSocket)))
	}

	// Metrics endpoint
	if app.config.Monitoring.Enabled {
		router.GET(app.config.Monitoring.MetricsPath, gin.WrapH(app.metrics.Handler()))
	}

	// Debug endpoints (only in debug mode)
	if app.config.Monitoring.PprofEnabled {
		app.setupDebugRoutes(router)
	}
}

func (app *Application) setupDebugRoutes(router *gin.Engine) {
	debug := router.Group("/debug")
	{
		debug.GET("/pprof/*path", gin.WrapH(http.DefaultServeMux))
	}
}

func (app *Application) startMetricsServer() {
	metricsAddr := fmt.Sprintf(":%d", app.config.Monitoring.MetricsPort)
	app.logger.Info("Starting metrics server", "addr", metricsAddr)
	
	if err := http.ListenAndServe(metricsAddr, app.metrics.Handler()); err != nil {
		app.logger.Error("Metrics server failed", "error", err)
	}
}

func (app *Application) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), app.config.Server.ShutdownTimeout)
	defer cancel()

	// Shutdown HTTP server
	if err := app.server.Shutdown(ctx); err != nil {
		app.logger.Error("Server shutdown failed", "error", err)
		return err
	}

	// Stop processing engine
	if err := app.processor.Stop(); err != nil {
		app.logger.Error("Processing engine shutdown failed", "error", err)
	}

	// Close progress tracker
	if app.progressTracker != nil {
		if err := app.progressTracker.Close(); err != nil {
			app.logger.Error("Progress tracker shutdown failed", "error", err)
		}
	}

	app.logger.Info("Server stopped gracefully")
	return nil
}

// HTTP Handlers

func (app *Application) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "file-processing-service",
		"version": "1.0.0",
		"timestamp": time.Now().UTC(),
	})
}

func (app *Application) readinessHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Check storage health
	if err := app.storage.Health(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"ready": false,
			"error": "storage not available",
		})
		return
	}

	// Check processing engine
	if !app.processor.IsRunning() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"ready": false,
			"error": "processing engine not running",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ready": true,
		"checks": gin.H{
			"storage":    "ok",
			"processor":  "ok",
			"websocket":  app.config.WebSocket.Enabled,
		},
	})
}

// Middleware

func (app *Application) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		duration := time.Since(start)
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		app.logger.Info("HTTP Request",
			"method", c.Request.Method,
			"path", path,
			"status", statusCode,
			"duration", duration,
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		)
	}
}

func (app *Application) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		app.metrics.RecordHTTPRequest(
			c.Request.Method,
			c.FullPath(),
			c.Writer.Status(),
			duration,
		)
	}
}

func (app *Application) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Helper functions

func initializeStorage(cfg *models.Config) (storage.Interface, error) {
	switch cfg.Storage.Provider {
	case "s3":
		if cfg.Storage.S3Config == nil {
			return nil, fmt.Errorf("S3 configuration is required")
		}
		return storage.NewS3Storage(cfg.Storage.S3Config)
	case "minio":
		if cfg.Storage.MinIOConfig == nil {
			return nil, fmt.Errorf("MinIO configuration is required")
		}
		// Convert MinIO config to S3 config
		s3Config := &models.S3Config{
			Endpoint:        cfg.Storage.MinIOConfig.Endpoint,
			Region:          "us-east-1", // Default region for MinIO
			Bucket:          cfg.Storage.MinIOConfig.Bucket,
			AccessKeyID:     cfg.Storage.MinIOConfig.AccessKeyID,
			SecretAccessKey: cfg.Storage.MinIOConfig.SecretAccessKey,
			DisableSSL:      !cfg.Storage.MinIOConfig.UseSSL,
			ForcePathStyle:  true,
		}
		return storage.NewS3Storage(s3Config)
	case "local":
		// TODO: Implement local storage
		return nil, fmt.Errorf("local storage not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", cfg.Storage.Provider)
	}
}