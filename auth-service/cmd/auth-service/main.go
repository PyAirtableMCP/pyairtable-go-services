package main

import (
    "context"
    "database/sql"
    "log"
    "strings"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    fiberLogger "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
    _ "github.com/lib/pq"
    
    "github.com/pyairtable-compose/auth-service/internal/config"
    "github.com/pyairtable-compose/auth-service/internal/handlers"
    "github.com/pyairtable-compose/auth-service/internal/repository/postgres"
    redisRepo "github.com/pyairtable-compose/auth-service/internal/repository/redis"
    "github.com/pyairtable-compose/auth-service/internal/services"
)

// Global config instance
var cfg *config.Config

// parseCORSOrigins parses comma-separated CORS origins and validates them
func parseCORSOrigins(originsStr string) string {
    if originsStr == "" {
        return "http://localhost:3000"
    }
    
    // Split by comma and clean up
    origins := strings.Split(originsStr, ",")
    validOrigins := make([]string, 0, len(origins))
    
    for _, origin := range origins {
        origin = strings.TrimSpace(origin)
        if origin != "" && origin != "*" {
            // Validate origin format (basic validation)
            if strings.HasPrefix(origin, "http://") || strings.HasPrefix(origin, "https://") {
                validOrigins = append(validOrigins, origin)
            } else {
                log.Printf("Warning: Invalid origin format ignored: %s", origin)
            }
        } else if origin == "*" {
            log.Printf("Security Warning: Wildcard origin '*' ignored for security")
        }
    }
    
    if len(validOrigins) == 0 {
        log.Printf("No valid origins found, using secure default: http://localhost:3000")
        return "http://localhost:3000"
    }
    
    result := strings.Join(validOrigins, ",")
    log.Printf("CORS origins configured: %s", result)
    return result
}


func main() {
    // Load configuration with validation
    cfg = config.Load()
    log.Printf("Configuration loaded successfully")
    
    // Initialize logger
    logger, err := zap.NewProduction()
    if err != nil {
        log.Fatal("Failed to initialize logger:", err)
    }
    defer logger.Sync()
    
    // Initialize database connection
    db, err := sql.Open("postgres", cfg.DatabaseURL)
    if err != nil {
        logger.Fatal("Failed to connect to database", zap.Error(err))
    }
    defer db.Close()
    
    // Test database connection
    if err = db.Ping(); err != nil {
        logger.Fatal("Failed to ping database", zap.Error(err))
    }
    logger.Info("Connected to database successfully")
    
    // Initialize Redis client
    // Parse Redis URL to extract address
    redisAddr := strings.TrimPrefix(cfg.RedisURL, "redis://")
    redisClient := redis.NewClient(&redis.Options{
        Addr:     redisAddr,
        Password: cfg.RedisPassword,
        DB:       0,
    })
    
    // Test Redis connection
    _, err = redisClient.Ping(context.Background()).Result()
    if err != nil {
        logger.Fatal("Failed to connect to Redis", zap.Error(err))
    }
    logger.Info("Connected to Redis successfully")
    
    // Initialize repositories
    userRepo := postgres.NewUserRepository(db)
    tokenRepo := redisRepo.NewTokenRepository(redisClient)
    
    // Initialize services
    authService := services.NewAuthService(logger, userRepo, tokenRepo, cfg.JWTSecret)
    
    // Initialize handlers
    authHandler := handlers.NewAuthHandler(logger, authService)
    
    // Create Fiber app
    app := fiber.New(fiber.Config{
        ErrorHandler: func(c *fiber.Ctx, err error) error {
            code := fiber.StatusInternalServerError
            message := "Internal Server Error"
            
            if e, ok := err.(*fiber.Error); ok {
                code = e.Code
                message = e.Message
            }
            
            logger.Error("HTTP Error", 
                zap.Error(err), 
                zap.String("path", c.Path()), 
                zap.String("method", c.Method()),
                zap.Int("status", code))
            
            return c.Status(code).JSON(fiber.Map{
                "error": message,
            })
        },
    })

    // Middleware
    app.Use(recover.New())
    app.Use(fiberLogger.New())
    app.Use(cors.New(cors.Config{
        AllowOrigins:     parseCORSOrigins(cfg.CORSOrigins),
        AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
        AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS, PATCH",
        AllowCredentials: true,
    }))

    // Health check endpoint
    app.Get("/health", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{
            "status": "ok",
            "service": "auth-service",
            "timestamp": time.Now().Unix(),
        })
    })

    // Auth endpoints
    app.Post("/auth/login", authHandler.Login)
    app.Post("/auth/register", authHandler.Register)
    app.Post("/auth/refresh", authHandler.RefreshToken)
    app.Post("/auth/logout", authHandler.Logout)
    app.Get("/auth/me", authHandler.GetMe)
    app.Put("/auth/me", authHandler.UpdateMe)
    app.Post("/auth/change-password", authHandler.ChangePassword)
    app.Post("/auth/validate", authHandler.ValidateToken)

    // Legacy endpoint for backward compatibility
    app.Post("/auth/login-skeleton", authHandler.LoginSkeleton)

    // Start server
    logger.Info("Starting auth service", 
        zap.String("port", cfg.Port),
        zap.String("environment", cfg.Environment))
    
    if err := app.Listen(":" + cfg.Port); err != nil {
        logger.Fatal("Failed to start server", zap.Error(err))
    }
}

