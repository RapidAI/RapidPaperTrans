package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewManagerWithPath(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "settings_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "settings.json")
	m := NewManagerWithPath(filePath)

	if m == nil {
		t.Fatal("expected non-nil manager")
	}

	if m.GetFilePath() != filePath {
		t.Errorf("expected file path %s, got %s", filePath, m.GetFilePath())
	}
}

func TestSetAndGetToken(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "settings_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "settings.json")
	m := NewManagerWithPath(filePath)

	// Initially no token
	if m.HasToken() {
		t.Error("expected no token initially")
	}

	// Set token
	testToken := "ghp_test_token_12345"
	if err := m.SetToken(testToken); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}

	// Verify token is set
	if !m.HasToken() {
		t.Error("expected token to be set")
	}

	if got := m.GetToken(); got != testToken {
		t.Errorf("expected token %s, got %s", testToken, got)
	}

	// Verify file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("expected settings file to be created")
	}

	// Create new manager and verify persistence
	m2 := NewManagerWithPath(filePath)
	if got := m2.GetToken(); got != testToken {
		t.Errorf("expected persisted token %s, got %s", testToken, got)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "settings_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "settings.json")
	
	// Write invalid JSON
	if err := os.WriteFile(filePath, []byte("invalid json"), 0600); err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}

	// Should not panic, should use empty settings
	m := NewManagerWithPath(filePath)
	if m.HasToken() {
		t.Error("expected no token with invalid JSON")
	}
}
