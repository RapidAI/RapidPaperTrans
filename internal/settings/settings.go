// Package settings provides local settings file management.
// Settings are stored in settings.json in the program directory.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	// SettingsFileName is the name of the settings file
	SettingsFileName = "settings.json"
)

// LocalSettings represents settings stored in the program directory
type LocalSettings struct {
	Token string `json:"token"` // GitHub token for sharing
}

// Manager manages local settings file
type Manager struct {
	filePath string
	settings *LocalSettings
	mu       sync.RWMutex
}

// NewManager creates a new settings manager
// It looks for settings.json in the program's directory
func NewManager() (*Manager, error) {
	// Get the executable's directory
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exePath)
	
	// Settings file path
	filePath := filepath.Join(exeDir, SettingsFileName)
	
	m := &Manager{
		filePath: filePath,
		settings: &LocalSettings{},
	}
	
	// Try to load existing settings
	_ = m.Load() // Ignore error if file doesn't exist
	
	return m, nil
}

// NewManagerWithPath creates a new settings manager with a custom path
// Useful for testing
func NewManagerWithPath(filePath string) *Manager {
	m := &Manager{
		filePath: filePath,
		settings: &LocalSettings{},
	}
	_ = m.Load()
	return m
}

// Load loads settings from the file
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, use empty settings
			m.settings = &LocalSettings{}
			return nil
		}
		return err
	}
	
	var settings LocalSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		// Invalid JSON, use empty settings
		m.settings = &LocalSettings{}
		return err
	}
	
	m.settings = &settings
	return nil
}

// Save saves settings to the file
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	data, err := json.MarshalIndent(m.settings, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(m.filePath, data, 0600)
}

// GetToken returns the GitHub token
func (m *Manager) GetToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.Token
}

// SetToken sets the GitHub token and saves
func (m *Manager) SetToken(token string) error {
	m.mu.Lock()
	m.settings.Token = token
	m.mu.Unlock()
	
	return m.Save()
}

// GetFilePath returns the settings file path
func (m *Manager) GetFilePath() string {
	return m.filePath
}

// HasToken returns true if a token is configured
func (m *Manager) HasToken() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.Token != ""
}
