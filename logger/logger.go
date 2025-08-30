package logger

import (
	"context"
	"fmt"
	"gin/objects"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// cleanOldLogs removes all files from the logs directory
func cleanOldLogs(logDir string) error {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Read directory contents
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}

	// Remove all files in the directory
	for _, entry := range entries {
		if !entry.IsDir() { // Only remove files, not subdirectories
			if err := os.Remove(filepath.Join(logDir, entry.Name())); err != nil {
				return fmt.Errorf("failed to remove old log file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// createLogFile creates a new log file with timestamp
func createLogFile(filePath string) (*os.File, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create or truncate the log file
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	return f, nil
}

// InitLogger initializes the logger and sets output to the given file path.
// If filePath is empty, logs are written to stdout.
func InitLogger(filePath string, level logrus.Level) error {
	l := logrus.New()
	l.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			s := strings.Split(f.Function, ".")
			funcName := s[len(s)-1]
			return funcName, fmt.Sprintf("%s:%d", f.File, f.Line)
		},
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyLevel: "0_level",
			logrus.FieldKeyFile:  "1_file",
			logrus.FieldKeyFunc:  "2_func",
			logrus.FieldKeyMsg:   "3_msg",
			logrus.FieldKeyTime:  "4_time",
		},
	})

	l.SetReportCaller(true)
	l.SetLevel(level)

	if filePath != "" {
		// Clean old logs before starting
		logDir := filepath.Dir(filePath)
		if err := cleanOldLogs(logDir); err != nil {
			l.WithError(err).Warn("Failed to clean old logs")
		}

		// Create new log file
		f, err := createLogFile(filePath)
		if err != nil {
			l.WithError(err).Warn("Failed to create log file, falling back to stdout")
			l.SetOutput(os.Stdout)
			objects.FileLog = l
			return err
		}

		l.SetOutput(f)
		// Log startup message
		l.WithFields(logrus.Fields{
			"time": time.Now().Format(time.RFC3339),
			"file": filePath,
		}).Info("Started new log file after cleaning old logs")
	} else {
		l.SetOutput(os.Stdout)
	}

	objects.FileLog = l
	return nil
}

// WithContext creates a new entry with fields from context
func WithContext(ctx context.Context) *logrus.Entry {
	fields := logrus.Fields{}

	// Add transaction ID if exists
	if transID, ok := ctx.Value(objects.TransactionIDKey).(string); ok {
		fields["transaction_id"] = transID
	}

	// Add user ID if exists
	if userID, ok := ctx.Value(objects.UserIDKey).(string); ok {
		fields["user_id"] = userID
	}

	// Add request ID if exists
	if reqID, ok := ctx.Value(objects.RequestIDKey).(string); ok {
		fields["request_id"] = reqID
	}

	return objects.FileLog.WithFields(fields)
}

// NewContext creates a new context with transaction ID
func NewContext() context.Context {
	return context.WithValue(context.Background(), objects.TransactionIDKey, uuid.New().String())
}

// WithTransactionID adds a transaction ID to the context
func WithTransactionID(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, objects.TransactionIDKey, uuid.New().String())
}

// WithUserID adds a user ID to the context
func WithUserID(ctx context.Context, userID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, objects.UserIDKey, userID)
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, objects.RequestIDKey, uuid.New().String())
}
