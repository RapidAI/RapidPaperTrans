package translator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig tests that DefaultConfig returns sensible defaults
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Test protection settings defaults
	if !config.ProtectMath {
		t.Error("ProtectMath should be true by default")
	}
	if !config.ProtectTables {
		t.Error("ProtectTables should be true by default")
	}
	if !config.ProtectComments {
		t.Error("ProtectComments should be true by default")
	}
	if config.ProtectedCommands == nil {
		t.Error("ProtectedCommands should not be nil")
	}
	if len(config.ProtectedCommands) != 0 {
		t.Error("ProtectedCommands should be empty by default")
	}

	// Test chunking settings defaults
	if config.MaxChunkSize != 4000 {
		t.Errorf("MaxChunkSize should be 4000, got %d", config.MaxChunkSize)
	}
	if config.ChunkOverlap != 100 {
		t.Errorf("ChunkOverlap should be 100, got %d", config.ChunkOverlap)
	}
	if !config.RespectEnvironments {
		t.Error("RespectEnvironments should be true by default")
	}

	// Test validation settings defaults
	if config.StrictValidation {
		t.Error("StrictValidation should be false by default")
	}
	if config.MinLengthRatio != 0.3 {
		t.Errorf("MinLengthRatio should be 0.3, got %f", config.MinLengthRatio)
	}
	if config.MaxLengthRatio != 3.0 {
		t.Errorf("MaxLengthRatio should be 3.0, got %f", config.MaxLengthRatio)
	}
	if config.MinChineseRatio != 0.05 {
		t.Errorf("MinChineseRatio should be 0.05, got %f", config.MinChineseRatio)
	}

	// Test recovery settings defaults
	if !config.AutoFixLineStructure {
		t.Error("AutoFixLineStructure should be true by default")
	}
	if !config.AutoFixBraces {
		t.Error("AutoFixBraces should be true by default")
	}
	if !config.AutoFixEnvironments {
		t.Error("AutoFixEnvironments should be true by default")
	}

	// Test prompt settings defaults
	if !config.IncludeExamples {
		t.Error("IncludeExamples should be true by default")
	}
	if !config.StrictLineCount {
		t.Error("StrictLineCount should be true by default")
	}
}

// TestValidateConfig_ValidConfig tests validation of a valid configuration
func TestValidateConfig_ValidConfig(t *testing.T) {
	config := DefaultConfig()
	result := ValidateConfig(config)

	if !result.IsValid {
		t.Errorf("Default config should be valid, got errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Valid config should have no errors, got %d", len(result.Errors))
	}
}

// TestValidateConfig_NilConfig tests validation of nil configuration
func TestValidateConfig_NilConfig(t *testing.T) {
	result := ValidateConfig(nil)

	if result.IsValid {
		t.Error("Nil config should be invalid")
	}
	if len(result.Errors) == 0 {
		t.Error("Nil config should have errors")
	}
}

// TestValidateConfig_InvalidMaxChunkSize tests validation of invalid MaxChunkSize
func TestValidateConfig_InvalidMaxChunkSize(t *testing.T) {
	tests := []struct {
		name         string
		maxChunkSize int
		chunkOverlap int // Need to set this to avoid overlap validation failure
		expectValid  bool
	}{
		{"zero", 0, 0, false},
		{"negative", -100, 0, false},
		{"too large", 200000, 100, false},
		{"valid small", 100, 10, true},
		{"valid large", 100000, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MaxChunkSize = tt.maxChunkSize
			config.ChunkOverlap = tt.chunkOverlap
			result := ValidateConfig(config)

			if result.IsValid != tt.expectValid {
				t.Errorf("MaxChunkSize=%d, ChunkOverlap=%d: expected valid=%v, got valid=%v, errors=%v",
					tt.maxChunkSize, tt.chunkOverlap, tt.expectValid, result.IsValid, result.Errors)
			}
		})
	}
}

// TestValidateConfig_InvalidChunkOverlap tests validation of invalid ChunkOverlap
func TestValidateConfig_InvalidChunkOverlap(t *testing.T) {
	tests := []struct {
		name         string
		chunkOverlap int
		maxChunkSize int
		expectValid  bool
	}{
		{"negative", -10, 4000, false},
		{"equal to max", 4000, 4000, false},
		{"greater than max", 5000, 4000, false},
		{"valid zero", 0, 4000, true},
		{"valid small", 100, 4000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.ChunkOverlap = tt.chunkOverlap
			config.MaxChunkSize = tt.maxChunkSize
			result := ValidateConfig(config)

			if result.IsValid != tt.expectValid {
				t.Errorf("ChunkOverlap=%d, MaxChunkSize=%d: expected valid=%v, got valid=%v, errors=%v",
					tt.chunkOverlap, tt.maxChunkSize, tt.expectValid, result.IsValid, result.Errors)
			}
		})
	}
}

// TestValidateConfig_InvalidLengthRatios tests validation of invalid length ratios
func TestValidateConfig_InvalidLengthRatios(t *testing.T) {
	tests := []struct {
		name           string
		minLengthRatio float64
		maxLengthRatio float64
		expectValid    bool
	}{
		{"min zero", 0, 3.0, false},
		{"min negative", -0.5, 3.0, false},
		{"min >= 1", 1.0, 3.0, false},
		{"max <= 1", 0.3, 1.0, false},
		{"max too large", 0.3, 150.0, false},
		{"min >= max", 0.5, 0.4, false},
		{"valid", 0.3, 3.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MinLengthRatio = tt.minLengthRatio
			config.MaxLengthRatio = tt.maxLengthRatio
			result := ValidateConfig(config)

			if result.IsValid != tt.expectValid {
				t.Errorf("MinLengthRatio=%f, MaxLengthRatio=%f: expected valid=%v, got valid=%v, errors=%v",
					tt.minLengthRatio, tt.maxLengthRatio, tt.expectValid, result.IsValid, result.Errors)
			}
		})
	}
}

// TestValidateConfig_InvalidMinChineseRatio tests validation of invalid MinChineseRatio
func TestValidateConfig_InvalidMinChineseRatio(t *testing.T) {
	tests := []struct {
		name            string
		minChineseRatio float64
		expectValid     bool
	}{
		{"negative", -0.1, false},
		{"too large", 1.5, false},
		{"valid zero", 0, true},
		{"valid small", 0.05, true},
		{"valid max", 1.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MinChineseRatio = tt.minChineseRatio
			result := ValidateConfig(config)

			if result.IsValid != tt.expectValid {
				t.Errorf("MinChineseRatio=%f: expected valid=%v, got valid=%v, errors=%v",
					tt.minChineseRatio, tt.expectValid, result.IsValid, result.Errors)
			}
		})
	}
}

// TestSaveAndLoadConfig tests saving and loading configuration
func TestSaveAndLoadConfig(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")

	// Create a custom config
	originalConfig := &FormatPreservationConfig{
		ProtectMath:          true,
		ProtectTables:        false,
		ProtectComments:      true,
		ProtectedCommands:    []string{"\\mycmd", "\\anothercmd"},
		MaxChunkSize:         5000,
		ChunkOverlap:         200,
		RespectEnvironments:  false,
		StrictValidation:     true,
		MinLengthRatio:       0.4,
		MaxLengthRatio:       2.5,
		MinChineseRatio:      0.1,
		AutoFixLineStructure: false,
		AutoFixBraces:        true,
		AutoFixEnvironments:  false,
		IncludeExamples:      false,
		StrictLineCount:      true,
	}

	// Save the config
	err := SaveConfig(originalConfig, configPath)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load the config
	loadedConfig, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Compare configs
	if !originalConfig.Equal(loadedConfig) {
		t.Errorf("Loaded config doesn't match original.\nOriginal: %s\nLoaded: %s",
			originalConfig.String(), loadedConfig.String())
	}
}

// TestLoadConfig_NonExistentFile tests loading from a non-existent file
func TestLoadConfig_NonExistentFile(t *testing.T) {
	config, err := LoadConfig("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("LoadConfig should return default config for non-existent file, got error: %v", err)
	}

	defaultConfig := DefaultConfig()
	if !config.Equal(defaultConfig) {
		t.Error("LoadConfig should return default config for non-existent file")
	}
}

// TestLoadConfig_InvalidJSON tests loading from an invalid JSON file
func TestLoadConfig_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(configPath, []byte("not valid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig should return error for invalid JSON")
	}
}

// TestLoadConfig_InvalidValues tests loading config with invalid values
func TestLoadConfig_InvalidValues(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid_values.json")

	// Write JSON with invalid values
	invalidJSON := `{
		"max_chunk_size": -100,
		"min_length_ratio": 0.3,
		"max_length_ratio": 3.0
	}`
	err := os.WriteFile(configPath, []byte(invalidJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig should return error for invalid config values")
	}
}

// TestSaveConfig_NilConfig tests saving nil configuration
func TestSaveConfig_NilConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nil_config.json")

	err := SaveConfig(nil, configPath)
	if err == nil {
		t.Error("SaveConfig should return error for nil config")
	}
}

// TestSaveConfig_InvalidConfig tests saving invalid configuration
func TestSaveConfig_InvalidConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid_config.json")

	invalidConfig := &FormatPreservationConfig{
		MaxChunkSize:   -100, // Invalid
		MinLengthRatio: 0.3,
		MaxLengthRatio: 3.0,
	}

	err := SaveConfig(invalidConfig, configPath)
	if err == nil {
		t.Error("SaveConfig should return error for invalid config")
	}
}

// TestSaveConfig_CreatesDirectory tests that SaveConfig creates parent directories
func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "subdir", "nested", "config.json")

	config := DefaultConfig()
	err := SaveConfig(config, configPath)
	if err != nil {
		t.Fatalf("SaveConfig failed to create directories: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created in nested directory")
	}
}

// TestMergeConfig_BothNil tests merging two nil configs
func TestMergeConfig_BothNil(t *testing.T) {
	result := MergeConfig(nil, nil)

	defaultConfig := DefaultConfig()
	if !result.Equal(defaultConfig) {
		t.Error("MergeConfig(nil, nil) should return default config")
	}
}

// TestMergeConfig_BaseNil tests merging with nil base
func TestMergeConfig_BaseNil(t *testing.T) {
	override := &FormatPreservationConfig{
		ProtectMath:    false,
		MaxChunkSize:   5000,
		MinLengthRatio: 0.4,
		MaxLengthRatio: 2.5,
		MinChineseRatio: 0.1,
	}

	result := MergeConfig(nil, override)

	if result.ProtectMath != false {
		t.Error("ProtectMath should be overridden to false")
	}
	if result.MaxChunkSize != 5000 {
		t.Errorf("MaxChunkSize should be 5000, got %d", result.MaxChunkSize)
	}
}

// TestMergeConfig_OverrideNil tests merging with nil override
func TestMergeConfig_OverrideNil(t *testing.T) {
	base := &FormatPreservationConfig{
		ProtectMath:    false,
		MaxChunkSize:   5000,
		MinLengthRatio: 0.4,
		MaxLengthRatio: 2.5,
		MinChineseRatio: 0.1,
	}

	result := MergeConfig(base, nil)

	if !result.Equal(base) {
		t.Error("MergeConfig with nil override should return copy of base")
	}

	// Verify it's a copy, not the same pointer
	if result == base {
		t.Error("MergeConfig should return a new config, not the same pointer")
	}
}

// TestMergeConfig_ProtectedCommands tests merging protected commands
func TestMergeConfig_ProtectedCommands(t *testing.T) {
	base := &FormatPreservationConfig{
		ProtectedCommands: []string{"\\cmd1", "\\cmd2"},
		MaxChunkSize:      4000,
		MinLengthRatio:    0.3,
		MaxLengthRatio:    3.0,
	}

	override := &FormatPreservationConfig{
		ProtectedCommands: []string{"\\cmd2", "\\cmd3"}, // cmd2 is duplicate
		MaxChunkSize:      4000,
		MinLengthRatio:    0.3,
		MaxLengthRatio:    3.0,
	}

	result := MergeConfig(base, override)

	// Should have cmd1, cmd2, cmd3 (no duplicates)
	if len(result.ProtectedCommands) != 3 {
		t.Errorf("Expected 3 protected commands, got %d: %v",
			len(result.ProtectedCommands), result.ProtectedCommands)
	}

	// Check all commands are present
	cmdSet := make(map[string]bool)
	for _, cmd := range result.ProtectedCommands {
		cmdSet[cmd] = true
	}
	if !cmdSet["\\cmd1"] || !cmdSet["\\cmd2"] || !cmdSet["\\cmd3"] {
		t.Errorf("Missing expected commands in result: %v", result.ProtectedCommands)
	}
}

// TestMergeConfig_NumericOverrides tests that non-zero numeric values override
func TestMergeConfig_NumericOverrides(t *testing.T) {
	base := DefaultConfig()
	override := &FormatPreservationConfig{
		MaxChunkSize:    8000,
		ChunkOverlap:    500,
		MinLengthRatio:  0.5,
		MaxLengthRatio:  4.0,
		MinChineseRatio: 0.15,
	}

	result := MergeConfig(base, override)

	if result.MaxChunkSize != 8000 {
		t.Errorf("MaxChunkSize should be 8000, got %d", result.MaxChunkSize)
	}
	if result.ChunkOverlap != 500 {
		t.Errorf("ChunkOverlap should be 500, got %d", result.ChunkOverlap)
	}
	if result.MinLengthRatio != 0.5 {
		t.Errorf("MinLengthRatio should be 0.5, got %f", result.MinLengthRatio)
	}
	if result.MaxLengthRatio != 4.0 {
		t.Errorf("MaxLengthRatio should be 4.0, got %f", result.MaxLengthRatio)
	}
	if result.MinChineseRatio != 0.15 {
		t.Errorf("MinChineseRatio should be 0.15, got %f", result.MinChineseRatio)
	}
}

// TestConfigEqual tests the Equal method
func TestConfigEqual(t *testing.T) {
	config1 := DefaultConfig()
	config2 := DefaultConfig()

	if !config1.Equal(config2) {
		t.Error("Two default configs should be equal")
	}

	// Test nil comparisons
	var nilConfig *FormatPreservationConfig
	if nilConfig.Equal(config1) {
		t.Error("nil should not equal non-nil config")
	}
	if config1.Equal(nilConfig) {
		t.Error("non-nil config should not equal nil")
	}
	if !nilConfig.Equal(nil) {
		t.Error("nil should equal nil")
	}

	// Test different values
	config2.MaxChunkSize = 9999
	if config1.Equal(config2) {
		t.Error("Configs with different MaxChunkSize should not be equal")
	}

	// Test different protected commands
	config3 := DefaultConfig()
	config3.ProtectedCommands = []string{"\\test"}
	if config1.Equal(config3) {
		t.Error("Configs with different ProtectedCommands should not be equal")
	}
}

// TestConfigString tests the String method
func TestConfigString(t *testing.T) {
	config := DefaultConfig()
	str := config.String()

	if str == "" {
		t.Error("String() should not return empty string")
	}

	// Check that key values are present in the string
	if !configContains(str, "ProtectMath:true") {
		t.Error("String() should contain ProtectMath value")
	}
	if !configContains(str, "MaxChunkSize:4000") {
		t.Error("String() should contain MaxChunkSize value")
	}

	// Test nil config
	var nilConfig *FormatPreservationConfig
	nilStr := nilConfig.String()
	if nilStr != "FormatPreservationConfig{nil}" {
		t.Errorf("nil config String() should return 'FormatPreservationConfig{nil}', got '%s'", nilStr)
	}
}

// TestConfigValidationError tests the ConfigValidationError type
func TestConfigValidationError(t *testing.T) {
	err := &ConfigValidationError{
		Field:   "MaxChunkSize",
		Value:   -100,
		Message: "must be greater than 0",
	}

	errStr := err.Error()
	if !configContains(errStr, "MaxChunkSize") {
		t.Error("Error string should contain field name")
	}
	if !configContains(errStr, "-100") {
		t.Error("Error string should contain value")
	}
	if !configContains(errStr, "must be greater than 0") {
		t.Error("Error string should contain message")
	}
}

// TestCopyConfig tests the copyConfig function
func TestCopyConfig(t *testing.T) {
	original := &FormatPreservationConfig{
		ProtectMath:       true,
		ProtectedCommands: []string{"\\cmd1", "\\cmd2"},
		MaxChunkSize:      5000,
		MinLengthRatio:    0.3,
		MaxLengthRatio:    3.0,
	}

	copied := copyConfig(original)

	// Verify it's a different pointer
	if copied == original {
		t.Error("copyConfig should return a new pointer")
	}

	// Verify values are equal
	if !copied.Equal(original) {
		t.Error("Copied config should equal original")
	}

	// Verify slice is a deep copy
	if len(copied.ProtectedCommands) > 0 && len(original.ProtectedCommands) > 0 {
		copied.ProtectedCommands[0] = "\\modified"
		if original.ProtectedCommands[0] == "\\modified" {
			t.Error("Modifying copied slice should not affect original")
		}
	}

	// Test nil input
	nilCopy := copyConfig(nil)
	if nilCopy != nil {
		t.Error("copyConfig(nil) should return nil")
	}
}

// Helper function to check if a string contains a substring
func configContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && configContainsHelper(s, substr))
}

func configContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
