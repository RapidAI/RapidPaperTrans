package compiler

import (
	"strings"
	"testing"
)

func TestQuickFix_ExtraClosingBraces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantFix  bool
	}{
		{
			name: "extra braces after figure*",
			input: `\begin{figure*}[ht]
\includegraphics{test.png}
\caption{Test figure}
\end{figure*}}}}`,
			expected: `\begin{figure*}[ht]
\includegraphics{test.png}
\caption{Test figure}
\end{figure*}`,
			wantFix: true,
		},
		{
			name: "extra braces after table",
			input: `\begin{table}
\caption{Test table}
\end{table}}}`,
			expected: `\begin{table}
\caption{Test table}
\end{table}`,
			wantFix: true,
		},
		{
			name: "extra braces after tikzpicture",
			input: `\begin{tikzpicture}
\node {test};
\end{tikzpicture}}`,
			expected: `\begin{tikzpicture}
\node {test};
\end{tikzpicture}`,
			wantFix: true,
		},
		{
			name: "no extra braces",
			input: `\begin{figure}
\includegraphics{test.png}
\end{figure}`,
			expected: `\begin{figure}
\includegraphics{test.png}
\end{figure}`,
			wantFix: false,
		},
		{
			name: "multiple environments with extra braces",
			input: `\begin{figure*}
\caption{Fig 1}
\end{figure*}}}

\begin{table}
\caption{Table 1}
\end{table}}`,
			expected: `\begin{figure*}
\caption{Fig 1}
\end{figure*}

\begin{table}
\caption{Table 1}
\end{table}`,
			wantFix: true,
		},
		{
			name: "arxiv 2501.17161 case",
			input: `\begin{tikzpicture}
\node {test};
\end{tikzpicture}
\caption{\textbf{Example}}
\label{fig:failed_example_rl_with_too_many_sft}
\end{figure*}}}}`,
			expected: `\begin{tikzpicture}
\node {test};
\end{tikzpicture}
\caption{\textbf{Example}}
\label{fig:failed_example_rl_with_too_many_sft}
\end{figure*}`,
			wantFix: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := QuickFix(tt.input)
			
			if fixed != tt.wantFix {
				t.Errorf("QuickFix() fixed = %v, want %v", fixed, tt.wantFix)
			}
			
			// Normalize whitespace for comparison
			resultNorm := strings.TrimSpace(result)
			expectedNorm := strings.TrimSpace(tt.expected)
			
			if resultNorm != expectedNorm {
				t.Errorf("QuickFix() result mismatch\nGot:\n%s\n\nWant:\n%s", result, tt.expected)
			}
		})
	}
}

func TestQuickFix_ExtraBracesWithContext(t *testing.T) {
	// Test with more realistic document context
	input := `\documentclass{article}
\begin{document}

\section{Introduction}
Some text here.

\begin{figure*}[ht]
    \centering
    \begin{tikzpicture}
    \node[draw] (box) {Content};
    \end{tikzpicture}
    \caption{\textbf{Test figure with tikzpicture}}
    \label{fig:test}
\end{figure*}}}}

More text after the figure.

\end{document}`

	expected := `\documentclass{article}
\begin{document}

\section{Introduction}
Some text here.

\begin{figure*}[ht]
    \centering
    \begin{tikzpicture}
    \node[draw] (box) {Content};
    \end{tikzpicture}
    \caption{\textbf{Test figure with tikzpicture}}
    \label{fig:test}
\end{figure*}

More text after the figure.

\end{document}`

	result, fixed := QuickFix(input)
	
	if !fixed {
		t.Error("QuickFix() should have detected and fixed extra braces")
	}
	
	if strings.TrimSpace(result) != strings.TrimSpace(expected) {
		t.Errorf("QuickFix() result mismatch\nGot:\n%s\n\nWant:\n%s", result, expected)
	}
}
