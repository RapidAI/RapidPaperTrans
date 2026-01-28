package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

// SimpleFixer provides automatic fixes for common LaTeX errors
type SimpleFixer struct {
	workDir string
}

// FixResult contains the result of an automatic fix attempt
type FixResult struct {
	Fixed       bool     // Whether any fixes were applied
	FixesApplied []string // List of fixes that were applied
	BackupPath  string   // Path to backup file
}

// NewSimpleFixer creates a new simple fixer
func NewSimpleFixer(workDir string) *SimpleFixer {
	return &SimpleFixer{
		workDir: workDir,
	}
}

// TryFixFile attempts to automatically fix common errors in a LaTeX file
func (f *SimpleFixer) TryFixFile(filePath string, validationResult *ValidationResult) (*FixResult, error) {
	logger.Info("attempting to fix LaTeX file", logger.String("file", filePath))

	result := &FixResult{
		Fixed:        false,
		FixesApplied: []string{},
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "failed to read file", err)
	}

	originalContent := string(content)
	fixedContent := originalContent

	// Create backup
	backupPath := filePath + ".backup"
	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		logger.Warn("failed to create backup", logger.Err(err))
	} else {
		result.BackupPath = backupPath
		logger.Debug("created backup", logger.String("path", backupPath))
	}

	// Apply fixes based on validation issues
	if validationResult != nil {
		for _, issue := range validationResult.Issues {
			switch issue.Message {
			case "Unbalanced braces":
				if strings.Contains(issue.Details, "unclosed opening braces") {
					fixedContent, _ = f.fixUnbalancedBraces(fixedContent, issue)
					result.FixesApplied = append(result.FixesApplied, "Fixed unbalanced braces")
				}

			case "Content after \\end{document}":
				fixedContent = f.fixContentAfterEndDocument(fixedContent)
				result.FixesApplied = append(result.FixesApplied, "Removed content after \\end{document}")

			case "Unclosed environments":
				// This is harder to fix automatically, skip for now
				logger.Debug("skipping unclosed environments fix (too complex)")
			}
		}
	}

	// Apply general fixes
	fixedContent, applied := f.fixCommonIssues(fixedContent)
	result.FixesApplied = append(result.FixesApplied, applied...)

	// Check if any changes were made
	if fixedContent != originalContent {
		result.Fixed = true

		// Write fixed content
		if err := os.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
			return nil, types.NewAppError(types.ErrInternal, "failed to write fixed file", err)
		}

		logger.Info("applied fixes to file",
			logger.String("file", filePath),
			logger.Int("fixCount", len(result.FixesApplied)))
	} else {
		logger.Info("no fixes needed or applicable", logger.String("file", filePath))
	}

	return result, nil
}

// fixUnbalancedBraces attempts to fix unbalanced braces
func (f *SimpleFixer) fixUnbalancedBraces(content string, issue ValidationIssue) (string, bool) {
	// Extract the number of unclosed braces from the details
	var count int
	if _, err := fmt.Sscanf(issue.Details, "%d unclosed opening braces", &count); err != nil {
		return content, false
	}

	// Simple heuristic: add closing braces before \end{document}
	endDocIdx := strings.LastIndex(content, "\\end{document}")
	if endDocIdx == -1 {
		return content, false
	}

	// Add the missing closing braces
	closingBraces := strings.Repeat("}", count)
	fixed := content[:endDocIdx] + "\n" + closingBraces + "\n" + content[endDocIdx:]

	logger.Debug("added closing braces", logger.Int("count", count))
	return fixed, true
}

// fixContentAfterEndDocument removes content after \end{document}
func (f *SimpleFixer) fixContentAfterEndDocument(content string) string {
	endDocIdx := strings.LastIndex(content, "\\end{document}")
	if endDocIdx == -1 {
		return content
	}

	// Keep everything up to and including \end{document}
	fixed := content[:endDocIdx+len("\\end{document}")] + "\n"

	logger.Debug("removed content after \\end{document}")
	return fixed
}

// fixCommonIssues applies fixes for common LaTeX issues
func (f *SimpleFixer) fixCommonIssues(content string) (string, []string) {
	applied := []string{}
	fixed := content

	// Fix 0: Fix TikZ and environment issues (most critical)
	fixed, tikzFixes := f.fixTikZIssues(fixed)
	applied = append(applied, tikzFixes...)

	// Fix 0.5: Fix unclosed color commands like \blue{, \red{
	fixed, colorFixes := f.fixUnclosedColorCommands(fixed)
	applied = append(applied, colorFixes...)

	// Fix 1: Remove duplicate \end{document}
	endDocCount := strings.Count(fixed, "\\end{document}")
	if endDocCount > 1 {
		// Keep only the last one
		lastIdx := strings.LastIndex(fixed, "\\end{document}")
		beforeLast := fixed[:lastIdx]
		// Remove all other occurrences
		beforeLast = strings.ReplaceAll(beforeLast, "\\end{document}", "")
		fixed = beforeLast + fixed[lastIdx:]
		applied = append(applied, fmt.Sprintf("Removed %d duplicate \\end{document}", endDocCount-1))
		logger.Debug("removed duplicate \\end{document}", logger.Int("count", endDocCount-1))
	}

	// Fix 2: Fix common typos
	typos := map[string]string{
		"\\begn{":      "\\begin{",
		"\\ened{":      "\\end{",
		"\\docmentclass": "\\documentclass",
	}

	for typo, correct := range typos {
		if strings.Contains(fixed, typo) {
			count := strings.Count(fixed, typo)
			fixed = strings.ReplaceAll(fixed, typo, correct)
			applied = append(applied, fmt.Sprintf("Fixed typo: %s -> %s (%d occurrences)", typo, correct, count))
			logger.Debug("fixed typo", logger.String("from", typo), logger.String("to", correct), logger.Int("count", count))
		}
	}

	// Fix 3: Remove trailing whitespace and ensure single newline at end
	lines := strings.Split(fixed, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	fixed = strings.Join(lines, "\n")

	// Ensure file ends with exactly one newline
	fixed = strings.TrimRight(fixed, "\n") + "\n"

	// Fix 4: Fix common encoding issues (if any)
	// Replace common problematic characters
	replacements := map[string]string{
		"\r\n": "\n",  // Windows line endings
		"\r":   "\n",  // Old Mac line endings
	}

	for old, new := range replacements {
		if strings.Contains(fixed, old) {
			fixed = strings.ReplaceAll(fixed, old, new)
		}
	}

	// Fix 5: Fix unmatched dollar signs in comments
	// This is a simple heuristic: if a line has an odd number of $ and starts with %,
	// it's probably a comment and we can ignore it
	fixedLines := strings.Split(fixed, "\n")
	for i, line := range fixedLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%") {
			// It's a comment, check for unmatched $
			dollarCount := strings.Count(line, "$") - strings.Count(line, "\\$")
			if dollarCount%2 != 0 {
				// Odd number of $, escape the last one
				lastDollarIdx := strings.LastIndex(line, "$")
				if lastDollarIdx > 0 && line[lastDollarIdx-1] != '\\' {
					fixedLines[i] = line[:lastDollarIdx] + "\\$" + line[lastDollarIdx+1:]
					if len(applied) == 0 || applied[len(applied)-1] != "Fixed unmatched $ in comments" {
						applied = append(applied, "Fixed unmatched $ in comments")
					}
				}
			}
		}
	}
	fixed = strings.Join(fixedLines, "\n")

	return fixed, applied
}

// RestoreBackup restores a file from its backup
func (f *SimpleFixer) RestoreBackup(filePath string) error {
	backupPath := filePath + ".backup"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return types.NewAppError(types.ErrFileNotFound, "backup file not found", err)
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		return types.NewAppError(types.ErrInternal, "failed to read backup", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return types.NewAppError(types.ErrInternal, "failed to restore backup", err)
	}

	logger.Info("restored file from backup", logger.String("file", filePath))
	return nil
}

// CleanupBackup removes the backup file
func (f *SimpleFixer) CleanupBackup(filePath string) error {
	backupPath := filePath + ".backup"

	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove backup", logger.Err(err))
		return err
	}

	logger.Debug("removed backup file", logger.String("path", backupPath))
	return nil
}

// FixAllTexFiles attempts to fix all .tex files in a directory
func (f *SimpleFixer) FixAllTexFiles(dir string) (map[string]*FixResult, error) {
	results := make(map[string]*FixResult)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".tex") {
			// Validate first
			validator := NewLaTeXValidator(dir)
			validationResult, err := validator.ValidateMainFile(path)
			if err != nil {
				logger.Warn("validation failed, skipping fix", logger.String("file", path), logger.Err(err))
				return nil
			}

			// Only fix if there are issues
			if !validationResult.Valid || len(validationResult.Issues) > 0 {
				fixResult, err := f.TryFixFile(path, validationResult)
				if err != nil {
					logger.Warn("fix failed", logger.String("file", path), logger.Err(err))
					return nil
				}

				if fixResult.Fixed {
					relPath, _ := filepath.Rel(dir, path)
					results[relPath] = fixResult
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "failed to walk directory", err)
	}

	return results, nil
}

// SmartFix attempts to fix a file with validation-guided fixes
func (f *SimpleFixer) SmartFix(filePath string) (*FixResult, error) {
	// First, validate the file
	validator := NewLaTeXValidator(f.workDir)
	validationResult, err := validator.ValidateMainFile(filePath)
	if err != nil {
		return nil, err
	}

	// If valid and no issues, no need to fix
	if validationResult.Valid && len(validationResult.Issues) == 0 {
		logger.Info("file is valid, no fixes needed", logger.String("file", filePath))
		return &FixResult{Fixed: false}, nil
	}

	// Try to fix
	return f.TryFixFile(filePath, validationResult)
}

// FixWithRegex applies a regex-based fix to a file
func (f *SimpleFixer) FixWithRegex(filePath string, pattern string, replacement string, description string) (*FixResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "failed to read file", err)
	}

	originalContent := string(content)

	// Compile regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "invalid regex pattern", err)
	}

	// Apply replacement
	fixedContent := re.ReplaceAllString(originalContent, replacement)

	result := &FixResult{
		Fixed:        fixedContent != originalContent,
		FixesApplied: []string{},
	}

	if result.Fixed {
		// Create backup
		backupPath := filePath + ".backup"
		if err := os.WriteFile(backupPath, content, 0644); err != nil {
			logger.Warn("failed to create backup", logger.Err(err))
		} else {
			result.BackupPath = backupPath
		}

		// Write fixed content
		if err := os.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
			return nil, types.NewAppError(types.ErrInternal, "failed to write fixed file", err)
		}

		result.FixesApplied = append(result.FixesApplied, description)
		logger.Info("applied regex fix", logger.String("file", filePath), logger.String("description", description))
	}

	return result, nil
}


// fixTikZIssues fixes common TikZ-related issues
func (f *SimpleFixer) fixTikZIssues(content string) (string, []string) {
	applied := []string{}
	fixed := content

	// Count tikzpicture environments
	beginTikz := strings.Count(fixed, "\\begin{tikzpicture}")
	endTikz := strings.Count(fixed, "\\end{tikzpicture}")

	// Fix 1: Wrong order of \end{figure*} and \end{tikzpicture}
	// Pattern: \end{figure*}\n\end{tikzpicture} should be \end{tikzpicture}\n\end{figure*}
	wrongOrderPattern := regexp.MustCompile(`(\\end\{figure\*?\}\s*)(\\end\{tikzpicture\})`)
	if wrongOrderPattern.MatchString(fixed) {
		fixed = wrongOrderPattern.ReplaceAllString(fixed, "$2\n$1")
		applied = append(applied, "Fixed wrong order of \\end{figure*} and \\end{tikzpicture}")
		logger.Debug("fixed wrong order of figure and tikzpicture end tags")
	}

	// Fix 2: Missing semicolon after TikZ node
	// Pattern: node content ends with } but no ; before \end{tikzpicture}
	nodeWithoutSemicolon := regexp.MustCompile(`(\})\s*(\\end\{tikzpicture\})`)
	matches := nodeWithoutSemicolon.FindAllStringSubmatchIndex(fixed, -1)
	if len(matches) > 0 {
		// Check if there's a node that needs a semicolon
		for i := len(matches) - 1; i >= 0; i-- {
			match := matches[i]
			// Look back to see if this is a node definition
			beforeMatch := fixed[:match[0]]
			if strings.Contains(beforeMatch[max(0, len(beforeMatch)-500):], "\\node") {
				// Check if there's already a semicolon
				between := fixed[match[2]:match[4]]
				if !strings.Contains(between, ";") {
					// Insert semicolon
					fixed = fixed[:match[2]] + ";" + fixed[match[2]:]
					applied = append(applied, "Added missing semicolon after TikZ node")
					logger.Debug("added missing semicolon after TikZ node")
				}
			}
		}
	}

	// Fix 3: Missing \end{tikzpicture}
	if beginTikz > endTikz {
		diff := beginTikz - endTikz
		// Find the last \end{figure*} and add \end{tikzpicture} before it
		lastFigureEnd := strings.LastIndex(fixed, "\\end{figure*}")
		if lastFigureEnd != -1 {
			// Add missing \end{tikzpicture} before \end{figure*}
			insertion := strings.Repeat("    };\n    \\end{tikzpicture}\n", diff)
			fixed = fixed[:lastFigureEnd] + insertion + fixed[lastFigureEnd:]
			applied = append(applied, fmt.Sprintf("Added %d missing \\end{tikzpicture}", diff))
			logger.Debug("added missing \\end{tikzpicture}", logger.Int("count", diff))
		}
	}

	// Fix 4: Extra closing braces at end of file
	// Pattern: \end{figure*}}}} should be \end{figure*}
	extraBracesPattern := regexp.MustCompile(`(\\end\{figure\*?\})\}+\s*$`)
	if extraBracesPattern.MatchString(fixed) {
		fixed = extraBracesPattern.ReplaceAllString(fixed, "$1\n")
		applied = append(applied, "Removed extra closing braces after \\end{figure*}")
		logger.Debug("removed extra closing braces after \\end{figure*}")
	}

	// Fix 5: Incomplete \end{figure*} (missing closing brace)
	// Pattern: \end{figure* at end of file should be \end{figure*}
	incompleteEndFigure := regexp.MustCompile(`\\end\{figure\*?\s*$`)
	if incompleteEndFigure.MatchString(fixed) {
		fixed = incompleteEndFigure.ReplaceAllString(fixed, "\\end{figure*}\n")
		applied = append(applied, "Fixed incomplete \\end{figure*}")
		logger.Debug("fixed incomplete \\end{figure*}")
	}

	return fixed, applied
}

// fixUnclosedColorCommands fixes unclosed color commands like \blue{, \red{
func (f *SimpleFixer) fixUnclosedColorCommands(content string) (string, []string) {
	applied := []string{}
	fixed := content

	// Color commands that might be unclosed
	colorCommands := []string{"\\blue", "\\red", "\\green", "\\brown", "\\purple", "\\orange", "\\cyan", "\\magenta"}

	for _, cmd := range colorCommands {
		// Find all occurrences of the color command
		cmdPattern := regexp.MustCompile(regexp.QuoteMeta(cmd) + `\{`)
		matches := cmdPattern.FindAllStringIndex(fixed, -1)

		for _, match := range matches {
			startIdx := match[1] // Position after the opening brace

			// Count braces to find if this command is properly closed
			braceCount := 1
			for i := startIdx; i < len(fixed) && braceCount > 0; i++ {
				if fixed[i] == '{' {
					braceCount++
				} else if fixed[i] == '}' {
					braceCount--
				}
			}

			// If braceCount > 0, the command is not properly closed
			if braceCount > 0 {
				// Find a good place to close it (before \end{tikzpicture} or end of line with \\)
				searchArea := fixed[startIdx:min(len(fixed), startIdx+2000)]

				// Look for \\ followed by newline as a good closing point
				closePoint := -1
				lineEndPattern := regexp.MustCompile(`\\\\\s*\n`)
				if loc := lineEndPattern.FindStringIndex(searchArea); loc != nil {
					closePoint = startIdx + loc[0] + 2 // After the \\
				}

				if closePoint > 0 && closePoint < len(fixed) {
					// Insert closing brace
					fixed = fixed[:closePoint] + "}" + fixed[closePoint:]
					applied = append(applied, fmt.Sprintf("Closed unclosed %s command", cmd))
					logger.Debug("closed unclosed color command", logger.String("command", cmd))
				}
			}
		}
	}

	return fixed, applied
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
