// Package config provides configuration management for the LaTeX translator application.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

const (
	// DefaultConfigFileName is the default configuration file name
	DefaultConfigFileName = "latex-translator-config.json"
	// EnvOpenAIAPIKey is the environment variable name for OpenAI API key
	EnvOpenAIAPIKey = "OPENAI_API_KEY"
	// EnvOpenAIBaseURL is the environment variable name for OpenAI base URL
	EnvOpenAIBaseURL = "OPENAI_BASE_URL"
	// DefaultBaseURL is the default OpenAI API base URL
	DefaultBaseURL = "https://api.openai.com/v1"
	// DefaultModel is the default OpenAI model to use
	DefaultModel = "gpt-4"
	// DefaultContextWindow is the default context window size in tokens
	// Used for both LaTeX and PDF translation batch size control
	DefaultContextWindow = 8192
	// DefaultCompiler is the default LaTeX compiler
	DefaultCompiler = "pdflatex"
	// DefaultConcurrency is the default translation concurrency
	// Used for both LaTeX and PDF translation concurrent batch processing
	DefaultConcurrency = 3
	// DefaultLibraryPageSize is the default number of papers to display per page in library browser
	DefaultLibraryPageSize = 20
)

// ConfigManager manages application configuration
type ConfigManager struct {
	configPath string
	config     *types.Config
}

// NewConfigManager creates a new ConfigManager with the specified config path.
// If configPath is empty, it uses the default path in user's home directory.
func NewConfigManager(configPath string) (*ConfigManager, error) {
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			logger.Error("failed to get user home directory", err)
			return nil, types.NewAppError(types.ErrConfig, "failed to get user home directory", err)
		}
		configPath = filepath.Join(homeDir, ".config", "latex-translator", DefaultConfigFileName)
	}

	logger.Info("ConfigManager initialized", logger.String("configPath", configPath))
	return &ConfigManager{
		configPath: configPath,
		config:     defaultConfig(),
	}, nil
}

// defaultConfig returns a Config with default values
func defaultConfig() *types.Config {
	return &types.Config{
		OpenAIAPIKey:       "",
		OpenAIBaseURL:      DefaultBaseURL,
		OpenAIModel:        DefaultModel,
		ContextWindow:      DefaultContextWindow,
		DefaultCompiler:    DefaultCompiler,
		WorkDirectory:      "",
		Concurrency:        DefaultConcurrency,
		LibraryPageSize:    DefaultLibraryPageSize,
		SharePromptEnabled: true, // 默认开启分享提示
	}
}

// Load loads configuration from the config file.
// If the file doesn't exist, it uses default values.
// Environment variables take precedence for API key if config file value is empty.
func (m *ConfigManager) Load() error {
	logger.Debug("loading configuration", logger.String("path", m.configPath))

	// Try to read the config file
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, use defaults
			logger.Info("config file not found, using defaults", logger.String("path", m.configPath))
			m.config = defaultConfig()
		} else {
			logger.Error("failed to read config file", err, logger.String("path", m.configPath))
			return types.NewAppError(types.ErrConfig, "failed to read config file", err)
		}
	} else {
		// Parse the config file
		config := &types.Config{}
		if err := json.Unmarshal(data, config); err != nil {
			// Invalid JSON, use defaults
			logger.Warn("invalid config file format, using defaults", logger.String("path", m.configPath), logger.Err(err))
			m.config = defaultConfig()
		} else {
			logger.Info("configuration loaded successfully", 
				logger.String("path", m.configPath),
				logger.Int("apiKeyLength", len(config.OpenAIAPIKey)),
				logger.String("baseURL", config.OpenAIBaseURL),
				logger.String("model", config.OpenAIModel))
			m.config = config
		}
	}

	// Apply defaults for empty fields
	if m.config.OpenAIModel == "" {
		m.config.OpenAIModel = DefaultModel
	}
	if m.config.OpenAIBaseURL == "" {
		m.config.OpenAIBaseURL = DefaultBaseURL
	}
	if m.config.ContextWindow == 0 {
		m.config.ContextWindow = DefaultContextWindow
	}
	if m.config.DefaultCompiler == "" {
		m.config.DefaultCompiler = DefaultCompiler
	}
	if m.config.Concurrency == 0 {
		m.config.Concurrency = DefaultConcurrency
	}

	return nil
}

// Save saves the current configuration to the config file.
func (m *ConfigManager) Save() error {
	logger.Debug("saving configuration", logger.String("path", m.configPath))

	// Ensure the directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("failed to create config directory", err, logger.String("dir", dir))
		return types.NewAppError(types.ErrConfig, "failed to create config directory", err)
	}

	// Marshal config to JSON
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		logger.Error("failed to marshal config", err)
		return types.NewAppError(types.ErrConfig, "failed to marshal config", err)
	}

	// Write to file
	if err := os.WriteFile(m.configPath, data, 0600); err != nil {
		logger.Error("failed to write config file", err, logger.String("path", m.configPath))
		return types.NewAppError(types.ErrConfig, "failed to write config file", err)
	}

	logger.Info("configuration saved successfully", logger.String("path", m.configPath))
	return nil
}

// GetAPIKey returns the OpenAI API key.
// It first checks the config file value, then falls back to the environment variable.
func (m *ConfigManager) GetAPIKey() string {
	// First check config file value
	if m.config != nil && m.config.OpenAIAPIKey != "" {
		return m.config.OpenAIAPIKey
	}

	// Fall back to environment variable
	return os.Getenv(EnvOpenAIAPIKey)
}

// SetAPIKey sets the OpenAI API key and saves the configuration.
func (m *ConfigManager) SetAPIKey(key string) error {
	logger.Info("setting API key")
	if m.config == nil {
		m.config = defaultConfig()
	}
	m.config.OpenAIAPIKey = key
	return m.Save()
}

// GetConfig returns the current configuration.
func (m *ConfigManager) GetConfig() *types.Config {
	if m.config == nil {
		return defaultConfig()
	}
	return m.config
}

// SetConfig sets the entire configuration.
func (m *ConfigManager) SetConfig(config *types.Config) {
	m.config = config
}

// GetConfigPath returns the path to the config file.
func (m *ConfigManager) GetConfigPath() string {
	return m.configPath
}

// GetModel returns the OpenAI model to use.
func (m *ConfigManager) GetModel() string {
	if m.config != nil && m.config.OpenAIModel != "" {
		return m.config.OpenAIModel
	}
	return DefaultModel
}

// GetDefaultCompiler returns the default LaTeX compiler.
func (m *ConfigManager) GetDefaultCompiler() string {
	if m.config != nil && m.config.DefaultCompiler != "" {
		return m.config.DefaultCompiler
	}
	return DefaultCompiler
}

// GetWorkDirectory returns the work directory.
func (m *ConfigManager) GetWorkDirectory() string {
	if m.config != nil {
		return m.config.WorkDirectory
	}
	return ""
}

// GetBaseURL returns the OpenAI API base URL.
// It first checks the config file value, then falls back to the environment variable.
func (m *ConfigManager) GetBaseURL() string {
	// First check config file value
	if m.config != nil && m.config.OpenAIBaseURL != "" {
		return m.config.OpenAIBaseURL
	}

	// Fall back to environment variable
	envURL := os.Getenv(EnvOpenAIBaseURL)
	if envURL != "" {
		return envURL
	}

	return DefaultBaseURL
}

// UpdateConfig updates the configuration with new values and saves it.
func (m *ConfigManager) UpdateConfig(apiKey, baseURL, model string, contextWindow int, compiler, workDir string, concurrency int, libraryPageSize int, sharePromptEnabled bool) error {
	logger.Info("updating configuration")
	if m.config == nil {
		m.config = defaultConfig()
	}

	// Update fields if provided
	if apiKey != "" {
		m.config.OpenAIAPIKey = apiKey
	}
	if baseURL != "" {
		m.config.OpenAIBaseURL = baseURL
	}
	if model != "" {
		m.config.OpenAIModel = model
	}
	if contextWindow > 0 {
		m.config.ContextWindow = contextWindow
	}
	if compiler != "" {
		m.config.DefaultCompiler = compiler
	}
	if workDir != "" {
		m.config.WorkDirectory = workDir
	}
	if concurrency > 0 {
		m.config.Concurrency = concurrency
	}
	if libraryPageSize > 0 {
		m.config.LibraryPageSize = libraryPageSize
	}
	// SharePromptEnabled is a bool, always update it
	m.config.SharePromptEnabled = sharePromptEnabled

	return m.Save()
}

// GetContextWindow returns the context window size.
// This value is used for both LaTeX and PDF translation to control batch size.
// For PDF translation, it determines how many text blocks are merged into a single API call.
// Requirements: 3.2 - 根据 LLM 上下文窗口大小动态调整批次大小
func (m *ConfigManager) GetContextWindow() int {
	if m.config != nil && m.config.ContextWindow > 0 {
		return m.config.ContextWindow
	}
	return DefaultContextWindow
}

// GetLastInput returns the last input value.
func (m *ConfigManager) GetLastInput() string {
	if m.config != nil {
		return m.config.LastInput
	}
	return ""
}

// SetLastInput sets the last input value and saves the configuration.
func (m *ConfigManager) SetLastInput(input string) {
	if m.config == nil {
		m.config = defaultConfig()
	}
	m.config.LastInput = input
	// Save silently, don't fail if it doesn't work
	_ = m.Save()
}

// GetConcurrency returns the translation concurrency.
// This value is used for both LaTeX and PDF translation to control parallel processing.
// For PDF translation, it determines how many batches are processed concurrently.
// Requirements: 3.6 - 支持并发处理多个批次以提升速度
func (m *ConfigManager) GetConcurrency() int {
	if m.config != nil && m.config.Concurrency > 0 {
		return m.config.Concurrency
	}
	return DefaultConcurrency
}

// GetLibraryPageSize returns the number of papers to display per page in library browser
func (m *ConfigManager) GetLibraryPageSize() int {
	if m.config != nil && m.config.LibraryPageSize > 0 {
		return m.config.LibraryPageSize
	}
	return DefaultLibraryPageSize
}
