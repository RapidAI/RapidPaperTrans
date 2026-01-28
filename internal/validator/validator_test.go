// Package validator provides LaTeX syntax validation and fixing functionality.
package validator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"latex-translator/internal/types"
)

// =============================================================================
// Constructor Tests
// =============================================================================

func TestNewSyntaxValidator(t *testing.T) {
	apiKey := "test-api-key"
	v := NewSyntaxValidator(apiKey)

	if v.GetAPIKey() != apiKey {
		t.Errorf("expected API key %q, got %q", apiKey, v.GetAPIKey())
	}

	if v.GetModel() != DefaultModel {
		t.Errorf("expected model %q, got %q", DefaultModel, v.GetModel())
	}
}

func TestNewSyntaxValidatorWithConfig(t *testing.T) {
	apiKey := "test-api-key"
	model := "gpt-3.5-turbo"
	apiURL := "https://custom.api.com"

	v := NewSyntaxValidatorWithConfig(apiKey, model, apiURL, 0)

	if v.GetAPIKey() != apiKey {
		t.Errorf("expected API key %q, got %q", apiKey, v.GetAPIKey())
	}

	if v.GetModel() != model {
		t.Errorf("expected model %q, got %q", model, v.GetModel())
	}
}

func TestNewSyntaxValidatorWithConfigDefaults(t *testing.T) {
	v := NewSyntaxValidatorWithConfig("key", "", "", 0)

	if v.GetModel() != DefaultModel {
		t.Errorf("expected default model %q, got %q", DefaultModel, v.GetModel())
	}
}

// =============================================================================
// Validate Tests - Empty and Valid Content
// =============================================================================

func TestValidate_EmptyContent(t *testing.T) {
	v := NewSyntaxValidator("test-key")
	result, err := v.Validate("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsValid {
		t.Error("expected empty content to be valid")
	}

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %d", len(result.Errors))
	}
}

func TestValidate_ValidContent(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	validContent := `\documentclass{article}
\begin{document}
Hello, world!
\section{Introduction}
This is a test with $x = 1$ inline math.
\begin{equation}
E = mc^2
\end{equation}
\end{document}`

	result, err := v.Validate(validContent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsValid {
		t.Errorf("expected valid content, got errors: %v", result.Errors)
	}
}

// =============================================================================
// Validate Tests - Unmatched Braces
// =============================================================================

func TestValidate_UnmatchedOpeningBrace(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `\section{Introduction`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched brace")
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "brace" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected brace error type")
	}
}

func TestValidate_UnmatchedClosingBrace(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `text}`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched closing brace")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "brace" && e.Message == "unmatched closing brace '}'" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected unmatched closing brace error")
	}
}

// =============================================================================
// Validate Tests - Unmatched Brackets
// =============================================================================

func TestValidate_UnmatchedOpeningBracket(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `\section[Introduction`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched bracket")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "bracket" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected bracket error type")
	}
}

func TestValidate_UnmatchedClosingBracket(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `text]`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched closing bracket")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "bracket" && e.Message == "unmatched closing bracket ']'" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected unmatched closing bracket error")
	}
}

// =============================================================================
// Validate Tests - Unmatched Math Delimiters
// =============================================================================

func TestValidate_UnmatchedInlineMath(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `This is $x = 1 without closing`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched inline math")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "math" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected math error type")
	}
}

func TestValidate_UnmatchedDisplayMath(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `This is $$x = 1 without closing`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched display math")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "math" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected math error type")
	}
}

func TestValidate_UnmatchedParenMath(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `This is \(x = 1 without closing`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched paren math")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "math" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected math error type")
	}
}

func TestValidate_UnmatchedBracketMath(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `This is \[x = 1 without closing`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched bracket math")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "math" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected math error type")
	}
}

// =============================================================================
// Validate Tests - Unmatched Environments
// =============================================================================

func TestValidate_UnmatchedBeginEnvironment(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `\begin{document}
Hello, world!`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched begin")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "environment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected environment error type")
	}
}

func TestValidate_UnmatchedEndEnvironment(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	content := `Hello, world!
\end{document}`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsValid {
		t.Error("expected invalid content due to unmatched end")
	}

	found := false
	for _, e := range result.Errors {
		if e.Type == "environment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected environment error type")
	}
}

func TestValidate_MismatchedEnvironment(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	// This has \begin{itemize} but \end{enumerate} - a true mismatch
	content := `\begin{document}
\begin{itemize}
\item Test
\end{enumerate}
\end{document}`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// This should detect mismatched environments
	if result.IsValid {
		t.Error("expected invalid content due to mismatched environments")
	}

	// Should have errors for both unmatched \begin{itemize} and unmatched \end{enumerate}
	foundEnvError := false
	for _, e := range result.Errors {
		if e.Type == "environment" {
			foundEnvError = true
			break
		}
	}
	if !foundEnvError {
		t.Error("expected environment error type")
	}
}

// =============================================================================
// Validate Tests - Comments
// =============================================================================

func TestValidate_IgnoresComments(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	// The unmatched brace is in a comment, so it should be ignored
	content := `\section{Test}
% This is a comment with unmatched { brace
\end{document}`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only have the unmatched \end{document} error
	braceError := false
	for _, e := range result.Errors {
		if e.Type == "brace" {
			braceError = true
			break
		}
	}
	if braceError {
		t.Error("should not report brace error in comment")
	}
}

// =============================================================================
// Validate Tests - Escaped Characters
// =============================================================================

func TestValidate_EscapedBraces(t *testing.T) {
	v := NewSyntaxValidator("test-key")

	// Escaped braces should not be counted
	content := `\{ and \}`

	result, err := v.Validate(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	braceError := false
	for _, e := range result.Errors {
		if e.Type == "brace" {
			braceError = true
			break
		}
	}
	if braceError {
		t.Error("should not report error for escaped braces")
	}
}

// =============================================================================
// Fix Tests
// =============================================================================

func TestFix_EmptyContent(t *testing.T) {
	v := NewSyntaxValidator("test-key")
	result, err := v.Fix("", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestFix_NoErrors(t *testing.T) {
	v := NewSyntaxValidator("test-key")
	content := "valid content"
	result, err := v.Fix(content, []types.SyntaxError{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != content {
		t.Errorf("expected %q, got %q", content, result)
	}
}

func TestFix_NoAPIKey(t *testing.T) {
	v := NewSyntaxValidator("")
	errors := []types.SyntaxError{{Line: 1, Column: 1, Message: "test", Type: "brace"}}
	_, err := v.Fix("content", errors)

	if err == nil {
		t.Fatal("expected error for missing API key")
	}

	appErr, ok := err.(*types.AppError)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}

	if appErr.Code != types.ErrConfig {
		t.Errorf("expected ErrConfig, got %v", appErr.Code)
	}
}

func TestFix_APISuccess(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected authorization header")
		}

		// Return mock response
		resp := ChatCompletionResponse{
			Choices: []Choice{
				{
					Message: Message{
						Content: "\\section{Fixed}",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := NewSyntaxValidator("test-key")
	v.SetAPIURL(server.URL)

	errors := []types.SyntaxError{
		{Line: 1, Column: 10, Message: "unmatched opening brace '{'", Type: "brace"},
	}

	result, err := v.Fix("\\section{Broken", errors)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "\\section{Fixed}" {
		t.Errorf("expected fixed content, got %q", result)
	}
}

func TestFix_APIError(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "Invalid API key"}}`))
	}))
	defer server.Close()

	v := NewSyntaxValidator("invalid-key")
	v.SetAPIURL(server.URL)

	errors := []types.SyntaxError{
		{Line: 1, Column: 1, Message: "test", Type: "brace"},
	}

	_, err := v.Fix("content", errors)

	if err == nil {
		t.Fatal("expected error for API failure")
	}

	appErr, ok := err.(*types.AppError)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}

	if appErr.Code != types.ErrAPICall {
		t.Errorf("expected ErrAPICall, got %v", appErr.Code)
	}
}

// =============================================================================
// Utility Function Tests
// =============================================================================

func TestFindUnescapedPercent(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected int
	}{
		{"no percent", "hello world", -1},
		{"simple percent", "hello % comment", 6},
		{"escaped percent", "hello \\% not comment", -1},
		{"double escaped percent", "hello \\\\% comment", 8},
		{"percent at start", "% comment", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findUnescapedPercent(tt.line)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetLineAndColumn(t *testing.T) {
	content := "line1\nline2\nline3"

	tests := []struct {
		pos          int
		expectedLine int
		expectedCol  int
	}{
		{0, 1, 1},   // Start of line 1
		{5, 1, 6},   // End of line 1 (newline)
		{6, 2, 1},   // Start of line 2
		{11, 2, 6},  // End of line 2 (newline)
		{12, 3, 1},  // Start of line 3
		{16, 3, 5},  // End of line 3
	}

	for _, tt := range tests {
		line, col := getLineAndColumn(content, tt.pos)
		if line != tt.expectedLine || col != tt.expectedCol {
			t.Errorf("pos %d: expected (%d, %d), got (%d, %d)",
				tt.pos, tt.expectedLine, tt.expectedCol, line, col)
		}
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestValidateAndFix_Integration(t *testing.T) {
	// Create a mock server for the fix
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatCompletionResponse{
			Choices: []Choice{
				{
					Message: Message{
						Content: `\begin{document}
Hello, world!
\end{document}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := NewSyntaxValidator("test-key")
	v.SetAPIURL(server.URL)

	// Content with missing \end{document}
	content := `\begin{document}
Hello, world!`

	// First validate
	result, err := v.Validate(content)
	if err != nil {
		t.Fatalf("validation error: %v", err)
	}

	if result.IsValid {
		t.Fatal("expected invalid content")
	}

	// Then fix
	fixed, err := v.Fix(content, result.Errors)
	if err != nil {
		t.Fatalf("fix error: %v", err)
	}

	// Validate the fixed content
	fixedResult, err := v.Validate(fixed)
	if err != nil {
		t.Fatalf("validation error after fix: %v", err)
	}

	if !fixedResult.IsValid {
		t.Errorf("expected fixed content to be valid, got errors: %v", fixedResult.Errors)
	}
}
