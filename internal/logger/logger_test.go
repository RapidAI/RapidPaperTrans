package logger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDefaultLogger(t *testing.T) {
	// Create a temporary directory for test logs
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024,
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Verify log file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Log file was not created")
	}
}

func TestLogLevels(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Log messages at different levels
	logger.Debug("debug message", String("key", "value"))
	logger.Info("info message", Int("count", 42))
	logger.Warn("warn message", Bool("flag", true))
	logger.Error("error message", errors.New("test error"), Float64("rate", 3.14))

	logger.Close()

	// Read log file and verify content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Verify all log levels are present
	if !strings.Contains(logContent, "[DEBUG]") {
		t.Error("Debug message not found in log")
	}
	if !strings.Contains(logContent, "[INFO]") {
		t.Error("Info message not found in log")
	}
	if !strings.Contains(logContent, "[WARN]") {
		t.Error("Warn message not found in log")
	}
	if !strings.Contains(logContent, "[ERROR]") {
		t.Error("Error message not found in log")
	}

	// Verify messages
	if !strings.Contains(logContent, "debug message") {
		t.Error("Debug message content not found")
	}
	if !strings.Contains(logContent, "info message") {
		t.Error("Info message content not found")
	}
	if !strings.Contains(logContent, "warn message") {
		t.Error("Warn message content not found")
	}
	if !strings.Contains(logContent, "error message") {
		t.Error("Error message content not found")
	}

	// Verify fields
	if !strings.Contains(logContent, "key=value") {
		t.Error("String field not found")
	}
	if !strings.Contains(logContent, "count=42") {
		t.Error("Int field not found")
	}
	if !strings.Contains(logContent, "flag=true") {
		t.Error("Bool field not found")
	}
	if !strings.Contains(logContent, "rate=3.14") {
		t.Error("Float64 field not found")
	}

	// Verify error is logged
	if !strings.Contains(logContent, "test error") {
		t.Error("Error not found in log")
	}

	// Verify stack trace for error level
	if !strings.Contains(logContent, "Stack trace:") {
		t.Error("Stack trace not found for error level")
	}
}

func TestLogLevelFiltering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	// Set level to Warn - should filter out Debug and Info
	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    3,
		Level:         LevelWarn,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message", nil)

	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Debug and Info should be filtered out
	if strings.Contains(logContent, "[DEBUG]") {
		t.Error("Debug message should be filtered out")
	}
	if strings.Contains(logContent, "[INFO]") {
		t.Error("Info message should be filtered out")
	}

	// Warn and Error should be present
	if !strings.Contains(logContent, "[WARN]") {
		t.Error("Warn message should be present")
	}
	if !strings.Contains(logContent, "[ERROR]") {
		t.Error("Error message should be present")
	}
}

func TestSetLevel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Log a debug message (should appear)
	logger.Debug("debug before")

	// Change level to Error
	logger.SetLevel(LevelError)

	// Log messages (only error should appear)
	logger.Debug("debug after")
	logger.Info("info after")
	logger.Warn("warn after")
	logger.Error("error after", nil)

	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if !strings.Contains(logContent, "debug before") {
		t.Error("Debug before level change should be present")
	}
	if strings.Contains(logContent, "debug after") {
		t.Error("Debug after level change should be filtered")
	}
	if strings.Contains(logContent, "info after") {
		t.Error("Info after level change should be filtered")
	}
	if strings.Contains(logContent, "warn after") {
		t.Error("Warn after level change should be filtered")
	}
	if !strings.Contains(logContent, "error after") {
		t.Error("Error after level change should be present")
	}
}

func TestLogRotation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	// Set a very small max file size to trigger rotation
	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   100, // 100 bytes
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Write enough messages to trigger rotation
	for i := 0; i < 20; i++ {
		logger.Info("This is a test message that should trigger log rotation eventually")
	}

	logger.Close()

	// Check that backup files were created
	backupPath := logPath + ".1"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup log file was not created after rotation")
	}
}

func TestFieldTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test all field types
	logger.Info("test fields",
		String("str", "hello"),
		Int("int", 42),
		Int64("int64", 9223372036854775807),
		Float64("float", 3.14159),
		Bool("bool", true),
		Err(errors.New("sample error")),
		Any("any", map[string]int{"a": 1}),
	)

	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if !strings.Contains(logContent, "str=hello") {
		t.Error("String field not formatted correctly")
	}
	if !strings.Contains(logContent, "int=42") {
		t.Error("Int field not formatted correctly")
	}
	if !strings.Contains(logContent, "int64=9223372036854775807") {
		t.Error("Int64 field not formatted correctly")
	}
	if !strings.Contains(logContent, "float=3.14159") {
		t.Error("Float64 field not formatted correctly")
	}
	if !strings.Contains(logContent, "bool=true") {
		t.Error("Bool field not formatted correctly")
	}
	if !strings.Contains(logContent, "error=sample error") {
		t.Error("Err field not formatted correctly")
	}
}

func TestGlobalLogger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "global.log")

	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	// Initialize global logger
	err = Init(config)
	if err != nil {
		t.Fatalf("Failed to initialize global logger: %v", err)
	}

	// Use global convenience functions
	Debug("global debug")
	Info("global info")
	Warn("global warn")
	Error("global error", errors.New("global test error"))

	// Close global logger
	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if !strings.Contains(logContent, "global debug") {
		t.Error("Global debug message not found")
	}
	if !strings.Contains(logContent, "global info") {
		t.Error("Global info message not found")
	}
	if !strings.Contains(logContent, "global warn") {
		t.Error("Global warn message not found")
	}
	if !strings.Contains(logContent, "global error") {
		t.Error("Global error message not found")
	}
}

func TestNoopLogger(t *testing.T) {
	// Reset global logger to nil
	SetGlobalLogger(nil)

	// These should not panic - noop logger should handle them
	Debug("test")
	Info("test")
	Warn("test")
	Error("test", nil)

	// GetLogger should return noop logger when not initialized
	logger := GetLogger()
	if logger == nil {
		t.Error("GetLogger should return noop logger, not nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.LogFilePath == "" {
		t.Error("Default log file path should not be empty")
	}
	if config.MaxFileSize <= 0 {
		t.Error("Default max file size should be positive")
	}
	if config.MaxBackups <= 0 {
		t.Error("Default max backups should be positive")
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("Level(%d).String() = %s, want %s", tt.level, got, tt.expected)
		}
	}
}

func TestErrFieldWithNil(t *testing.T) {
	field := Err(nil)
	if field.Key != "error" {
		t.Errorf("Err(nil).Key = %s, want error", field.Key)
	}
	if field.Value != nil {
		t.Errorf("Err(nil).Value = %v, want nil", field.Value)
	}
}

func TestLogDirectoryCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a nested directory that doesn't exist
	logPath := filepath.Join(tmpDir, "nested", "dir", "test.log")

	config := &Config{
		LogFilePath:   logPath,
		MaxFileSize:   1024 * 1024,
		MaxBackups:    3,
		Level:         LevelDebug,
		EnableConsole: false,
	}

	logger, err := NewDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger with nested directory: %v", err)
	}
	defer logger.Close()

	// Verify nested directory was created
	if _, err := os.Stat(filepath.Dir(logPath)); os.IsNotExist(err) {
		t.Error("Nested log directory was not created")
	}
}
