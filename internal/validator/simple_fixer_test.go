package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimpleFixer_FixContentAfterEndDocument(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	content := `\documentclass{article}
\begin{document}
Hello, world!
\end{document}
}}
extra content
`

	fixed := fixer.fixContentAfterEndDocument(content)

	if strings.Contains(fixed, "}}") {
		t.Error("Failed to remove content after \\end{document}")
	}

	if strings.Contains(fixed, "extra content") {
		t.Error("Failed to remove extra content")
	}

	if !strings.HasSuffix(strings.TrimSpace(fixed), "\\end{document}") {
		t.Error("File should end with \\end{document}")
	}
}

func TestSimpleFixer_FixCommonIssues(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	tests := []struct {
		name     string
		input    string
		wantFix  string
		wantDesc string
	}{
		{
			name:     "fix typo begn",
			input:    "\\begn{document}",
			wantFix:  "\\begin{document}",
			wantDesc: "Fixed typo",
		},
		{
			name:     "fix duplicate end document",
			input:    "\\end{document}\n\\end{document}",
			wantFix:  "\\end{document}",
			wantDesc: "duplicate",
		},
		{
			name:    "windows line endings",
			input:   "line1\r\nline2\r\n",
			wantFix: "line1\nline2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed, applied := fixer.fixCommonIssues(tt.input)

			if !strings.Contains(fixed, tt.wantFix) {
				t.Errorf("Expected fixed content to contain %q, got %q", tt.wantFix, fixed)
			}

			if tt.wantDesc != "" {
				found := false
				for _, desc := range applied {
					if strings.Contains(desc, tt.wantDesc) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected fix description containing %q, got %v", tt.wantDesc, applied)
				}
			}
		})
	}
}

func TestSimpleFixer_TryFixFile(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	// Create a file with issues
	content := `\documentclass{article}
\begin{document}
Hello, world!
\end{document}
}}
`
	testFile := filepath.Join(tmpDir, "test.tex")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create validation result
	validationResult := &ValidationResult{
		Valid: false,
		Issues: []ValidationIssue{
			{
				Severity: "error",
				Message:  "Content after \\end{document}",
			},
		},
	}

	// Try to fix
	result, err := fixer.TryFixFile(testFile, validationResult)
	if err != nil {
		t.Fatalf("TryFixFile failed: %v", err)
	}

	if !result.Fixed {
		t.Error("Expected file to be fixed")
	}

	if len(result.FixesApplied) == 0 {
		t.Error("Expected at least one fix to be applied")
	}

	// Check backup was created
	if result.BackupPath == "" {
		t.Error("Expected backup path to be set")
	}

	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Check fixed content
	fixedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read fixed file: %v", err)
	}

	if strings.Contains(string(fixedContent), "}}") {
		t.Error("Fixed file still contains '}}'")
	}
}

func TestSimpleFixer_RestoreBackup(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	originalContent := "original content"
	testFile := filepath.Join(tmpDir, "test.tex")
	backupFile := testFile + ".backup"

	// Create original and backup
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(backupFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Restore backup
	if err := fixer.RestoreBackup(testFile); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Check content was restored
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if string(content) != originalContent {
		t.Errorf("Expected content %q, got %q", originalContent, string(content))
	}
}

func TestSimpleFixer_SmartFix(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	// Create a file with a fixable issue
	content := `\documentclass{article}
\begin{document}
Hello, world!
\end{document}
extra
`
	testFile := filepath.Join(tmpDir, "test.tex")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try smart fix
	result, err := fixer.SmartFix(testFile)
	if err != nil {
		t.Fatalf("SmartFix failed: %v", err)
	}

	if !result.Fixed {
		t.Error("Expected file to be fixed")
	}

	// Verify the fix
	fixedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read fixed file: %v", err)
	}

	if strings.Contains(string(fixedContent), "extra") {
		t.Error("Fixed file still contains 'extra' after \\end{document}")
	}
}

func TestSimpleFixer_FixWithRegex(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	content := "\\begn{document}\nHello\n\\ened{document}"
	testFile := filepath.Join(tmpDir, "test.tex")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Fix with regex
	result, err := fixer.FixWithRegex(testFile, `\\begn\{`, `\\begin{`, "Fix begn typo")
	if err != nil {
		t.Fatalf("FixWithRegex failed: %v", err)
	}

	if !result.Fixed {
		t.Error("Expected file to be fixed")
	}

	// Check fixed content
	fixedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read fixed file: %v", err)
	}

	if !strings.Contains(string(fixedContent), "\\begin{document}") {
		t.Error("Regex fix did not work")
	}
}

func TestSimpleFixer_FixUnbalancedBraces(t *testing.T) {
	tmpDir := t.TempDir()
	fixer := NewSimpleFixer(tmpDir)

	content := `\documentclass{article}
\newcommand{\test}{
\begin{document}
Hello
\end{document}`

	issue := ValidationIssue{
		Message: "Unbalanced braces",
		Details: "1 unclosed opening braces",
	}

	fixed, ok := fixer.fixUnbalancedBraces(content, issue)
	if !ok {
		t.Error("Expected fix to be applied")
	}

	// Count braces
	openCount := strings.Count(fixed, "{") - strings.Count(fixed, "\\{")
	closeCount := strings.Count(fixed, "}") - strings.Count(fixed, "\\}")

	if openCount != closeCount {
		t.Errorf("Braces still unbalanced: %d open, %d close", openCount, closeCount)
	}
}
