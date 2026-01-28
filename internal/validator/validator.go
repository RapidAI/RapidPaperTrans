// Package validator provides LaTeX syntax validation and fixing functionality.
// It detects syntax errors in LaTeX documents and uses OpenAI API to fix them.
//
// Validates: Requirements 3.3, 3.4, 3.5
package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

const (
	// DefaultTimeout is the default HTTP client timeout for API calls
	DefaultTimeout = 120 * time.Second
	// MaxRetries is the maximum number of retry attempts for API errors
	MaxRetries = 2
	// BaseRetryDelay is the base delay between retries
	BaseRetryDelay = 2 * time.Second
	// OpenAIAPIURL is the OpenAI chat completions API endpoint
	OpenAIAPIURL = "https://api.openai.com/v1/chat/completions"
	// DefaultModel is the default OpenAI model to use
	DefaultModel = "gpt-4o"
)

// SyntaxValidator is responsible for detecting and fixing LaTeX syntax errors.
// It performs basic syntax checking locally and uses OpenAI API for complex fixes.
//
// Validates: Requirements 3.3, 3.4
type SyntaxValidator struct {
	apiKey string
	client *http.Client
	model  string
	apiURL string
}

// NewSyntaxValidator creates a new SyntaxValidator with the specified API key.
func NewSyntaxValidator(apiKey string) *SyntaxValidator {
	return &SyntaxValidator{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
		model:  DefaultModel,
		apiURL: OpenAIAPIURL,
	}
}

// NewSyntaxValidatorWithConfig creates a new SyntaxValidator with full configuration.
func NewSyntaxValidatorWithConfig(apiKey, model, apiURL string, timeout time.Duration) *SyntaxValidator {
	if model == "" {
		model = DefaultModel
	}
	if apiURL == "" {
		apiURL = OpenAIAPIURL
	} else {
		// Auto-complete the chat/completions path if not present
		apiURL = normalizeAPIURL(apiURL)
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &SyntaxValidator{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: timeout,
		},
		model:  model,
		apiURL: apiURL,
	}
}

// normalizeAPIURL ensures the API URL ends with /chat/completions
func normalizeAPIURL(url string) string {
	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")
	
	// If URL already ends with /chat/completions, return as is
	if strings.HasSuffix(url, "/chat/completions") {
		return url
	}
	
	// Append /chat/completions
	return url + "/chat/completions"
}

// GetAPIKey returns the API key used by the validator.
func (v *SyntaxValidator) GetAPIKey() string {
	return v.apiKey
}

// GetModel returns the model used by the validator.
func (v *SyntaxValidator) GetModel() string {
	return v.model
}

// SetModel sets the model to use for fixing.
func (v *SyntaxValidator) SetModel(model string) {
	v.model = model
}

// SetAPIURL sets the API URL (useful for testing with mock servers).
func (v *SyntaxValidator) SetAPIURL(url string) {
	v.apiURL = url
}

// Validate checks LaTeX content for syntax errors.
// It performs basic syntax validation including:
// - Unmatched braces { }
// - Unmatched brackets [ ]
// - Unmatched math delimiters $ and $$
// - Unmatched \begin{} and \end{} environments
// - Unmatched parenthesis math \( \) and bracket math \[ \]
//
// Returns a ValidationResult containing whether the content is valid
// and a list of any syntax errors found.
//
// Validates: Requirements 3.3
func (v *SyntaxValidator) Validate(content string) (*types.ValidationResult, error) {
	logger.Info("validating LaTeX syntax", logger.Int("contentLength", len(content)))

	if content == "" {
		logger.Debug("empty content, returning valid result")
		return &types.ValidationResult{
			IsValid: true,
			Errors:  []types.SyntaxError{},
		}, nil
	}

	var errors []types.SyntaxError

	// Check for unmatched braces
	braceErrors := checkUnmatchedBraces(content)
	errors = append(errors, braceErrors...)

	// Check for unmatched brackets
	bracketErrors := checkUnmatchedBrackets(content)
	errors = append(errors, bracketErrors...)

	// Check for unmatched math delimiters
	mathErrors := checkUnmatchedMathDelimiters(content)
	errors = append(errors, mathErrors...)

	// Check for unmatched environments
	envErrors := checkUnmatchedEnvironments(content)
	errors = append(errors, envErrors...)

	if len(errors) > 0 {
		logger.Warn("syntax errors found", logger.Int("errorCount", len(errors)))
		for _, err := range errors {
			logger.Debug("syntax error", logger.Int("line", err.Line), logger.Int("column", err.Column), logger.String("message", err.Message))
		}
	} else {
		logger.Info("syntax validation passed, no errors found")
	}

	return &types.ValidationResult{
		IsValid: len(errors) == 0,
		Errors:  errors,
	}, nil
}

// Fix attempts to fix LaTeX syntax errors using OpenAI API.
// It takes the content and a list of errors, and returns the corrected content.
//
// Validates: Requirements 3.4
func (v *SyntaxValidator) Fix(content string, errors []types.SyntaxError) (string, error) {
	logger.Info("fixing LaTeX syntax errors", logger.Int("errorCount", len(errors)))

	if v.apiKey == "" {
		logger.Error("API key not configured", nil)
		return "", types.NewAppError(types.ErrConfig, "OpenAI API key is not configured", nil)
	}

	if content == "" {
		logger.Debug("empty content, returning empty result")
		return "", nil
	}

	if len(errors) == 0 {
		logger.Debug("no errors to fix, returning original content")
		return content, nil
	}

	// Try to fix with retry logic
	fixed, err := v.fixWithRetry(content, errors)
	if err != nil {
		logger.Error("syntax fix failed", err)
		return "", err
	}

	logger.Info("syntax errors fixed successfully")
	return fixed, nil
}

// fixWithRetry attempts to fix content with retry logic for transient errors.
func (v *SyntaxValidator) fixWithRetry(content string, errors []types.SyntaxError) (string, error) {
	var lastErr error

	for attempt := 1; attempt <= MaxRetries; attempt++ {
		logger.Debug("fix attempt", logger.Int("attempt", attempt))
		fixed, err := v.doFix(content, errors)
		if err == nil {
			return fixed, nil
		}

		lastErr = err
		logger.Warn("fix attempt failed", logger.Int("attempt", attempt), logger.Err(err))

		// Check if the error is retryable
		if !isRetryableAPIError(err) {
			logger.Error("non-retryable fix error", err)
			return "", err
		}

		// Don't sleep after the last attempt
		if attempt < MaxRetries {
			delay := BaseRetryDelay * time.Duration(attempt)
			logger.Debug("retrying after delay", logger.String("delay", delay.String()))
			time.Sleep(delay)
		}
	}

	logger.Error("syntax fix failed after all retries", lastErr, logger.Int("maxRetries", MaxRetries))
	return "", types.NewAppErrorWithDetails(
		types.ErrAPICall,
		"syntax fix failed after multiple retries",
		fmt.Sprintf("attempted %d times", MaxRetries),
		lastErr,
	)
}

// ChatCompletionRequest represents the request body for OpenAI chat completions API.
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

// Message represents a message in the chat completion request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents the response from OpenAI chat completions API.
type ChatCompletionResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []Choice  `json:"choices"`
	Usage   Usage     `json:"usage"`
	Error   *APIError `json:"error,omitempty"`
}

// Choice represents a choice in the chat completion response.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// APIError represents an error response from the OpenAI API.
type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// doFix performs the actual API call to fix syntax errors.
func (v *SyntaxValidator) doFix(content string, errors []types.SyntaxError) (string, error) {
	logger.Debug("calling OpenAI API for syntax fix", logger.String("model", v.model))

	// Build the fix prompt
	systemPrompt := buildFixSystemPrompt()
	userPrompt := buildFixUserPrompt(content, errors)

	// Create the request body
	reqBody := ChatCompletionRequest{
		Model: v.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.1, // Very low temperature for consistent fixes
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("failed to marshal request body", err)
		return "", types.NewAppError(types.ErrInternal, "failed to marshal request body", err)
	}

	// Create HTTP request
	req, err := http.NewRequest(http.MethodPost, v.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create HTTP request", err)
		return "", types.NewAppError(types.ErrInternal, "failed to create HTTP request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	// Send the request
	resp, err := v.client.Do(req)
	if err != nil {
		logger.Error("API request failed", err)
		return "", types.NewAppError(types.ErrNetwork, "API request failed", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read API response", err)
		return "", types.NewAppError(types.ErrNetwork, "failed to read API response", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		logger.Error("API returned error status", nil, logger.Int("statusCode", resp.StatusCode))
		return "", handleAPIHTTPError(resp.StatusCode, body)
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error("failed to parse API response", err)
		return "", types.NewAppError(types.ErrAPICall, "failed to parse API response", err)
	}

	// Check for API error in response
	if chatResp.Error != nil {
		logger.Error("API returned error in response", nil, logger.String("errorMessage", chatResp.Error.Message))
		return "", types.NewAppErrorWithDetails(
			types.ErrAPICall,
			"API returned error",
			chatResp.Error.Message,
			nil,
		)
	}

	// Extract fixed content
	if len(chatResp.Choices) == 0 {
		logger.Error("API returned no choices", nil)
		return "", types.NewAppError(types.ErrAPICall, "API returned no choices", nil)
	}

	fixedContent := chatResp.Choices[0].Message.Content
	logger.Debug("API call successful")
	return fixedContent, nil
}

// buildFixSystemPrompt creates the system prompt for the fix task.
func buildFixSystemPrompt() string {
	return `You are a LaTeX syntax expert. Your task is to fix LaTeX syntax errors in documents.

CRITICAL RULES:
1. Fix ONLY the syntax errors mentioned - do not change anything else.
2. Preserve all content, formatting, and structure.
3. Common fixes include:
   - Adding missing closing braces } or brackets ]
   - Adding missing \end{} for unclosed environments
   - Fixing unmatched math delimiters $ or $$
   - Fixing unmatched \( \) or \[ \]
4. Output ONLY the corrected LaTeX content - no explanations or comments.
5. Maintain the exact same line structure where possible.
6. Do not add any new content or remove existing content beyond fixing syntax.`
}

// buildFixUserPrompt creates the user prompt with the content and errors to fix.
func buildFixUserPrompt(content string, errors []types.SyntaxError) string {
	var errorDescriptions []string
	for _, err := range errors {
		desc := fmt.Sprintf("- Line %d, Column %d: %s (Type: %s)", err.Line, err.Column, err.Message, err.Type)
		errorDescriptions = append(errorDescriptions, desc)
	}

	return fmt.Sprintf(`Fix the following LaTeX syntax errors in the document below.

ERRORS TO FIX:
%s

DOCUMENT:
%s`, strings.Join(errorDescriptions, "\n"), content)
}

// handleAPIHTTPError creates an appropriate AppError based on the HTTP status code.
func handleAPIHTTPError(statusCode int, body []byte) error {
	// Try to parse error message from response body
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	errorDetails := ""
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		errorDetails = errResp.Error.Message
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return types.NewAppErrorWithDetails(
			types.ErrAPICall,
			"API authentication failed",
			"invalid API key or unauthorized access",
			nil,
		)
	case http.StatusTooManyRequests:
		return types.NewAppErrorWithDetails(
			types.ErrAPIRateLimit,
			"API rate limit exceeded",
			errorDetails,
			nil,
		)
	case http.StatusBadRequest:
		return types.NewAppErrorWithDetails(
			types.ErrAPICall,
			"invalid API request",
			errorDetails,
			nil,
		)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return types.NewAppErrorWithDetails(
			types.ErrAPICall,
			"API server error",
			fmt.Sprintf("status %d: %s", statusCode, errorDetails),
			nil,
		)
	default:
		return types.NewAppErrorWithDetails(
			types.ErrAPICall,
			"API request failed",
			fmt.Sprintf("status %d: %s", statusCode, errorDetails),
			nil,
		)
	}
}

// isRetryableAPIError determines if an error should trigger a retry.
func isRetryableAPIError(err error) bool {
	if err == nil {
		return false
	}

	if appErr, ok := err.(*types.AppError); ok {
		switch appErr.Code {
		case types.ErrNetwork:
			return true
		case types.ErrAPIRateLimit:
			return true
		case types.ErrAPICall:
			// Retry on server errors, but not on client errors
			if strings.Contains(appErr.Details, "status 5") {
				return true
			}
			return false
		default:
			return false
		}
	}

	return false
}

// =============================================================================
// Syntax Checking Helper Functions
// =============================================================================

// checkUnmatchedBraces checks for unmatched curly braces { }.
// It tracks brace depth and reports errors for unmatched opening or closing braces.
func checkUnmatchedBraces(content string) []types.SyntaxError {
	var errors []types.SyntaxError
	lines := strings.Split(content, "\n")

	// Stack to track opening brace positions
	type bracePos struct {
		line   int
		column int
	}
	var stack []bracePos

	for lineNum, line := range lines {
		// Skip comments
		commentIdx := findUnescapedPercent(line)
		if commentIdx >= 0 {
			line = line[:commentIdx]
		}

		for colNum := 0; colNum < len(line); colNum++ {
			// Skip escaped braces
			if colNum > 0 && line[colNum-1] == '\\' {
				continue
			}

			if line[colNum] == '{' {
				stack = append(stack, bracePos{line: lineNum + 1, column: colNum + 1})
			} else if line[colNum] == '}' {
				if len(stack) == 0 {
					errors = append(errors, types.SyntaxError{
						Line:    lineNum + 1,
						Column:  colNum + 1,
						Message: "unmatched closing brace '}'",
						Type:    "brace",
					})
				} else {
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	// Report any unclosed braces
	for _, pos := range stack {
		errors = append(errors, types.SyntaxError{
			Line:    pos.line,
			Column:  pos.column,
			Message: "unmatched opening brace '{'",
			Type:    "brace",
		})
	}

	return errors
}

// checkUnmatchedBrackets checks for unmatched square brackets [ ].
// Note: This is more lenient than braces since brackets are often used in optional arguments.
func checkUnmatchedBrackets(content string) []types.SyntaxError {
	var errors []types.SyntaxError
	lines := strings.Split(content, "\n")

	// Stack to track opening bracket positions
	type bracketPos struct {
		line   int
		column int
	}
	var stack []bracketPos

	for lineNum, line := range lines {
		// Skip comments
		commentIdx := findUnescapedPercent(line)
		if commentIdx >= 0 {
			line = line[:commentIdx]
		}

		for colNum := 0; colNum < len(line); colNum++ {
			// Skip escaped brackets
			if colNum > 0 && line[colNum-1] == '\\' {
				continue
			}

			if line[colNum] == '[' {
				stack = append(stack, bracketPos{line: lineNum + 1, column: colNum + 1})
			} else if line[colNum] == ']' {
				if len(stack) == 0 {
					errors = append(errors, types.SyntaxError{
						Line:    lineNum + 1,
						Column:  colNum + 1,
						Message: "unmatched closing bracket ']'",
						Type:    "bracket",
					})
				} else {
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	// Report any unclosed brackets
	for _, pos := range stack {
		errors = append(errors, types.SyntaxError{
			Line:    pos.line,
			Column:  pos.column,
			Message: "unmatched opening bracket '['",
			Type:    "bracket",
		})
	}

	return errors
}

// checkUnmatchedMathDelimiters checks for unmatched math delimiters.
// This includes:
// - Single dollar signs $ for inline math
// - Double dollar signs $$ for display math
// - \( \) for inline math
// - \[ \] for display math
func checkUnmatchedMathDelimiters(content string) []types.SyntaxError {
	var errors []types.SyntaxError

	// Check inline math $...$
	errors = append(errors, checkInlineMath(content)...)

	// Check display math $$...$$
	errors = append(errors, checkDisplayMath(content)...)

	// Check \(...\)
	errors = append(errors, checkParenMath(content)...)

	// Check \[...\]
	errors = append(errors, checkBracketMath(content)...)

	return errors
}

// checkInlineMath checks for unmatched single dollar signs.
func checkInlineMath(content string) []types.SyntaxError {
	var errors []types.SyntaxError
	lines := strings.Split(content, "\n")

	inMath := false
	var mathStart struct {
		line   int
		column int
	}

	for lineNum, line := range lines {
		// Skip comments
		commentIdx := findUnescapedPercent(line)
		if commentIdx >= 0 {
			line = line[:commentIdx]
		}

		for colNum := 0; colNum < len(line); colNum++ {
			// Skip escaped dollar signs
			if colNum > 0 && line[colNum-1] == '\\' {
				continue
			}

			// Skip double dollar signs (handled separately)
			if line[colNum] == '$' {
				if colNum+1 < len(line) && line[colNum+1] == '$' {
					colNum++ // Skip the second $
					continue
				}
				if colNum > 0 && line[colNum-1] == '$' {
					continue // This is the second $ of $$
				}

				if !inMath {
					inMath = true
					mathStart.line = lineNum + 1
					mathStart.column = colNum + 1
				} else {
					inMath = false
				}
			}
		}
	}

	if inMath {
		errors = append(errors, types.SyntaxError{
			Line:    mathStart.line,
			Column:  mathStart.column,
			Message: "unmatched inline math delimiter '$'",
			Type:    "math",
		})
	}

	return errors
}

// checkDisplayMath checks for unmatched double dollar signs.
func checkDisplayMath(content string) []types.SyntaxError {
	var errors []types.SyntaxError

	// Use regex to find $$ patterns
	pattern := regexp.MustCompile(`\$\$`)
	matches := pattern.FindAllStringIndex(content, -1)

	if len(matches)%2 != 0 {
		// Find the position of the last unmatched $$
		lastMatch := matches[len(matches)-1]
		line, col := getLineAndColumn(content, lastMatch[0])
		errors = append(errors, types.SyntaxError{
			Line:    line,
			Column:  col,
			Message: "unmatched display math delimiter '$$'",
			Type:    "math",
		})
	}

	return errors
}

// checkParenMath checks for unmatched \( and \).
func checkParenMath(content string) []types.SyntaxError {
	var errors []types.SyntaxError

	openPattern := regexp.MustCompile(`\\\(`)
	closePattern := regexp.MustCompile(`\\\)`)

	opens := openPattern.FindAllStringIndex(content, -1)
	closes := closePattern.FindAllStringIndex(content, -1)

	if len(opens) > len(closes) {
		// Find unmatched opens
		for i := len(closes); i < len(opens); i++ {
			line, col := getLineAndColumn(content, opens[i][0])
			errors = append(errors, types.SyntaxError{
				Line:    line,
				Column:  col,
				Message: "unmatched opening math delimiter '\\('",
				Type:    "math",
			})
		}
	} else if len(closes) > len(opens) {
		// Find unmatched closes
		for i := len(opens); i < len(closes); i++ {
			line, col := getLineAndColumn(content, closes[i][0])
			errors = append(errors, types.SyntaxError{
				Line:    line,
				Column:  col,
				Message: "unmatched closing math delimiter '\\)'",
				Type:    "math",
			})
		}
	}

	return errors
}

// checkBracketMath checks for unmatched \[ and \].
func checkBracketMath(content string) []types.SyntaxError {
	var errors []types.SyntaxError

	openPattern := regexp.MustCompile(`\\\[`)
	closePattern := regexp.MustCompile(`\\\]`)

	opens := openPattern.FindAllStringIndex(content, -1)
	closes := closePattern.FindAllStringIndex(content, -1)

	if len(opens) > len(closes) {
		// Find unmatched opens
		for i := len(closes); i < len(opens); i++ {
			line, col := getLineAndColumn(content, opens[i][0])
			errors = append(errors, types.SyntaxError{
				Line:    line,
				Column:  col,
				Message: "unmatched opening math delimiter '\\['",
				Type:    "math",
			})
		}
	} else if len(closes) > len(opens) {
		// Find unmatched closes
		for i := len(opens); i < len(closes); i++ {
			line, col := getLineAndColumn(content, closes[i][0])
			errors = append(errors, types.SyntaxError{
				Line:    line,
				Column:  col,
				Message: "unmatched closing math delimiter '\\]'",
				Type:    "math",
			})
		}
	}

	return errors
}

// checkUnmatchedEnvironments checks for unmatched \begin{} and \end{} pairs.
func checkUnmatchedEnvironments(content string) []types.SyntaxError {
	var errors []types.SyntaxError

	// Pattern to match \begin{envname} and \end{envname}
	beginPattern := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endPattern := regexp.MustCompile(`\\end\{([^}]+)\}`)

	beginMatches := beginPattern.FindAllStringSubmatchIndex(content, -1)
	endMatches := endPattern.FindAllStringSubmatchIndex(content, -1)

	// Build a map of environment names to their begin positions
	type envInfo struct {
		name   string
		pos    int
		line   int
		column int
	}

	var stack []envInfo

	// Process all begin and end tags in order
	type tagInfo struct {
		isBegin bool
		name    string
		pos     int
	}

	var tags []tagInfo

	for _, match := range beginMatches {
		name := content[match[2]:match[3]]
		tags = append(tags, tagInfo{isBegin: true, name: name, pos: match[0]})
	}

	for _, match := range endMatches {
		name := content[match[2]:match[3]]
		tags = append(tags, tagInfo{isBegin: false, name: name, pos: match[0]})
	}

	// Sort tags by position
	for i := 0; i < len(tags)-1; i++ {
		for j := i + 1; j < len(tags); j++ {
			if tags[j].pos < tags[i].pos {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}

	// Process tags in order
	for _, tag := range tags {
		line, col := getLineAndColumn(content, tag.pos)

		if tag.isBegin {
			stack = append(stack, envInfo{
				name:   tag.name,
				pos:    tag.pos,
				line:   line,
				column: col,
			})
		} else {
			// Look for matching begin
			found := false
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == tag.name {
					// Remove from stack
					stack = append(stack[:i], stack[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				errors = append(errors, types.SyntaxError{
					Line:    line,
					Column:  col,
					Message: fmt.Sprintf("unmatched \\end{%s} without corresponding \\begin{%s}", tag.name, tag.name),
					Type:    "environment",
				})
			}
		}
	}

	// Report any unclosed environments
	for _, env := range stack {
		errors = append(errors, types.SyntaxError{
			Line:    env.line,
			Column:  env.column,
			Message: fmt.Sprintf("unmatched \\begin{%s} without corresponding \\end{%s}", env.name, env.name),
			Type:    "environment",
		})
	}

	return errors
}

// =============================================================================
// Utility Functions
// =============================================================================

// findUnescapedPercent finds the position of the first unescaped % in a line.
// Returns -1 if no unescaped % is found.
func findUnescapedPercent(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] == '%' {
			// Check if it's escaped
			backslashCount := 0
			for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
				backslashCount++
			}
			// If even number of backslashes, the % is not escaped
			if backslashCount%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// getLineAndColumn converts a byte position to line and column numbers.
func getLineAndColumn(content string, pos int) (int, int) {
	if pos < 0 || pos > len(content) {
		return 1, 1
	}

	line := 1
	lastNewline := -1

	for i := 0; i < pos; i++ {
		if content[i] == '\n' {
			line++
			lastNewline = i
		}
	}

	column := pos - lastNewline
	return line, column
}
