// Package translator provides functionality for translating LaTeX documents using OpenAI API.
package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

const (
	// DefaultModel is the default OpenAI model to use for translation
	DefaultModel = "gpt-4o"
	// DefaultTimeout is the default HTTP client timeout for API calls
	DefaultTimeout = 120 * time.Second
	// MaxRetries is the maximum number of retry attempts for API errors
	MaxRetries = 2
	// BaseRetryDelay is the base delay between retries
	BaseRetryDelay = 2 * time.Second
	// MaxChunkSize is the maximum size of a text chunk for translation (in characters)
	// This helps avoid token limits and ensures reliable translation
	MaxChunkSize = 4000
	// OpenAIAPIURL is the OpenAI chat completions API endpoint
	OpenAIAPIURL = "https://api.openai.com/v1/chat/completions"
)

// TranslationEngine is responsible for translating LaTeX documents using OpenAI API.
// It handles text chunking for large documents and preserves LaTeX commands during translation.
//
// Validates: Requirements 3.1, 3.2
type TranslationEngine struct {
	apiKey      string
	client      *http.Client
	model       string
	apiURL      string
	concurrency int
}

// NewTranslationEngine creates a new TranslationEngine with the specified API key.
// It uses the default model (gpt-4o) and configures an HTTP client with appropriate timeout.
func NewTranslationEngine(apiKey string) *TranslationEngine {
	return &TranslationEngine{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
		model:       DefaultModel,
		apiURL:      OpenAIAPIURL,
		concurrency: 3,
	}
}

// NewTranslationEngineWithModel creates a new TranslationEngine with a custom model.
func NewTranslationEngineWithModel(apiKey, model string) *TranslationEngine {
	return &TranslationEngine{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
		model:       model,
		apiURL:      OpenAIAPIURL,
		concurrency: 3,
	}
}

// NewTranslationEngineWithConfig creates a new TranslationEngine with full configuration.
func NewTranslationEngineWithConfig(apiKey, model, apiURL string, timeout time.Duration, concurrency int) *TranslationEngine {
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
	if concurrency <= 0 {
		concurrency = 3
	}
	return &TranslationEngine{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: timeout,
		},
		model:       model,
		apiURL:      apiURL,
		concurrency: concurrency,
	}
}

// normalizeAPIURL ensures the API URL ends with /chat/completions
func normalizeAPIURL(url string) string {
	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")
	
	// If URL already ends with /chat/completions, return as is
	if strings.HasSuffix(url, "/chat/completions") {
		logger.Debug("API URL already complete", logger.String("url", url))
		return url
	}
	
	// Append /chat/completions
	result := url + "/chat/completions"
	logger.Debug("API URL normalized", logger.String("original", url), logger.String("normalized", result))
	return result
}

// GetAPIKey returns the API key used by the engine.
func (t *TranslationEngine) GetAPIKey() string {
	return t.apiKey
}

// GetModel returns the model used by the engine.
func (t *TranslationEngine) GetModel() string {
	return t.model
}

// SetModel sets the model to use for translation.
func (t *TranslationEngine) SetModel(model string) {
	t.model = model
}

// SetAPIURL sets the API URL (useful for testing with mock servers).
func (t *TranslationEngine) SetAPIURL(url string) {
	t.apiURL = url
}

// TestConnection tests the API connection by sending a simple request.
// It asks the LLM to respond with just "ok" to minimize token usage.
// Returns nil if successful, or an error if the connection fails.
func (t *TranslationEngine) TestConnection() error {
	logger.Info("testing API connection", logger.String("apiURL", t.apiURL), logger.String("model", t.model))

	if t.apiKey == "" {
		return types.NewAppError(types.ErrConfig, "API key is not configured", nil)
	}

	// Build a minimal test request
	reqBody := ChatCompletionRequest{
		Model: t.model,
		Messages: []Message{
			{Role: "user", Content: "Reply with only the word 'ok', nothing else."},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("failed to marshal test request", err)
		return types.NewAppError(types.ErrInternal, "failed to create test request", err)
	}

	// Create HTTP request
	req, err := http.NewRequest(http.MethodPost, t.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create HTTP request", err)
		return types.NewAppError(types.ErrInternal, "failed to create HTTP request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	// Send the request
	resp, err := t.client.Do(req)
	if err != nil {
		logger.Error("API test request failed", err)
		return types.NewAppError(types.ErrNetwork, "API 连接失败", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read API response", err)
		return types.NewAppError(types.ErrNetwork, "读取响应失败", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		logger.Error("API test returned error status", nil, logger.Int("statusCode", resp.StatusCode))
		return handleAPIHTTPError(resp.StatusCode, body)
	}

	// Parse response to verify it's valid
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error("failed to parse API response", err)
		return types.NewAppError(types.ErrAPICall, "响应格式错误", err)
	}

	// Check for API error in response
	if chatResp.Error != nil {
		logger.Error("API returned error", nil, logger.String("errorMessage", chatResp.Error.Message))
		return types.NewAppErrorWithDetails(types.ErrAPICall, "API 错误", chatResp.Error.Message, nil)
	}

	// Verify LLM actually responded
	if len(chatResp.Choices) == 0 {
		logger.Error("API returned no choices", nil)
		return types.NewAppError(types.ErrAPICall, "LLM 未返回响应", nil)
	}

	// Check if LLM response contains "ok"
	llmResponse := strings.ToLower(strings.TrimSpace(chatResp.Choices[0].Message.Content))
	logger.Info("LLM test response", logger.String("response", llmResponse))
	
	if !strings.Contains(llmResponse, "ok") {
		logger.Error("LLM did not respond with 'ok'", nil, logger.String("response", llmResponse))
		return types.NewAppErrorWithDetails(types.ErrAPICall, "LLM 响应异常", 
			fmt.Sprintf("期望返回 'ok'，实际返回: %s", llmResponse), nil)
	}

	logger.Info("API connection test successful - LLM responded correctly")
	return nil
}

// TranslationProgressCallback is called during translation to report progress
type TranslationProgressCallback func(current, total int, message string)

// TranslateTeX translates a complete LaTeX document from English to Chinese.
// It handles large documents by splitting them into chunks and translating each chunk separately.
// All LaTeX commands and mathematical formulas are preserved during translation.
//
// The function returns a TranslationResult containing the original content,
// translated content, and the total number of tokens used.
//
// Validates: Requirements 3.1, 3.2
func (t *TranslationEngine) TranslateTeX(content string) (*types.TranslationResult, error) {
	return t.TranslateTeXWithProgress(content, nil)
}

// TranslateTeXWithProgress translates a LaTeX document with progress callback.
// The callback is called after each chunk is translated with (current, total, message).
func (t *TranslationEngine) TranslateTeXWithProgress(content string, progressCallback TranslationProgressCallback) (*types.TranslationResult, error) {
	logger.Info("starting LaTeX translation", logger.Int("contentLength", len(content)), logger.Int("concurrency", t.concurrency))

	if t.apiKey == "" {
		logger.Error("API key not configured", nil)
		return nil, types.NewAppError(types.ErrConfig, "OpenAI API key is not configured", nil)
	}

	if content == "" {
		logger.Debug("empty content, returning empty result")
		return &types.TranslationResult{
			OriginalContent:   "",
			TranslatedContent: "",
			TokensUsed:        0,
		}, nil
	}

	// Protect comment environments - they should not be translated
	// The comment package in LaTeX treats everything between \begin{comment} and \end{comment} as comments
	contentWithProtectedComments, commentPlaceholders := protectCommentEnvironments(content)
	if len(commentPlaceholders) > 0 {
		logger.Info("protected comment environments", logger.Int("count", len(commentPlaceholders)))
	}

	// Note: Caption pre-translation is disabled for now because it may cause issues
	// with the main translation flow. The table/figure environments are protected
	// in protectedEnvNames, so their structure will be preserved.
	// TODO: Re-enable caption translation after fixing the issues
	contentWithTranslatedCaptions := contentWithProtectedComments

	// Note: Preprocessing (comment removal) is disabled for now because:
	// 1. Some documents have intentionally commented-out code that affects structure
	// 2. Removing comments can change the document structure unexpectedly
	// 3. The post-processor can handle most issues without preprocessing
	// 
	// To enable preprocessing in the future, uncomment the following:
	// preprocessedContent := PreprocessLaTeX(content, DefaultPreprocessOptions())
	// and use preprocessedContent instead of content for splitting

	// Split content into chunks for translation
	chunks := splitIntoChunks(contentWithTranslatedCaptions, MaxChunkSize)
	totalChunks := len(chunks)
	logger.Info("content split into chunks", logger.Int("chunkCount", totalChunks))

	// Prepare result storage
	translatedChunks := make([]string, totalChunks)
	tokenCounts := make([]int, totalChunks)
	errors := make([]error, totalChunks)

	// Use semaphore for concurrency control
	sem := make(chan struct{}, t.concurrency)
	var wg sync.WaitGroup
	var completedCount int32
	var mu sync.Mutex

	for i, chunk := range chunks {
		wg.Add(1)
		go func(idx int, chunkContent string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			chunkNum := idx + 1
			logger.Debug("translating chunk", logger.Int("chunkIndex", chunkNum), logger.Int("totalChunks", totalChunks))

			translated, tokens, err := t.translateChunkWithRetry(chunkContent)

			// Post-process each chunk immediately after translation
			// Compare with original chunk to fix format issues
			if err == nil && translated != "" {
				beforeLen := len(translated)
				translated = PostprocessChunk(translated, chunkContent)
				afterLen := len(translated)
				if beforeLen != afterLen {
					logger.Info("postprocessor changed chunk",
						logger.Int("chunkIndex", chunkNum),
						logger.Int("beforeLen", beforeLen),
						logger.Int("afterLen", afterLen))
				}
			}

			mu.Lock()
			translatedChunks[idx] = translated
			tokenCounts[idx] = tokens
			errors[idx] = err
			completedCount++
			completed := int(completedCount)
			mu.Unlock()

			// Report progress after translating
			if progressCallback != nil {
				progressCallback(completed, totalChunks, fmt.Sprintf("翻译中 (%d/%d 分块)...", completed, totalChunks))
			}

			if err != nil {
				logger.Error("chunk translation failed", err, logger.Int("chunkIndex", chunkNum))
			} else {
				logger.Debug("chunk translated successfully", logger.Int("chunkIndex", chunkNum), logger.Int("tokensUsed", tokens))
			}
		}(i, chunk)
	}

	// Wait for all translations to complete
	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			// If it's already an AppError, add chunk info to details and return it
			if appErr, ok := err.(*types.AppError); ok {
				appErr.Details = fmt.Sprintf("chunk %d: %s", i+1, appErr.Details)
				return nil, appErr
			}
			// Otherwise wrap it in an AppError
			return nil, types.NewAppErrorWithDetails(
				types.ErrAPICall,
				fmt.Sprintf("chunk %d translation failed", i+1),
				err.Error(),
				err,
			)
		}
	}

	// Calculate total tokens
	totalTokens := 0
	for _, tokens := range tokenCounts {
		totalTokens += tokens
	}

	// Join translated chunks back together
	translatedContent := strings.Join(translatedChunks, "")

	// Restore protected comment environments
	if len(commentPlaceholders) > 0 {
		translatedContent = restoreCommentEnvironments(translatedContent, commentPlaceholders)
		logger.Info("restored comment environments", logger.Int("count", len(commentPlaceholders)))
	}

	// Final fixes on the complete translated content
	// Fix LaTeX line structure issues caused by LLM merging lines
	translatedContent = FixLaTeXLineStructure(translatedContent)

	// Apply reference-based fixes using original content
	// This compares the translated content with the original to fix structural issues
	translatedContent = ApplyReferenceBasedFixes(translatedContent, content)

	// Validate the translation result to detect anomalies
	validator := NewTranslationValidator()
	validationResult := validator.ValidateTranslation(content, translatedContent)
	
	if !validationResult.IsValid {
		errorMsg := FormatValidationErrors(validationResult)
		logger.Error("translation validation failed", nil,
			logger.Int("originalLength", validationResult.OriginalLength),
			logger.Int("translatedLength", validationResult.TranslatedLength),
			logger.Int("chineseCharCount", validationResult.ChineseCharCount),
			logger.Float64("lengthRatio", validationResult.LengthRatio))
		
		return nil, types.NewAppErrorWithDetails(
			types.ErrTranslation,
			"翻译结果验证失败",
			errorMsg,
			nil,
		)
	}

	// Log warnings if any
	for _, warning := range validationResult.Warnings {
		logger.Warn("translation validation warning", logger.String("warning", warning))
	}

	logger.Info("translation completed successfully", 
		logger.Int("totalTokens", totalTokens),
		logger.Int("chineseCharCount", validationResult.ChineseCharCount),
		logger.Float64("lengthRatio", validationResult.LengthRatio))
	return &types.TranslationResult{
		OriginalContent:   content,
		TranslatedContent: translatedContent,
		TokensUsed:        totalTokens,
	}, nil
}

// TranslateChunk translates a single text chunk from English to Chinese.
// It preserves all LaTeX commands and mathematical formulas.
//
// Validates: Requirements 3.1, 3.2
func (t *TranslationEngine) TranslateChunk(chunk string) (string, error) {
	if t.apiKey == "" {
		return "", types.NewAppError(types.ErrConfig, "OpenAI API key is not configured", nil)
	}

	if chunk == "" {
		return "", nil
	}

	translated, _, err := t.translateChunkWithRetry(chunk)
	return translated, err
}

// translateChunkWithRetry translates a chunk with retry logic for transient errors.
func (t *TranslationEngine) translateChunkWithRetry(chunk string) (string, int, error) {
	var lastErr error

	for attempt := 1; attempt <= MaxRetries; attempt++ {
		logger.Debug("translation attempt", logger.Int("attempt", attempt))
		translated, tokens, err := t.doTranslateChunk(chunk)
		if err == nil {
			return translated, tokens, nil
		}

		lastErr = err
		logger.Warn("translation attempt failed", logger.Int("attempt", attempt), logger.Err(err))

		// Check if the error is retryable
		if !isRetryableAPIError(err) {
			logger.Error("non-retryable translation error", err)
			return "", 0, err
		}

		// Don't sleep after the last attempt
		if attempt < MaxRetries {
			delay := BaseRetryDelay * time.Duration(attempt)
			logger.Debug("retrying after delay", logger.String("delay", delay.String()))
			time.Sleep(delay)
		}
	}

	logger.Error("translation failed after all retries", lastErr, logger.Int("maxRetries", MaxRetries))
	return "", 0, types.NewAppErrorWithDetails(
		types.ErrAPICall,
		"translation failed after multiple retries",
		fmt.Sprintf("attempted %d times", MaxRetries),
		lastErr,
	)
}

// ChatCompletionRequest represents the request body for OpenAI chat completions API.
type ChatCompletionRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// Message represents a message in the chat completion request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents the response from OpenAI chat completions API.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
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

// doTranslateChunk performs the actual API call to translate a chunk.
func (t *TranslationEngine) doTranslateChunk(chunk string) (string, int, error) {
	logger.Debug("calling OpenAI API for translation", logger.String("model", t.model))

	// Step 1: Protect LaTeX commands before translation
	protectedContent, placeholders := ProtectLaTeXCommands(chunk)
	logger.Debug("protected LaTeX commands",
		logger.Int("originalLength", len(chunk)),
		logger.Int("protectedLength", len(protectedContent)),
		logger.Int("placeholderCount", len(placeholders)))

	// Build the translation prompt with protected content
	systemPrompt := buildSystemPromptWithProtection()
	userPrompt := buildUserPromptWithProtection(protectedContent, len(placeholders))

	// Create the request body
	// Set max_tokens based on input size to avoid truncation
	// Translation typically expands text by 1.5-2x for Chinese
	// Estimate: 1 token ≈ 4 chars for English, 1 token ≈ 1.5 chars for Chinese
	// So output tokens ≈ input_chars / 4 * 2 = input_chars / 2
	// Add 50% buffer for safety
	estimatedOutputTokens := len(chunk) / 2 * 3 / 2 // input_chars / 2 * 1.5
	if estimatedOutputTokens < 512 {
		estimatedOutputTokens = 512 // Minimum 512 tokens
	}
	if estimatedOutputTokens > 8192 {
		estimatedOutputTokens = 8192 // Cap at 8192 to avoid API limits
	}
	
	reqBody := ChatCompletionRequest{
		Model: t.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: estimatedOutputTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("failed to marshal request body", err)
		return "", 0, types.NewAppError(types.ErrInternal, "failed to marshal request body", err)
	}

	// Create HTTP request
	req, err := http.NewRequest(http.MethodPost, t.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create HTTP request", err)
		return "", 0, types.NewAppError(types.ErrInternal, "failed to create HTTP request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	// Send the request
	resp, err := t.client.Do(req)
	if err != nil {
		logger.Error("API request failed", err)
		return "", 0, types.NewAppError(types.ErrNetwork, "API request failed", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read API response", err)
		return "", 0, types.NewAppError(types.ErrNetwork, "failed to read API response", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		logger.Error("API returned error status", nil, logger.Int("statusCode", resp.StatusCode))
		return "", 0, handleAPIHTTPError(resp.StatusCode, body)
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error("failed to parse API response", err)
		return "", 0, types.NewAppError(types.ErrAPICall, "failed to parse API response", err)
	}

	// Check for API error in response
	if chatResp.Error != nil {
		logger.Error("API returned error in response", nil, logger.String("errorMessage", chatResp.Error.Message))
		return "", 0, types.NewAppErrorWithDetails(
			types.ErrAPICall,
			"API returned error",
			chatResp.Error.Message,
			nil,
		)
	}

	// Extract translated content
	if len(chatResp.Choices) == 0 {
		logger.Error("API returned no choices", nil)
		return "", 0, types.NewAppError(types.ErrAPICall, "API returned no choices", nil)
	}

	// Check if output was truncated due to length limit
	finishReason := chatResp.Choices[0].FinishReason
	if finishReason == "length" {
		logger.Warn("translation output was truncated due to length limit",
			logger.Int("completionTokens", chatResp.Usage.CompletionTokens),
			logger.Int("inputLength", len(chunk)))
		// Continue with truncated content but log warning
		// The content may be incomplete, but we'll try to use what we have
		// Consider reducing MaxChunkSize if this happens frequently
	}

	translatedContent := chatResp.Choices[0].Message.Content
	tokensUsed := chatResp.Usage.TotalTokens

	// Clean up translation result to remove JSON formatting artifacts
	translatedContent = cleanTranslationResult(translatedContent)

	// Step 2: Validate and restore LaTeX commands after translation
	if len(placeholders) > 0 {
		// Validate that all placeholders are present in the translation
		missingPlaceholders := validatePlaceholders(translatedContent, placeholders)
		if len(missingPlaceholders) > 0 {
			logger.Warn("some placeholders were lost during translation",
				logger.Int("missingCount", len(missingPlaceholders)),
				logger.String("missing", strings.Join(missingPlaceholders, ", ")))
			// Try to recover by re-inserting missing placeholders at reasonable positions
			translatedContent = recoverMissingPlaceholders(translatedContent, placeholders, missingPlaceholders)
		}

		translatedContent = RestoreLaTeXCommands(translatedContent, placeholders)
		logger.Debug("restored LaTeX commands",
			logger.Int("placeholderCount", len(placeholders)),
			logger.Int("finalLength", len(translatedContent)))
	}

	logger.Debug("API call successful", logger.Int("tokensUsed", tokensUsed), logger.String("finishReason", finishReason))
	return translatedContent, tokensUsed, nil
}

// validatePlaceholders checks if all placeholders are present in the translated content.
// Returns a list of missing placeholder keys.
func validatePlaceholders(content string, placeholders map[string]string) []string {
	var missing []string
	for placeholder := range placeholders {
		if !strings.Contains(content, placeholder) {
			missing = append(missing, placeholder)
		}
	}
	return missing
}

// RecoveryStrategy represents the strategy used to recover a placeholder
// Validates: Requirements 1.5
type RecoveryStrategy string

const (
	// RecoveryStrategyPatternMatch - recovered by finding similar pattern in content
	RecoveryStrategyPatternMatch RecoveryStrategy = "pattern_match"
	// RecoveryStrategyLineBased - recovered by analyzing line structure
	RecoveryStrategyLineBased RecoveryStrategy = "line_based"
	// RecoveryStrategyContextBefore - recovered using context before the placeholder
	RecoveryStrategyContextBefore RecoveryStrategy = "context_before"
	// RecoveryStrategyContextAfter - recovered using context after the placeholder
	RecoveryStrategyContextAfter RecoveryStrategy = "context_after"
	// RecoveryStrategyAppend - fallback: appended at the end
	RecoveryStrategyAppend RecoveryStrategy = "append"
	// RecoveryStrategyFailed - recovery failed
	RecoveryStrategyFailed RecoveryStrategy = "failed"
)

// RecoveryAttempt represents a single recovery attempt for a placeholder
// Validates: Requirements 1.5
type RecoveryAttempt struct {
	Placeholder    string           // The placeholder being recovered
	Original       string           // The original content the placeholder represents
	Strategy       RecoveryStrategy // The strategy used for recovery
	InsertPosition int              // Position where the placeholder was inserted (-1 if appended)
	Success        bool             // Whether the recovery was successful
	Message        string           // Additional information about the recovery
}

// RecoveryReport tracks all recovery attempts and their results
// Validates: Requirements 1.5
type RecoveryReport struct {
	TotalMissing     int               // Total number of missing placeholders
	TotalRecovered   int               // Number of successfully recovered placeholders
	TotalFailed      int               // Number of failed recovery attempts
	Attempts         []RecoveryAttempt // Individual recovery attempts
	StrategiesUsed   map[RecoveryStrategy]int // Count of each strategy used
	ProcessingTimeMs int64             // Time taken for recovery in milliseconds
}

// NewRecoveryReport creates a new recovery report
func NewRecoveryReport() *RecoveryReport {
	return &RecoveryReport{
		Attempts:       make([]RecoveryAttempt, 0),
		StrategiesUsed: make(map[RecoveryStrategy]int),
	}
}

// AddAttempt adds a recovery attempt to the report
func (r *RecoveryReport) AddAttempt(attempt RecoveryAttempt) {
	r.Attempts = append(r.Attempts, attempt)
	r.StrategiesUsed[attempt.Strategy]++
	if attempt.Success {
		r.TotalRecovered++
	} else {
		r.TotalFailed++
	}
}

// LogReport logs the recovery report details
func (r *RecoveryReport) LogReport() {
	logger.Info("placeholder recovery report",
		logger.Int("totalMissing", r.TotalMissing),
		logger.Int("totalRecovered", r.TotalRecovered),
		logger.Int("totalFailed", r.TotalFailed),
		logger.Int64("processingTimeMs", r.ProcessingTimeMs))
	
	for strategy, count := range r.StrategiesUsed {
		logger.Debug("recovery strategy usage",
			logger.String("strategy", string(strategy)),
			logger.Int("count", count))
	}
	
	for _, attempt := range r.Attempts {
		if attempt.Success {
			logger.Debug("recovery attempt succeeded",
				logger.String("placeholder", attempt.Placeholder),
				logger.String("strategy", string(attempt.Strategy)),
				logger.Int("insertPosition", attempt.InsertPosition),
				logger.String("message", attempt.Message))
		} else {
			logger.Warn("recovery attempt failed",
				logger.String("placeholder", attempt.Placeholder),
				logger.String("strategy", string(attempt.Strategy)),
				logger.String("message", attempt.Message))
		}
	}
}

// recoverMissingPlaceholders attempts to recover missing placeholders using multiple strategies.
// This is a standalone function that can be used independently of PlaceholderSystem.
// Validates: Requirements 1.5
func recoverMissingPlaceholders(content string, placeholders map[string]string, missing []string) string {
	result, _ := recoverMissingPlaceholdersWithReport(content, "", placeholders, missing)
	return result
}

// recoverMissingPlaceholdersWithContext attempts to recover missing placeholders using context analysis.
// This is a standalone function that uses the original content for better recovery.
// Validates: Requirements 1.5
func recoverMissingPlaceholdersWithContext(content string, originalContent string, placeholders map[string]string, missing []string) string {
	result, _ := recoverMissingPlaceholdersWithReport(content, originalContent, placeholders, missing)
	return result
}

// recoverMissingPlaceholdersWithReport attempts to recover missing placeholders and returns a detailed report.
// This is the main recovery function that implements multiple strategies.
// Validates: Requirements 1.5
func recoverMissingPlaceholdersWithReport(content string, originalContent string, placeholders map[string]string, missing []string) (string, *RecoveryReport) {
	report := NewRecoveryReport()
	report.TotalMissing = len(missing)
	
	if len(missing) == 0 {
		return content, report
	}

	startTime := time.Now()
	
	// Sort missing placeholders by their number to maintain order
	sortedMissing := sortPlaceholdersByNumber(missing)

	result := content
	
	for _, placeholder := range sortedMissing {
		original, exists := placeholders[placeholder]
		if !exists {
			report.AddAttempt(RecoveryAttempt{
				Placeholder: placeholder,
				Strategy:    RecoveryStrategyFailed,
				Success:     false,
				Message:     "placeholder not found in mapping",
			})
			continue
		}

		// Try multiple recovery strategies in order of preference
		recovered, attempt := tryRecoveryStrategies(result, originalContent, placeholder, original)
		result = recovered
		report.AddAttempt(attempt)
	}

	report.ProcessingTimeMs = time.Since(startTime).Milliseconds()
	report.LogReport()
	
	return result, report
}

// sortPlaceholdersByNumber sorts placeholders by their numeric index
func sortPlaceholdersByNumber(missing []string) []string {
	sorted := make([]string, len(missing))
	copy(sorted, missing)
	
	sort.Slice(sorted, func(i, j int) bool {
		numI := extractPlaceholderNumber(sorted[i])
		numJ := extractPlaceholderNumber(sorted[j])
		return numI < numJ
	})
	
	return sorted
}

// extractPlaceholderNumber extracts the numeric index from a placeholder
func extractPlaceholderNumber(placeholder string) int {
	patterns := []string{
		"<<<LATEX_CMD_%d>>>",
		"<<<LATEX_MATH_%d>>>",
		"<<<LATEX_ENV_%d>>>",
		"<<<LATEX_TABLE_%d>>>",
		"<<<LATEX_COMMENT_%d>>>",
	}
	
	for _, pattern := range patterns {
		var num int
		if n, err := fmt.Sscanf(placeholder, pattern, &num); n == 1 && err == nil {
			return num
		}
	}
	return 0
}

// tryRecoveryStrategies attempts multiple recovery strategies for a single placeholder
func tryRecoveryStrategies(content string, originalContent string, placeholder string, original string) (string, RecoveryAttempt) {
	attempt := RecoveryAttempt{
		Placeholder: placeholder,
		Original:    original,
	}

	// Strategy 1: Pattern matching - look for similar LaTeX patterns
	if recovered, pos, ok := tryPatternMatchRecovery(content, placeholder, original); ok {
		attempt.Strategy = RecoveryStrategyPatternMatch
		attempt.InsertPosition = pos
		attempt.Success = true
		attempt.Message = fmt.Sprintf("found similar pattern at position %d", pos)
		logger.Debug("recovered placeholder using pattern match",
			logger.String("placeholder", placeholder),
			logger.Int("position", pos))
		return recovered, attempt
	}

	// Strategy 2: Line-based recovery - analyze line structure
	if recovered, pos, ok := tryLineBasedRecovery(content, originalContent, placeholder, original); ok {
		attempt.Strategy = RecoveryStrategyLineBased
		attempt.InsertPosition = pos
		attempt.Success = true
		attempt.Message = fmt.Sprintf("found appropriate line position at %d", pos)
		logger.Debug("recovered placeholder using line-based analysis",
			logger.String("placeholder", placeholder),
			logger.Int("position", pos))
		return recovered, attempt
	}

	// Strategy 3: Context before - use text before the placeholder in original
	if originalContent != "" {
		if recovered, pos, ok := tryContextBeforeRecovery(content, originalContent, placeholder, original); ok {
			attempt.Strategy = RecoveryStrategyContextBefore
			attempt.InsertPosition = pos
			attempt.Success = true
			attempt.Message = fmt.Sprintf("found context match before position %d", pos)
			logger.Debug("recovered placeholder using context before",
				logger.String("placeholder", placeholder),
				logger.Int("position", pos))
			return recovered, attempt
		}
	}

	// Strategy 4: Context after - use text after the placeholder in original
	if originalContent != "" {
		if recovered, pos, ok := tryContextAfterRecovery(content, originalContent, placeholder, original); ok {
			attempt.Strategy = RecoveryStrategyContextAfter
			attempt.InsertPosition = pos
			attempt.Success = true
			attempt.Message = fmt.Sprintf("found context match after position %d", pos)
			logger.Debug("recovered placeholder using context after",
				logger.String("placeholder", placeholder),
				logger.Int("position", pos))
			return recovered, attempt
		}
	}

	// Strategy 5: Fallback - append at the end
	recovered := content + " " + placeholder
	attempt.Strategy = RecoveryStrategyAppend
	attempt.InsertPosition = -1
	attempt.Success = true
	attempt.Message = "appended at end as fallback"
	logger.Warn("recovered placeholder by appending (all strategies failed)",
		logger.String("placeholder", placeholder))
	
	return recovered, attempt
}

// tryPatternMatchRecovery attempts to recover by finding similar LaTeX patterns
func tryPatternMatchRecovery(content string, placeholder string, original string) (string, int, bool) {
	// Extract the LaTeX command type from the original content
	cmdPattern := regexp.MustCompile(`^\\([a-zA-Z]+)`)
	matches := cmdPattern.FindStringSubmatch(original)
	
	if len(matches) < 2 {
		return "", 0, false
	}
	
	cmdName := matches[1]
	
	// Look for similar commands in the content and find a good insertion point
	// Find all occurrences of similar commands
	similarPattern := regexp.MustCompile(`\\` + regexp.QuoteMeta(cmdName) + `\b`)
	locs := similarPattern.FindAllStringIndex(content, -1)
	
	if len(locs) == 0 {
		return "", 0, false
	}
	
	// Insert after the last similar command (heuristic: related content is nearby)
	lastLoc := locs[len(locs)-1]
	insertPos := lastLoc[1]
	
	// Find the end of the current command (after closing brace if present)
	braceCount := 0
	for i := insertPos; i < len(content); i++ {
		if content[i] == '{' {
			braceCount++
		} else if content[i] == '}' {
			braceCount--
			if braceCount == 0 {
				insertPos = i + 1
				break
			}
		} else if braceCount == 0 && (content[i] == ' ' || content[i] == '\n') {
			insertPos = i
			break
		}
	}
	
	// Insert the placeholder
	result := content[:insertPos] + " " + placeholder + content[insertPos:]
	return result, insertPos, true
}

// tryLineBasedRecovery attempts to recover by analyzing line structure
func tryLineBasedRecovery(content string, originalContent string, placeholder string, original string) (string, int, bool) {
	if originalContent == "" {
		return "", 0, false
	}
	
	// Find the line number where the original content appears
	originalLines := strings.Split(originalContent, "\n")
	contentLines := strings.Split(content, "\n")
	
	targetLineNum := -1
	for i, line := range originalLines {
		if strings.Contains(line, original) {
			targetLineNum = i
			break
		}
	}
	
	if targetLineNum == -1 {
		return "", 0, false
	}
	
	// If the translated content has the same or similar number of lines,
	// try to insert at the corresponding line
	if len(contentLines) > 0 && targetLineNum < len(contentLines) {
		// Calculate the position in the content string
		pos := 0
		for i := 0; i < targetLineNum && i < len(contentLines); i++ {
			pos += len(contentLines[i]) + 1 // +1 for newline
		}
		
		// Find a good insertion point within the line
		lineEnd := pos + len(contentLines[targetLineNum])
		if lineEnd > len(content) {
			lineEnd = len(content)
		}
		
		// Insert at the end of the corresponding line
		result := content[:lineEnd] + " " + placeholder + content[lineEnd:]
		return result, lineEnd, true
	}
	
	return "", 0, false
}

// tryContextBeforeRecovery attempts to recover using context before the placeholder
func tryContextBeforeRecovery(content string, originalContent string, placeholder string, original string) (string, int, bool) {
	// Find the position of the original content in the original document
	originalPos := strings.Index(originalContent, original)
	if originalPos == -1 {
		return "", 0, false
	}
	
	// Get context before (up to 30 characters, but stop at newlines for better matching)
	contextStart := originalPos - 30
	if contextStart < 0 {
		contextStart = 0
	}
	
	contextBefore := originalContent[contextStart:originalPos]
	
	// Trim to last newline for cleaner matching
	if lastNewline := strings.LastIndex(contextBefore, "\n"); lastNewline != -1 {
		contextBefore = contextBefore[lastNewline+1:]
	}
	
	// Skip if context is too short
	if len(contextBefore) < 5 {
		return "", 0, false
	}
	
	// Find this context in the translated content
	contextPos := strings.Index(content, contextBefore)
	if contextPos == -1 {
		// Try with trimmed whitespace
		trimmedContext := strings.TrimSpace(contextBefore)
		if len(trimmedContext) >= 5 {
			contextPos = strings.Index(content, trimmedContext)
		}
	}
	
	if contextPos == -1 {
		return "", 0, false
	}
	
	// Insert after the context
	insertPos := contextPos + len(contextBefore)
	result := content[:insertPos] + placeholder + content[insertPos:]
	return result, insertPos, true
}

// tryContextAfterRecovery attempts to recover using context after the placeholder
func tryContextAfterRecovery(content string, originalContent string, placeholder string, original string) (string, int, bool) {
	// Find the position of the original content in the original document
	originalPos := strings.Index(originalContent, original)
	if originalPos == -1 {
		return "", 0, false
	}
	
	// Get context after (up to 30 characters, but stop at newlines for better matching)
	afterStart := originalPos + len(original)
	contextEnd := afterStart + 30
	if contextEnd > len(originalContent) {
		contextEnd = len(originalContent)
	}
	
	contextAfter := originalContent[afterStart:contextEnd]
	
	// Trim to first newline for cleaner matching
	if firstNewline := strings.Index(contextAfter, "\n"); firstNewline != -1 {
		contextAfter = contextAfter[:firstNewline]
	}
	
	// Skip if context is too short
	if len(contextAfter) < 5 {
		return "", 0, false
	}
	
	// Find this context in the translated content
	contextPos := strings.Index(content, contextAfter)
	if contextPos == -1 {
		// Try with trimmed whitespace
		trimmedContext := strings.TrimSpace(contextAfter)
		if len(trimmedContext) >= 5 {
			contextPos = strings.Index(content, trimmedContext)
		}
	}
	
	if contextPos == -1 {
		return "", 0, false
	}
	
	// Insert before the context
	insertPos := contextPos
	result := content[:insertPos] + placeholder + content[insertPos:]
	return result, insertPos, true
}

// buildSystemPrompt creates the system prompt for the translation task.
func buildSystemPrompt() string {
	return `You are a professional translator specializing in academic and scientific documents. 
Your task is to translate LaTeX documents from English to Chinese.

CRITICAL RULES:
1. Translate ONLY the natural language text content from English to Chinese.
2. PRESERVE ALL LaTeX commands exactly as they are, including:
   - Document structure commands: \section, \subsection, \chapter, \begin, \end, etc.
   - Formatting commands: \textbf, \textit, \emph, \underline, etc.
   - References: \ref, \cite, \label, \eqref, etc.
   - Mathematical environments: $...$, $$...$$, \[...\], \(...\), equation, align, etc.
   - All mathematical formulas and symbols inside math environments
   - Comments starting with %
   - Package imports and document class declarations
3. TRANSLATE comments: For lines starting with %, translate the text after % to Chinese, but keep the % symbol and line structure.
4. Keep the document structure and formatting intact.
5. Maintain proper Chinese punctuation (use Chinese quotation marks, periods, etc.).
6. Do not add any explanations or notes - output only the translated LaTeX content.
CRITICAL LINE STRUCTURE RULES (MUST FOLLOW):
7. PRESERVE THE EXACT LINE STRUCTURE of the original document:
   - Each \item MUST remain on its own separate line
   - Each \begin{...} MUST remain on its own separate line
   - Each \end{...} MUST remain on its own separate line
   - NEVER merge multiple lines into a single line
   - NEVER put \end{itemize} and \item on the same line
   - NEVER put \end{enumerate} and \item on the same line
   - The output MUST have approximately the same number of lines as the input
8. For list environments (itemize, enumerate, description):
   - Keep each \item on a separate line
   - Keep \begin{itemize/enumerate} on its own line
   - Keep \end{itemize/enumerate} on its own line
   - Translate the text content after \item, but keep the line structure`
}

// buildUserPrompt creates the user prompt with the content to translate.
func buildUserPrompt(content string) string {
	return fmt.Sprintf(`Translate the following LaTeX content from English to Chinese.

IMPORTANT REMINDERS:
- Preserve all LaTeX commands and mathematical formulas exactly as they are
- KEEP EACH LINE SEPARATE - do not merge lines together
- Each \item, \begin{...}, \end{...} must stay on its own line
- The translated output must have the same line structure as the input

LaTeX content to translate:
%s`, content)
}

// buildSystemPromptWithProtection creates the system prompt for protected translation.
// This prompt is used when LaTeX commands have been replaced with placeholders.
func buildSystemPromptWithProtection() string {
	return `You are a STRICT LaTeX document translator. You translate English to Chinese while preserving EXACT document structure.

## CRITICAL CONTEXT
- This input is a FRAGMENT of a larger LaTeX document, NOT a complete document
- You may see \begin{...} without matching \end{...} - THIS IS NORMAL
- You may see \end{...} without matching \begin{...} - THIS IS NORMAL
- You may see incomplete tables, figures, or environments - THIS IS NORMAL
- DO NOT try to "fix" or "complete" anything - just translate the text

## YOUR ROLE: FAITHFUL TRANSLATOR
- You are a translation tool, NOT an editor or improver
- Your output must be a MIRROR of the input structure with only text translated
- NEVER add, remove, or modify anything except translating English text to Chinese

## ABSOLUTE RULES (VIOLATION = CRITICAL FAILURE):

### Rule 1: COMMENT PRESERVATION (MOST CRITICAL)
- Lines starting with % are COMMENTS - they MUST stay as comments
- Lines NOT starting with % are CODE - they MUST stay as code
- NEVER add % to any line that doesn't have it in the input
- NEVER remove % from any line that has it in the input
- This rule has NO exceptions - even if the code looks "wrong" or "incomplete"

### Rule 2: PLACEHOLDER PRESERVATION (CRITICAL)
- Placeholders look like: <<<LATEX_CMD_0>>>, <<<LATEX_CMD_1>>>, etc.
- Copy each placeholder EXACTLY - character by character, position by position
- NEVER modify, translate, explain, or interpret placeholders
- NEVER change the number in a placeholder
- NEVER add spaces inside placeholders
- NEVER split a placeholder across lines

### Rule 3: STRUCTURE PRESERVATION (CRITICAL)
- Output MUST have EXACTLY the same number of lines as input
- Each line in output corresponds to the same line in input
- If input line N has a placeholder, output line N must have that SAME placeholder
- If input line starts with %, output line must start with %
- If input line is empty, output line must be empty
- NEVER merge multiple input lines into one output line
- NEVER split one input line into multiple output lines
- NEVER add new lines that don't exist in input
- NEVER remove lines that exist in input

### Rule 4: NO ADDITIONS OR MODIFICATIONS
- Do NOT add \end{document}, \end{table}, \end{tabular} or any LaTeX commands
- Do NOT add comments (% lines) that don't exist in input
- Do NOT remove comments that exist in input
- Do NOT add explanations, notes, or annotations
- Do NOT "fix", "complete", or "improve" anything
- Do NOT add content that wasn't in the original
- Even if you see \begin{table} without \end{table}, DO NOT add \end{table}
- Translate ONLY the English text, leave everything else UNCHANGED

### Rule 5: OUTPUT FORMAT
- Output ONLY the translated text
- No JSON, no code blocks, no markdown formatting
- No "Translation:" prefix or similar labels
- Start directly with the translated content
- End exactly where the input ends

## TRANSLATION GUIDELINES:
- Use proper Chinese punctuation: 。，、；：""''（）
- Maintain academic/formal tone
- Preserve technical terms when appropriate

## EXAMPLE:
Input (3 lines):
The quick brown fox
<<<LATEX_CMD_0>>>
jumps over the lazy dog.

Output (MUST be exactly 3 lines):
敏捷的棕色狐狸
<<<LATEX_CMD_0>>>
跳过了懒狗。`
}

// buildUserPromptWithProtection creates the user prompt for protected translation.
func buildUserPromptWithProtection(content string, placeholderCount int) string {
	if placeholderCount == 0 {
		return fmt.Sprintf(`Translate to Chinese. Keep the same line structure.

%s`, content)
	}

	return fmt.Sprintf(`Translate to Chinese. This text contains %d placeholders (<<<LATEX_CMD_N>>> format).

CRITICAL: Copy every placeholder EXACTLY as shown. Do not modify any placeholder.

Example of correct handling:
- Input: "The equation <<<LATEX_CMD_0>>> shows that..."
- Output: "方程 <<<LATEX_CMD_0>>> 表明..."

Now translate:

%s`, placeholderCount, content)
}

// handleAPIHTTPError creates an appropriate AppError based on the HTTP status code and response body.
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
// LaTeX-Aware Chunking
// =============================================================================
// The following functions implement intelligent chunking that respects LaTeX
// environment boundaries to prevent splitting tables, figures, and other
// environments across chunks.
// =============================================================================

// environmentBoundary represents a LaTeX environment boundary
type environmentBoundary struct {
	envName string
	start   int // position of \begin{...}
	end     int // position after \end{...}
}

// findEnvironmentBoundaries finds all complete LaTeX environments in the content
func findEnvironmentBoundaries(content string) []environmentBoundary {
	var boundaries []environmentBoundary

	// Environments that should be kept together
	protectedEnvs := []string{
		"table", "table*", "tabular", "tabular*", "tabularx",
		"figure", "figure*",
		"equation", "equation*", "align", "align*", "gather", "gather*",
		"tikzpicture", "algorithm", "algorithmic",
		"lstlisting", "verbatim", "minted",
		"itemize", "enumerate", "description",
		"theorem", "lemma", "proof", "definition", "corollary",
		"abstract", "quote", "quotation",
	}

	for _, envName := range protectedEnvs {
		beginTag := `\begin{` + envName + `}`

		startIdx := 0
		for {
			beginPos := strings.Index(content[startIdx:], beginTag)
			if beginPos == -1 {
				break
			}
			beginPos += startIdx

			// Find matching end tag (handle nesting)
			endPos := findMatchingEnd(content, beginPos, envName)
			if endPos == -1 {
				startIdx = beginPos + len(beginTag)
				continue
			}

			boundaries = append(boundaries, environmentBoundary{
				envName: envName,
				start:   beginPos,
				end:     endPos,
			})

			startIdx = endPos
		}
	}

	// Sort by start position
	sort.Slice(boundaries, func(i, j int) bool {
		return boundaries[i].start < boundaries[j].start
	})

	return boundaries
}

// findMatchingEnd finds the matching \end{envName} for a \begin{envName}
func findMatchingEnd(content string, beginPos int, envName string) int {
	beginTag := `\begin{` + envName + `}`
	endTag := `\end{` + envName + `}`

	depth := 1
	searchPos := beginPos + len(beginTag)

	for depth > 0 && searchPos < len(content) {
		nextBegin := strings.Index(content[searchPos:], beginTag)
		nextEnd := strings.Index(content[searchPos:], endTag)

		if nextEnd == -1 {
			return -1 // No matching end found
		}

		if nextBegin != -1 && nextBegin < nextEnd {
			depth++
			searchPos += nextBegin + len(beginTag)
		} else {
			depth--
			if depth == 0 {
				return searchPos + nextEnd + len(endTag)
			}
			searchPos += nextEnd + len(endTag)
		}
	}

	return -1
}

// isInsideEnvironment checks if a position is inside any protected environment
func isInsideEnvironment(pos int, boundaries []environmentBoundary) bool {
	for _, b := range boundaries {
		if pos >= b.start && pos < b.end {
			return true
		}
	}
	return false
}

// findSafeBreakPoint finds a break point that doesn't split any environment
func findSafeBreakPoint(content string, targetPos int, boundaries []environmentBoundary) int {
	// If target position is not inside any environment, it's safe
	if !isInsideEnvironment(targetPos, boundaries) {
		return targetPos
	}

	// Find the environment we're inside and return its end position
	for _, b := range boundaries {
		if targetPos >= b.start && targetPos < b.end {
			// We're inside this environment, need to include the whole thing
			// But if the environment is too large, we need to break before it
			return b.start
		}
	}

	return targetPos
}

// splitIntoChunks splits content into chunks suitable for translation.
// It tries to split at natural boundaries (paragraphs, sections) to maintain context.
// Each chunk will be at most maxSize characters.
//
// The function uses the following strategy:
// 1. First, identify all LaTeX environments that should not be split
// 2. Try to split by LaTeX section commands (\section, \subsection, etc.)
// 3. If sections are too large, split by paragraphs (double newlines)
// 4. Ensure no split occurs inside a protected environment
// 5. As a last resort, split by character count at safe positions
func splitIntoChunks(content string, maxSize int) []string {
	if len(content) <= maxSize {
		return []string{content}
	}

	// Find all environment boundaries first
	boundaries := findEnvironmentBoundaries(content)

	// Try to split by sections first, respecting environment boundaries
	chunks := splitBySectionsWithEnvProtection(content, maxSize, boundaries)
	if len(chunks) > 1 {
		return chunks
	}

	// Fall back to splitting by paragraphs, respecting environment boundaries
	chunks = splitByParagraphsWithEnvProtection(content, maxSize, boundaries)
	if len(chunks) > 1 {
		return chunks
	}

	// Fall back to splitting by size with environment protection
	return splitBySizeWithEnvProtection(content, maxSize, boundaries)
}

// sectionPattern matches LaTeX section commands
var sectionPattern = regexp.MustCompile(`(?m)^\\(section|subsection|subsubsection|chapter|part)\s*[\[{]`)

// splitBySectionsWithEnvProtection splits content by LaTeX section commands while protecting environments.
func splitBySectionsWithEnvProtection(content string, maxSize int, boundaries []environmentBoundary) []string {
	// Find all section command positions
	matches := sectionPattern.FindAllStringIndex(content, -1)
	if len(matches) < 2 {
		return []string{content}
	}

	var chunks []string
	var currentChunk strings.Builder
	lastEnd := 0

	for _, match := range matches {
		sectionStart := match[0]

		// Skip if this section start is inside a protected environment
		if isInsideEnvironment(sectionStart, boundaries) {
			continue
		}

		// Get content before this section
		beforeSection := content[lastEnd:sectionStart]

		// Check if adding this content would exceed maxSize
		if currentChunk.Len()+len(beforeSection) > maxSize && currentChunk.Len() > 0 {
			// Save current chunk and start a new one
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
		}

		currentChunk.WriteString(beforeSection)
		lastEnd = sectionStart
	}

	// Add remaining content
	remaining := content[lastEnd:]
	if currentChunk.Len()+len(remaining) > maxSize && currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
		currentChunk.Reset()
	}
	currentChunk.WriteString(remaining)

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	// If we still have chunks that are too large, split them further
	var result []string
	for _, chunk := range chunks {
		if len(chunk) > maxSize {
			// Re-find boundaries for this chunk
			chunkBoundaries := findEnvironmentBoundaries(chunk)
			result = append(result, splitByParagraphsWithEnvProtection(chunk, maxSize, chunkBoundaries)...)
		} else {
			result = append(result, chunk)
		}
	}

	return result
}

// splitByParagraphsWithEnvProtection splits content by paragraph boundaries while protecting environments.
func splitByParagraphsWithEnvProtection(content string, maxSize int, boundaries []environmentBoundary) []string {
	// Find paragraph boundaries (double newlines)
	paragraphPattern := regexp.MustCompile(`\n\s*\n`)
	paragraphMatches := paragraphPattern.FindAllStringIndex(content, -1)

	if len(paragraphMatches) < 1 {
		return splitBySizeWithEnvProtection(content, maxSize, boundaries)
	}

	var chunks []string
	var currentChunk strings.Builder
	lastEnd := 0

	for _, match := range paragraphMatches {
		paragraphEnd := match[1]

		// Skip if this break point is inside a protected environment
		if isInsideEnvironment(match[0], boundaries) {
			continue
		}

		// Get content up to this paragraph break
		segment := content[lastEnd:paragraphEnd]

		// Check if adding this content would exceed maxSize
		if currentChunk.Len()+len(segment) > maxSize && currentChunk.Len() > 0 {
			// Save current chunk and start a new one
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
		}

		currentChunk.WriteString(segment)
		lastEnd = paragraphEnd
	}

	// Add remaining content
	remaining := content[lastEnd:]
	if currentChunk.Len()+len(remaining) > maxSize && currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
		currentChunk.Reset()
	}
	currentChunk.WriteString(remaining)

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	// If we still have chunks that are too large, split them further
	var result []string
	for _, chunk := range chunks {
		if len(chunk) > maxSize {
			chunkBoundaries := findEnvironmentBoundaries(chunk)
			result = append(result, splitBySizeWithEnvProtection(chunk, maxSize, chunkBoundaries)...)
		} else {
			result = append(result, chunk)
		}
	}

	return result
}

// splitBySizeWithEnvProtection splits content by size while protecting environments.
func splitBySizeWithEnvProtection(content string, maxSize int, boundaries []environmentBoundary) []string {
	if len(content) <= maxSize {
		return []string{content}
	}

	var chunks []string
	remaining := content
	offset := 0

	for len(remaining) > 0 {
		if len(remaining) <= maxSize {
			chunks = append(chunks, remaining)
			break
		}

		// Find a good break point within maxSize
		targetBreak := findBreakPoint(remaining, maxSize)

		// Adjust boundaries for the current offset
		adjustedBoundaries := make([]environmentBoundary, 0)
		for _, b := range boundaries {
			if b.start >= offset && b.start < offset+len(remaining) {
				adjustedBoundaries = append(adjustedBoundaries, environmentBoundary{
					envName: b.envName,
					start:   b.start - offset,
					end:     b.end - offset,
				})
			}
		}

		// Check if break point is inside an environment
		safeBreak := findSafeBreakPoint(remaining, targetBreak, adjustedBoundaries)

		// If safe break is 0 (meaning we're at the start of a large environment),
		// we need to include the entire environment even if it exceeds maxSize
		if safeBreak == 0 {
			// Find the end of the environment we're trying to avoid splitting
			for _, b := range adjustedBoundaries {
				if b.start == 0 {
					safeBreak = b.end
					break
				}
			}
			// If still 0, just use the target break (shouldn't happen)
			if safeBreak == 0 {
				safeBreak = targetBreak
			}
		}

		chunks = append(chunks, remaining[:safeBreak])
		remaining = remaining[safeBreak:]
		offset += safeBreak
	}

	return chunks
}

// splitBySections splits content by LaTeX section commands (legacy function for compatibility).
func splitBySections(content string, maxSize int) []string {
	boundaries := findEnvironmentBoundaries(content)
	return splitBySectionsWithEnvProtection(content, maxSize, boundaries)
}

// splitByParagraphs splits content by paragraph boundaries (legacy function for compatibility).
func splitByParagraphs(content string, maxSize int) []string {
	boundaries := findEnvironmentBoundaries(content)
	return splitByParagraphsWithEnvProtection(content, maxSize, boundaries)
}

// splitBySize splits content by size (legacy function for compatibility).
func splitBySize(content string, maxSize int) []string {
	boundaries := findEnvironmentBoundaries(content)
	return splitBySizeWithEnvProtection(content, maxSize, boundaries)
}

// findBreakPoint finds a good position to break the text, preferring sentence boundaries.
func findBreakPoint(text string, maxSize int) int {
	if len(text) <= maxSize {
		return len(text)
	}

	// Look for sentence endings within the last 20% of maxSize
	searchStart := maxSize * 80 / 100
	searchEnd := maxSize

	// Sentence ending patterns (period, question mark, exclamation followed by space or newline)
	sentenceEndings := []string{". ", ".\n", "? ", "?\n", "! ", "!\n", "。", "？", "！"}

	bestBreak := -1
	for _, ending := range sentenceEndings {
		idx := strings.LastIndex(text[searchStart:searchEnd], ending)
		if idx != -1 {
			breakPos := searchStart + idx + len(ending)
			if breakPos > bestBreak {
				bestBreak = breakPos
			}
		}
	}

	if bestBreak > 0 {
		return bestBreak
	}

	// Look for newlines as secondary break points
	idx := strings.LastIndex(text[searchStart:searchEnd], "\n")
	if idx != -1 {
		return searchStart + idx + 1
	}

	// Look for spaces as last resort
	idx = strings.LastIndex(text[:maxSize], " ")
	if idx != -1 && idx > maxSize/2 {
		return idx + 1
	}

	// Hard break at maxSize if no good break point found
	return maxSize
}

// =============================================================================
// LaTeX Command Protection Logic
// =============================================================================
// The following functions implement LaTeX command extraction and protection
// to ensure that LaTeX commands and mathematical formulas are preserved
// during translation.
//
// Validates: Requirements 3.2
// Property 4: LaTeX 命令保留（翻译不变量）
// =============================================================================

// LaTeXCommandType represents the type of LaTeX command
type LaTeXCommandType int

const (
	// CommandTypeMath represents mathematical environments ($...$, $$...$$, \[...\], \(...\), etc.)
	CommandTypeMath LaTeXCommandType = iota
	// CommandTypeStructure represents document structure commands (\section, \begin, \end, etc.)
	CommandTypeStructure
	// CommandTypeReference represents reference commands (\ref, \cite, \label, etc.)
	CommandTypeReference
	// CommandTypeFormatting represents formatting commands (\textbf, \textit, etc.)
	CommandTypeFormatting
	// CommandTypeOther represents other LaTeX commands
	CommandTypeOther
)

// LaTeXCommand represents an extracted LaTeX command with its position and type
type LaTeXCommand struct {
	// Command is the full text of the LaTeX command
	Command string
	// Start is the starting position in the original text
	Start int
	// End is the ending position in the original text
	End int
	// Type is the type of the command
	Type LaTeXCommandType
}

// Regular expressions for matching different types of LaTeX commands
var (
	// Math environment patterns
	// Inline math: $...$
	inlineMathPattern = regexp.MustCompile(`\$[^$]+\$`)
	// Display math: $$...$$
	displayMathPattern = regexp.MustCompile(`\$\$[^$]+\$\$`)
	// Bracket math: \[...\]
	bracketMathPattern = regexp.MustCompile(`\\\[[\s\S]*?\\\]`)
	// Parenthesis math: \(...\)
	parenMathPattern = regexp.MustCompile(`\\\([\s\S]*?\\\)`)
	// Named math environments - we'll handle these with a custom function since Go regex doesn't support backreferences
	// This pattern matches the opening tag only, we'll find the matching end tag programmatically
	mathEnvNames = []string{"equation", "align", "alignat", "gather", "multline", "flalign", "eqnarray", "math", "displaymath"}

	// Complex environments that should be protected entirely (not just begin/end tags)
	// These environments contain content that should NOT be translated
	protectedEnvNames = []string{
		// Math environments
		"equation", "align", "alignat", "gather", "multline", "flalign", "eqnarray", "math", "displaymath",
		"equation*", "align*", "alignat*", "gather*", "multline*", "flalign*",
		// TikZ environments
		"tikzpicture", "tikzcd", "pgfpicture",
		// Table environments (structure should be preserved)
		"tabular", "tabular*", "tabularx", "longtable", "array",
		// Algorithm environments
		"algorithm", "algorithmic", "algorithm2e",
		// Verbatim environments
		"verbatim", "lstlisting", "minted", "Verbatim",
		// Other technical environments
		"proof", "cases", "matrix", "pmatrix", "bmatrix", "vmatrix", "Vmatrix",
		"split", "subequations",
	}
	
	// Float environments that need caption translation (not currently used)
	floatEnvNames = []string{"table", "table*", "figure", "figure*", "sidewaystable", "sidewaysfigure"}

	// Document structure command patterns
	// Section commands: \section{...}, \subsection{...}, etc.
	sectionCommandPattern = regexp.MustCompile(`\\(section|subsection|subsubsection|chapter|part|paragraph|subparagraph)\*?\s*(\[[^\]]*\])?\s*\{[^}]*\}`)
	// Begin/end environments: \begin{...}, \end{...}
	beginEndPattern = regexp.MustCompile(`\\(begin|end)\s*\{[^}]+\}`)
	// Document class and packages: \documentclass{...}, \usepackage{...}
	documentClassPattern = regexp.MustCompile(`\\(documentclass|usepackage|RequirePackage)(\[[^\]]*\])?\s*\{[^}]*\}`)
	// Input/include commands: \input{...}, \include{...}
	inputIncludePattern = regexp.MustCompile(`\\(input|include|includeonly|bibliography|bibliographystyle)\s*\{[^}]*\}`)

	// Reference command patterns
	// References: \ref{...}, \eqref{...}, \pageref{...}
	refPattern = regexp.MustCompile(`\\(ref|eqref|pageref|autoref|cref|Cref|nameref)\s*\{[^}]*\}`)
	// Citations: \cite{...}, \citep{...}, \citet{...}
	citePattern = regexp.MustCompile(`\\(cite|citep|citet|citeauthor|citeyear|citealt|citealp|nocite)(\[[^\]]*\])*\s*\{[^}]*\}`)
	// Labels: \label{...}
	labelPattern = regexp.MustCompile(`\\label\s*\{[^}]*\}`)

	// Formatting command patterns
	// Text formatting: \textbf{...}, \textit{...}, \emph{...}, etc.
	textFormattingPattern = regexp.MustCompile(`\\(textbf|textit|texttt|textrm|textsf|textsc|emph|underline|overline|textcolor)\s*(\{[^}]*\})?\s*\{[^}]*\}`)
	// Font size commands: \tiny, \small, \large, etc.
	fontSizePattern = regexp.MustCompile(`\\(tiny|scriptsize|footnotesize|small|normalsize|large|Large|LARGE|huge|Huge)\b`)
	// Font style commands: \bfseries, \itshape, etc.
	fontStylePattern = regexp.MustCompile(`\\(bfseries|itshape|ttfamily|rmfamily|sffamily|scshape|upshape|slshape|mdseries)\b`)

	// Other common LaTeX commands
	// Newline and spacing: \\, \newline, \hspace, \vspace, etc.
	spacingPattern = regexp.MustCompile(`\\(newline|linebreak|pagebreak|newpage|clearpage|hspace|vspace|hfill|vfill|quad|qquad|,|;|!)\*?(\{[^}]*\})?`)
	// Double backslash for line breaks
	doubleBackslashPattern = regexp.MustCompile(`\\\\`)
	// Special characters: \%, \$, \&, \#, \_, \{, \}
	specialCharPattern = regexp.MustCompile(`\\[%$&#_{}~^]`)
	// Generic commands with arguments: \command{...} or \command[...]{...}
	genericCommandPattern = regexp.MustCompile(`\\[a-zA-Z]+\*?(\[[^\]]*\])*(\{[^}]*\})*`)
	// Comments: % ... (to end of line)
	commentPattern = regexp.MustCompile(`%[^\n]*`)
)

// ExtractLaTeXCommands extracts all LaTeX commands from the given content.
// It returns a slice of LaTeXCommand structs containing the command text,
// position, and type.
//
// The function identifies the following types of commands:
// - Math environments: $...$, $$...$$, \[...\], \(...\), equation, align, etc.
// - Document structure: \section, \begin, \end, \documentclass, etc.
// - References: \ref, \cite, \label, etc.
// - Formatting: \textbf, \textit, \emph, etc.
// - Other: spacing commands, special characters, generic commands
//
// Validates: Requirements 3.2
func ExtractLaTeXCommands(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	// Extract protected environments first (highest priority - entire content preserved)
	commands = append(commands, extractProtectedEnvironments(content)...)

	// Extract math environments (they have high priority)
	commands = append(commands, extractMathCommands(content)...)

	// Extract document structure commands
	commands = append(commands, extractStructureCommands(content)...)

	// Extract reference commands
	commands = append(commands, extractReferenceCommands(content)...)

	// Extract formatting commands
	commands = append(commands, extractFormattingCommands(content)...)

	// Extract other commands
	commands = append(commands, extractOtherCommands(content)...)

	// Sort commands by position and remove duplicates/overlaps
	commands = deduplicateAndSortCommands(commands)

	return commands
}

// extractProtectedEnvironments extracts complete environments that should not be translated.
// This includes TikZ, tables, algorithms, verbatim, and other technical environments.
func extractProtectedEnvironments(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	for _, envName := range protectedEnvNames {
		// Match both starred and non-starred versions
		for _, starred := range []string{"", "*"} {
			actualEnvName := envName
			if starred == "*" && !strings.HasSuffix(envName, "*") {
				actualEnvName = envName + "*"
			} else if starred == "*" {
				continue // Already has *, skip
			}

			beginTag := fmt.Sprintf("\\begin{%s}", actualEnvName)
			// endTag is used in findMatchingEndTag

			startIdx := 0
			for {
				// Find the begin tag
				beginPos := strings.Index(content[startIdx:], beginTag)
				if beginPos == -1 {
					break
				}
				beginPos += startIdx

				// Find the matching end tag (handle nested environments)
				endPos := findMatchingEndTag(content, beginPos, actualEnvName)
				if endPos == -1 {
					break
				}

				commands = append(commands, LaTeXCommand{
					Command: content[beginPos:endPos],
					Start:   beginPos,
					End:     endPos,
					Type:    CommandTypeMath, // Use Math type for protected environments
				})

				startIdx = endPos
			}
		}
	}

	return commands
}

// findMatchingEndTag finds the matching \end{envName} for a \begin{envName} at startPos.
// It handles nested environments of the same type.
func findMatchingEndTag(content string, startPos int, envName string) int {
	beginTag := fmt.Sprintf("\\begin{%s}", envName)
	endTag := fmt.Sprintf("\\end{%s}", envName)

	depth := 1
	searchPos := startPos + len(beginTag)

	for depth > 0 && searchPos < len(content) {
		nextBegin := strings.Index(content[searchPos:], beginTag)
		nextEnd := strings.Index(content[searchPos:], endTag)

		if nextEnd == -1 {
			// No matching end tag found
			return -1
		}

		if nextBegin != -1 && nextBegin < nextEnd {
			// Found another begin before end, increase depth
			depth++
			searchPos += nextBegin + len(beginTag)
		} else {
			// Found end tag
			depth--
			if depth == 0 {
				return searchPos + nextEnd + len(endTag)
			}
			searchPos += nextEnd + len(endTag)
		}
	}

	return -1
}

// extractMathCommands extracts all mathematical environment commands.
func extractMathCommands(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	// Extract named math environments first (they can contain other patterns)
	commands = append(commands, extractNamedMathEnvironments(content)...)

	// Extract display math ($$...$$) before inline math ($...$)
	for _, match := range displayMathPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeMath,
		})
	}

	// Extract bracket math \[...\]
	for _, match := range bracketMathPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeMath,
		})
	}

	// Extract parenthesis math \(...\)
	for _, match := range parenMathPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeMath,
		})
	}

	// Extract inline math ($...$)
	for _, match := range inlineMathPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeMath,
		})
	}

	return commands
}

// extractNamedMathEnvironments extracts named math environments like \begin{equation}...\end{equation}
// This function handles the matching manually since Go's regexp doesn't support backreferences.
func extractNamedMathEnvironments(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	for _, envName := range mathEnvNames {
		// Match both starred and non-starred versions
		for _, starred := range []string{"", "*"} {
			beginTag := fmt.Sprintf("\\begin{%s%s}", envName, starred)
			endTag := fmt.Sprintf("\\end{%s%s}", envName, starred)

			startIdx := 0
			for {
				// Find the begin tag
				beginPos := strings.Index(content[startIdx:], beginTag)
				if beginPos == -1 {
					break
				}
				beginPos += startIdx

				// Find the matching end tag
				endPos := strings.Index(content[beginPos:], endTag)
				if endPos == -1 {
					break
				}
				endPos += beginPos + len(endTag)

				commands = append(commands, LaTeXCommand{
					Command: content[beginPos:endPos],
					Start:   beginPos,
					End:     endPos,
					Type:    CommandTypeMath,
				})

				startIdx = endPos
			}
		}
	}

	return commands
}

// extractStructureCommands extracts document structure commands.
func extractStructureCommands(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	// Section commands
	for _, match := range sectionCommandPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeStructure,
		})
	}

	// Begin/end environments
	for _, match := range beginEndPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeStructure,
		})
	}

	// Document class and packages
	for _, match := range documentClassPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeStructure,
		})
	}

	// Input/include commands
	for _, match := range inputIncludePattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeStructure,
		})
	}

	return commands
}

// extractReferenceCommands extracts reference-related commands.
func extractReferenceCommands(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	// References
	for _, match := range refPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeReference,
		})
	}

	// Citations
	for _, match := range citePattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeReference,
		})
	}

	// Labels
	for _, match := range labelPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeReference,
		})
	}

	return commands
}

// extractFormattingCommands extracts text formatting commands.
func extractFormattingCommands(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	// Text formatting
	for _, match := range textFormattingPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeFormatting,
		})
	}

	// Font size commands
	for _, match := range fontSizePattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeFormatting,
		})
	}

	// Font style commands
	for _, match := range fontStylePattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeFormatting,
		})
	}

	return commands
}

// extractOtherCommands extracts other LaTeX commands (spacing, special chars, etc.).
func extractOtherCommands(content string) []LaTeXCommand {
	var commands []LaTeXCommand

	// Comments
	for _, match := range commentPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeOther,
		})
	}

	// Double backslash (line breaks)
	for _, match := range doubleBackslashPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeOther,
		})
	}

	// Spacing commands
	for _, match := range spacingPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeOther,
		})
	}

	// Special characters
	for _, match := range specialCharPattern.FindAllStringIndex(content, -1) {
		commands = append(commands, LaTeXCommand{
			Command: content[match[0]:match[1]],
			Start:   match[0],
			End:     match[1],
			Type:    CommandTypeOther,
		})
	}

	return commands
}

// deduplicateAndSortCommands removes overlapping commands and sorts by position.
// When commands overlap, the longer (more specific) command is kept.
func deduplicateAndSortCommands(commands []LaTeXCommand) []LaTeXCommand {
	if len(commands) == 0 {
		return commands
	}

	// Sort by start position, then by length (longer first)
	sortCommands(commands)

	// Remove overlapping commands (keep the first one, which is longer due to sorting)
	var result []LaTeXCommand
	for _, cmd := range commands {
		overlaps := false
		for _, existing := range result {
			if commandsOverlap(existing, cmd) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, cmd)
		}
	}

	return result
}

// sortCommands sorts commands by start position, then by length (longer first).
func sortCommands(commands []LaTeXCommand) {
	// Simple bubble sort (sufficient for typical document sizes)
	n := len(commands)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			// Sort by start position first
			if commands[j].Start > commands[j+1].Start {
				commands[j], commands[j+1] = commands[j+1], commands[j]
			} else if commands[j].Start == commands[j+1].Start {
				// If same start, longer command comes first
				if len(commands[j].Command) < len(commands[j+1].Command) {
					commands[j], commands[j+1] = commands[j+1], commands[j]
				}
			}
		}
	}
}

// commandsOverlap checks if two commands overlap in position.
func commandsOverlap(a, b LaTeXCommand) bool {
	return !(a.End <= b.Start || b.End <= a.Start)
}

// GetLaTeXCommandStrings extracts all LaTeX commands from content and returns them as strings.
// This is a convenience function that returns just the command strings without position info.
//
// Validates: Requirements 3.2
func GetLaTeXCommandStrings(content string) []string {
	commands := ExtractLaTeXCommands(content)
	result := make([]string, len(commands))
	for i, cmd := range commands {
		result[i] = cmd.Command
	}
	return result
}

// TranslateCaptionsInFloats finds all float environments (table, figure, etc.) and translates
// only the \caption content within them. The rest of the float environment is preserved as-is.
// This is called BEFORE the main translation to handle captions in protected environments.
//
// Parameters:
//   - content: the LaTeX content to process
//   - translateFunc: a function that translates a single caption text
//
// Returns:
//   - the content with translated captions
//   - error if translation fails
func TranslateCaptionsInFloats(content string, translateFunc func(string) (string, error)) (string, error) {
	result := content
	
	// Pattern to match \caption{...} or \caption[short]{long}
	// We need to handle nested braces in the caption content
	captionPattern := regexp.MustCompile(`\\caption\s*(\[[^\]]*\])?\s*\{`)
	
	// Find all float environments and process captions within them
	for _, envName := range floatEnvNames {
		beginTag := fmt.Sprintf("\\begin{%s}", envName)
		// endTag is used internally by findMatchingEndTag
		
		searchPos := 0
		for {
			// Find the begin tag
			beginPos := strings.Index(result[searchPos:], beginTag)
			if beginPos == -1 {
				break
			}
			beginPos += searchPos
			
			// Find the matching end tag
			endPos := findMatchingEndTag(result, beginPos, envName)
			if endPos == -1 {
				searchPos = beginPos + len(beginTag)
				continue
			}
			
			// Extract the float environment content
			floatContent := result[beginPos:endPos]
			
			// Find and translate captions within this float
			translatedFloat, err := translateCaptionsInContent(floatContent, captionPattern, translateFunc)
			if err != nil {
				return "", err
			}
			
			// Replace the float content with translated version
			result = result[:beginPos] + translatedFloat + result[endPos:]
			
			// Move search position past this float
			searchPos = beginPos + len(translatedFloat)
		}
	}
	
	return result, nil
}

// translateCaptionsInContent translates all \caption commands within the given content.
func translateCaptionsInContent(content string, captionPattern *regexp.Regexp, translateFunc func(string) (string, error)) (string, error) {
	result := content
	offset := 0
	
	matches := captionPattern.FindAllStringIndex(content, -1)
	for _, match := range matches {
		captionStart := match[0] + offset
		
		// Find the opening brace position
		braceStart := strings.Index(result[captionStart:], "{")
		if braceStart == -1 {
			continue
		}
		braceStart += captionStart
		
		// Find the matching closing brace
		braceEnd := findMatchingBrace(result, braceStart)
		if braceEnd == -1 {
			continue
		}
		
		// Extract the caption text (without braces)
		captionText := result[braceStart+1 : braceEnd]
		
		// Skip if caption is empty or only contains LaTeX commands
		if strings.TrimSpace(captionText) == "" {
			continue
		}
		
		// Check if caption contains any translatable text (not just commands/refs)
		if !containsTranslatableText(captionText) {
			continue
		}
		
		// Translate the caption text
		translatedCaption, err := translateFunc(captionText)
		if err != nil {
			logger.Warn("failed to translate caption, keeping original",
				logger.Err(err),
				logger.String("caption", truncateString(captionText, 50)))
			continue
		}
		
		// Replace the caption text
		newCaption := result[:braceStart+1] + translatedCaption + result[braceEnd:]
		
		// Update offset for subsequent matches
		offset += len(translatedCaption) - len(captionText)
		result = newCaption
	}
	
	return result, nil
}

// findMatchingBrace finds the position of the closing brace that matches the opening brace at pos.
func findMatchingBrace(content string, pos int) int {
	if pos >= len(content) || content[pos] != '{' {
		return -1
	}
	
	depth := 1
	for i := pos + 1; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		case '\\':
			// Skip escaped characters
			if i+1 < len(content) {
				i++
			}
		}
	}
	return -1
}

// containsTranslatableText checks if the text contains any content that should be translated.
// Returns false if the text only contains LaTeX commands, references, or math.
func containsTranslatableText(text string) bool {
	// Remove common LaTeX commands and check if anything remains
	cleaned := text
	
	// Remove \label{...}
	cleaned = regexp.MustCompile(`\\label\{[^}]*\}`).ReplaceAllString(cleaned, "")
	// Remove \ref{...}, \eqref{...}, etc.
	cleaned = regexp.MustCompile(`\\(?:ref|eqref|pageref|autoref|cref|Cref)\{[^}]*\}`).ReplaceAllString(cleaned, "")
	// Remove \cite{...}
	cleaned = regexp.MustCompile(`\\cite\w*(\[[^\]]*\])*\{[^}]*\}`).ReplaceAllString(cleaned, "")
	// Remove inline math $...$
	cleaned = regexp.MustCompile(`\$[^$]+\$`).ReplaceAllString(cleaned, "")
	// Remove \textbf{}, \textit{}, etc. but keep their content
	cleaned = regexp.MustCompile(`\\(?:textbf|textit|texttt|emph)\{([^}]*)\}`).ReplaceAllString(cleaned, "$1")
	
	// Check if any alphabetic characters remain
	return regexp.MustCompile(`[a-zA-Z]`).MatchString(cleaned)
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ProtectLaTeXCommands replaces LaTeX commands with placeholders for safe translation.
// It returns the modified content and a map of placeholders to original commands.
//
// The placeholders are in the format: <<<LATEX_CMD_N>>> where N is a unique number.
// Math environments use the format: <<<LATEX_MATH_N>>> for better identification.
// This format is chosen to be unlikely to appear in normal text and to be preserved
// by translation APIs.
//
// This function now provides comprehensive math environment protection including:
// - Inline math: $...$ and \(...\)
// - Display math: $$...$$ and \[...\]
// - Math environments: equation, align, gather, multline, eqnarray, etc.
//
// Validates: Requirements 1.3, 1.4, 3.2
func ProtectLaTeXCommands(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	
	// Step 0: Protect author and title blocks first (they contain proper nouns)
	// Author names should NEVER be translated
	protectedContent, authorPlaceholders := ProtectAuthorBlock(content)
	for k, v := range authorPlaceholders {
		placeholders[k] = v
	}
	
	logger.Debug("protected author blocks",
		logger.Int("authorPlaceholders", len(authorPlaceholders)))
	
	// Step 1: Protect table environments first (highest priority)
	// Tables contain complex structures that should not be translated
	// This includes tabular, table, longtable, array, etc.
	protectedContent, tablePlaceholders := ProtectTableStructures(protectedContent)
	
	// Merge table placeholders into the main placeholders map
	for k, v := range tablePlaceholders {
		placeholders[k] = v
	}
	
	logger.Debug("protected table structures",
		logger.Int("tablePlaceholders", len(tablePlaceholders)))
	
	// Step 2: Protect math environments with dedicated math placeholders
	// This ensures complete math expressions are protected as single units
	protectedContent, mathPlaceholders := ProtectMathEnvironments(protectedContent)
	
	// Merge math placeholders into the main placeholders map
	for k, v := range mathPlaceholders {
		placeholders[k] = v
	}
	
	// Step 3: Extract and protect other LaTeX commands from the protected content
	commands := ExtractLaTeXCommands(protectedContent)

	if len(commands) == 0 {
		return protectedContent, placeholders
	}

	// Filter out commands that overlap with table or math placeholders
	// (since they are already protected)
	var filteredCommands []LaTeXCommand
	for _, cmd := range commands {
		// Skip if this command is inside a table placeholder
		isInsideTablePlaceholder := false
		for placeholder := range tablePlaceholders {
			if strings.Contains(cmd.Command, placeholder) {
				isInsideTablePlaceholder = true
				break
			}
		}
		if isInsideTablePlaceholder {
			continue
		}
		
		// Skip if this command is inside a math placeholder
		isInsideMathPlaceholder := false
		for placeholder := range mathPlaceholders {
			if strings.Contains(cmd.Command, placeholder) {
				isInsideMathPlaceholder = true
				break
			}
		}
		if !isInsideMathPlaceholder {
			filteredCommands = append(filteredCommands, cmd)
		}
	}

	// Process commands in reverse order to maintain correct positions
	result := protectedContent
	for i := len(filteredCommands) - 1; i >= 0; i-- {
		cmd := filteredCommands[i]
		placeholder := fmt.Sprintf("<<<LATEX_CMD_%d>>>", i)
		placeholders[placeholder] = cmd.Command

		// Replace the command with the placeholder
		result = result[:cmd.Start] + placeholder + result[cmd.End:]
	}

	return result, placeholders
}

// RestoreLaTeXCommands restores the original LaTeX commands from placeholders.
// It takes the translated content and the placeholder map from ProtectLaTeXCommands.
//
// Validates: Requirements 3.2
func RestoreLaTeXCommands(content string, placeholders map[string]string) string {
	result := content
	for placeholder, original := range placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

// IsLaTeXCommand checks if the given text is a LaTeX command.
// It returns true if the text matches any known LaTeX command pattern.
func IsLaTeXCommand(text string) bool {
	// Check if it starts with backslash (most LaTeX commands)
	if strings.HasPrefix(text, "\\") {
		return true
	}

	// Check if it's a math environment
	if strings.HasPrefix(text, "$") || strings.HasPrefix(text, "\\[") || strings.HasPrefix(text, "\\(") {
		return true
	}

	// Check if it's a comment
	if strings.HasPrefix(text, "%") {
		return true
	}

	return false
}

// ContainsLaTeXCommands checks if the content contains any LaTeX commands.
func ContainsLaTeXCommands(content string) bool {
	commands := ExtractLaTeXCommands(content)
	return len(commands) > 0
}

// GetMathEnvironments extracts only mathematical environments from the content.
// This is useful when you need to specifically protect math content.
func GetMathEnvironments(content string) []LaTeXCommand {
	commands := ExtractLaTeXCommands(content)
	var mathCommands []LaTeXCommand
	for _, cmd := range commands {
		if cmd.Type == CommandTypeMath {
			mathCommands = append(mathCommands, cmd)
		}
	}
	return mathCommands
}

// GetNonMathText extracts the text content that is not inside math environments.
// This is useful for identifying the translatable portions of a LaTeX document.
func GetNonMathText(content string) string {
	commands := ExtractLaTeXCommands(content)

	if len(commands) == 0 {
		return content
	}

	var result strings.Builder
	lastEnd := 0

	for _, cmd := range commands {
		// Add text between commands
		if cmd.Start > lastEnd {
			result.WriteString(content[lastEnd:cmd.Start])
		}
		lastEnd = cmd.End
	}

	// Add remaining text after last command
	if lastEnd < len(content) {
		result.WriteString(content[lastEnd:])
	}

	return result.String()
}


// FixLaTeXLineStructure fixes common line structure issues in translated LaTeX content.
// This function addresses problems where LLM translations merge lines that should be separate,
// particularly around \item, \begin{...}, and \end{...} commands.
//
// Common issues fixed:
// - \end{itemize}\item -> \end{itemize}\n\item
// - \end{enumerate}\item -> \end{enumerate}\n\item
// - Multiple \item on same line
// - \begin{...} and \end{...} merged with other content
// - Super long lines where LLM merged multiple paragraphs/tables
func FixLaTeXLineStructure(content string) string {
	logger.Info("FixLaTeXLineStructure: starting",
		logger.Int("contentLength", len(content)),
		logger.Bool("hasTabular", strings.Contains(content, "\\begin{tabular}")))
	
	result := content

	// ============================================================
	// CRITICAL: Fix super long lines where LLM merged multiple structures
	// This must run FIRST before other fixes
	// ============================================================
	result = fixSuperLongLines(result)

	// Fix \end{itemize} followed by \item on same line
	// Pattern: \end{itemize}\item or \end{itemize}  \item (with spaces)
	endItemizeItemPattern := regexp.MustCompile(`(\\end\{itemize\})\s*(\\item\b)`)
	result = endItemizeItemPattern.ReplaceAllString(result, "$1\n$2")

	// Fix \end{enumerate} followed by \item on same line
	endEnumerateItemPattern := regexp.MustCompile(`(\\end\{enumerate\})\s*(\\item\b)`)
	result = endEnumerateItemPattern.ReplaceAllString(result, "$1\n$2")

	// Fix \end{description} followed by \item on same line
	endDescriptionItemPattern := regexp.MustCompile(`(\\end\{description\})\s*(\\item\b)`)
	result = endDescriptionItemPattern.ReplaceAllString(result, "$1\n$2")

	// Fix multiple \item on same line (but not at line start)
	// This handles cases like: "text \item more text \item even more"
	// We need to be careful not to break legitimate uses
	multipleItemPattern := regexp.MustCompile(`([^\n])(\\item\b)`)
	// Only fix if there's non-whitespace before \item (not at line start)
	result = multipleItemPattern.ReplaceAllStringFunc(result, func(match string) string {
		// Check if the character before \item is not whitespace at line start
		if len(match) > 0 && match[0] != '\n' && match[0] != ' ' && match[0] != '\t' {
			return string(match[0]) + "\n\\item"
		}
		return match
	})

	// Fix \end{...} followed by \begin{...} on same line
	endBeginPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\begin\{[^}]+\})`)
	result = endBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Fix \end{...} followed by \section/\subsection/etc on same line
	endSectionPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\b)`)
	result = endSectionPattern.ReplaceAllString(result, "$1\n\n$2")

	// Fix text followed immediately by \begin{itemize/enumerate/description}
	textBeginListPattern := regexp.MustCompile(`([^\n\s])\s*(\\begin\{(?:itemize|enumerate|description)\})`)
	result = textBeginListPattern.ReplaceAllString(result, "$1\n$2")

	// Fix \end{itemize/enumerate/description} followed by text (not a command)
	endListTextPattern := regexp.MustCompile(`(\\end\{(?:itemize|enumerate|description)\})([^\n\\])`)
	result = endListTextPattern.ReplaceAllString(result, "$1\n$2")

	// Fix closing brace followed by \section/\subsection/etc on same line
	// This handles cases like: \revised{...}\subsection{...} -> \revised{...}\n\n\subsection{...}
	// Common pattern when LLM merges lines after commands like \revised{}, \textbf{}, etc.
	braceSectionPattern := regexp.MustCompile(`(\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\b)`)
	result = braceSectionPattern.ReplaceAllString(result, "$1\n\n$2")

	// Fix closing brace followed by \begin{env} on same line (for list environments)
	// This handles cases like: \revised{...}\begin{itemize} -> \revised{...}\n\begin{itemize}
	braceBeginListPattern := regexp.MustCompile(`(\})\s*(\\begin\{(?:itemize|enumerate|description|quote|quotation|center)\})`)
	result = braceBeginListPattern.ReplaceAllString(result, "$1\n$2")

	// Fix comment line (%%...%%) followed immediately by \abstract, \maketitle, \title, etc.
	// Pattern: %%==...==%%\abstract{ -> %%==...==%%\n\abstract{
	commentAbstractPattern := regexp.MustCompile(`(%%[^%\n]*%%)\s*(\\(?:abstract|maketitle|title|author|date|keywords|pacs)\b)`)
	result = commentAbstractPattern.ReplaceAllString(result, "$1\n$2")

	// Fix any non-newline character followed by \abstract (except backslash for commands like \renewcommand{\abstract})
	// This ensures \abstract always starts on a new line
	abstractNewlinePattern := regexp.MustCompile(`([^\\%\n])\s*(\\abstract\b)`)
	result = abstractNewlinePattern.ReplaceAllString(result, "$1\n$2")

	// Fix \maketitle followed immediately by content (should have newline after)
	maketitlePattern := regexp.MustCompile(`(\\maketitle)\s*([^\n\s\\])`)
	result = maketitlePattern.ReplaceAllString(result, "$1\n\n$2")

	// Fix custom LaTeX commands followed directly by CJK characters
	// In XeLaTeX, commands like \cech followed by Chinese characters (e.g., \cech复形)
	// are parsed incorrectly. We need to add {} after the command to terminate it properly.
	// Common custom commands that might be followed by Chinese text:
	// \cech, \RR, \ZZ, \NN, \CC, etc.
	// Pattern: \commandname followed by CJK character -> \commandname{} followed by CJK character
	// CJK Unicode ranges: \x{4E00}-\x{9FFF} (CJK Unified Ideographs)
	cjkCommandPattern := regexp.MustCompile(`(\\(?:cech|RR|ZZ|NN|CC|Circle|id|Diff|Met|Imm|Vect|Rn|sgn|eps|ud|SO|vol|Sym|img|re|ad|Ad|tr|rank))([\x{4E00}-\x{9FFF}])`)
	result = cjkCommandPattern.ReplaceAllString(result, "$1{}$2")

	// Fix comment line followed by \begin{...} on same line
	// This is a critical fix for cases where LLM merges a comment line with the next \begin{...}
	// Pattern: % comment text\begin{env} -> % comment text\n\begin{env}
	// This happens when LLM removes empty lines between comment and \begin
	commentBeginPattern := regexp.MustCompile(`(%[^\n]*[^\s\n])\s*(\\begin\{[^}]+\})`)
	result = commentBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Fix any text (not at line start) followed by \begin{wrapfigure/figure/table/etc}
	// These environments should always start on their own line
	textBeginFloatPattern := regexp.MustCompile(`([^\n])\s*(\\begin\{(?:wrapfigure|figure|figure\*|table|table\*|sidewaystable|equation|align)\})`)
	result = textBeginFloatPattern.ReplaceAllString(result, "$1\n$2")

	// Fix \end{wrapfigure/figure/table} followed by text on same line
	endFloatTextPattern := regexp.MustCompile(`(\\end\{(?:wrapfigure|figure|figure\*|table|table\*|sidewaystable)\})\s*([^\n\s\\])`)
	result = endFloatTextPattern.ReplaceAllString(result, "$1\n$2")

	// ============================================================
	// Table-specific fixes
	// ============================================================

	// Fix: \caption{...}\footnotesize -> \caption{...}\n\footnotesize
	// This is a common issue where LLM merges caption with table formatting commands
	captionFootnotePattern := regexp.MustCompile(`(\\caption\{[^}]*\})\s*(\\(?:footnotesize|small|tiny|scriptsize|normalsize|large|Large|LARGE|huge|Huge)\b)`)
	result = captionFootnotePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \caption{...}\setlength -> \caption{...}\n\setlength
	captionSetlengthPattern := regexp.MustCompile(`(\\caption\{[^}]*\})\s*(\\setlength)`)
	result = captionSetlengthPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \caption{...}\begin{tabular} -> \caption{...}\n\begin{tabular}
	captionTabularPattern := regexp.MustCompile(`(\\caption\{[^}]*\})\s*(\\begin\{(?:tabular|tabularx|longtable|array)\})`)
	result = captionTabularPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \begin{tabular}{...}\toprule -> \begin{tabular}{...}\n\toprule
	// The column spec can be very complex with nested braces, so we need a more robust pattern
	// Match \begin{tabular} followed by any column spec (which may contain nested braces)
	// Then match \toprule/\midrule/etc on the same line
	tabularToprulePattern := regexp.MustCompile(`(\\begin\{(?:tabular|tabularx|longtable|array)\}\{[^}]*(?:\{[^}]*\}[^}]*)*\})\s*(\\(?:toprule|midrule|bottomrule|hline)\b)`)
	result = tabularToprulePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \toprule...\midrule on same line -> separate lines
	rulePattern := regexp.MustCompile(`(\\(?:toprule|midrule|bottomrule|hline))\s*(\\(?:toprule|midrule|bottomrule|hline)\b)`)
	result = rulePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: table row ending with \\ followed by \midrule on same line
	rowMidrulePattern := regexp.MustCompile(`(\\\\)\s*(\\(?:midrule|bottomrule|hline)\b)`)
	result = rowMidrulePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \midrule followed by table content on same line
	midruleContentPattern := regexp.MustCompile(`(\\(?:midrule|toprule|hline))\s*(&|\w)`)
	result = midruleContentPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \end{tabular}\end{table} -> \end{tabular}\n\end{table}
	endTabularTablePattern := regexp.MustCompile(`(\\end\{(?:tabular|tabularx|longtable|array)\})\s*(\\end\{(?:table|table\*)\})`)
	result = endTabularTablePattern.ReplaceAllString(result, "$1\n$2")

	// Fix: closing brace followed by \begin{tabular} on same line
	braceTabularPattern := regexp.MustCompile(`(\})\s*(\\begin\{(?:tabular|tabularx|longtable|array)\})`)
	result = braceTabularPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix incomplete tabular column specs using a function-based approach
	// This handles cases like: \begin{tabular}{@{}l cccc@{}\n\toprule
	// where the column spec is missing the closing }
	beforeFix := result
	result = fixIncompleteTabularColumnSpecInTranslator(result)
	if result != beforeFix {
		logger.Info("FixLaTeXLineStructure: fixed incomplete tabular column spec")
	}

	// Fix: \centering followed by \caption on same line
	centeringCaptionPattern := regexp.MustCompile(`(\\centering)\s*(\\caption)`)
	result = centeringCaptionPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \label{...}\end{table} -> \label{...}\n\end{table}
	labelEndTablePattern := regexp.MustCompile(`(\\label\{[^}]*\})\s*(\\end\{(?:table|table\*|figure|figure\*)\})`)
	result = labelEndTablePattern.ReplaceAllString(result, "$1\n$2")

	logger.Debug("fixed LaTeX line structure", 
		logger.Int("originalLength", len(content)),
		logger.Int("fixedLength", len(result)))

	return result
}

// fixIncompleteTabularColumnSpecInTranslator fixes tabular column specs that are missing the closing brace.
// This handles cases like: \begin{tabular}{@{}l cccc@{}\n\toprule
// where the column spec should be {@{}l cccc@{}} but is missing the final }
func fixIncompleteTabularColumnSpecInTranslator(content string) string {
	// Find all \begin{tabular} (and variants) patterns
	tabularPattern := regexp.MustCompile(`\\begin\{(tabular|tabularx|longtable|array)\*?\}\{`)
	
	result := content
	offset := 0
	
	matches := tabularPattern.FindAllStringIndex(content, -1)
	
	logger.Info("fixIncompleteTabularColumnSpecInTranslator: checking content",
		logger.Int("contentLength", len(content)),
		logger.Int("tabularMatches", len(matches)))
	
	for _, match := range matches {
		startIdx := match[1] + offset // Position right after the opening { of column spec
		
		// Find the column spec by counting braces, but STOP at newline
		// The column spec should be on the same line as \begin{tabular}
		braceCount := 1 // We're starting after the opening {
		stoppedAt := "end"
		
		for i := startIdx; i < len(result); i++ {
			c := result[i]
			if c == '\n' {
				// Stop at newline - column spec should not span multiple lines
				stoppedAt = "newline"
				break
			}
			if c == '{' {
				braceCount++
			} else if c == '}' {
				braceCount--
				if braceCount == 0 {
					stoppedAt = "balanced"
					break // Column spec is properly closed
				}
			}
		}
		
		// Log the result of brace counting
		lineEnd := strings.Index(result[startIdx:], "\n")
		if lineEnd == -1 {
			lineEnd = len(result) - startIdx
		}
		lineContent := result[startIdx:startIdx+minInt(lineEnd, 80)]
		logger.Info("fixIncompleteTabularColumnSpecInTranslator: brace count result",
			logger.Int("startIdx", startIdx),
			logger.Int("braceCount", braceCount),
			logger.String("stoppedAt", stoppedAt),
			logger.String("lineContent", lineContent))
		
		// If braceCount > 0 after reaching newline, the column spec is incomplete
		if braceCount > 0 {
			// Find where the column spec should end (before \toprule, \midrule, \hline, or newline)
			remaining := result[startIdx:]
			
			logger.Info("fixIncompleteTabularColumnSpecInTranslator: incomplete column spec found",
				logger.Int("startIdx", startIdx),
				logger.Int("braceCount", braceCount))
			
			// Look for the pattern where column spec ends with @{} followed by newline and \toprule/\midrule
			// Pattern: ...@{}\n    \toprule or ...@{} \n\toprule
			endPattern := regexp.MustCompile(`(@\{\})\s*\n\s*(\\(?:toprule|midrule|bottomrule|hline)\b)`)
			if endMatch := endPattern.FindStringSubmatchIndex(remaining); endMatch != nil {
				// Insert the missing } after @{}
				insertPos := startIdx + endMatch[3] // Position after @{}
				result = result[:insertPos] + "}" + result[insertPos:]
				offset++
				logger.Info("fixed incomplete tabular column spec in translator",
					logger.Int("position", insertPos))
			} else {
				logger.Info("fixIncompleteTabularColumnSpecInTranslator: end pattern not found",
					logger.String("remainingStart", remaining[:minInt(100, len(remaining))]))
			}
		}
	}
	
	return result
}

// fixSuperLongLines breaks up super long lines where LLM merged multiple structures.
// This is critical because LLM sometimes merges entire sections, tables, and paragraphs
// into single lines, which breaks LaTeX compilation and causes missing content.
func fixSuperLongLines(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	
	for _, line := range lines {
		// Only process lines longer than 1000 characters
		if len(line) < 1000 {
			result = append(result, line)
			continue
		}
		
		logger.Info("fixSuperLongLines: processing long line",
			logger.Int("length", len(line)))
		
		// Run multiple passes to handle nested cases
		fixed := line
		for pass := 0; pass < 5; pass++ {
			prevLen := len(fixed)
			
			// Add newlines BEFORE these commands (they should start on their own line)
			beforePatterns := []string{
				`\\section\{`,
				`\\subsection\{`,
				`\\subsubsection\{`,
				`\\paragraph\{`,
				`\\begin\{table`,
				`\\begin\{figure`,
				`\\begin\{tabular`,
				`\\begin\{equation`,
				`\\begin\{align`,
				`\\begin\{itemize`,
				`\\begin\{enumerate`,
				`\\begin\{description`,
				`\\caption\{`,
				`\\footnotetext\{`,
				`\\toprule`,
				`\\midrule`,
				`\\bottomrule`,
				`\\hline`,
				`\\centering`,
				`\\resizebox`,
				`\\label\{`,
			}
			
			for _, pattern := range beforePatterns {
				re := regexp.MustCompile(`([^\n])(\s*)(` + pattern + `)`)
				fixed = re.ReplaceAllString(fixed, "$1\n$3")
			}
			
			// Add newlines AFTER these commands (content should continue on next line)
			afterPatterns := []string{
				`\\end\{table\*?\}`,
				`\\end\{figure\*?\}`,
				`\\end\{tabular\*?\}`,
				`\\end\{equation\*?\}`,
				`\\end\{align\*?\}`,
				`\\end\{itemize\}`,
				`\\end\{enumerate\}`,
				`\\end\{description\}`,
				`\\bottomrule`,
				`\\\\`, // Table row endings
			}
			
			for _, pattern := range afterPatterns {
				re := regexp.MustCompile(`(` + pattern + `)(\s*)([^\n])`)
				fixed = re.ReplaceAllString(fixed, "$1\n$3")
			}
			
			// If no changes were made, stop iterating
			if len(fixed) == prevLen {
				break
			}
		}
		
		// Split the fixed line and add all parts
		fixedLines := strings.Split(fixed, "\n")
		result = append(result, fixedLines...)
	}
	
	return strings.Join(result, "\n")
}

// ApplyReferenceBasedFixes applies fixes by comparing translated content with original.
// This is more accurate than rule-based fixes because it can detect structural differences
// between the original and translated content.
//
// Common issues fixed:
// - Missing closing braces for \resizebox in tables
// - Unbalanced braces in \caption commands
// - Missing \end{environment} tags
// - Trailing garbage braces at end of file
// - Wrongly commented environments (LLM added % to environments that shouldn't be commented)
// - Duplicate or misplaced \end{document}
// - Extra closing braces after commands like \textit{...}}
func ApplyReferenceBasedFixes(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated
	anyFixed := false

	// Count commented tables before fix for debugging
	beforeCount := strings.Count(result, "% \\begin{table")
	logger.Debug("ApplyReferenceBasedFixes: before wrongly commented fix",
		logger.Int("commentedTables", beforeCount))

	// 1. Fix wrongly commented environments FIRST (most critical for compilation)
	// This must run before FixByLineComparison because line numbers may not match
	result, wrongCommentFixed := fixWronglyCommentedEnvironments(result, original)
	if wrongCommentFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed wrongly commented environments")
	}

	// 1.5. Restore missing \input commands that were deleted during translation
	result, inputRestored := fixMissingInputCommands(result, original)
	if inputRestored {
		anyFixed = true
		logger.Debug("reference-based fix: restored missing \\input commands")
	}

	// 1.6. Restore missing appendix section if it was truncated during translation
	result, appendixRestored := fixMissingAppendixSection(result, original)
	if appendixRestored {
		anyFixed = true
		logger.Debug("reference-based fix: restored missing appendix section")
	}

	// 1.7. Fix wrongly uncommented \bibliography commands in preamble
	// If original has commented \bibliography in preamble, ensure translated also has it commented
	result, bibFixed := fixWronglyUncommentedBibliography(result, original)
	if bibFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed wrongly uncommented \\bibliography in preamble")
	}

	// 1.8. Fix wrongly uncommented lines in preamble
	// If original has commented lines in preamble, ensure translated also has them commented
	// This fixes cases where LLM removes % from comment lines, causing "Missing \begin{document}" errors
	result, preambleFixed := fixWronglyUncommentedPreambleLines(result, original)
	if preambleFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed wrongly uncommented preamble lines")
	}

	// Count commented tables after fix for debugging
	afterCount := strings.Count(result, "% \\begin{table")
	logger.Debug("ApplyReferenceBasedFixes: after wrongly commented fix",
		logger.Int("commentedTables", afterCount),
		logger.Bool("fixed", wrongCommentFixed))

	// 2. Apply line-by-line comparison fix (for line-aligned content)
	lineFixResult := FixByLineComparison(result, original)
	if lineFixResult.FixCount > 0 {
		result = lineFixResult.Fixed
		anyFixed = true
		logger.Debug("reference-based fix: applied line-by-line fixes",
			logger.Int("fixCount", lineFixResult.FixCount))
	}

	// Count commented tables after line-by-line fix for debugging
	afterLineFixCount := strings.Count(result, "% \\begin{table")
	logger.Debug("ApplyReferenceBasedFixes: after line-by-line fix",
		logger.Int("commentedTables", afterLineFixCount))

	// 3. Fix duplicate/misplaced \end{document} (critical)
	result, endDocFixed := fixDuplicateEndDocument(result, original)
	if endDocFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed duplicate/misplaced \\end{document}")
	}

	// 4. Fix table structures (resizebox closing braces)
	result, tableFixed := fixTableResizeboxBraces(result, original)
	if tableFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed table resizebox braces")
	}

	// 5. Fix caption brace issues
	result, captionFixed := fixCaptionBraceIssues(result, original)
	if captionFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed caption brace issues")
	}

	// 6. Fix extra closing braces (e.g., \textit{...}})
	result, extraBraceFixed := fixExtraClosingBraces(result, original)
	if extraBraceFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed extra closing braces")
	}

	// 6. Fix trailing garbage braces
	result, trailingFixed := fixTrailingGarbageByReference(result, original)
	if trailingFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed trailing garbage braces")
	}

	// 7. Fix environment structure
	result, envFixed := fixEnvironmentStructureByReference(result, original)
	if envFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed environment structure")
	}

	// 8. Fix missing comment markers (original has % but translated doesn't)
	result, commentFixed := fixMissingCommentsByReference(result, original)
	if commentFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed missing comment markers")
	}

	if anyFixed {
		logger.Info("applied reference-based fixes to translated content")
	}

	// 9. Fix multirow/multicolumn brace issues (run on final content)
	beforeMultirowFix := result
	result = FixMultirowMulticolumnBracesGlobal(result)
	if result != beforeMultirowFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed multirow/multicolumn braces")
	}

	// 10. Fix table environment line structure issues
	// This fixes cases where } (closing scalebox) is merged with following text
	beforeTableFix := result
	result = FixTableEnvironmentLineStructure(result)
	if result != beforeTableFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed table environment line structure")
	}

	// 11. Fix scalebox closing brace issues (more specific fix)
	beforeScaleboxFix := result
	result = FixScaleboxClosingBrace(result)
	if result != beforeScaleboxFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed scalebox closing braces")
	}

	// 12. Fix \end{tabular}} merged braces
	// This fixes cases where } is merged with \end{tabular} on the same line
	beforeTabularFix := result
	result = FixEndTabularBraces(result)
	if result != beforeTabularFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed end tabular braces")
	}

	// 13. Fix sidewaystable structure issues
	// This fixes cases where \end{sidewaystable} is placed in the wrong position
	beforeSidewaystableFix := result
	result = FixSidewaystableStructure(result)
	if result != beforeSidewaystableFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed sidewaystable structure")
	}

	// 14. Fix extra closing brace after \end{table*}
	// This fixes cases where there's an extra } that doesn't match any opening brace
	beforeExtraBraceFix := result
	result = FixExtraClosingBraceAfterTable(result)
	if result != beforeExtraBraceFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed extra closing brace after table")
	}

	// 15. Fix opening brace { merged with comment line
	// This fixes cases where { is incorrectly merged to end of a comment line
	beforeBraceMergedFix := result
	result = FixBraceMergedWithComment(result)
	if result != beforeBraceMergedFix {
		anyFixed = true
		logger.Debug("reference-based fix: fixed brace merged with comment")
	}

	// 16. Clean CCS metadata with mathematical italic characters
	// This fixes cases where XML tags like <conceptid> are rendered as math italics
	beforeCCSFix := result
	result = CleanCCSMathItalics(result)
	if result != beforeCCSFix {
		anyFixed = true
		logger.Debug("reference-based fix: cleaned CCS math italics")
	}

	return result
}

// fixTableResizeboxBraces fixes missing closing braces for \resizebox in tables.
func fixTableResizeboxBraces(translated, original string) (string, bool) {
	// Check if original has resizebox with proper closing
	origHasResizebox := strings.Contains(original, "\\resizebox")
	transHasResizebox := strings.Contains(translated, "\\resizebox")

	if !origHasResizebox || !transHasResizebox {
		return translated, false
	}

	// Count \end{tabular}} in original vs translated
	origClosedCount := strings.Count(original, "\\end{tabular}}")
	transClosedCount := strings.Count(translated, "\\end{tabular}}")

	if origClosedCount > transClosedCount {
		// Need to add closing braces
		lines := strings.Split(translated, "\n")
		fixed := false

		inResizebox := false
		for i, line := range lines {
			if strings.Contains(line, "\\resizebox") {
				inResizebox = true
			}
			if inResizebox && strings.Contains(line, "\\end{tabular}") && !strings.Contains(line, "\\end{tabular}}") {
				lines[i] = strings.Replace(line, "\\end{tabular}", "\\end{tabular}}", 1)
				fixed = true
				inResizebox = false
			}
			if strings.Contains(line, "\\end{table") {
				inResizebox = false
			}
		}

		if fixed {
			return strings.Join(lines, "\n"), true
		}
	}

	return translated, false
}

// fixCaptionBraceIssues fixes brace issues in captions by comparing with original.
func fixCaptionBraceIssues(translated, original string) (string, bool) {
	// Common pattern: \textbf{\textit{n}=64 should be \textbf{\textit{n}}=64
	// This happens when LLM translation loses a closing brace

	patterns := []struct {
		pattern *regexp.Regexp
		replace string
	}{
		// \textbf{\textit{X}= -> \textbf{\textit{X}}=
		// The } closes \textit, but we need another } to close \textbf before =
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+\})=`), "${1}}="},
		// Same pattern with other separators
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+\})[,;:，；：]`), "${1}}"},
		// \textbf{\textit{X}） -> \textbf{\textit{X}}）
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+\})[）\)]`), "${1}}"},
		// Similar for other nested commands (textit containing textbf)
		{regexp.MustCompile(`(\\textit\{\\textbf\{[^}]+\})=`), "${1}}="},
	}

	result := translated
	fixed := false

	for _, p := range patterns {
		if p.pattern.MatchString(result) {
			newResult := p.pattern.ReplaceAllString(result, p.replace)
			if newResult != result {
				logger.Debug("fixCaptionBraceIssues: pattern matched",
					logger.String("before", result),
					logger.String("after", newResult))
				result = newResult
				fixed = true
			}
		}
	}

	return result, fixed
}

// fixTrailingGarbageByReference removes trailing garbage that doesn't exist in original.
func fixTrailingGarbageByReference(translated, original string) (string, bool) {
	// Get trailing characters
	origTrimmed := strings.TrimSpace(original)
	transTrimmed := strings.TrimSpace(translated)

	// Count trailing braces
	origTrailingBraces := countTrailingBraces(origTrimmed)
	transTrailingBraces := countTrailingBraces(transTrimmed)

	if transTrailingBraces > origTrailingBraces+2 {
		// Remove extra trailing braces
		extra := transTrailingBraces - origTrailingBraces
		result := transTrimmed
		for i := 0; i < extra && len(result) > 0 && result[len(result)-1] == '}'; i++ {
			result = result[:len(result)-1]
		}
		return result + "\n", true
	}

	return translated, false
}

// countTrailingBraces counts consecutive closing braces at the end of a string.
func countTrailingBraces(s string) int {
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '}' {
			count++
		} else if s[i] != ' ' && s[i] != '\n' && s[i] != '\t' {
			break
		}
	}
	return count
}

// fixEnvironmentStructureByReference ensures environments match between original and translated.
func fixEnvironmentStructureByReference(translated, original string) (string, bool) {
	beginRe := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endRe := regexp.MustCompile(`\\end\{([^}]+)\}`)

	// Count environments in original
	origBegins := make(map[string]int)
	origEnds := make(map[string]int)
	for _, match := range beginRe.FindAllStringSubmatch(original, -1) {
		origBegins[match[1]]++
	}
	for _, match := range endRe.FindAllStringSubmatch(original, -1) {
		origEnds[match[1]]++
	}

	// Count environments in translated
	transBegins := make(map[string]int)
	transEnds := make(map[string]int)
	for _, match := range beginRe.FindAllStringSubmatch(translated, -1) {
		transBegins[match[1]]++
	}
	for _, match := range endRe.FindAllStringSubmatch(translated, -1) {
		transEnds[match[1]]++
	}

	result := translated
	fixed := false

	// Check for missing \end tags
	for env, origEndCount := range origEnds {
		transEndCount := transEnds[env]
		if transEndCount < origEndCount && transBegins[env] >= origBegins[env] {
			// Missing end tags - add them
			missing := origEndCount - transEndCount
			for i := 0; i < missing; i++ {
				// Find appropriate place to insert
				endTag := "\\end{" + env + "}"
				// Insert before \end{document} if present
				if strings.Contains(result, "\\end{document}") {
					result = strings.Replace(result, "\\end{document}", endTag+"\n\\end{document}", 1)
				} else {
					result = result + "\n" + endTag
				}
				fixed = true
			}
		}
	}

	return result, fixed
}

// fixMissingCommentsByReference fixes cases where LLM translation removed comment markers
// from lines that should be commented. This compares the original and translated content
// to detect environments that are commented in original but not in translated.
// 
// IMPORTANT: This function should NOT comment environments that are uncommented in the original.
// It only adds comments to environments that are ALWAYS commented in the original.
func fixMissingCommentsByReference(translated, original string) (string, bool) {
	// Build a map of commented environments in original
	// Pattern: lines starting with % followed by \begin{env} or \end{env}
	commentedEnvPattern := regexp.MustCompile(`(?m)^%\s*(\\(?:begin|end)\{[^}]+\})`)
	
	origCommentedEnvs := make(map[string]bool)
	for _, match := range commentedEnvPattern.FindAllStringSubmatch(original, -1) {
		origCommentedEnvs[match[1]] = true
	}
	
	if len(origCommentedEnvs) == 0 {
		return translated, false
	}
	
	// Also build a map of UNcommented environments in original
	// If an environment appears both commented and uncommented, we should NOT add comments
	uncommentedEnvPattern := regexp.MustCompile(`(?m)^[^%\n]*(\\(?:begin|end)\{[^}]+\})`)
	origUncommentedEnvs := make(map[string]bool)
	for _, match := range uncommentedEnvPattern.FindAllStringSubmatch(original, -1) {
		origUncommentedEnvs[match[1]] = true
	}
	
	// Remove environments that appear both commented and uncommented
	// These are ambiguous and should not be auto-commented
	for env := range origUncommentedEnvs {
		delete(origCommentedEnvs, env)
	}
	
	if len(origCommentedEnvs) == 0 {
		return translated, false
	}
	
	// Check translated content for uncommented versions of these environments
	lines := strings.Split(translated, "\n")
	fixed := false
	
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		// Skip already commented lines
		if strings.HasPrefix(trimmedLine, "%") {
			continue
		}
		
		// Check if this line contains an environment that should be commented
		for commentedEnv := range origCommentedEnvs {
			if strings.Contains(trimmedLine, commentedEnv) {
				// This environment should be commented but isn't
				lines[i] = "% " + line
				fixed = true
				logger.Debug("fixed missing comment marker",
					logger.String("line", trimmedLine),
					logger.String("env", commentedEnv))
				break
			}
		}
	}
	
	if fixed {
		return strings.Join(lines, "\n"), true
	}
	
	return translated, false
}

// fixWronglyCommentedEnvironments fixes cases where LLM translation incorrectly added
// comment markers to environments that are NOT commented in the original.
// This is the opposite of fixMissingCommentsByReference.
// Also handles sectioning commands like \section, \subsection, etc.
// Also handles \input and \include commands.
func fixWronglyCommentedEnvironments(translated, original string) (string, bool) {
	if original == "" {
		return translated, false
	}

	result := translated
	fixed := false

	// Part 1: Fix wrongly commented \begin{} and \end{} environments
	// Find all environments in original that are NOT commented
	// Pattern: \begin{env} or \end{env} that is NOT preceded by %
	envPattern := regexp.MustCompile(`(?m)^([^%\n]*)(\\(?:begin|end)\{([^}]+)\})`)

	origUncommentedEnvs := make(map[string]bool)
	for _, match := range envPattern.FindAllStringSubmatch(original, -1) {
		// match[2] is the full \begin{env} or \end{env}
		// match[3] is just the env name
		origUncommentedEnvs[match[2]] = true
	}

	if len(origUncommentedEnvs) > 0 {
		// Check translated content for commented versions of these environments
		// Pattern: % \begin{env} or % \end{env} (with optional spaces after %)
		// Also handle cases like "% \begin{table*}[]"
		commentedEnvPattern := regexp.MustCompile(`(?m)^(\s*)%\s*(\\(?:begin|end)\{([^}]+)\}(\[\])?)`)

		commentedMatches := commentedEnvPattern.FindAllStringSubmatch(result, -1)

		for _, match := range commentedMatches {
			envCmd := match[2] // e.g., \begin{figure*} or \begin{table*}[]
			// Normalize: remove [] suffix for comparison
			envCmdNormalized := strings.TrimSuffix(envCmd, "[]")
			
			// Check if this environment (with or without []) is uncommented in original
			if origUncommentedEnvs[envCmd] || origUncommentedEnvs[envCmdNormalized] || origUncommentedEnvs[envCmdNormalized+"[]"] {
				// This environment is NOT commented in original but IS commented in translated
				// Remove the comment marker
				oldLine := match[0]
				newLine := match[1] + match[2] // preserve leading whitespace, remove % 
				if match[4] != "" {
					newLine = match[1] + match[2] // include the [] if present
				}
				logger.Info("fixWronglyCommentedEnvironments: fixing env",
					logger.String("env", envCmd))
				result = strings.Replace(result, oldLine, newLine, 1)
				fixed = true
				logger.Debug("fixed wrongly commented environment",
					logger.String("env", envCmd))
			}
		}
	}

	// Part 2: Fix wrongly commented sectioning commands (\section, \subsection, etc.)
	// Pattern: \section{...} or \subsection{...} etc. that is NOT preceded by %
	sectionPattern := regexp.MustCompile(`(?m)^([^%\n]*)(\\(?:section|subsection|subsubsection|paragraph|chapter)\*?\{[^}]*\})`)

	origUncommentedSections := make(map[string]bool)
	for _, match := range sectionPattern.FindAllStringSubmatch(original, -1) {
		// match[2] is the full \section{...} command
		origUncommentedSections[match[2]] = true
	}

	if len(origUncommentedSections) > 0 {
		// Check translated content for commented versions of these sections
		// Pattern: % \section{...} (with optional spaces after %)
		commentedSectionPattern := regexp.MustCompile(`(?m)^(\s*)%+\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\*?\{[^}]*\})`)

		for _, match := range commentedSectionPattern.FindAllStringSubmatch(result, -1) {
			sectionCmd := match[2] // e.g., \section{Position}
			
			// Check if this section command is uncommented in original
			if origUncommentedSections[sectionCmd] {
				// This section is NOT commented in original but IS commented in translated
				// Remove the comment marker
				oldLine := match[0]
				newLine := match[1] + match[2] // preserve leading whitespace, remove %
				result = strings.Replace(result, oldLine, newLine, 1)
				fixed = true
				logger.Debug("fixed wrongly commented section",
					logger.String("section", sectionCmd))
			}
		}
	}

	// Part 3: Fix wrongly commented \input and \include commands
	// Pattern: \input{...} or \include{...} that is NOT preceded by %
	inputPattern := regexp.MustCompile(`(?m)^([^%\n]*)(\\(?:input|include)\{([^}]+)\})`)

	origUncommentedInputs := make(map[string]bool)
	for _, match := range inputPattern.FindAllStringSubmatch(original, -1) {
		// match[2] is the full \input{...} or \include{...} command
		origUncommentedInputs[match[2]] = true
	}

	if len(origUncommentedInputs) > 0 {
		// Check translated content for commented versions of these input commands
		// Pattern: % \input{...} (with optional spaces after %)
		commentedInputPattern := regexp.MustCompile(`(?m)^(\s*)%+\s*(\\(?:input|include)\{([^}]+)\})`)

		for _, match := range commentedInputPattern.FindAllStringSubmatch(result, -1) {
			inputCmd := match[2] // e.g., \input{sections/intro}
			
			// Check if this input command is uncommented in original
			if origUncommentedInputs[inputCmd] {
				// This input is NOT commented in original but IS commented in translated
				// Remove the comment marker
				oldLine := match[0]
				newLine := match[1] + match[2] // preserve leading whitespace, remove %
				result = strings.Replace(result, oldLine, newLine, 1)
				fixed = true
				logger.Debug("fixed wrongly commented input/include",
					logger.String("cmd", inputCmd))
			}
		}
	}

	// Part 4: Clean up garbage after \input commands
	// Sometimes LLM appends random content after \input{...} on the same line
	// Pattern: \input{...} followed by non-whitespace content that's not a comment
	inputGarbagePattern := regexp.MustCompile(`(\\input\{[^}]+\})\s+([^%\s][^\n]*)`)
	if inputGarbagePattern.MatchString(result) {
		result = inputGarbagePattern.ReplaceAllString(result, "$1")
		fixed = true
		logger.Debug("cleaned up garbage after \\input commands")
	}

	return result, fixed
}

// fixMissingAppendixSection restores the entire appendix section if it was truncated during translation.
// This handles cases where LLM output was truncated and the appendix section (including \appendix,
// \onecolumn, \input{appendix}, etc.) was lost.
func fixMissingAppendixSection(translated, original string) (string, bool) {
	if original == "" {
		return translated, false
	}

	// Check if original has \appendix command
	appendixPattern := regexp.MustCompile(`(?m)^[^%\n]*\\appendix\b`)
	if !appendixPattern.MatchString(original) {
		// Original doesn't have appendix section
		return translated, false
	}

	// Check if translated already has \appendix command
	if appendixPattern.MatchString(translated) {
		// Translated already has appendix, no need to restore
		return translated, false
	}

	logger.Info("detected missing \\appendix section, attempting to restore")

	// Find the appendix section in original
	// Look for \appendix and extract everything from there to \end{document}
	origAppendixIdx := -1
	origLines := strings.Split(original, "\n")
	for i, line := range origLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `\appendix`) && !strings.HasPrefix(trimmed, "%") {
			origAppendixIdx = i
			break
		}
	}

	if origAppendixIdx == -1 {
		return translated, false
	}

	// Find \end{document} in original
	origEndDocIdx := -1
	for i := len(origLines) - 1; i >= origAppendixIdx; i-- {
		if strings.Contains(origLines[i], `\end{document}`) {
			origEndDocIdx = i
			break
		}
	}

	if origEndDocIdx == -1 {
		return translated, false
	}

	// Extract the appendix section from original (from \appendix to just before \end{document})
	appendixSection := strings.Join(origLines[origAppendixIdx:origEndDocIdx], "\n")

	// Find \end{document} in translated
	transLines := strings.Split(translated, "\n")
	transEndDocIdx := -1
	for i := len(transLines) - 1; i >= 0; i-- {
		if strings.Contains(transLines[i], `\end{document}`) {
			transEndDocIdx = i
			break
		}
	}

	if transEndDocIdx == -1 {
		// No \end{document} in translated, append at the end
		result := translated + "\n\n" + appendixSection + "\n\\end{document}\n"
		logger.Info("restored missing appendix section (appended at end)")
		return result, true
	}

	// Insert appendix section before \end{document}
	var newLines []string
	newLines = append(newLines, transLines[:transEndDocIdx]...)
	newLines = append(newLines, "") // Empty line before appendix
	newLines = append(newLines, appendixSection)
	newLines = append(newLines, "") // Empty line before \end{document}
	newLines = append(newLines, transLines[transEndDocIdx:]...)

	result := strings.Join(newLines, "\n")
	logger.Info("restored missing appendix section",
		logger.Int("appendixLines", origEndDocIdx-origAppendixIdx))

	return result, true
}

// fixMissingInputCommands restores \input and \include commands that were deleted during translation.
// It compares the original and translated content and restores any missing \input commands
// by inserting them at the appropriate location (after \begin{document}).
func fixMissingInputCommands(translated, original string) (string, bool) {
	if original == "" {
		return translated, false
	}

	// Extract all \input and \include commands from original (non-commented)
	inputPattern := regexp.MustCompile(`(?m)^[^%\n]*(\\(?:input|include)\{([^}]+)\})`)
	
	origInputs := make([]string, 0)
	origInputSet := make(map[string]bool)
	for _, match := range inputPattern.FindAllStringSubmatch(original, -1) {
		cmd := match[1] // e.g., \input{sections/intro}
		if !origInputSet[cmd] {
			origInputs = append(origInputs, cmd)
			origInputSet[cmd] = true
		}
	}

	if len(origInputs) == 0 {
		return translated, false
	}

	// Extract all \input and \include commands from translated (non-commented)
	transInputSet := make(map[string]bool)
	for _, match := range inputPattern.FindAllStringSubmatch(translated, -1) {
		cmd := match[1]
		transInputSet[cmd] = true
	}

	// Find missing inputs
	missingInputs := make([]string, 0)
	for _, cmd := range origInputs {
		if !transInputSet[cmd] {
			missingInputs = append(missingInputs, cmd)
		}
	}

	if len(missingInputs) == 0 {
		return translated, false
	}

	logger.Info("found missing \\input commands, attempting to restore",
		logger.Int("missingCount", len(missingInputs)),
		logger.String("missing", strings.Join(missingInputs, ", ")))

	result := translated
	fixed := false

	// For each missing input, try to find where it should be inserted
	// Strategy: Find the input command that comes BEFORE it in the original,
	// then insert after that command in the translated content
	for _, missingCmd := range missingInputs {
		// Find the position of this command in the original
		missingIdx := -1
		for i, cmd := range origInputs {
			if cmd == missingCmd {
				missingIdx = i
				break
			}
		}

		if missingIdx == -1 {
			continue
		}

		// Find the previous input command that exists in translated
		var insertAfter string
		for i := missingIdx - 1; i >= 0; i-- {
			if transInputSet[origInputs[i]] {
				insertAfter = origInputs[i]
				break
			}
		}

		if insertAfter != "" {
			// Insert after the previous command
			// Find the line containing insertAfter and add the missing command after it
			lines := strings.Split(result, "\n")
			var newLines []string
			inserted := false
			for _, line := range lines {
				newLines = append(newLines, line)
				if !inserted && strings.Contains(line, insertAfter) && !strings.HasPrefix(strings.TrimSpace(line), "%") {
					// Insert the missing command after this line
					newLines = append(newLines, missingCmd)
					inserted = true
					fixed = true
					logger.Debug("restored missing \\input command",
						logger.String("cmd", missingCmd),
						logger.String("afterCmd", insertAfter))
				}
			}
			if inserted {
				result = strings.Join(newLines, "\n")
				transInputSet[missingCmd] = true // Mark as present now
			}
		} else {
			// No previous command found, try to insert after \begin{document}
			beginDocIdx := strings.Index(result, `\begin{document}`)
			if beginDocIdx != -1 {
				// Find the end of the \begin{document} line
				endOfLine := strings.Index(result[beginDocIdx:], "\n")
				if endOfLine != -1 {
					insertPos := beginDocIdx + endOfLine + 1
					result = result[:insertPos] + missingCmd + "\n" + result[insertPos:]
					fixed = true
					transInputSet[missingCmd] = true
					logger.Debug("restored missing \\input command after \\begin{document}",
						logger.String("cmd", missingCmd))
				}
			}
		}
	}

	return result, fixed
}

// fixWronglyUncommentedBibliography fixes cases where LLM translation incorrectly removed
// comment markers from \bibliography commands that ARE commented in the original.
// This is particularly important for \bibliography commands in the preamble, which should
// remain commented to avoid putting thebibliography in the wrong place.
func fixWronglyUncommentedBibliography(translated, original string) (string, bool) {
	if original == "" {
		return translated, false
	}

	// Find \begin{document} in both original and translated
	origBeginDocIdx := strings.Index(original, `\begin{document}`)
	transBeginDocIdx := strings.Index(translated, `\begin{document}`)

	if origBeginDocIdx == -1 || transBeginDocIdx == -1 {
		return translated, false
	}

	origPreamble := original[:origBeginDocIdx]
	transPreamble := translated[:transBeginDocIdx]
	transBody := translated[transBeginDocIdx:]

	result := translated
	fixed := false

	// Find commented \bibliography commands in original preamble
	// Pattern: % \bibliography{...} (with optional spaces after %)
	commentedBibPattern := regexp.MustCompile(`(?m)^(\s*)%+\s*(\\bibliography\{[^}]+\})`)
	origCommentedBibs := make(map[string]bool)
	for _, match := range commentedBibPattern.FindAllStringSubmatch(origPreamble, -1) {
		bibCmd := match[2] // e.g., \bibliography{main}
		origCommentedBibs[bibCmd] = true
	}

	if len(origCommentedBibs) == 0 {
		return translated, false
	}

	// Find uncommented \bibliography commands in translated preamble
	// Pattern: line that contains \bibliography{...} but does NOT start with %
	// We need to be careful: the line should not have % before \bibliography
	uncommentedBibPattern := regexp.MustCompile(`(?m)^([^%\n]*?)(\\bibliography\{([^}]+)\})`)
	
	allMatches := uncommentedBibPattern.FindAllStringSubmatch(transPreamble, -1)
	
	for _, match := range allMatches {
		prefix := match[1]  // content before \bibliography (should not contain %)
		bibCmd := match[2]  // e.g., \bibliography{main}

		// Check if this bibliography command is commented in original
		if origCommentedBibs[bibCmd] {
			// This bibliography is commented in original but NOT commented in translated
			// Comment it out
			oldLine := match[0]
			newLine := prefix + "% " + bibCmd
			transPreamble = strings.Replace(transPreamble, oldLine, newLine, 1)
			fixed = true
			logger.Debug("fixed wrongly uncommented \\bibliography in preamble",
				logger.String("cmd", bibCmd))
		}
	}

	if fixed {
		result = transPreamble + transBody
	}

	return result, fixed
}

// fixWronglyUncommentedPreambleLines fixes cases where LLM translation incorrectly removed
// comment markers from lines in the preamble that should remain commented.
// This is critical because uncommented text in the preamble causes "Missing \begin{document}" errors.
// 
// Key issue: LLM sometimes splits "% comment text" into two lines:
//   Line N:   %
//   Line N+1: comment text (without %)
// This function detects and fixes such cases.
//
// Additional issue: When \usepackage{ctex} is inserted after \documentclass, it causes
// line number offset between original and translated files. This function also handles
// lines that were incorrectly uncommented due to this offset.
func fixWronglyUncommentedPreambleLines(translated, original string) (string, bool) {
	if original == "" {
		return translated, false
	}

	// Find \begin{document} in both original and translated
	origBeginDocIdx := strings.Index(original, `\begin{document}`)
	transBeginDocIdx := strings.Index(translated, `\begin{document}`)

	logger.Info("fixWronglyUncommentedPreambleLines: checking preamble",
		logger.Int("origBeginDocIdx", origBeginDocIdx),
		logger.Int("transBeginDocIdx", transBeginDocIdx))

	if origBeginDocIdx == -1 || transBeginDocIdx == -1 {
		return translated, false
	}

	origPreamble := original[:origBeginDocIdx]
	transPreamble := translated[:transBeginDocIdx]
	transBody := translated[transBeginDocIdx:]

	origLines := strings.Split(origPreamble, "\n")
	transLines := strings.Split(transPreamble, "\n")
	
	// Detect line offset caused by \usepackage{ctex} insertion
	// If translated has \usepackage{ctex} after \documentclass but original doesn't,
	// there's a 1-line offset
	lineOffset := 0
	hasCtexInTrans := false
	hasCtexInOrig := strings.Contains(origPreamble, `\usepackage{ctex}`)
	
	for i, line := range transLines {
		if strings.Contains(line, `\usepackage{ctex}`) {
			hasCtexInTrans = true
			// Check if original has ctex at similar position
			if !hasCtexInOrig {
				lineOffset = 1
				logger.Info("fixWronglyUncommentedPreambleLines: detected ctex insertion offset",
					logger.Int("ctexLine", i),
					logger.Int("lineOffset", lineOffset))
			}
			break
		}
	}

	fixed := false

	// Build a map of commented lines in original (normalized)
	// Key: line content without leading % and whitespace, normalized to remove extra spaces
	origCommentedLines := make(map[string]bool)
	for _, line := range origLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%") {
			// Remove % and leading whitespace
			content := strings.TrimSpace(strings.TrimPrefix(trimmed, "%"))
			// Skip empty comments and lines that are just %
			if content != "" && !strings.HasPrefix(content, "%") {
				// Normalize: collapse multiple spaces into one
				normalized := strings.Join(strings.Fields(content), " ")
				origCommentedLines[normalized] = true
			}
		}
	}

	logger.Info("fixWronglyUncommentedPreambleLines: found commented lines in original",
		logger.Int("count", len(origCommentedLines)))

	// Debug: log the first 10 lines of the translated preamble
	for i := 0; i < minInt(10, len(transLines)); i++ {
		trimmed := strings.TrimSpace(transLines[i])
		logger.Info("fixWronglyUncommentedPreambleLines: preamble line",
			logger.Int("lineNum", i),
			logger.String("trimmed", trimmed[:minInt(50, len(trimmed))]),
			logger.Int("trimmedLen", len(trimmed)),
			logger.Bool("isJustPercent", trimmed == "%" || trimmed == "% "))
	}

	// FIRST PASS: Fix split comment lines where % is on its own line
	// Pattern: Line N is just "%" or "% ", Line N+1 is uncommented text that should be commented
	for i := 0; i < len(transLines)-1; i++ {
		trimmed := strings.TrimSpace(transLines[i])
		
		// Debug: log lines that are just %
		if trimmed == "%" || trimmed == "% " {
			nextLine := transLines[i+1]
			nextTrimmed := strings.TrimSpace(nextLine)
			logger.Info("fixWronglyUncommentedPreambleLines: found isolated % line",
				logger.Int("lineNum", i),
				logger.String("nextTrimmed", nextTrimmed[:minInt(60, len(nextTrimmed))]),
				logger.Bool("nextIsEmpty", nextTrimmed == ""),
				logger.Bool("nextStartsWithPercent", strings.HasPrefix(nextTrimmed, "%")),
				logger.Bool("nextStartsWithBackslash", strings.HasPrefix(nextTrimmed, "\\")))
		}
		
		// Check if this line is just "%" or "% " (isolated percent sign)
		if trimmed == "%" || trimmed == "% " {
			// Check the next line
			nextLine := transLines[i+1]
			nextTrimmed := strings.TrimSpace(nextLine)
			
			// Skip if next line is empty, already commented, or is a LaTeX command
			if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, "%") || strings.HasPrefix(nextTrimmed, "\\") {
				continue
			}
			
			// Normalize for comparison
			nextNormalized := strings.Join(strings.Fields(nextTrimmed), " ")
			
			// Check if next line matches an original commented line
			shouldComment := false
			if origCommentedLines[nextNormalized] {
				shouldComment = true
				logger.Info("fixWronglyUncommentedPreambleLines: found split comment (exact match)",
					logger.Int("lineNum", i+1),
					logger.String("nextLine", nextTrimmed[:minInt(60, len(nextTrimmed))]))
			} else {
				// Check partial match
				for origLine := range origCommentedLines {
					if len(nextNormalized) > 10 && strings.Contains(origLine, nextNormalized) {
						shouldComment = true
						logger.Info("fixWronglyUncommentedPreambleLines: found split comment (partial match)",
							logger.Int("lineNum", i+1),
							logger.String("nextLine", nextTrimmed[:minInt(60, len(nextTrimmed))]),
							logger.String("origLine", origLine[:minInt(60, len(origLine))]))
						break
					}
				}
			}
			
			// Also check for common comment indicators
			if !shouldComment {
				lowerNext := strings.ToLower(nextTrimmed)
				splitCommentIndicators := []string{
					"recommended", "optional", "packages", "figures", "typesetting",
					"hyperref", "hyperlinks", "resulting", "build breaks",
					"comment out", "following", "attempt", "algorithmic",
					"initial blind", "submitted", "review", "preprint",
					"accepted", "camera-ready", "submission", "for preprint",
					"above.", "better", "work together", "for the",
					"use the", "instead use", "if accepted",
				}
				for _, indicator := range splitCommentIndicators {
					if strings.Contains(lowerNext, indicator) {
						shouldComment = true
						logger.Info("fixWronglyUncommentedPreambleLines: found split comment (indicator match)",
							logger.Int("lineNum", i+1),
							logger.String("nextLine", nextTrimmed[:minInt(60, len(nextTrimmed))]),
							logger.String("indicator", indicator))
						break
					}
				}
			}
			
			if shouldComment {
				// Comment the next line
				leadingWhitespace := nextLine[:len(nextLine)-len(strings.TrimLeft(nextLine, " \t"))]
				transLines[i+1] = leadingWhitespace + "% " + nextTrimmed
				fixed = true
			}
		}
	}

	// SECOND PASS: Check each line in translated preamble for other uncommented lines
	for i, line := range transLines {
		trimmed := strings.TrimSpace(line)
		
		// Skip empty lines, lines that are already comments, and lines starting with \
		if trimmed == "" || strings.HasPrefix(trimmed, "%") || strings.HasPrefix(trimmed, "\\") {
			continue
		}

		// Normalize the line for comparison
		normalized := strings.Join(strings.Fields(trimmed), " ")

		// Check if this line (or a similar line) was commented in original
		if origCommentedLines[normalized] {
			// This line was commented in original but is not commented in translated
			// Add % at the beginning
			leadingWhitespace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			transLines[i] = leadingWhitespace + "% " + trimmed
			fixed = true
			logger.Info("fixWronglyUncommentedPreambleLines: commenting line (exact match)",
				logger.String("line", trimmed))
		} else {
			// Check for partial matches - the translated line might be a substring of an original commented line
			// This handles cases where lines are wrapped differently
			partialMatch := false
			for origLine := range origCommentedLines {
				if len(normalized) > 15 && strings.Contains(origLine, normalized) {
					leadingWhitespace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
					transLines[i] = leadingWhitespace + "% " + trimmed
					fixed = true
					partialMatch = true
					logger.Info("fixWronglyUncommentedPreambleLines: commenting line (partial match)",
						logger.String("line", trimmed),
						logger.String("origLine", origLine[:minInt(60, len(origLine))]))
					break
				}
			}
			
			if !partialMatch {
				// Also check if this looks like a comment that lost its % marker
				// Common patterns: lines that are English text (not LaTeX commands)
				// Check if this line contains words that suggest it's a comment
				lowerLine := strings.ToLower(trimmed)
				commentIndicators := []string{
					"recommended", "optional", "packages", "figures", "typesetting",
					"hyperref", "hyperlinks", "resulting", "build breaks",
					"comment out", "following", "attempt", "algorithmic",
					"initial blind", "submitted", "review", "preprint",
					"accepted", "camera-ready", "submission", "setting:",
					"above.", "better", "work together", "for the",
					"use the", "instead use", "if accepted", "for preprint",
				}
				
				// Check for comment indicators
				for _, indicator := range commentIndicators {
					if strings.Contains(lowerLine, indicator) {
						// Additional check: if line contains LaTeX commands, make sure it's really a comment
						// by checking if it has explanatory text patterns
						if strings.ContainsAny(trimmed, "{}$\\") {
							// Line has LaTeX commands - only comment if it has clear comment patterns
							if strings.Contains(lowerLine, "with") || 
							   strings.Contains(lowerLine, "above") ||
							   strings.Contains(lowerLine, "instead") ||
							   strings.Contains(lowerLine, "for the") ||
							   strings.Contains(lowerLine, "use the") {
								leadingWhitespace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
								transLines[i] = leadingWhitespace + "% " + trimmed
								fixed = true
								logger.Info("fixWronglyUncommentedPreambleLines: commenting suspected comment line (with LaTeX)",
									logger.String("line", trimmed),
									logger.String("indicator", indicator))
								break
							}
						} else if len(trimmed) > 10 {
							// No LaTeX commands - safe to comment
							leadingWhitespace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
							transLines[i] = leadingWhitespace + "% " + trimmed
							fixed = true
							logger.Info("fixWronglyUncommentedPreambleLines: commenting suspected comment line",
								logger.String("line", trimmed),
								logger.String("indicator", indicator))
							break
						}
					}
				}
			}
		}
	}

	// THIRD PASS: Handle line offset caused by \usepackage{ctex} insertion
	// When ctex is inserted after \documentclass, it shifts all subsequent lines by 1
	// This can cause FixByLineComparison to incorrectly uncomment lines
	// We need to check if any line in translated that should be commented (based on original)
	// was incorrectly left uncommented due to line offset
	if lineOffset > 0 && hasCtexInTrans && !hasCtexInOrig {
		logger.Info("fixWronglyUncommentedPreambleLines: applying line offset correction",
			logger.Int("lineOffset", lineOffset))
		
		// For each line in translated preamble (after ctex line), check if the corresponding
		// line in original (offset by -1) was commented
		for i := 0; i < len(transLines); i++ {
			transLine := transLines[i]
			transTrimmed := strings.TrimSpace(transLine)
			
			// Skip if already commented, empty, or is the ctex line itself
			if strings.HasPrefix(transTrimmed, "%") || transTrimmed == "" {
				continue
			}
			if strings.Contains(transLine, `\usepackage{ctex}`) {
				continue
			}
			
			// Calculate the corresponding line in original (accounting for offset)
			origIdx := i - lineOffset
			if origIdx < 0 || origIdx >= len(origLines) {
				continue
			}
			
			origLine := origLines[origIdx]
			origTrimmed := strings.TrimSpace(origLine)
			
			// Check if original line was commented
			if strings.HasPrefix(origTrimmed, "%") {
				// Original was commented - check if content matches
				origContent := strings.TrimSpace(strings.TrimPrefix(origTrimmed, "%"))
				origContent = strings.TrimSpace(strings.TrimPrefix(origContent, " "))
				
				// Normalize both for comparison
				origNormalized := strings.Join(strings.Fields(origContent), " ")
				transNormalized := strings.Join(strings.Fields(transTrimmed), " ")
				
				// Check if they're similar (same LaTeX command or similar content)
				shouldComment := false
				
				// Check for exact match
				if origNormalized == transNormalized {
					shouldComment = true
				} else {
					// Check if they start with the same LaTeX command
					cmdPattern := regexp.MustCompile(`^\\[a-zA-Z]+`)
					origCmd := cmdPattern.FindString(origNormalized)
					transCmd := cmdPattern.FindString(transNormalized)
					
					if origCmd != "" && origCmd == transCmd {
						// Same command - check if arguments are similar
						// This handles cases like:
						// Original: % \usepackage{icml2026} with \usepackage[nohyperref]{icml2026} above.
						// Translated: \usepackage{icml2026} with \usepackage[nohyperref]{icml2026} above.
						if strings.Contains(origNormalized, "with") && strings.Contains(transNormalized, "with") {
							shouldComment = true
						} else if strings.Contains(origNormalized, "above") && strings.Contains(transNormalized, "above") {
							shouldComment = true
						} else if strings.HasPrefix(origNormalized, transNormalized[:minInt(30, len(transNormalized))]) {
							shouldComment = true
						}
					}
				}
				
				if shouldComment {
					leadingWhitespace := transLine[:len(transLine)-len(strings.TrimLeft(transLine, " \t"))]
					transLines[i] = leadingWhitespace + "% " + transTrimmed
					fixed = true
					logger.Info("fixWronglyUncommentedPreambleLines: commenting line due to offset correction",
						logger.Int("transLineNum", i),
						logger.Int("origLineNum", origIdx),
						logger.String("transLine", transTrimmed[:minInt(60, len(transTrimmed))]),
						logger.String("origLine", origTrimmed[:minInt(60, len(origTrimmed))]))
				}
			}
		}
	}

	if fixed {
		transPreamble = strings.Join(transLines, "\n")
		return transPreamble + transBody, true
	}

	return translated, false
}
// There should only be ONE \end{document} at the very end of the file.
func fixDuplicateEndDocument(translated, original string) (string, bool) {
	endDocPattern := regexp.MustCompile(`\\end\{document\}`)
	matches := endDocPattern.FindAllStringIndex(translated, -1)

	if len(matches) <= 1 {
		return translated, false
	}

	// Multiple \end{document} found - keep only the last one
	logger.Warn("found multiple \\end{document} tags",
		logger.Int("count", len(matches)))

	result := translated
	// Remove all but the last \end{document}
	for i := 0; i < len(matches)-1; i++ {
		result = strings.Replace(result, "\\end{document}", "%%REMOVED_END_DOC%%", 1)
	}
	// Clean up the markers (remove the lines entirely if they only contain the marker)
	lines := strings.Split(result, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "%%REMOVED_END_DOC%%" {
			// Skip this line entirely
			continue
		}
		// Replace marker with empty string if it's part of a larger line
		line = strings.ReplaceAll(line, "%%REMOVED_END_DOC%%", "")
		cleanedLines = append(cleanedLines, line)
	}
	result = strings.Join(cleanedLines, "\n")

	return result, true
}

// fixExtraClosingBraces fixes patterns like \textit{...}} where there's an extra }
// This commonly happens when LLM adds extra braces after commands.
func fixExtraClosingBraces(translated, original string) (string, bool) {
	if original == "" {
		return translated, false
	}

	// Common patterns where LLM adds extra closing braces
	// IMPORTANT: These patterns should only match when the }} is truly extra,
	// not when the second } is closing a parent command like \textbf{\textit{n}}
	patterns := []struct {
		pattern *regexp.Regexp
		replace string
		desc    string
	}{
		// \textit{...}} followed by space or end -> \textit{...}
		// The (?:\s|$) ensures we only match when }} is at word boundary
		{regexp.MustCompile(`(\\textit\{[^{}]*)\}\}(\s|$|[&,;])`), "$1}$2", "textit double brace"},
		// \textbf{...}} followed by space or end -> \textbf{...}
		{regexp.MustCompile(`(\\textbf\{[^{}]*)\}\}(\s|$|[&,;])`), "$1}$2", "textbf double brace"},
		// \emph{...}} followed by space or end -> \emph{...}
		{regexp.MustCompile(`(\\emph\{[^{}]*)\}\}(\s|$|[&,;])`), "$1}$2", "emph double brace"},
		// \underline{...}} followed by space or end -> \underline{...}
		{regexp.MustCompile(`(\\underline\{[^{}]*)\}\}(\s|$|[&,;])`), "$1}$2", "underline double brace"},
	}

	result := translated
	fixed := false

	for _, p := range patterns {
		if p.pattern.MatchString(result) {
			newResult := p.pattern.ReplaceAllString(result, p.replace)
			if newResult != result {
				result = newResult
				fixed = true
				logger.Debug("fixed extra closing braces", logger.String("pattern", p.desc))
			}
		}
	}

	return result, fixed
}
func cleanTranslationResult(content string) string {
	// Remove JSON wrapper if present
	if strings.Contains(content, "```json") {
		jsonStart := strings.Index(content, "```json")
		content = content[:jsonStart]
	}
	
	// Remove JSON field markers
	content = strings.ReplaceAll(content, `"translated_content":`, "")
	content = strings.ReplaceAll(content, `"translated_content": "`, "")
	content = strings.ReplaceAll(content, `"translated_content":"`, "")
	
	// Remove markdown code blocks
	content = strings.ReplaceAll(content, "```latex", "")
	content = strings.ReplaceAll(content, "```", "")
	
	// Trim whitespace
	content = strings.TrimSpace(content)
	
	return content
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================
// PlaceholderSystem - 占位符系统
// ============================================================

// PlaceholderConfig 占位符配置
// Validates: Requirements 1.2
type PlaceholderConfig struct {
	Prefix    string // 占位符前缀，默认 "<<<LATEX_"
	Suffix    string // 占位符后缀，默认 ">>>"
	Separator string // 类型和编号分隔符，默认 "_"
}

// DefaultPlaceholderConfig 返回默认占位符配置
func DefaultPlaceholderConfig() *PlaceholderConfig {
	return &PlaceholderConfig{
		Prefix:    "<<<LATEX_",
		Suffix:    ">>>",
		Separator: "_",
	}
}

// PlaceholderSystem 占位符系统，管理占位符的生成、存储和恢复
// Validates: Requirements 1.2, 1.5
type PlaceholderSystem struct {
	config       *PlaceholderConfig
	placeholders map[string]string // placeholder -> original content
	counters     map[StructureType]int // 每种类型的计数器
	mu           sync.RWMutex
}

// NewPlaceholderSystem 创建新的占位符系统
func NewPlaceholderSystem() *PlaceholderSystem {
	return NewPlaceholderSystemWithConfig(DefaultPlaceholderConfig())
}

// NewPlaceholderSystemWithConfig 使用自定义配置创建占位符系统
func NewPlaceholderSystemWithConfig(config *PlaceholderConfig) *PlaceholderSystem {
	if config == nil {
		config = DefaultPlaceholderConfig()
	}
	return &PlaceholderSystem{
		config:       config,
		placeholders: make(map[string]string),
		counters:     make(map[StructureType]int),
	}
}

// GeneratePlaceholder 生成唯一占位符
// 根据结构类型生成格式为 <<<LATEX_TYPE_N>>> 的占位符
// Validates: Requirements 1.2
func (ps *PlaceholderSystem) GeneratePlaceholder(structureType StructureType) string {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 获取当前计数并递增
	count := ps.counters[structureType]
	ps.counters[structureType] = count + 1

	// 根据类型生成占位符名称
	typeName := ps.getTypeName(structureType)
	
	// 生成占位符: <<<LATEX_TYPE_N>>>
	placeholder := fmt.Sprintf("%s%s%s%d%s",
		ps.config.Prefix,
		typeName,
		ps.config.Separator,
		count,
		ps.config.Suffix)

	return placeholder
}

// getTypeName 获取结构类型的名称
func (ps *PlaceholderSystem) getTypeName(structureType StructureType) string {
	switch structureType {
	case StructureCommand:
		return "CMD"
	case StructureEnvironment:
		return "ENV"
	case StructureMathInline:
		return "MATH"
	case StructureMathDisplay:
		return "MATH"
	case StructureTable:
		return "TABLE"
	case StructureComment:
		return "COMMENT"
	default:
		return "UNKNOWN"
	}
}

// StorePlaceholder 存储占位符映射
// Validates: Requirements 1.2
func (ps *PlaceholderSystem) StorePlaceholder(placeholder string, original string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.placeholders[placeholder] = original
	logger.Debug("stored placeholder",
		logger.String("placeholder", placeholder),
		logger.Int("originalLength", len(original)))
}

// RestorePlaceholder 恢复单个占位符
// 将内容中的指定占位符替换为原始内容
// Validates: Requirements 1.2
func (ps *PlaceholderSystem) RestorePlaceholder(content string, placeholder string) string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	original, exists := ps.placeholders[placeholder]
	if !exists {
		logger.Warn("placeholder not found for restoration",
			logger.String("placeholder", placeholder))
		return content
	}

	return strings.ReplaceAll(content, placeholder, original)
}

// RestoreAll 恢复所有占位符
// 将内容中的所有占位符替换为原始内容
// Validates: Requirements 1.2
func (ps *PlaceholderSystem) RestoreAll(content string) string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := content
	for placeholder, original := range ps.placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}

	logger.Debug("restored all placeholders",
		logger.Int("placeholderCount", len(ps.placeholders)),
		logger.Int("originalLength", len(content)),
		logger.Int("restoredLength", len(result)))

	return result
}

// ValidatePlaceholders 验证所有占位符是否存在于内容中
// 返回丢失的占位符列表
// Validates: Requirements 1.5
func (ps *PlaceholderSystem) ValidatePlaceholders(content string) []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var missing []string
	for placeholder := range ps.placeholders {
		if !strings.Contains(content, placeholder) {
			missing = append(missing, placeholder)
		}
	}

	if len(missing) > 0 {
		logger.Warn("missing placeholders detected",
			logger.Int("missingCount", len(missing)),
			logger.String("missing", strings.Join(missing, ", ")))
	}

	return missing
}

// GetPlaceholders 获取所有占位符映射的副本
func (ps *PlaceholderSystem) GetPlaceholders() map[string]string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make(map[string]string, len(ps.placeholders))
	for k, v := range ps.placeholders {
		result[k] = v
	}
	return result
}

// GetPlaceholderCount 获取占位符数量
func (ps *PlaceholderSystem) GetPlaceholderCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.placeholders)
}

// Clear 清空所有占位符和计数器
func (ps *PlaceholderSystem) Clear() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.placeholders = make(map[string]string)
	ps.counters = make(map[StructureType]int)
}

// ProtectContent 保护内容中的LaTeX结构
// 识别并替换所有LaTeX结构为占位符
// 返回受保护的内容
// Validates: Requirements 1.2
func (ps *PlaceholderSystem) ProtectContent(content string) string {
	// 识别所有结构
	structures := IdentifyStructures(content)
	
	if len(structures) == 0 {
		return content
	}

	// 按位置降序排序，以便从后向前替换
	sort.Slice(structures, func(i, j int) bool {
		return structures[i].Start > structures[j].Start
	})

	result := content
	for _, structure := range structures {
		// 跳过嵌套结构（它们会被父结构保护）
		if structure.IsNested {
			continue
		}

		// 生成占位符
		placeholder := ps.GeneratePlaceholder(structure.Type)
		
		// 存储映射
		ps.StorePlaceholder(placeholder, structure.Content)
		
		// 替换内容
		result = result[:structure.Start] + placeholder + result[structure.End:]
	}

	logger.Debug("protected content with placeholders",
		logger.Int("structureCount", len(structures)),
		logger.Int("placeholderCount", ps.GetPlaceholderCount()))

	return result
}

// RecoverMissingPlaceholders 尝试恢复丢失的占位符
// 当翻译过程中占位符丢失时，尝试通过多种策略恢复
// 使用增强的恢复机制，包括模式匹配、行分析等策略
// Validates: Requirements 1.5
func (ps *PlaceholderSystem) RecoverMissingPlaceholders(content string, missing []string) string {
	result, _ := ps.RecoverMissingPlaceholdersWithReport(content, "", missing)
	return result
}

// RecoverMissingPlaceholdersWithContext 使用上下文分析恢复丢失的占位符
// 这是一个更智能的恢复策略，尝试根据原始内容的上下文找到合适的插入位置
// Validates: Requirements 1.5
func (ps *PlaceholderSystem) RecoverMissingPlaceholdersWithContext(content string, originalContent string, missing []string) string {
	result, _ := ps.RecoverMissingPlaceholdersWithReport(content, originalContent, missing)
	return result
}

// RecoverMissingPlaceholdersWithReport 恢复丢失的占位符并返回详细报告
// 这是主要的恢复函数，实现多种恢复策略
// Validates: Requirements 1.5
func (ps *PlaceholderSystem) RecoverMissingPlaceholdersWithReport(content string, originalContent string, missing []string) (string, *RecoveryReport) {
	if len(missing) == 0 {
		return content, NewRecoveryReport()
	}

	ps.mu.RLock()
	placeholdersCopy := make(map[string]string, len(ps.placeholders))
	for k, v := range ps.placeholders {
		placeholdersCopy[k] = v
	}
	ps.mu.RUnlock()

	// 使用增强的恢复函数
	return recoverMissingPlaceholdersWithReport(content, originalContent, placeholdersCopy, missing)
}

// PlaceholderInfo 占位符信息
type PlaceholderInfo struct {
	Placeholder string        // 占位符字符串
	Original    string        // 原始内容
	Type        StructureType // 结构类型
	Index       int           // 索引编号
}

// GetPlaceholderInfo 获取占位符的详细信息
func (ps *PlaceholderSystem) GetPlaceholderInfo(placeholder string) (*PlaceholderInfo, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	original, exists := ps.placeholders[placeholder]
	if !exists {
		return nil, false
	}

	// 解析占位符类型和索引
	info := &PlaceholderInfo{
		Placeholder: placeholder,
		Original:    original,
	}

	// 尝试解析不同类型的占位符
	typePatterns := map[string]StructureType{
		"<<<LATEX_CMD_%d>>>":     StructureCommand,
		"<<<LATEX_MATH_%d>>>":    StructureMathInline, // 或 StructureMathDisplay
		"<<<LATEX_ENV_%d>>>":     StructureEnvironment,
		"<<<LATEX_TABLE_%d>>>":   StructureTable,
		"<<<LATEX_COMMENT_%d>>>": StructureComment,
	}

	for pattern, structType := range typePatterns {
		var idx int
		if n, err := fmt.Sscanf(placeholder, pattern, &idx); n == 1 && err == nil {
			info.Type = structType
			info.Index = idx
			break
		}
	}

	return info, true
}

// HasPlaceholder 检查是否存在指定的占位符
func (ps *PlaceholderSystem) HasPlaceholder(placeholder string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	_, exists := ps.placeholders[placeholder]
	return exists
}

// GetOriginal 获取占位符对应的原始内容
func (ps *PlaceholderSystem) GetOriginal(placeholder string) (string, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	original, exists := ps.placeholders[placeholder]
	return original, exists
}

// MergePlaceholders 合并另一个占位符系统的占位符
func (ps *PlaceholderSystem) MergePlaceholders(other *PlaceholderSystem) {
	if other == nil {
		return
	}

	other.mu.RLock()
	defer other.mu.RUnlock()

	ps.mu.Lock()
	defer ps.mu.Unlock()

	for k, v := range other.placeholders {
		ps.placeholders[k] = v
	}
}

// Clone 创建占位符系统的副本
func (ps *PlaceholderSystem) Clone() *PlaceholderSystem {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	clone := &PlaceholderSystem{
		config:       ps.config,
		placeholders: make(map[string]string, len(ps.placeholders)),
		counters:     make(map[StructureType]int, len(ps.counters)),
	}

	for k, v := range ps.placeholders {
		clone.placeholders[k] = v
	}
	for k, v := range ps.counters {
		clone.counters[k] = v
	}

	return clone
}

// commentPlaceholder represents a protected comment environment
type commentPlaceholder struct {
	placeholder string
	original    string
}

// protectCommentEnvironments finds and replaces \begin{comment}...\end{comment} blocks
// with placeholders to prevent them from being translated.
// The comment package in LaTeX treats everything between these tags as comments,
// so they should not be translated.
func protectCommentEnvironments(content string) (string, []commentPlaceholder) {
	var placeholders []commentPlaceholder
	result := content
	
	// Pattern to match \begin{comment}...\end{comment} including nested content
	beginTag := `\begin{comment}`
	endTag := `\end{comment}`
	
	counter := 0
	for {
		beginPos := strings.Index(result, beginTag)
		if beginPos == -1 {
			break
		}
		
		// Find matching end tag
		endPos := strings.Index(result[beginPos:], endTag)
		if endPos == -1 {
			// No matching end tag found, skip this begin tag
			logger.Warn("unmatched \\begin{comment} found", logger.Int("position", beginPos))
			break
		}
		endPos += beginPos + len(endTag)
		
		// Extract the complete comment block
		commentBlock := result[beginPos:endPos]
		
		// Create placeholder
		placeholder := fmt.Sprintf("%%COMMENT_ENV_PLACEHOLDER_%d%%", counter)
		counter++
		
		placeholders = append(placeholders, commentPlaceholder{
			placeholder: placeholder,
			original:    commentBlock,
		})
		
		// Replace in result
		result = result[:beginPos] + placeholder + result[endPos:]
		
		logger.Debug("protected comment environment",
			logger.Int("index", counter-1),
			logger.Int("length", len(commentBlock)))
	}
	
	return result, placeholders
}

// restoreCommentEnvironments restores the original comment environments from placeholders
func restoreCommentEnvironments(content string, placeholders []commentPlaceholder) string {
	result := content
	
	for _, p := range placeholders {
		result = strings.Replace(result, p.placeholder, p.original, 1)
	}
	
	return result
}
