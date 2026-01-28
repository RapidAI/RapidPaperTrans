package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"latex-translator/internal/types"
)

func TestNewConfigManager(t *testing.T) {
	t.Run("with custom path", func(t *testing.T) {
		customPath := "/tmp/test-config.json"
		cm, err := NewConfigManager(customPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}
		if cm.GetConfigPath() != customPath {
			t.Errorf("expected config path %s, got %s", customPath, cm.GetConfigPath())
		}
	})

	t.Run("with empty path uses default", func(t *testing.T) {
		cm, err := NewConfigManager("")
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}
		if cm.GetConfigPath() == "" {
			t.Error("expected non-empty config path")
		}
	})
}

func TestConfigManager_LoadSave(t *testing.T) {
	// Create a temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "test-config.json")

	t.Run("Load with non-existent file uses defaults", func(t *testing.T) {
		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		err = cm.Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		config := cm.GetConfig()
		if config.OpenAIModel != DefaultModel {
			t.Errorf("expected default model %s, got %s", DefaultModel, config.OpenAIModel)
		}
		if config.DefaultCompiler != DefaultCompiler {
			t.Errorf("expected default compiler %s, got %s", DefaultCompiler, config.DefaultCompiler)
		}
	})

	t.Run("Save creates config file", func(t *testing.T) {
		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		cm.SetConfig(&types.Config{
			OpenAIAPIKey:    "test-api-key",
			OpenAIModel:     "gpt-3.5-turbo",
			DefaultCompiler: "xelatex",
			WorkDirectory:   "/tmp/work",
		})

		err = cm.Save()
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("config file was not created")
		}
	})

	t.Run("Load reads saved config", func(t *testing.T) {
		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		err = cm.Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		config := cm.GetConfig()
		if config.OpenAIAPIKey != "test-api-key" {
			t.Errorf("expected API key 'test-api-key', got '%s'", config.OpenAIAPIKey)
		}
		if config.OpenAIModel != "gpt-3.5-turbo" {
			t.Errorf("expected model 'gpt-3.5-turbo', got '%s'", config.OpenAIModel)
		}
		if config.DefaultCompiler != "xelatex" {
			t.Errorf("expected compiler 'xelatex', got '%s'", config.DefaultCompiler)
		}
		if config.WorkDirectory != "/tmp/work" {
			t.Errorf("expected work directory '/tmp/work', got '%s'", config.WorkDirectory)
		}
	})

	t.Run("Load with invalid JSON uses defaults", func(t *testing.T) {
		invalidConfigPath := filepath.Join(tmpDir, "invalid-config.json")
		err := os.WriteFile(invalidConfigPath, []byte("invalid json"), 0644)
		if err != nil {
			t.Fatalf("failed to write invalid config: %v", err)
		}

		cm, err := NewConfigManager(invalidConfigPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		err = cm.Load()
		if err != nil {
			t.Fatalf("Load should not fail with invalid JSON: %v", err)
		}

		config := cm.GetConfig()
		if config.OpenAIModel != DefaultModel {
			t.Errorf("expected default model after invalid JSON, got %s", config.OpenAIModel)
		}
	})
}

func TestConfigManager_GetAPIKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "test-config.json")

	t.Run("returns config file value when set", func(t *testing.T) {
		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		cm.SetConfig(&types.Config{
			OpenAIAPIKey: "config-api-key",
		})

		apiKey := cm.GetAPIKey()
		if apiKey != "config-api-key" {
			t.Errorf("expected 'config-api-key', got '%s'", apiKey)
		}
	})

	t.Run("falls back to environment variable", func(t *testing.T) {
		// Clear any existing env var first
		originalEnv := os.Getenv(EnvOpenAIAPIKey)
		defer os.Setenv(EnvOpenAIAPIKey, originalEnv)

		os.Setenv(EnvOpenAIAPIKey, "env-api-key")

		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		// Set empty API key in config
		cm.SetConfig(&types.Config{
			OpenAIAPIKey: "",
		})

		apiKey := cm.GetAPIKey()
		if apiKey != "env-api-key" {
			t.Errorf("expected 'env-api-key', got '%s'", apiKey)
		}
	})

	t.Run("config file takes precedence over env var", func(t *testing.T) {
		originalEnv := os.Getenv(EnvOpenAIAPIKey)
		defer os.Setenv(EnvOpenAIAPIKey, originalEnv)

		os.Setenv(EnvOpenAIAPIKey, "env-api-key")

		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		cm.SetConfig(&types.Config{
			OpenAIAPIKey: "config-api-key",
		})

		apiKey := cm.GetAPIKey()
		if apiKey != "config-api-key" {
			t.Errorf("expected 'config-api-key' (from config), got '%s'", apiKey)
		}
	})
}

func TestConfigManager_SetAPIKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "test-config.json")

	t.Run("SetAPIKey saves to file", func(t *testing.T) {
		cm, err := NewConfigManager(configPath)
		if err != nil {
			t.Fatalf("NewConfigManager failed: %v", err)
		}

		err = cm.SetAPIKey("new-api-key")
		if err != nil {
			t.Fatalf("SetAPIKey failed: %v", err)
		}

		// Verify the key is set in memory
		if cm.GetAPIKey() != "new-api-key" {
			t.Errorf("expected 'new-api-key', got '%s'", cm.GetAPIKey())
		}

		// Verify the file was saved
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read config file: %v", err)
		}

		var savedConfig types.Config
		if err := json.Unmarshal(data, &savedConfig); err != nil {
			t.Fatalf("failed to parse saved config: %v", err)
		}

		if savedConfig.OpenAIAPIKey != "new-api-key" {
			t.Errorf("expected saved API key 'new-api-key', got '%s'", savedConfig.OpenAIAPIKey)
		}
	})
}

func TestConfigManager_GettersWithDefaults(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "test-config.json")

	cm, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	t.Run("GetModel returns default when empty", func(t *testing.T) {
		cm.SetConfig(&types.Config{OpenAIModel: ""})
		if cm.GetModel() != DefaultModel {
			t.Errorf("expected default model %s, got %s", DefaultModel, cm.GetModel())
		}
	})

	t.Run("GetModel returns configured value", func(t *testing.T) {
		cm.SetConfig(&types.Config{OpenAIModel: "gpt-3.5-turbo"})
		if cm.GetModel() != "gpt-3.5-turbo" {
			t.Errorf("expected 'gpt-3.5-turbo', got %s", cm.GetModel())
		}
	})

	t.Run("GetDefaultCompiler returns default when empty", func(t *testing.T) {
		cm.SetConfig(&types.Config{DefaultCompiler: ""})
		if cm.GetDefaultCompiler() != DefaultCompiler {
			t.Errorf("expected default compiler %s, got %s", DefaultCompiler, cm.GetDefaultCompiler())
		}
	})

	t.Run("GetDefaultCompiler returns configured value", func(t *testing.T) {
		cm.SetConfig(&types.Config{DefaultCompiler: "xelatex"})
		if cm.GetDefaultCompiler() != "xelatex" {
			t.Errorf("expected 'xelatex', got %s", cm.GetDefaultCompiler())
		}
	})

	t.Run("GetWorkDirectory returns configured value", func(t *testing.T) {
		cm.SetConfig(&types.Config{WorkDirectory: "/custom/work"})
		if cm.GetWorkDirectory() != "/custom/work" {
			t.Errorf("expected '/custom/work', got %s", cm.GetWorkDirectory())
		}
	})
}

func TestConfigManager_SaveCreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a nested path that doesn't exist
	configPath := filepath.Join(tmpDir, "nested", "dir", "config.json")

	cm, err := NewConfigManager(configPath)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}

	err = cm.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the directory and file were created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created in nested directory")
	}
}
