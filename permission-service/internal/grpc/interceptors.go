package grpc

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor provides authentication for gRPC requests
type AuthInterceptor struct {
	logger *zap.Logger
}

// NewAuthInterceptor creates a new authentication interceptor
func NewAuthInterceptor(logger *zap.Logger) *AuthInterceptor {
	return &AuthInterceptor{
		logger: logger,
	}
}

// UnaryInterceptor provides authentication for unary gRPC calls
func (a *AuthInterceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// Skip authentication for health check
	if info.FullMethod == "/pyairtable.permission.v1.PermissionService/HealthCheck" {
		return handler(ctx, req)
	}

	// Extract and validate authentication
	newCtx, err := a.authenticate(ctx)
	if err != nil {
		a.logger.Warn("Authentication failed",
			zap.String("method", info.FullMethod),
			zap.Error(err),
		)
		return nil, err
	}

	return handler(newCtx, req)
}

// StreamInterceptor provides authentication for streaming gRPC calls
func (a *AuthInterceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	// Extract and validate authentication
	newCtx, err := a.authenticate(ss.Context())
	if err != nil {
		a.logger.Warn("Authentication failed",
			zap.String("method", info.FullMethod),
			zap.Error(err),
		)
		return err
	}

	// Create a new stream with authenticated context
	wrapped := &contextServerStream{
		ServerStream: ss,
		ctx:          newCtx,
	}

	return handler(srv, wrapped)
}

// authenticate extracts and validates authentication from gRPC metadata
func (a *AuthInterceptor) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	// Extract Authorization header
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "empty bearer token")
	}

	// TODO: Implement actual JWT token validation here
	// For now, we'll extract user_id from a custom header as a placeholder
	userIDHeaders := md.Get("x-user-id")
	if len(userIDHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing user ID")
	}

	userID := userIDHeaders[0]
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "empty user ID")
	}

	// Add user information to context
	newCtx := context.WithValue(ctx, "user_id", userID)
	newCtx = context.WithValue(newCtx, "token", token)

	// Extract tenant information if available
	tenantHeaders := md.Get("x-tenant-id")
	if len(tenantHeaders) > 0 && tenantHeaders[0] != "" {
		newCtx = context.WithValue(newCtx, "tenant_id", tenantHeaders[0])
	}

	return newCtx, nil
}

// contextServerStream wraps a ServerStream with a new context
type contextServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (css *contextServerStream) Context() context.Context {
	return css.ctx
}

// LoggingInterceptor provides request logging for gRPC
type LoggingInterceptor struct {
	logger *zap.Logger
}

// NewLoggingInterceptor creates a new logging interceptor
func NewLoggingInterceptor(logger *zap.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger: logger,
	}
}

// UnaryInterceptor logs unary gRPC requests
func (l *LoggingInterceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	l.logger.Info("gRPC request started",
		zap.String("method", info.FullMethod),
		zap.Time("start_time", start),
	)

	resp, err := handler(ctx, req)

	duration := time.Since(start)
	fields := []zap.Field{
		zap.String("method", info.FullMethod),
		zap.Duration("duration", duration),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.logger.Error("gRPC request failed", fields...)
	} else {
		l.logger.Info("gRPC request completed", fields...)
	}

	return resp, err
}

// StreamInterceptor logs streaming gRPC requests
func (l *LoggingInterceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	l.logger.Info("gRPC stream started",
		zap.String("method", info.FullMethod),
		zap.Time("start_time", start),
	)

	err := handler(srv, ss)

	duration := time.Since(start)
	fields := []zap.Field{
		zap.String("method", info.FullMethod),
		zap.Duration("duration", duration),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.logger.Error("gRPC stream failed", fields...)
	} else {
		l.logger.Info("gRPC stream completed", fields...)
	}

	return err
}

// RecoveryInterceptor provides panic recovery for gRPC
type RecoveryInterceptor struct {
	logger *zap.Logger
}

// NewRecoveryInterceptor creates a new recovery interceptor
func NewRecoveryInterceptor(logger *zap.Logger) *RecoveryInterceptor {
	return &RecoveryInterceptor{
		logger: logger,
	}
}

// UnaryInterceptor recovers from panics in unary gRPC calls
func (r *RecoveryInterceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error("gRPC panic recovered",
				zap.String("method", info.FullMethod),
				zap.Any("panic", p),
			)
			err = status.Error(codes.Internal, "internal server error")
		}
	}()

	return handler(ctx, req)
}

// StreamInterceptor recovers from panics in streaming gRPC calls
func (r *RecoveryInterceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error("gRPC stream panic recovered",
				zap.String("method", info.FullMethod),
				zap.Any("panic", p),
			)
			err = status.Error(codes.Internal, "internal server error")
		}
	}()

	return handler(srv, ss)
}

// CreateServerInterceptors creates all server interceptors
func CreateServerInterceptors(logger *zap.Logger, meter metric.Meter) ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	// Create interceptors
	auth := NewAuthInterceptor(logger)
	logging := NewLoggingInterceptor(logger)
	recovery := NewRecoveryInterceptor(logger)

	// Unary interceptors (order matters - recovery should be first, auth last)
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		recovery.UnaryInterceptor,
		logging.UnaryInterceptor,
		otelgrpc.UnaryServerInterceptor(),
		auth.UnaryInterceptor,
	}

	// Stream interceptors (order matters - recovery should be first, auth last)
	streamInterceptors := []grpc.StreamServerInterceptor{
		recovery.StreamInterceptor,
		logging.StreamInterceptor,
		otelgrpc.StreamServerInterceptor(),
		auth.StreamInterceptor,
	}

	return unaryInterceptors, streamInterceptors
}