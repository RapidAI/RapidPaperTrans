// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"strings"
	"testing"
)

// TestValidateComments_BasicCommentDetection tests basic comment line detection
// Validates: Requirement 5.1 - WHEN a line starts with `%`, THE Format_Protector SHALL mark it as a comment line
func TestValidateComments_BasicCommentDetection(t *testing.T) {
	original := `\documentclass{article}
% This is a comment
\begin{document}
% Another comment
Hello world
\end{document}`

	translated := `\documentclass{article}
% 这是一个注释
\begin{document}
% 另一个注释
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if result.TotalOriginalComments != 2 {
		t.Errorf("Expected 2 original comments, got %d", result.TotalOriginalComments)
	}

	if result.TotalTranslatedComments != 2 {
		t.Errorf("Expected 2 translated comments, got %d", result.TotalTranslatedComments)
	}

	if !result.IsValid {
		t.Errorf("Expected validation to pass, but it failed")
	}
}

// TestValidateComments_RemovedCommentSymbol tests detection of removed % symbol
// Validates: Requirement 5.1 - comment lines should be preserved
func TestValidateComments_RemovedCommentSymbol(t *testing.T) {
	original := `\documentclass{article}
% This is a comment
\begin{document}
Hello world
\end{document}`

	// The comment symbol was removed in translation
	translated := `\documentclass{article}
This is a comment
\begin{document}
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if result.IsValid {
		t.Errorf("Expected validation to fail when comment symbol is removed")
	}

	if len(result.RemovedCommentLines) == 0 {
		t.Errorf("Expected to detect removed comment line")
	}

	if len(result.RemovedCommentLines) > 0 {
		if result.RemovedCommentLines[0].OriginalLine != 2 {
			t.Errorf("Expected removed comment at line 2, got line %d", result.RemovedCommentLines[0].OriginalLine)
		}
	}
}

// TestValidateComments_CommentedEnvTagPreserved tests that commented env tags remain commented
// Validates: Requirement 5.4 - WHEN a `\begin{...}` or `\end{...}` is inside a comment, 
// THE Format_Protector SHALL ensure it remains commented
func TestValidateComments_CommentedEnvTagPreserved(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
% \begin{figure}
% \end{figure}
Hello world
\end{document}`

	// Commented env tags remain commented
	translated := `\documentclass{article}
\begin{document}
% \begin{figure}
% \end{figure}
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if !result.IsValid {
		t.Errorf("Expected validation to pass when commented env tags are preserved")
	}

	if len(result.UncommentedEnvTags) > 0 {
		t.Errorf("Expected no uncommented env tags, got %d", len(result.UncommentedEnvTags))
	}
}

// TestValidateComments_UncommentedEnvTag tests detection of uncommented environment tags
// Validates: Requirement 5.4 - commented env tags should remain commented
func TestValidateComments_UncommentedEnvTag(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
% \begin{figure}
% \end{figure}
Hello world
\end{document}`

	// The commented env tags were uncommented in translation
	translated := `\documentclass{article}
\begin{document}
\begin{figure}
\end{figure}
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if result.IsValid {
		t.Errorf("Expected validation to fail when commented env tags are uncommented")
	}

	if len(result.UncommentedEnvTags) == 0 {
		t.Errorf("Expected to detect uncommented env tags")
	}

	// Should detect both \begin{figure} and \end{figure}
	if len(result.UncommentedEnvTags) < 2 {
		t.Errorf("Expected at least 2 uncommented env tags, got %d", len(result.UncommentedEnvTags))
	}
}

// TestValidateComments_IndentedComment tests detection of indented comment lines
func TestValidateComments_IndentedComment(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
    % Indented comment
	% Tab indented comment
Hello world
\end{document}`

	translated := `\documentclass{article}
\begin{document}
    % 缩进的注释
	% Tab缩进的注释
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if result.TotalOriginalComments != 2 {
		t.Errorf("Expected 2 original comments (including indented), got %d", result.TotalOriginalComments)
	}

	if !result.IsValid {
		t.Errorf("Expected validation to pass for indented comments")
	}
}

// TestValidateComments_EmptyContent tests handling of empty content
func TestValidateComments_EmptyContent(t *testing.T) {
	result := ValidateComments("", "")

	if !result.IsValid {
		t.Errorf("Expected validation to pass for empty content")
	}

	if result.TotalOriginalComments != 0 {
		t.Errorf("Expected 0 original comments for empty content, got %d", result.TotalOriginalComments)
	}
}

// TestValidateComments_NoComments tests content without any comments
func TestValidateComments_NoComments(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
Hello world
\end{document}`

	translated := `\documentclass{article}
\begin{document}
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if !result.IsValid {
		t.Errorf("Expected validation to pass for content without comments")
	}

	if result.TotalOriginalComments != 0 {
		t.Errorf("Expected 0 original comments, got %d", result.TotalOriginalComments)
	}
}

// TestValidateComments_MultipleEnvTagsInComment tests multiple env tags in a single comment line
func TestValidateComments_MultipleEnvTagsInComment(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
% \begin{figure} \end{figure}
Hello world
\end{document}`

	// Both env tags were uncommented
	translated := `\documentclass{article}
\begin{document}
\begin{figure} \end{figure}
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if result.IsValid {
		t.Errorf("Expected validation to fail when multiple env tags are uncommented")
	}

	// Should detect both tags
	if len(result.UncommentedEnvTags) < 2 {
		t.Errorf("Expected at least 2 uncommented env tags, got %d", len(result.UncommentedEnvTags))
	}
}

// TestValidateComments_PartialCommentRemoval tests when only some comments are removed
func TestValidateComments_PartialCommentRemoval(t *testing.T) {
	original := `\documentclass{article}
% Comment 1
% Comment 2
% Comment 3
\begin{document}
Hello world
\end{document}`

	// Only the second comment was removed
	translated := `\documentclass{article}
% 注释 1
Comment 2
% 注释 3
\begin{document}
你好世界
\end{document}`

	result := ValidateComments(original, translated)

	if result.IsValid {
		t.Errorf("Expected validation to fail when some comments are removed")
	}

	if result.TotalOriginalComments != 3 {
		t.Errorf("Expected 3 original comments, got %d", result.TotalOriginalComments)
	}

	if result.TotalTranslatedComments != 2 {
		t.Errorf("Expected 2 translated comments, got %d", result.TotalTranslatedComments)
	}
}

// TestFormatCommentValidation_Valid tests formatting of valid result
func TestFormatCommentValidation_Valid(t *testing.T) {
	result := &CommentValidation{
		IsValid:                 true,
		TotalOriginalComments:   5,
		TotalTranslatedComments: 5,
	}

	formatted := FormatCommentValidation(result)

	if !strings.Contains(formatted, "验证通过") {
		t.Errorf("Expected '验证通过' in formatted output, got: %s", formatted)
	}
}

// TestFormatCommentValidation_Invalid tests formatting of invalid result
func TestFormatCommentValidation_Invalid(t *testing.T) {
	result := &CommentValidation{
		IsValid:                 false,
		TotalOriginalComments:   5,
		TotalTranslatedComments: 3,
		RemovedCommentLines: []CommentLineChange{
			{
				OriginalLine:   2,
				OriginalText:   "% This is a comment",
				TranslatedLine: 2,
				TranslatedText: "This is a comment",
			},
		},
		UncommentedEnvTags: []UncommentedEnvTag{
			{
				OriginalLine:   4,
				EnvTag:         "\\begin{figure}",
				EnvName:        "figure",
				IsBegin:        true,
				TranslatedLine: 4,
			},
		},
	}

	formatted := FormatCommentValidation(result)

	if !strings.Contains(formatted, "验证失败") {
		t.Errorf("Expected '验证失败' in formatted output, got: %s", formatted)
	}

	if !strings.Contains(formatted, "被移除注释符号") {
		t.Errorf("Expected '被移除注释符号' in formatted output, got: %s", formatted)
	}

	if !strings.Contains(formatted, "被取消注释的环境标签") {
		t.Errorf("Expected '被取消注释的环境标签' in formatted output, got: %s", formatted)
	}
}

// TestExtractCommentLines tests the internal extractCommentLines function
func TestExtractCommentLines(t *testing.T) {
	content := `Line 1
% Comment on line 2
Line 3
  % Indented comment on line 4
% \begin{figure} on line 5
Line 6`

	comments := extractCommentLines(content)

	if len(comments) != 3 {
		t.Errorf("Expected 3 comments, got %d", len(comments))
	}

	// Check line numbers
	expectedLines := []int{2, 4, 5}
	for i, expected := range expectedLines {
		if i < len(comments) && comments[i].lineNumber != expected {
			t.Errorf("Expected comment %d at line %d, got line %d", i, expected, comments[i].lineNumber)
		}
	}

	// Check env tags in the last comment
	if len(comments) >= 3 {
		if len(comments[2].envTags) != 1 {
			t.Errorf("Expected 1 env tag in comment 3, got %d", len(comments[2].envTags))
		}
		if len(comments[2].envTags) > 0 && comments[2].envTags[0].envName != "figure" {
			t.Errorf("Expected env name 'figure', got '%s'", comments[2].envTags[0].envName)
		}
	}
}

// TestIsSimilarContent tests the internal isSimilarContent function
func TestIsSimilarContent(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected bool
	}{
		{"hello world", "hello world", true},
		{"hello", "hello world", true},
		{"hello world", "hello", true},
		{"abc", "xyz", false},
		{"", "hello", false},
		{"hello", "", false},
		{"This is a test", "This is a test with more", true},
	}

	for _, test := range tests {
		result := isSimilarContent(test.s1, test.s2)
		if result != test.expected {
			t.Errorf("isSimilarContent(%q, %q) = %v, expected %v", test.s1, test.s2, result, test.expected)
		}
	}
}

// TestContainsSimilarEnvTags tests the internal containsSimilarEnvTags function
func TestContainsSimilarEnvTags(t *testing.T) {
	tests := []struct {
		line1    string
		line2    string
		expected bool
	}{
		{"% \\begin{figure}", "\\begin{figure}", true},
		{"% \\end{table}", "\\end{table}", true},
		{"% \\begin{figure}", "\\begin{table}", false},
		{"no env tags", "also no env tags", false},
		{"% \\begin{figure} \\end{figure}", "\\begin{figure}", true},
	}

	for _, test := range tests {
		result := containsSimilarEnvTags(test.line1, test.line2)
		if result != test.expected {
			t.Errorf("containsSimilarEnvTags(%q, %q) = %v, expected %v", test.line1, test.line2, result, test.expected)
		}
	}
}
