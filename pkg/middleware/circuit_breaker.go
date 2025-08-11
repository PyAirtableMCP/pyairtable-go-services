package middleware

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateHalfOpen:
		return "HALF_OPEN"
	case StateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig contains configuration for the circuit breaker
type CircuitBreakerConfig struct {
	MaxRequests          uint32        // Maximum requests allowed in half-open state
	Interval             time.Duration // Time period for collecting statistics
	Timeout              time.Duration // Time to wait before switching from open to half-open
	ReadyToTrip          func(counts Counts) bool // Function to determine when to trip
	OnStateChange        func(name string, from CircuitState, to CircuitState) // Callback for state changes
	IsSuccessful         func(err error) bool // Function to determine if request was successful
	ShouldTrip           func(counts Counts) bool // Function to determine if circuit should trip
	MaxConsecutiveFailures uint32 // Maximum consecutive failures before tripping
}

// Counts holds the statistics for the circuit breaker
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name          string
	maxRequests   uint32
	interval      time.Duration
	timeout       time.Duration
	readyToTrip   func(counts Counts) bool
	isSuccessful  func(err error) bool
	onStateChange func(name string, from CircuitState, to CircuitState)

	mutex      sync.Mutex
	state      CircuitState
	generation uint64
	counts     Counts
	expiry     time.Time
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:        name,
		maxRequests: config.MaxRequests,
		interval:    config.Interval,
		timeout:     config.Timeout,
		readyToTrip: config.ReadyToTrip,
		isSuccessful: config.IsSuccessful,
		onStateChange: config.OnStateChange,
	}

	// Set default values
	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}
	if cb.interval <= 0 {
		cb.interval = 60 * time.Second
	}
	if cb.timeout <= 0 {
		cb.timeout = 60 * time.Second
	}
	if cb.readyToTrip == nil {
		cb.readyToTrip = func(counts Counts) bool {
			return counts.ConsecutiveFailures > 5
		}
	}
	if cb.isSuccessful == nil {
		cb.isSuccessful = func(err error) bool {
			return err == nil
		}
	}

	cb.toNewGeneration(time.Now())
	return cb
}

// Execute executes the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	result := fn()
	cb.afterRequest(generation, cb.isSuccessful(result))
	return result
}

// beforeRequest is called before executing a request
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, fmt.Errorf("circuit breaker '%s' is OPEN", cb.name)
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.maxRequests {
		return generation, fmt.Errorf("circuit breaker '%s' is HALF_OPEN and max requests exceeded", cb.name)
	}

	cb.counts.Requests++
	return generation, nil
}

// afterRequest is called after executing a request
func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

// onSuccess handles successful requests
func (cb *CircuitBreaker) onSuccess(state CircuitState, now time.Time) {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0

	if state == StateHalfOpen {
		cb.setState(StateClosed, now)
	}
}

// onFailure handles failed requests
func (cb *CircuitBreaker) onFailure(state CircuitState, now time.Time) {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccresses = 0

	if cb.readyToTrip(cb.counts) {
		cb.setState(StateOpen, now)
	}
}

// currentState returns the current state of the circuit breaker
func (cb *CircuitBreaker) currentState(now time.Time) (CircuitState, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

// setState changes the state of the circuit breaker
func (cb *CircuitBreaker) setState(state CircuitState, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}

	log.Printf("Circuit breaker '%s' changed state from %s to %s", cb.name, prev, state)
}

// toNewGeneration starts a new generation
func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts = Counts{}

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitState {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns the current counts
func (cb *CircuitBreaker) Counts() Counts {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	return cb.counts
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mutex    sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (cbm *CircuitBreakerManager) GetOrCreate(name string, config CircuitBreakerConfig) *CircuitBreaker {
	cbm.mutex.RLock()
	if cb, exists := cbm.breakers[name]; exists {
		cbm.mutex.RUnlock()
		return cb
	}
	cbm.mutex.RUnlock()

	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()

	// Check again in case another goroutine created it
	if cb, exists := cbm.breakers[name]; exists {
		return cb
	}

	cb := NewCircuitBreaker(name, config)
	cbm.breakers[name] = cb
	return cb
}

// GetStats returns statistics for all circuit breakers
func (cbm *CircuitBreakerManager) GetStats() map[string]map[string]interface{} {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	stats := make(map[string]map[string]interface{})
	for name, cb := range cbm.breakers {
		counts := cb.Counts()
		stats[name] = map[string]interface{}{
			"state":                  cb.State().String(),
			"requests":               counts.Requests,
			"total_successes":        counts.TotalSuccesses,
			"total_failures":         counts.TotalFailures,
			"consecutive_successes":  counts.ConsecutiveSuccesses,
			"consecutive_failures":   counts.ConsecutiveFailures,
		}
	}
	return stats
}

// Global circuit breaker manager instance
var globalCircuitBreakerManager = NewCircuitBreakerManager()

// CircuitBreakerUnaryInterceptor returns a gRPC unary interceptor with circuit breaker functionality
func CircuitBreakerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		serviceName := info.FullMethod

		config := CircuitBreakerConfig{
			MaxRequests: 10,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts Counts) bool {
				failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
				return counts.Requests >= 3 && failureRatio >= 0.6
			},
			IsSuccessful: func(err error) bool {
				if err == nil {
					return true
				}
				// Consider certain errors as successful (e.g., validation errors)
				st, ok := status.FromError(err)
				if !ok {
					return false
				}
				return st.Code() == codes.InvalidArgument || st.Code() == codes.NotFound
			},
			OnStateChange: func(name string, from CircuitState, to CircuitState) {
				log.Printf("Circuit breaker '%s' state changed: %s -> %s", name, from, to)
			},
		}

		cb := globalCircuitBreakerManager.GetOrCreate(serviceName, config)

		var resp interface{}
		err := cb.Execute(func() error {
			var execErr error
			resp, execErr = handler(ctx, req)
			return execErr
		})

		if err != nil {
			// If circuit breaker is open, return service unavailable
			if cb.State() == StateOpen {
				return nil, status.Errorf(codes.Unavailable, "service temporarily unavailable due to circuit breaker")
			}
		}

		return resp, err
	}
}

// GetCircuitBreakerStats returns statistics for all circuit breakers
func GetCircuitBreakerStats() map[string]map[string]interface{} {
	return globalCircuitBreakerManager.GetStats()
}