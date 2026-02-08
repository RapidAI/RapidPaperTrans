// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"strings"
	"testing"
)

// TestRestoreLineStructure tests the RestoreLineStructure function
// Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5
func TestRestoreLineStructure(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantCheck  func(result string) bool
		wantDesc   string
	}{
		{
			name:       "begin on separate line",
			translated: "Some text\\begin{figure}content\\end{figure}",
			original:   "Some text\n\\begin{figure}\ncontent\n\\end{figure}",
			wantCheck: func(result string) bool {
				// \begin{figure} should be on its own line
				lines := strings.Split(result, "\n")
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if strings.Contains(trimmed, "\\begin{figure}") {
						// Line should start with \begin or be only \begin
						return strings.HasPrefix(trimmed, "\\begin{figure}")
					}
				}
				return false
			},
			wantDesc: "\\begin{figure} should be on its own line",
		},
		{
			name:       "end on separate line",
			translated: "content\\end{figure}more text",
			original:   "content\n\\end{figure}\nmore text",
			wantCheck: func(result string) bool {
				// \end{figure} should be on its own line
				lines := strings.Split(result, "\n")
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if strings.Contains(trimmed, "\\end{figure}") {
						// Line should be only \end{figure} or start with it
						return strings.HasPrefix(trimmed, "\\end{figure}")
					}
				}
				return false
			},
			wantDesc: "\\end{figure} should be on its own line",
		},
		{
			name:       "multiple items on separate lines",
			translated: "\\begin{itemize}\\item First\\item Second\\item Third\\end{itemize}",
			original:   "\\begin{itemize}\n\\item First\n\\item Second\n\\item Third\n\\end{itemize}",
			wantCheck: func(result string) bool {
				// Count \item occurrences on separate lines
				lines := strings.Split(result, "\n")
				itemCount := 0
				for _, line := range lines {
					if strings.Contains(line, "\\item") {
						// Count items that start a line (after trimming)
						trimmed := strings.TrimSpace(line)
						if strings.HasPrefix(trimmed, "\\item") {
							itemCount++
						}
					}
				}
				return itemCount >= 3
			},
			wantDesc: "each \\item should be on a separate line",
		},
		{
			name:       "end followed by begin",
			translated: "\\end{figure}\\begin{table}",
			original:   "\\end{figure}\n\\begin{table}",
			wantCheck: func(result string) bool {
				return strings.Contains(result, "\\end{figure}\n") &&
					strings.Contains(result, "\n\\begin{table}")
			},
			wantDesc: "\\end{figure} and \\begin{table} should be on separate lines",
		},
		{
			name:       "preserve already correct structure",
			translated: "\\begin{document}\nHello\n\\end{document}",
			original:   "\\begin{document}\nHello\n\\end{document}",
			wantCheck: func(result string) bool {
				lines := strings.Split(result, "\n")
				return len(lines) >= 3
			},
			wantDesc: "already correct structure should be preserved",
		},
		{
			name:       "empty translated content",
			translated: "",
			original:   "\\begin{document}\n\\end{document}",
			wantCheck: func(result string) bool {
				return result == ""
			},
			wantDesc: "empty content should remain empty",
		},
		{
			name:       "no original content",
			translated: "text\\begin{figure}content\\end{figure}",
			original:   "",
			wantCheck: func(result string) bool {
				// Should still fix basic structure even without original
				return strings.Contains(result, "\n\\begin{figure}") ||
					strings.HasPrefix(result, "\\begin{figure}")
			},
			wantDesc: "should fix structure even without original",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreLineStructure(tt.translated, tt.original)
			if !tt.wantCheck(result) {
				t.Errorf("RestoreLineStructure() failed: %s\nInput: %q\nOutput: %q",
					tt.wantDesc, tt.translated, result)
			}
		})
	}
}

// TestRestoreLineStructureWithDetails tests the detailed version of line structure restoration
func TestRestoreLineStructureWithDetails(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFixes  bool
	}{
		{
			name:       "needs fixes",
			translated: "text\\begin{figure}\\end{figure}",
			original:   "text\n\\begin{figure}\n\\end{figure}",
			wantFixes:  true,
		},
		{
			name:       "already correct",
			translated: "text\n\\begin{figure}\n\\end{figure}",
			original:   "text\n\\begin{figure}\n\\end{figure}",
			wantFixes:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreLineStructureWithDetails(tt.translated, tt.original)
			if result == nil {
				t.Fatal("RestoreLineStructureWithDetails() returned nil")
			}
			if tt.wantFixes && len(result.FixesApplied) == 0 {
				t.Errorf("Expected fixes to be applied, but none were")
			}
			if !tt.wantFixes && len(result.FixesApplied) > 0 {
				t.Errorf("Expected no fixes, but got: %v", result.FixesApplied)
			}
		})
	}
}

// TestEnsureCommandsOnSeparateLines tests the ensureCommandsOnSeparateLines function
func TestEnsureCommandsOnSeparateLines(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(result string) bool
		wantMsg string
	}{
		{
			name:  "text before begin",
			input: "Some text\\begin{figure}",
			check: func(result string) bool {
				return strings.Contains(result, "text\n\\begin{figure}")
			},
			wantMsg: "should add newline before \\begin",
		},
		{
			name:  "text after end",
			input: "\\end{figure}Some text",
			check: func(result string) bool {
				return strings.Contains(result, "\\end{figure}\n")
			},
			wantMsg: "should add newline after \\end",
		},
		{
			name:  "text before item",
			input: "Previous content\\item New item",
			check: func(result string) bool {
				return strings.Contains(result, "content\n\\item")
			},
			wantMsg: "should add newline before \\item",
		},
		{
			name:  "multiple items",
			input: "\\item First item\\item Second item",
			check: func(result string) bool {
				lines := strings.Split(result, "\n")
				itemLines := 0
				for _, line := range lines {
					if strings.Contains(line, "\\item") {
						itemLines++
					}
				}
				return itemLines >= 2
			},
			wantMsg: "should split multiple \\item onto separate lines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureCommandsOnSeparateLines(tt.input)
			if !tt.check(result) {
				t.Errorf("ensureCommandsOnSeparateLines() %s\nInput: %q\nOutput: %q",
					tt.wantMsg, tt.input, result)
			}
		})
	}
}

// TestAnalyzeLineStructure tests the analyzeLineStructure function
func TestAnalyzeLineStructure(t *testing.T) {
	content := `\begin{document}
\begin{figure}
\item First
\item Second
\end{figure}
\section{Test}
\end{document}`

	info := analyzeLineStructure(content)

	if info.TotalLines != 7 {
		t.Errorf("Expected 7 total lines, got %d", info.TotalLines)
	}

	if len(info.BeginLines) != 2 {
		t.Errorf("Expected 2 begin lines, got %d", len(info.BeginLines))
	}

	if len(info.EndLines) != 2 {
		t.Errorf("Expected 2 end lines, got %d", len(info.EndLines))
	}

	if len(info.ItemLines) != 2 {
		t.Errorf("Expected 2 item lines, got %d", len(info.ItemLines))
	}

	if len(info.SectionLines) != 1 {
		t.Errorf("Expected 1 section line, got %d", len(info.SectionLines))
	}
}

// TestSplitMergedStructuralCommands tests the splitMergedStructuralCommands function
func TestSplitMergedStructuralCommands(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(result string) bool
		wantMsg string
	}{
		{
			name:  "end followed by begin",
			input: "\\end{figure}\\begin{table}",
			check: func(result string) bool {
				return strings.Contains(result, "\\end{figure}\n\\begin{table}")
			},
			wantMsg: "should split \\end{figure}\\begin{table}",
		},
		{
			name:  "end followed by section",
			input: "\\end{figure}\\section{Test}",
			check: func(result string) bool {
				return strings.Contains(result, "\\end{figure}\n") &&
					strings.Contains(result, "\\section{Test}")
			},
			wantMsg: "should split \\end{figure}\\section{Test}",
		},
		{
			name:  "end followed by item",
			input: "\\end{enumerate}\\item Next",
			check: func(result string) bool {
				return strings.Contains(result, "\\end{enumerate}\n\\item")
			},
			wantMsg: "should split \\end{enumerate}\\item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitMergedStructuralCommands(tt.input)
			if !tt.check(result) {
				t.Errorf("splitMergedStructuralCommands() %s\nInput: %q\nOutput: %q",
					tt.wantMsg, tt.input, result)
			}
		})
	}
}

// TestLineCountPreservation tests that line count is preserved within tolerance
// Validates: Requirement 2.1
func TestLineCountPreservation(t *testing.T) {
	original := `\begin{document}
\section{Introduction}
This is some text.
\begin{itemize}
\item First item
\item Second item
\end{itemize}
\end{document}`

	// Simulate LLM merging some lines
	translated := `\begin{document}
\section{Introduction}
This is some text.\begin{itemize}\item First item\item Second item\end{itemize}
\end{document}`

	result := RestoreLineStructure(translated, original)

	origLines := len(strings.Split(original, "\n"))
	resultLines := len(strings.Split(result, "\n"))

	// Allow 20% tolerance
	tolerance := origLines / 5
	if tolerance < 2 {
		tolerance = 2
	}

	diff := abs(origLines - resultLines)
	if diff > tolerance {
		t.Errorf("Line count difference too large: original=%d, result=%d, diff=%d, tolerance=%d",
			origLines, resultLines, diff, tolerance)
	}
}
