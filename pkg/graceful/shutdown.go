// Package graceful provides graceful shutdown capabilities for Go services
// Task: immediate-11 - Configure graceful shutdown handlers in all Go services
package graceful

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// ShutdownHandler manages graceful shutdown of services
type ShutdownHandler struct {
	logger          *logrus.Logger
	shutdownTimeout time.Duration
	signals         []os.Signal
	hooks           []ShutdownHook
	mu              sync.RWMutex
	shutdownStarted bool
}

// ShutdownHook represents a function to be called during shutdown
type ShutdownHook func(ctx context.Context) error

// Option is a functional option for configuring ShutdownHandler
type Option func(*ShutdownHandler)

// NewShutdownHandler creates a new graceful shutdown handler
func NewShutdownHandler(options ...Option) *ShutdownHandler {
	handler := &ShutdownHandler{
		logger:          logrus.New(),
		shutdownTimeout: 30 * time.Second,
		signals:         []os.Signal{syscall.SIGTERM, syscall.SIGINT},
		hooks:           make([]ShutdownHook, 0),
	}

	for _, option := range options {
		option(handler)
	}

	return handler
}

// WithLogger sets a custom logger
func WithLogger(logger *logrus.Logger) Option {
	return func(h *ShutdownHandler) {
		h.logger = logger
	}
}

// WithTimeout sets the shutdown timeout
func WithTimeout(timeout time.Duration) Option {
	return func(h *ShutdownHandler) {
		h.shutdownTimeout = timeout
	}
}

// WithSignals sets custom signals to listen for
func WithSignals(signals ...os.Signal) Option {
	return func(h *ShutdownHandler) {
		h.signals = signals
	}
}

// AddHook adds a shutdown hook
func (h *ShutdownHandler) AddHook(hook ShutdownHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, hook)
}

// Wait blocks until a shutdown signal is received and executes all hooks
func (h *ShutdownHandler) Wait() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, h.signals...)

	// Wait for signal
	sig := <-sigChan
	h.logger.WithField("signal", sig).Info("Received shutdown signal, initiating graceful shutdown")

	h.mu.Lock()
	if h.shutdownStarted {
		h.mu.Unlock()
		h.logger.Warn("Shutdown already in progress")
		return
	}
	h.shutdownStarted = true
	h.mu.Unlock()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
	defer cancel()

	// Execute all hooks concurrently
	h.executeHooks(ctx)

	h.logger.Info("Graceful shutdown completed")
}

// Shutdown initiates shutdown programmatically
func (h *ShutdownHandler) Shutdown() {
	h.mu.Lock()
	if h.shutdownStarted {
		h.mu.Unlock()
		h.logger.Warn("Shutdown already in progress")
		return
	}
	h.shutdownStarted = true
	h.mu.Unlock()

	h.logger.Info("Initiating programmatic shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
	defer cancel()

	h.executeHooks(ctx)
	h.logger.Info("Programmatic shutdown completed")
}

// executeHooks runs all shutdown hooks with proper error handling
func (h *ShutdownHandler) executeHooks(ctx context.Context) {
	if len(h.hooks) == 0 {
		h.logger.Info("No shutdown hooks to execute")
		return
	}

	var wg sync.WaitGroup
	errorChan := make(chan error, len(h.hooks))

	// Execute hooks concurrently
	for i, hook := range h.hooks {
		wg.Add(1)
		go func(index int, hookFunc ShutdownHook) {
			defer wg.Done()
			
			hookLogger := h.logger.WithField("hook_index", index)
			hookLogger.Info("Executing shutdown hook")
			
			start := time.Now()
			if err := hookFunc(ctx); err != nil {
				hookLogger.WithError(err).Error("Shutdown hook failed")
				errorChan <- err
			} else {
				hookLogger.WithField("duration", time.Since(start)).Info("Shutdown hook completed successfully")
			}
		}(i, hook)
	}

	// Wait for all hooks to complete or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		h.logger.WithField("hooks_count", len(h.hooks)).Info("All shutdown hooks completed")
	case <-ctx.Done():
		h.logger.Warn("Shutdown timeout reached, some hooks may not have completed")
	}

	// Collect any errors
	close(errorChan)
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		h.logger.WithField("error_count", len(errors)).Error("Some shutdown hooks failed")
		for i, err := range errors {
			h.logger.WithField("error_index", i).WithError(err).Error("Hook error")
		}
	}
}

// CommonShutdownHooks provides commonly used shutdown hooks

// HTTPServerShutdownHook creates a hook for graceful HTTP server shutdown
func HTTPServerShutdownHook(server interface{ Shutdown(ctx context.Context) error }) ShutdownHook {
	return func(ctx context.Context) error {
		return server.Shutdown(ctx)
	}
}

// DatabaseConnectionShutdownHook creates a hook for closing database connections
func DatabaseConnectionShutdownHook(db interface{ Close() error }) ShutdownHook {
	return func(ctx context.Context) error {
		return db.Close()
	}
}

// RedisConnectionShutdownHook creates a hook for closing Redis connections
func RedisConnectionShutdownHook(client interface{ Close() error }) ShutdownHook {
	return func(ctx context.Context) error {
		return client.Close()
	}
}

// WorkerPoolShutdownHook creates a hook for shutting down worker pools
func WorkerPoolShutdownHook(shutdown func() error) ShutdownHook {
	return func(ctx context.Context) error {
		return shutdown()
	}
}

// ResourceCleanupHook creates a hook for general resource cleanup
func ResourceCleanupHook(cleanup func() error) ShutdownHook {
	return func(ctx context.Context) error {
		return cleanup()
	}
}

// MetricsFlushHook creates a hook for flushing metrics before shutdown
func MetricsFlushHook(flush func() error) ShutdownHook {
	return func(ctx context.Context) error {
		return flush()
	}
}

// FileHandleCleanupHook creates a hook for closing file handles
func FileHandleCleanupHook(files ...interface{ Close() error }) ShutdownHook {
	return func(ctx context.Context) error {
		for _, file := range files {
			if err := file.Close(); err != nil {
				return err
			}
		}
		return nil
	}
}