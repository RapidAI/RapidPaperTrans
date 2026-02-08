// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"strings"
	"testing"
)

// TestRestoreComments_BasicCommentRestoration tests basic % symbol restoration.
// Validates: Requirements 5.3
func TestRestoreComments_BasicCommentRestoration(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		translated string
		want       string
	}{
		{
			name:       "restore single comment line",
			original:   "% This is a comment",
			translated: "This is a comment",
			want:       "% This is a comment",
		},
		{
			name:       "restore comment with leading space",
			original:   "    % Indented comment",
			translated: "    Indented comment",
			want:       "    % Indented comment",
		},
		{
			name:       "preserve already commented line",
			original:   "% Already commented",
			translated: "% Already commented",
			want:       "% Already commented",
		},
		{
			name:       "restore multiple comment lines",
			original:   "% Line 1\n% Line 2\n% Line 3",
			translated: "Line 1\nLine 2\nLine 3",
			want:       "% Line 1\n% Line 2\n% Line 3",
		},
		{
			name:       "mixed commented and non-commented",
			original:   "Normal line\n% Comment line\nAnother normal",
			translated: "Normal line\nComment line\nAnother normal",
			want:       "Normal line\n% Comment line\nAnother normal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestoreComments(tt.translated, tt.original)
			if got != tt.want {
				t.Errorf("RestoreComments() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRestoreComments_EnvTagRecommenting tests re-commenting of environment tags.
// Validates: Requirements 5.5
func TestRestoreComments_EnvTagRecommenting(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		translated string
		wantTag    string // The tag that should be commented
	}{
		{
			name:       "recomment begin figure",
			original:   "% \\begin{figure}",
			translated: "\\begin{figure}",
			wantTag:    "% \\begin{figure}",
		},
		{
			name:       "recomment end table",
			original:   "% \\end{table}",
			translated: "\\end{table}",
			wantTag:    "% \\end{table}",
		},
		{
			name:       "recomment begin with content",
			original:   "Some text\n% \\begin{equation}\nMore text",
			translated: "Some text\n\\begin{equation}\nMore text",
			wantTag:    "% \\begin{equation}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestoreComments(tt.translated, tt.original)
			if !strings.Contains(got, tt.wantTag) {
				t.Errorf("RestoreComments() should contain %q, got %q", tt.wantTag, got)
			}
		})
	}
}

// TestRestoreComments_CommentConsistency tests that begin/end pairs have consistent comment status.
// Validates: Requirements 5.4, 5.5
func TestRestoreComments_CommentConsistency(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		translated string
		checkBegin bool // Whether \begin should be commented
		checkEnd   bool // Whether \end should be commented
		envName    string
	}{
		{
			name:       "both should be commented",
			original:   "% \\begin{figure}\nContent\n% \\end{figure}",
			translated: "% \\begin{figure}\nContent\n\\end{figure}",
			checkBegin: true,
			checkEnd:   true,
			envName:    "figure",
		},
		{
			name:       "begin commented end not - should comment end",
			original:   "% \\begin{table}\nContent\n% \\end{table}",
			translated: "% \\begin{table}\nContent\n\\end{table}",
			checkBegin: true,
			checkEnd:   true,
			envName:    "table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestoreComments(tt.translated, tt.original)

			beginTag := `\begin{` + tt.envName + `}`
			endTag := `\end{` + tt.envName + `}`

			beginCommented := isTagCommented(got, beginTag)
			endCommented := isTagCommented(got, endTag)

			if tt.checkBegin && !beginCommented {
				t.Errorf("\\begin{%s} should be commented in result: %q", tt.envName, got)
			}
			if tt.checkEnd && !endCommented {
				t.Errorf("\\end{%s} should be commented in result: %q", tt.envName, got)
			}
		})
	}
}

// TestRestoreComments_PreserveTranslatedContent tests that translated content is preserved.
// Validates: Requirements 5.2
func TestRestoreComments_PreserveTranslatedContent(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		translated string
		wantText   string // Text that should be preserved
	}{
		{
			name:       "preserve translated comment text",
			original:   "% This is English",
			translated: "这是中文",
			wantText:   "这是中文",
		},
		{
			name:       "preserve translated text with comment marker",
			original:   "% English comment",
			translated: "% 中文注释",
			wantText:   "中文注释",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestoreComments(tt.translated, tt.original)
			if !strings.Contains(got, tt.wantText) {
				t.Errorf("RestoreComments() should preserve %q, got %q", tt.wantText, got)
			}
		})
	}
}

// TestRestoreComments_EmptyInput tests handling of empty inputs.
func TestRestoreComments_EmptyInput(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		translated string
		want       string
	}{
		{
			name:       "empty translated",
			original:   "% Comment",
			translated: "",
			want:       "",
		},
		{
			name:       "empty original",
			original:   "",
			translated: "Some text",
			want:       "Some text",
		},
		{
			name:       "both empty",
			original:   "",
			translated: "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestoreComments(tt.translated, tt.original)
			if got != tt.want {
				t.Errorf("RestoreComments() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRestoreComments_LineMismatch tests behavior when line counts differ significantly.
func TestRestoreComments_LineMismatch(t *testing.T) {
	// When line counts differ by more than 20%, the function should not attempt
	// line-by-line restoration to avoid incorrect fixes
	original := "% Line 1\n% Line 2\n% Line 3\n% Line 4\n% Line 5"
	translated := "Line 1" // Only 1 line vs 5 lines

	got := RestoreComments(translated, original)

	// Should return translated unchanged since line counts differ too much
	if got != translated {
		t.Errorf("RestoreComments() should return translated unchanged when line counts differ significantly, got %q", got)
	}
}
