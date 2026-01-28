package translator

import (
	"strings"
	"testing"
)

func TestApplyReferenceBasedFixes_TableResizebox(t *testing.T) {
	original := `\begin{table}[h]
\resizebox{\columnwidth}{!}{
\begin{tabular}{c|c}
A & B \\
\end{tabular}}
\end{table}`

	translated := `\begin{table}[h]
\resizebox{\columnwidth}{!}{
\begin{tabular}{c|c}
甲 & 乙 \\
\end{tabular}
\end{table}`

	result := ApplyReferenceBasedFixes(translated, original)

	if !strings.Contains(result, "\\end{tabular}}") {
		t.Error("Expected resizebox closing brace to be added")
		t.Logf("Result:\n%s", result)
	}
}

func TestApplyReferenceBasedFixes_CaptionBraces(t *testing.T) {
	original := `\caption{Test with \textbf{\textit{n}}=64}`
	translated := `\caption{测试 \textbf{\textit{n}=64}`

	result := ApplyReferenceBasedFixes(translated, original)

	// The fix should add the missing closing brace
	if strings.Contains(result, "\\textit{n}=") {
		t.Error("Expected caption brace issue to be fixed")
		t.Logf("Result: %s", result)
	}
}

func TestApplyReferenceBasedFixes_TrailingBraces(t *testing.T) {
	original := `\section{Test}
Content
\end{document}`

	translated := `\section{测试}
内容
\end{document}}}}}}`

	result := ApplyReferenceBasedFixes(translated, original)

	if strings.HasSuffix(strings.TrimSpace(result), "}}}}}") {
		t.Error("Expected trailing braces to be removed")
		t.Logf("Result:\n%s", result)
	}
}

func TestApplyReferenceBasedFixes_MissingEndEnvironment(t *testing.T) {
	original := `\begin{figure*}[h]
\includegraphics{test.png}
\caption{Test}
\end{figure*}
\end{document}`

	translated := `\begin{figure*}[h]
\includegraphics{test.png}
\caption{测试}
\end{document}`
	// Missing \end{figure*}

	result := ApplyReferenceBasedFixes(translated, original)

	if !strings.Contains(result, "\\end{figure*}") {
		t.Error("Expected \\end{figure*} to be added")
		t.Logf("Result:\n%s", result)
	}
}

func TestApplyReferenceBasedFixes_NoOriginal(t *testing.T) {
	translated := `\section{Test}
Content`

	result := ApplyReferenceBasedFixes(translated, "")

	if result != translated {
		t.Error("Expected no changes when original is empty")
	}
}

func TestFixTableResizeboxBraces(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		translated string
		wantFixed  bool
		wantResult string
	}{
		{
			name: "fix missing closing brace",
			original: `\begin{table}
\resizebox{\columnwidth}{!}{
\begin{tabular}{c}
A
\end{tabular}}
\end{table}`,
			translated: `\begin{table}
\resizebox{\columnwidth}{!}{
\begin{tabular}{c}
甲
\end{tabular}
\end{table}`,
			wantFixed:  true,
			wantResult: "\\end{tabular}}",
		},
		{
			name: "no fix needed",
			original: `\begin{table}
\resizebox{\columnwidth}{!}{
\begin{tabular}{c}
A
\end{tabular}}
\end{table}`,
			translated: `\begin{table}
\resizebox{\columnwidth}{!}{
\begin{tabular}{c}
甲
\end{tabular}}
\end{table}`,
			wantFixed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := fixTableResizeboxBraces(tt.translated, tt.original)

			if fixed != tt.wantFixed {
				t.Errorf("fixTableResizeboxBraces() fixed = %v, want %v", fixed, tt.wantFixed)
			}

			if tt.wantFixed && !strings.Contains(result, tt.wantResult) {
				t.Errorf("fixTableResizeboxBraces() result doesn't contain %q", tt.wantResult)
				t.Logf("Result:\n%s", result)
			}
		})
	}
}

func TestCountTrailingBraces(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"content}", 1},
		{"content}}", 2},
		{"content}}}}}", 5},
		{"content} }", 2},
		{"content}\n}", 2},
		{"content", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := countTrailingBraces(tt.input)
			if got != tt.want {
				t.Errorf("countTrailingBraces(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
