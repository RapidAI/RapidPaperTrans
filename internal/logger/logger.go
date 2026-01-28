// Package logger provides logging functionality for the LaTeX translator application.
// It implements structured logging with support for file output, log rotation,
// and different log levels.
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents the severity level of a log message
type Level int

const (
	// LevelDebug is for detailed debugging information
	LevelDebug Level = iota
	// LevelInfo is for general informational messages
	LevelInfo
	// LevelWarn is for warning messages
	LevelWarn
	// LevelError is for error messages
	LevelError
)

// String returns the string representation of the log level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
	Value interface{}
}

// String creates a string field
func String(key string, value string) Field {
	return Field{Key: key, Value: value}
}

// Int creates an integer field
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Int64 creates an int64 field
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Float64 creates a float64 field
func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

// Bool creates a boolean field
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

// Err creates an error field
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

// Any creates a field with any value
func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Logger defines the logging interface
type Logger interface {
	// Debug logs a debug message with optional fields
	Debug(msg string, fields ...Field)
	// Info logs an informational message with optional fields
	Info(msg string, fields ...Field)
	// Warn logs a warning message with optional fields
	Warn(msg string, fields ...Field)
	// Error logs an error message with error and optional fields
	Error(msg string, err error, fields ...Field)
	// SetLevel sets the minimum log level
	SetLevel(level Level)
	// Close closes the logger and releases resources
	Close() error
}

// Config holds the configuration for the logger
type Config struct {
	// LogFilePath is the path to the log file
	LogFilePath string
	// MaxFileSize is the maximum size of a log file in bytes before rotation
	MaxFileSize int64
	// MaxBackups is the maximum number of backup log files to keep
	MaxBackups int
	// Level is the minimum log level to output
	Level Level
	// EnableConsole enables output to console in addition to file
	EnableConsole bool
}

// DefaultConfig returns a default logger configuration
func DefaultConfig() *Config {
	return &Config{
		LogFilePath:   "latex-translator.log",
		MaxFileSize:   10 * 1024 * 1024, // 10 MB
		MaxBackups:    5,
		Level:         LevelInfo,
		EnableConsole: false,
	}
}

// DefaultLogger is the default implementation of the Logger interface
type DefaultLogger struct {
	config     *Config
	file       *os.File
	mu         sync.Mutex
	level      Level
	fileSize   int64
	writers    []io.Writer
	timeFormat string
}

// NewDefaultLogger creates a new DefaultLogger with the given configuration
func NewDefaultLogger(config *Config) (*DefaultLogger, error) {
	if config == nil {
		config = DefaultConfig()
	}

	logger := &DefaultLogger{
		config:     config,
		level:      config.Level,
		timeFormat: "2006-01-02 15:04:05.000",
	}

	// Create log directory if it doesn't exist
	logDir := filepath.Dir(config.LogFilePath)
	if logDir != "" && logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// Open log file
	if err := logger.openLogFile(); err != nil {
		return nil, err
	}

	// Setup writers
	logger.setupWriters()

	return logger, nil
}

// openLogFile opens or creates the log file
func (l *DefaultLogger) openLogFile() error {
	file, err := os.OpenFile(l.config.LogFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	l.file = file
	l.fileSize = info.Size()
	return nil
}

// setupWriters configures the output writers
func (l *DefaultLogger) setupWriters() {
	l.writers = []io.Writer{l.file}
	if l.config.EnableConsole {
		l.writers = append(l.writers, os.Stdout)
	}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, nil, fields...)
}

// Info logs an informational message
func (l *DefaultLogger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, nil, fields...)
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, nil, fields...)
}

// Error logs an error message with stack trace
func (l *DefaultLogger) Error(msg string, err error, fields ...Field) {
	l.log(LevelError, msg, err, fields...)
}

// SetLevel sets the minimum log level
func (l *DefaultLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Close closes the logger and releases resources
func (l *DefaultLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log writes a log entry
func (l *DefaultLogger) log(level Level, msg string, err error, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if level is enabled
	if level < l.level {
		return
	}

	// Build log entry
	entry := l.formatEntry(level, msg, err, fields...)

	// Check for rotation before writing
	if l.shouldRotate(int64(len(entry))) {
		l.rotate()
	}

	// Write to all writers
	for _, w := range l.writers {
		w.Write([]byte(entry))
	}

	l.fileSize += int64(len(entry))
}

// formatEntry formats a log entry
func (l *DefaultLogger) formatEntry(level Level, msg string, err error, fields ...Field) string {
	var sb strings.Builder

	// Timestamp
	sb.WriteString(time.Now().Format(l.timeFormat))
	sb.WriteString(" ")

	// Level
	sb.WriteString("[")
	sb.WriteString(level.String())
	sb.WriteString("] ")

	// Message
	sb.WriteString(msg)

	// Error (if present)
	if err != nil {
		sb.WriteString(" error=\"")
		sb.WriteString(err.Error())
		sb.WriteString("\"")
	}

	// Fields
	for _, f := range fields {
		sb.WriteString(" ")
		sb.WriteString(f.Key)
		sb.WriteString("=")
		sb.WriteString(fmt.Sprintf("%v", f.Value))
	}

	// Stack trace for Error level
	if level == LevelError {
		sb.WriteString("\n")
		sb.WriteString(l.getStackTrace())
	}

	sb.WriteString("\n")
	return sb.String()
}

// getStackTrace returns the current stack trace
func (l *DefaultLogger) getStackTrace() string {
	var sb strings.Builder
	sb.WriteString("Stack trace:\n")

	// Skip the first few frames (log, Error, etc.)
	const skip = 4
	for i := skip; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		fn := runtime.FuncForPC(pc)
		funcName := "unknown"
		if fn != nil {
			funcName = fn.Name()
		}

		// Skip runtime and testing frames
		if strings.Contains(funcName, "runtime.") || strings.Contains(funcName, "testing.") {
			continue
		}

		sb.WriteString(fmt.Sprintf("  %s:%d %s\n", file, line, funcName))

		// Limit stack trace depth
		if i-skip > 10 {
			sb.WriteString("  ... (truncated)\n")
			break
		}
	}

	return sb.String()
}

// shouldRotate checks if log rotation is needed
func (l *DefaultLogger) shouldRotate(additionalSize int64) bool {
	return l.fileSize+additionalSize > l.config.MaxFileSize
}

// rotate performs log file rotation
func (l *DefaultLogger) rotate() error {
	if l.file != nil {
		l.file.Close()
	}

	// Rotate existing backup files
	for i := l.config.MaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.config.LogFilePath, i)
		newPath := fmt.Sprintf("%s.%d", l.config.LogFilePath, i+1)
		os.Rename(oldPath, newPath)
	}

	// Rename current log file to .1
	if _, err := os.Stat(l.config.LogFilePath); err == nil {
		os.Rename(l.config.LogFilePath, l.config.LogFilePath+".1")
	}

	// Remove oldest backup if exceeds MaxBackups
	oldestBackup := fmt.Sprintf("%s.%d", l.config.LogFilePath, l.config.MaxBackups+1)
	os.Remove(oldestBackup)

	// Open new log file
	if err := l.openLogFile(); err != nil {
		return err
	}

	l.setupWriters()
	return nil
}

// Global logger instance
var (
	globalLogger Logger
	globalMu     sync.RWMutex
)

// Init initializes the global logger with the given configuration
func Init(config *Config) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	logger, err := NewDefaultLogger(config)
	if err != nil {
		return err
	}

	// Close existing logger if any
	if globalLogger != nil {
		globalLogger.Close()
	}

	globalLogger = logger
	return nil
}

// GetLogger returns the global logger instance
func GetLogger() Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()

	if globalLogger == nil {
		// Return a no-op logger if not initialized
		return &noopLogger{}
	}
	return globalLogger
}

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = logger
}

// Close closes the global logger
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalLogger != nil {
		err := globalLogger.Close()
		globalLogger = nil
		return err
	}
	return nil
}

// Convenience functions for global logger

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...Field) {
	GetLogger().Debug(msg, fields...)
}

// Info logs an informational message using the global logger
func Info(msg string, fields ...Field) {
	GetLogger().Info(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...Field) {
	GetLogger().Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, err error, fields ...Field) {
	GetLogger().Error(msg, err, fields...)
}

// noopLogger is a no-operation logger that discards all log messages
type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...Field)          {}
func (n *noopLogger) Info(msg string, fields ...Field)           {}
func (n *noopLogger) Warn(msg string, fields ...Field)           {}
func (n *noopLogger) Error(msg string, err error, fields ...Field) {}
func (n *noopLogger) SetLevel(level Level)                       {}
func (n *noopLogger) Close() error                               { return nil }
