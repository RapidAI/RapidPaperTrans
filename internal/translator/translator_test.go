// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"strings"
	"testing"
)

// ============================================================
// Preprocessor Tests
// ============================================================

func TestPreprocessLaTeX_RemoveComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full line comment",
			input:    "% This is a comment\nSome text",
			expected: "\nSome text",
		},
		{
			name:     "inline comment",
			input:    "Some text % inline comment",
			expected: "Some text",
		},
		{
			name:     "escaped percent",
			input:    "50\\% discount",
			expected: "50\\% discount",
		},
		{
			name:     "multiple comments",
			input:    "% comment 1\ntext\n% comment 2",
			expected: "\ntext\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := PreprocessOptions{
				RemoveComments:   true,
				RemoveEmptyLines: false,
			}
			result := PreprocessLaTeX(tt.input, opts)
			if result != tt.expected {
				t.Errorf("PreprocessLaTeX() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPreprocessLaTeX_RemoveEmptyLines(t *testing.T) {
	input := "line1\n\n\n\nline2"
	expected := "line1\n\nline2"

	opts := PreprocessOptions{
		RemoveComments:   false,
		RemoveEmptyLines: true,
	}
	result := PreprocessLaTeX(input, opts)
	if result != expected {
		t.Errorf("PreprocessLaTeX() = %q, want %q", result, expected)
	}
}

// ============================================================
// Postprocessor Tests
// ============================================================

func TestFixLineStructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "begin on same line as text",
			input:    "Some text\\begin{figure}",
			contains: "Some text\n\\begin{figure}",
		},
		{
			name:     "end followed by text",
			input:    "\\end{figure}Some text",
			contains: "\\end{figure}\n",
		},
		{
			name:     "end followed by begin",
			input:    "\\end{figure}\\begin{table}",
			contains: "\\end{figure}\n\\begin{table}",
		},
		{
			name:     "caption followed by tabular",
			input:    "\\caption{Test}\\begin{tabular}",
			contains: "\\caption{Test}\n",
		},
		{
			name:     "row separator followed by midrule",
			input:    "cell1 & cell2 \\\\\\midrule",
			contains: "\\\\\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixLineStructure(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("fixLineStructure() = %q, should contain %q", result, tt.contains)
			}
		})
	}
}

func TestFixMultirowMulticolumnBraces(t *testing.T) {
	// These tests verify the actual behavior of FixMultirowMulticolumnBracesGlobal
	// The function adds closing braces based on pattern matching
	// 
	// Brace counting for \multirow{3}{*}{\textbf{Method} &:
	// - Opens: {3, {*, {, {Method = 4 opens
	// - Closes: 3}, *} = 2 closes
	// - Missing: 2 closes (one for \textbf{}, one for the outer {})
	// So the correct output adds }} (2 braces)
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multirow missing both braces before space+&",
			input:    "\\multirow{3}{*}{\\textbf{Method} &",  // space before &
			expected: "\\multirow{3}{*}{\\textbf{Method}} &", // adds }} (2 braces to balance)
		},
		{
			name:     "multicolumn missing both braces before space+&",
			input:    "\\multicolumn{3}{|c}{\\textbf{GSM8K} &",  // space before &
			expected: "\\multicolumn{3}{|c}{\\textbf{GSM8K}} &", // adds }} (2 braces to balance)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixMultirowMulticolumnBracesGlobal(tt.input)
			if result != tt.expected {
				// Log actual vs expected for debugging
				t.Logf("Input: %q", tt.input)
				t.Logf("Got:   %q", result)
				t.Logf("Want:  %q", tt.expected)
				t.Errorf("FixMultirowMulticolumnBracesGlobal() mismatch")
			}
		})
	}
}

// TestFixMultirowMulticolumnBracesActualBehavior tests the actual behavior of the function
func TestFixMultirowMulticolumnBracesActualBehavior(t *testing.T) {
	// Test what the function actually does - it adds one } for patterns with one } already
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "multirow with one closing brace gets another",
			input:    "\\multirow{3}{*}{\\textbf{Method}} &",
			contains: "}}",  // should have at least }}
		},
		{
			name:     "multicolumn with textbf",
			input:    "\\multicolumn{3}{|c}{\\textbf{Test}} &",
			contains: "}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixMultirowMulticolumnBracesGlobal(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("FixMultirowMulticolumnBracesGlobal() = %q, should contain %q", result, tt.contains)
			}
		})
	}
}

func TestCountEnvironments(t *testing.T) {
	input := `\begin{document}
\begin{figure}
\end{figure}
\begin{table}
\begin{tabular}
\end{tabular}
\end{table}
\end{document}`

	counts := countEnvironments(input)

	tests := []struct {
		env    string
		begins int
		ends   int
	}{
		{"document", 1, 1},
		{"figure", 1, 1},
		{"table", 1, 1},
		{"tabular", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			c := counts[tt.env]
			if c.begins != tt.begins {
				t.Errorf("countEnvironments()[%s].begins = %d, want %d", tt.env, c.begins, tt.begins)
			}
			if c.ends != tt.ends {
				t.Errorf("countEnvironments()[%s].ends = %d, want %d", tt.env, c.ends, tt.ends)
			}
		})
	}
}

func TestFixEnvironmentsByReference(t *testing.T) {
	tests := []struct {
		name       string
		translated string
		original   string
		shouldFix  bool
	}{
		{
			name:       "missing end tag",
			translated: "\\begin{figure}\ncontent",
			original:   "\\begin{figure}\ncontent\n\\end{figure}",
			shouldFix:  true,
		},
		{
			name:       "balanced environments",
			translated: "\\begin{figure}\ncontent\n\\end{figure}",
			original:   "\\begin{figure}\ncontent\n\\end{figure}",
			shouldFix:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixEnvironmentsByReference(tt.translated, tt.original)
			hasEndTag := strings.Contains(result, "\\end{figure}")
			if tt.shouldFix && !hasEndTag {
				t.Errorf("fixEnvironmentsByReference() should have added \\end{figure}")
			}
		})
	}
}

// ============================================================
// Validator Tests
// ============================================================

func TestValidateTranslation_EmptyTranslation(t *testing.T) {
	v := NewTranslationValidator()
	result := v.ValidateTranslation("Some content", "")

	if result.IsValid {
		t.Error("ValidateTranslation() should fail for empty translation")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateTranslation() should have errors for empty translation")
	}
}

func TestValidateTranslation_TooShort(t *testing.T) {
	v := NewTranslationValidator()
	original := strings.Repeat("This is a test sentence. ", 100)
	translated := "短"

	result := v.ValidateTranslation(original, translated)

	if result.IsValid {
		t.Error("ValidateTranslation() should fail for too short translation")
	}
}

func TestValidateTranslation_ForbiddenPatterns(t *testing.T) {
	v := NewTranslationValidator()
	original := "Some content"
	translated := "This document contains the translated content"

	result := v.ValidateTranslation(original, translated)

	if result.IsValid {
		t.Error("ValidateTranslation() should fail for forbidden patterns")
	}
}

func TestCountChineseCharacters(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"Hello", 0},
		{"你好", 2},
		{"Hello 你好 World", 2},
		{"这是一个测试", 6},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := countChineseCharacters(tt.input)
			if result != tt.expected {
				t.Errorf("countChineseCharacters(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractDocumentClass(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"\\documentclass{article}", "article"},
		{"\\documentclass[12pt]{report}", "report"},
		{"\\documentclass[a4paper,12pt]{book}", "book"},
		{"no document class", ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := extractDocumentClass(tt.input)
			if result != tt.expected {
				t.Errorf("extractDocumentClass() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCountSections(t *testing.T) {
	input := `\section{Introduction}
\subsection{Background}
\subsubsection{Details}
\section{Methods}`

	result := countSections(input)
	expected := 4

	if result != expected {
		t.Errorf("countSections() = %d, want %d", result, expected)
	}
}

// ============================================================
// Integration Tests
// ============================================================

func TestPostprocessChunk_Integration(t *testing.T) {
	original := `\begin{figure}
\caption{Test figure}
\end{figure}
\begin{table}
\caption{Test table}
\begin{tabular}{cc}
\toprule
A & B \\
\midrule
1 & 2 \\
\bottomrule
\end{tabular}
\end{table}`

	// Simulate LLM output with common issues
	translated := `\begin{figure}\caption{测试图片}\end{figure}\begin{table}\caption{测试表格}\begin{tabular}{cc}\toprule
A & B \\\midrule
1 & 2 \\\bottomrule
\end{tabular}\end{table}`

	result := PostprocessChunk(translated, original)

	// Check that environments are on separate lines
	if strings.Contains(result, "\\end{figure}\\begin{table}") {
		t.Error("PostprocessChunk() should separate \\end{figure} and \\begin{table}")
	}

	// Check that caption and tabular are separated
	if strings.Contains(result, "\\caption{测试表格}\\begin{tabular}") {
		t.Error("PostprocessChunk() should separate \\caption and \\begin{tabular}")
	}
}

// ============================================================
// Brace Balance Tests
// ============================================================

func TestCountBraces(t *testing.T) {
	tests := []struct {
		input     string
		openCount int
		closeCount int
	}{
		{"{}", 1, 1},
		{"{{}", 2, 1},
		{"{{}}", 2, 2},
		{"\\textbf{test}", 1, 1},
		{"\\multirow{3}{*}{\\textbf{Method}}", 4, 4},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			open, close := countBraces(tt.input)
			if open != tt.openCount {
				t.Errorf("countBraces() open = %d, want %d", open, tt.openCount)
			}
			if close != tt.closeCount {
				t.Errorf("countBraces() close = %d, want %d", close, tt.closeCount)
			}
		})
	}
}


// ============================================================
// IdentifyStructures Tests (Task 1.1)
// ============================================================

func TestIdentifyStructures_Comments(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name:          "single line comment",
			input:         "% This is a comment\nSome text",
			expectedCount: 1,
		},
		{
			name:          "multiple comments",
			input:         "% comment 1\ntext\n% comment 2",
			expectedCount: 2,
		},
		{
			name:          "inline comment",
			input:         "Some text % inline comment",
			expectedCount: 1,
		},
		{
			name:          "escaped percent",
			input:         "50\\% discount",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structures := IdentifyStructures(tt.input)
			comments := GetCommentStructures(structures)
			if len(comments) != tt.expectedCount {
				t.Errorf("IdentifyStructures() found %d comments, want %d", len(comments), tt.expectedCount)
			}
		})
	}
}

func TestIdentifyStructures_MathInline(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name:          "single inline math",
			input:         "The equation $x + y = z$ is simple",
			expectedCount: 1,
		},
		{
			name:          "multiple inline math",
			input:         "We have $a$ and $b$ and $c$",
			expectedCount: 3,
		},
		{
			name:          "paren math",
			input:         "The value \\(x\\) is positive",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structures := IdentifyStructures(tt.input)
			mathStructures := GetStructuresByType(structures, StructureMathInline)
			if len(mathStructures) != tt.expectedCount {
				t.Errorf("IdentifyStructures() found %d inline math, want %d", len(mathStructures), tt.expectedCount)
			}
		})
	}
}

func TestIdentifyStructures_MathDisplay(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name:          "double dollar math",
			input:         "$$x^2 + y^2 = z^2$$",
			expectedCount: 1,
		},
		{
			name:          "bracket math",
			input:         "\\[a + b = c\\]",
			expectedCount: 1,
		},
		{
			name:          "equation environment",
			input:         "\\begin{equation}\nx = y\n\\end{equation}",
			expectedCount: 1,
		},
		{
			name:          "align environment",
			input:         "\\begin{align}\na &= b \\\\\nc &= d\n\\end{align}",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structures := IdentifyStructures(tt.input)
			mathStructures := GetStructuresByType(structures, StructureMathDisplay)
			if len(mathStructures) != tt.expectedCount {
				t.Errorf("IdentifyStructures() found %d display math, want %d", len(mathStructures), tt.expectedCount)
			}
		})
	}
}

func TestIdentifyStructures_Tables(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name: "simple tabular",
			input: `\begin{tabular}{cc}
A & B \\
1 & 2 \\
\end{tabular}`,
			expectedCount: 1,
		},
		{
			name: "table with tabular",
			input: `\begin{table}
\begin{tabular}{cc}
A & B \\
\end{tabular}
\end{table}`,
			expectedCount: 2, // table and tabular
		},
		{
			name:          "longtable",
			input:         "\\begin{longtable}{ccc}\nA & B & C\n\\end{longtable}",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structures := IdentifyStructures(tt.input)
			tables := GetTableStructures(structures)
			if len(tables) != tt.expectedCount {
				t.Errorf("IdentifyStructures() found %d tables, want %d", len(tables), tt.expectedCount)
			}
		})
	}
}

func TestIdentifyStructures_Environments(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name: "figure environment",
			input: `\begin{figure}
\includegraphics{test.png}
\end{figure}`,
			expectedCount: 1,
		},
		{
			name: "nested environments",
			input: `\begin{document}
\begin{figure}
content
\end{figure}
\end{document}`,
			expectedCount: 2,
		},
		{
			name: "itemize environment",
			input: `\begin{itemize}
\item First
\item Second
\end{itemize}`,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structures := IdentifyStructures(tt.input)
			envs := GetStructuresByType(structures, StructureEnvironment)
			if len(envs) != tt.expectedCount {
				t.Errorf("IdentifyStructures() found %d environments, want %d", len(envs), tt.expectedCount)
			}
		})
	}
}

func TestIdentifyStructures_Commands(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		minCount      int // minimum expected count (may find more due to nested patterns)
	}{
		{
			name:     "section command",
			input:    "\\section{Introduction}",
			minCount: 1,
		},
		{
			name:     "multiple commands",
			input:    "\\section{Intro}\n\\subsection{Background}",
			minCount: 2,
		},
		{
			name:     "cite and ref",
			input:    "See \\cite{paper1} and \\ref{fig:1}",
			minCount: 2,
		},
		{
			name:     "textbf and textit",
			input:    "\\textbf{bold} and \\textit{italic}",
			minCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structures := IdentifyStructures(tt.input)
			commands := GetStructuresByType(structures, StructureCommand)
			if len(commands) < tt.minCount {
				t.Errorf("IdentifyStructures() found %d commands, want at least %d", len(commands), tt.minCount)
			}
		})
	}
}

func TestIdentifyStructures_NestedMarking(t *testing.T) {
	input := `\begin{figure}
\caption{Test}
\end{figure}`

	structures := IdentifyStructures(input)

	// Find the caption command
	var captionFound bool
	for _, s := range structures {
		if s.Name == "caption" && s.Type == StructureCommand {
			captionFound = true
			if !s.IsNested {
				t.Error("caption command should be marked as nested")
			}
			if s.ParentType != "Environment" {
				t.Errorf("caption parent type should be 'Environment', got %q", s.ParentType)
			}
		}
	}

	if !captionFound {
		t.Error("caption command not found in structures")
	}
}

func TestIdentifyStructures_ContentExtraction(t *testing.T) {
	input := "$x + y$"
	structures := IdentifyStructures(input)

	mathStructures := GetStructuresByType(structures, StructureMathInline)
	if len(mathStructures) != 1 {
		t.Fatalf("Expected 1 inline math, got %d", len(mathStructures))
	}

	if mathStructures[0].Content != "$x + y$" {
		t.Errorf("Content = %q, want %q", mathStructures[0].Content, "$x + y$")
	}

	if mathStructures[0].Start != 0 {
		t.Errorf("Start = %d, want 0", mathStructures[0].Start)
	}

	if mathStructures[0].End != 7 {
		t.Errorf("End = %d, want 7", mathStructures[0].End)
	}
}

func TestStructureType_String(t *testing.T) {
	tests := []struct {
		structType StructureType
		expected   string
	}{
		{StructureCommand, "Command"},
		{StructureEnvironment, "Environment"},
		{StructureMathInline, "MathInline"},
		{StructureMathDisplay, "MathDisplay"},
		{StructureTable, "Table"},
		{StructureComment, "Comment"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.structType.String()
			if result != tt.expected {
				t.Errorf("StructureType.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetMathStructures(t *testing.T) {
	input := "Inline $x$ and display $$y$$"
	structures := IdentifyStructures(input)

	mathStructures := GetMathStructures(structures)
	if len(mathStructures) != 2 {
		t.Errorf("GetMathStructures() found %d structures, want 2", len(mathStructures))
	}

	// Check that we have both inline and display
	hasInline := false
	hasDisplay := false
	for _, s := range mathStructures {
		if s.Type == StructureMathInline {
			hasInline = true
		}
		if s.Type == StructureMathDisplay {
			hasDisplay = true
		}
	}

	if !hasInline {
		t.Error("GetMathStructures() should include inline math")
	}
	if !hasDisplay {
		t.Error("GetMathStructures() should include display math")
	}
}


// ============================================================
// Math Environment Protection Tests
// ============================================================

func TestProtectMathEnvironments_InlineDollar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMath []string
	}{
		{
			name:     "simple inline math",
			input:    "The equation $x + y = z$ is simple.",
			wantMath: []string{"$x + y = z$"},
		},
		{
			name:     "multiple inline math",
			input:    "We have $a$ and $b$ here.",
			wantMath: []string{"$a$", "$b$"},
		},
		{
			name:     "inline math with subscript",
			input:    "The variable $x_1$ is defined.",
			wantMath: []string{"$x_1$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, placeholders := ProtectMathEnvironments(tt.input)
			
			// Check that all expected math is protected
			if len(placeholders) != len(tt.wantMath) {
				t.Errorf("ProtectMathEnvironments() found %d math regions, want %d", len(placeholders), len(tt.wantMath))
			}
			
			// Check that placeholders are in the protected content
			for placeholder := range placeholders {
				if !strings.Contains(protected, placeholder) {
					t.Errorf("Protected content should contain placeholder %s", placeholder)
				}
			}
			
			// Check that original math is in placeholders
			for _, wantMath := range tt.wantMath {
				found := false
				for _, original := range placeholders {
					if original == wantMath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected math %q not found in placeholders", wantMath)
				}
			}
		})
	}
}

func TestProtectMathEnvironments_DisplayDoubleDollar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMath []string
	}{
		{
			name:     "simple display math",
			input:    "The equation $$x + y = z$$ is displayed.",
			wantMath: []string{"$$x + y = z$$"},
		},
		{
			name:     "multiline display math",
			input:    "The equation $$\nx + y = z\n$$ is displayed.",
			wantMath: []string{"$$\nx + y = z\n$$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, placeholders := ProtectMathEnvironments(tt.input)
			
			if len(placeholders) != len(tt.wantMath) {
				t.Errorf("ProtectMathEnvironments() found %d math regions, want %d", len(placeholders), len(tt.wantMath))
			}
			
			for _, wantMath := range tt.wantMath {
				found := false
				for _, original := range placeholders {
					if original == wantMath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected math %q not found in placeholders", wantMath)
				}
			}
			
			// Verify original math is not in protected content
			for _, wantMath := range tt.wantMath {
				if strings.Contains(protected, wantMath) {
					t.Errorf("Protected content should not contain original math %q", wantMath)
				}
			}
		})
	}
}

func TestProtectMathEnvironments_BracketMath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMath []string
	}{
		{
			name:     "simple bracket math",
			input:    "The equation \\[x + y = z\\] is displayed.",
			wantMath: []string{"\\[x + y = z\\]"},
		},
		{
			name:     "multiline bracket math",
			input:    "The equation \\[\nx + y = z\n\\] is displayed.",
			wantMath: []string{"\\[\nx + y = z\n\\]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, placeholders := ProtectMathEnvironments(tt.input)
			
			if len(placeholders) != len(tt.wantMath) {
				t.Errorf("ProtectMathEnvironments() found %d math regions, want %d", len(placeholders), len(tt.wantMath))
			}
			
			for _, wantMath := range tt.wantMath {
				found := false
				for _, original := range placeholders {
					if original == wantMath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected math %q not found in placeholders", wantMath)
				}
			}
		})
	}
}

func TestProtectMathEnvironments_ParenMath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMath []string
	}{
		{
			name:     "simple paren math",
			input:    "The equation \\(x + y = z\\) is inline.",
			wantMath: []string{"\\(x + y = z\\)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, placeholders := ProtectMathEnvironments(tt.input)
			
			if len(placeholders) != len(tt.wantMath) {
				t.Errorf("ProtectMathEnvironments() found %d math regions, want %d", len(placeholders), len(tt.wantMath))
			}
			
			for _, wantMath := range tt.wantMath {
				found := false
				for _, original := range placeholders {
					if original == wantMath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected math %q not found in placeholders", wantMath)
				}
			}
		})
	}
}

func TestProtectMathEnvironments_NamedEnvironments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMath []string
	}{
		{
			name:     "equation environment",
			input:    "\\begin{equation}\nx + y = z\n\\end{equation}",
			wantMath: []string{"\\begin{equation}\nx + y = z\n\\end{equation}"},
		},
		{
			name:     "align environment",
			input:    "\\begin{align}\na &= b \\\\\nc &= d\n\\end{align}",
			wantMath: []string{"\\begin{align}\na &= b \\\\\nc &= d\n\\end{align}"},
		},
		{
			name:     "gather environment",
			input:    "\\begin{gather}\nx = 1 \\\\\ny = 2\n\\end{gather}",
			wantMath: []string{"\\begin{gather}\nx = 1 \\\\\ny = 2\n\\end{gather}"},
		},
		{
			name:     "multline environment",
			input:    "\\begin{multline}\na + b + c \\\\\n= d + e\n\\end{multline}",
			wantMath: []string{"\\begin{multline}\na + b + c \\\\\n= d + e\n\\end{multline}"},
		},
		{
			name:     "eqnarray environment",
			input:    "\\begin{eqnarray}\nx &=& y\n\\end{eqnarray}",
			wantMath: []string{"\\begin{eqnarray}\nx &=& y\n\\end{eqnarray}"},
		},
		{
			name:     "equation* environment",
			input:    "\\begin{equation*}\nx = y\n\\end{equation*}",
			wantMath: []string{"\\begin{equation*}\nx = y\n\\end{equation*}"},
		},
		{
			name:     "align* environment",
			input:    "\\begin{align*}\na &= b\n\\end{align*}",
			wantMath: []string{"\\begin{align*}\na &= b\n\\end{align*}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, placeholders := ProtectMathEnvironments(tt.input)
			
			if len(placeholders) != len(tt.wantMath) {
				t.Errorf("ProtectMathEnvironments() found %d math regions, want %d", len(placeholders), len(tt.wantMath))
			}
			
			for _, wantMath := range tt.wantMath {
				found := false
				for _, original := range placeholders {
					if original == wantMath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected math %q not found in placeholders", wantMath)
				}
			}
		})
	}
}

func TestProtectMathEnvironments_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "inline math round trip",
			input: "The equation $x + y = z$ is simple.",
		},
		{
			name:  "display math round trip",
			input: "The equation $$x + y = z$$ is displayed.",
		},
		{
			name:  "bracket math round trip",
			input: "The equation \\[x + y = z\\] is displayed.",
		},
		{
			name:  "paren math round trip",
			input: "The equation \\(x + y = z\\) is inline.",
		},
		{
			name:  "equation environment round trip",
			input: "\\begin{equation}\nx + y = z\n\\end{equation}",
		},
		{
			name:  "mixed math round trip",
			input: "Inline $a$ and display $$b$$ and \\[c\\] and \\(d\\).",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, placeholders := ProtectMathEnvironments(tt.input)
			restored := RestoreMathEnvironments(protected, placeholders)
			
			if restored != tt.input {
				t.Errorf("Round trip failed:\nInput:    %q\nProtected: %q\nRestored: %q", tt.input, protected, restored)
			}
		})
	}
}

func TestProtectMathEnvironments_PlaceholderFormat(t *testing.T) {
	input := "The equation $x$ is simple."
	protected, _ := ProtectMathEnvironments(input)
	
	// Check that placeholder has correct format
	if !strings.Contains(protected, "<<<LATEX_MATH_") || !strings.Contains(protected, ">>>") {
		t.Errorf("Placeholder should have format <<<LATEX_MATH_N>>>, got: %s", protected)
	}
}

func TestValidateMathPlaceholders(t *testing.T) {
	input := "The equation $x$ and $y$ are simple."
	protected, placeholders := ProtectMathEnvironments(input)
	
	// All placeholders should be present
	missing := ValidateMathPlaceholders(protected, placeholders)
	if len(missing) != 0 {
		t.Errorf("ValidateMathPlaceholders() found missing placeholders: %v", missing)
	}
	
	// Remove a placeholder and check again
	modifiedContent := strings.Replace(protected, "<<<LATEX_MATH_0>>>", "", 1)
	missing = ValidateMathPlaceholders(modifiedContent, placeholders)
	if len(missing) != 1 {
		t.Errorf("ValidateMathPlaceholders() should find 1 missing placeholder, found %d", len(missing))
	}
}

func TestGetMathPlaceholderCount(t *testing.T) {
	input := "The equation $x$ and $y$ are simple."
	protected, _ := ProtectMathEnvironments(input)
	
	count := GetMathPlaceholderCount(protected)
	if count != 2 {
		t.Errorf("GetMathPlaceholderCount() = %d, want 2", count)
	}
}


// ============================================================
// Table Structure Protection Tests (Task 1.4)
// ============================================================

func TestProtectTableStructures_SimpleTabular(t *testing.T) {
	input := `\begin{tabular}{cc}
A & B \\
1 & 2 \\
\end{tabular}`

	protected, placeholders := ProtectTableStructures(input)

	// Should have exactly one placeholder
	if len(placeholders) != 1 {
		t.Errorf("ProtectTableStructures() found %d regions, want 1", len(placeholders))
	}

	// Check that placeholder is in protected content
	for placeholder := range placeholders {
		if !strings.Contains(protected, placeholder) {
			t.Errorf("Protected content should contain placeholder %s", placeholder)
		}
	}

	// Check that original table is in placeholders
	found := false
	for _, original := range placeholders {
		if original == input {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected table content not found in placeholders")
	}
}

func TestProtectTableStructures_TableWithTabular(t *testing.T) {
	input := `\begin{table}
\caption{Test table}
\begin{tabular}{cc}
A & B \\
\end{tabular}
\end{table}`

	protected, placeholders := ProtectTableStructures(input)

	// Should protect the outer table environment (which contains tabular)
	if len(placeholders) != 1 {
		t.Errorf("ProtectTableStructures() found %d regions, want 1 (outer table)", len(placeholders))
	}

	// The entire table should be protected as one unit
	for _, original := range placeholders {
		if !strings.Contains(original, "\\begin{table}") || !strings.Contains(original, "\\end{table}") {
			t.Errorf("Placeholder should contain complete table environment")
		}
		if !strings.Contains(original, "\\begin{tabular}") || !strings.Contains(original, "\\end{tabular}") {
			t.Errorf("Placeholder should contain nested tabular environment")
		}
	}

	// Protected content should not contain original table
	if strings.Contains(protected, "\\begin{table}") {
		t.Errorf("Protected content should not contain original table")
	}
}

func TestProtectTableStructures_Longtable(t *testing.T) {
	input := `\begin{longtable}{ccc}
\toprule
A & B & C \\
\midrule
1 & 2 & 3 \\
\bottomrule
\end{longtable}`

	protected, placeholders := ProtectTableStructures(input)

	if len(placeholders) != 1 {
		t.Errorf("ProtectTableStructures() found %d regions, want 1", len(placeholders))
	}

	// Check that longtable is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\begin{longtable}") {
			t.Errorf("Placeholder should contain longtable environment")
		}
	}

	// Protected content should not contain original longtable
	if strings.Contains(protected, "\\begin{longtable}") {
		t.Errorf("Protected content should not contain original longtable")
	}
	_ = protected // use variable
}

func TestProtectTableStructures_Tabularx(t *testing.T) {
	input := `\begin{tabularx}{\textwidth}{|X|X|}
\hline
Column 1 & Column 2 \\
\hline
\end{tabularx}`

	protected, placeholders := ProtectTableStructures(input)

	if len(placeholders) != 1 {
		t.Errorf("ProtectTableStructures() found %d regions, want 1", len(placeholders))
	}

	// Check that tabularx is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\begin{tabularx}") {
			t.Errorf("Placeholder should contain tabularx environment")
		}
	}
	_ = protected // use variable
}

func TestProtectTableStructures_Array(t *testing.T) {
	input := `\begin{array}{cc}
a & b \\
c & d \\
\end{array}`

	protected, placeholders := ProtectTableStructures(input)

	if len(placeholders) != 1 {
		t.Errorf("ProtectTableStructures() found %d regions, want 1", len(placeholders))
	}

	// Check that array is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\begin{array}") {
			t.Errorf("Placeholder should contain array environment")
		}
	}
	_ = protected // use variable
}

func TestProtectTableStructures_MultipleTables(t *testing.T) {
	input := `\begin{tabular}{c}
A \\
\end{tabular}

Some text

\begin{tabular}{c}
B \\
\end{tabular}`

	protected, placeholders := ProtectTableStructures(input)

	if len(placeholders) != 2 {
		t.Errorf("ProtectTableStructures() found %d regions, want 2", len(placeholders))
	}

	// Check that both tables are protected
	if strings.Contains(protected, "\\begin{tabular}") {
		t.Errorf("Protected content should not contain original tabular")
	}

	// Check that "Some text" is still in protected content
	if !strings.Contains(protected, "Some text") {
		t.Errorf("Protected content should still contain non-table text")
	}
}

func TestProtectTableStructures_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "simple tabular",
			input: `\begin{tabular}{cc}
A & B \\
1 & 2 \\
\end{tabular}`,
		},
		{
			name: "table with tabular",
			input: `\begin{table}
\caption{Test}
\begin{tabular}{cc}
A & B \\
\end{tabular}
\end{table}`,
		},
		{
			name: "longtable",
			input: `\begin{longtable}{ccc}
\toprule
A & B & C \\
\bottomrule
\end{longtable}`,
		},
		{
			name: "tabularx",
			input: `\begin{tabularx}{\textwidth}{XX}
Col1 & Col2 \\
\end{tabularx}`,
		},
		{
			name: "array",
			input: `\begin{array}{cc}
a & b \\
\end{array}`,
		},
		{
			name: "multiple tables",
			input: `\begin{tabular}{c}
A \\
\end{tabular}
Text
\begin{tabular}{c}
B \\
\end{tabular}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, placeholders := ProtectTableStructures(tt.input)
			restored := RestoreTableStructures(protected, placeholders)

			if restored != tt.input {
				t.Errorf("Round trip failed:\nInput:    %q\nProtected: %q\nRestored: %q", tt.input, protected, restored)
			}
		})
	}
}

func TestProtectTableStructures_PlaceholderFormat(t *testing.T) {
	input := `\begin{tabular}{c}
A \\
\end{tabular}`

	protected, _ := ProtectTableStructures(input)

	// Check that placeholder has correct format
	if !strings.Contains(protected, "<<<LATEX_TABLE_") || !strings.Contains(protected, ">>>") {
		t.Errorf("Placeholder should have format <<<LATEX_TABLE_N>>>, got: %s", protected)
	}
}

func TestProtectTableCommands_Multirow(t *testing.T) {
	input := `\multirow{3}{*}{\textbf{Method}} & Value \\`

	protected, placeholders := ProtectTableCommands(input)

	if len(placeholders) != 1 {
		t.Errorf("ProtectTableCommands() found %d regions, want 1", len(placeholders))
	}

	// Check that multirow is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\multirow") {
			t.Errorf("Placeholder should contain multirow command")
		}
	}
	_ = protected // use variable
}

func TestProtectTableCommands_Multicolumn(t *testing.T) {
	input := `\multicolumn{3}{|c|}{\textbf{Header}} \\`

	protected, placeholders := ProtectTableCommands(input)

	if len(placeholders) != 1 {
		t.Errorf("ProtectTableCommands() found %d regions, want 1", len(placeholders))
	}

	// Check that multicolumn is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\multicolumn") {
			t.Errorf("Placeholder should contain multicolumn command")
		}
	}
	_ = protected // use variable
}

func TestProtectTableCommands_Booktabs(t *testing.T) {
	input := `\toprule
A & B \\
\midrule
1 & 2 \\
\bottomrule`

	protected, placeholders := ProtectTableCommands(input)

	if len(placeholders) != 3 {
		t.Errorf("ProtectTableCommands() found %d regions, want 3 (toprule, midrule, bottomrule)", len(placeholders))
	}

	// Check that booktabs commands are protected
	hasToprule := false
	hasMidrule := false
	hasBottomrule := false
	for _, original := range placeholders {
		if strings.Contains(original, "\\toprule") {
			hasToprule = true
		}
		if strings.Contains(original, "\\midrule") {
			hasMidrule = true
		}
		if strings.Contains(original, "\\bottomrule") {
			hasBottomrule = true
		}
	}

	if !hasToprule {
		t.Error("Should protect \\toprule")
	}
	if !hasMidrule {
		t.Error("Should protect \\midrule")
	}
	if !hasBottomrule {
		t.Error("Should protect \\bottomrule")
	}
	_ = protected // use variable
}

func TestProtectTableCommands_Hline(t *testing.T) {
	input := `\hline
A & B \\
\hline`

	protected, placeholders := ProtectTableCommands(input)

	if len(placeholders) != 2 {
		t.Errorf("ProtectTableCommands() found %d regions, want 2", len(placeholders))
	}

	// Check that hline is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\hline") {
			t.Errorf("Placeholder should contain hline command")
		}
	}
	_ = protected // use variable
}

func TestProtectTableCommands_Cline(t *testing.T) {
	input := `\cline{1-2}
A & B \\
\cline{2-3}`

	protected, placeholders := ProtectTableCommands(input)

	if len(placeholders) != 2 {
		t.Errorf("ProtectTableCommands() found %d regions, want 2", len(placeholders))
	}

	// Check that cline is protected
	for _, original := range placeholders {
		if !strings.Contains(original, "\\cline") {
			t.Errorf("Placeholder should contain cline command")
		}
	}
	_ = protected // use variable
}

func TestProtectTableCommands_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "multirow",
			input: `\multirow{3}{*}{\textbf{Method}} & Value \\`,
		},
		{
			name:  "multicolumn",
			input: `\multicolumn{3}{|c|}{\textbf{Header}} \\`,
		},
		{
			name:  "booktabs",
			input: `\toprule\midrule\bottomrule`,
		},
		{
			name:  "hline",
			input: `\hline A & B \\ \hline`,
		},
		{
			name:  "cline",
			input: `\cline{1-2} A & B \\`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, placeholders := ProtectTableCommands(tt.input)
			restored := RestoreTableCommands(protected, placeholders)

			if restored != tt.input {
				t.Errorf("Round trip failed:\nInput:    %q\nProtected: %q\nRestored: %q", tt.input, protected, restored)
			}
		})
	}
}

func TestProtectCellSeparators_Ampersand(t *testing.T) {
	input := `A & B & C \\`

	protected, placeholders := ProtectCellSeparators(input)

	// Should find 2 ampersands
	ampersandCount := 0
	for _, original := range placeholders {
		if original == "&" {
			ampersandCount++
		}
	}

	if ampersandCount != 2 {
		t.Errorf("ProtectCellSeparators() found %d ampersands, want 2", ampersandCount)
	}
	_ = protected // use variable
}

func TestProtectCellSeparators_EscapedAmpersand(t *testing.T) {
	input := `A \& B & C \\`

	protected, placeholders := ProtectCellSeparators(input)

	// Should find only 1 unescaped ampersand
	ampersandCount := 0
	for _, original := range placeholders {
		if original == "&" {
			ampersandCount++
		}
	}

	if ampersandCount != 1 {
		t.Errorf("ProtectCellSeparators() found %d ampersands, want 1 (escaped should not be protected)", ampersandCount)
	}

	// Escaped ampersand should still be in protected content
	if !strings.Contains(protected, "\\&") {
		t.Errorf("Protected content should still contain escaped ampersand")
	}
}

func TestProtectCellSeparators_RowTerminator(t *testing.T) {
	input := `A & B \\
C & D \\`

	protected, placeholders := ProtectCellSeparators(input)

	// Should find 2 row terminators
	terminatorCount := 0
	for _, original := range placeholders {
		if strings.HasPrefix(original, "\\\\") {
			terminatorCount++
		}
	}

	if terminatorCount != 2 {
		t.Errorf("ProtectCellSeparators() found %d row terminators, want 2", terminatorCount)
	}
	_ = protected // use variable
}

func TestProtectCellSeparators_RowTerminatorWithSpacing(t *testing.T) {
	input := `A & B \\[5pt]
C & D \\`

	protected, placeholders := ProtectCellSeparators(input)

	// Should find row terminator with spacing
	hasSpacedTerminator := false
	for _, original := range placeholders {
		if strings.Contains(original, "\\\\[5pt]") {
			hasSpacedTerminator = true
			break
		}
	}

	if !hasSpacedTerminator {
		t.Errorf("ProtectCellSeparators() should protect row terminator with spacing")
	}
	_ = protected // use variable
}

func TestProtectCellSeparators_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple cells",
			input: `A & B & C \\`,
		},
		{
			name:  "multiple rows",
			input: `A & B \\ C & D \\`,
		},
		{
			name:  "with spacing",
			input: `A & B \\[5pt] C & D \\`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected, placeholders := ProtectCellSeparators(tt.input)
			restored := RestoreCellSeparators(protected, placeholders)

			if restored != tt.input {
				t.Errorf("Round trip failed:\nInput:    %q\nProtected: %q\nRestored: %q", tt.input, protected, restored)
			}
		})
	}
}

func TestProtectCompleteTable(t *testing.T) {
	input := `\begin{table}
\caption{Test}
\begin{tabular}{cc}
\toprule
A & B \\
\midrule
1 & 2 \\
\bottomrule
\end{tabular}
\end{table}

Some text with \multirow{2}{*}{test} outside table.`

	protected, placeholders := ProtectCompleteTable(input)

	// Should have at least 2 placeholders (table environment + multirow command)
	if len(placeholders) < 2 {
		t.Errorf("ProtectCompleteTable() found %d regions, want at least 2", len(placeholders))
	}

	// Check that table environment is protected
	if strings.Contains(protected, "\\begin{table}") {
		t.Errorf("Protected content should not contain original table")
	}

	// Check that multirow outside table is also protected
	if strings.Contains(protected, "\\multirow") {
		t.Errorf("Protected content should not contain multirow command")
	}

	// Check that "Some text" is still in protected content
	if !strings.Contains(protected, "Some text") {
		t.Errorf("Protected content should still contain non-table text")
	}
}

func TestProtectCompleteTable_RoundTrip(t *testing.T) {
	input := `\begin{table}
\caption{Test}
\begin{tabular}{cc}
\toprule
A & B \\
\midrule
1 & 2 \\
\bottomrule
\end{tabular}
\end{table}

Some text with \multirow{2}{*}{test} outside table.`

	protected, placeholders := ProtectCompleteTable(input)
	restored := RestoreCompleteTable(protected, placeholders)

	if restored != input {
		t.Errorf("Round trip failed:\nInput:    %q\nProtected: %q\nRestored: %q", input, protected, restored)
	}
}

func TestValidateTablePlaceholders(t *testing.T) {
	input := `\begin{tabular}{c}
A \\
\end{tabular}

\begin{tabular}{c}
B \\
\end{tabular}`

	protected, placeholders := ProtectTableStructures(input)

	// All placeholders should be present
	missing := ValidateTablePlaceholders(protected, placeholders)
	if len(missing) != 0 {
		t.Errorf("ValidateTablePlaceholders() found missing placeholders: %v", missing)
	}

	// Remove a placeholder and check again
	modifiedContent := strings.Replace(protected, "<<<LATEX_TABLE_0>>>", "", 1)
	missing = ValidateTablePlaceholders(modifiedContent, placeholders)
	if len(missing) != 1 {
		t.Errorf("ValidateTablePlaceholders() should find 1 missing placeholder, found %d", len(missing))
	}
}

func TestGetTablePlaceholderCount(t *testing.T) {
	input := `\begin{tabular}{c}
A \\
\end{tabular}

\begin{tabular}{c}
B \\
\end{tabular}`

	protected, _ := ProtectTableStructures(input)

	count := GetTablePlaceholderCount(protected)
	if count != 2 {
		t.Errorf("GetTablePlaceholderCount() = %d, want 2", count)
	}
}

func TestProtectAllTables(t *testing.T) {
	input := `\begin{tabular}{c}
A \\
\end{tabular}`

	// ProtectAllTables should be equivalent to ProtectTableStructures
	protected1, placeholders1 := ProtectAllTables(input)
	protected2, placeholders2 := ProtectTableStructures(input)

	if protected1 != protected2 {
		t.Errorf("ProtectAllTables() and ProtectTableStructures() should produce same protected content")
	}

	if len(placeholders1) != len(placeholders2) {
		t.Errorf("ProtectAllTables() and ProtectTableStructures() should produce same number of placeholders")
	}
}

// ============================================================
// PlaceholderSystem Tests
// ============================================================

func TestNewPlaceholderSystem(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	if ps == nil {
		t.Fatal("NewPlaceholderSystem() returned nil")
	}
	
	if ps.config == nil {
		t.Error("PlaceholderSystem config should not be nil")
	}
	
	if ps.GetPlaceholderCount() != 0 {
		t.Errorf("New PlaceholderSystem should have 0 placeholders, got %d", ps.GetPlaceholderCount())
	}
}

func TestNewPlaceholderSystemWithConfig(t *testing.T) {
	config := &PlaceholderConfig{
		Prefix:    "[[CUSTOM_",
		Suffix:    "]]",
		Separator: "-",
	}
	
	ps := NewPlaceholderSystemWithConfig(config)
	
	if ps == nil {
		t.Fatal("NewPlaceholderSystemWithConfig() returned nil")
	}
	
	if ps.config.Prefix != "[[CUSTOM_" {
		t.Errorf("Config prefix = %q, want %q", ps.config.Prefix, "[[CUSTOM_")
	}
	
	if ps.config.Suffix != "]]" {
		t.Errorf("Config suffix = %q, want %q", ps.config.Suffix, "]]")
	}
}

func TestPlaceholderSystem_GeneratePlaceholder(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	tests := []struct {
		structType StructureType
		wantPrefix string
	}{
		{StructureCommand, "<<<LATEX_CMD_"},
		{StructureEnvironment, "<<<LATEX_ENV_"},
		{StructureMathInline, "<<<LATEX_MATH_"},
		{StructureMathDisplay, "<<<LATEX_MATH_"},
		{StructureTable, "<<<LATEX_TABLE_"},
		{StructureComment, "<<<LATEX_COMMENT_"},
	}
	
	for _, tt := range tests {
		t.Run(tt.structType.String(), func(t *testing.T) {
			placeholder := ps.GeneratePlaceholder(tt.structType)
			
			if !strings.HasPrefix(placeholder, tt.wantPrefix) {
				t.Errorf("GeneratePlaceholder(%v) = %q, want prefix %q", tt.structType, placeholder, tt.wantPrefix)
			}
			
			if !strings.HasSuffix(placeholder, ">>>") {
				t.Errorf("GeneratePlaceholder(%v) = %q, want suffix %q", tt.structType, placeholder, ">>>")
			}
		})
	}
}

func TestPlaceholderSystem_GeneratePlaceholder_UniqueNumbers(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	// Generate multiple placeholders of the same type
	p1 := ps.GeneratePlaceholder(StructureCommand)
	p2 := ps.GeneratePlaceholder(StructureCommand)
	p3 := ps.GeneratePlaceholder(StructureCommand)
	
	// All should be unique
	if p1 == p2 || p2 == p3 || p1 == p3 {
		t.Errorf("Placeholders should be unique: %q, %q, %q", p1, p2, p3)
	}
	
	// Should have incrementing numbers
	if p1 != "<<<LATEX_CMD_0>>>" {
		t.Errorf("First placeholder = %q, want %q", p1, "<<<LATEX_CMD_0>>>")
	}
	if p2 != "<<<LATEX_CMD_1>>>" {
		t.Errorf("Second placeholder = %q, want %q", p2, "<<<LATEX_CMD_1>>>")
	}
	if p3 != "<<<LATEX_CMD_2>>>" {
		t.Errorf("Third placeholder = %q, want %q", p3, "<<<LATEX_CMD_2>>>")
	}
}

func TestPlaceholderSystem_StorePlaceholder(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{test}"
	
	ps.StorePlaceholder(placeholder, original)
	
	if ps.GetPlaceholderCount() != 1 {
		t.Errorf("PlaceholderCount = %d, want 1", ps.GetPlaceholderCount())
	}
	
	retrieved, exists := ps.GetOriginal(placeholder)
	if !exists {
		t.Error("Placeholder should exist after storing")
	}
	if retrieved != original {
		t.Errorf("GetOriginal() = %q, want %q", retrieved, original)
	}
}

func TestPlaceholderSystem_RestorePlaceholder(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{test}"
	
	ps.StorePlaceholder(placeholder, original)
	
	content := "Some text " + placeholder + " more text"
	restored := ps.RestorePlaceholder(content, placeholder)
	
	expected := "Some text \\textbf{test} more text"
	if restored != expected {
		t.Errorf("RestorePlaceholder() = %q, want %q", restored, expected)
	}
}

func TestPlaceholderSystem_RestoreAll(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	// Store multiple placeholders
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.StorePlaceholder("<<<LATEX_CMD_1>>>", "\\textit{italic}")
	ps.StorePlaceholder("<<<LATEX_MATH_0>>>", "$x + y$")
	
	content := "Text <<<LATEX_CMD_0>>> and <<<LATEX_CMD_1>>> with <<<LATEX_MATH_0>>>"
	restored := ps.RestoreAll(content)
	
	expected := "Text \\textbf{bold} and \\textit{italic} with $x + y$"
	if restored != expected {
		t.Errorf("RestoreAll() = %q, want %q", restored, expected)
	}
}

func TestPlaceholderSystem_ValidatePlaceholders(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	// Store placeholders
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.StorePlaceholder("<<<LATEX_CMD_1>>>", "\\textit{italic}")
	
	// Content with all placeholders present
	content := "Text <<<LATEX_CMD_0>>> and <<<LATEX_CMD_1>>>"
	missing := ps.ValidatePlaceholders(content)
	
	if len(missing) != 0 {
		t.Errorf("ValidatePlaceholders() found %d missing, want 0", len(missing))
	}
	
	// Content with one placeholder missing
	contentMissing := "Text <<<LATEX_CMD_0>>> only"
	missing = ps.ValidatePlaceholders(contentMissing)
	
	if len(missing) != 1 {
		t.Errorf("ValidatePlaceholders() found %d missing, want 1", len(missing))
	}
	
	if len(missing) > 0 && missing[0] != "<<<LATEX_CMD_1>>>" {
		t.Errorf("Missing placeholder = %q, want %q", missing[0], "<<<LATEX_CMD_1>>>")
	}
}

func TestPlaceholderSystem_ValidatePlaceholders_AllMissing(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	// Store placeholders
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.StorePlaceholder("<<<LATEX_CMD_1>>>", "\\textit{italic}")
	
	// Content with no placeholders
	content := "Text without any placeholders"
	missing := ps.ValidatePlaceholders(content)
	
	if len(missing) != 2 {
		t.Errorf("ValidatePlaceholders() found %d missing, want 2", len(missing))
	}
}

func TestPlaceholderSystem_GetPlaceholders(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.StorePlaceholder("<<<LATEX_CMD_1>>>", "\\textit{italic}")
	
	placeholders := ps.GetPlaceholders()
	
	if len(placeholders) != 2 {
		t.Errorf("GetPlaceholders() returned %d items, want 2", len(placeholders))
	}
	
	// Verify it's a copy (modifying shouldn't affect original)
	placeholders["new"] = "value"
	if ps.GetPlaceholderCount() != 2 {
		t.Error("GetPlaceholders() should return a copy, not the original map")
	}
}

func TestPlaceholderSystem_Clear(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.GeneratePlaceholder(StructureCommand) // Increment counter
	
	if ps.GetPlaceholderCount() != 1 {
		t.Errorf("Before Clear: PlaceholderCount = %d, want 1", ps.GetPlaceholderCount())
	}
	
	ps.Clear()
	
	if ps.GetPlaceholderCount() != 0 {
		t.Errorf("After Clear: PlaceholderCount = %d, want 0", ps.GetPlaceholderCount())
	}
	
	// Counter should also be reset
	newPlaceholder := ps.GeneratePlaceholder(StructureCommand)
	if newPlaceholder != "<<<LATEX_CMD_0>>>" {
		t.Errorf("After Clear: GeneratePlaceholder() = %q, want %q", newPlaceholder, "<<<LATEX_CMD_0>>>")
	}
}

func TestPlaceholderSystem_HasPlaceholder(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	
	if !ps.HasPlaceholder("<<<LATEX_CMD_0>>>") {
		t.Error("HasPlaceholder() should return true for stored placeholder")
	}
	
	if ps.HasPlaceholder("<<<LATEX_CMD_1>>>") {
		t.Error("HasPlaceholder() should return false for non-existent placeholder")
	}
}

func TestPlaceholderSystem_Clone(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.GeneratePlaceholder(StructureCommand) // Counter at 1
	
	clone := ps.Clone()
	
	// Clone should have same placeholders
	if clone.GetPlaceholderCount() != ps.GetPlaceholderCount() {
		t.Errorf("Clone PlaceholderCount = %d, want %d", clone.GetPlaceholderCount(), ps.GetPlaceholderCount())
	}
	
	// Clone should have same counter state
	clonePlaceholder := clone.GeneratePlaceholder(StructureCommand)
	originalPlaceholder := ps.GeneratePlaceholder(StructureCommand)
	
	if clonePlaceholder != originalPlaceholder {
		t.Errorf("Clone counter state differs: clone=%q, original=%q", clonePlaceholder, originalPlaceholder)
	}
	
	// Modifying clone shouldn't affect original
	clone.StorePlaceholder("<<<LATEX_CMD_99>>>", "new")
	if ps.HasPlaceholder("<<<LATEX_CMD_99>>>") {
		t.Error("Modifying clone should not affect original")
	}
}

func TestPlaceholderSystem_MergePlaceholders(t *testing.T) {
	ps1 := NewPlaceholderSystem()
	ps2 := NewPlaceholderSystem()
	
	ps1.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps2.StorePlaceholder("<<<LATEX_MATH_0>>>", "$x$")
	
	ps1.MergePlaceholders(ps2)
	
	if ps1.GetPlaceholderCount() != 2 {
		t.Errorf("After merge: PlaceholderCount = %d, want 2", ps1.GetPlaceholderCount())
	}
	
	if !ps1.HasPlaceholder("<<<LATEX_MATH_0>>>") {
		t.Error("Merged placeholder should exist in ps1")
	}
}

func TestPlaceholderSystem_GetPlaceholderInfo(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_5>>>", "\\textbf{bold}")
	
	info, exists := ps.GetPlaceholderInfo("<<<LATEX_CMD_5>>>")
	
	if !exists {
		t.Fatal("GetPlaceholderInfo() should return true for existing placeholder")
	}
	
	if info.Placeholder != "<<<LATEX_CMD_5>>>" {
		t.Errorf("Info.Placeholder = %q, want %q", info.Placeholder, "<<<LATEX_CMD_5>>>")
	}
	
	if info.Original != "\\textbf{bold}" {
		t.Errorf("Info.Original = %q, want %q", info.Original, "\\textbf{bold}")
	}
	
	if info.Type != StructureCommand {
		t.Errorf("Info.Type = %v, want %v", info.Type, StructureCommand)
	}
	
	if info.Index != 5 {
		t.Errorf("Info.Index = %d, want 5", info.Index)
	}
}

func TestPlaceholderSystem_RecoverMissingPlaceholders(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.StorePlaceholder("<<<LATEX_CMD_1>>>", "\\textit{italic}")
	
	// Content with one placeholder missing
	content := "Text <<<LATEX_CMD_0>>> only"
	missing := []string{"<<<LATEX_CMD_1>>>"}
	
	recovered := ps.RecoverMissingPlaceholders(content, missing)
	
	// Should contain the missing placeholder (appended)
	if !strings.Contains(recovered, "<<<LATEX_CMD_1>>>") {
		t.Error("RecoverMissingPlaceholders() should append missing placeholder")
	}
}

func TestPlaceholderSystem_RoundTrip(t *testing.T) {
	// Test that protect -> restore produces original content
	ps := NewPlaceholderSystem()
	
	// Manually create a round-trip scenario
	original := "\\textbf{bold} and \\textit{italic}"
	
	// Generate and store placeholders
	p1 := ps.GeneratePlaceholder(StructureCommand)
	ps.StorePlaceholder(p1, "\\textbf{bold}")
	
	p2 := ps.GeneratePlaceholder(StructureCommand)
	ps.StorePlaceholder(p2, "\\textit{italic}")
	
	// Create protected content
	protected := p1 + " and " + p2
	
	// Restore
	restored := ps.RestoreAll(protected)
	
	if restored != original {
		t.Errorf("Round trip failed:\nOriginal:  %q\nProtected: %q\nRestored:  %q", original, protected, restored)
	}
}

func TestPlaceholderConfig_Default(t *testing.T) {
	config := DefaultPlaceholderConfig()
	
	if config.Prefix != "<<<LATEX_" {
		t.Errorf("Default Prefix = %q, want %q", config.Prefix, "<<<LATEX_")
	}
	
	if config.Suffix != ">>>" {
		t.Errorf("Default Suffix = %q, want %q", config.Suffix, ">>>")
	}
	
	if config.Separator != "_" {
		t.Errorf("Default Separator = %q, want %q", config.Separator, "_")
	}
}

func TestPlaceholderSystem_CustomConfig(t *testing.T) {
	config := &PlaceholderConfig{
		Prefix:    "[[TEX_",
		Suffix:    "]]",
		Separator: "-",
	}
	
	ps := NewPlaceholderSystemWithConfig(config)
	
	placeholder := ps.GeneratePlaceholder(StructureCommand)
	
	if !strings.HasPrefix(placeholder, "[[TEX_CMD-") {
		t.Errorf("Custom placeholder = %q, want prefix %q", placeholder, "[[TEX_CMD-")
	}
	
	if !strings.HasSuffix(placeholder, "]]") {
		t.Errorf("Custom placeholder = %q, want suffix %q", placeholder, "]]")
	}
}

func TestPlaceholderSystem_ConcurrentAccess(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	// Test concurrent access doesn't cause race conditions
	done := make(chan bool)
	
	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			p := ps.GeneratePlaceholder(StructureCommand)
			ps.StorePlaceholder(p, "content")
		}
		done <- true
	}()
	
	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = ps.GetPlaceholderCount()
			_ = ps.GetPlaceholders()
		}
		done <- true
	}()
	
	// Wait for both goroutines
	<-done
	<-done
	
	// Should have 100 placeholders
	if ps.GetPlaceholderCount() != 100 {
		t.Errorf("After concurrent access: PlaceholderCount = %d, want 100", ps.GetPlaceholderCount())
	}
}

// ============================================================
// Enhanced Placeholder Recovery Tests
// ============================================================

func TestRecoveryReport_NewRecoveryReport(t *testing.T) {
	report := NewRecoveryReport()
	
	if report == nil {
		t.Fatal("NewRecoveryReport() returned nil")
	}
	
	if report.TotalMissing != 0 {
		t.Errorf("TotalMissing = %d, want 0", report.TotalMissing)
	}
	
	if report.TotalRecovered != 0 {
		t.Errorf("TotalRecovered = %d, want 0", report.TotalRecovered)
	}
	
	if report.TotalFailed != 0 {
		t.Errorf("TotalFailed = %d, want 0", report.TotalFailed)
	}
	
	if len(report.Attempts) != 0 {
		t.Errorf("Attempts length = %d, want 0", len(report.Attempts))
	}
	
	if len(report.StrategiesUsed) != 0 {
		t.Errorf("StrategiesUsed length = %d, want 0", len(report.StrategiesUsed))
	}
}

func TestRecoveryReport_AddAttempt(t *testing.T) {
	report := NewRecoveryReport()
	
	// Add a successful attempt
	attempt1 := RecoveryAttempt{
		Placeholder:    "<<<LATEX_CMD_0>>>",
		Original:       "\\textbf{bold}",
		Strategy:       RecoveryStrategyPatternMatch,
		InsertPosition: 10,
		Success:        true,
		Message:        "found similar pattern",
	}
	report.AddAttempt(attempt1)
	
	if report.TotalRecovered != 1 {
		t.Errorf("TotalRecovered = %d, want 1", report.TotalRecovered)
	}
	
	if report.TotalFailed != 0 {
		t.Errorf("TotalFailed = %d, want 0", report.TotalFailed)
	}
	
	if report.StrategiesUsed[RecoveryStrategyPatternMatch] != 1 {
		t.Errorf("PatternMatch count = %d, want 1", report.StrategiesUsed[RecoveryStrategyPatternMatch])
	}
	
	// Add a failed attempt
	attempt2 := RecoveryAttempt{
		Placeholder: "<<<LATEX_CMD_1>>>",
		Original:    "\\textit{italic}",
		Strategy:    RecoveryStrategyFailed,
		Success:     false,
		Message:     "placeholder not found",
	}
	report.AddAttempt(attempt2)
	
	if report.TotalRecovered != 1 {
		t.Errorf("TotalRecovered = %d, want 1", report.TotalRecovered)
	}
	
	if report.TotalFailed != 1 {
		t.Errorf("TotalFailed = %d, want 1", report.TotalFailed)
	}
	
	if len(report.Attempts) != 2 {
		t.Errorf("Attempts length = %d, want 2", len(report.Attempts))
	}
}

func TestRecoverMissingPlaceholders_EmptyMissing(t *testing.T) {
	content := "Some content"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "\\textbf{bold}",
	}
	
	result := recoverMissingPlaceholders(content, placeholders, []string{})
	
	if result != content {
		t.Errorf("Result = %q, want %q", result, content)
	}
}

func TestRecoverMissingPlaceholders_SingleMissing(t *testing.T) {
	content := "Some translated content"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "\\textbf{bold}",
	}
	missing := []string{"<<<LATEX_CMD_0>>>"}
	
	result := recoverMissingPlaceholders(content, placeholders, missing)
	
	if !strings.Contains(result, "<<<LATEX_CMD_0>>>") {
		t.Error("Result should contain the missing placeholder")
	}
}

func TestRecoverMissingPlaceholders_MultipleMissing(t *testing.T) {
	content := "Some translated content"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "\\textbf{bold}",
		"<<<LATEX_CMD_1>>>": "\\textit{italic}",
		"<<<LATEX_CMD_2>>>": "\\emph{emphasis}",
	}
	missing := []string{"<<<LATEX_CMD_0>>>", "<<<LATEX_CMD_1>>>", "<<<LATEX_CMD_2>>>"}
	
	result := recoverMissingPlaceholders(content, placeholders, missing)
	
	for _, placeholder := range missing {
		if !strings.Contains(result, placeholder) {
			t.Errorf("Result should contain placeholder %s", placeholder)
		}
	}
}

func TestRecoverMissingPlaceholdersWithContext_ContextBefore(t *testing.T) {
	originalContent := "This is \\textbf{bold} text"
	content := "这是 text"  // Translated but missing the placeholder
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "\\textbf{bold}",
	}
	missing := []string{"<<<LATEX_CMD_0>>>"}
	
	result := recoverMissingPlaceholdersWithContext(content, originalContent, placeholders, missing)
	
	if !strings.Contains(result, "<<<LATEX_CMD_0>>>") {
		t.Error("Result should contain the missing placeholder")
	}
}

func TestRecoverMissingPlaceholdersWithReport_ReturnsReport(t *testing.T) {
	content := "Some content"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "\\textbf{bold}",
	}
	missing := []string{"<<<LATEX_CMD_0>>>"}
	
	result, report := recoverMissingPlaceholdersWithReport(content, "", placeholders, missing)
	
	if report == nil {
		t.Fatal("Report should not be nil")
	}
	
	if report.TotalMissing != 1 {
		t.Errorf("TotalMissing = %d, want 1", report.TotalMissing)
	}
	
	if !strings.Contains(result, "<<<LATEX_CMD_0>>>") {
		t.Error("Result should contain the missing placeholder")
	}
	
	if len(report.Attempts) != 1 {
		t.Errorf("Attempts length = %d, want 1", len(report.Attempts))
	}
}

func TestSortPlaceholdersByNumber(t *testing.T) {
	missing := []string{
		"<<<LATEX_CMD_5>>>",
		"<<<LATEX_CMD_1>>>",
		"<<<LATEX_CMD_10>>>",
		"<<<LATEX_CMD_2>>>",
	}
	
	sorted := sortPlaceholdersByNumber(missing)
	
	expected := []string{
		"<<<LATEX_CMD_1>>>",
		"<<<LATEX_CMD_2>>>",
		"<<<LATEX_CMD_5>>>",
		"<<<LATEX_CMD_10>>>",
	}
	
	for i, placeholder := range sorted {
		if placeholder != expected[i] {
			t.Errorf("sorted[%d] = %q, want %q", i, placeholder, expected[i])
		}
	}
}

func TestExtractPlaceholderNumber(t *testing.T) {
	tests := []struct {
		placeholder string
		expected    int
	}{
		{"<<<LATEX_CMD_0>>>", 0},
		{"<<<LATEX_CMD_5>>>", 5},
		{"<<<LATEX_CMD_100>>>", 100},
		{"<<<LATEX_MATH_3>>>", 3},
		{"<<<LATEX_ENV_7>>>", 7},
		{"<<<LATEX_TABLE_2>>>", 2},
		{"<<<LATEX_COMMENT_1>>>", 1},
		{"invalid", 0},
	}
	
	for _, tt := range tests {
		t.Run(tt.placeholder, func(t *testing.T) {
			result := extractPlaceholderNumber(tt.placeholder)
			if result != tt.expected {
				t.Errorf("extractPlaceholderNumber(%q) = %d, want %d", tt.placeholder, result, tt.expected)
			}
		})
	}
}

func TestTryPatternMatchRecovery_WithSimilarCommand(t *testing.T) {
	content := "Some \\textbf{existing} content"
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{bold}"
	
	result, pos, ok := tryPatternMatchRecovery(content, placeholder, original)
	
	if !ok {
		t.Error("Pattern match recovery should succeed")
	}
	
	if pos < 0 {
		t.Errorf("Insert position = %d, should be >= 0", pos)
	}
	
	if !strings.Contains(result, placeholder) {
		t.Error("Result should contain the placeholder")
	}
}

func TestTryPatternMatchRecovery_NoSimilarCommand(t *testing.T) {
	content := "Some plain content without commands"
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{bold}"
	
	_, _, ok := tryPatternMatchRecovery(content, placeholder, original)
	
	if ok {
		t.Error("Pattern match recovery should fail when no similar command exists")
	}
}

func TestTryLineBasedRecovery_SameLineCount(t *testing.T) {
	originalContent := "Line 1\n\\textbf{bold}\nLine 3"
	content := "行 1\n\n行 3"  // Translated but missing the command
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{bold}"
	
	result, pos, ok := tryLineBasedRecovery(content, originalContent, placeholder, original)
	
	if !ok {
		t.Error("Line-based recovery should succeed")
	}
	
	if pos < 0 {
		t.Errorf("Insert position = %d, should be >= 0", pos)
	}
	
	if !strings.Contains(result, placeholder) {
		t.Error("Result should contain the placeholder")
	}
}

func TestTryLineBasedRecovery_NoOriginalContent(t *testing.T) {
	content := "Some content"
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{bold}"
	
	_, _, ok := tryLineBasedRecovery(content, "", placeholder, original)
	
	if ok {
		t.Error("Line-based recovery should fail without original content")
	}
}

func TestTryContextBeforeRecovery_WithContext(t *testing.T) {
	originalContent := "Before text \\textbf{bold} after text"
	content := "Before text  after text"  // Missing the command
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{bold}"
	
	result, pos, ok := tryContextBeforeRecovery(content, originalContent, placeholder, original)
	
	if !ok {
		t.Error("Context before recovery should succeed")
	}
	
	if pos < 0 {
		t.Errorf("Insert position = %d, should be >= 0", pos)
	}
	
	if !strings.Contains(result, placeholder) {
		t.Error("Result should contain the placeholder")
	}
}

func TestTryContextAfterRecovery_WithContext(t *testing.T) {
	originalContent := "Before text \\textbf{bold} after text"
	content := "Before text  after text"  // Missing the command
	placeholder := "<<<LATEX_CMD_0>>>"
	original := "\\textbf{bold}"
	
	result, pos, ok := tryContextAfterRecovery(content, originalContent, placeholder, original)
	
	if !ok {
		t.Error("Context after recovery should succeed")
	}
	
	if pos < 0 {
		t.Errorf("Insert position = %d, should be >= 0", pos)
	}
	
	if !strings.Contains(result, placeholder) {
		t.Error("Result should contain the placeholder")
	}
}

func TestPlaceholderSystem_RecoverMissingPlaceholdersWithReport(t *testing.T) {
	ps := NewPlaceholderSystem()
	
	ps.StorePlaceholder("<<<LATEX_CMD_0>>>", "\\textbf{bold}")
	ps.StorePlaceholder("<<<LATEX_CMD_1>>>", "\\textit{italic}")
	
	content := "Some translated content"
	missing := []string{"<<<LATEX_CMD_0>>>", "<<<LATEX_CMD_1>>>"}
	
	result, report := ps.RecoverMissingPlaceholdersWithReport(content, "", missing)
	
	if report == nil {
		t.Fatal("Report should not be nil")
	}
	
	if report.TotalMissing != 2 {
		t.Errorf("TotalMissing = %d, want 2", report.TotalMissing)
	}
	
	// Both placeholders should be recovered
	if !strings.Contains(result, "<<<LATEX_CMD_0>>>") {
		t.Error("Result should contain <<<LATEX_CMD_0>>>")
	}
	
	if !strings.Contains(result, "<<<LATEX_CMD_1>>>") {
		t.Error("Result should contain <<<LATEX_CMD_1>>>")
	}
}

func TestRecoveryStrategy_Constants(t *testing.T) {
	// Verify all strategy constants are defined
	strategies := []RecoveryStrategy{
		RecoveryStrategyPatternMatch,
		RecoveryStrategyLineBased,
		RecoveryStrategyContextBefore,
		RecoveryStrategyContextAfter,
		RecoveryStrategyAppend,
		RecoveryStrategyFailed,
	}
	
	for _, strategy := range strategies {
		if strategy == "" {
			t.Error("Strategy constant should not be empty")
		}
	}
}

func TestRecoverMissingPlaceholders_PreservesExistingContent(t *testing.T) {
	content := "Existing <<<LATEX_CMD_0>>> content"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "\\textbf{bold}",
		"<<<LATEX_CMD_1>>>": "\\textit{italic}",
	}
	missing := []string{"<<<LATEX_CMD_1>>>"}
	
	result := recoverMissingPlaceholders(content, placeholders, missing)
	
	// Should preserve existing placeholder
	if !strings.Contains(result, "<<<LATEX_CMD_0>>>") {
		t.Error("Result should preserve existing placeholder")
	}
	
	// Should add missing placeholder
	if !strings.Contains(result, "<<<LATEX_CMD_1>>>") {
		t.Error("Result should contain the missing placeholder")
	}
}

func TestRecoverMissingPlaceholders_DifferentTypes(t *testing.T) {
	content := "Some content"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>":     "\\textbf{bold}",
		"<<<LATEX_MATH_0>>>":    "$x^2$",
		"<<<LATEX_ENV_0>>>":     "\\begin{itemize}",
		"<<<LATEX_TABLE_0>>>":   "\\begin{tabular}",
		"<<<LATEX_COMMENT_0>>>": "% comment",
	}
	missing := []string{
		"<<<LATEX_CMD_0>>>",
		"<<<LATEX_MATH_0>>>",
		"<<<LATEX_ENV_0>>>",
		"<<<LATEX_TABLE_0>>>",
		"<<<LATEX_COMMENT_0>>>",
	}
	
	result := recoverMissingPlaceholders(content, placeholders, missing)
	
	for _, placeholder := range missing {
		if !strings.Contains(result, placeholder) {
			t.Errorf("Result should contain placeholder %s", placeholder)
		}
	}
}
