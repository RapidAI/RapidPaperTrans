// Package editor provides file editing tools for LaTeX documents.
package editor

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"latex-translator/internal/logger"
)

// ValidationResult contains the result of LaTeX validation
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error
type ValidationError struct {
	Line    int
	Column  int
	Message string
	Type    string // "syntax", "encoding", "structure"
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Line    int
	Column  int
	Message string
	Type    string
}

// LaTeXValidator validates LaTeX files
type LaTeXValidator struct{}

// NewLaTeXValidator creates a new LaTeXValidator
func NewLaTeXValidator() *LaTeXValidator {
	return &LaTeXValidator{}
}

// ValidateLaTeX validates a LaTeX file
func (v *LaTeXValidator) ValidateLaTeX(path string) (*ValidationResult, error) {
	logger.Debug("validating LaTeX file", logger.String("path", path))

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	result := &ValidationResult{Valid: true}

	// Check brace balance
	v.checkBraceBalance(string(content), result)

	// Check environments
	v.checkEnvironments(string(content), result)

	// Check common errors
	v.checkCommonErrors(string(content), result)

	// Check encoding issues
	v.checkEncodingIssues(string(content), result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	logger.Debug("validation completed",
		logger.Bool("valid", result.Valid),
		logger.Int("errorCount", len(result.Errors)),
		logger.Int("warningCount", len(result.Warnings)))

	return result, nil
}

// checkBraceBalance checks if braces are balanced
func (v *LaTeXValidator) checkBraceBalance(content string, result *ValidationResult) {
	openBrace := 0
	closeBrace := 0
	openSquare := 0
	closeSquare := 0
	openParen := 0
	closeParen := 0

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		// Skip comments
		if commentIdx := strings.Index(line, "%"); commentIdx != -1 {
			line = line[:commentIdx]
		}

		for i, ch := range line {
			switch ch {
			case '{':
				openBrace++
			case '}':
				closeBrace++
				if closeBrace > openBrace {
					result.Errors = append(result.Errors, ValidationError{
						Line:    lineNum + 1,
						Column:  i + 1,
						Message: "Unexpected closing brace '}'",
						Type:    "syntax",
					})
				}
			case '[':
				openSquare++
			case ']':
				closeSquare++
			case '(':
				openParen++
			case ')':
				closeParen++
			}
		}
	}

	if openBrace != closeBrace {
		result.Errors = append(result.Errors, ValidationError{
			Line:    0,
			Column:  0,
			Message: fmt.Sprintf("Unbalanced braces: %d open, %d close", openBrace, closeBrace),
			Type:    "syntax",
		})
	}

	if openSquare != closeSquare {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Line:    0,
			Column:  0,
			Message: fmt.Sprintf("Unbalanced square brackets: %d open, %d close", openSquare, closeSquare),
			Type:    "syntax",
		})
	}

	if openParen != closeParen {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Line:    0,
			Column:  0,
			Message: fmt.Sprintf("Unbalanced parentheses: %d open, %d close", openParen, closeParen),
			Type:    "syntax",
		})
	}
}

// checkEnvironments checks if LaTeX environments are properly closed
func (v *LaTeXValidator) checkEnvironments(content string, result *ValidationResult) {
	envPattern := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)
	matches := envPattern.FindAllStringSubmatch(content, -1)

	envStack := make([]string, 0)
	currentLine := 1
	currentPos := 0

	for _, match := range matches {
		// Find line number
		matchPos := strings.Index(content[currentPos:], match[0])
		if matchPos == -1 {
			continue
		}
		matchPos += currentPos

		// Count lines up to this match
		for currentPos < matchPos {
			if content[currentPos] == '\n' {
				currentLine++
			}
			currentPos++
		}

		cmd := match[1]  // "begin" or "end"
		env := match[2]  // environment name

		if cmd == "begin" {
			envStack = append(envStack, env)
		} else if cmd == "end" {
			if len(envStack) == 0 {
				result.Errors = append(result.Errors, ValidationError{
					Line:    currentLine,
					Column:  0,
					Message: fmt.Sprintf("Unexpected \\end{%s} without matching \\begin", env),
					Type:    "structure",
				})
			} else {
				last := envStack[len(envStack)-1]
				if last != env {
					result.Errors = append(result.Errors, ValidationError{
						Line:    currentLine,
						Column:  0,
						Message: fmt.Sprintf("Mismatched environment: \\begin{%s} ... \\end{%s}", last, env),
						Type:    "structure",
					})
				}
				envStack = envStack[:len(envStack)-1]
			}
		}
	}

	if len(envStack) > 0 {
		result.Errors = append(result.Errors, ValidationError{
			Line:    0,
			Column:  0,
			Message: fmt.Sprintf("Unclosed environments: %v", envStack),
			Type:    "structure",
		})
	}
}

// checkCommonErrors checks for common LaTeX errors
func (v *LaTeXValidator) checkCommonErrors(content string, result *ValidationResult) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		// Skip comments
		if commentIdx := strings.Index(line, "%"); commentIdx != -1 {
			line = line[:commentIdx]
		}

		// Check for common typos
		typos := map[string]string{
			"\\begn{":        "\\begin{",
			"\\ened{":        "\\end{",
			"\\docmentclass": "\\documentclass",
			"\\usepackge":    "\\usepackage",
		}

		for typo, correct := range typos {
			if strings.Contains(line, typo) {
				result.Errors = append(result.Errors, ValidationError{
					Line:    lineNum + 1,
					Column:  strings.Index(line, typo) + 1,
					Message: fmt.Sprintf("Possible typo: '%s' (did you mean '%s'?)", typo, correct),
					Type:    "syntax",
				})
			}
		}

		// Check for unclosed math environments
		dollarCount := strings.Count(line, "$")
		if dollarCount%2 != 0 {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Line:    lineNum + 1,
				Column:  0,
				Message: "Odd number of $ signs (possible unclosed math mode)",
				Type:    "syntax",
			})
		}

		// Check for double backslash at end of line (common error)
		if strings.HasSuffix(strings.TrimSpace(line), "\\\\\\") {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Line:    lineNum + 1,
				Column:  len(line),
				Message: "Triple backslash found (possible error)",
				Type:    "syntax",
			})
		}
	}

	// Check for missing \end{document}
	if !strings.Contains(content, "\\end{document}") {
		result.Errors = append(result.Errors, ValidationError{
			Line:    0,
			Column:  0,
			Message: "Missing \\end{document}",
			Type:    "structure",
		})
	}

	// Check for missing \begin{document}
	if !strings.Contains(content, "\\begin{document}") {
		result.Errors = append(result.Errors, ValidationError{
			Line:    0,
			Column:  0,
			Message: "Missing \\begin{document}",
			Type:    "structure",
		})
	}

	// Check for content after \end{document}
	endDocIdx := strings.Index(content, "\\end{document}")
	if endDocIdx != -1 {
		afterEnd := strings.TrimSpace(content[endDocIdx+len("\\end{document}"):])
		if len(afterEnd) > 0 {
			// Check if it's not just comments
			lines := strings.Split(afterEnd, "\n")
			hasNonComment := false
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" && !strings.HasPrefix(trimmed, "%") {
					hasNonComment = true
					break
				}
			}
			if hasNonComment {
				result.Warnings = append(result.Warnings, ValidationWarning{
					Line:    0,
					Column:  0,
					Message: "Content found after \\end{document}",
					Type:    "structure",
				})
			}
		}
	}
}

// checkEncodingIssues checks for encoding-related issues
func (v *LaTeXValidator) checkEncodingIssues(content string, result *ValidationResult) {
	// Check for common encoding issue patterns
	encodingIssuePatterns := []string{
		"鎮ㄧ殑",  // Garbled Chinese
		"锟斤拷",   // Common UTF-8 corruption
		"�",      // Replacement character
	}

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		for _, pattern := range encodingIssuePatterns {
			if strings.Contains(line, pattern) {
				result.Errors = append(result.Errors, ValidationError{
					Line:    lineNum + 1,
					Column:  strings.Index(line, pattern) + 1,
					Message: "Possible encoding issue detected (garbled text)",
					Type:    "encoding",
				})
				break
			}
		}
	}

	// Check for BOM in middle of file (should only be at start)
	if idx := strings.Index(content[3:], "\uFEFF"); idx != -1 {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Line:    0,
			Column:  0,
			Message: "BOM marker found in middle of file",
			Type:    "encoding",
		})
	}
}

// ValidateAndReport validates a file and returns a formatted report
func (v *LaTeXValidator) ValidateAndReport(path string) (string, error) {
	result, err := v.ValidateLaTeX(path)
	if err != nil {
		return "", err
	}

	var report strings.Builder
	report.WriteString(fmt.Sprintf("Validation Report for: %s\n", path))
	report.WriteString(strings.Repeat("=", 60) + "\n\n")

	if result.Valid {
		report.WriteString("✓ Validation PASSED\n\n")
	} else {
		report.WriteString("✗ Validation FAILED\n\n")
	}

	if len(result.Errors) > 0 {
		report.WriteString(fmt.Sprintf("Errors (%d):\n", len(result.Errors)))
		for i, err := range result.Errors {
			if err.Line > 0 {
				report.WriteString(fmt.Sprintf("  %d. Line %d, Col %d: %s [%s]\n",
					i+1, err.Line, err.Column, err.Message, err.Type))
			} else {
				report.WriteString(fmt.Sprintf("  %d. %s [%s]\n",
					i+1, err.Message, err.Type))
			}
		}
		report.WriteString("\n")
	}

	if len(result.Warnings) > 0 {
		report.WriteString(fmt.Sprintf("Warnings (%d):\n", len(result.Warnings)))
		for i, warn := range result.Warnings {
			if warn.Line > 0 {
				report.WriteString(fmt.Sprintf("  %d. Line %d, Col %d: %s [%s]\n",
					i+1, warn.Line, warn.Column, warn.Message, warn.Type))
			} else {
				report.WriteString(fmt.Sprintf("  %d. %s [%s]\n",
					i+1, warn.Message, warn.Type))
			}
		}
		report.WriteString("\n")
	}

	if result.Valid && len(result.Warnings) == 0 {
		report.WriteString("No issues found.\n")
	}

	return report.String(), nil
}

// QuickCheck performs a quick validation check (only critical errors)
func (v *LaTeXValidator) QuickCheck(path string) (bool, error) {
	result, err := v.ValidateLaTeX(path)
	if err != nil {
		return false, err
	}

	// Only check for critical errors
	criticalErrors := 0
	for _, err := range result.Errors {
		if err.Type == "syntax" || err.Type == "structure" {
			criticalErrors++
		}
	}

	return criticalErrors == 0, nil
}

// GetErrorSummary returns a brief summary of validation errors
func (v *LaTeXValidator) GetErrorSummary(result *ValidationResult) string {
	if result.Valid {
		return "No errors"
	}

	syntaxErrors := 0
	structureErrors := 0
	encodingErrors := 0

	for _, err := range result.Errors {
		switch err.Type {
		case "syntax":
			syntaxErrors++
		case "structure":
			structureErrors++
		case "encoding":
			encodingErrors++
		}
	}

	return fmt.Sprintf("%d syntax, %d structure, %d encoding errors",
		syntaxErrors, structureErrors, encodingErrors)
}
