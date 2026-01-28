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
		Temperature: 0,
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

	// Split content into chunks for translation
	chunks := splitIntoChunks(content, MaxChunkSize)
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

	// Fix LaTeX line structure issues caused by LLM merging lines
	translatedContent = FixLaTeXLineStructure(translatedContent)

	// Apply reference-based fixes using original content
	// This compares the translated content with the original to fix structural issues
	translatedContent = ApplyReferenceBasedFixes(translatedContent, content)

	logger.Info("translation completed successfully", logger.Int("totalTokens", totalTokens))
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
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
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
	if estimatedOutputTokens < 4096 {
		estimatedOutputTokens = 4096 // Minimum 4096 tokens
	}
	if estimatedOutputTokens > 16384 {
		estimatedOutputTokens = 16384 // Cap at 16384 to avoid model limits
	}
	
	reqBody := ChatCompletionRequest{
		Model: t.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3, // Lower temperature for more consistent translations
		MaxTokens:   estimatedOutputTokens,
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
			logger.Int("completionTokens", chatResp.Usage.CompletionTokens))
		// Return the truncated content but log a warning
		// The caller should handle this appropriately
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

// recoverMissingPlaceholders attempts to recover missing placeholders by inserting them
// at the end of the content. This is a fallback mechanism when LLM drops placeholders.
func recoverMissingPlaceholders(content string, placeholders map[string]string, missing []string) string {
	if len(missing) == 0 {
		return content
	}

	// Sort missing placeholders by their number to maintain order
	sort.Slice(missing, func(i, j int) bool {
		// Extract numbers from placeholder names
		var numI, numJ int
		fmt.Sscanf(missing[i], "<<<LATEX_CMD_%d>>>", &numI)
		fmt.Sscanf(missing[j], "<<<LATEX_CMD_%d>>>", &numJ)
		return numI < numJ
	})

	// Append missing placeholders at the end with a warning comment
	result := content
	for _, placeholder := range missing {
		// Try to find a good insertion point based on the placeholder number
		// For now, just append at the end
		result += " " + placeholder
		logger.Warn("recovered missing placeholder by appending",
			logger.String("placeholder", placeholder))
	}

	return result
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
3. Keep the document structure and formatting intact.
4. Maintain proper Chinese punctuation (use Chinese quotation marks, periods, etc.).
5. Do not add any explanations or notes - output only the translated LaTeX content.
CRITICAL LINE STRUCTURE RULES (MUST FOLLOW):
6. PRESERVE THE EXACT LINE STRUCTURE of the original document:
   - Each \item MUST remain on its own separate line
   - Each \begin{...} MUST remain on its own separate line
   - Each \end{...} MUST remain on its own separate line
   - NEVER merge multiple lines into a single line
   - NEVER put \end{itemize} and \item on the same line
   - NEVER put \end{enumerate} and \item on the same line
   - The output MUST have approximately the same number of lines as the input
7. For list environments (itemize, enumerate, description):
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
	return `You are a precise academic translator. Your ONLY task is to translate English text to Chinese.

## ABSOLUTE RULES (VIOLATION = FAILURE):

### Rule 1: PLACEHOLDER PRESERVATION (MOST CRITICAL)
- Placeholders look like: <<<LATEX_CMD_0>>>, <<<LATEX_CMD_1>>>, etc.
- You MUST copy each placeholder EXACTLY as it appears - character by character
- NEVER modify, translate, explain, or interpret placeholders
- NEVER change the number in a placeholder (e.g., <<<LATEX_CMD_0>>> must stay as <<<LATEX_CMD_0>>>)
- NEVER add spaces inside placeholders
- NEVER split a placeholder across lines
- If input has <<<LATEX_CMD_5>>>, output MUST have <<<LATEX_CMD_5>>> (identical)

### Rule 2: NO HALLUCINATION
- Translate ONLY what is given - do not add information
- Do not add explanations, notes, or comments
- Do not add content that wasn't in the original
- Do not "improve" or "clarify" the text
- If something is unclear, translate it literally

### Rule 3: STRUCTURE PRESERVATION
- Keep the same number of lines as input
- Keep placeholders on their original lines
- Do not merge lines
- Do not split lines
- Preserve paragraph breaks

### Rule 4: OUTPUT FORMAT
- Output ONLY the translated text
- No JSON, no code blocks, no markdown formatting
- No "Translation:" prefix or similar labels
- Start directly with the translated content

## TRANSLATION GUIDELINES:
- Use proper Chinese punctuation: 。，、；：""''（）
- Maintain academic/formal tone
- Preserve technical terms when appropriate`
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


// splitIntoChunks splits content into chunks suitable for translation.
// It tries to split at natural boundaries (paragraphs, sections) to maintain context.
// Each chunk will be at most maxSize characters.
//
// The function uses the following strategy:
// 1. First, try to split by LaTeX section commands (\section, \subsection, etc.)
// 2. If sections are too large, split by paragraphs (double newlines)
// 3. If paragraphs are too large, split by sentences
// 4. As a last resort, split by character count
func splitIntoChunks(content string, maxSize int) []string {
	if len(content) <= maxSize {
		return []string{content}
	}

	// Try to split by sections first
	chunks := splitBySections(content, maxSize)
	if len(chunks) > 1 {
		return chunks
	}

	// Fall back to splitting by paragraphs
	chunks = splitByParagraphs(content, maxSize)
	if len(chunks) > 1 {
		return chunks
	}

	// Fall back to splitting by size with sentence boundaries
	return splitBySize(content, maxSize)
}

// sectionPattern matches LaTeX section commands
var sectionPattern = regexp.MustCompile(`(?m)^\\(section|subsection|subsubsection|chapter|part)\s*[\[{]`)

// splitBySections splits content by LaTeX section commands.
func splitBySections(content string, maxSize int) []string {
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
			result = append(result, splitByParagraphs(chunk, maxSize)...)
		} else {
			result = append(result, chunk)
		}
	}

	return result
}

// splitByParagraphs splits content by paragraph boundaries (double newlines).
func splitByParagraphs(content string, maxSize int) []string {
	// Split by double newlines (paragraph boundaries)
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(content, -1)

	if len(paragraphs) < 2 {
		return splitBySize(content, maxSize)
	}

	var chunks []string
	var currentChunk strings.Builder

	for i, para := range paragraphs {
		// Add paragraph separator back (except for first paragraph)
		if i > 0 {
			para = "\n\n" + para
		}

		// Check if adding this paragraph would exceed maxSize
		if currentChunk.Len()+len(para) > maxSize {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
			}

			// If single paragraph is too large, split it further
			if len(para) > maxSize {
				subChunks := splitBySize(para, maxSize)
				// Add all but the last sub-chunk to results
				for j, subChunk := range subChunks {
					if j < len(subChunks)-1 {
						chunks = append(chunks, subChunk)
					} else {
						currentChunk.WriteString(subChunk)
					}
				}
			} else {
				currentChunk.WriteString(para)
			}
		} else {
			currentChunk.WriteString(para)
		}
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

// splitBySize splits content by size, trying to break at sentence boundaries.
func splitBySize(content string, maxSize int) []string {
	if len(content) <= maxSize {
		return []string{content}
	}

	var chunks []string
	remaining := content

	for len(remaining) > 0 {
		if len(remaining) <= maxSize {
			chunks = append(chunks, remaining)
			break
		}

		// Find a good break point within maxSize
		breakPoint := findBreakPoint(remaining, maxSize)
		chunks = append(chunks, remaining[:breakPoint])
		remaining = remaining[breakPoint:]
	}

	return chunks
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

// ProtectLaTeXCommands replaces LaTeX commands with placeholders for safe translation.
// It returns the modified content and a map of placeholders to original commands.
//
// The placeholders are in the format: <<<LATEX_CMD_N>>> where N is a unique number.
// This format is chosen to be unlikely to appear in normal text and to be preserved
// by translation APIs.
//
// Validates: Requirements 3.2
func ProtectLaTeXCommands(content string) (string, map[string]string) {
	commands := ExtractLaTeXCommands(content)
	placeholders := make(map[string]string)

	if len(commands) == 0 {
		return content, placeholders
	}

	// Process commands in reverse order to maintain correct positions
	result := content
	for i := len(commands) - 1; i >= 0; i-- {
		cmd := commands[i]
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
func FixLaTeXLineStructure(content string) string {
	result := content

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

	logger.Debug("fixed LaTeX line structure", 
		logger.Int("originalLength", len(content)),
		logger.Int("fixedLength", len(result)))

	return result
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
func ApplyReferenceBasedFixes(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated
	anyFixed := false

	// 1. Fix table structures (resizebox closing braces)
	result, tableFixed := fixTableResizeboxBraces(result, original)
	if tableFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed table resizebox braces")
	}

	// 2. Fix caption brace issues
	result, captionFixed := fixCaptionBraceIssues(result, original)
	if captionFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed caption brace issues")
	}

	// 3. Fix trailing garbage braces
	result, trailingFixed := fixTrailingGarbageByReference(result, original)
	if trailingFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed trailing garbage braces")
	}

	// 4. Fix environment structure
	result, envFixed := fixEnvironmentStructureByReference(result, original)
	if envFixed {
		anyFixed = true
		logger.Debug("reference-based fix: fixed environment structure")
	}

	if anyFixed {
		logger.Info("applied reference-based fixes to translated content")
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
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+)\}([=,;:，；：])`), "${1}}}${2}"},
		// \textbf{\textit{X}） -> \textbf{\textit{X}}）
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+)\}([）\)])`), "${1}}}${2}"},
		// Similar for other nested commands
		{regexp.MustCompile(`(\\textit\{\\textbf\{[^}]+)\}([=,;:，；：])`), "${1}}}${2}"},
	}

	result := translated
	fixed := false

	for _, p := range patterns {
		if p.pattern.MatchString(result) {
			result = p.pattern.ReplaceAllString(result, p.replace)
			fixed = true
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


// cleanTranslationResult removes JSON formatting artifacts and other issues from translation results
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
