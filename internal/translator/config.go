// Package translator provides functionality for translating LaTeX documents using OpenAI API.
package translator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FormatPreservationConfig 格式保护配置
// This configuration controls how LaTeX format preservation works during translation.
// Validates: All Requirements
type FormatPreservationConfig struct {
	// 保护设置 (Protection Settings)
	ProtectMath       bool     `json:"protect_math"`       // 保护数学环境
	ProtectTables     bool     `json:"protect_tables"`     // 保护表格
	ProtectComments   bool     `json:"protect_comments"`   // 保护注释
	ProtectedCommands []string `json:"protected_commands"` // 额外保护的命令

	// 分块设置 (Chunking Settings)
	MaxChunkSize        int  `json:"max_chunk_size"`        // 最大分块大小
	ChunkOverlap        int  `json:"chunk_overlap"`         // 分块重叠大小
	RespectEnvironments bool `json:"respect_environments"` // 尊重环境边界

	// 验证设置 (Validation Settings)
	StrictValidation bool    `json:"strict_validation"` // 严格验证模式
	MinLengthRatio   float64 `json:"min_length_ratio"`  // 最小长度比
	MaxLengthRatio   float64 `json:"max_length_ratio"`  // 最大长度比
	MinChineseRatio  float64 `json:"min_chinese_ratio"` // 最小中文比例

	// 恢复设置 (Recovery Settings)
	AutoFixLineStructure bool `json:"auto_fix_line_structure"` // 自动修复行结构
	AutoFixBraces        bool `json:"auto_fix_braces"`         // 自动修复大括号
	AutoFixEnvironments  bool `json:"auto_fix_environments"`   // 自动修复环境

	// Prompt设置 (Prompt Settings)
	IncludeExamples bool `json:"include_examples"` // 在prompt中包含示例
	StrictLineCount bool `json:"strict_line_count"` // 要求严格行数匹配
}

// DefaultConfig 返回默认配置
// Returns a configuration with sensible defaults for LaTeX format preservation.
func DefaultConfig() *FormatPreservationConfig {
	return &FormatPreservationConfig{
		// Protection settings - all enabled by default
		ProtectMath:       true,
		ProtectTables:     true,
		ProtectComments:   true,
		ProtectedCommands: []string{},

		// Chunking settings
		MaxChunkSize:        4000,
		ChunkOverlap:        100,
		RespectEnvironments: true,

		// Validation settings
		StrictValidation: false,
		MinLengthRatio:   0.3,
		MaxLengthRatio:   3.0,
		MinChineseRatio:  0.05,

		// Recovery settings - all enabled by default
		AutoFixLineStructure: true,
		AutoFixBraces:        true,
		AutoFixEnvironments:  true,

		// Prompt settings
		IncludeExamples: true,
		StrictLineCount: true,
	}
}

// ConfigValidationError represents a configuration validation error
type ConfigValidationError struct {
	Field   string // The field that failed validation
	Value   interface{} // The invalid value
	Message string // Description of the error
}

func (e *ConfigValidationError) Error() string {
	return fmt.Sprintf("config validation error: field '%s' with value '%v': %s", e.Field, e.Value, e.Message)
}

// ConfigValidationResult holds the result of config validation
type ConfigValidationResult struct {
	IsValid bool
	Errors  []*ConfigValidationError
}

// ValidateConfig validates the configuration values and returns validation result.
// It checks that all numeric values are within acceptable ranges.
func ValidateConfig(config *FormatPreservationConfig) *ConfigValidationResult {
	result := &ConfigValidationResult{
		IsValid: true,
		Errors:  make([]*ConfigValidationError, 0),
	}

	if config == nil {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "config",
			Value:   nil,
			Message: "configuration cannot be nil",
		})
		return result
	}

	// Validate MaxChunkSize
	if config.MaxChunkSize <= 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MaxChunkSize",
			Value:   config.MaxChunkSize,
			Message: "must be greater than 0",
		})
	} else if config.MaxChunkSize > 100000 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MaxChunkSize",
			Value:   config.MaxChunkSize,
			Message: "must not exceed 100000",
		})
	}

	// Validate ChunkOverlap
	if config.ChunkOverlap < 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "ChunkOverlap",
			Value:   config.ChunkOverlap,
			Message: "must be non-negative",
		})
	} else if config.ChunkOverlap >= config.MaxChunkSize {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "ChunkOverlap",
			Value:   config.ChunkOverlap,
			Message: "must be less than MaxChunkSize",
		})
	}

	// Validate MinLengthRatio
	if config.MinLengthRatio <= 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MinLengthRatio",
			Value:   config.MinLengthRatio,
			Message: "must be greater than 0",
		})
	} else if config.MinLengthRatio >= 1.0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MinLengthRatio",
			Value:   config.MinLengthRatio,
			Message: "must be less than 1.0",
		})
	}

	// Validate MaxLengthRatio
	if config.MaxLengthRatio <= 1.0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MaxLengthRatio",
			Value:   config.MaxLengthRatio,
			Message: "must be greater than 1.0",
		})
	} else if config.MaxLengthRatio > 100.0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MaxLengthRatio",
			Value:   config.MaxLengthRatio,
			Message: "must not exceed 100.0",
		})
	}

	// Validate MinLengthRatio < MaxLengthRatio
	if config.MinLengthRatio >= config.MaxLengthRatio {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MinLengthRatio/MaxLengthRatio",
			Value:   fmt.Sprintf("%.2f/%.2f", config.MinLengthRatio, config.MaxLengthRatio),
			Message: "MinLengthRatio must be less than MaxLengthRatio",
		})
	}

	// Validate MinChineseRatio
	if config.MinChineseRatio < 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MinChineseRatio",
			Value:   config.MinChineseRatio,
			Message: "must be non-negative",
		})
	} else if config.MinChineseRatio > 1.0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &ConfigValidationError{
			Field:   "MinChineseRatio",
			Value:   config.MinChineseRatio,
			Message: "must not exceed 1.0",
		})
	}

	return result
}

// LoadConfig loads configuration from a JSON file.
// If the file doesn't exist, it returns the default configuration.
// Returns an error if the file exists but cannot be read or parsed.
func LoadConfig(path string) (*FormatPreservationConfig, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		return DefaultConfig(), nil
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	// Parse JSON
	config := &FormatPreservationConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file '%s': %w", path, err)
	}

	// Validate the loaded config
	validationResult := ValidateConfig(config)
	if !validationResult.IsValid {
		// Return the first error for simplicity
		if len(validationResult.Errors) > 0 {
			return nil, fmt.Errorf("invalid config in '%s': %s", path, validationResult.Errors[0].Error())
		}
		return nil, fmt.Errorf("invalid config in '%s'", path)
	}

	return config, nil
}

// SaveConfig saves configuration to a JSON file.
// It creates the parent directory if it doesn't exist.
// The file is written with indentation for readability.
func SaveConfig(config *FormatPreservationConfig, path string) error {
	if config == nil {
		return fmt.Errorf("cannot save nil configuration")
	}

	// Validate before saving
	validationResult := ValidateConfig(config)
	if !validationResult.IsValid {
		if len(validationResult.Errors) > 0 {
			return fmt.Errorf("cannot save invalid config: %s", validationResult.Errors[0].Error())
		}
		return fmt.Errorf("cannot save invalid config")
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory '%s': %w", dir, err)
		}
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file '%s': %w", path, err)
	}

	return nil
}

// MergeConfig merges two configurations, with override values taking precedence.
// Only non-zero/non-empty values from override are applied to base.
// Returns a new configuration without modifying the inputs.
func MergeConfig(base, override *FormatPreservationConfig) *FormatPreservationConfig {
	// Start with default if base is nil
	if base == nil {
		base = DefaultConfig()
	}

	// Return a copy of base if override is nil
	if override == nil {
		return copyConfig(base)
	}

	// Create a new config starting from base
	result := copyConfig(base)

	// Merge protection settings
	// For booleans, we check if override explicitly sets them differently
	// Since we can't distinguish between "not set" and "set to false" in Go,
	// we always use override values for booleans
	result.ProtectMath = override.ProtectMath
	result.ProtectTables = override.ProtectTables
	result.ProtectComments = override.ProtectComments

	// Merge protected commands (append override to base)
	if len(override.ProtectedCommands) > 0 {
		// Create a set to avoid duplicates
		cmdSet := make(map[string]bool)
		for _, cmd := range result.ProtectedCommands {
			cmdSet[cmd] = true
		}
		for _, cmd := range override.ProtectedCommands {
			if !cmdSet[cmd] {
				result.ProtectedCommands = append(result.ProtectedCommands, cmd)
				cmdSet[cmd] = true
			}
		}
	}

	// Merge chunking settings (only if override has non-zero values)
	if override.MaxChunkSize > 0 {
		result.MaxChunkSize = override.MaxChunkSize
	}
	if override.ChunkOverlap > 0 {
		result.ChunkOverlap = override.ChunkOverlap
	}
	result.RespectEnvironments = override.RespectEnvironments

	// Merge validation settings
	result.StrictValidation = override.StrictValidation
	if override.MinLengthRatio > 0 {
		result.MinLengthRatio = override.MinLengthRatio
	}
	if override.MaxLengthRatio > 0 {
		result.MaxLengthRatio = override.MaxLengthRatio
	}
	if override.MinChineseRatio > 0 {
		result.MinChineseRatio = override.MinChineseRatio
	}

	// Merge recovery settings
	result.AutoFixLineStructure = override.AutoFixLineStructure
	result.AutoFixBraces = override.AutoFixBraces
	result.AutoFixEnvironments = override.AutoFixEnvironments

	// Merge prompt settings
	result.IncludeExamples = override.IncludeExamples
	result.StrictLineCount = override.StrictLineCount

	return result
}

// copyConfig creates a deep copy of a configuration
func copyConfig(config *FormatPreservationConfig) *FormatPreservationConfig {
	if config == nil {
		return nil
	}

	result := &FormatPreservationConfig{
		ProtectMath:          config.ProtectMath,
		ProtectTables:        config.ProtectTables,
		ProtectComments:      config.ProtectComments,
		MaxChunkSize:         config.MaxChunkSize,
		ChunkOverlap:         config.ChunkOverlap,
		RespectEnvironments:  config.RespectEnvironments,
		StrictValidation:     config.StrictValidation,
		MinLengthRatio:       config.MinLengthRatio,
		MaxLengthRatio:       config.MaxLengthRatio,
		MinChineseRatio:      config.MinChineseRatio,
		AutoFixLineStructure: config.AutoFixLineStructure,
		AutoFixBraces:        config.AutoFixBraces,
		AutoFixEnvironments:  config.AutoFixEnvironments,
		IncludeExamples:      config.IncludeExamples,
		StrictLineCount:      config.StrictLineCount,
	}

	// Deep copy the slice
	if len(config.ProtectedCommands) > 0 {
		result.ProtectedCommands = make([]string, len(config.ProtectedCommands))
		copy(result.ProtectedCommands, config.ProtectedCommands)
	} else {
		result.ProtectedCommands = []string{}
	}

	return result
}

// String returns a human-readable representation of the configuration
func (c *FormatPreservationConfig) String() string {
	if c == nil {
		return "FormatPreservationConfig{nil}"
	}
	return fmt.Sprintf("FormatPreservationConfig{ProtectMath:%v, ProtectTables:%v, ProtectComments:%v, "+
		"MaxChunkSize:%d, ChunkOverlap:%d, RespectEnvironments:%v, "+
		"StrictValidation:%v, MinLengthRatio:%.2f, MaxLengthRatio:%.2f, MinChineseRatio:%.2f, "+
		"AutoFixLineStructure:%v, AutoFixBraces:%v, AutoFixEnvironments:%v, "+
		"IncludeExamples:%v, StrictLineCount:%v, ProtectedCommands:%v}",
		c.ProtectMath, c.ProtectTables, c.ProtectComments,
		c.MaxChunkSize, c.ChunkOverlap, c.RespectEnvironments,
		c.StrictValidation, c.MinLengthRatio, c.MaxLengthRatio, c.MinChineseRatio,
		c.AutoFixLineStructure, c.AutoFixBraces, c.AutoFixEnvironments,
		c.IncludeExamples, c.StrictLineCount, c.ProtectedCommands)
}

// Equal checks if two configurations are equal
func (c *FormatPreservationConfig) Equal(other *FormatPreservationConfig) bool {
	if c == nil && other == nil {
		return true
	}
	if c == nil || other == nil {
		return false
	}

	// Compare all fields
	if c.ProtectMath != other.ProtectMath ||
		c.ProtectTables != other.ProtectTables ||
		c.ProtectComments != other.ProtectComments ||
		c.MaxChunkSize != other.MaxChunkSize ||
		c.ChunkOverlap != other.ChunkOverlap ||
		c.RespectEnvironments != other.RespectEnvironments ||
		c.StrictValidation != other.StrictValidation ||
		c.MinLengthRatio != other.MinLengthRatio ||
		c.MaxLengthRatio != other.MaxLengthRatio ||
		c.MinChineseRatio != other.MinChineseRatio ||
		c.AutoFixLineStructure != other.AutoFixLineStructure ||
		c.AutoFixBraces != other.AutoFixBraces ||
		c.AutoFixEnvironments != other.AutoFixEnvironments ||
		c.IncludeExamples != other.IncludeExamples ||
		c.StrictLineCount != other.StrictLineCount {
		return false
	}

	// Compare slices
	if len(c.ProtectedCommands) != len(other.ProtectedCommands) {
		return false
	}
	for i, cmd := range c.ProtectedCommands {
		if cmd != other.ProtectedCommands[i] {
			return false
		}
	}

	return true
}
