// Package circuitbreaker provides circuit breaker pattern implementation for external service calls
// Task: immediate-12 - Implement circuit breaker pattern for external service calls
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// State represents the current state of the circuit breaker
type State int

const (
	// StateClosed - normal operation, requests are allowed
	StateClosed State = iota
	// StateHalfOpen - testing mode, limited requests allowed to test service recovery
	StateHalfOpen
	// StateOpen - failure mode, all requests are rejected
	StateOpen
)

func (s State) String() string {
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

// Config holds configuration for the circuit breaker
type Config struct {
	// Name of the circuit breaker for logging and metrics
	Name string
	// MaxRequests is the maximum number of requests allowed to pass through when the CircuitBreaker is half-open
	MaxRequests uint32
	// Interval is the cyclic period of the closed state for the CircuitBreaker to clear the internal Counts
	Interval time.Duration
	// Timeout is the period of the open state, after which the state becomes half-open
	Timeout time.Duration
	// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state
	ReadyToTrip func(counts Counts) bool
	// OnStateChange is called whenever the state changes
	OnStateChange func(name string, from State, to State)
	// IsSuccessful determines whether a result is successful or failure
	IsSuccessful func(err error) bool
	// Logger for circuit breaker events
	Logger *logrus.Logger
}

// Counts holds the numbers of requests and their outcomes
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
	onStateChange func(name string, from State, to State)
	logger        *logrus.Logger

	mutex      sync.Mutex
	state      State
	generation uint64
	counts     Counts
	expiry     time.Time
}

var (
	// ErrTooManyRequests is returned when the CB state is half open and the requests count is over the cb maxRequests
	ErrTooManyRequests = errors.New("circuit breaker: too many requests")
	// ErrOpenState is returned when the CB state is open
	ErrOpenState = errors.New("circuit breaker: open state")
)

// NewCircuitBreaker creates a new CircuitBreaker with the given Config
func NewCircuitBreaker(config Config) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:        config.Name,
		maxRequests: config.MaxRequests,
		interval:    config.Interval,
		timeout:     config.Timeout,
		logger:      config.Logger,
	}

	if cb.logger == nil {
		cb.logger = logrus.New()
	}

	if config.ReadyToTrip == nil {
		cb.readyToTrip = defaultReadyToTrip
	} else {
		cb.readyToTrip = config.ReadyToTrip
	}

	if config.IsSuccessful == nil {
		cb.isSuccessful = defaultIsSuccessful
	} else {
		cb.isSuccessful = config.IsSuccessful
	}

	if config.OnStateChange != nil {
		cb.onStateChange = config.OnStateChange
	}

	cb.toNewGeneration(time.Now())

	return cb
}

// defaultReadyToTrip - trip when there are more than 5 failures and failure rate is over 60%
func defaultReadyToTrip(counts Counts) bool {
	return counts.Requests >= 5 && counts.TotalFailures > 0 && 
		   float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6
}

// defaultIsSuccessful - consider nil error as success
func defaultIsSuccessful(err error) bool {
	return err == nil
}

// Name returns the name of the CircuitBreaker
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// State returns the current state of the CircuitBreaker
func (cb *CircuitBreaker) State() State {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns a copy of the internal Counts
func (cb *CircuitBreaker) Counts() Counts {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	return cb.counts
}

// Execute runs the given request if the CircuitBreaker accepts it
func (cb *CircuitBreaker) Execute(req func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		e := recover()
		if e != nil {
			cb.afterRequest(generation, false)
			panic(e)
		}
	}()

	err = req()
	cb.afterRequest(generation, cb.isSuccessful(err))
	return err
}

// ExecuteWithContext runs the given request if the CircuitBreaker accepts it, with context support
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, req func(ctx context.Context) error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		e := recover()
		if e != nil {
			cb.afterRequest(generation, false)
			panic(e)
		}
	}()

	// Check context cancellation
	select {
	case <-ctx.Done():
		cb.afterRequest(generation, false)
		return ctx.Err()
	default:
	}

	err = req(ctx)
	cb.afterRequest(generation, cb.isSuccessful(err))
	return err
}

// Call is a convenience method that wraps Execute for functions that return a result
func (cb *CircuitBreaker) Call(fn func() (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.Execute(func() error {
		var execErr error
		result, execErr = fn()
		return execErr
	})
	return result, err
}

// beforeRequest is called before a request
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		cb.logger.WithFields(logrus.Fields{
			"circuit_breaker": cb.name,
			"state":          state.String(),
		}).Debug("Circuit breaker rejected request - open state")
		return generation, ErrOpenState
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.maxRequests {
		cb.logger.WithFields(logrus.Fields{
			"circuit_breaker": cb.name,
			"state":          state.String(),
			"requests":       cb.counts.Requests,
			"max_requests":   cb.maxRequests,
		}).Debug("Circuit breaker rejected request - too many requests in half-open state")
		return generation, ErrTooManyRequests
	}

	cb.counts.onRequest()
	return generation, nil
}

// afterRequest is called after a request
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
func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	cb.counts.onSuccess()

	if state == StateHalfOpen {
		cb.logger.WithFields(logrus.Fields{
			"circuit_breaker":       cb.name,
			"consecutive_successes": cb.counts.ConsecutiveSuccesses,
			"max_requests":         cb.maxRequests,
		}).Debug("Successful request in half-open state")

		// If we've had enough successful requests, close the circuit
		if cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
			cb.setState(StateClosed, now)
		}
	}
}

// onFailure handles failed requests
func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	cb.counts.onFailure()

	switch state {
	case StateClosed:
		if cb.readyToTrip(cb.counts) {
			cb.setState(StateOpen, now)
		}
	case StateHalfOpen:
		cb.setState(StateOpen, now)
	}
}

// currentState returns the current state and generation
func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
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

// setState changes the state and calls the onStateChange callback
func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state

	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}

	cb.logger.WithFields(logrus.Fields{
		"circuit_breaker": cb.name,
		"from_state":     prev.String(),
		"to_state":       state.String(),
		"counts":         cb.counts,
	}).Info("Circuit breaker state changed")
}

// toNewGeneration resets counts and sets expiry
func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts.clear()

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

// Counts methods
func (c *Counts) onRequest() {
	c.Requests++
}

func (c *Counts) onSuccess() {
	c.TotalSuccesses++
	c.ConsecutiveSuccesses++
	c.ConsecutiveFailures = 0
}

func (c *Counts) onFailure() {
	c.TotalFailures++
	c.ConsecutiveFailures++
	c.ConsecutiveSuccesses = 0
}

func (c *Counts) clear() {
	c.Requests = 0
	c.TotalSuccesses = 0
	c.TotalFailures = 0
	c.ConsecutiveSuccesses = 0
	c.ConsecutiveFailures = 0
}

// String returns a string representation of Counts
func (c Counts) String() string {
	return fmt.Sprintf("Requests: %d, TotalSuccesses: %d, TotalFailures: %d, ConsecutiveSuccesses: %d, ConsecutiveFailures: %d",
		c.Requests, c.TotalSuccesses, c.TotalFailures, c.ConsecutiveSuccesses, c.ConsecutiveFailures)
}

// Utility functions for common configurations

// NewDefaultCircuitBreaker creates a circuit breaker with sensible defaults for most use cases
func NewDefaultCircuitBreaker(name string) *CircuitBreaker {
	return NewCircuitBreaker(Config{
		Name:        name,
		MaxRequests: 10,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.Requests >= 5 && counts.TotalFailures > 0 &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	})
}

// NewHTTPCircuitBreaker creates a circuit breaker optimized for HTTP calls
func NewHTTPCircuitBreaker(name string, logger *logrus.Logger) *CircuitBreaker {
	return NewCircuitBreaker(Config{
		Name:        name,
		MaxRequests: 5,
		Interval:    30 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			// Trip if we have at least 3 requests and failure rate is over 50%
			return counts.Requests >= 3 && counts.TotalFailures > 0 &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
		},
		IsSuccessful: func(err error) bool {
			// Consider nil error or specific non-critical errors as success
			return err == nil
		},
		Logger: logger,
		OnStateChange: func(name string, from State, to State) {
			if logger != nil {
				logger.WithFields(logrus.Fields{
					"circuit_breaker": name,
					"from_state":     from.String(),
					"to_state":       to.String(),
				}).Warn("HTTP Circuit breaker state changed")
			}
		},
	})
}

// NewDatabaseCircuitBreaker creates a circuit breaker optimized for database calls
func NewDatabaseCircuitBreaker(name string, logger *logrus.Logger) *CircuitBreaker {
	return NewCircuitBreaker(Config{
		Name:        name,
		MaxRequests: 3,
		Interval:    120 * time.Second, // Longer interval for DB recovery
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			// More conservative for DB - trip on 3 consecutive failures or 70% failure rate
			return (counts.ConsecutiveFailures >= 3) ||
				(counts.Requests >= 5 && counts.TotalFailures > 0 &&
					float64(counts.TotalFailures)/float64(counts.Requests) >= 0.7)
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
		Logger: logger,
		OnStateChange: func(name string, from State, to State) {
			if logger != nil {
				logger.WithFields(logrus.Fields{
					"circuit_breaker": name,
					"from_state":     from.String(),
					"to_state":       to.String(),
				}).Error("Database Circuit breaker state changed")
			}
		},
	})
}