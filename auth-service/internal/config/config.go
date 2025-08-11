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
	
	// CORS
	CORSOrigins string
	
	// Timeouts
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8001"),
		Environment: getEnv("ENVIRONMENT", "development"),
		
		// Database
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5432/pyairtable_auth?sslmode=require"),
		
		// Redis
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		
		// JWT
		JWTSecret: getEnv("JWT_SECRET", "your-secret-key-here"),
		
		// CORS
		CORSOrigins: getEnv("CORS_ORIGINS", "*"),
		
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