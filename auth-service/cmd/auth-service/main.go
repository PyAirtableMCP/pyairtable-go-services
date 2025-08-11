package main

import (
    "context"
    "database/sql"
    "fmt"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/pyairtable-compose/auth-service/internal/config"
    "github.com/pyairtable-compose/auth-service/internal/handlers"
    "github.com/pyairtable-compose/auth-service/internal/middleware"
    "github.com/pyairtable-compose/auth-service/internal/repository/postgres"
    redisRepo "github.com/pyairtable-compose/auth-service/internal/repository/redis"
    "github.com/pyairtable-compose/auth-service/internal/services"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/joho/godotenv"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
    _ "github.com/lib/pq"
)

func main() {
    // Load environment variables
    _ = godotenv.Load()
    
    // Initialize logger
    zapLogger, err := zap.NewProduction()
    if err != nil {
        panic(fmt.Sprintf("Failed to initialize logger: %v", err))
    }
    defer zapLogger.Sync()
    
    // Load configuration
    cfg := config.Load()
    
    // Connect to PostgreSQL
    db, err := sql.Open("postgres", cfg.DatabaseURL)
    if err != nil {
        zapLogger.Fatal("Failed to connect to database", zap.Error(err))
    }
    defer db.Close()
    
    // Test database connection
    if err := db.Ping(); err != nil {
        zapLogger.Fatal("Failed to ping database", zap.Error(err))
    }
    
    // Connect to Redis
    opt, err := redis.ParseURL(cfg.RedisURL)
    if err != nil {
        zapLogger.Fatal("Failed to parse Redis URL", zap.Error(err))
    }
    if cfg.RedisPassword != "" {
        opt.Password = cfg.RedisPassword
    }
    
    redisClient := redis.NewClient(opt)
    defer redisClient.Close()
    
    // Test Redis connection
    ctx := context.Background()
    if err := redisClient.Ping(ctx).Err(); err != nil {
        zapLogger.Fatal("Failed to connect to Redis", zap.Error(err))
    }
    
    // Initialize repositories
    userRepo := postgres.NewUserRepository(db)
    tokenRepo := redisRepo.NewTokenRepository(redisClient)
    
    // Initialize services
    authService := services.NewAuthService(zapLogger, userRepo, tokenRepo, cfg.JWTSecret)
    
    // Initialize handlers and middleware
    authHandler := handlers.NewAuthHandler(zapLogger, authService)
    authMiddleware := middleware.NewAuthMiddleware(zapLogger, authService)
    
    // Create Fiber app
    app := fiber.New(fiber.Config{
        ReadTimeout:  cfg.RequestTimeout,
        WriteTimeout: cfg.RequestTimeout,
        BodyLimit:    1 * 1024 * 1024, // 1MB body limit
        ErrorHandler: func(c *fiber.Ctx, err error) error {
            code := fiber.StatusInternalServerError
            message := "Internal Server Error"
            
            if e, ok := err.(*fiber.Error); ok {
                code = e.Code
                message = e.Message
            }
            
            zapLogger.Error("Request error", 
                zap.Error(err),
                zap.String("path", c.Path()),
                zap.String("method", c.Method()),
                zap.String("ip", c.IP()))
            
            return c.Status(code).JSON(fiber.Map{
                "error": message,
            })
        },
    })
    
    // Basic middleware
    app.Use(recover.New())
    app.Use(logger.New(logger.Config{
        Format: "[${time}] ${status} - ${latency} ${method} ${path} ${ip} ${ua}\n",
    }))
    app.Use(cors.New(cors.Config{
        AllowOrigins: cfg.CORSOrigins,
        AllowHeaders: "Origin, Content-Type, Accept, Authorization",
        AllowMethods: "GET, POST, PUT, DELETE, OPTIONS, PATCH",
        AllowCredentials: false, // Security: disable credentials
    }))
    
    // Health check - simple alive check
    app.Get("/health", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{
            "status": "ok",
        })
    })
    
    // Auth routes
    auth := app.Group("/auth")
    auth.Post("/login", authHandler.Login)
    auth.Post("/register", authHandler.Register)
    auth.Post("/refresh", authHandler.RefreshToken)
    auth.Post("/logout", authHandler.Logout)
    auth.Post("/validate", authHandler.ValidateToken)
    
    // Protected routes (require auth)
    protected := auth.Group("", authMiddleware.RequireAuth)
    protected.Get("/me", authHandler.GetMe)
    protected.Put("/me", authHandler.UpdateMe)
    protected.Post("/change-password", authHandler.ChangePassword)
    
    // Start server
    go func() {
        zapLogger.Info("Starting Auth Service", zap.String("port", cfg.Port))
        if err := app.Listen(":" + cfg.Port); err != nil {
            zapLogger.Fatal("Failed to start server", zap.Error(err))
        }
    }()
    
    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    zapLogger.Info("Shutting down server...")
    
    // Graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
    defer cancel()
    
    if err := app.ShutdownWithContext(ctx); err != nil {
        zapLogger.Error("Server forced to shutdown", zap.Error(err))
    }
    
    zapLogger.Info("Server exited")
}
