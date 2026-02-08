package translator

import (
	"testing"
)

func TestFixTabularStructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "fix begin tabular with column spec on new line",
			input: `\begin{tabular}
{lcccc}`,
			expected: `\begin{tabular}{lcccc}`,
		},
		{
			name: "fix begin tabular with position and column spec on new line",
			input: `\begin{tabular}[c]
{@{}c@{}}`,
			expected: `\begin{tabular}[c]{@{}c@{}}`,
		},
		{
			name: "fix multirow with nested tabular on new line",
			input: `\multirow{4}{*}{
\begin{tabular}[l]{@{}l@{}}AIR-Bench`,
			expected: `\multirow{4}{*}{\begin{tabular}[l]{@{}l@{}}AIR-Bench`,
		},
		{
			name: "fix end tabular with extra closing brace",
			input: `\end{tabular}}
}

& next cell`,
			expected: `\end{tabular}} & next cell`,
		},
		{
			name: "preserve correct tabular structure",
			input:    `\begin{tabular}{lcc}\toprule\end{tabular}`,
			expected: `\begin{tabular}{lcc}\toprule\end{tabular}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixTabularStructure(tt.input)
			if result != tt.expected {
				t.Errorf("fixTabularStructure() =\n%q\nwant:\n%q", result, tt.expected)
			}
		})
	}
}
