package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	permissionv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/permission/v1"
	"github.com/pyairtable/pyairtable-permission-service/internal/services"
)

// Server represents the gRPC server
type Server struct {
	server          *grpc.Server
	permissionSvc   *PermissionServer
	healthServer    *health.Server
	logger          *zap.Logger
	port            string
}

// ServerConfig holds configuration for the gRPC server
type ServerConfig struct {
	Port                   string
	MaxRecvMsgSize         int
	MaxSendMsgSize         int
	ConnectionTimeout      time.Duration
	MaxConnectionIdle      time.Duration
	MaxConnectionAge       time.Duration
	MaxConnectionAgeGrace  time.Duration
	Time                   time.Duration
	Timeout                time.Duration
	EnableReflection       bool
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:                   "50051",
		MaxRecvMsgSize:         1024 * 1024 * 4,  // 4MB
		MaxSendMsgSize:         1024 * 1024 * 4,  // 4MB
		ConnectionTimeout:      5 * time.Second,
		MaxConnectionIdle:      30 * time.Minute,
		MaxConnectionAge:       30 * time.Minute,
		MaxConnectionAgeGrace:  5 * time.Second,
		Time:                   10 * time.Second,
		Timeout:                5 * time.Second,
		EnableReflection:       true,
	}
}

// NewServer creates a new gRPC server
func NewServer(
	config *ServerConfig,
	logger *zap.Logger,
	permissionSvc *services.PermissionService,
	roleSvc *services.RoleService,
	auditSvc *services.AuditService,
	meter metric.Meter,
) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	// Create interceptors
	unaryInterceptors, streamInterceptors := CreateServerInterceptors(logger, meter)

	// Configure server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(config.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(config.MaxSendMsgSize),
		grpc.ConnectionTimeout(config.ConnectionTimeout),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     config.MaxConnectionIdle,
			MaxConnectionAge:      config.MaxConnectionAge,
			MaxConnectionAgeGrace: config.MaxConnectionAgeGrace,
			Time:                  config.Time,
			Timeout:               config.Timeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	}

	// Create gRPC server
	grpcServer := grpc.NewServer(opts...)

	// Create permission service server
	permissionServer, err := NewPermissionServer(logger, permissionSvc, roleSvc, auditSvc, meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create permission server: %w", err)
	}

	// Register services
	permissionv1.RegisterPermissionServiceServer(grpcServer, permissionServer)

	// Create health server
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Enable reflection if configured
	if config.EnableReflection {
		reflection.Register(grpcServer)
	}

	return &Server{
		server:        grpcServer,
		permissionSvc: permissionServer,
		healthServer:  healthServer,
		logger:        logger,
		port:          config.Port,
	}, nil
}

// Start starts the gRPC server
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", s.port, err)
	}

	s.logger.Info("Starting gRPC server",
		zap.String("port", s.port),
		zap.String("address", listener.Addr().String()),
	)

	// Set initial health status
	s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.healthServer.SetServingStatus("pyairtable.permission.v1.PermissionService", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := s.server.Serve(listener); err != nil {
			serverErr <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down gRPC server due to context cancellation")
		return s.Shutdown(ctx)
	case err := <-serverErr:
		return err
	}
}

// Shutdown gracefully shuts down the gRPC server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Gracefully shutting down gRPC server")

	// Set health status to NOT_SERVING
	s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	s.healthServer.SetServingStatus("pyairtable.permission.v1.PermissionService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// Create a channel to track graceful shutdown completion
	done := make(chan struct{})

	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		s.logger.Info("gRPC server gracefully stopped")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Timeout waiting for graceful shutdown, forcing stop")
		s.server.Stop()
		return ctx.Err()
	}
}

// GetPort returns the server port
func (s *Server) GetPort() string {
	return s.port
}

// GetServer returns the underlying gRPC server
func (s *Server) GetServer() *grpc.Server {
	return s.server
}