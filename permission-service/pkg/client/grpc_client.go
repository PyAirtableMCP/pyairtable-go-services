package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	permissionv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/permission/v1"
)

// ClientConfig holds configuration for gRPC client
type ClientConfig struct {
	Address              string
	MaxRetries           int
	RetryInterval        time.Duration
	ConnectionTimeout    time.Duration
	KeepAliveTime        time.Duration
	KeepAliveTimeout     time.Duration
	PermitWithoutStream  bool
	MaxRecvMsgSize       int
	MaxSendMsgSize       int
	EnableCompression    bool
	EnableTracing        bool
}

// DefaultClientConfig returns default client configuration
func DefaultClientConfig(address string) *ClientConfig {
	return &ClientConfig{
		Address:              address,
		MaxRetries:           3,
		RetryInterval:        time.Second,
		ConnectionTimeout:    10 * time.Second,
		KeepAliveTime:        30 * time.Second,
		KeepAliveTimeout:     5 * time.Second,
		PermitWithoutStream:  true,
		MaxRecvMsgSize:       1024 * 1024 * 4, // 4MB
		MaxSendMsgSize:       1024 * 1024 * 4, // 4MB
		EnableCompression:    true,
		EnableTracing:        true,
	}
}

// PermissionClient is a gRPC client for the Permission Service
type PermissionClient struct {
	config *ClientConfig
	logger *zap.Logger
	
	// Connection pool
	connPool []*grpc.ClientConn
	poolSize int
	poolMu   sync.RWMutex
	
	// Round-robin index for load balancing
	roundRobinIndex int
	
	// Circuit breaker state
	circuitBreaker *CircuitBreaker
}

// CircuitBreaker implements a simple circuit breaker pattern
type CircuitBreaker struct {
	mu           sync.RWMutex
	state        CircuitState
	failures     int
	lastFailTime time.Time
	maxFailures  int
	timeout      time.Duration
}

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:       StateClosed,
		maxFailures: maxFailures,
		timeout:     timeout,
	}
}

// Call executes a function through the circuit breaker
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateOpen:
		if time.Since(cb.lastFailTime) > cb.timeout {
			cb.state = StateHalfOpen
		} else {
			return fmt.Errorf("circuit breaker is open")
		}
	}

	err := fn()
	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()
		if cb.failures >= cb.maxFailures {
			cb.state = StateOpen
		}
		return err
	}

	// Success
	cb.failures = 0
	cb.state = StateClosed
	return nil
}

// NewPermissionClient creates a new permission gRPC client with connection pooling
func NewPermissionClient(config *ClientConfig, logger *zap.Logger) (*PermissionClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	client := &PermissionClient{
		config:         config,
		logger:         logger,
		poolSize:       5, // Default pool size
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second),
	}

	// Initialize connection pool
	if err := client.initConnectionPool(); err != nil {
		return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
	}

	return client, nil
}

// initConnectionPool initializes the gRPC connection pool
func (c *PermissionClient) initConnectionPool() error {
	c.connPool = make([]*grpc.ClientConn, c.poolSize)

	for i := 0; i < c.poolSize; i++ {
		conn, err := c.createConnection()
		if err != nil {
			// Close any connections we've already created
			for j := 0; j < i; j++ {
				c.connPool[j].Close()
			}
			return fmt.Errorf("failed to create connection %d: %w", i, err)
		}
		c.connPool[i] = conn
	}

	c.logger.Info("gRPC connection pool initialized",
		zap.String("address", c.config.Address),
		zap.Int("pool_size", c.poolSize),
	)

	return nil
}

// createConnection creates a new gRPC connection
func (c *PermissionClient) createConnection() (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.ConnectionTimeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  100 * time.Millisecond,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   30 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                c.config.KeepAliveTime,
			Timeout:             c.config.KeepAliveTimeout,
			PermitWithoutStream: c.config.PermitWithoutStream,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(c.config.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(c.config.MaxSendMsgSize),
		),
	}

	if c.config.EnableCompression {
		opts = append(opts, grpc.WithCompressor("gzip"))
	}

	if c.config.EnableTracing {
		opts = append(opts,
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		)
	}

	return grpc.DialContext(ctx, c.config.Address, opts...)
}

// getConnection returns a connection from the pool using round-robin
func (c *PermissionClient) getConnection() *grpc.ClientConn {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()

	conn := c.connPool[c.roundRobinIndex]
	c.roundRobinIndex = (c.roundRobinIndex + 1) % c.poolSize

	// Check if connection is healthy
	if conn.GetState() == connectivity.TransientFailure || conn.GetState() == connectivity.Shutdown {
		c.logger.Warn("Connection is unhealthy, attempting to reconnect",
			zap.String("state", conn.GetState().String()),
		)
		
		// Try to create a new connection
		if newConn, err := c.createConnection(); err == nil {
			oldConn := c.connPool[c.roundRobinIndex]
			c.connPool[c.roundRobinIndex] = newConn
			go oldConn.Close() // Close old connection asynchronously
			conn = newConn
		}
	}

	return conn
}

// withRetry executes a function with retry logic and circuit breaker
func (c *PermissionClient) withRetry(ctx context.Context, fn func(permissionv1.PermissionServiceClient) error) error {
	return c.circuitBreaker.Call(func() error {
		var lastErr error
		
		for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
			conn := c.getConnection()
			client := permissionv1.NewPermissionServiceClient(conn)
			
			err := fn(client)
			if err == nil {
				return nil
			}
			
			lastErr = err
			
			// Check if error is retryable
			if !isRetryableError(err) {
				return err
			}
			
			if attempt < c.config.MaxRetries {
				c.logger.Warn("Request failed, retrying",
					zap.Error(err),
					zap.Int("attempt", attempt+1),
					zap.Int("max_retries", c.config.MaxRetries),
				)
				
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(c.config.RetryInterval):
					// Continue with retry
				}
			}
		}
		
		return lastErr
	})
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// CheckPermission checks if a user has permission for a specific action
func (c *PermissionClient) CheckPermission(ctx context.Context, req *permissionv1.CheckPermissionRequest) (*permissionv1.CheckPermissionResponse, error) {
	var response *permissionv1.CheckPermissionResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.CheckPermission(ctx, req)
		return err
	})
	
	return response, err
}

// BatchCheckPermission checks multiple permissions in a single request
func (c *PermissionClient) BatchCheckPermission(ctx context.Context, req *permissionv1.BatchCheckPermissionRequest) (*permissionv1.BatchCheckPermissionResponse, error) {
	var response *permissionv1.BatchCheckPermissionResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.BatchCheckPermission(ctx, req)
		return err
	})
	
	return response, err
}

// GrantPermission grants a permission to a user
func (c *PermissionClient) GrantPermission(ctx context.Context, req *permissionv1.GrantPermissionRequest) (*permissionv1.GrantPermissionResponse, error) {
	var response *permissionv1.GrantPermissionResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.GrantPermission(ctx, req)
		return err
	})
	
	return response, err
}

// RevokePermission revokes a permission from a user
func (c *PermissionClient) RevokePermission(ctx context.Context, req *permissionv1.RevokePermissionRequest) (*permissionv1.RevokePermissionResponse, error) {
	var response *permissionv1.RevokePermissionResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.RevokePermission(ctx, req)
		return err
	})
	
	return response, err
}

// ListPermissions lists permissions with optional filtering
func (c *PermissionClient) ListPermissions(ctx context.Context, req *permissionv1.ListPermissionsRequest) (*permissionv1.ListPermissionsResponse, error) {
	var response *permissionv1.ListPermissionsResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.ListPermissions(ctx, req)
		return err
	})
	
	return response, err
}

// CreateRole creates a new role
func (c *PermissionClient) CreateRole(ctx context.Context, req *permissionv1.CreateRoleRequest) (*permissionv1.CreateRoleResponse, error) {
	var response *permissionv1.CreateRoleResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.CreateRole(ctx, req)
		return err
	})
	
	return response, err
}

// AssignRole assigns a role to a user
func (c *PermissionClient) AssignRole(ctx context.Context, req *permissionv1.AssignRoleRequest) (*permissionv1.AssignRoleResponse, error) {
	var response *permissionv1.AssignRoleResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.AssignRole(ctx, req)
		return err
	})
	
	return response, err
}

// ListUserRoles lists roles assigned to a user
func (c *PermissionClient) ListUserRoles(ctx context.Context, req *permissionv1.ListUserRolesRequest) (*permissionv1.ListUserRolesResponse, error) {
	var response *permissionv1.ListUserRolesResponse
	
	err := c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		var err error
		response, err = client.ListUserRoles(ctx, req)
		return err
	})
	
	return response, err
}

// HealthCheck performs a health check
func (c *PermissionClient) HealthCheck(ctx context.Context) error {
	return c.withRetry(ctx, func(client permissionv1.PermissionServiceClient) error {
		_, err := client.HealthCheck(ctx, &emptypb.Empty{})
		return err
	})
}

// Close closes all connections in the pool
func (c *PermissionClient) Close() error {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()
	
	var errors []error
	for i, conn := range c.connPool {
		if err := conn.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close connection %d: %w", i, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("errors closing connections: %v", errors)
	}
	
	c.logger.Info("gRPC client connection pool closed")
	return nil
}

// GetConnectionStats returns statistics about the connection pool
func (c *PermissionClient) GetConnectionStats() map[string]interface{} {
	c.poolMu.RLock()
	defer c.poolMu.RUnlock()
	
	stats := make(map[string]interface{})
	stats["pool_size"] = c.poolSize
	stats["address"] = c.config.Address
	
	states := make(map[string]int)
	for _, conn := range c.connPool {
		state := conn.GetState().String()
		states[state]++
	}
	stats["connection_states"] = states
	
	c.circuitBreaker.mu.RLock()
	stats["circuit_breaker_state"] = c.circuitBreaker.state
	stats["circuit_breaker_failures"] = c.circuitBreaker.failures
	c.circuitBreaker.mu.RUnlock()
	
	return stats
}