package main

import (
    "log"
    "os"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    "github.com/golang-jwt/jwt/v5"
)

const (
    // Hardcoded credentials as requested
    ADMIN_EMAIL    = "admin@test.com"
    ADMIN_PASSWORD = "admin123"
    // Hardcoded JWT secret (in production, use env var)
    JWT_SECRET = "your-super-secret-jwt-key"
    // Server port
    PORT = "8080"
)

type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type LoginResponse struct {
    Token string    `json:"token"`
    User  UserInfo `json:"user"`
}

type UserInfo struct {
    ID    string `json:"id"`
    Email string `json:"email"`
}

type Claims struct {
    Email  string `json:"email"`
    UserID string `json:"user_id"`
    jwt.RegisteredClaims
}

func main() {
    // Create Fiber app
    app := fiber.New(fiber.Config{
        ErrorHandler: func(c *fiber.Ctx, err error) error {
            code := fiber.StatusInternalServerError
            message := "Internal Server Error"
            
            if e, ok := err.(*fiber.Error); ok {
                code = e.Code
                message = e.Message
            }
            
            log.Printf("Error: %v - Path: %s - Method: %s", err, c.Path(), c.Method())
            
            return c.Status(code).JSON(fiber.Map{
                "error": message,
            })
        },
    })

    // Middleware
    app.Use(recover.New())
    app.Use(logger.New())
    app.Use(cors.New(cors.Config{
        AllowOrigins:     "*",
        AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
        AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS, PATCH",
        AllowCredentials: false,
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
    app.Post("/auth/login", handleLogin)

    // Start server
    port := PORT
    if envPort := os.Getenv("PORT"); envPort != "" {
        port = envPort
    }

    log.Printf("Starting simple auth service on port %s", port)
    log.Printf("Hardcoded login: %s / %s", ADMIN_EMAIL, ADMIN_PASSWORD)
    
    if err := app.Listen(":" + port); err != nil {
        log.Fatal("Failed to start server:", err)
    }
}

func handleLogin(c *fiber.Ctx) error {
    var req LoginRequest
    
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "Invalid request body",
        })
    }

    // Validate required fields
    if req.Email == "" || req.Password == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "Email and password are required",
        })
    }

    // Check hardcoded credentials
    if req.Email != ADMIN_EMAIL || req.Password != ADMIN_PASSWORD {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "error": "Invalid credentials",
        })
    }

    // Generate JWT token
    token, err := generateJWT(req.Email)
    if err != nil {
        log.Printf("Failed to generate JWT: %v", err)
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "error": "Failed to generate token",
        })
    }

    // Return successful response
    return c.JSON(LoginResponse{
        Token: token,
        User: UserInfo{
            ID:    "1",
            Email: req.Email,
        },
    })
}

func generateJWT(email string) (string, error) {
    // Create claims
    claims := Claims{
        Email:  email,
        UserID: "1", // Hardcoded user ID
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24 hours
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            NotBefore: jwt.NewNumericDate(time.Now()),
            Issuer:    "pyairtable-auth-service",
            Subject:   email,
        },
    }

    // Create token
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

    // Sign token with secret
    tokenString, err := token.SignedString([]byte(JWT_SECRET))
    if err != nil {
        return "", err
    }

    return tokenString, nil
}