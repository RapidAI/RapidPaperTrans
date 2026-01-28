// Package editor provides file editing tools for LaTeX documents.
package editor

import (
	"fmt"
	"strings"

	"latex-translator/internal/logger"
)

// FixWorkflow implements the Edit-Fix-Validate workflow
type FixWorkflow struct {
	lineEditor       *LineEditor
	encodingHandler  *EncodingHandler
	validator        *LaTeXValidator
	backupMgr        *BackupManager
}

// NewFixWorkflow creates a new FixWorkflow
func NewFixWorkflow(backupDir string) *FixWorkflow {
	backupMgr := NewBackupManager(backupDir)
	return &FixWorkflow{
		lineEditor:      NewLineEditor(backupMgr),
		encodingHandler: NewEncodingHandler(backupMgr),
		validator:       NewLaTeXValidator(),
		backupMgr:       backupMgr,
	}
}

// FixResult contains the result of a fix operation
type FixResult struct {
	Success       bool
	FixesApplied  []string
	ValidationResult *ValidationResult
	BackupPath    string
	ErrorMessage  string
}

// AutoFix automatically fixes common LaTeX issues
func (w *FixWorkflow) AutoFix(path string) (*FixResult, error) {
	logger.Info("starting auto-fix workflow", logger.String("path", path))

	result := &FixResult{
		Success:      false,
		FixesApplied: []string{},
	}

	// Step 1: Create backup
	backup, err := w.backupMgr.CreateBackup(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backup

	// Step 2: Fix encoding issues
	if err := w.fixEncodingIssues(path, result); err != nil {
		logger.Warn("encoding fix failed", logger.Err(err))
		// Continue anyway - encoding might be fine
	}

	// Step 3: Validate
	validationResult, err := w.validator.ValidateLaTeX(path)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("validation failed: %v", err)
		return result, err
	}
	result.ValidationResult = validationResult

	// Step 4: Apply fixes based on validation errors
	if !validationResult.Valid {
		if err := w.applyFixes(path, validationResult, result); err != nil {
			logger.Error("failed to apply fixes", err)
			result.ErrorMessage = fmt.Sprintf("failed to apply fixes: %v", err)
			// Restore backup
			w.backupMgr.Restore(backup, path)
			return result, err
		}

		// Step 5: Re-validate after fixes
		validationResult, err = w.validator.ValidateLaTeX(path)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("re-validation failed: %v", err)
			return result, err
		}
		result.ValidationResult = validationResult
	}

	result.Success = validationResult.Valid
	logger.Info("auto-fix workflow completed",
		logger.Bool("success", result.Success),
		logger.Int("fixCount", len(result.FixesApplied)))

	return result, nil
}

// fixEncodingIssues fixes encoding-related issues
func (w *FixWorkflow) fixEncodingIssues(path string, result *FixResult) error {
	// Detect encoding
	encoding, err := w.encodingHandler.DetectEncoding(path)
	if err != nil {
		return err
	}

	logger.Debug("detected encoding", logger.String("encoding", encoding))

	// Fix encoding if needed
	if encoding == "UTF-8" {
		// Already UTF-8, no fix needed
		return nil
	}

	if encoding == "UTF-8-BOM" {
		// Remove BOM
		if err := w.encodingHandler.RemoveBOM(path); err != nil {
			return err
		}
		result.FixesApplied = append(result.FixesApplied, "Removed UTF-8 BOM")
		logger.Info("removed UTF-8 BOM")
		return nil
	}

	// Convert to UTF-8
	if err := w.encodingHandler.ConvertEncoding(path, encoding, "UTF-8"); err != nil {
		return err
	}
	result.FixesApplied = append(result.FixesApplied,
		fmt.Sprintf("Converted encoding from %s to UTF-8", encoding))
	logger.Info("converted encoding to UTF-8", logger.String("from", encoding))

	return nil
}

// applyFixes applies fixes based on validation errors
func (w *FixWorkflow) applyFixes(path string, validationResult *ValidationResult, result *FixResult) error {
	for _, err := range validationResult.Errors {
		switch err.Type {
		case "structure":
			if strings.Contains(err.Message, "Unclosed environments") {
				// Try to fix unclosed environments
				if fixed := w.fixUnclosedEnvironments(path, err); fixed {
					result.FixesApplied = append(result.FixesApplied, "Fixed unclosed environments")
				}
			}
		case "syntax":
			if strings.Contains(err.Message, "typo") {
				// Fix typos
				if fixed := w.fixTypos(path); fixed {
					result.FixesApplied = append(result.FixesApplied, "Fixed typos")
				}
			}
		}
	}

	return nil
}

// fixUnclosedEnvironments attempts to fix unclosed LaTeX environments
func (w *FixWorkflow) fixUnclosedEnvironments(path string, err ValidationError) bool {
	// This is a simplified implementation
	// In a real implementation, you would parse the error message to determine
	// which environments are unclosed and add the appropriate \end{} commands
	logger.Debug("attempting to fix unclosed environments")
	return false
}

// fixTypos fixes common LaTeX typos
func (w *FixWorkflow) fixTypos(path string) bool {
	content, err := w.encodingHandler.ReadFileWithEncoding(path)
	if err != nil {
		return false
	}

	typos := map[string]string{
		"\\begn{":        "\\begin{",
		"\\ened{":        "\\end{",
		"\\docmentclass": "\\documentclass",
		"\\usepackge":    "\\usepackage",
	}

	fixed := false
	for typo, correct := range typos {
		if strings.Contains(content, typo) {
			content = strings.ReplaceAll(content, typo, correct)
			fixed = true
			logger.Debug("fixed typo", logger.String("typo", typo), logger.String("correct", correct))
		}
	}

	if fixed {
		if err := w.encodingHandler.WriteFileWithEncoding(path, content, "UTF-8"); err != nil {
			logger.Error("failed to write fixed content", err)
			return false
		}
	}

	return fixed
}

// ValidateAndFix validates a file and applies fixes if needed
func (w *FixWorkflow) ValidateAndFix(path string) (*FixResult, error) {
	logger.Info("validate and fix", logger.String("path", path))

	// First validate
	validationResult, err := w.validator.ValidateLaTeX(path)
	if err != nil {
		return nil, err
	}

	// If valid, no fixes needed
	if validationResult.Valid {
		return &FixResult{
			Success:          true,
			ValidationResult: validationResult,
			FixesApplied:     []string{},
		}, nil
	}

	// Apply auto-fix
	return w.AutoFix(path)
}

// FixEncodingOnly fixes only encoding issues without other fixes
func (w *FixWorkflow) FixEncodingOnly(path string) (*FixResult, error) {
	logger.Info("fixing encoding only", logger.String("path", path))

	result := &FixResult{
		Success:      false,
		FixesApplied: []string{},
	}

	// Create backup
	backup, err := w.backupMgr.CreateBackup(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backup

	// Fix encoding
	if err := w.fixEncodingIssues(path, result); err != nil {
		w.backupMgr.Restore(backup, path)
		result.ErrorMessage = fmt.Sprintf("encoding fix failed: %v", err)
		return result, err
	}

	// Validate
	validationResult, err := w.validator.ValidateLaTeX(path)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("validation failed: %v", err)
		return result, err
	}
	result.ValidationResult = validationResult
	result.Success = true

	return result, nil
}

// GetLineEditor returns the line editor
func (w *FixWorkflow) GetLineEditor() *LineEditor {
	return w.lineEditor
}

// GetEncodingHandler returns the encoding handler
func (w *FixWorkflow) GetEncodingHandler() *EncodingHandler {
	return w.encodingHandler
}

// GetValidator returns the validator
func (w *FixWorkflow) GetValidator() *LaTeXValidator {
	return w.validator
}

// GetBackupManager returns the backup manager
func (w *FixWorkflow) GetBackupManager() *BackupManager {
	return w.backupMgr
}

// FormatFixResult formats a FixResult as a human-readable string
func FormatFixResult(result *FixResult) string {
	var sb strings.Builder

	sb.WriteString("Fix Result:\n")
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	if result.Success {
		sb.WriteString("✓ Fix SUCCESSFUL\n\n")
	} else {
		sb.WriteString("✗ Fix FAILED\n\n")
		if result.ErrorMessage != "" {
			sb.WriteString(fmt.Sprintf("Error: %s\n\n", result.ErrorMessage))
		}
	}

	if len(result.FixesApplied) > 0 {
		sb.WriteString(fmt.Sprintf("Fixes Applied (%d):\n", len(result.FixesApplied)))
		for i, fix := range result.FixesApplied {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, fix))
		}
		sb.WriteString("\n")
	}

	if result.BackupPath != "" {
		sb.WriteString(fmt.Sprintf("Backup: %s\n\n", result.BackupPath))
	}

	if result.ValidationResult != nil {
		sb.WriteString("Validation Result:\n")
		if result.ValidationResult.Valid {
			sb.WriteString("  ✓ Valid\n")
		} else {
			sb.WriteString(fmt.Sprintf("  ✗ Invalid (%d errors, %d warnings)\n",
				len(result.ValidationResult.Errors),
				len(result.ValidationResult.Warnings)))
		}
	}

	return sb.String()
}

// SafeEdit performs an edit operation with automatic backup and validation
type SafeEditFunc func() error

// SafeEdit executes an edit function with backup and validation
func (w *FixWorkflow) SafeEdit(path string, editFunc SafeEditFunc) error {
	// Create backup
	backup, err := w.backupMgr.CreateBackup(path)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Perform edit
	if err := editFunc(); err != nil {
		// Restore backup on error
		w.backupMgr.Restore(backup, path)
		return fmt.Errorf("edit failed: %w", err)
	}

	// Validate
	validationResult, err := w.validator.ValidateLaTeX(path)
	if err != nil {
		w.backupMgr.Restore(backup, path)
		return fmt.Errorf("validation failed: %w", err)
	}

	if !validationResult.Valid {
		// Restore backup if validation fails
		w.backupMgr.Restore(backup, path)
		return fmt.Errorf("validation failed after edit: %s",
			w.validator.GetErrorSummary(validationResult))
	}

	logger.Info("safe edit completed successfully")
	return nil
}

// BatchFix applies fixes to multiple files
func (w *FixWorkflow) BatchFix(paths []string) map[string]*FixResult {
	results := make(map[string]*FixResult)

	for _, path := range paths {
		logger.Info("processing file", logger.String("path", path))
		result, err := w.AutoFix(path)
		if err != nil {
			result = &FixResult{
				Success:      false,
				ErrorMessage: err.Error(),
			}
		}
		results[path] = result
	}

	return results
}
