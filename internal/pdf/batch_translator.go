package pdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"latex-translator/internal/logger"
)

// BatchSeparator is the delimiter used to separate text blocks in a batch for translation
const BatchSeparator = "\n---BLOCK_SEPARATOR---\n"

// DefaultContextWindow is the default context window size in characters
const DefaultContextWindow = 4000

// DefaultConcurrency is the default number of concurrent batch translations
const DefaultConcurrency = 3

// DefaultTimeout is the default HTTP client timeout
const DefaultTimeout = 180 * time.Second

// DefaultMaxRetries is the default maximum number of retry attempts for API errors
const DefaultMaxRetries = 3

// BaseRetryDelay is the base delay between retries (exponential backoff)
const BaseRetryDelay = 2 * time.Second

// BatchTranslator 负责批量翻译文本
type BatchTranslator struct {
	apiKey        string
	baseURL       string
	model         string
	contextWindow int
	concurrency   int
	client        *http.Client
}

// BatchTranslatorConfig holds configuration options for creating a BatchTranslator
type BatchTranslatorConfig struct {
	APIKey        string
	BaseURL       string
	Model         string
	ContextWindow int
	Concurrency   int
	Timeout       time.Duration
}

// NewBatchTranslator creates a new BatchTranslator with the given configuration
func NewBatchTranslator(cfg BatchTranslatorConfig) *BatchTranslator {
	// Apply defaults for zero values
	contextWindow := cfg.ContextWindow
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &BatchTranslator{
		apiKey:        cfg.APIKey,
		baseURL:       cfg.BaseURL,
		model:         cfg.Model,
		contextWindow: contextWindow,
		concurrency:   concurrency,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// MergeBatches 将文本块合并为批次
// 根据上下文窗口大小动态调整批次大小
// Property 5: 每个批次的总字符数应小于上下文窗口大小，所有批次的文本块总数等于输入总数
func (b *BatchTranslator) MergeBatches(blocks []TextBlock) [][]TextBlock {
	if len(blocks) == 0 {
		return nil
	}

	var batches [][]TextBlock
	var currentBatch []TextBlock
	currentBatchSize := 0

	// Calculate the overhead per block (separator size)
	separatorSize := len(BatchSeparator)

	for _, block := range blocks {
		blockSize := len(block.Text)

		// If a single block exceeds the context window, it must be in its own batch
		if blockSize >= b.contextWindow {
			// Flush current batch if not empty
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = nil
				currentBatchSize = 0
			}
			// Add the large block as its own batch
			batches = append(batches, []TextBlock{block})
			continue
		}

		// Calculate the size if we add this block to the current batch
		additionalSize := blockSize
		if len(currentBatch) > 0 {
			// Add separator size for non-first blocks
			additionalSize += separatorSize
		}

		// Check if adding this block would exceed the context window
		if currentBatchSize+additionalSize > b.contextWindow {
			// Flush current batch and start a new one
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
			}
			currentBatch = []TextBlock{block}
			currentBatchSize = blockSize
		} else {
			// Add block to current batch
			currentBatch = append(currentBatch, block)
			currentBatchSize += additionalSize
		}
	}

	// Don't forget the last batch
	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// GetBatchText combines all text blocks in a batch into a single string with separators
func (b *BatchTranslator) GetBatchText(batch []TextBlock) string {
	if len(batch) == 0 {
		return ""
	}

	result := batch[0].Text
	for i := 1; i < len(batch); i++ {
		result += BatchSeparator + batch[i].Text
	}
	return result
}

// GetContextWindow returns the configured context window size
func (b *BatchTranslator) GetContextWindow() int {
	return b.contextWindow
}

// GetConcurrency returns the configured concurrency level
func (b *BatchTranslator) GetConcurrency() int {
	return b.concurrency
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

// TranslateBatch 批量翻译文本块
// Property 6: 翻译结果映射 - 翻译后返回的TranslatedBlock数量等于输入TextBlock数量，ID一致
// Validates: Requirements 3.3, 3.6
func (b *BatchTranslator) TranslateBatch(blocks []TextBlock) ([]TranslatedBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	// Merge blocks into batches based on context window
	batches := b.MergeBatches(blocks)
	if len(batches) == 0 {
		return nil, nil
	}

	// Create result slice to hold all translated blocks
	results := make([]TranslatedBlock, 0, len(blocks))
	
	// Use semaphore for concurrency control
	sem := make(chan struct{}, b.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Store batch results with their original order
	type batchResult struct {
		index  int
		blocks []TranslatedBlock
		err    error
	}
	batchResults := make([]batchResult, len(batches))

	// Process batches concurrently
	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, batchBlocks []TextBlock) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Translate this batch
			translatedBlocks, err := b.translateSingleBatch(batchBlocks)
			
			mu.Lock()
			batchResults[idx] = batchResult{
				index:  idx,
				blocks: translatedBlocks,
				err:    err,
			}
			mu.Unlock()
		}(i, batch)
	}

	// Wait for all batches to complete
	wg.Wait()

	// Check for errors and collect results in order
	for _, br := range batchResults {
		if br.err != nil {
			return nil, br.err
		}
		results = append(results, br.blocks...)
	}

	return results, nil
}

// translateSingleBatch translates a single batch of text blocks
func (b *BatchTranslator) translateSingleBatch(batch []TextBlock) ([]TranslatedBlock, error) {
	if len(batch) == 0 {
		return nil, nil
	}

	// Get combined batch text
	batchText := b.GetBatchText(batch)

	// Call OpenAI API
	translatedText, err := b.callOpenAIAPI(batchText)
	if err != nil {
		return nil, err
	}

	// Split the translated text back into individual blocks
	translatedParts := b.splitTranslatedText(translatedText, len(batch))

	// Map translated text back to original blocks
	results := make([]TranslatedBlock, len(batch))
	for i, block := range batch {
		results[i] = TranslatedBlock{
			TextBlock:      block,
			TranslatedText: translatedParts[i],
			FromCache:      false,
		}
	}

	return results, nil
}

// splitTranslatedText splits the translated text by BatchSeparator and ensures
// the number of parts matches the expected count
func (b *BatchTranslator) splitTranslatedText(translatedText string, expectedCount int) []string {
	// Split by the batch separator
	parts := strings.Split(translatedText, BatchSeparator)

	// If we got the expected number of parts, return them
	if len(parts) == expectedCount {
		// Trim whitespace from each part
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	// If we got fewer parts, pad with empty strings
	if len(parts) < expectedCount {
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		for len(parts) < expectedCount {
			parts = append(parts, "")
		}
		return parts
	}

	// If we got more parts, try to merge extra parts into the last expected slot
	// This handles cases where the separator might appear in the translated text
	result := make([]string, expectedCount)
	for i := 0; i < expectedCount-1; i++ {
		result[i] = strings.TrimSpace(parts[i])
	}
	// Merge remaining parts into the last slot
	remaining := parts[expectedCount-1:]
	result[expectedCount-1] = strings.TrimSpace(strings.Join(remaining, BatchSeparator))

	return result
}

// callOpenAIAPI calls the OpenAI API to translate the batch text
func (b *BatchTranslator) callOpenAIAPI(batchText string) (string, error) {
	logger.Info("calling OpenAI API",
		logger.String("model", b.model),
		logger.Int("textLen", len(batchText)),
		logger.String("baseURL", b.baseURL))

	// Build the translation prompt
	systemPrompt := b.buildSystemPrompt()
	userPrompt := b.buildUserPrompt(batchText)

	// Create the request body
	reqBody := ChatCompletionRequest{
		Model: b.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3, // Lower temperature for more consistent translations
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", NewPDFError(ErrAPIFailed, "failed to marshal request body", err)
	}

	// Normalize API URL
	apiURL := b.normalizeAPIURL(b.baseURL)

	// Create HTTP request
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", NewPDFError(ErrAPIFailed, "failed to create HTTP request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	// Send the request
	resp, err := b.client.Do(req)
	if err != nil {
		logger.Error("API request failed", err)
		return "", NewPDFError(ErrAPIFailed, "API request failed", err)
	}
	defer resp.Body.Close()

	logger.Debug("API response received", logger.Int("statusCode", resp.StatusCode))

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", NewPDFError(ErrAPIFailed, "failed to read API response", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", b.handleAPIHTTPError(resp.StatusCode, body)
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", NewPDFError(ErrAPIFailed, "failed to parse API response", err)
	}

	// Check for API error in response
	if chatResp.Error != nil {
		return "", NewPDFErrorWithDetails(ErrAPIFailed, "API returned error", chatResp.Error.Message, nil)
	}

	// Extract translated content
	if len(chatResp.Choices) == 0 {
		return "", NewPDFError(ErrAPIFailed, "API returned no choices", nil)
	}

	return chatResp.Choices[0].Message.Content, nil
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// normalizeAPIURL ensures the API URL ends with /chat/completions
func (b *BatchTranslator) normalizeAPIURL(url string) string {
	if url == "" {
		return "https://api.openai.com/v1/chat/completions"
	}

	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")

	// If URL already ends with /chat/completions, return as is
	if strings.HasSuffix(url, "/chat/completions") {
		return url
	}

	// Append /chat/completions
	return url + "/chat/completions"
}

// buildSystemPrompt creates the system prompt for PDF text translation
func (b *BatchTranslator) buildSystemPrompt() string {
	return `You are a professional translator specializing in academic and scientific documents.
Your task is to translate text extracted from PDF documents from English to Chinese.

CRITICAL RULES:
1. Translate the text content from English to Chinese accurately.
2. Preserve any mathematical formulas, symbols, or special characters exactly as they are.
3. Maintain proper Chinese punctuation (use Chinese quotation marks, periods, etc.).
4. Do not add any explanations or notes - output only the translated text.
5. IMPORTANT: The input may contain multiple text blocks separated by "` + BatchSeparator + `".
6. You MUST preserve these separators in your output exactly as they appear.
7. Each block should be translated independently but the separators must remain intact.
8. Do not merge blocks or remove separators.`
}

// buildUserPrompt creates the user prompt with the content to translate
func (b *BatchTranslator) buildUserPrompt(batchText string) string {
	return fmt.Sprintf(`Translate the following text from English to Chinese. 
If there are multiple blocks separated by "%s", translate each block separately and keep the separators in your output.

%s`, BatchSeparator, batchText)
}

// handleAPIHTTPError creates an appropriate PDFError based on the HTTP status code
func (b *BatchTranslator) handleAPIHTTPError(statusCode int, body []byte) error {
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
		return NewPDFErrorWithDetails(ErrAPIFailed, "API authentication failed", "invalid API key or unauthorized access", nil)
	case http.StatusTooManyRequests:
		return NewPDFErrorWithDetails(ErrAPIFailed, "API rate limit exceeded", errorDetails, nil)
	case http.StatusBadRequest:
		return NewPDFErrorWithDetails(ErrAPIFailed, "invalid API request", errorDetails, nil)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return NewPDFErrorWithDetails(ErrAPIFailed, "API server error", fmt.Sprintf("status %d: %s", statusCode, errorDetails), nil)
	default:
		return NewPDFErrorWithDetails(ErrAPIFailed, "API request failed", fmt.Sprintf("status %d: %s", statusCode, errorDetails), nil)
	}
}


// ProgressCallback is a function type for progress updates during translation
// completed: number of blocks translated so far
// total: total number of blocks to translate
type ProgressCallback func(completed, total int)

// TranslateWithRetry 带重试的翻译
// 实现错误处理和重试逻辑：
// 1. 先将块分成批次
// 2. 对每个批次尝试翻译，最多重试 maxRetries 次
// 3. 使用指数退避策略
// 4. 如果批量翻译最终失败，降级为单块翻译
// Validates: Requirements 3.4, 7.3
func (b *BatchTranslator) TranslateWithRetry(blocks []TextBlock, maxRetries int) ([]TranslatedBlock, error) {
	return b.TranslateWithRetryAndProgress(blocks, maxRetries, nil)
}

// TranslateWithRetryAndProgress 带重试和进度回调的翻译
// progressCallback: 可选的进度回调函数，每完成一个批次后调用
func (b *BatchTranslator) TranslateWithRetryAndProgress(blocks []TextBlock, maxRetries int, progressCallback ProgressCallback) ([]TranslatedBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	// First, merge blocks into batches based on context window
	batches := b.MergeBatches(blocks)
	if len(batches) == 0 {
		return nil, nil
	}

	logger.Info("starting batch translation",
		logger.Int("totalBlocks", len(blocks)),
		logger.Int("batchCount", len(batches)),
		logger.Int("contextWindow", b.contextWindow))

	// Process each batch with retries
	results := make([]TranslatedBlock, 0, len(blocks))
	completedBlocks := 0
	totalBlocks := len(blocks)
	
	for batchIdx, batch := range batches {
		logger.Debug("translating batch",
			logger.Int("batchIndex", batchIdx+1),
			logger.Int("totalBatches", len(batches)),
			logger.Int("blocksInBatch", len(batch)))

		// Try batch translation with retries
		var lastErr error
		var translatedBatch []TranslatedBlock
		
		for attempt := 1; attempt <= maxRetries; attempt++ {
			logger.Debug("batch translation attempt",
				logger.Int("batchIndex", batchIdx+1),
				logger.Int("attempt", attempt),
				logger.Int("maxRetries", maxRetries))

			var err error
			translatedBatch, err = b.translateSingleBatch(batch)
			if err == nil {
				logger.Debug("batch translation succeeded",
					logger.Int("batchIndex", batchIdx+1),
					logger.Int("attempt", attempt))
				break
			}

			lastErr = err
			logger.Warn("batch translation attempt failed",
				logger.Int("batchIndex", batchIdx+1),
				logger.Int("attempt", attempt),
				logger.Err(err))

			// Check if the error is retryable
			if !b.isRetryableError(err) {
				logger.Error("non-retryable error, failing batch", err)
				break
			}

			// Don't sleep after the last attempt
			if attempt < maxRetries {
				delay := b.calculateBackoffDelay(attempt)
				logger.Debug("retrying after delay",
					logger.String("delay", delay.String()),
					logger.Int("nextAttempt", attempt+1))
				time.Sleep(delay)
			}
		}

		// If batch translation failed, try single-block translation as fallback
		if translatedBatch == nil && lastErr != nil {
			logger.Warn("batch translation failed, falling back to single-block translation",
				logger.Int("batchIndex", batchIdx+1),
				logger.Int("blocksInBatch", len(batch)))
			
			var err error
			translatedBatch, err = b.translateBlocksIndividuallyWithProgress(batch, maxRetries, lastErr, completedBlocks, totalBlocks, progressCallback)
			if err != nil {
				return nil, err
			}
		}

		results = append(results, translatedBatch...)
		
		// Update progress after each batch
		completedBlocks += len(batch)
		if progressCallback != nil {
			progressCallback(completedBlocks, totalBlocks)
		}
	}

	logger.Info("batch translation completed",
		logger.Int("totalBlocks", len(blocks)),
		logger.Int("translatedBlocks", len(results)))

	return results, nil
}

// translateBlocksIndividually translates each block individually as a fallback
// when batch translation fails. This is the degradation strategy mentioned in
// the design document: "批次翻译失败 -> 降级为单块翻译"
func (b *BatchTranslator) translateBlocksIndividually(blocks []TextBlock, maxRetries int, batchErr error) ([]TranslatedBlock, error) {
	return b.translateBlocksIndividuallyWithProgress(blocks, maxRetries, batchErr, 0, len(blocks), nil)
}

// translateBlocksIndividuallyWithProgress translates each block individually with progress callback
func (b *BatchTranslator) translateBlocksIndividuallyWithProgress(blocks []TextBlock, maxRetries int, batchErr error, startCompleted, totalBlocks int, progressCallback ProgressCallback) ([]TranslatedBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	// If there's only one block and batch translation already failed,
	// return the original error since single-block translation won't help
	if len(blocks) == 1 {
		logger.Error("single block translation also failed (same as batch)", batchErr)
		return nil, NewPDFErrorWithDetails(
			ErrTranslateFailed,
			"translation failed",
			"batch and single-block translation both failed",
			batchErr,
		)
	}

	logger.Info("starting single-block translation fallback",
		logger.Int("blockCount", len(blocks)))

	results := make([]TranslatedBlock, 0, len(blocks))
	var failedBlocks []string
	completedInFallback := 0

	for i, block := range blocks {
		logger.Debug("translating single block",
			logger.Int("blockIndex", i+1),
			logger.Int("totalBlocks", len(blocks)),
			logger.String("blockID", block.ID))

		// Try to translate this single block with retries
		translatedBlock, err := b.translateSingleBlockWithRetry(block, maxRetries)
		if err != nil {
			logger.Warn("single block translation failed",
				logger.String("blockID", block.ID),
				logger.Err(err))
			
			// Record the failure but continue with other blocks
			failedBlocks = append(failedBlocks, block.ID)
			
			// Create a result with empty translation for failed blocks
			results = append(results, TranslatedBlock{
				TextBlock:      block,
				TranslatedText: "", // Empty translation for failed block
				FromCache:      false,
			})
		} else {
			results = append(results, translatedBlock)
		}
		
		// Update progress after each block in fallback mode
		completedInFallback++
		if progressCallback != nil {
			progressCallback(startCompleted+completedInFallback, totalBlocks)
		}
	}

	// If all blocks failed, return an error
	if len(failedBlocks) == len(blocks) {
		return nil, NewPDFErrorWithDetails(
			ErrTranslateFailed,
			"all blocks failed to translate",
			fmt.Sprintf("failed blocks: %v", failedBlocks),
			batchErr,
		)
	}

	// If some blocks failed, log a warning but return partial results
	if len(failedBlocks) > 0 {
		logger.Warn("some blocks failed to translate",
			logger.Int("failedCount", len(failedBlocks)),
			logger.Int("totalCount", len(blocks)))
	}

	logger.Info("single-block translation fallback completed",
		logger.Int("successCount", len(blocks)-len(failedBlocks)),
		logger.Int("failedCount", len(failedBlocks)))

	return results, nil
}

// translateSingleBlockWithRetry translates a single block with retry logic
func (b *BatchTranslator) translateSingleBlockWithRetry(block TextBlock, maxRetries int) (TranslatedBlock, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create a single-block batch
		singleBatch := []TextBlock{block}
		
		translatedBlocks, err := b.translateSingleBatch(singleBatch)
		if err == nil && len(translatedBlocks) > 0 {
			return translatedBlocks[0], nil
		}

		lastErr = err
		if err != nil {
			logger.Debug("single block translation attempt failed",
				logger.Int("attempt", attempt),
				logger.String("blockID", block.ID),
				logger.Err(err))
		}

		// Check if the error is retryable
		if !b.isRetryableError(err) {
			break
		}

		// Don't sleep after the last attempt
		if attempt < maxRetries {
			delay := b.calculateBackoffDelay(attempt)
			time.Sleep(delay)
		}
	}

	return TranslatedBlock{}, lastErr
}

// isRetryableError determines if an error should trigger a retry
// Retryable errors include:
// - Network errors
// - Rate limit errors (429)
// - Server errors (5xx)
// Non-retryable errors include:
// - Authentication failures (401)
// - Invalid requests (400)
func (b *BatchTranslator) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a PDFError
	if pdfErr, ok := err.(*PDFError); ok {
		// Check the error details for specific HTTP status codes
		details := pdfErr.Details
		
		// Non-retryable: authentication failures
		if strings.Contains(details, "authentication failed") ||
			strings.Contains(details, "invalid API key") ||
			strings.Contains(details, "unauthorized") {
			return false
		}
		
		// Non-retryable: invalid requests
		if strings.Contains(details, "invalid API request") ||
			strings.Contains(pdfErr.Message, "invalid API request") {
			return false
		}
		
		// Retryable: rate limits
		if strings.Contains(details, "rate limit") ||
			strings.Contains(pdfErr.Message, "rate limit") {
			return true
		}
		
		// Retryable: server errors (5xx)
		if strings.Contains(details, "status 5") ||
			strings.Contains(details, "server error") ||
			strings.Contains(pdfErr.Message, "server error") {
			return true
		}
		
		// Retryable: API failed errors (could be transient)
		if pdfErr.Code == ErrAPIFailed {
			// Default to retryable for API failures unless explicitly non-retryable
			return true
		}
	}

	// For other errors, check if they look like network errors
	errStr := err.Error()
	if strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "reset by peer") {
		return true
	}

	// Default to not retryable for unknown errors
	return false
}

// calculateBackoffDelay calculates the delay for exponential backoff
// The delay doubles with each attempt: 2s, 4s, 8s, etc.
func (b *BatchTranslator) calculateBackoffDelay(attempt int) time.Duration {
	// Exponential backoff: BaseRetryDelay * 2^(attempt-1)
	delay := BaseRetryDelay * time.Duration(1<<uint(attempt-1))
	
	// Cap the maximum delay at 30 seconds
	maxDelay := 30 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}
	
	return delay
}
