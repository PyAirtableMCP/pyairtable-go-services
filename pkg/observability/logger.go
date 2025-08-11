package observability

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
)

// Logger wraps logrus with additional functionality
type Logger struct {
	*logrus.Logger
}

// NewLogger creates a new logger instance
func NewLogger(level, format string) *Logger {
	logger := logrus.New()

	// Set log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	// Set formatter
	switch format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	default:
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	}

	// Set output
	logger.SetOutput(os.Stdout)

	return &Logger{logger}
}

// WithContext adds context fields to the logger
func (l *Logger) WithContext(ctx context.Context) *logrus.Entry {
	entry := l.Logger.WithContext(ctx)

	// Add request ID if available
	if requestID := ctx.Value("request_id"); requestID != nil {
		entry = entry.WithField("request_id", requestID)
	}

	// Add user ID if available
	if userID := ctx.Value("user_id"); userID != nil {
		entry = entry.WithField("user_id", userID)
	}

	// Add tenant ID if available
	if tenantID := ctx.Value("tenant_id"); tenantID != nil {
		entry = entry.WithField("tenant_id", tenantID)
	}

	return entry
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *logrus.Entry {
	return l.Logger.WithFields(fields)
}

// WithField adds a single field to the logger
func (l *Logger) WithField(key string, value interface{}) *logrus.Entry {
	return l.Logger.WithField(key, value)
}

// WithError adds an error field to the logger
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// Info logs an info message with optional key-value pairs
func (l *Logger) Info(msg string, keyvals ...interface{}) {
	entry := l.Logger.WithFields(parseKeyVals(keyvals...))
	entry.Info(msg)
}

// Error logs an error message with optional key-value pairs
func (l *Logger) Error(msg string, keyvals ...interface{}) {
	entry := l.Logger.WithFields(parseKeyVals(keyvals...))
	entry.Error(msg)
}

// Warn logs a warning message with optional key-value pairs
func (l *Logger) Warn(msg string, keyvals ...interface{}) {
	entry := l.Logger.WithFields(parseKeyVals(keyvals...))
	entry.Warn(msg)
}

// Debug logs a debug message with optional key-value pairs
func (l *Logger) Debug(msg string, keyvals ...interface{}) {
	entry := l.Logger.WithFields(parseKeyVals(keyvals...))
	entry.Debug(msg)
}

// Fatal logs a fatal message with optional key-value pairs and exits
func (l *Logger) Fatal(msg string, keyvals ...interface{}) {
	entry := l.Logger.WithFields(parseKeyVals(keyvals...))
	entry.Fatal(msg)
}

// parseKeyVals converts key-value pairs to logrus Fields
func parseKeyVals(keyvals ...interface{}) logrus.Fields {
	fields := make(logrus.Fields)

	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			key, ok := keyvals[i].(string)
			if ok {
				fields[key] = keyvals[i+1]
			}
		}
	}

	return fields
}

// RequestLogger creates a logger with request context
func (l *Logger) RequestLogger(requestID, method, path string) *logrus.Entry {
	return l.Logger.WithFields(logrus.Fields{
		"request_id": requestID,
		"method":     method,
		"path":       path,
	})
}

// ServiceLogger creates a logger with service context
func (l *Logger) ServiceLogger(service, component string) *logrus.Entry {
	return l.Logger.WithFields(logrus.Fields{
		"service":   service,
		"component": component,
	})
}

// DatabaseLogger creates a logger for database operations
func (l *Logger) DatabaseLogger(operation, table string) *logrus.Entry {
	return l.Logger.WithFields(logrus.Fields{
		"operation": operation,
		"table":     table,
		"component": "database",
	})
}

// WorkflowLogger creates a logger for workflow operations
func (l *Logger) WorkflowLogger(workflowID, executionID string) *logrus.Entry {
	return l.Logger.WithFields(logrus.Fields{
		"workflow_id":  workflowID,
		"execution_id": executionID,
		"component":    "workflow",
	})
}