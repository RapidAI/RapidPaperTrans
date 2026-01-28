package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMainFile_Valid(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a valid LaTeX file
	validContent := `\documentclass{article}
\begin{document}
Hello, world!
\end{document}
`
	mainTexPath := filepath.Join(tmpDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(validContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewLaTeXValidator(tmpDir)
	result, err := validator.ValidateMainFile(mainTexPath)

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected valid result, got invalid. Issues: %v", result.Issues)
	}

	if len(result.Issues) != 0 {
		t.Errorf("Expected no issues, got %d: %v", len(result.Issues), result.Issues)
	}
}

func TestValidateMainFile_UnbalancedBraces(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with unbalanced braces
	invalidContent := `\documentclass{article}
\newcommand{\test}{
\begin{document}
Hello, world!
\end{document}
`
	mainTexPath := filepath.Join(tmpDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewLaTeXValidator(tmpDir)
	result, err := validator.ValidateMainFile(mainTexPath)

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid result due to unbalanced braces")
	}

	hasBalanceError := false
	for _, issue := range result.Issues {
		if issue.Severity == "error" && issue.Message == "Unbalanced braces" {
			hasBalanceError = true
			break
		}
	}

	if !hasBalanceError {
		t.Errorf("Expected unbalanced braces error, got issues: %v", result.Issues)
	}
}

func TestValidateMainFile_MissingDocumentClass(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `\begin{document}
Hello, world!
\end{document}
`
	mainTexPath := filepath.Join(tmpDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewLaTeXValidator(tmpDir)
	result, err := validator.ValidateMainFile(mainTexPath)

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid result due to missing \\documentclass")
	}

	hasDocClassError := false
	for _, issue := range result.Issues {
		if issue.Message == "Missing \\documentclass" {
			hasDocClassError = true
			break
		}
	}

	if !hasDocClassError {
		t.Errorf("Expected missing documentclass error, got issues: %v", result.Issues)
	}
}

func TestValidateMainFile_ContentAfterEndDocument(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `\documentclass{article}
\begin{document}
Hello, world!
\end{document}
}}
`
	mainTexPath := filepath.Join(tmpDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewLaTeXValidator(tmpDir)
	result, err := validator.ValidateMainFile(mainTexPath)

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid result due to content after \\end{document}")
	}

	hasContentAfterError := false
	for _, issue := range result.Issues {
		if issue.Message == "Content after \\end{document}" {
			hasContentAfterError = true
			break
		}
	}

	if !hasContentAfterError {
		t.Errorf("Expected content after end document error, got issues: %v", result.Issues)
	}
}

func TestValidateMainFile_UnclosedEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `\documentclass{article}
\begin{document}
\begin{itemize}
\item Test
\end{document}
`
	mainTexPath := filepath.Join(tmpDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewLaTeXValidator(tmpDir)
	result, err := validator.ValidateMainFile(mainTexPath)

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	hasUnclosedEnvWarning := false
	for _, issue := range result.Issues {
		if issue.Message == "Unclosed environments" {
			hasUnclosedEnvWarning = true
			break
		}
	}

	if !hasUnclosedEnvWarning {
		t.Errorf("Expected unclosed environment warning, got issues: %v", result.Issues)
	}
}

func TestValidateMainFile_MissingIncludedFile(t *testing.T) {
	tmpDir := t.TempDir()

	content := `\documentclass{article}
\begin{document}
\input{nonexistent}
\end{document}
`
	mainTexPath := filepath.Join(tmpDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewLaTeXValidator(tmpDir)
	result, err := validator.ValidateMainFile(mainTexPath)

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	hasMissingFileWarning := false
	for _, issue := range result.Issues {
		if issue.Message == "Included file not found" {
			hasMissingFileWarning = true
			break
		}
	}

	if !hasMissingFileWarning {
		t.Errorf("Expected missing included file warning, got issues: %v", result.Issues)
	}
}

func TestFormatIssues(t *testing.T) {
	issues := []ValidationIssue{
		{
			Severity: "error",
			File:     "main.tex",
			Line:     10,
			Message:  "Unbalanced braces",
			Details:  "1 unclosed opening brace",
		},
		{
			Severity: "warning",
			File:     "main.tex",
			Line:     20,
			Message:  "Unclosed environment",
			Details:  "Missing \\end{itemize}",
		},
	}

	formatted := FormatIssues(issues)

	if formatted == "" {
		t.Error("Expected non-empty formatted output")
	}

	if !contains(formatted, "Unbalanced braces") {
		t.Error("Expected formatted output to contain 'Unbalanced braces'")
	}

	if !contains(formatted, "Unclosed environment") {
		t.Error("Expected formatted output to contain 'Unclosed environment'")
	}
}

func TestFormatIssues_Empty(t *testing.T) {
	formatted := FormatIssues([]ValidationIssue{})

	if formatted != "No issues found" {
		t.Errorf("Expected 'No issues found', got: %s", formatted)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		len(s) > len(substr)+1 && s[1:len(substr)+1] == substr ||
		findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
