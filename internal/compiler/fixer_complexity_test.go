package compiler

import (
	"testing"
)

func TestAnalyzeErrorComplexity(t *testing.T) {
	tests := []struct {
		name           string
		errors         []LaTeXError
		compileLog     string
		wantComplexity string
	}{
		{
			name:           "no errors",
			errors:         []LaTeXError{},
			compileLog:     "No errors found",
			wantComplexity: "low",
		},
		{
			name: "single simple error",
			errors: []LaTeXError{
				{File: "main.tex", Line: 10, Message: "Undefined control sequence"},
			},
			compileLog:     "! Undefined control sequence.\nl.10 \\unknowncommand",
			wantComplexity: "medium", // Undefined control sequence is medium complexity
		},
		{
			name: "emergency stop - high complexity",
			errors: []LaTeXError{
				{File: "main.tex", Line: 100, Message: "Extra }"},
			},
			compileLog:     "! Extra }\n! Emergency stop.\n*** (job aborted, no legal \\end found)",
			wantComplexity: "high",
		},
		{
			name: "ended by end document - high complexity",
			errors: []LaTeXError{
				{File: "appendix.tex", Line: 518, Message: "begin{figure*} ended by end{document}"},
			},
			compileLog:     "! LaTeX Error: \\begin{figure*} on input line 518 ended by \\end{document}.",
			wantComplexity: "high",
		},
		{
			name: "mismatched environment - medium complexity",
			errors: []LaTeXError{
				{File: "main.tex", Line: 50, Message: "Extra }"},
			},
			compileLog:     "! Extra }, or forgotten \\endgroup.\nl.50 \\end{figure*}}}",
			wantComplexity: "medium",
		},
		{
			name: "lonely item - medium complexity",
			errors: []LaTeXError{
				{File: "main.tex", Line: 30, Message: "Lonely \\item"},
			},
			compileLog:     "! LaTeX Error: Lonely \\item--perhaps a missing list environment.",
			wantComplexity: "medium",
		},
		{
			name: "many errors - high complexity",
			errors: []LaTeXError{
				{File: "main.tex", Line: 10, Message: "Error 1"},
				{File: "main.tex", Line: 20, Message: "Error 2"},
				{File: "main.tex", Line: 30, Message: "Error 3"},
				{File: "main.tex", Line: 40, Message: "Error 4"},
				{File: "main.tex", Line: 50, Message: "Error 5"},
				{File: "main.tex", Line: 60, Message: "Error 6"},
			},
			compileLog:     "Multiple errors detected",
			wantComplexity: "high",
		},
		{
			name: "moderate errors - medium complexity",
			errors: []LaTeXError{
				{File: "main.tex", Line: 10, Message: "Error 1"},
				{File: "main.tex", Line: 20, Message: "Error 2"},
				{File: "main.tex", Line: 30, Message: "Error 3"},
			},
			compileLog:     "Some errors detected",
			wantComplexity: "medium",
		},
		{
			name: "package error - medium complexity",
			errors: []LaTeXError{
				{File: "main.tex", Line: 5, Message: "Package error"},
			},
			compileLog:     "! Package xcolor Error: Undefined color `mycolor'.\nSee the xcolor package documentation",
			wantComplexity: "medium",
		},
		{
			name: "arxiv 2501.17161 case - high complexity",
			errors: []LaTeXError{
				{File: "Tex/appendix.tex", Line: 589, Message: "Extra }"},
			},
			compileLog: `! Extra }, or forgotten \endgroup.
l.589 \end{figure*}}}
                     }
! LaTeX Error: \begin{figure*} on input line 518 ended by \end{document}.
! Emergency stop.
<*> translated_main.tex
*** (job aborted, no legal \end found)`,
			wantComplexity: "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzeErrorComplexity(tt.errors, tt.compileLog)
			if got != tt.wantComplexity {
				t.Errorf("analyzeErrorComplexity() = %v, want %v", got, tt.wantComplexity)
			}
		})
	}
}

func TestAnalyzeErrorComplexity_CaseInsensitive(t *testing.T) {
	// Test that pattern matching is case-insensitive
	errors := []LaTeXError{
		{File: "main.tex", Line: 10, Message: "Error"},
	}
	
	compileLog := "! EMERGENCY STOP"
	complexity := analyzeErrorComplexity(errors, compileLog)
	
	if complexity != "high" {
		t.Errorf("Expected high complexity for uppercase EMERGENCY STOP, got %v", complexity)
	}
}
