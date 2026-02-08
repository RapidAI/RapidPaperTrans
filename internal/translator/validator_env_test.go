// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"testing"
)

// ============================================================
// ValidateEnvironments Tests (Task 4.1)
// ============================================================

func TestValidateEnvironments_BalancedEnvironments(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{
			name: "single balanced environment",
			input: `\begin{document}
content
\end{document}`,
			isValid: true,
		},
		{
			name: "multiple balanced environments",
			input: `\begin{document}
\begin{figure}
\end{figure}
\begin{table}
\end{table}
\end{document}`,
			isValid: true,
		},
		{
			name: "nested balanced environments",
			input: `\begin{document}
\begin{figure}
\begin{center}
content
\end{center}
\end{figure}
\end{document}`,
			isValid: true,
		},
		{
			name:    "empty content",
			input:   "",
			isValid: true,
		},
		{
			name:    "no environments",
			input:   "Just some text without environments.",
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEnvironments(tt.input)
			if result.IsValid != tt.isValid {
				t.Errorf("ValidateEnvironments().IsValid = %v, want %v", result.IsValid, tt.isValid)
			}
		})
	}
}

func TestValidateEnvironments_MissingEndTag(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedEnv    string
		expectedDiff   int
	}{
		{
			name: "missing end document",
			input: `\begin{document}
content`,
			expectedEnv:  "document",
			expectedDiff: 1, // 1 begin, 0 end
		},
		{
			name: "missing end figure",
			input: `\begin{document}
\begin{figure}
content
\end{document}`,
			expectedEnv:  "figure",
			expectedDiff: 1,
		},
		{
			name: "multiple missing end tags",
			input: `\begin{document}
\begin{figure}
\begin{center}
content`,
			expectedEnv:  "center", // Will check all, but center is one of them
			expectedDiff: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEnvironments(tt.input)
			
			if result.IsValid {
				t.Error("ValidateEnvironments().IsValid should be false for missing end tags")
			}
			
			// Check that the expected environment is in mismatches
			found := false
			for _, m := range result.Mismatches {
				if m.EnvName == tt.expectedEnv {
					found = true
					if m.Difference != tt.expectedDiff {
						t.Errorf("Mismatch difference for %s = %d, want %d", tt.expectedEnv, m.Difference, tt.expectedDiff)
					}
				}
			}
			if !found {
				t.Errorf("Expected environment %s not found in mismatches", tt.expectedEnv)
			}
		})
	}
}

func TestValidateEnvironments_ExtraEndTag(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedEnv    string
		expectedDiff   int
	}{
		{
			name: "extra end document",
			input: `\begin{document}
content
\end{document}
\end{document}`,
			expectedEnv:  "document",
			expectedDiff: -1, // 1 begin, 2 end
		},
		{
			name: "end without begin",
			input: `content
\end{figure}`,
			expectedEnv:  "figure",
			expectedDiff: -1, // 0 begin, 1 end
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEnvironments(tt.input)
			
			if result.IsValid {
				t.Error("ValidateEnvironments().IsValid should be false for extra end tags")
			}
			
			// Check that the expected environment is in mismatches
			found := false
			for _, m := range result.Mismatches {
				if m.EnvName == tt.expectedEnv {
					found = true
					if m.Difference != tt.expectedDiff {
						t.Errorf("Mismatch difference for %s = %d, want %d", tt.expectedEnv, m.Difference, tt.expectedDiff)
					}
				}
			}
			if !found {
				t.Errorf("Expected environment %s not found in mismatches", tt.expectedEnv)
			}
		})
	}
}

func TestValidateEnvironments_EnvironmentCounting(t *testing.T) {
	input := `\begin{document}
\begin{figure}
\end{figure}
\begin{figure}
\end{figure}
\begin{table}
\begin{tabular}{cc}
\end{tabular}
\end{table}
\end{document}`

	result := ValidateEnvironments(input)

	// Check counts for each environment
	expectedCounts := map[string]EnvCount{
		"document": {BeginCount: 1, EndCount: 1},
		"figure":   {BeginCount: 2, EndCount: 2},
		"table":    {BeginCount: 1, EndCount: 1},
		"tabular":  {BeginCount: 1, EndCount: 1},
	}

	for envName, expected := range expectedCounts {
		actual, ok := result.Environments[envName]
		if !ok {
			t.Errorf("Environment %s not found in results", envName)
			continue
		}
		if actual.BeginCount != expected.BeginCount {
			t.Errorf("Environment %s BeginCount = %d, want %d", envName, actual.BeginCount, expected.BeginCount)
		}
		if actual.EndCount != expected.EndCount {
			t.Errorf("Environment %s EndCount = %d, want %d", envName, actual.EndCount, expected.EndCount)
		}
	}
}

func TestValidateEnvironments_NestingOrder(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		nestingValid bool
		hasError     bool
	}{
		{
			name: "correct nesting",
			input: `\begin{document}
\begin{figure}
\end{figure}
\end{document}`,
			nestingValid: true,
			hasError:     false,
		},
		{
			name: "incorrect nesting - wrong order",
			input: `\begin{document}
\begin{figure}
\end{document}
\end{figure}`,
			nestingValid: false,
			hasError:     true,
		},
		{
			name: "incorrect nesting - overlapping",
			input: `\begin{figure}
\begin{table}
\end{figure}
\end{table}`,
			nestingValid: false,
			hasError:     true,
		},
		{
			name: "deeply nested correct",
			input: `\begin{document}
\begin{figure}
\begin{center}
\begin{minipage}{0.5\textwidth}
content
\end{minipage}
\end{center}
\end{figure}
\end{document}`,
			nestingValid: true,
			hasError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEnvironments(tt.input)
			
			if result.NestingValid != tt.nestingValid {
				t.Errorf("ValidateEnvironments().NestingValid = %v, want %v", result.NestingValid, tt.nestingValid)
			}
			
			if tt.hasError && result.NestingError == "" {
				t.Error("Expected nesting error message but got empty string")
			}
			
			if !tt.hasError && result.NestingError != "" {
				t.Errorf("Expected no nesting error but got: %s", result.NestingError)
			}
		})
	}
}

func TestValidateEnvironments_MismatchReporting(t *testing.T) {
	input := `\begin{document}
\begin{figure}
\begin{figure}
\end{figure}
\end{document}`

	result := ValidateEnvironments(input)

	if result.IsValid {
		t.Error("ValidateEnvironments().IsValid should be false")
	}

	// Should have exactly one mismatch (figure)
	if len(result.Mismatches) != 1 {
		t.Errorf("Expected 1 mismatch, got %d", len(result.Mismatches))
	}

	if len(result.Mismatches) > 0 {
		m := result.Mismatches[0]
		if m.EnvName != "figure" {
			t.Errorf("Mismatch EnvName = %s, want figure", m.EnvName)
		}
		if m.Expected != 2 {
			t.Errorf("Mismatch Expected = %d, want 2", m.Expected)
		}
		if m.Actual != 1 {
			t.Errorf("Mismatch Actual = %d, want 1", m.Actual)
		}
		if m.Difference != 1 {
			t.Errorf("Mismatch Difference = %d, want 1", m.Difference)
		}
	}
}

func TestValidateEnvironments_SpecialEnvironments(t *testing.T) {
	// Test with math environments and other special cases
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{
			name: "equation environment",
			input: `\begin{equation}
x + y = z
\end{equation}`,
			isValid: true,
		},
		{
			name: "align environment",
			input: `\begin{align}
a &= b \\
c &= d
\end{align}`,
			isValid: true,
		},
		{
			name: "itemize environment",
			input: `\begin{itemize}
\item First
\item Second
\end{itemize}`,
			isValid: true,
		},
		{
			name: "enumerate environment",
			input: `\begin{enumerate}
\item First
\item Second
\end{enumerate}`,
			isValid: true,
		},
		{
			name: "verbatim environment",
			input: `\begin{verbatim}
code here
\end{verbatim}`,
			isValid: true,
		},
		{
			name: "starred environment",
			input: `\begin{equation*}
x = y
\end{equation*}`,
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEnvironments(tt.input)
			if result.IsValid != tt.isValid {
				t.Errorf("ValidateEnvironments().IsValid = %v, want %v", result.IsValid, tt.isValid)
			}
		})
	}
}

func TestFormatEnvironmentValidation_Valid(t *testing.T) {
	result := &EnvironmentValidation{
		IsValid:      true,
		Environments: map[string]EnvCount{"document": {1, 1}},
		Mismatches:   []EnvMismatch{},
		NestingValid: true,
	}

	formatted := FormatEnvironmentValidation(result)
	
	if formatted == "" {
		t.Error("FormatEnvironmentValidation() should return non-empty string for valid result")
	}
	
	if formatted != "环境验证通过: 所有 \\begin 和 \\end 标签匹配正确" {
		t.Errorf("Unexpected format: %s", formatted)
	}
}

func TestFormatEnvironmentValidation_Invalid(t *testing.T) {
	result := &EnvironmentValidation{
		IsValid: false,
		Environments: map[string]EnvCount{
			"figure": {BeginCount: 2, EndCount: 1},
		},
		Mismatches: []EnvMismatch{
			{EnvName: "figure", Expected: 2, Actual: 1, Difference: 1},
		},
		NestingValid: true,
	}

	formatted := FormatEnvironmentValidation(result)
	
	if formatted == "" {
		t.Error("FormatEnvironmentValidation() should return non-empty string for invalid result")
	}
	
	// Check that it contains expected information
	if !contains(formatted, "figure") {
		t.Error("Formatted output should contain environment name 'figure'")
	}
	if !contains(formatted, "缺少") {
		t.Error("Formatted output should indicate missing end tag")
	}
}

func TestFormatEnvironmentValidation_NestingError(t *testing.T) {
	result := &EnvironmentValidation{
		IsValid:      false,
		Environments: map[string]EnvCount{},
		Mismatches:   []EnvMismatch{},
		NestingValid: false,
		NestingError: "nesting error: expected \\end{figure} but found \\end{document}",
	}

	formatted := FormatEnvironmentValidation(result)
	
	if !contains(formatted, "嵌套错误") {
		t.Error("Formatted output should contain nesting error message")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEnvCount_Fields(t *testing.T) {
	count := EnvCount{
		BeginCount: 5,
		EndCount:   3,
	}

	if count.BeginCount != 5 {
		t.Errorf("EnvCount.BeginCount = %d, want 5", count.BeginCount)
	}
	if count.EndCount != 3 {
		t.Errorf("EnvCount.EndCount = %d, want 3", count.EndCount)
	}
}

func TestEnvMismatch_Fields(t *testing.T) {
	mismatch := EnvMismatch{
		EnvName:    "figure",
		Expected:   5,
		Actual:     3,
		Difference: 2,
	}

	if mismatch.EnvName != "figure" {
		t.Errorf("EnvMismatch.EnvName = %s, want figure", mismatch.EnvName)
	}
	if mismatch.Expected != 5 {
		t.Errorf("EnvMismatch.Expected = %d, want 5", mismatch.Expected)
	}
	if mismatch.Actual != 3 {
		t.Errorf("EnvMismatch.Actual = %d, want 3", mismatch.Actual)
	}
	if mismatch.Difference != 2 {
		t.Errorf("EnvMismatch.Difference = %d, want 2", mismatch.Difference)
	}
}

func TestEnvironmentValidation_Fields(t *testing.T) {
	validation := EnvironmentValidation{
		IsValid: true,
		Environments: map[string]EnvCount{
			"document": {BeginCount: 1, EndCount: 1},
		},
		Mismatches:   []EnvMismatch{},
		NestingValid: true,
		NestingError: "",
	}

	if !validation.IsValid {
		t.Error("EnvironmentValidation.IsValid should be true")
	}
	if !validation.NestingValid {
		t.Error("EnvironmentValidation.NestingValid should be true")
	}
	if validation.NestingError != "" {
		t.Errorf("EnvironmentValidation.NestingError should be empty, got %s", validation.NestingError)
	}
	if len(validation.Environments) != 1 {
		t.Errorf("EnvironmentValidation.Environments should have 1 entry, got %d", len(validation.Environments))
	}
}
