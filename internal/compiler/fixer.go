// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
	"latex-translator/internal/types"
)

// FixLevel represents the level of fix strategy
type FixLevel int

const (
	// FixLevelRule is the first level - fast rule-based fixes
	FixLevelRule FixLevel = iota
	// FixLevelLLM is the second level - simple LLM-based fixes
	FixLevelLLM
	// FixLevelAgent is the third level - intelligent agent-based fixes
	FixLevelAgent
)

// LaTeXFixer uses LLM to automatically fix LaTeX compilation errors.
type LaTeXFixer struct {
	apiKey       string
	apiURL       string
	model        string
	agentModel   string // Model for agent-level fixes (more capable)
	client       *http.Client
	maxRetries   int
	enableAgent  bool // Whether to enable agent-level fixes
}

// NewLaTeXFixer creates a new LaTeXFixer instance.
func NewLaTeXFixer(apiKey, apiURL, model string) *LaTeXFixer {
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1/chat/completions"
	} else if !strings.HasSuffix(apiURL, "/chat/completions") {
		apiURL = strings.TrimSuffix(apiURL, "/") + "/chat/completions"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &LaTeXFixer{
		apiKey:      apiKey,
		apiURL:      apiURL,
		model:       model,
		agentModel:  model, // Default to same model, can be overridden
		client: &http.Client{
			Timeout: 180 * time.Second, // Longer timeout for agent fixes
		},
		maxRetries:  3,
		enableAgent: true, // Enable agent fixes by default
	}
}

// NewLaTeXFixerWithAgent creates a new LaTeXFixer with agent support.
func NewLaTeXFixerWithAgent(apiKey, apiURL, model, agentModel string, enableAgent bool) *LaTeXFixer {
	fixer := NewLaTeXFixer(apiKey, apiURL, model)
	if agentModel != "" {
		fixer.agentModel = agentModel
	}
	fixer.enableAgent = enableAgent
	return fixer
}

// SetAgentModel sets the model to use for agent-level fixes.
func (f *LaTeXFixer) SetAgentModel(model string) {
	f.agentModel = model
}

// SetEnableAgent enables or disables agent-level fixes.
func (f *LaTeXFixer) SetEnableAgent(enable bool) {
	f.enableAgent = enable
}

// FixResult represents the result of a fix attempt.
type FixResult struct {
	Success     bool              `json:"success"`
	FixedFiles  map[string]string `json:"fixed_files"`  // filename -> fixed content
	Description string            `json:"description"`  // description of what was fixed
	Iterations  int               `json:"iterations"`   // number of fix iterations
	FixLevel    FixLevel          `json:"fix_level"`    // which level of fix succeeded
}

// HierarchicalFixResult represents the result of hierarchical fix attempts.
type HierarchicalFixResult struct {
	Success          bool              `json:"success"`
	FixedFiles       map[string]string `json:"fixed_files"`
	Description      string            `json:"description"`
	TotalIterations  int               `json:"total_iterations"`
	RuleFixAttempts  int               `json:"rule_fix_attempts"`
	LLMFixAttempts   int               `json:"llm_fix_attempts"`
	AgentFixAttempts int               `json:"agent_fix_attempts"`
	FinalFixLevel    FixLevel          `json:"final_fix_level"`
}

// FixCompilationErrors attempts to fix LaTeX compilation errors using LLM.
// It reads the error log, identifies the problematic files, and asks LLM to fix them.
// The process repeats until compilation succeeds or maxRetries is reached.
func (f *LaTeXFixer) FixCompilationErrors(
	texDir string,
	mainTexFile string,
	compileLog string,
	compiler *LaTeXCompiler,
	outputDir string,
) (*FixResult, error) {
	logger.Info("starting LaTeX error fix process",
		logger.String("texDir", texDir),
		logger.String("mainTexFile", mainTexFile))

	result := &FixResult{
		Success:    false,
		FixedFiles: make(map[string]string),
		Iterations: 0,
	}

	currentLog := compileLog

	for i := 0; i < f.maxRetries; i++ {
		result.Iterations = i + 1
		logger.Info("fix iteration", logger.Int("iteration", i+1))

		// Parse errors from log
		errors := parseLatexErrors(currentLog)
		if len(errors) == 0 {
			logger.Info("no errors found in log, compilation may have succeeded")
			result.Success = true
			result.Description = "No errors found"
			return result, nil
		}

		logger.Info("found LaTeX errors", logger.Int("count", len(errors)))

		// Get the problematic file content
		fileContents := make(map[string]string)
		for _, err := range errors {
			if err.File != "" && fileContents[err.File] == "" {
				filePath := filepath.Join(texDir, err.File)
				if content, readErr := os.ReadFile(filePath); readErr == nil {
					fileContents[err.File] = string(content)
				}
			}
		}

		// If no specific file identified, read the main tex file
		if len(fileContents) == 0 {
			mainPath := filepath.Join(texDir, mainTexFile)
			if content, err := os.ReadFile(mainPath); err == nil {
				fileContents[mainTexFile] = string(content)
			}
		}

		// Ask LLM to fix the errors
		fixes, description, err := f.askLLMToFix(errors, fileContents)
		if err != nil {
			logger.Error("LLM fix request failed", err)
			return result, err
		}

		if len(fixes) == 0 {
			logger.Warn("LLM returned no fixes")
			result.Description = "LLM could not determine fixes"
			return result, nil
		}

		// Apply fixes
		for filename, fixedContent := range fixes {
			filePath := filepath.Join(texDir, filename)
			if err := os.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
				logger.Error("failed to write fixed file", err, logger.String("file", filename))
				continue
			}
			result.FixedFiles[filename] = fixedContent
			logger.Info("applied fix to file", logger.String("file", filename))
		}

		result.Description = description

		// Try to compile again
		mainTexPath := filepath.Join(texDir, mainTexFile)
		compileResult, compileErr := compiler.Compile(mainTexPath, outputDir)
		
		if compileErr == nil && compileResult.Success {
			logger.Info("compilation succeeded after fix")
			result.Success = true
			return result, nil
		}

		// Update log for next iteration
		if compileResult != nil {
			currentLog = compileResult.Log
		}
	}

	logger.Warn("max fix retries reached", logger.Int("maxRetries", f.maxRetries))
	return result, nil
}

// HierarchicalFixCompilationErrors implements a hierarchical fix strategy:
// Level 1: Rule-based fixes (fast, reliable for common issues)
// Level 2: Simple LLM fixes (moderate cost, handles most errors)
// Level 3: Agent-based fixes (higher cost, handles complex errors)
// This ensures compilation success while minimizing API costs.
//
// STRATEGY IMPROVEMENT:
// - If agent is enabled and errors are complex, skip directly to agent level
// - Agent gets the original error context, not polluted by failed rule fixes
// - This allows agent to make holistic decisions about the best fix approach
func (f *LaTeXFixer) HierarchicalFixCompilationErrors(
	texDir string,
	mainTexFile string,
	compileLog string,
	compiler *LaTeXCompiler,
	outputDir string,
	progressCallback func(level FixLevel, attempt int, message string),
) (*HierarchicalFixResult, error) {
	logger.Info("starting hierarchical LaTeX error fix process",
		logger.String("texDir", texDir),
		logger.String("mainTexFile", mainTexFile),
		logger.Bool("agentEnabled", f.enableAgent))

	result := &HierarchicalFixResult{
		Success:    false,
		FixedFiles: make(map[string]string),
	}

	currentLog := compileLog
	mainTexPath := filepath.Join(texDir, mainTexFile)

	// Read current content
	mainContent, err := os.ReadFile(mainTexPath)
	if err != nil {
		return result, fmt.Errorf("failed to read main tex file: %w", err)
	}
	currentContent := string(mainContent)

	// Analyze error complexity to decide strategy
	errors := parseLatexErrors(currentLog)
	errorComplexity := analyzeErrorComplexity(errors, currentLog)
	
	logger.Info("error analysis",
		logger.Int("errorCount", len(errors)),
		logger.String("complexity", errorComplexity))

	// If errors are complex and agent is enabled, skip directly to agent
	// This gives agent the original context without rule-based pollution
	if f.enableAgent && (errorComplexity == "high" || errorComplexity == "medium") {
		logger.Info("complex errors detected, skipping to agent-based fix")
		if progressCallback != nil {
			progressCallback(FixLevelAgent, 1, "检测到复杂错误，使用 Agent 智能修复...")
		}
		return f.runAgentFix(texDir, mainTexFile, currentLog, compiler, outputDir, result, progressCallback)
	}

	// ============ Level 1: Rule-based fixes ============
	logger.Info("attempting Level 1: Rule-based fixes")
	if progressCallback != nil {
		progressCallback(FixLevelRule, 1, "尝试规则修复...")
	}

	for attempt := 1; attempt <= 2; attempt++ {
		result.RuleFixAttempts++
		result.TotalIterations++

		fixedContent, wasFixed := QuickFix(currentContent)
		if wasFixed {
			currentContent = fixedContent
			if err := os.WriteFile(mainTexPath, []byte(currentContent), 0644); err != nil {
				logger.Warn("failed to save rule-fixed file", logger.Err(err))
				continue
			}
			result.FixedFiles[mainTexFile] = currentContent
			logger.Info("applied rule-based fixes", logger.Int("attempt", attempt))

			// Try to compile
			compileResult, compileErr := compiler.Compile(mainTexPath, outputDir)
			if compileErr == nil && compileResult.Success {
				logger.Info("compilation succeeded after rule-based fix")
				result.Success = true
				result.Description = "规则修复成功"
				result.FinalFixLevel = FixLevelRule
				return result, nil
			}
			currentLog = compileResult.Log
		} else {
			logger.Debug("no rule-based fixes applicable", logger.Int("attempt", attempt))
			break
		}
	}

	// ============ Level 2: Simple LLM fixes ============
	logger.Info("attempting Level 2: Simple LLM fixes")
	if progressCallback != nil {
		progressCallback(FixLevelLLM, 1, "尝试 LLM 修复...")
	}

	for attempt := 1; attempt <= f.maxRetries; attempt++ {
		result.LLMFixAttempts++
		result.TotalIterations++

		if progressCallback != nil {
			progressCallback(FixLevelLLM, attempt, fmt.Sprintf("LLM 修复尝试 %d/%d...", attempt, f.maxRetries))
		}

		// Parse errors from log
		errors := parseLatexErrors(currentLog)
		if len(errors) == 0 {
			logger.Info("no errors found in log after LLM fix")
			result.Success = true
			result.Description = "LLM 修复成功"
			result.FinalFixLevel = FixLevelLLM
			return result, nil
		}

		// Get file contents for fixing
		fileContents := f.collectFileContents(texDir, mainTexFile, errors)

		// Ask LLM to fix
		fixes, description, err := f.askLLMToFix(errors, fileContents)
		if err != nil {
			logger.Warn("LLM fix request failed", logger.Err(err))
			continue
		}

		if len(fixes) == 0 {
			logger.Warn("LLM returned no fixes")
			continue
		}

		// Apply fixes
		for filename, fixedContent := range fixes {
			filePath := filepath.Join(texDir, filename)
			if err := os.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
				logger.Warn("failed to write fixed file", logger.Err(err), logger.String("file", filename))
				continue
			}
			result.FixedFiles[filename] = fixedContent
			if filename == mainTexFile {
				currentContent = fixedContent
			}
		}
		result.Description = description

		// Try to compile
		compileResult, compileErr := compiler.Compile(mainTexPath, outputDir)
		if compileErr == nil && compileResult.Success {
			logger.Info("compilation succeeded after LLM fix")
			result.Success = true
			result.FinalFixLevel = FixLevelLLM
			return result, nil
		}
		currentLog = compileResult.Log
	}

	// ============ Level 3: Agent-based fixes ============
	if !f.enableAgent {
		logger.Info("agent fixes disabled, stopping at LLM level")
		result.Description = "LLM 修复未能解决所有问题，Agent 修复已禁用"
		return result, nil
	}

	// Use the extracted runAgentFix function for cleaner code
	return f.runAgentFix(texDir, mainTexFile, currentLog, compiler, outputDir, result, progressCallback)
}

// collectFileContents collects file contents for the files mentioned in errors.
func (f *LaTeXFixer) collectFileContents(texDir, mainTexFile string, errors []LaTeXError) map[string]string {
	fileContents := make(map[string]string)

	for _, err := range errors {
		if err.File != "" && fileContents[err.File] == "" {
			filePath := filepath.Join(texDir, err.File)
			if content, readErr := os.ReadFile(filePath); readErr == nil {
				fileContents[err.File] = string(content)
			}
		}
	}

	// Always include main tex file
	if fileContents[mainTexFile] == "" {
		mainPath := filepath.Join(texDir, mainTexFile)
		if content, err := os.ReadFile(mainPath); err == nil {
			fileContents[mainTexFile] = string(content)
		}
	}

	return fileContents
}

// collectAllRelatedFiles collects all tex files in the directory for comprehensive analysis.
func (f *LaTeXFixer) collectAllRelatedFiles(texDir, mainTexFile string) map[string]string {
	fileContents := make(map[string]string)

	// Read main file first
	mainPath := filepath.Join(texDir, mainTexFile)
	if content, err := os.ReadFile(mainPath); err == nil {
		fileContents[mainTexFile] = string(content)
	}

	// Find all \input and \include references
	if mainContent, ok := fileContents[mainTexFile]; ok {
		inputPattern := regexp.MustCompile(`\\(?:input|include)\{([^}]+)\}`)
		matches := inputPattern.FindAllStringSubmatch(mainContent, -1)
		for _, match := range matches {
			if len(match) > 1 {
				inputFile := match[1]
				if !strings.HasSuffix(inputFile, ".tex") {
					inputFile += ".tex"
				}
				if fileContents[inputFile] == "" {
					inputPath := filepath.Join(texDir, inputFile)
					if content, err := os.ReadFile(inputPath); err == nil {
						fileContents[inputFile] = string(content)
					}
				}
			}
		}
	}

	// Also check for files in subdirectories (e.g., sec/*.tex)
	filepath.Walk(texDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tex") {
			relPath, _ := filepath.Rel(texDir, path)
			if fileContents[relPath] == "" {
				if content, err := os.ReadFile(path); err == nil {
					// Limit file size to avoid token overflow
					if len(content) < 50000 {
						fileContents[relPath] = string(content)
					}
				}
			}
		}
		return nil
	})

	return fileContents
}

// askAgentToFix uses a more sophisticated prompt for complex error fixing.
// It provides comprehensive context and asks for detailed analysis.
func (f *LaTeXFixer) askAgentToFix(errors []LaTeXError, fileContents map[string]string, compileLog string) (map[string]string, string, error) {
	systemPrompt := `You are an expert LaTeX debugging agent. Your task is to analyze and fix complex LaTeX compilation errors that simpler fixes couldn't resolve.

ANALYSIS APPROACH:
1. First, understand the document structure by examining all provided files
2. Identify the root cause of each error, not just the symptoms
3. Consider interactions between files (e.g., \input, \include)
4. Check for package conflicts, missing dependencies, or encoding issues
5. Look for translation artifacts that may have corrupted LaTeX commands

COMMON COMPLEX ISSUES:
- Nested environment mismatches across multiple files
- Package conflicts (especially with CJK/ctex packages)
- Encoding issues with Chinese characters
- Corrupted LaTeX commands from translation
- Missing or incorrect package options
- Cross-reference issues between files

FIX STRATEGY:
1. Make minimal changes to fix the errors
2. Preserve all content and translations
3. Ensure Chinese text support is maintained
4. Fix all related issues, not just the first error
5. Consider the document as a whole, not just individual files

OUTPUT FORMAT (JSON only):
{
  "analysis": "Brief analysis of the root cause",
  "fixes": {
    "filename.tex": "full fixed content of the file"
  },
  "description": "Summary of all fixes applied"
}

If you cannot fix the error, return:
{
  "analysis": "Explanation of why the error cannot be fixed",
  "fixes": {},
  "description": "Suggested manual intervention"
}`

	// Build comprehensive user prompt
	var userPromptBuilder strings.Builder
	userPromptBuilder.WriteString("Please analyze and fix the following LaTeX compilation errors.\n\n")

	// Add error summary
	userPromptBuilder.WriteString("=== COMPILATION ERRORS ===\n")
	for i, err := range errors {
		userPromptBuilder.WriteString(fmt.Sprintf("\nError %d:\n", i+1))
		userPromptBuilder.WriteString(fmt.Sprintf("File: %s\n", err.File))
		userPromptBuilder.WriteString(fmt.Sprintf("Line: %d\n", err.Line))
		userPromptBuilder.WriteString(fmt.Sprintf("Message: %s\n", err.Message))
		if err.Context != "" {
			userPromptBuilder.WriteString(fmt.Sprintf("Context:\n%s\n", err.Context))
		}
	}

	// Add relevant portion of compile log
	userPromptBuilder.WriteString("\n=== COMPILATION LOG (relevant portion) ===\n")
	logExcerpt := extractRelevantLogPortion(compileLog, 3000)
	userPromptBuilder.WriteString(logExcerpt)

	// Add all file contents
	userPromptBuilder.WriteString("\n\n=== FILE CONTENTS ===\n")
	for filename, content := range fileContents {
		userPromptBuilder.WriteString(fmt.Sprintf("\n--- %s ---\n", filename))
		// Truncate very large files
		if len(content) > 20000 {
			userPromptBuilder.WriteString("[File truncated for size]\n")
			userPromptBuilder.WriteString(content[:10000])
			userPromptBuilder.WriteString("\n...[middle truncated]...\n")
			userPromptBuilder.WriteString(content[len(content)-10000:])
		} else {
			userPromptBuilder.WriteString(content)
		}
	}

	userPromptBuilder.WriteString("\n\nPlease provide your analysis and fixes in JSON format.")

	// Make API request with agent model
	reqBody := map[string]interface{}{
		"model": f.agentModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPromptBuilder.String()},
		},
		"max_tokens": 8192, // More tokens for comprehensive fixes
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, f.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	logger.Info("sending agent fix request", logger.String("model", f.agentModel))

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse API response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, "", fmt.Errorf("API returned no choices")
	}

	// Extract JSON from response
	content := apiResp.Choices[0].Message.Content
	content = extractJSON(content)

	// Parse the agent fix response
	var agentResp struct {
		Analysis    string            `json:"analysis"`
		Fixes       map[string]string `json:"fixes"`
		Description string            `json:"description"`
	}

	if err := json.Unmarshal([]byte(content), &agentResp); err != nil {
		logger.Error("failed to parse agent response", err, logger.String("content", content[:min(500, len(content))]))
		return nil, "", fmt.Errorf("failed to parse agent response: %w", err)
	}

	logger.Info("agent analysis complete", logger.String("analysis", agentResp.Analysis))

	return agentResp.Fixes, agentResp.Description, nil
}

// extractRelevantLogPortion extracts the most relevant portion of the compile log.
func extractRelevantLogPortion(log string, maxLen int) string {
	if len(log) <= maxLen {
		return log
	}

	// Try to find error sections
	lines := strings.Split(log, "\n")
	var relevantLines []string
	inErrorSection := false

	for i, line := range lines {
		// Start of error section
		if strings.HasPrefix(line, "!") || strings.Contains(line, "Error") {
			inErrorSection = true
		}

		if inErrorSection {
			relevantLines = append(relevantLines, line)
			// Include some context after error
			if len(relevantLines) > 50 && !strings.HasPrefix(line, "!") && !strings.Contains(line, "Error") {
				inErrorSection = false
			}
		}

		// Also include lines with file references
		if strings.Contains(line, ".tex") && len(relevantLines) < 100 {
			// Include surrounding context
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 3
			if end > len(lines) {
				end = len(lines)
			}
			for j := start; j < end; j++ {
				if j < len(lines) {
					relevantLines = append(relevantLines, lines[j])
				}
			}
		}
	}

	result := strings.Join(relevantLines, "\n")
	if len(result) > maxLen {
		return result[:maxLen] + "\n...[truncated]"
	}
	if len(result) == 0 {
		// Fallback: return last portion of log
		return log[len(log)-maxLen:]
	}
	return result
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LaTeXError represents a parsed LaTeX error.
type LaTeXError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Context string `json:"context"` // surrounding lines
}

// parseLatexErrors extracts error information from a LaTeX log.
func parseLatexErrors(log string) []LaTeXError {
	var errors []LaTeXError

	// Pattern for LaTeX errors: ! Error message
	errorPattern := regexp.MustCompile(`(?m)^!\s*(.+)$`)
	// Pattern for file:line info: l.123 or (./file.tex
	linePattern := regexp.MustCompile(`(?m)^l\.(\d+)\s*(.*)$`)
	filePattern := regexp.MustCompile(`\(\.?/?([^()]+\.tex)`)

	lines := strings.Split(log, "\n")
	var currentError *LaTeXError
	var currentFile string

	for i, line := range lines {
		// Check for file references
		if matches := filePattern.FindStringSubmatch(line); len(matches) > 1 {
			currentFile = matches[1]
		}

		// Check for error start
		if matches := errorPattern.FindStringSubmatch(line); len(matches) > 1 {
			if currentError != nil {
				errors = append(errors, *currentError)
			}
			currentError = &LaTeXError{
				File:    currentFile,
				Message: matches[1],
			}
			// Get context (surrounding lines)
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 5
			if end > len(lines) {
				end = len(lines)
			}
			currentError.Context = strings.Join(lines[start:end], "\n")
		}

		// Check for line number
		if currentError != nil {
			if matches := linePattern.FindStringSubmatch(line); len(matches) > 1 {
				fmt.Sscanf(matches[1], "%d", &currentError.Line)
			}
		}
	}

	if currentError != nil {
		errors = append(errors, *currentError)
	}

	return errors
}

// askLLMToFix sends the errors and file contents to LLM and gets fixes.
func (f *LaTeXFixer) askLLMToFix(errors []LaTeXError, fileContents map[string]string) (map[string]string, string, error) {
	// Build the prompt
	systemPrompt := `You are a LaTeX expert. Your task is to fix LaTeX compilation errors.

IMPORTANT RULES:
1. Analyze the error messages and file contents carefully.
2. Return ONLY the fixed file contents in the specified JSON format.
3. Do NOT add any explanations outside the JSON.
4. Preserve all content - only fix the errors, don't remove or change other content.
5. Common fixes include:
   - Adding missing \end{...} or \begin{...} commands
   - Fixing mismatched braces
   - Ensuring \item commands are on separate lines from \end{itemize}
   - Fixing "Lonely \item" errors by ensuring proper list environment structure
   - Adding missing packages

OUTPUT FORMAT (JSON only):
{
  "fixes": {
    "filename.tex": "full fixed content of the file"
  },
  "description": "brief description of what was fixed"
}

If you cannot fix the error, return:
{
  "fixes": {},
  "description": "explanation of why it cannot be fixed"
}`

	// Build user prompt with errors and file contents
	var userPromptBuilder strings.Builder
	userPromptBuilder.WriteString("Please fix the following LaTeX compilation errors:\n\n")
	
	userPromptBuilder.WriteString("=== ERRORS ===\n")
	for i, err := range errors {
		userPromptBuilder.WriteString(fmt.Sprintf("\nError %d:\n", i+1))
		userPromptBuilder.WriteString(fmt.Sprintf("File: %s\n", err.File))
		userPromptBuilder.WriteString(fmt.Sprintf("Line: %d\n", err.Line))
		userPromptBuilder.WriteString(fmt.Sprintf("Message: %s\n", err.Message))
		userPromptBuilder.WriteString(fmt.Sprintf("Context:\n%s\n", err.Context))
	}

	userPromptBuilder.WriteString("\n=== FILE CONTENTS ===\n")
	for filename, content := range fileContents {
		userPromptBuilder.WriteString(fmt.Sprintf("\n--- %s ---\n", filename))
		// Limit content length to avoid token limits
		if len(content) > 15000 {
			// Find the error line and include surrounding context
			userPromptBuilder.WriteString("[File truncated, showing relevant sections]\n")
			userPromptBuilder.WriteString(content[:7500])
			userPromptBuilder.WriteString("\n...[truncated]...\n")
			userPromptBuilder.WriteString(content[len(content)-7500:])
		} else {
			userPromptBuilder.WriteString(content)
		}
	}

	userPromptBuilder.WriteString("\n\nPlease provide the fixed file contents in JSON format.")

	// Make API request
	reqBody := map[string]interface{}{
		"model": f.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPromptBuilder.String()},
		},
		"max_tokens": 8192,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, f.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse API response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, "", fmt.Errorf("API returned no choices")
	}

	// Extract JSON from response (it might be wrapped in markdown code blocks)
	content := apiResp.Choices[0].Message.Content
	content = extractJSON(content)

	// Parse the fix response
	var fixResp struct {
		Fixes       map[string]string `json:"fixes"`
		Description string            `json:"description"`
	}

	if err := json.Unmarshal([]byte(content), &fixResp); err != nil {
		logger.Error("failed to parse fix response", err, logger.String("content", content))
		return nil, "", fmt.Errorf("failed to parse fix response: %w", err)
	}

	return fixResp.Fixes, fixResp.Description, nil
}

// extractJSON extracts JSON from a string that might be wrapped in markdown code blocks.
func extractJSON(content string) string {
	// Try to find JSON in code blocks
	jsonBlockPattern := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(\\{.*?\\})\\s*```")
	if matches := jsonBlockPattern.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	// Try to find raw JSON
	jsonPattern := regexp.MustCompile(`(?s)\{.*\}`)
	if matches := jsonPattern.FindString(content); matches != "" {
		return matches
	}

	return content
}

// QuickFix applies common LaTeX fixes without using LLM.
// This is faster and can handle simple, well-known issues.
// Returns the fixed content and a boolean indicating if any fixes were applied.
func QuickFix(content string) (string, bool) {
	result := content
	anyFixed := false

	// Rule 1: Fix translated LaTeX commands (Chinese -> English)
	// These are common LaTeX commands that might get translated to Chinese
	translatedCommands := map[string]string{
		`\\引用`:   `\cite`,
		`\\参考`:   `\ref`,
		`\\标签`:   `\label`,
		`\\章节`:   `\section`,
		`\\小节`:   `\subsection`,
		`\\段落`:   `\paragraph`,
		`\\图`:     `\figure`,
		`\\表`:     `\table`,
		`\\公式`:   `\equation`,
		`\\开始`:   `\begin`,
		`\\结束`:   `\end`,
		`\\文本`:   `\text`,
		`\\粗体`:   `\textbf`,
		`\\斜体`:   `\textit`,
		`\\下划线`: `\underline`,
		`\\脚注`:   `\footnote`,
		`\\项目`:   `\item`,
	}

	for pattern, replacement := range translatedCommands {
		re := regexp.MustCompile(pattern)
		if re.MatchString(result) {
			result = re.ReplaceAllString(result, replacement)
			anyFixed = true
			logger.Debug("rule-based fix: replaced translated command",
				logger.String("pattern", pattern),
				logger.String("replacement", replacement))
		}
	}

	// Rule 2: \end{itemize}\item -> \end{itemize}\n\item
	endItemizeItemPattern := regexp.MustCompile(`(\\end\{itemize\})\s*(\\item\b)`)
	if endItemizeItemPattern.MatchString(result) {
		result = endItemizeItemPattern.ReplaceAllString(result, "$1\n$2")
		anyFixed = true
	}

	// Rule 3: \end{enumerate}\item -> \end{enumerate}\n\item
	endEnumerateItemPattern := regexp.MustCompile(`(\\end\{enumerate\})\s*(\\item\b)`)
	if endEnumerateItemPattern.MatchString(result) {
		result = endEnumerateItemPattern.ReplaceAllString(result, "$1\n$2")
		anyFixed = true
	}

	// Rule 4: \end{description}\item -> \end{description}\n\item
	endDescriptionItemPattern := regexp.MustCompile(`(\\end\{description\})\s*(\\item\b)`)
	if endDescriptionItemPattern.MatchString(result) {
		result = endDescriptionItemPattern.ReplaceAllString(result, "$1\n$2")
		anyFixed = true
	}

	// Rule 5: \end{...}\begin{...} -> \end{...}\n\begin{...}
	endBeginPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\begin\{[^}]+\})`)
	if endBeginPattern.MatchString(result) {
		result = endBeginPattern.ReplaceAllString(result, "$1\n$2")
		anyFixed = true
	}

	// Rule 6: \end{...}\section -> \end{...}\n\n\section
	endSectionPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\b)`)
	if endSectionPattern.MatchString(result) {
		result = endSectionPattern.ReplaceAllString(result, "$1\n\n$2")
		anyFixed = true
	}

	// Rule 7: text\begin{itemize} -> text\n\begin{itemize}
	textBeginListPattern := regexp.MustCompile(`([^\n\s])\s*(\\begin\{(?:itemize|enumerate|description)\})`)
	if textBeginListPattern.MatchString(result) {
		result = textBeginListPattern.ReplaceAllString(result, "$1\n$2")
		anyFixed = true
	}

	// Rule 8: \end{itemize}text -> \end{itemize}\ntext
	endListTextPattern := regexp.MustCompile(`(\\end\{(?:itemize|enumerate|description)\})([^\n\\])`)
	if endListTextPattern.MatchString(result) {
		result = endListTextPattern.ReplaceAllString(result, "$1\n$2")
		anyFixed = true
	}

	// Rule 9: Fix extra closing braces after \end{environment} FIRST
	// Pattern: \end{figure*}}}} or \end{table}}} etc.
	// This is a common error from translation where extra braces are added
	// We do this BEFORE the general brace counting to avoid conflicts
	// NOTE: We need to be careful not to remove braces that are part of
	// valid LaTeX structures like \newcommand{\foo}{\begin{...}\end{...}}
	extraBracesPattern := regexp.MustCompile(`(\\end\{[^}]+\})(\}+)`)
	if extraBracesPattern.MatchString(result) {
		matches := extraBracesPattern.FindAllStringSubmatchIndex(result, -1)
		// Process matches in reverse order to preserve indices
		for i := len(matches) - 1; i >= 0; i-- {
			matchIndices := matches[i]
			if len(matchIndices) < 6 {
				continue
			}
			
			fullMatchStart := matchIndices[0]
			fullMatchEnd := matchIndices[1]
			endCmdEnd := matchIndices[3]
			extraBracesCount := fullMatchEnd - endCmdEnd
			
			// Find the matching \begin for this \end
			endEnvMatch := regexp.MustCompile(`\\end\{([^}]+)\}`).FindStringSubmatch(result[matchIndices[2]:matchIndices[3]])
			if len(endEnvMatch) < 2 {
				continue
			}
			envName := endEnvMatch[1]
			
			// Search backwards for the matching \begin
			beginPat := regexp.MustCompile(`\\begin\{` + regexp.QuoteMeta(envName) + `\}`)
			searchStart := fullMatchStart - 5000
			if searchStart < 0 {
				searchStart = 0
			}
			searchRegion := result[searchStart:fullMatchStart]
			beginMatches := beginPat.FindAllStringIndex(searchRegion, -1)
			
			if len(beginMatches) == 0 {
				// No matching \begin found, this is likely an error - remove extra braces
				result = result[:endCmdEnd] + result[fullMatchEnd:]
				anyFixed = true
				continue
			}
			
			// Get the position of the last (innermost) matching \begin
			lastBeginIdx := searchStart + beginMatches[len(beginMatches)-1][0]
			
			// Check if we're inside a command definition by looking for \newcommand etc.
			// before the \begin that hasn't been closed
			preBeginStart := lastBeginIdx - 200
			if preBeginStart < 0 {
				preBeginStart = 0
			}
			preBeginRegion := result[preBeginStart:lastBeginIdx]
			
			// Look for command definition patterns that open a brace
			// Find the last occurrence of \newcommand, \def, etc.
			cmdDefPatterns := []string{"\\newcommand", "\\renewcommand", "\\providecommand", "\\def", "\\gdef", "\\edef", "\\xdef"}
			cmdDefIdx := -1
			for _, pat := range cmdDefPatterns {
				idx := strings.LastIndex(preBeginRegion, pat)
				if idx > cmdDefIdx {
					cmdDefIdx = idx
				}
			}
			
			bracesToKeep := 0
			if cmdDefIdx >= 0 {
				// Found a command definition before \begin
				// Count braces from the command definition to \end{...}
				cmdDefStart := preBeginStart + cmdDefIdx
				regionFromCmdDef := result[cmdDefStart:fullMatchStart]
				
				openBraces := strings.Count(regionFromCmdDef, "{") - strings.Count(regionFromCmdDef, "\\{")
				closeBraces := strings.Count(regionFromCmdDef, "}") - strings.Count(regionFromCmdDef, "\\}")
				unclosedBraces := openBraces - closeBraces
				
				if unclosedBraces > 0 {
					// There are unclosed braces from the command definition
					// Keep braces that are needed to close the definition
					bracesToKeep = unclosedBraces
					if bracesToKeep > extraBracesCount {
						bracesToKeep = extraBracesCount
					}
				}
			}
			
			// Only fix if there are truly extra braces
			if extraBracesCount > bracesToKeep {
				newBraces := strings.Repeat("}", bracesToKeep)
				result = result[:endCmdEnd] + newBraces + result[fullMatchEnd:]
				anyFixed = true
				logger.Debug("rule-based fix: removed extra closing braces after \\end",
					logger.Int("removed", extraBracesCount-bracesToKeep),
					logger.Int("kept", bracesToKeep))
			}
		}
	}

	// Rule 10: Fix unmatched braces (general case)
	openBraces := strings.Count(result, "{") - strings.Count(result, "\\{")
	closeBraces := strings.Count(result, "}") - strings.Count(result, "\\}")
	if openBraces > closeBraces {
		// Add missing closing braces BEFORE \end{document} if it exists
		missingBraces := strings.Repeat("}", openBraces-closeBraces)
		endDocIdx := strings.LastIndex(result, "\\end{document}")
		if endDocIdx != -1 {
			// Insert before \end{document}
			result = result[:endDocIdx] + missingBraces + "\n" + result[endDocIdx:]
		} else {
			// No \end{document}, append at end
			result += missingBraces
		}
		anyFixed = true
		logger.Debug("rule-based fix: added missing closing braces",
			logger.Int("count", openBraces-closeBraces))
	} else if closeBraces > openBraces {
		// Only remove extra braces if they weren't already handled by Rule 9
		// and if the count is small (to avoid breaking valid nested structures)
		diff := closeBraces - openBraces
		if diff <= 3 {
			for i := 0; i < diff; i++ {
				lastBrace := strings.LastIndex(result, "}")
				if lastBrace > 0 && (lastBrace == 0 || result[lastBrace-1] != '\\') {
					result = result[:lastBrace] + result[lastBrace+1:]
					anyFixed = true
				}
			}
			if anyFixed {
				logger.Debug("rule-based fix: removed extra closing braces",
					logger.Int("count", diff))
			}
		}
	}

	// Rule 11: Fix \begin{} without matching \end{}
	beginPattern := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endPattern := regexp.MustCompile(`\\end\{([^}]+)\}`)
	beginMatches := beginPattern.FindAllStringSubmatch(result, -1)
	endMatches := endPattern.FindAllStringSubmatch(result, -1)

	beginCounts := make(map[string]int)
	endCounts := make(map[string]int)
	for _, m := range beginMatches {
		beginCounts[m[1]]++
	}
	for _, m := range endMatches {
		endCounts[m[1]]++
	}

	for env, count := range beginCounts {
		if endCounts[env] < count {
			for i := 0; i < count-endCounts[env]; i++ {
				result += fmt.Sprintf("\n\\end{%s}", env)
				anyFixed = true
				logger.Debug("rule-based fix: added missing \\end", logger.String("env", env))
			}
		}
	}

	// Rule 12: Fix "Lonely \item" - when \item appears after \end{itemize/enumerate/description}
	// This happens when LLM translation breaks the list structure
	// Pattern: \end{itemize}\n\item ... \end{itemize} -> move the \item before the first \end{itemize}
	// Pattern: \end{itemize}\n\item ... \end{itemize} -> move the \item before the first \end{itemize}
	result, lonelyItemFixed := fixLonelyItems(result)
	if lonelyItemFixed {
		anyFixed = true
	}

	// Rule 13: Fix \bibliography and \bibliographystyle order
	// The correct order is \bibliographystyle{...} before \bibliography{...}
	// Many templates have this wrong, causing bibtex to fail
	// Rule 13: Fix \bibliography and \bibliographystyle order
	// The correct order is \bibliographystyle{...} before \bibliography{...}
	// Many templates have this wrong, causing bibtex to fail
	result, bibOrderFixed := fixBibliographyOrder(result)
	if bibOrderFixed {
		anyFixed = true
	}

	// Rule 14: Fix breakurl package compatibility with xelatex
	// The breakurl package uses \pdf@box which is not available in xelatex
	// We add a workaround by defining \pdf@box as a dummy box
	result, breakurlFixed := fixBreakurlCompatibility(result)
	if breakurlFixed {
		anyFixed = true
	}

	// Rule 14b: Fix subfigure/subcaption package conflict
	// These two packages are incompatible - remove subfigure if subcaption is present
	result, subfigureFixed := fixSubfigureConflict(result)
	if subfigureFixed {
		anyFixed = true
	}

	// Rule 14c: Fix missing closing braces in \newenvironment definitions
	// This is a common LLM translation artifact
	result, newenvFixed := fixNewenvironmentBraces(result)
	if newenvFixed {
		anyFixed = true
	}

	// Rule 15: Fix trailing garbage braces at end of file
	// This is a common LLM translation artifact where extra braces appear at the very end
	// Pattern: content followed by }}}}} at the end of file
	result, trailingBracesFixed := fixTrailingGarbageBraces(result)
	if trailingBracesFixed {
		anyFixed = true
	}

	// Rule 16: Fix \end{document}} with extra braces
	// Pattern: \end{document}}} should be \end{document}
	// Also handles case where } is on a new line after \end{document}
	endDocExtraBracesPattern := regexp.MustCompile(`(\\end\{document\})\s*(\}+)\s*$`)
	if endDocExtraBracesPattern.MatchString(result) {
		result = endDocExtraBracesPattern.ReplaceAllString(result, "$1\n")
		anyFixed = true
		logger.Debug("rule-based fix: removed extra braces after \\end{document}")
	}

	// Rule 16b: Fix nested/duplicate \resizebox commands
	// Pattern: \resizebox{\textwidth}{!}{\resizebox{\textwidth}{!}{...
	// This is a common LLM translation artifact where resizebox is duplicated
	result, nestedResizeboxFixed := fixNestedResizebox(result)
	if nestedResizeboxFixed {
		anyFixed = true
	}

	// Rule 17: Fix missing closing brace for \resizebox in tables
	// Pattern: \resizebox{\columnwidth}{!}{ ... \end{tabular} should have } after \end{tabular}
	// This is a common issue in arXiv source files
	result, resizeboxFixed := fixResizeboxClosingBrace(result)
	if resizeboxFixed {
		anyFixed = true
	}

	// Rule 18: Fix orphaned \begin{itemize/enumerate/description} followed by commented content
	// Pattern: \begin{itemize} followed by lines starting with % and no matching \end{itemize}
	// This happens when authors comment out list content but forget to comment out \begin
	result, orphanedListFixed := fixOrphanedListEnvironments(result)
	if orphanedListFixed {
		anyFixed = true
	}

	// Rule 19: Fix duplicate image path in \includegraphics when \graphicspath is set
	// Pattern: \graphicspath{ {./images/} } combined with \includegraphics{images/file.jpg}
	// This causes LaTeX to look for ./images/images/file.jpg which doesn't exist
	// Fix: Remove the redundant path prefix from \includegraphics
	result, graphicsPathFixed := fixDuplicateGraphicsPath(result)
	if graphicsPathFixed {
		anyFixed = true
	}

	// Rule 20: Fix document end issues (extra braces, incomplete \end{document}, duplicates)
	// Pattern: }}} \end{document or \end{document \end{document}
	// This is a common issue in arXiv source files with corrupted endings
	result, docEndFixed := fixDocumentEndIssues(result)
	if docEndFixed {
		anyFixed = true
	}

	// Rule 21: Fix \newreptheorem definition missing closing braces
	// This is a common bug in arXiv papers using the rep@theorem pattern
	result, newreptheoremFixed := fixNewreptheoremDefinition(result)
	if newreptheoremFixed {
		anyFixed = true
	}

	// Rule 22: Fix extra closing brace after nested \end{tabular}
	// Pattern: \end{tabular}} & should be \end{tabular} &
	// This is common in tables with nested tabular environments in cells
	result, nestedTabularFixed := fixNestedTabularExtraBrace(result)
	if nestedTabularFixed {
		anyFixed = true
	}

	// Rule 23: Fix table layout issues (overflow, overlap)
	// This includes reducing column spacing, applying smart scaling, and improving float placement
	result, tableLayoutFixed := FixTableLayout(result)
	if tableLayoutFixed {
		anyFixed = true
	}

	// Rule 24: Fix figure layout issues (overflow, overlap)
	// This includes fixing environment mismatches and adjusting widths
	result, figureLayoutFixed := FixFigureLayout(result)
	if figureLayoutFixed {
		anyFixed = true
	}

	// Rule 25: Convert extremely wide tables (14+ columns) to table*
	// This prevents overlap with surrounding text
	result, wideTableFixed := ConvertWideTableToTableStar(result)
	if wideTableFixed {
		anyFixed = true
	}

	// Rule 26: Fix unbalanced braces at end of file
	// This is a common issue when LLM translation truncates content
	// We add missing closing braces before the last \end{...} or at the end
	result, unbalancedFixed := fixUnbalancedBracesAtEnd(result)
	if unbalancedFixed {
		anyFixed = true
	}

	return result, anyFixed
}

// fixUnbalancedBracesAtEnd fixes unbalanced braces by adding missing closing braces at the end.
// This is a common issue when LLM translation truncates content or loses braces.
// The function counts open and close braces (excluding escaped ones) and adds missing } at the end.
func fixUnbalancedBracesAtEnd(content string) (string, bool) {
	// Count braces (excluding escaped ones like \{ and \})
	openBraces := 0
	closeBraces := 0
	
	for i := 0; i < len(content); i++ {
		if content[i] == '{' && (i == 0 || content[i-1] != '\\') {
			openBraces++
		}
		if content[i] == '}' && (i == 0 || content[i-1] != '\\') {
			closeBraces++
		}
	}
	
	diff := openBraces - closeBraces
	
	if diff <= 0 {
		// No missing closing braces (or more closing than opening, handled elsewhere)
		return content, false
	}
	
	// Only fix if the difference is reasonable (< 50 missing braces)
	// Larger differences likely indicate a more serious structural problem
	if diff > 50 {
		logger.Warn("too many missing closing braces, skipping auto-fix",
			logger.Int("openBraces", openBraces),
			logger.Int("closeBraces", closeBraces),
			logger.Int("diff", diff))
		return content, false
	}
	
	// Find the best position to insert missing braces
	// Priority: before \end{document}, before last \end{table/figure}, or at end
	result := content
	missingBraces := strings.Repeat("}", diff)
	
	// Try to find \end{document}
	endDocIdx := strings.LastIndex(result, "\\end{document}")
	if endDocIdx != -1 {
		// Insert before \end{document}
		result = result[:endDocIdx] + missingBraces + "\n" + result[endDocIdx:]
		logger.Debug("rule-based fix: added missing closing braces before \\end{document}",
			logger.Int("count", diff))
		return result, true
	}
	
	// Try to find last \end{table} or \end{figure} or \end{table*} or \end{figure*}
	lastEndEnvIdx := -1
	for _, env := range []string{"\\end{table*}", "\\end{figure*}", "\\end{table}", "\\end{figure}"} {
		idx := strings.LastIndex(result, env)
		if idx > lastEndEnvIdx {
			lastEndEnvIdx = idx + len(env)
		}
	}
	
	if lastEndEnvIdx > 0 {
		// Insert after the last table/figure environment
		result = result[:lastEndEnvIdx] + "\n" + missingBraces + result[lastEndEnvIdx:]
		logger.Debug("rule-based fix: added missing closing braces after last table/figure",
			logger.Int("count", diff))
		return result, true
	}
	
	// Fallback: append at the end
	result = result + "\n" + missingBraces
	logger.Debug("rule-based fix: added missing closing braces at end of file",
		logger.Int("count", diff))
	return result, true
}

// fixNestedResizebox removes duplicate/nested \resizebox commands.
// This is a common LLM translation artifact where \resizebox is duplicated multiple times.
// Pattern: \resizebox{\textwidth}{!}{\resizebox{\textwidth}{!}{... -> \resizebox{\textwidth}{!}{...
func fixNestedResizebox(content string) (string, bool) {
	// Pattern to match nested resizebox: \resizebox{...}{...}{\resizebox{...}{...}{
	// We want to keep only the outermost one
	nestedPattern := regexp.MustCompile(`(\\resizebox\s*\{[^}]+\}\s*\{[^}]+\}\s*\{%?\s*\n?\s*)(\\resizebox\s*\{[^}]+\}\s*\{[^}]+\}\s*\{%?\s*\n?\s*)+`)
	
	if !nestedPattern.MatchString(content) {
		return content, false
	}
	
	// Replace nested resizebox with single resizebox
	result := nestedPattern.ReplaceAllStringFunc(content, func(match string) string {
		// Count how many nested resizebox we have
		count := strings.Count(match, "\\resizebox")
		if count <= 1 {
			return match
		}
		
		// Keep only the first \resizebox{...}{...}{
		firstResizebox := regexp.MustCompile(`^\\resizebox\s*\{[^}]+\}\s*\{[^}]+\}\s*\{%?\s*\n?\s*`)
		firstMatch := firstResizebox.FindString(match)
		if firstMatch == "" {
			return match
		}
		
		logger.Debug("rule-based fix: removed nested resizebox",
			logger.Int("nestedCount", count-1))
		return firstMatch
	})
	
	return result, result != content
}

// fixTrailingGarbageBraces removes trailing garbage braces at the end of a file.
// This is a common LLM translation artifact where extra braces appear at the very end.
// It handles patterns like:
// - \balance\n}}}}}}} at end of file
// - \end{section}\n}}}} at end of file
// - Any content followed by multiple } at the end
func fixTrailingGarbageBraces(content string) (string, bool) {
	// Pattern: multiple closing braces at the end of file (3 or more)
	// that are not part of a valid LaTeX command
	trailingBracesPattern := regexp.MustCompile(`(\n\s*|\s+)(\}{3,})\s*$`)
	
	if trailingBracesPattern.MatchString(content) {
		result := trailingBracesPattern.ReplaceAllString(content, "$1")
		logger.Debug("rule-based fix: removed trailing garbage braces at end of file")
		return result, true
	}
	
	// Also check for braces right after \balance or similar commands at end of file
	balanceBracesPattern := regexp.MustCompile(`(\\balance)\s*(\}{2,})\s*$`)
	if balanceBracesPattern.MatchString(content) {
		result := balanceBracesPattern.ReplaceAllString(content, "$1\n")
		logger.Debug("rule-based fix: removed garbage braces after \\balance")
		return result, true
	}
	
	// Check for braces after \end{...} at end of file (more than 1 extra brace)
	endEnvBracesPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\}{2,})\s*$`)
	if endEnvBracesPattern.MatchString(content) {
		result := endEnvBracesPattern.ReplaceAllString(content, "$1\n")
		logger.Debug("rule-based fix: removed garbage braces after \\end{...} at end of file")
		return result, true
	}
	
	return content, false
}

// fixBibliographyOrder fixes the order of \bibliography and \bibliographystyle commands.
// The correct order is \bibliographystyle{...} before \bibliography{...}.
// Many templates have this wrong, causing bibtex to fail with "I found no \bibstyle command".
func fixBibliographyOrder(content string) (string, bool) {
	// Pattern to match \bibliography{...} followed by \bibliographystyle{...}
	// This is the wrong order that needs to be fixed
	bibPattern := regexp.MustCompile(`(?s)(\\bibliography\{([^}]+)\})\s*(\\bibliographystyle\{([^}]+)\})`)
	
	if bibPattern.MatchString(content) {
		// Swap the order: \bibliographystyle should come before \bibliography
		result := bibPattern.ReplaceAllString(content, `\bibliographystyle{$4}`+"\n"+`\bibliography{$2}`)
		logger.Debug("rule-based fix: swapped \\bibliography and \\bibliographystyle order")
		return result, true
	}
	
	return content, false
}

// fixBreakurlCompatibility fixes compatibility issues with the breakurl package in xelatex.
// The breakurl package uses \pdf@box and other \pdf@ commands which are not available in xelatex.
// We add a workaround by defining these commands before the document begins.
func fixBreakurlCompatibility(content string) (string, bool) {
	// Check if the document uses ctex or xeCJK (indicators of xelatex usage)
	// and might have breakurl compatibility issues
	usesXelatex := strings.Contains(content, `\usepackage{ctex}`) ||
		strings.Contains(content, `\usepackage{xeCJK}`) ||
		strings.Contains(content, `\usepackage[UTF8]{ctex}`)

	if !usesXelatex {
		return content, false
	}

	// Check if the fix is already applied
	if strings.Contains(content, `% Fix for breakurl package compatibility with xelatex`) {
		// Check if the enhanced fix is already applied
		if strings.Contains(content, `\headerps@out`) {
			return content, false
		}
		// Need to upgrade the fix to the enhanced version
		// Remove the old fix and apply the new one
		oldFixPattern := regexp.MustCompile(`(?s)% Fix for breakurl package compatibility with xelatex.*?\\makeatother\s*\n`)
		content = oldFixPattern.ReplaceAllString(content, "")
	}

	// Find the position after \begin{document} to add the fix
	// We need to add the fix in the preamble, before \begin{document}
	beginDocPattern := regexp.MustCompile(`(\\begin\{document\})`)
	if !beginDocPattern.MatchString(content) {
		return content, false
	}

	// Add the fix before \begin{document}
	// This defines the missing \pdf@ commands to prevent errors from breakurl package
	// We also redefine \burl to use a simpler implementation that works with xelatex
	// Additionally, we disable the hyperref PostScript header that causes garbage output
	fix := `
% Fix for breakurl package compatibility with xelatex
% The breakurl package uses \pdf@box and other \pdf@ commands which are not available in xelatex
\makeatletter
\@ifundefined{pdf@box}{%
  \newsavebox{\pdf@box}%
}{}
% Define \pdf@addtoksx as a no-op if not defined
\@ifundefined{pdf@addtoksx}{%
  \def\pdf@addtoksx#1{}%
}{}
% Define \headerps@out as a no-op to prevent PostScript garbage in xelatex output
% This is needed because hyperref's breakurl support tries to output PostScript code
\@ifundefined{headerps@out}{%
  \def\headerps@out#1{}%
}{}
% Define burl-related PostScript commands as no-ops
\@ifundefined{burl@stx}{%
  \def\burl@stx{}%
}{}
\@ifundefined{burl@etx}{%
  \def\burl@etx{}%
}{}
% Redefine \burl to use a simpler implementation that works with xelatex
\@ifundefined{burl}{%
  \newcommand{\burl}[1]{\url{#1}}%
}{%
  \renewcommand{\burl}[1]{\url{#1}}%
}
\makeatother

`
	result := beginDocPattern.ReplaceAllString(content, fix+`$1`)
	logger.Debug("rule-based fix: added breakurl compatibility fix for xelatex")
	return result, true
}

// fixSubfigureConflict fixes the conflict between subfigure and subcaption packages
// These packages are incompatible and cause "Command \c@subfigure already defined" errors
// When both are present, we remove subfigure and keep subcaption (the newer package)
func fixSubfigureConflict(content string) (string, bool) {
	// Check if both packages are present
	hasSubfigure := strings.Contains(content, `\usepackage{subfigure}`) ||
		strings.Contains(content, `\usepackage[`) && strings.Contains(content, `]{subfigure}`)
	hasSubcaption := strings.Contains(content, `\usepackage{subcaption}`) ||
		strings.Contains(content, `\usepackage[`) && strings.Contains(content, `]{subcaption}`)

	if !hasSubfigure || !hasSubcaption {
		return content, false
	}

	// Remove subfigure package (keep subcaption as it's the newer, more compatible package)
	// Match \usepackage{subfigure} or \usepackage[options]{subfigure}
	subfigurePattern := regexp.MustCompile(`\\usepackage(\[[^\]]*\])?\{subfigure\}\s*\n?`)
	result := subfigurePattern.ReplaceAllString(content, "")

	logger.Debug("rule-based fix: removed subfigure package (conflicts with subcaption)")
	return result, true
}

// fixNewenvironmentBraces fixes missing closing braces in \newenvironment definitions
// This is a common issue when LLM translation breaks the environment definition structure
// Pattern: \newenvironment{name}[args]{begin}{end} where {end} is missing the closing brace
func fixNewenvironmentBraces(content string) (string, bool) {
	// Pattern to find \newenvironment definitions
	// Look for patterns like:
	// \newenvironment{name}[1]%
	//  {begin code}
	//  {\end{list}   <- missing closing brace
	//
	// Should be:
	//  {\end{list}}

	anyFixed := false
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Look for lines that start with {\end{ and don't have a matching closing brace
		if strings.HasPrefix(trimmed, `{\end{`) {
			// Count braces in this line
			openBraces := strings.Count(trimmed, "{")
			closeBraces := strings.Count(trimmed, "}")

			// If there are more open braces than close braces, add the missing one
			if openBraces > closeBraces {
				// Check if the next line is empty or starts with a command (not a continuation)
				isEndOfDefinition := false
				if i+1 >= len(lines) {
					isEndOfDefinition = true
				} else {
					nextTrimmed := strings.TrimSpace(lines[i+1])
					// If next line is empty, starts with \, or starts with %, it's likely end of definition
					if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, `\`) || strings.HasPrefix(nextTrimmed, `%`) {
						isEndOfDefinition = true
					}
				}

				if isEndOfDefinition {
					// Add missing closing braces
					missingBraces := openBraces - closeBraces
					lines[i] = line + strings.Repeat("}", missingBraces)
					anyFixed = true
					logger.Debug("rule-based fix: added missing closing brace(s) to newenvironment definition",
						logger.Int("line", i+1), logger.Int("count", missingBraces))
				}
			}
		}
	}

	if anyFixed {
		return strings.Join(lines, "\n"), true
	}
	return content, false
}

// fixLonelyItems fixes cases where \item appears after \end{itemize/enumerate/description}
// This is a common issue when LLM translation breaks list structure
func fixLonelyItems(content string) (string, bool) {
	anyFixed := false
	lines := strings.Split(content, "\n")

	// For each list environment type
	for _, envType := range []string{"itemize", "enumerate", "description"} {
		beginTag := `\begin{` + envType + `}`
		endTag := `\end{` + envType + `}`

		// Find all positions of \begin{envType} and \end{envType}
		var beginPositions []int
		var endPositions []int
		for i, line := range lines {
			if strings.Contains(line, beginTag) {
				beginPositions = append(beginPositions, i)
			}
			if strings.Contains(line, endTag) {
				endPositions = append(endPositions, i)
			}
		}

		// Check for lonely items: \item that appears on a separate line after \end{envType}
		// but before the next \begin{envType}
		// This is a sign that the list structure was broken
		for _, endPos := range endPositions {
			// Skip if the \end{envType} line also contains \item (this is handled by Rule 2/3/4)
			if strings.Contains(lines[endPos], `\item`) {
				continue
			}

			// Find the next \begin{envType} after this \end{envType}
			nextBeginPos := -1
			for _, beginPos := range beginPositions {
				if beginPos > endPos {
					nextBeginPos = beginPos
					break
				}
			}

			// Find the next \end{envType} after this \end{envType}
			nextEndPos := -1
			for _, ep := range endPositions {
				if ep > endPos {
					nextEndPos = ep
					break
				}
			}

			// Check for \item between endPos and nextBeginPos (or end of file)
			searchEnd := len(lines)
			if nextBeginPos > 0 {
				searchEnd = nextBeginPos
			}

			// Look for lonely \item lines
			// A lonely item is one that:
			// 1. Is on a separate line after \end{envType}
			// 2. Either:
			//    a. Has non-empty content between it and \end{envType}, OR
			//    b. Is followed by another \end{envType} without a matching \begin{envType}
			//       (indicating broken list structure)
			var lonelyItemLines []int
			hasNonEmptyBetween := false
			hasAnotherEndWithoutBegin := nextEndPos > 0 && (nextBeginPos < 0 || nextEndPos < nextBeginPos)

			for j := endPos + 1; j < searchEnd; j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed == "" {
					continue
				}
				if strings.HasPrefix(trimmed, `\item`) {
					// Consider it lonely if:
					// - There's content between \end and \item, OR
					// - There's another \end without matching \begin (broken structure)
					if hasNonEmptyBetween || hasAnotherEndWithoutBegin {
						lonelyItemLines = append(lonelyItemLines, j)
					}
				} else if !strings.HasPrefix(trimmed, `\end{`) {
					// Non-empty, non-item, non-end line
					hasNonEmptyBetween = true
				}
			}

			// If we found lonely items, move them before the \end{envType}
			if len(lonelyItemLines) > 0 {
				// Collect the lonely items and their content
				var lonelyItems []string
				for _, lineNum := range lonelyItemLines {
					lonelyItems = append(lonelyItems, lines[lineNum])
				}

				// Rebuild lines: insert lonely items before the \end
				var newLines []string
				newLines = append(newLines, lines[:endPos]...)
				newLines = append(newLines, lonelyItems...)
				newLines = append(newLines, lines[endPos])

				// Add remaining lines, skipping the lonely item lines
				for j := endPos + 1; j < len(lines); j++ {
					isLonelyItem := false
					for _, lineNum := range lonelyItemLines {
						if j == lineNum {
							isLonelyItem = true
							break
						}
					}
					if !isLonelyItem {
						newLines = append(newLines, lines[j])
					}
				}

				lines = newLines
				anyFixed = true
				logger.Debug("rule-based fix: moved lonely \\item back into list",
					logger.String("envType", envType),
					logger.Int("itemCount", len(lonelyItems)))

				// Recalculate positions after modification
				beginPositions = nil
				endPositions = nil
				for k, line := range lines {
					if strings.Contains(line, beginTag) {
						beginPositions = append(beginPositions, k)
					}
					if strings.Contains(line, endTag) {
						endPositions = append(endPositions, k)
					}
				}
				// Restart the loop since positions changed
				break
			}
		}
	}

	return strings.Join(lines, "\n"), anyFixed
}

// CompileWithAutoFix compiles a LaTeX document and automatically fixes errors.
func (c *LaTeXCompiler) CompileWithAutoFix(
	texPath string,
	outputDir string,
	fixer *LaTeXFixer,
) (*types.CompileResult, *FixResult, error) {
	logger.Info("compiling with auto-fix", logger.String("texPath", texPath))

	// First, try normal compilation
	result, err := c.Compile(texPath, outputDir)
	if err == nil && result.Success {
		return result, nil, nil
	}

	// If compilation failed and we have a fixer, try to fix
	if fixer != nil && result != nil && result.Log != "" {
		texDir := filepath.Dir(texPath)
		mainTexFile := filepath.Base(texPath)

		fixResult, fixErr := fixer.FixCompilationErrors(texDir, mainTexFile, result.Log, c, outputDir)
		if fixErr != nil {
			logger.Error("auto-fix failed", fixErr)
			return result, fixResult, err
		}

		if fixResult.Success {
			// Try compiling again after fixes
			finalResult, finalErr := c.Compile(texPath, outputDir)
			return finalResult, fixResult, finalErr
		}

		return result, fixResult, err
	}

	return result, nil, err
}


// PreprocessTexFiles preprocesses all .tex files in the given directory.
// It applies QuickFix rules to fix common LaTeX issues before compilation.
// This should be called after extracting the source archive and before compilation.
//
// The function:
// 1. Recursively scans the directory for .tex files
// 2. Applies QuickFix to each file
// 3. Writes back the fixed content if any changes were made
// 4. Logs all fixes applied
//
// Returns an error only if there's a critical failure (e.g., cannot read directory).
// Individual file errors are logged but don't stop processing.
func PreprocessTexFiles(dir string) error {
	logger.Info("preprocessing tex files", logger.String("dir", dir))

	var filesProcessed, filesFixed int

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Warn("error accessing path during preprocessing",
				logger.String("path", path),
				logger.Err(err))
			return nil // Continue processing other files
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process .tex files
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".tex") {
			return nil
		}

		filesProcessed++

		// Only apply QuickFix to translated files (files starting with "translated_")
		// QuickFix is designed for translated documents and can break original documents
		if !strings.HasPrefix(info.Name(), "translated_") {
			relPath, _ := filepath.Rel(dir, path)
			logger.Info("preprocessed tex file",
				logger.String("file", relPath))
			return nil
		}

		// Read file content
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			logger.Warn("failed to read tex file during preprocessing",
				logger.String("path", path),
				logger.Err(readErr))
			return nil // Continue processing other files
		}

		// Apply QuickFix
		fixedContent, wasFixed := QuickFix(string(content))

		if wasFixed {
			// Write back the fixed content
			writeErr := os.WriteFile(path, []byte(fixedContent), info.Mode())
			if writeErr != nil {
				logger.Warn("failed to write fixed tex file",
					logger.String("path", path),
					logger.Err(writeErr))
				return nil // Continue processing other files
			}

			filesFixed++
			relPath, _ := filepath.Rel(dir, path)
			logger.Info("preprocessed tex file",
				logger.String("file", relPath))
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory during preprocessing: %w", err)
	}

	logger.Info("preprocessing complete",
		logger.Int("filesProcessed", filesProcessed),
		logger.Int("filesFixed", filesFixed))

	return nil
}

// analyzeErrorComplexity analyzes the complexity of LaTeX errors to determine fix strategy.
// Returns "low", "medium", or "high" complexity.
func analyzeErrorComplexity(errors []LaTeXError, compileLog string) string {
	if len(errors) == 0 {
		return "low"
	}

	// High complexity indicators
	highComplexityPatterns := []string{
		"emergency stop",
		"no legal \\end found",
		"ended by \\end{document}",
		"fatal error",
		"tex capacity exceeded",
		"missing \\begin{document}",
	}

	// Medium complexity indicators
	mediumComplexityPatterns := []string{
		"mismatched",
		"extra }",
		"missing }",
		"ended by \\end{",
		"lonely \\item",
		"package xcolor error",
		"package hyperref error",
		"environment.*undefined",
		"undefined control sequence",
	}

	logLower := strings.ToLower(compileLog)

	// Check for high complexity
	for _, pattern := range highComplexityPatterns {
		if strings.Contains(logLower, pattern) {
			return "high"
		}
	}

	// Check for medium complexity
	for _, pattern := range mediumComplexityPatterns {
		if strings.Contains(logLower, pattern) {
			return "medium"
		}
	}

	// Check error count
	if len(errors) > 5 {
		return "high"
	} else if len(errors) > 2 {
		return "medium"
	}

	return "low"
}

// runAgentFix runs the agent-based fix strategy.
// This is extracted as a separate function to allow direct invocation for complex errors.
func (f *LaTeXFixer) runAgentFix(
	texDir string,
	mainTexFile string,
	compileLog string,
	compiler *LaTeXCompiler,
	outputDir string,
	result *HierarchicalFixResult,
	progressCallback func(level FixLevel, attempt int, message string),
) (*HierarchicalFixResult, error) {
	mainTexPath := filepath.Join(texDir, mainTexFile)

	// ============ Agent-based fixes ============
	logger.Info("attempting Agent-based fixes (eino ReAct agent)")
	if progressCallback != nil {
		progressCallback(FixLevelAgent, 1, "使用 Eino Agent 智能修复...")
	}

	// Try eino agent first (more sophisticated)
	einoFixer := NewEinoAgentFixer(f.apiKey, f.apiURL, f.agentModel)
	ctx := context.Background()
	
	einoResult, einoErr := einoFixer.FixWithEinoAgent(
		ctx,
		texDir,
		mainTexFile,
		compileLog,
		compiler,
		outputDir,
		func(step int, message string) {
			result.AgentFixAttempts = step
			result.TotalIterations++
			if progressCallback != nil {
				progressCallback(FixLevelAgent, step, message)
			}
		},
	)

	if einoErr != nil {
		logger.Warn("Eino agent fix failed", logger.Err(einoErr))
	} else if einoResult != nil && einoResult.Success {
		logger.Info("compilation succeeded after Eino agent fix", logger.String("summary", einoResult.Summary))
		result.Success = true
		result.Description = einoResult.Summary
		result.FinalFixLevel = FixLevelAgent
		for filename, content := range einoResult.FixedFiles {
			result.FixedFiles[filename] = content
		}
		return result, nil
	}

	// Fallback to tool-calling agent if eino agent fails
	logger.Info("eino agent did not succeed, trying tool-calling agent")
	if progressCallback != nil {
		progressCallback(FixLevelAgent, 2, "尝试备用 Agent 修复...")
	}

	agentFixer := NewLaTeXAgentFixer(f.apiKey, f.apiURL, f.agentModel)
	
	agentResult, agentErr := agentFixer.FixWithAgent(
		ctx,
		texDir,
		mainTexFile,
		compileLog,
		compiler,
		outputDir,
		func(step int, message string) {
			result.AgentFixAttempts++
			result.TotalIterations++
			if progressCallback != nil {
				progressCallback(FixLevelAgent, step+10, message)
			}
		},
	)

	if agentErr != nil {
		logger.Warn("Tool-calling agent fix failed", logger.Err(agentErr))
	} else if agentResult != nil && agentResult.Success {
		logger.Info("compilation succeeded after tool-calling agent fix", logger.String("summary", agentResult.Summary))
		result.Success = true
		result.Description = agentResult.Summary
		result.FinalFixLevel = FixLevelAgent
		for filename, content := range agentResult.FixedFiles {
			result.FixedFiles[filename] = content
		}
		return result, nil
	}

	// Final fallback to prompt-based agent
	logger.Info("tool-calling agents did not succeed, trying prompt-based agent")
	currentLog := compileLog
	
	for attempt := 1; attempt <= 2; attempt++ {
		result.AgentFixAttempts++
		result.TotalIterations++

		if progressCallback != nil {
			progressCallback(FixLevelAgent, attempt+15, fmt.Sprintf("Agent 备用修复尝试 %d/2...", attempt))
		}

		// Parse errors from log
		errors := parseLatexErrors(currentLog)
		if len(errors) == 0 {
			logger.Info("no errors found after agent fix")
			result.Success = true
			result.Description = "Agent 智能修复成功"
			result.FinalFixLevel = FixLevelAgent
			return result, nil
		}

		// Collect all related files for comprehensive analysis
		allFiles := f.collectAllRelatedFiles(texDir, mainTexFile)

		// Ask Agent to fix with comprehensive context
		fixes, description, err := f.askAgentToFix(errors, allFiles, currentLog)
		if err != nil {
			logger.Warn("Agent fix request failed", logger.Err(err))
			continue
		}

		if len(fixes) == 0 {
			logger.Warn("Agent returned no fixes")
			continue
		}

		// Apply fixes
		for filename, fixedContent := range fixes {
			filePath := filepath.Join(texDir, filename)
			if err := os.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
				logger.Warn("failed to write agent-fixed file", logger.Err(err), logger.String("file", filename))
				continue
			}
			result.FixedFiles[filename] = fixedContent
		}
		result.Description = description

		// Try to compile
		compileResult, compileErr := compiler.Compile(mainTexPath, outputDir)
		if compileErr == nil && compileResult.Success {
			logger.Info("compilation succeeded after Agent fix")
			result.Success = true
			result.FinalFixLevel = FixLevelAgent
			return result, nil
		}
		currentLog = compileResult.Log
	}

	logger.Warn("all agent fix attempts exhausted without success")
	result.Description = "Agent 修复未能解决编译错误"
	return result, nil
}

// QuickFixWithReference applies fixes using original LaTeX as reference.
// This is more accurate than QuickFix alone because it can compare structures.
// Parameters:
//   - translated: the translated LaTeX content
//   - original: the original English LaTeX content (optional, can be empty)
// Returns the fixed content and a boolean indicating if any fixes were applied.
func QuickFixWithReference(translated, original string) (string, bool) {
	// First apply standard QuickFix rules
	result, anyFixed := QuickFix(translated)
	
	// If no original provided, return QuickFix result
	if original == "" {
		return result, anyFixed
	}
	
	// Apply translator's reference-based fixes (includes input command restoration)
	// This must be done BEFORE ReferenceFixer to ensure \input commands are present
	result = translator.ApplyReferenceBasedFixes(result, original)
	anyFixed = true // ApplyReferenceBasedFixes always returns modified content
	
	// Apply reference-based fixes
	fixer := NewReferenceFixer()
	fixer.SetOriginal("main.tex", original)
	fixer.SetTranslated("main.tex", result)
	
	refResult := fixer.Fix()
	if refResult.Fixed {
		result = refResult.FixedFiles["main.tex"]
		anyFixed = true
		
		for _, fix := range refResult.Fixes {
			logger.Debug("reference-based fix applied",
				logger.String("description", fix.Description),
				logger.Int("line", fix.Line))
		}
	}
	
	return result, anyFixed
}

// QuickFixFilesWithReference fixes multiple files using their originals as reference.
// Parameters:
//   - translatedFiles: map of filename -> translated content
//   - originalFiles: map of filename -> original content
// Returns map of filename -> fixed content and a boolean indicating if any fixes were applied.
func QuickFixFilesWithReference(translatedFiles, originalFiles map[string]string) (map[string]string, bool) {
	result := make(map[string]string)
	anyFixed := false
	
	// First apply QuickFix to all files
	for filename, content := range translatedFiles {
		fixed, wasFixed := QuickFix(content)
		result[filename] = fixed
		if wasFixed {
			anyFixed = true
		}
	}
	
	// Then apply reference-based fixes
	refResult := FixWithReference(originalFiles, result)
	if refResult.Fixed {
		for filename, content := range refResult.FixedFiles {
			result[filename] = content
		}
		anyFixed = true
		
		for _, fix := range refResult.Fixes {
			logger.Debug("reference-based fix applied",
				logger.String("file", fix.File),
				logger.String("description", fix.Description))
		}
	}
	
	return result, anyFixed
}
