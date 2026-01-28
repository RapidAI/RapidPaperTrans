package editor

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFixWorkflow_Integration tests the complete fix workflow
func TestFixWorkflow_Integration(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	
	// Create a test LaTeX file with encoding issues
	testFile := filepath.Join(tmpDir, "test.tex")
	
	// Content with UTF-8 BOM and some LaTeX
	content := "\xEF\xBB\xBF\\documentclass{article}\n" +
		"\\begin{document}\n" +
		"Hello World\n" +
		"\\end{document}\n"
	
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Create workflow
	backupDir := filepath.Join(tmpDir, ".backups")
	workflow := NewFixWorkflow(backupDir)

	// Test encoding fix
	t.Run("FixEncodingOnly", func(t *testing.T) {
		result, err := workflow.FixEncodingOnly(testFile)
		if err != nil {
			t.Fatalf("FixEncodingOnly() error = %v", err)
		}

		if !result.Success {
			t.Errorf("FixEncodingOnly() failed: %s", result.ErrorMessage)
		}

		if len(result.FixesApplied) == 0 {
			t.Error("Expected at least one fix to be applied (BOM removal)")
		}

		// Verify BOM was removed
		data, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
			t.Error("BOM was not removed")
		}
	})

	// Test validation
	t.Run("Validation", func(t *testing.T) {
		validator := workflow.GetValidator()
		result, err := validator.ValidateLaTeX(testFile)
		if err != nil {
			t.Fatalf("ValidateLaTeX() error = %v", err)
		}

		if !result.Valid {
			t.Errorf("Validation failed with %d errors", len(result.Errors))
			for _, err := range result.Errors {
				t.Logf("  Error: %s", err.Message)
			}
		}
	})

	// Test backup was created
	t.Run("BackupCreated", func(t *testing.T) {
		backupMgr := workflow.GetBackupManager()
		backups, err := backupMgr.ListBackups(testFile)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		if len(backups) == 0 {
			t.Error("No backup was created")
		}
	})
}

// TestFixWorkflow_ChineseDetection tests Chinese character detection after encoding fix
func TestFixWorkflow_ChineseDetection(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "chinese.tex")

	// Create file with Chinese content in GBK encoding
	// For simplicity, we'll use UTF-8 with BOM
	content := "\xEF\xBB\xBF\\documentclass{article}\n" +
		"\\begin{document}\n" +
		"你好世界\n" +
		"\\end{document}\n"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Create workflow and fix encoding
	workflow := NewFixWorkflow(tmpDir)
	result, err := workflow.FixEncodingOnly(testFile)
	if err != nil {
		t.Fatalf("FixEncodingOnly() error = %v", err)
	}

	if !result.Success {
		t.Fatalf("FixEncodingOnly() failed: %s", result.ErrorMessage)
	}

	// Read file and check if Chinese can be detected
	encodingHandler := workflow.GetEncodingHandler()
	content, err = encodingHandler.ReadFileWithEncoding(testFile)
	if err != nil {
		t.Fatalf("ReadFileWithEncoding() error = %v", err)
	}

	// Check for Chinese characters
	hasChinese := false
	for _, r := range content {
		if r >= 0x4E00 && r <= 0x9FFF {
			hasChinese = true
			break
		}
	}

	if !hasChinese {
		t.Error("Chinese characters not detected after encoding fix")
	}
}

// TestFixWorkflow_SafeEdit tests safe editing with automatic rollback
func TestFixWorkflow_SafeEdit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tex")

	// Create valid LaTeX file
	content := "\\documentclass{article}\n" +
		"\\begin{document}\n" +
		"Line 1\n" +
		"Line 2\n" +
		"Line 3\n" +
		"\\end{document}\n"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	workflow := NewFixWorkflow(tmpDir)
	lineEditor := workflow.GetLineEditor()

	// Test successful edit
	t.Run("SuccessfulEdit", func(t *testing.T) {
		err := workflow.SafeEdit(testFile, func() error {
			return lineEditor.ReplaceLine(testFile, 3, "Modified Line 1")
		})

		if err != nil {
			t.Errorf("SafeEdit() error = %v", err)
		}

		// Verify change was applied
		lines, err := lineEditor.ReadLines(testFile, 3, 3)
		if err != nil {
			t.Fatal(err)
		}

		if len(lines) != 1 || lines[0] != "Modified Line 1" {
			t.Errorf("Edit was not applied correctly, got: %v", lines)
		}
	})

	// Test edit that breaks validation (should rollback)
	t.Run("FailedEditRollback", func(t *testing.T) {
		// Read original content
		originalLines, err := lineEditor.ReadLines(testFile, 1, -1)
		if err != nil {
			t.Fatal(err)
		}

		// Try to make an edit that breaks the file
		err = workflow.SafeEdit(testFile, func() error {
			// Delete \end{document}
			return lineEditor.DeleteLine(testFile, 6)
		})

		// Should fail validation
		if err == nil {
			t.Error("Expected SafeEdit to fail validation")
		}

		// Verify file was rolled back
		currentLines, err := lineEditor.ReadLines(testFile, 1, -1)
		if err != nil {
			t.Fatal(err)
		}

		if len(currentLines) != len(originalLines) {
			t.Error("File was not rolled back after failed validation")
		}
	})
}

// TestFixWorkflow_BatchFix tests batch fixing multiple files
func TestFixWorkflow_BatchFix(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	files := []string{
		filepath.Join(tmpDir, "file1.tex"),
		filepath.Join(tmpDir, "file2.tex"),
		filepath.Join(tmpDir, "file3.tex"),
	}

	for _, file := range files {
		content := "\xEF\xBB\xBF\\documentclass{article}\n\\begin{document}\nTest\\end{document}\n"
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Batch fix
	workflow := NewFixWorkflow(tmpDir)
	results := workflow.BatchFix(files)

	// Verify all files were processed
	if len(results) != len(files) {
		t.Errorf("BatchFix() processed %d files, want %d", len(results), len(files))
	}

	// Verify all succeeded
	for file, result := range results {
		if !result.Success {
			t.Errorf("BatchFix() failed for %s: %s", file, result.ErrorMessage)
		}
	}
}
