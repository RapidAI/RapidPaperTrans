// Package config provides configuration management for the LaTeX translator application.
// All configuration is stored in a single file: ~/.config/RapidPaperTrans/config.json
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"latex-translator/internal/license"
	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

const (
	// DefaultConfigFileName is the default configuration file name
	DefaultConfigFileName = "config.json"
	// AppName is the application name used for config directory
	AppName = "RapidPaperTrans"
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
	// localEncryptionSecret is the app-specific secret for local encryption
	localEncryptionSecret = "RapidPaperTrans-Local-2024"
)

// ConfigManager manages application configuration
type ConfigManager struct {
	configPath string
	config     *types.Config
	mu         sync.RWMutex
}

// getConfigDir returns the config directory for the application
// All platforms: ~/.config/RapidPaperTrans
func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", AppName), nil
}

// GetConfigDir returns the config directory (exported for external use)
func GetConfigDir() (string, error) {
	return getConfigDir()
}

// getDeviceID generates a stable device identifier based on hardware characteristics
// This is used to bind the license to a specific machine
func getDeviceID() string {
	var parts []string

	// Get hostname
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		parts = append(parts, "host:"+hostname)
	}

	// Get username
	if username := os.Getenv("USERNAME"); username != "" {
		// Windows
		parts = append(parts, "user:"+username)
	} else if username := os.Getenv("USER"); username != "" {
		// Unix/Linux/macOS
		parts = append(parts, "user:"+username)
	}

	// Get MAC addresses from network interfaces
	if interfaces, err := net.Interfaces(); err == nil {
		var macs []string
		for _, iface := range interfaces {
			// Skip loopback and interfaces without MAC
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			mac := iface.HardwareAddr.String()
			if mac != "" && mac != "00:00:00:00:00:00" {
				macs = append(macs, mac)
			}
		}
		// Sort MACs for consistency
		sort.Strings(macs)
		for _, mac := range macs {
			parts = append(parts, "mac:"+mac)
		}
	}

	// If we couldn't get any identifiers, use a fallback
	if len(parts) == 0 {
		// Use home directory as fallback
		if home, err := os.UserHomeDir(); err == nil {
			parts = append(parts, "home:"+home)
		}
	}

	// Combine all parts and hash
	combined := strings.Join(parts, "|")
	hash := md5.Sum([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// GetDeviceID returns the device identifier for the current machine (exported)
func GetDeviceID() string {
	return getDeviceID()
}

// NewConfigManager creates a new ConfigManager.
// Configuration is stored in the system config directory.
func NewConfigManager(configPath string) (*ConfigManager, error) {
	// If configPath is provided and is just a filename, use it in the system config dir
	// If it's empty or a full path, handle accordingly
	var finalPath string
	
	if configPath == "" {
		// Use default path in system config directory
		configDir, err := getConfigDir()
		if err != nil {
			logger.Error("failed to get config directory", err)
			return nil, types.NewAppError(types.ErrConfig, "failed to get config directory", err)
		}
		finalPath = filepath.Join(configDir, DefaultConfigFileName)
	} else if filepath.IsAbs(configPath) {
		// Absolute path provided, use as-is
		finalPath = configPath
	} else {
		// Relative path or just filename - use in system config directory
		configDir, err := getConfigDir()
		if err != nil {
			logger.Error("failed to get config directory", err)
			return nil, types.NewAppError(types.ErrConfig, "failed to get config directory", err)
		}
		finalPath = filepath.Join(configDir, filepath.Base(configPath))
	}

	// Ensure the directory exists
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		logger.Error("failed to create config directory", err, logger.String("dir", dir))
		return nil, types.NewAppError(types.ErrConfig, "failed to create config directory", err)
	}

	logger.Info("ConfigManager initialized", logger.String("configPath", finalPath))
	m := &ConfigManager{
		configPath: finalPath,
		config:     defaultConfig(),
	}
	
	// Load existing config
	_ = m.Load()
	
	return m, nil
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

// MaxHistoryItems is the maximum number of history items to keep
const MaxHistoryItems = 50

// GetInputHistory returns the input history list.
func (m *ConfigManager) GetInputHistory() []types.InputHistoryItem {
	if m.config != nil && m.config.InputHistory != nil {
		return m.config.InputHistory
	}
	return []types.InputHistoryItem{}
}

// AddInputHistory adds an input to the history list.
// It avoids duplicates and keeps the list within MaxHistoryItems.
func (m *ConfigManager) AddInputHistory(input string, inputType string) {
	if m.config == nil {
		m.config = defaultConfig()
	}
	if m.config.InputHistory == nil {
		m.config.InputHistory = []types.InputHistoryItem{}
	}

	// Remove existing entry with the same input (to move it to the top)
	newHistory := []types.InputHistoryItem{}
	for _, item := range m.config.InputHistory {
		if item.Input != input {
			newHistory = append(newHistory, item)
		}
	}

	// Add new entry at the beginning
	newItem := types.InputHistoryItem{
		Input:     input,
		Timestamp: currentTimeMillis(),
		Type:      inputType,
	}
	m.config.InputHistory = append([]types.InputHistoryItem{newItem}, newHistory...)

	// Trim to max items
	if len(m.config.InputHistory) > MaxHistoryItems {
		m.config.InputHistory = m.config.InputHistory[:MaxHistoryItems]
	}

	// Save silently
	_ = m.Save()
}

// RemoveInputHistory removes a specific input from history.
func (m *ConfigManager) RemoveInputHistory(input string) {
	if m.config == nil || m.config.InputHistory == nil {
		return
	}

	newHistory := []types.InputHistoryItem{}
	for _, item := range m.config.InputHistory {
		if item.Input != input {
			newHistory = append(newHistory, item)
		}
	}
	m.config.InputHistory = newHistory

	// Save silently
	_ = m.Save()
}

// ClearInputHistory clears all input history.
func (m *ConfigManager) ClearInputHistory() {
	if m.config == nil {
		return
	}
	m.config.InputHistory = []types.InputHistoryItem{}
	_ = m.Save()
}

// currentTimeMillis returns the current time in milliseconds.
func currentTimeMillis() int64 {
	return time.Now().UnixMilli()
}

// ============================================================================
// License/Authorization Methods
// ============================================================================

// deriveLocalKey derives a local encryption key from the serial number and device ID
// Uses SHA-256(SN + deviceID + app-secret) to create a 32-byte key
func deriveLocalKey(serialNumber string) []byte {
	deviceID := getDeviceID()
	hash := sha256.Sum256([]byte(serialNumber + deviceID + localEncryptionSecret))
	return hash[:]
}

// encryptLicenseInfo encrypts license info using AES-256-GCM
func encryptLicenseInfo(info *license.LicenseInfo, serialNumber string) (string, error) {
	// Serialize to JSON
	plaintext, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	// Derive key
	key := deriveLocalKey(serialNumber)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Base64 encode
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptLicenseInfo decrypts license info using AES-256-GCM
func decryptLicenseInfo(encryptedData, serialNumber string) (*license.LicenseInfo, error) {
	// Base64 decode
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	// Derive key
	key := deriveLocalKey(serialNumber)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, err
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// Parse JSON
	var info license.LicenseInfo
	if err := json.Unmarshal(plaintext, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// GetWorkMode returns the current work mode
func (m *ConfigManager) GetWorkMode() license.WorkMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config != nil {
		return license.WorkMode(m.config.WorkMode)
	}
	return ""
}

// SetWorkMode sets the work mode and saves
func (m *ConfigManager) SetWorkMode(mode license.WorkMode) error {
	m.mu.Lock()
	if m.config == nil {
		m.config = defaultConfig()
	}
	m.config.WorkMode = string(mode)
	m.mu.Unlock()

	return m.Save()
}

// GetLicenseInfo returns the license information
// For commercial mode, decrypts the stored encrypted data
func (m *ConfigManager) GetLicenseInfo() *license.LicenseInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return nil
	}

	// No encrypted data or serial number
	if m.config.EncryptedLicenseInfo == "" || m.config.SerialNumber == "" {
		return nil
	}

	// Decrypt the license info
	info, err := decryptLicenseInfo(m.config.EncryptedLicenseInfo, m.config.SerialNumber)
	if err != nil {
		return nil
	}

	// Ensure serial number is set (it's stored separately for decryption key)
	if info != nil && info.SerialNumber == "" {
		info.SerialNumber = m.config.SerialNumber
	}

	return info
}

// SetLicenseInfo sets the license information and saves
// For commercial mode, encrypts the data before saving
func (m *ConfigManager) SetLicenseInfo(info *license.LicenseInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config == nil {
		m.config = defaultConfig()
	}

	if info == nil {
		m.config.SerialNumber = ""
		m.config.EncryptedLicenseInfo = ""
		return m.saveUnlocked()
	}

	// Encrypt the license info
	encrypted, err := encryptLicenseInfo(info, info.SerialNumber)
	if err != nil {
		return err
	}

	m.config.SerialNumber = info.SerialNumber
	m.config.EncryptedLicenseInfo = encrypted

	return m.saveUnlocked()
}

// saveUnlocked saves settings without acquiring the lock (caller must hold lock)
func (m *ConfigManager) saveUnlocked() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.configPath, data, 0600)
}

// HasValidLicense checks if there is a valid (non-expired) commercial license
func (m *ConfigManager) HasValidLicense() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return false
	}

	// Check if work mode is commercial
	if m.config.WorkMode != string(license.WorkModeCommercial) {
		return false
	}

	// Check if encrypted license info exists
	if m.config.EncryptedLicenseInfo == "" || m.config.SerialNumber == "" {
		return false
	}

	// Decrypt and check license info
	licenseInfo, err := decryptLicenseInfo(m.config.EncryptedLicenseInfo, m.config.SerialNumber)
	if err != nil || licenseInfo == nil {
		return false
	}

	// Check if activation data exists
	if licenseInfo.ActivationData == nil {
		return false
	}

	// Check if license is not expired
	client := license.NewClient()
	return !client.IsExpired(licenseInfo.ActivationData)
}

// HasWorkMode returns true if a work mode has been configured
func (m *ConfigManager) HasWorkMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config != nil && m.config.WorkMode != ""
}

// NeedsWorkModeSelection returns true if the user needs to select a work mode
func (m *ConfigManager) NeedsWorkModeSelection() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil || m.config.WorkMode == "" {
		return true
	}

	// Opensource mode - no selection needed
	if m.config.WorkMode == string(license.WorkModeOpenSource) {
		return false
	}

	// Commercial mode - check if license is valid
	if m.config.WorkMode == string(license.WorkModeCommercial) {
		if m.config.EncryptedLicenseInfo == "" || m.config.SerialNumber == "" {
			return true
		}

		licenseInfo, err := decryptLicenseInfo(m.config.EncryptedLicenseInfo, m.config.SerialNumber)
		if err != nil || licenseInfo == nil || licenseInfo.ActivationData == nil {
			return true
		}

		client := license.NewClient()
		if client.IsExpired(licenseInfo.ActivationData) {
			return true
		}
	}

	return false
}

// WorkModeStatus represents the detailed status of work mode configuration
type WorkModeStatus struct {
	HasWorkMode         bool             `json:"has_work_mode"`
	WorkMode            license.WorkMode `json:"work_mode"`
	NeedsSelection      bool             `json:"needs_selection"`
	IsCommercial        bool             `json:"is_commercial"`
	IsOpenSource        bool             `json:"is_open_source"`
	HasValidLicense     bool             `json:"has_valid_license"`
	LicenseExpired      bool             `json:"license_expired"`
	LicenseExpiringSoon bool             `json:"license_expiring_soon"`
	DaysUntilExpiry     int              `json:"days_until_expiry"`
}

// GetWorkModeStatus returns detailed status information about the work mode configuration
func (m *ConfigManager) GetWorkModeStatus() WorkModeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := WorkModeStatus{
		HasWorkMode:     m.config != nil && m.config.WorkMode != "",
		WorkMode:        license.WorkMode(m.config.WorkMode),
		NeedsSelection:  false,
		IsCommercial:    m.config != nil && m.config.WorkMode == string(license.WorkModeCommercial),
		IsOpenSource:    m.config != nil && m.config.WorkMode == string(license.WorkModeOpenSource),
		HasValidLicense: false,
		LicenseExpired:  false,
		LicenseExpiringSoon: false,
		DaysUntilExpiry: -1,
	}

	if !status.HasWorkMode {
		status.NeedsSelection = true
		return status
	}

	if status.IsOpenSource {
		return status
	}

	if status.IsCommercial {
		if m.config.EncryptedLicenseInfo == "" || m.config.SerialNumber == "" {
			status.NeedsSelection = true
			return status
		}

		licenseInfo, err := decryptLicenseInfo(m.config.EncryptedLicenseInfo, m.config.SerialNumber)
		if err != nil || licenseInfo == nil || licenseInfo.ActivationData == nil {
			status.NeedsSelection = true
			return status
		}

		client := license.NewClient()
		activationData := licenseInfo.ActivationData

		status.LicenseExpired = client.IsExpired(activationData)
		status.DaysUntilExpiry = client.DaysUntilExpiry(activationData)
		status.LicenseExpiringSoon = status.DaysUntilExpiry >= 0 && status.DaysUntilExpiry <= 7
		status.HasValidLicense = !status.LicenseExpired

		if status.LicenseExpired {
			status.NeedsSelection = true
		}
	}

	return status
}

// GetGitHubToken returns the GitHub token (for sharing feature)
func (m *ConfigManager) GetGitHubToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config != nil {
		return m.config.GitHubToken
	}
	return ""
}

// SetGitHubToken sets the GitHub token and saves
func (m *ConfigManager) SetGitHubToken(token string) error {
	m.mu.Lock()
	if m.config == nil {
		m.config = defaultConfig()
	}
	m.config.GitHubToken = token
	m.mu.Unlock()

	return m.Save()
}

// HasGitHubToken returns true if a GitHub token is configured
func (m *ConfigManager) HasGitHubToken() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config != nil && m.config.GitHubToken != ""
}
