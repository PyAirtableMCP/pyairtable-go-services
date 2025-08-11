package middleware

import (
	"sync"
	"time"
	
	"github.com/gofiber/fiber/v2"
)

type RateLimiter struct {
	perMinute int
	clients   map[string]*rateLimitClient
	mu        sync.Mutex
}

type rateLimitClient struct {
	count    int
	lastSeen time.Time
}

func NewRateLimiter(perMinute int) *RateLimiter {
	rl := &RateLimiter{
		perMinute: perMinute,
		clients:   make(map[string]*rateLimitClient),
	}
	
	// Start cleanup goroutine
	go rl.cleanupExpired()
	
	return rl
}

func (rl *RateLimiter) Limit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		
		rl.mu.Lock()
		defer rl.mu.Unlock()
		
		now := time.Now()
		
		if client, exists := rl.clients[ip]; exists {
			// Reset count if a minute has passed
			if now.Sub(client.lastSeen) > time.Minute {
				client.count = 1
				client.lastSeen = now
			} else {
				client.count++
				if client.count > rl.perMinute {
					return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
						"error": "Rate limit exceeded",
					})
				}
			}
		} else {
			rl.clients[ip] = &rateLimitClient{
				count:    1,
				lastSeen: now,
			}
		}
		
		return c.Next()
	}
}

func (rl *RateLimiter) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, client := range rl.clients {
			if now.Sub(client.lastSeen) > 10*time.Minute {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}