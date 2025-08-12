package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	Environment  string
	
	// Database
	DatabaseURL string
	
	// Redis
	RedisURL      string
	RedisPassword string
	
	// JWT
	JWTSecret string
	
	// Admin Credentials
	AdminEmail    string
	AdminPassword string
	
	// CORS
	CORSOrigins string
	
	// Timeouts
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

func Load() *Config {
	// Critical: JWT_SECRET is required for security
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		panic("JWT_SECRET environment variable is required")
	}
	
	// Critical: Admin credentials are required for authentication
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		panic("ADMIN_EMAIL environment variable is required")
	}
	
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		panic("ADMIN_PASSWORD environment variable is required")
	}
	
	// Critical: DATABASE_URL is required
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		panic("DATABASE_URL environment variable is required")
	}
	
	return &Config{
		Port:        getEnv("PORT", "8001"),
		Environment: getEnv("ENVIRONMENT", "development"),
		
		// Database - no fallback for security
		DatabaseURL: databaseURL,
		
		// Redis
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		
		// JWT - no fallback for security
		JWTSecret: jwtSecret,
		
		// Admin Credentials - no fallback for security
		AdminEmail:    adminEmail,
		AdminPassword: adminPassword,
		
		// CORS - secure default with common frontend URLs
		CORSOrigins: getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:3003"),
		
		// Timeouts
		RequestTimeout:  getEnvAsDuration("REQUEST_TIMEOUT", 30*time.Second),
		ShutdownTimeout: getEnvAsDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := getEnv(key, "")
	if value, err := time.ParseDuration(valueStr); err == nil {
		return value
	}
	return defaultValue
}