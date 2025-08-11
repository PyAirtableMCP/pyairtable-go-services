package routing

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// StickySessionManager manages sticky sessions for load balancing
type StickySessionManager struct {
	sessions map[string]map[string]*SessionBinding // sessionID -> serviceName -> binding
	mu       sync.RWMutex
	logger   *zap.Logger
	ttl      time.Duration
}

// SessionBinding represents a binding between a session and a service instance
type SessionBinding struct {
	Instance  *ServiceInstance
	CreatedAt time.Time
	LastUsed  time.Time
}

func NewStickySessionManager(logger *zap.Logger) *StickySessionManager {
	ssm := &StickySessionManager{
		sessions: make(map[string]map[string]*SessionBinding),
		logger:   logger,
		ttl:      30 * time.Minute, // Default TTL
	}

	// Start cleanup goroutine
	go ssm.cleanupExpiredSessions()

	return ssm
}

func (ssm *StickySessionManager) SetInstance(sessionID, serviceName string, instance *ServiceInstance) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	if _, exists := ssm.sessions[sessionID]; !exists {
		ssm.sessions[sessionID] = make(map[string]*SessionBinding)
	}

	now := time.Now()
	ssm.sessions[sessionID][serviceName] = &SessionBinding{
		Instance:  instance,
		CreatedAt: now,
		LastUsed:  now,
	}

	ssm.logger.Debug("Sticky session created",
		zap.String("session_id", sessionID),
		zap.String("service", serviceName),
		zap.String("instance", instance.ID))
}

func (ssm *StickySessionManager) GetInstance(sessionID, serviceName string) *ServiceInstance {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	services, exists := ssm.sessions[sessionID]
	if !exists {
		return nil
	}

	binding, exists := services[serviceName]
	if !exists {
		return nil
	}

	// Check if session has expired
	if time.Since(binding.CreatedAt) > ssm.ttl {
		delete(services, serviceName)
		if len(services) == 0 {
			delete(ssm.sessions, sessionID)
		}
		return nil
	}

	// Update last used time
	binding.LastUsed = time.Now()

	ssm.logger.Debug("Sticky session retrieved",
		zap.String("session_id", sessionID),
		zap.String("service", serviceName),
		zap.String("instance", binding.Instance.ID))

	return binding.Instance
}

func (ssm *StickySessionManager) RemoveSession(sessionID, serviceName string) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	services, exists := ssm.sessions[sessionID]
	if !exists {
		return
	}

	delete(services, serviceName)
	if len(services) == 0 {
		delete(ssm.sessions, sessionID)
	}

	ssm.logger.Debug("Sticky session removed",
		zap.String("session_id", sessionID),
		zap.String("service", serviceName))
}

func (ssm *StickySessionManager) RemoveAllSessions(sessionID string) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	delete(ssm.sessions, sessionID)

	ssm.logger.Debug("All sticky sessions removed",
		zap.String("session_id", sessionID))
}

func (ssm *StickySessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for range ticker.C {
		ssm.performCleanup()
	}
}

func (ssm *StickySessionManager) performCleanup() {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	now := time.Now()
	expiredSessions := 0
	expiredBindings := 0

	for sessionID, services := range ssm.sessions {
		for serviceName, binding := range services {
			if now.Sub(binding.CreatedAt) > ssm.ttl {
				delete(services, serviceName)
				expiredBindings++
			}
		}

		if len(services) == 0 {
			delete(ssm.sessions, sessionID)
			expiredSessions++
		}
	}

	if expiredSessions > 0 || expiredBindings > 0 {
		ssm.logger.Debug("Cleaned up expired sticky sessions",
			zap.Int("expired_sessions", expiredSessions),
			zap.Int("expired_bindings", expiredBindings))
	}
}

func (ssm *StickySessionManager) GetStats() map[string]interface{} {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()

	totalBindings := 0
	for _, services := range ssm.sessions {
		totalBindings += len(services)
	}

	return map[string]interface{}{
		"total_sessions": len(ssm.sessions),
		"total_bindings": totalBindings,
		"ttl_minutes":    ssm.ttl.Minutes(),
	}
}