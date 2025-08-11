package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"pyairtable-go/pkg/config"
	"pyairtable-go/pkg/metrics"
	"pyairtable-go/pkg/middleware"
)

// OptimizedGRPCServer provides a high-performance gRPC server with optimizations
type OptimizedGRPCServer struct {
	server      *grpc.Server
	listener    net.Listener
	config      *config.Config
	healthSrv   *health.Server
	metrics     *metrics.GRPCMetrics
}

// GRPCServerOptions contains configuration options for the gRPC server
type GRPCServerOptions struct {
	MaxRecvMsgSize          int
	MaxSendMsgSize          int
	MaxConcurrentStreams    uint32
	ConnectionTimeout       time.Duration
	KeepaliveTime           time.Duration
	KeepaliveTimeout        time.Duration
	MaxConnectionIdle       time.Duration
	MaxConnectionAge        time.Duration
	MaxConnectionAgeGrace   time.Duration
	EnableReflection        bool
	EnableHealthCheck       bool
	EnableTLS               bool
	TLSCertFile             string
	TLSKeyFile              string
}

// NewOptimizedGRPCServer creates a new optimized gRPC server
func NewOptimizedGRPCServer(cfg *config.Config, opts *GRPCServerOptions) (*OptimizedGRPCServer, error) {
	// Set default options if not provided
	if opts == nil {
		opts = &GRPCServerOptions{
			MaxRecvMsgSize:       4 * 1024 * 1024,  // 4MB
			MaxSendMsgSize:       4 * 1024 * 1024,  // 4MB
			MaxConcurrentStreams: 1000,
			ConnectionTimeout:    120 * time.Second,
			KeepaliveTime:        30 * time.Second,
			KeepaliveTimeout:     5 * time.Second,
			MaxConnectionIdle:    15 * time.Minute,
			MaxConnectionAge:     30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Second,
			EnableReflection:     cfg.IsDevelopment(),
			EnableHealthCheck:    true,
			EnableTLS:           false,
		}
	}

	// Create listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPC.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	// Initialize metrics
	grpcMetrics := metrics.NewGRPCMetrics()

	// Configure server options with performance optimizations
	serverOpts := []grpc.ServerOption{
		// Message size limits
		grpc.MaxRecvMsgSize(opts.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(opts.MaxSendMsgSize),
		grpc.MaxConcurrentStreams(opts.MaxConcurrentStreams),

		// Connection timeout
		grpc.ConnectionTimeout(opts.ConnectionTimeout),

		// Keepalive settings for better connection management
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    opts.KeepaliveTime,
			Timeout: opts.KeepaliveTimeout,
		}),
		
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),

		// Add interceptors for monitoring and middleware
		grpc.ChainUnaryInterceptor(
			grpcMetrics.UnaryServerInterceptor(),
			middleware.LoggingUnaryInterceptor(),
			middleware.AuthUnaryInterceptor(cfg),
			middleware.RateLimitUnaryInterceptor(cfg),
			middleware.CircuitBreakerUnaryInterceptor(),
		),
		
		grpc.ChainStreamInterceptor(
			grpcMetrics.StreamServerInterceptor(),
			middleware.LoggingStreamInterceptor(),
			middleware.AuthStreamInterceptor(cfg),
			middleware.RateLimitStreamInterceptor(cfg),
		),
	}

	// Add TLS credentials if enabled
	if opts.EnableTLS {
		creds, err := loadTLSCredentials(opts.TLSCertFile, opts.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	// Create gRPC server
	server := grpc.NewServer(serverOpts...)

	// Initialize health server if enabled
	var healthSrv *health.Server
	if opts.EnableHealthCheck {
		healthSrv = health.NewServer()
		grpc_health_v1.RegisterHealthServer(server, healthSrv)
	}

	// Enable reflection for development
	if opts.EnableReflection {
		reflection.Register(server)
		log.Println("gRPC reflection enabled")
	}

	optimizedServer := &OptimizedGRPCServer{
		server:    server,
		listener:  listener,
		config:    cfg,
		healthSrv: healthSrv,
		metrics:   grpcMetrics,
	}

	log.Printf("Optimized gRPC server created on port %d", cfg.GRPC.Port)
	return optimizedServer, nil
}

// Start starts the gRPC server
func (s *OptimizedGRPCServer) Start() error {
	log.Printf("Starting optimized gRPC server on %s", s.listener.Addr().String())
	
	// Set initial health status
	if s.healthSrv != nil {
		s.healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}

	return s.server.Serve(s.listener)
}

// GracefulStop gracefully stops the gRPC server
func (s *OptimizedGRPCServer) GracefulStop() {
	log.Println("Gracefully stopping gRPC server...")
	
	// Set health status to not serving
	if s.healthSrv != nil {
		s.healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}

	// Graceful stop with timeout
	stopCh := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopCh)
	}()

	// Wait for graceful stop or force stop after timeout
	select {
	case <-stopCh:
		log.Println("gRPC server stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Force stopping gRPC server after timeout")
		s.server.Stop()
	}
}

// RegisterService registers a service with the gRPC server
func (s *OptimizedGRPCServer) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	s.server.RegisterService(desc, impl)
	log.Printf("Registered gRPC service: %s", desc.ServiceName)
}

// GetMetrics returns the gRPC metrics collector
func (s *OptimizedGRPCServer) GetMetrics() *metrics.GRPCMetrics {
	return s.metrics
}

// loadTLSCredentials loads TLS credentials from certificate files
func loadTLSCredentials(certFile, keyFile string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	return credentials.NewTLS(config), nil
}

// OptimizedGRPCClient provides a high-performance gRPC client with connection pooling
type OptimizedGRPCClient struct {
	connections map[string]*grpc.ClientConn
	config      *config.Config
	metrics     *metrics.GRPCMetrics
}

// GRPCClientOptions contains configuration options for the gRPC client
type GRPCClientOptions struct {
	MaxRecvMsgSize       int
	MaxSendMsgSize       int
	KeepAliveTime        time.Duration
	KeepAliveTimeout     time.Duration
	PermitWithoutStream  bool
	MaxRetryAttempts     int
	InitialConnWindow    int32
	InitialWindowSize    int32
	EnableLoadBalancing  bool
	EnableTLS            bool
	ServerName           string
	Insecure             bool
}

// NewOptimizedGRPCClient creates a new optimized gRPC client
func NewOptimizedGRPCClient(cfg *config.Config, opts *GRPCClientOptions) *OptimizedGRPCClient {
	if opts == nil {
		opts = &GRPCClientOptions{
			MaxRecvMsgSize:      4 * 1024 * 1024,  // 4MB
			MaxSendMsgSize:      4 * 1024 * 1024,  // 4MB
			KeepAliveTime:       30 * time.Second,
			KeepAliveTimeout:    5 * time.Second,
			PermitWithoutStream: true,
			MaxRetryAttempts:    3,
			InitialConnWindow:   64 * 1024,       // 64KB
			InitialWindowSize:   64 * 1024,       // 64KB
			EnableLoadBalancing: true,
			Insecure:           true,
		}
	}

	return &OptimizedGRPCClient{
		connections: make(map[string]*grpc.ClientConn),
		config:      cfg,
		metrics:     metrics.NewGRPCMetrics(),
	}
}

// GetConnection returns an optimized connection to the specified service
func (c *OptimizedGRPCClient) GetConnection(serviceName, address string, opts *GRPCClientOptions) (*grpc.ClientConn, error) {
	// Check if connection already exists
	if conn, exists := c.connections[serviceName]; exists {
		// Check if connection is still healthy
		if conn.GetState().String() == "READY" {
			return conn, nil
		}
		// Close unhealthy connection
		conn.Close()
		delete(c.connections, serviceName)
	}

	// Set default options if not provided
	if opts == nil {
		opts = &GRPCClientOptions{
			MaxRecvMsgSize:      4 * 1024 * 1024,
			MaxSendMsgSize:      4 * 1024 * 1024,
			KeepAliveTime:       30 * time.Second,
			KeepAliveTimeout:    5 * time.Second,
			PermitWithoutStream: true,
			MaxRetryAttempts:    3,
			InitialConnWindow:   64 * 1024,
			InitialWindowSize:   64 * 1024,
			EnableLoadBalancing: true,
			Insecure:           true,
		}
	}

	// Configure dial options
	dialOpts := []grpc.DialOption{
		grpc.WithMaxMsgSize(opts.MaxRecvMsgSize),
		
		// Keep alive parameters
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                opts.KeepAliveTime,
			Timeout:             opts.KeepAliveTimeout,
			PermitWithoutStream: opts.PermitWithoutStream,
		}),

		// Window size settings for better throughput
		grpc.WithInitialWindowSize(opts.InitialWindowSize),
		grpc.WithInitialConnWindowSize(opts.InitialConnWindow),

		// Add client interceptors
		grpc.WithChainUnaryInterceptor(
			c.metrics.UnaryClientInterceptor(),
			middleware.RetryUnaryInterceptor(opts.MaxRetryAttempts),
			middleware.TimeoutUnaryInterceptor(30*time.Second),
		),
		
		grpc.WithChainStreamInterceptor(
			c.metrics.StreamClientInterceptor(),
			middleware.TimeoutStreamInterceptor(60*time.Second),
		),
	}

	// Configure load balancing
	if opts.EnableLoadBalancing {
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(`{
			"loadBalancingConfig": [ { "round_robin": {} } ],
			"retryPolicy": {
				"maxAttempts": 3,
				"initialBackoff": "0.1s",
				"maxBackoff": "1s",
				"backoffMultiplier": 2,
				"retryableStatusCodes": [ "UNAVAILABLE", "DEADLINE_EXCEEDED" ]
			}
		}`))
	}

	// Configure security
	if opts.Insecure {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	} else if opts.EnableTLS {
		creds := credentials.NewTLS(&tls.Config{
			ServerName: opts.ServerName,
			MinVersion: tls.VersionTLS12,
		})
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}

	// Create connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s at %s: %w", serviceName, address, err)
	}

	// Store connection for reuse
	c.connections[serviceName] = conn

	log.Printf("Created optimized gRPC connection to %s at %s", serviceName, address)
	return conn, nil
}

// CloseAll closes all client connections
func (c *OptimizedGRPCClient) CloseAll() {
	for serviceName, conn := range c.connections {
		conn.Close()
		log.Printf("Closed gRPC connection to %s", serviceName)
	}
	c.connections = make(map[string]*grpc.ClientConn)
}

// GetConnectionStats returns statistics for all connections
func (c *OptimizedGRPCClient) GetConnectionStats() map[string]string {
	stats := make(map[string]string)
	for serviceName, conn := range c.connections {
		stats[serviceName] = conn.GetState().String()
	}
	return stats
}