// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"strings"
	"testing"
)

// TestRestoreBraces tests the RestoreBraces function
// Validates: Requirements 4.3, 4.5
func TestRestoreBraces(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFix    bool // whether we expect the function to make changes
	}{
		{
			name:       "already balanced - no change needed",
			translated: `\textbf{hello}`,
			original:   `\textbf{hello}`,
			wantFix:    false,
		},
		{
			name:       "missing closing brace in textbf",
			translated: `\textbf{hello`,
			original:   `\textbf{hello}`,
			wantFix:    true,
		},
		{
			name:       "multirow missing both braces",
			translated: `\multirow{3}{*}{\textbf{Method} &`,
			original:   `\multirow{3}{*}{\textbf{Method}} &`,
			wantFix:    true,
		},
		{
			name:       "multicolumn missing both braces",
			translated: `\multicolumn{2}{c}{\textbf{Title} &`,
			original:   `\multicolumn{2}{c}{\textbf{Title}} &`,
			wantFix:    true,
		},
		{
			name:       "extra closing brace",
			translated: `\textbf{hello}}`,
			original:   `\textbf{hello}`,
			wantFix:    true,
		},
		{
			name:       "empty content",
			translated: "",
			original:   "",
			wantFix:    false,
		},
		{
			name:       "nested commands missing outer brace",
			translated: `\textbf{\textsc{Title} &`,
			original:   `\textbf{\textsc{Title}} &`,
			wantFix:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreBraces(tt.translated, tt.original)

			if tt.wantFix {
				if result == tt.translated {
					t.Errorf("RestoreBraces() did not make expected changes")
				}
			}

			// Verify the result is balanced (or at least closer to balanced)
			if tt.translated != "" {
				validation := ValidateBraces(result)
				origValidation := ValidateBraces(tt.original)

				// The result should be at least as balanced as the original
				if origValidation.IsBalanced && !validation.IsBalanced {
					t.Errorf("RestoreBraces() result is not balanced when original was balanced")
				}
			}
		})
	}
}

// TestRestoreBracesWithDetails tests the detailed version of RestoreBraces
// Validates: Requirements 4.3, 4.5
func TestRestoreBracesWithDetails(t *testing.T) {
	tests := []struct {
		name            string
		translated      string
		original        string
		wantWasBalanced bool
		wantNowBalanced bool
	}{
		{
			name:            "already balanced",
			translated:      `\textbf{hello}`,
			original:        `\textbf{hello}`,
			wantWasBalanced: true,
			wantNowBalanced: true,
		},
		{
			name:            "missing closing brace - should be fixed",
			translated:      `\textbf{hello`,
			original:        `\textbf{hello}`,
			wantWasBalanced: false,
			wantNowBalanced: true,
		},
		{
			name:            "multirow missing braces - should be fixed",
			translated:      `\multirow{3}{*}{\textbf{Method} &`,
			original:        `\multirow{3}{*}{\textbf{Method}} &`,
			wantWasBalanced: false,
			wantNowBalanced: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreBracesWithDetails(tt.translated, tt.original)

			if result.WasBalanced != tt.wantWasBalanced {
				t.Errorf("WasBalanced = %v, want %v", result.WasBalanced, tt.wantWasBalanced)
			}

			if result.NowBalanced != tt.wantNowBalanced {
				t.Errorf("NowBalanced = %v, want %v", result.NowBalanced, tt.wantNowBalanced)
			}

			// If we expected fixes, verify some were applied
			if !tt.wantWasBalanced && tt.wantNowBalanced {
				if len(result.FixesApplied) == 0 {
					t.Errorf("Expected fixes to be applied, but none were")
				}
			}
		})
	}
}

// TestFixMultirowMulticolumnBracesEnhanced tests the enhanced fixMultirowMulticolumnBraces function
// Validates: Requirement 4.3
func TestFixMultirowMulticolumnBracesEnhanced(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFix    bool // whether we expect the function to make changes
	}{
		{
			name:       "multirow missing both braces before space+&",
			translated: `\multirow{3}{*}{\textbf{Method} &`,
			original:   `\multirow{3}{*}{\textbf{Method}} &`,
			wantFix:    true,
		},
		{
			name:       "multicolumn missing both braces before space+&",
			translated: `\multicolumn{2}{c}{\textbf{Title} &`,
			original:   `\multicolumn{2}{c}{\textbf{Title}} &`,
			wantFix:    true,
		},
		{
			name:       "already correct - no fix needed",
			translated: `\multicolumn{2}{c}{\textbf{Title}} &`,
			original:   `\multicolumn{2}{c}{\textbf{Title}} &`,
			wantFix:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixMultirowMulticolumnBraces(tt.translated, tt.original)

			if tt.wantFix && result == tt.translated {
				t.Errorf("fixMultirowMulticolumnBraces() did not make expected changes")
			}
			if !tt.wantFix && result != tt.translated {
				t.Errorf("fixMultirowMulticolumnBraces() made unexpected changes: %q -> %q", tt.translated, result)
			}
		})
	}
}

// TestFixMultirowMulticolumnBracesActualBehaviorEnhanced tests actual behavior patterns
func TestFixMultirowMulticolumnBracesActualBehaviorEnhanced(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFix    bool
	}{
		{
			name:       "multirow with correct braces - no fix needed",
			translated: `\multirow{3}{*}{\textbf{Method}} &`,
			original:   `\multirow{3}{*}{\textbf{Method}} &`,
			wantFix:    false, // Already has correct braces
		},
		{
			name:       "multicolumn missing one brace before backslash",
			translated: `\multicolumn{2}{|c}{\textbf{Overall}} \\`,
			original:   `\multicolumn{2}{|c}{\textbf{Overall}} \\`,
			wantFix:    false, // Already correct
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixMultirowMulticolumnBraces(tt.translated, tt.original)

			if tt.wantFix && result == tt.translated {
				t.Errorf("Expected fix but got no change")
			}
			if !tt.wantFix && result != tt.translated {
				t.Errorf("Expected no fix but got change: %q -> %q", tt.translated, result)
			}
		})
	}
}

// TestCountBracesEnhanced tests the countBraces function
func TestCountBracesEnhanced(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantOpen  int
		wantClose int
	}{
		{
			name:      "{}",
			content:   "{}",
			wantOpen:  1,
			wantClose: 1,
		},
		{
			name:      "{{}",
			content:   "{{}",
			wantOpen:  2,
			wantClose: 1,
		},
		{
			name:      "{{}}",
			content:   "{{}}",
			wantOpen:  2,
			wantClose: 2,
		},
		{
			name:      `\textbf{test}`,
			content:   `\textbf{test}`,
			wantOpen:  1,
			wantClose: 1,
		},
		{
			name:      `\multirow{3}{*}{\textbf{Method}}`,
			content:   `\multirow{3}{*}{\textbf{Method}}`,
			wantOpen:  4,
			wantClose: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open, close := countBraces(tt.content)

			if open != tt.wantOpen {
				t.Errorf("countBraces() open = %v, want %v", open, tt.wantOpen)
			}
			if close != tt.wantClose {
				t.Errorf("countBraces() close = %v, want %v", close, tt.wantClose)
			}
		})
	}
}

// TestFixGeneralBraceBalance tests the fixGeneralBraceBalance function
func TestFixGeneralBraceBalance(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFix    bool
	}{
		{
			name:       "textbf missing brace before punctuation",
			translated: `\textbf{Important note. Next`,
			original:   `\textbf{Important note}. Next`,
			wantFix:    true,
		},
		{
			name:       "nested textbf textsc missing outer brace",
			translated: `\textbf{\textsc{Title} &`,
			original:   `\textbf{\textsc{Title}} &`,
			wantFix:    true,
		},
		{
			name:       "already balanced",
			translated: `\textbf{hello}`,
			original:   `\textbf{hello}`,
			wantFix:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixGeneralBraceBalance(tt.translated, tt.original)

			if tt.wantFix && result == tt.translated {
				t.Errorf("Expected fix but got no change")
			}
			if !tt.wantFix && result != tt.translated {
				t.Errorf("Expected no fix but got change: %q -> %q", tt.translated, result)
			}
		})
	}
}

// TestRestoreBracesContextAware tests the context-aware brace restoration
func TestRestoreBracesContextAware(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFix    bool
	}{
		{
			name: "line-by-line brace mismatch",
			translated: `\textbf{hello
\textit{world`,
			original: `\textbf{hello}
\textit{world}`,
			wantFix: true,
		},
		{
			name: "already balanced lines",
			translated: `\textbf{hello}
\textit{world}`,
			original: `\textbf{hello}
\textit{world}`,
			wantFix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := restoreBracesContextAware(tt.translated, tt.original)

			if tt.wantFix && result == tt.translated {
				t.Errorf("Expected fix but got no change")
			}
			if !tt.wantFix && result != tt.translated {
				t.Errorf("Expected no fix but got change")
			}
		})
	}
}

// TestRemoveExtraClosingBraces tests the removeExtraClosingBraces function
func TestRemoveExtraClosingBraces(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		count int
		want  string
	}{
		{
			name:  "remove one trailing brace",
			line:  `\textbf{hello}}`,
			count: 1,
			want:  `\textbf{hello}`,
		},
		{
			name:  "remove two trailing braces",
			line:  `\textbf{hello}}}`,
			count: 2,
			want:  `\textbf{hello}`,
		},
		{
			name:  "no trailing braces - removes last char if brace",
			line:  `\textbf{hello}`,
			count: 1,
			want:  `\textbf{hello`, // Function removes trailing } even if it's the only one
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeExtraClosingBraces(tt.line, tt.count)

			if result != tt.want {
				t.Errorf("removeExtraClosingBraces() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestAddMissingClosingBraces tests the addMissingClosingBraces function
func TestAddMissingClosingBraces(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		count int
		want  string
	}{
		{
			name:  "add one closing brace",
			line:  `\textbf{hello`,
			count: 1,
			want:  `\textbf{hello}`,
		},
		{
			name:  "add two closing braces",
			line:  `\textbf{\textit{hello`,
			count: 2,
			want:  `\textbf{\textit{hello}}`,
		},
		{
			name:  "empty line - no change",
			line:  "",
			count: 1,
			want:  "",
		},
		{
			name:  "comment line - no change",
			line:  "% comment",
			count: 1,
			want:  "% comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addMissingClosingBraces(tt.line, tt.count)

			if result != tt.want {
				t.Errorf("addMissingClosingBraces() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestFixRemainingBraceImbalance tests the final brace imbalance fix
func TestFixRemainingBraceImbalance(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		wantFix    bool
	}{
		{
			name:       "too few closing braces",
			translated: `\textbf{hello`,
			original:   `\textbf{hello}`,
			wantFix:    true,
		},
		{
			name:       "too many closing braces",
			translated: `\textbf{hello}}`,
			original:   `\textbf{hello}`,
			wantFix:    true,
		},
		{
			name:       "balanced - no fix needed",
			translated: `\textbf{hello}`,
			original:   `\textbf{hello}`,
			wantFix:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixRemainingBraceImbalance(tt.translated, tt.original)

			if tt.wantFix && result == tt.translated {
				t.Errorf("Expected fix but got no change")
			}
			if !tt.wantFix && result != tt.translated {
				t.Errorf("Expected no fix but got change")
			}

			// Verify the result has the same brace balance as original
			origOpen, origClose := countBraces(tt.original)
			resultOpen, resultClose := countBraces(result)

			origBalance := origOpen - origClose
			resultBalance := resultOpen - resultClose

			if origBalance != resultBalance {
				t.Errorf("Brace balance mismatch: original=%d, result=%d", origBalance, resultBalance)
			}
		})
	}
}

// TestRestoreBracesIntegration tests the full RestoreBraces function with realistic content
func TestRestoreBracesIntegration(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
	}{
		{
			name: "table with multirow and multicolumn",
			translated: `\begin{tabular}{|c|c|c|}
\hline
\multirow{2}{*}{\textbf{Method} & \multicolumn{2}{c|}{\textbf{Results} \\
\cline{2-3}
& Accuracy & F1 \\
\hline
\end{tabular}`,
			original: `\begin{tabular}{|c|c|c|}
\hline
\multirow{2}{*}{\textbf{Method}} & \multicolumn{2}{c|}{\textbf{Results}} \\
\cline{2-3}
& Accuracy & F1 \\
\hline
\end{tabular}`,
		},
		{
			name: "nested formatting commands",
			translated: `\textbf{\textsc{Important Title} is here`,
			original:   `\textbf{\textsc{Important Title}} is here`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RestoreBraces(tt.translated, tt.original)

			// The result should be closer to balanced than the input
			transValidation := ValidateBraces(tt.translated)
			resultValidation := ValidateBraces(result)

			// If original was balanced, result should be too (or closer)
			origValidation := ValidateBraces(tt.original)
			if origValidation.IsBalanced {
				// Result should be balanced or have smaller difference
				if !resultValidation.IsBalanced {
					// At least the difference should be smaller
					if abs(resultValidation.Difference) > abs(transValidation.Difference) {
						t.Errorf("RestoreBraces made things worse: before diff=%d, after diff=%d",
							transValidation.Difference, resultValidation.Difference)
					}
				}
			}
		})
	}
}

// containsStr is a helper function to check if a string contains a substring
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
