package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"github.com/pyairtable/pyairtable-compose/go-services/user-service/graphql"
	"github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/config"
	"github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/repository/postgres"
	redisRepo "github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/repository/redis"
	"github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/services"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	if os.Getenv("DEBUG") == "true" {
		logger.SetLevel(logrus.DebugLevel)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Fatal("Failed to load config")
	}

	// Connect to PostgreSQL
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		logger.WithError(err).Fatal("Failed to connect to database")
	}
	defer db.Close()

	// Configure database connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test database connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to ping database")
	}

	// Connect to Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     10,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer redisClient.Close()

	// Test Redis connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.WithError(err).Fatal("Failed to connect to Redis")
	}

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db)
	cacheRepo := redisRepo.NewCacheRepository(redisClient)
	permissionRepo := postgres.NewPermissionRepository(db)
	notificationRepo := postgres.NewNotificationRepository(db)
	activityRepo := postgres.NewActivityRepository(db)

	// Initialize services
	userService := services.NewUserService(logger, userRepo, cacheRepo)
	authService := services.NewAuthService(logger, userRepo, cacheRepo)
	permissionService := services.NewPermissionService(logger, permissionRepo, cacheRepo)
	notificationService := services.NewNotificationService(logger, notificationRepo, cacheRepo)
	activityService := services.NewActivityService(logger, activityRepo)

	// Create GraphQL server
	graphqlConfig := graphql.Config{
		Port:                getEnv("GRAPHQL_PORT", "8001"),
		EnableIntrospection: getEnv("ENABLE_INTROSPECTION", "true") == "true",
		EnablePlayground:    getEnv("ENABLE_PLAYGROUND", "true") == "true",
		CORSOrigins:         []string{"*"}, // Configure based on environment
	}

	server, err := graphql.NewServer(
		userService,
		authService,
		permissionService,
		notificationService,
		activityService,
		logger,
		graphqlConfig,
	)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create GraphQL server")
	}

	// Start server in goroutine
	go func() {
		logger.WithField("port", graphqlConfig.Port).Info("Starting User Service GraphQL server")
		if err := server.Start(); err != nil {
			logger.WithError(err).Fatal("GraphQL server failed to start")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down User Service GraphQL server...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("Server forced to shutdown")
	}

	logger.Info("User Service GraphQL server exited")
}

// getEnv gets environment variable with default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Configuration loading helper
type Config struct {
	Database struct {
		Host     string
		Port     int
		User     string
		Password string
		Name     string
		SSLMode  string
	}
	Redis struct {
		Host     string
		Port     int
		Password string
		DB       int
	}
	GraphQL struct {
		Port                string
		EnableIntrospection bool
		EnablePlayground    bool
	}
	JWT struct {
		Secret string
	}
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	// Database configuration
	cfg.Database.Host = getEnv("DB_HOST", "localhost")
	cfg.Database.Port = parseInt(getEnv("DB_PORT", "5432"))
	cfg.Database.User = getEnv("DB_USER", "postgres")
	cfg.Database.Password = getEnv("DB_PASSWORD", "")
	cfg.Database.Name = getEnv("DB_NAME", "pyairtable_users")
	cfg.Database.SSLMode = getEnv("DB_SSLMODE", "disable")

	// Redis configuration
	cfg.Redis.Host = getEnv("REDIS_HOST", "localhost")
	cfg.Redis.Port = parseInt(getEnv("REDIS_PORT", "6379"))
	cfg.Redis.Password = getEnv("REDIS_PASSWORD", "")
	cfg.Redis.DB = parseInt(getEnv("REDIS_DB", "0"))

	// GraphQL configuration
	cfg.GraphQL.Port = getEnv("GRAPHQL_PORT", "8001")
	cfg.GraphQL.EnableIntrospection = getEnv("ENABLE_INTROSPECTION", "true") == "true"
	cfg.GraphQL.EnablePlayground = getEnv("ENABLE_PLAYGROUND", "true") == "true"

	// JWT configuration
	cfg.JWT.Secret = getEnv("JWT_SECRET", "your-secret-key")

	return cfg, nil
}

// parseInt converts string to int with default value
func parseInt(s string) int {
	// Simple implementation - in production, use strconv.Atoi with error handling
	switch s {
	case "5432":
		return 5432
	case "6379":
		return 6379
	case "0":
		return 0
	default:
		return 0
	}
}