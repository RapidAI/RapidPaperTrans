// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"testing"
)

func TestValidateBraces(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantBalanced   bool
		wantOpenCount  int
		wantCloseCount int
		wantDifference int
	}{
		{
			name:           "balanced simple",
			content:        `\textbf{hello}`,
			wantBalanced:   true,
			wantOpenCount:  1,
			wantCloseCount: 1,
			wantDifference: 0,
		},
		{
			name:           "balanced nested",
			content:        `\textbf{\textit{hello}}`,
			wantBalanced:   true,
			wantOpenCount:  2,
			wantCloseCount: 2,
			wantDifference: 0,
		},
		{
			name:           "balanced complex",
			content:        `\multirow{3}{*}{\textbf{Method}}`,
			wantBalanced:   true,
			wantOpenCount:  4,
			wantCloseCount: 4,
			wantDifference: 0,
		},
		{
			name:           "missing closing brace",
			content:        `\textbf{hello`,
			wantBalanced:   false,
			wantOpenCount:  1,
			wantCloseCount: 0,
			wantDifference: 1,
		},
		{
			name:           "extra closing brace",
			content:        `\textbf{hello}}`,
			wantBalanced:   false,
			wantOpenCount:  1,
			wantCloseCount: 2,
			wantDifference: -1,
		},
		{
			name:           "multiple missing closing braces",
			content:        `\textbf{\textit{hello}`,
			wantBalanced:   false,
			wantOpenCount:  2,
			wantCloseCount: 1,
			wantDifference: 1,
		},
		{
			name:           "escaped braces should be ignored",
			content:        `\textbf{hello \{ world \}}`,
			wantBalanced:   true,
			wantOpenCount:  1,
			wantCloseCount: 1,
			wantDifference: 0,
		},
		{
			name:           "braces in comments should be ignored",
			content:        "% {{{{\n\\textbf{hello}",
			wantBalanced:   true,
			wantOpenCount:  1,
			wantCloseCount: 1,
			wantDifference: 0,
		},
		{
			name:           "empty content",
			content:        "",
			wantBalanced:   true,
			wantOpenCount:  0,
			wantCloseCount: 0,
			wantDifference: 0,
		},
		{
			name:           "no braces",
			content:        "hello world",
			wantBalanced:   true,
			wantOpenCount:  0,
			wantCloseCount: 0,
			wantDifference: 0,
		},
		{
			name: "multiline with nested environments",
			content: `\begin{document}
\section{Test}
\textbf{bold \textit{italic}}
\end{document}`,
			wantBalanced:   true,
			wantOpenCount:  5, // document, section, textbf, textit, document
			wantCloseCount: 5,
			wantDifference: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBraces(tt.content)

			if result.IsBalanced != tt.wantBalanced {
				t.Errorf("IsBalanced = %v, want %v", result.IsBalanced, tt.wantBalanced)
			}
			if result.OpenCount != tt.wantOpenCount {
				t.Errorf("OpenCount = %v, want %v", result.OpenCount, tt.wantOpenCount)
			}
			if result.CloseCount != tt.wantCloseCount {
				t.Errorf("CloseCount = %v, want %v", result.CloseCount, tt.wantCloseCount)
			}
			if result.Difference != tt.wantDifference {
				t.Errorf("Difference = %v, want %v", result.Difference, tt.wantDifference)
			}
		})
	}
}

func TestValidateBracesErrorLocations(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		wantErrorLines    []int
		wantMaxNestDepth  int
	}{
		{
			name:             "unclosed brace on line 1",
			content:          `\textbf{hello`,
			wantErrorLines:   []int{1},
			wantMaxNestDepth: 1,
		},
		{
			name:             "extra closing brace on line 1",
			content:          `\textbf{hello}}`,
			wantErrorLines:   []int{1},
			wantMaxNestDepth: 1,
		},
		{
			name: "unclosed brace on line 2",
			content: `\section{Test}
\textbf{hello`,
			wantErrorLines:   []int{2},
			wantMaxNestDepth: 1,
		},
		{
			name: "multiple unclosed braces",
			content: `\textbf{
\textit{hello`,
			wantErrorLines:   []int{1, 2},
			wantMaxNestDepth: 2,
		},
		{
			name:             "deeply nested balanced",
			content:          `\a{\b{\c{\d{x}}}}`,
			wantErrorLines:   []int{},
			wantMaxNestDepth: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBraces(tt.content)

			if len(result.ErrorLines) != len(tt.wantErrorLines) {
				t.Errorf("ErrorLines count = %v, want %v", len(result.ErrorLines), len(tt.wantErrorLines))
			}

			for i, line := range tt.wantErrorLines {
				if i < len(result.ErrorLines) && result.ErrorLines[i] != line {
					t.Errorf("ErrorLines[%d] = %v, want %v", i, result.ErrorLines[i], line)
				}
			}

			if result.MaxNestDepth != tt.wantMaxNestDepth {
				t.Errorf("MaxNestDepth = %v, want %v", result.MaxNestDepth, tt.wantMaxNestDepth)
			}
		})
	}
}

func TestFormatBraceValidation(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantPass bool
	}{
		{
			name:     "balanced content",
			content:  `\textbf{hello}`,
			wantPass: true,
		},
		{
			name:     "unbalanced content",
			content:  `\textbf{hello`,
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBraces(tt.content)
			formatted := FormatBraceValidation(result)

			if tt.wantPass {
				if !contains(formatted, "验证通过") {
					t.Errorf("Expected '验证通过' in output, got: %s", formatted)
				}
			} else {
				if !contains(formatted, "验证失败") {
					t.Errorf("Expected '验证失败' in output, got: %s", formatted)
				}
			}
		})
	}
}
