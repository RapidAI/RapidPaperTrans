package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

// LaTeXValidator validates LaTeX source files for common syntax errors
type LaTeXValidator struct {
	workDir string
}

// ValidationIssue represents a validation problem found in LaTeX source
type ValidationIssue struct {
	Severity string // "error", "warning"
	File     string
	Line     int
	Message  string
	Details  string
}

// ValidationResult contains the results of LaTeX validation
type ValidationResult struct {
	Valid   bool
	Issues  []ValidationIssue
	Summary string
}

// NewLaTeXValidator creates a new LaTeX validator
func NewLaTeXValidator(workDir string) *LaTeXValidator {
	return &LaTeXValidator{
		workDir: workDir,
	}
}

// ValidateMainFile validates the main LaTeX file and its dependencies
func (v *LaTeXValidator) ValidateMainFile(mainTexPath string) (*ValidationResult, error) {
	logger.Info("validating LaTeX source", logger.String("file", mainTexPath))

	result := &ValidationResult{
		Valid:  true,
		Issues: []ValidationIssue{},
	}

	// Check if file exists
	if _, err := os.Stat(mainTexPath); os.IsNotExist(err) {
		return nil, types.NewAppError(types.ErrFileNotFound, "main tex file not found", err)
	}

	// Read main file
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "failed to read main tex file", err)
	}

	contentStr := string(content)
	fileName := filepath.Base(mainTexPath)

	// Run validation checks
	v.checkBraceBalance(fileName, contentStr, result)
	v.checkDocumentStructure(fileName, contentStr, result)
	v.checkCommandDefinitions(fileName, contentStr, result)
	v.checkEnvironments(fileName, contentStr, result)
	v.checkCommonErrors(fileName, contentStr, result)

	// Check included files
	v.checkIncludedFiles(mainTexPath, result)

	// Generate summary
	v.generateSummary(result)

	logger.Info("validation completed",
		logger.Bool("valid", result.Valid),
		logger.Int("issues", len(result.Issues)))

	return result, nil
}

// checkBraceBalance checks if braces are balanced
func (v *LaTeXValidator) checkBraceBalance(fileName, content string, result *ValidationResult) {
	depth := 0
	line := 1
	inComment := false
	maxDepth := 0
	lastChar := ' '

	for i, char := range content {
		if char == '\n' {
			line++
			inComment = false
			continue
		}

		// Skip comments
		if char == '%' && lastChar != '\\' {
			inComment = true
		}

		if inComment {
			lastChar = char
			continue
		}

		// Check braces (skip escaped ones)
		if char == '{' && lastChar != '\\' {
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
			if depth > 100 {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity: "warning",
					File:     fileName,
					Line:     line,
					Message:  "Suspiciously deep brace nesting",
					Details:  fmt.Sprintf("Nesting depth: %d at position %d", depth, i),
				})
			}
		} else if char == '}' && lastChar != '\\' {
			depth--
			if depth < 0 {
				result.Valid = false
				result.Issues = append(result.Issues, ValidationIssue{
					Severity: "error",
					File:     fileName,
					Line:     line,
					Message:  "Extra closing brace",
					Details:  "Found '}' without matching '{'",
				})
				// Reset to prevent cascading errors
				depth = 0
			}
		}

		lastChar = char
	}

	if depth != 0 {
		result.Valid = false
		result.Issues = append(result.Issues, ValidationIssue{
			Severity: "error",
			File:     fileName,
			Line:     0,
			Message:  "Unbalanced braces",
			Details:  fmt.Sprintf("%d unclosed opening braces", depth),
		})
	}

	logger.Debug("brace balance check",
		logger.String("file", fileName),
		logger.Int("finalDepth", depth),
		logger.Int("maxDepth", maxDepth))
}

// checkDocumentStructure checks for \documentclass and \begin{document}/\end{document}
func (v *LaTeXValidator) checkDocumentStructure(fileName, content string, result *ValidationResult) {
	hasDocumentClass := strings.Contains(content, "\\documentclass")
	hasBeginDocument := strings.Contains(content, "\\begin{document}")
	hasEndDocument := strings.Contains(content, "\\end{document}")

	if !hasDocumentClass {
		result.Valid = false
		result.Issues = append(result.Issues, ValidationIssue{
			Severity: "error",
			File:     fileName,
			Line:     0,
			Message:  "Missing \\documentclass",
			Details:  "LaTeX document must start with \\documentclass",
		})
	}

	if !hasBeginDocument {
		result.Valid = false
		result.Issues = append(result.Issues, ValidationIssue{
			Severity: "error",
			File:     fileName,
			Line:     0,
			Message:  "Missing \\begin{document}",
			Details:  "LaTeX document must contain \\begin{document}",
		})
	}

	if !hasEndDocument {
		result.Valid = false
		result.Issues = append(result.Issues, ValidationIssue{
			Severity: "error",
			File:     fileName,
			Line:     0,
			Message:  "Missing \\end{document}",
			Details:  "LaTeX document must end with \\end{document}",
		})
	}

	// Check for content after \end{document}
	if hasEndDocument {
		endDocIdx := strings.LastIndex(content, "\\end{document}")
		afterEnd := strings.TrimSpace(content[endDocIdx+len("\\end{document}"):])
		if len(afterEnd) > 0 {
			// Check if it's just comments or whitespace
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
				result.Valid = false
				result.Issues = append(result.Issues, ValidationIssue{
					Severity: "error",
					File:     fileName,
					Line:     0,
					Message:  "Content after \\end{document}",
					Details:  fmt.Sprintf("Found %d characters after \\end{document}: %s", len(afterEnd), truncate(afterEnd, 50)),
				})
			}
		}
	}
}

// checkCommandDefinitions checks \newcommand and \renewcommand definitions
func (v *LaTeXValidator) checkCommandDefinitions(fileName, content string, result *ValidationResult) {
	lines := strings.Split(content, "\n")
	inComment := false

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Track comments
		if strings.HasPrefix(trimmed, "%") {
			inComment = true
			continue
		}
		if inComment && !strings.Contains(trimmed, "\\") {
			continue
		}
		inComment = false

		// Check for \newcommand or \renewcommand
		if strings.Contains(line, "\\newcommand") || strings.Contains(line, "\\renewcommand") {
			// Simple heuristic: if the line contains the command definition start
			// but doesn't have balanced braces, it might be incomplete
			openBraces := strings.Count(line, "{") - strings.Count(line, "\\{")
			closeBraces := strings.Count(line, "}") - strings.Count(line, "\\}")

			if openBraces > closeBraces {
				// Check next few lines to see if it's a multi-line definition
				balanced := false
				checkLines := 10
				if lineNum+checkLines > len(lines) {
					checkLines = len(lines) - lineNum - 1
				}

				totalOpen := openBraces
				totalClose := closeBraces
				for i := 1; i <= checkLines; i++ {
					nextLine := lines[lineNum+i]
					totalOpen += strings.Count(nextLine, "{") - strings.Count(nextLine, "\\{")
					totalClose += strings.Count(nextLine, "}") - strings.Count(nextLine, "\\}")
					if totalOpen == totalClose {
						balanced = true
						break
					}
				}

				if !balanced {
					result.Issues = append(result.Issues, ValidationIssue{
						Severity: "warning",
						File:     fileName,
						Line:     lineNum + 1,
						Message:  "Possibly incomplete command definition",
						Details:  fmt.Sprintf("Line: %s", truncate(line, 60)),
					})
				}
			}
		}
	}
}

// checkEnvironments checks for balanced \begin{} and \end{} pairs
func (v *LaTeXValidator) checkEnvironments(fileName, content string, result *ValidationResult) {
	// Find all \begin{env} and \end{env} pairs
	envStack := []string{}
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		// Skip comments
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "%") {
			continue
		}

		// Find \begin{...}
		beginIdx := 0
		for {
			idx := strings.Index(line[beginIdx:], "\\begin{")
			if idx == -1 {
				break
			}
			idx += beginIdx

			// Extract environment name
			endIdx := strings.Index(line[idx:], "}")
			if endIdx != -1 {
				envName := line[idx+7 : idx+endIdx]
				envStack = append(envStack, envName)
			}
			beginIdx = idx + 7
		}

		// Find \end{...}
		endIdx := 0
		for {
			idx := strings.Index(line[endIdx:], "\\end{")
			if idx == -1 {
				break
			}
			idx += endIdx

			// Extract environment name
			closeIdx := strings.Index(line[idx:], "}")
			if closeIdx != -1 {
				envName := line[idx+5 : idx+closeIdx]

				// Check if it matches the last opened environment
				if len(envStack) == 0 {
					result.Issues = append(result.Issues, ValidationIssue{
						Severity: "warning",
						File:     fileName,
						Line:     lineNum + 1,
						Message:  fmt.Sprintf("\\end{%s} without matching \\begin{%s}", envName, envName),
						Details:  truncate(line, 60),
					})
				} else {
					lastEnv := envStack[len(envStack)-1]
					if lastEnv != envName {
						result.Issues = append(result.Issues, ValidationIssue{
							Severity: "warning",
							File:     fileName,
							Line:     lineNum + 1,
							Message:  fmt.Sprintf("Environment mismatch: expected \\end{%s}, found \\end{%s}", lastEnv, envName),
							Details:  truncate(line, 60),
						})
					}
					envStack = envStack[:len(envStack)-1]
				}
			}
			endIdx = idx + 5
		}
	}

	// Check for unclosed environments
	if len(envStack) > 0 {
		result.Issues = append(result.Issues, ValidationIssue{
			Severity: "warning",
			File:     fileName,
			Line:     0,
			Message:  "Unclosed environments",
			Details:  fmt.Sprintf("Missing \\end{} for: %s", strings.Join(envStack, ", ")),
		})
	}
}

// checkCommonErrors checks for common LaTeX errors
func (v *LaTeXValidator) checkCommonErrors(fileName, content string, result *ValidationResult) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "%") {
			continue
		}

		// Check for unescaped special characters in text
		// (This is a simplified check)
		if strings.Contains(line, "$") {
			// Count $ signs (should be even for inline math)
			count := strings.Count(line, "$") - strings.Count(line, "\\$")
			if count%2 != 0 {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity: "warning",
					File:     fileName,
					Line:     lineNum + 1,
					Message:  "Unmatched $ for inline math",
					Details:  truncate(line, 60),
				})
			}
		}

		// Check for common typos
		if strings.Contains(line, "\\begn{") {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity: "error",
				File:     fileName,
				Line:     lineNum + 1,
				Message:  "Typo: \\begn should be \\begin",
				Details:  truncate(line, 60),
			})
		}
	}
}

// checkIncludedFiles validates files included via \input or \include
func (v *LaTeXValidator) checkIncludedFiles(mainTexPath string, result *ValidationResult) {
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		return
	}

	contentStr := string(content)
	baseDir := filepath.Dir(mainTexPath)

	// Find \input{...} and \include{...}
	patterns := []string{"\\input{", "\\include{"}
	for _, pattern := range patterns {
		idx := 0
		for {
			pos := strings.Index(contentStr[idx:], pattern)
			if pos == -1 {
				break
			}
			pos += idx

			// Extract filename
			endIdx := strings.Index(contentStr[pos:], "}")
			if endIdx != -1 {
				filename := contentStr[pos+len(pattern) : pos+endIdx]
				filename = strings.TrimSpace(filename)

				// Try with and without .tex extension
				paths := []string{
					filepath.Join(baseDir, filename),
					filepath.Join(baseDir, filename+".tex"),
				}

				found := false
				for _, path := range paths {
					if _, err := os.Stat(path); err == nil {
						found = true
						break
					}
				}

				if !found {
					result.Issues = append(result.Issues, ValidationIssue{
						Severity: "warning",
						File:     filepath.Base(mainTexPath),
						Line:     0,
						Message:  "Included file not found",
						Details:  fmt.Sprintf("%s{%s}", pattern[:len(pattern)-1], filename),
					})
				}
			}

			idx = pos + len(pattern)
		}
	}
}

// generateSummary creates a human-readable summary of validation results
func (v *LaTeXValidator) generateSummary(result *ValidationResult) {
	if result.Valid && len(result.Issues) == 0 {
		result.Summary = "✓ LaTeX source validation passed with no issues"
		return
	}

	errorCount := 0
	warningCount := 0

	for _, issue := range result.Issues {
		if issue.Severity == "error" {
			errorCount++
		} else {
			warningCount++
		}
	}

	if errorCount > 0 {
		result.Summary = fmt.Sprintf("✗ Validation failed: %d error(s), %d warning(s)", errorCount, warningCount)
	} else {
		result.Summary = fmt.Sprintf("⚠ Validation passed with %d warning(s)", warningCount)
	}
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FormatIssues formats validation issues for display
func FormatIssues(issues []ValidationIssue) string {
	if len(issues) == 0 {
		return "No issues found"
	}

	var sb strings.Builder
	for i, issue := range issues {
		icon := "⚠"
		if issue.Severity == "error" {
			icon = "✗"
		}

		sb.WriteString(fmt.Sprintf("%s [%s] %s", icon, strings.ToUpper(issue.Severity), issue.Message))
		if issue.File != "" {
			sb.WriteString(fmt.Sprintf(" (file: %s", issue.File))
			if issue.Line > 0 {
				sb.WriteString(fmt.Sprintf(", line: %d", issue.Line))
			}
			sb.WriteString(")")
		}
		if issue.Details != "" {
			sb.WriteString(fmt.Sprintf("\n  Details: %s", issue.Details))
		}
		if i < len(issues)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
