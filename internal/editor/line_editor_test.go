package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLineEditor_ReadLines(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	backupMgr := NewBackupManager(tmpDir)
	editor := NewLineEditor(backupMgr)

	tests := []struct {
		name      string
		start     int
		end       int
		wantLines []string
		wantErr   bool
	}{
		{
			name:      "read all lines",
			start:     1,
			end:       -1,
			wantLines: []string{"line1", "line2", "line3", "line4", "line5"},
			wantErr:   false,
		},
		{
			name:      "read first 3 lines",
			start:     1,
			end:       3,
			wantLines: []string{"line1", "line2", "line3"},
			wantErr:   false,
		},
		{
			name:      "read middle lines",
			start:     2,
			end:       4,
			wantLines: []string{"line2", "line3", "line4"},
			wantErr:   false,
		},
		{
			name:      "read last line",
			start:     5,
			end:       5,
			wantLines: []string{"line5"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, err := editor.ReadLines(testFile, tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadLines() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(lines) != len(tt.wantLines) {
				t.Errorf("ReadLines() got %d lines, want %d", len(lines), len(tt.wantLines))
				return
			}
			for i, line := range lines {
				if line != tt.wantLines[i] {
					t.Errorf("ReadLines() line %d = %v, want %v", i, line, tt.wantLines[i])
				}
			}
		})
	}
}

func TestLineEditor_ReplaceLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	backupMgr := NewBackupManager(tmpDir)
	editor := NewLineEditor(backupMgr)

	// Replace line 2
	if err := editor.ReplaceLine(testFile, 2, "new line 2"); err != nil {
		t.Fatalf("ReplaceLine() error = %v", err)
	}

	// Read back and verify
	lines, err := editor.ReadLines(testFile, 1, -1)
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}

	expected := []string{"line1", "new line 2", "line3"}
	if len(lines) != len(expected) {
		t.Errorf("got %d lines, want %d", len(lines), len(expected))
	}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("line %d = %v, want %v", i+1, line, expected[i])
		}
	}
}

func TestLineEditor_InsertLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	backupMgr := NewBackupManager(tmpDir)
	editor := NewLineEditor(backupMgr)

	// Insert at line 2
	if err := editor.InsertLine(testFile, 2, "inserted line"); err != nil {
		t.Fatalf("InsertLine() error = %v", err)
	}

	// Read back and verify
	lines, err := editor.ReadLines(testFile, 1, -1)
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}

	expected := []string{"line1", "inserted line", "line2", "line3"}
	if len(lines) != len(expected) {
		t.Errorf("got %d lines, want %d", len(lines), len(expected))
	}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("line %d = %v, want %v", i+1, line, expected[i])
		}
	}
}

func TestLineEditor_DeleteLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	backupMgr := NewBackupManager(tmpDir)
	editor := NewLineEditor(backupMgr)

	// Delete line 2
	if err := editor.DeleteLine(testFile, 2); err != nil {
		t.Fatalf("DeleteLine() error = %v", err)
	}

	// Read back and verify
	lines, err := editor.ReadLines(testFile, 1, -1)
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}

	expected := []string{"line1", "line3"}
	if len(lines) != len(expected) {
		t.Errorf("got %d lines, want %d", len(lines), len(expected))
	}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("line %d = %v, want %v", i+1, line, expected[i])
		}
	}
}

func TestLineEditor_CountLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	backupMgr := NewBackupManager(tmpDir)
	editor := NewLineEditor(backupMgr)

	count, err := editor.CountLines(testFile)
	if err != nil {
		t.Fatalf("CountLines() error = %v", err)
	}

	if count != 5 {
		t.Errorf("CountLines() = %d, want 5", count)
	}
}

func TestLineEditor_SearchLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world\nfoo bar\nhello again\ntest\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	backupMgr := NewBackupManager(tmpDir)
	editor := NewLineEditor(backupMgr)

	lines, err := editor.SearchLines(testFile, "hello")
	if err != nil {
		t.Fatalf("SearchLines() error = %v", err)
	}

	expected := []int{1, 3}
	if len(lines) != len(expected) {
		t.Errorf("SearchLines() found %d matches, want %d", len(lines), len(expected))
	}
	for i, lineNum := range lines {
		if lineNum != expected[i] {
			t.Errorf("match %d at line %d, want line %d", i, lineNum, expected[i])
		}
	}
}
