package compiler

import (
	"strings"
	"testing"
)

func TestQuickFix_TrailingGarbageBraces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantFix  bool
	}{
		{
			name: "trailing braces after balance",
			input: `\section{AI Assistant}
我们仅使用人工智能助手（例如，GPT-3.5-Turbo）来润色和完善文章。

\balance
}}}}}}}`,
			expected: `\section{AI Assistant}
我们仅使用人工智能助手（例如，GPT-3.5-Turbo）来润色和完善文章。

\balance
`,
			wantFix: true,
		},
		{
			name: "trailing braces at end of file",
			input: `Some content here
}}}}}`,
			expected: `Some content here
`,
			wantFix: true,
		},
		{
			name: "end document with extra braces",
			input: `\begin{document}
Content
\end{document}}}`,
			expected: `\begin{document}
Content
\end{document}
`,
			wantFix: true,
		},
		{
			name: "no trailing braces - should not fix",
			input: `\section{Test}
Content here
\end{document}`,
			expected: `\section{Test}
Content here
\end{document}`,
			wantFix: false,
		},
		{
			name: "single trailing brace - handled by brace balancing rule",
			input: `Content
}`,
			expected: `Content
`,
			wantFix: true, // Rule 10 handles this
		},
		{
			name: "arxiv 2412.12094 appendix case",
			input: `\section{AI Assistant}
我们仅使用人工智能助手（例如，GPT-3.5-Turbo）来润色和完善文章。

\balance
}}}}}}}`,
			expected: `\section{AI Assistant}
我们仅使用人工智能助手（例如，GPT-3.5-Turbo）来润色和完善文章。

\balance
`,
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

func TestFixTrailingGarbageBraces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantFix  bool
	}{
		{
			name:     "multiple trailing braces",
			input:    "content\n}}}}}",
			expected: "content\n",
			wantFix:  true,
		},
		{
			name:     "braces after balance",
			input:    "\\balance\n}}}}}}}",
			expected: "\\balance\n",
			wantFix:  true,
		},
		{
			name:     "braces after end env",
			input:    "\\end{figure*}}}}",
			expected: "\\end{figure*}\n",
			wantFix:  true,
		},
		{
			name:     "no trailing braces",
			input:    "content\n\\end{document}",
			expected: "content\n\\end{document}",
			wantFix:  false,
		},
		{
			name:     "only two braces - not enough to trigger",
			input:    "content\n}}",
			expected: "content\n}}",
			wantFix:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := fixTrailingGarbageBraces(tt.input)

			if fixed != tt.wantFix {
				t.Errorf("fixTrailingGarbageBraces() fixed = %v, want %v", fixed, tt.wantFix)
			}

			if strings.TrimSpace(result) != strings.TrimSpace(tt.expected) {
				t.Errorf("fixTrailingGarbageBraces() result mismatch\nGot:\n%q\n\nWant:\n%q", result, tt.expected)
			}
		})
	}
}
