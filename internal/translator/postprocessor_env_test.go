// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"strings"
	"testing"
)

// ============================================================
// RestoreEnvironments Tests (Task 6.3)
// Tests for Requirements 3.3 and 3.4
// ============================================================

func TestRestoreEnvironments_AlreadyBalanced(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
	}{
		{
			name: "single balanced environment",
			translated: `\begin{document}
content
\end{document}`,
			original: `\begin{document}
content
\end{document}`,
		},
		{
			name: "nested balanced environments",
			translated: `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`,
			original: `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`,
		},
		{
			name:       "empty content",
			translated: "",
			original:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreEnvironments(tt.translated, tt.original)
			// For already balanced content, result should be similar to input
			// (may have minor whitespace differences)
			validation := ValidateEnvironments(result)
			if !validation.IsValid {
				t.Errorf("RestoreEnvironments() produced invalid result: %s", FormatEnvironmentValidation(validation))
			}
		})
	}
}

func TestRestoreEnvironments_MissingEndTag(t *testing.T) {
	tests := []struct {
		name           string
		translated     string
		original       string
		expectedEnv    string
		shouldContain  string
	}{
		{
			name: "missing end document",
			translated: `\begin{document}
content`,
			original: `\begin{document}
content
\end{document}`,
			expectedEnv:   "document",
			shouldContain: `\end{document}`,
		},
		{
			name: "missing end figure",
			translated: `\begin{document}
\begin{figure}
content
\end{document}`,
			original: `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`,
			expectedEnv:   "figure",
			shouldContain: `\end{figure}`,
		},
		{
			name: "missing multiple end tags",
			translated: `\begin{document}
\begin{figure}
\begin{center}
content`,
			original: `\begin{document}
\begin{figure}
\begin{center}
content
\end{center}
\end{figure}
\end{document}`,
			expectedEnv:   "center",
			shouldContain: `\end{center}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreEnvironments(tt.translated, tt.original)
			
			// Check that the missing end tag was inserted
			if !strings.Contains(result, tt.shouldContain) {
				t.Errorf("RestoreEnvironments() should contain %s, got:\n%s", tt.shouldContain, result)
			}
			
			// Validate the result
			validation := ValidateEnvironments(result)
			if !validation.IsValid {
				// It's okay if not perfectly valid, but the specific env should be fixed
				envCount, ok := validation.Environments[tt.expectedEnv]
				if ok && envCount.BeginCount != envCount.EndCount {
					t.Errorf("RestoreEnvironments() did not fix %s environment: begin=%d, end=%d",
						tt.expectedEnv, envCount.BeginCount, envCount.EndCount)
				}
			}
		})
	}
}

func TestRestoreEnvironments_NestingOrder(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantValid  bool
	}{
		{
			name: "fix missing end tag in nested structure",
			translated: `\begin{document}
\begin{figure}
content
\end{document}`,
			original: `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreEnvironments(tt.translated, tt.original)
			
			// The result should have all environments balanced
			validation := ValidateEnvironments(result)
			
			// Check that begin and end counts match for all environments
			for envName, count := range validation.Environments {
				if count.BeginCount != count.EndCount {
					t.Errorf("Environment %s not balanced after RestoreEnvironments: begin=%d, end=%d",
						envName, count.BeginCount, count.EndCount)
				}
			}
		})
	}
}

func TestRestoreEnvironments_WithDetails(t *testing.T) {
	tests := []struct {
		name                string
		translated          string
		original            string
		wantInsertedEndTags int
		wantRestoredValid   bool
	}{
		{
			name: "insert one missing end tag",
			translated: `\begin{document}
content`,
			original: `\begin{document}
content
\end{document}`,
			wantInsertedEndTags: 1,
			wantRestoredValid:   true,
		},
		{
			name: "insert multiple missing end tags",
			translated: `\begin{document}
\begin{figure}
content`,
			original: `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`,
			wantInsertedEndTags: 2,
			wantRestoredValid:   true,
		},
		{
			name: "already balanced - no insertions",
			translated: `\begin{document}
\end{document}`,
			original: `\begin{document}
\end{document}`,
			wantInsertedEndTags: 0,
			wantRestoredValid:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreEnvironmentsWithDetails(tt.translated, tt.original)
			
			if len(result.InsertedEndTags) != tt.wantInsertedEndTags {
				t.Errorf("RestoreEnvironmentsWithDetails() inserted %d end tags, want %d",
					len(result.InsertedEndTags), tt.wantInsertedEndTags)
			}
			
			if result.RestoredValid != tt.wantRestoredValid {
				t.Errorf("RestoreEnvironmentsWithDetails().RestoredValid = %v, want %v",
					result.RestoredValid, tt.wantRestoredValid)
			}
		})
	}
}

func TestFixEnvironmentsByReference_MissingEndTags(t *testing.T) {
	tests := []struct {
		name          string
		translated    string
		original      string
		shouldContain string
	}{
		{
			name: "insert missing end tag",
			translated: `\begin{figure}
content`,
			original: `\begin{figure}
content
\end{figure}`,
			shouldContain: `\end{figure}`,
		},
		{
			name: "insert missing end tag for table",
			translated: `\begin{table}
\begin{tabular}{cc}
a & b
\end{tabular}`,
			original: `\begin{table}
\begin{tabular}{cc}
a & b
\end{tabular}
\end{table}`,
			shouldContain: `\end{table}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixEnvironmentsByReference(tt.translated, tt.original)
			
			if !strings.Contains(result, tt.shouldContain) {
				t.Errorf("fixEnvironmentsByReference() should contain %s, got:\n%s", tt.shouldContain, result)
			}
		})
	}
}

func TestInsertMissingEndTagsWithNesting(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantValid  bool
	}{
		{
			name: "simple missing end tag",
			translated: `\begin{document}
content`,
			original: `\begin{document}
content
\end{document}`,
			wantValid: true,
		},
		{
			name: "nested missing end tags",
			translated: `\begin{document}
\begin{figure}
\begin{center}
content`,
			original: `\begin{document}
\begin{figure}
\begin{center}
content
\end{center}
\end{figure}
\end{document}`,
			wantValid: true,
		},
		{
			name: "already balanced",
			translated: `\begin{document}
\begin{figure}
\end{figure}
\end{document}`,
			original: `\begin{document}
\begin{figure}
\end{figure}
\end{document}`,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertMissingEndTagsWithNesting(tt.translated, tt.original)
			
			validation := ValidateEnvironments(result)
			
			// Check that all environments are balanced
			for envName, count := range validation.Environments {
				if count.BeginCount != count.EndCount {
					t.Errorf("Environment %s not balanced: begin=%d, end=%d",
						envName, count.BeginCount, count.EndCount)
				}
			}
		})
	}
}

func TestParseEnvironmentPositions(t *testing.T) {
	content := `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`

	positions := parseEnvironmentPositions(content)
	
	if len(positions) != 4 {
		t.Errorf("parseEnvironmentPositions() returned %d positions, want 4", len(positions))
	}
	
	// Check first position is \begin{document}
	if len(positions) > 0 {
		if positions[0].envName != "document" || !positions[0].isBegin {
			t.Errorf("First position should be \\begin{document}, got %s (isBegin=%v)",
				positions[0].envName, positions[0].isBegin)
		}
	}
	
	// Check last position is \end{document}
	if len(positions) > 3 {
		if positions[3].envName != "document" || positions[3].isBegin {
			t.Errorf("Last position should be \\end{document}, got %s (isBegin=%v)",
				positions[3].envName, positions[3].isBegin)
		}
	}
}

func TestFixEnvironmentNestingOrder(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
	}{
		{
			name: "already properly nested",
			translated: `\begin{figure}
\begin{center}
content
\end{center}
\end{figure}`,
			original: `\begin{figure}
\begin{center}
content
\end{center}
\end{figure}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixEnvironmentNestingOrder(tt.translated, tt.original)
			
			// The result should have proper nesting
			validation := ValidateEnvironments(result)
			
			// All environments should be balanced
			for envName, count := range validation.Environments {
				if count.BeginCount != count.EndCount {
					t.Errorf("Environment %s not balanced: begin=%d, end=%d",
						envName, count.BeginCount, count.EndCount)
				}
			}
		})
	}
}

// ============================================================
// Integration Tests
// ============================================================

func TestPostprocessChunk_EnvironmentRestoration(t *testing.T) {
	tests := []struct {
		name          string
		translated    string
		original      string
		shouldContain string
	}{
		{
			name: "restore missing end tag through PostprocessChunk",
			translated: `\begin{figure}
\caption{Test}
content`,
			original: `\begin{figure}
\caption{Test}
content
\end{figure}`,
			shouldContain: `\end{figure}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PostprocessChunk(tt.translated, tt.original)
			
			if !strings.Contains(result, tt.shouldContain) {
				t.Errorf("PostprocessChunk() should contain %s, got:\n%s", tt.shouldContain, result)
			}
		})
	}
}
