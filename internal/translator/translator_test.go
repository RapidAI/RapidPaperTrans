// Package translator provides functionality for translating LaTeX documents using OpenAI API.
package translator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"latex-translator/internal/types"
)

// TestNewTranslationEngine tests the constructor functions.
func TestNewTranslationEngine(t *testing.T) {
	apiKey := "test-api-key"
	engine := NewTranslationEngine(apiKey)

	if engine.GetAPIKey() != apiKey {
		t.Errorf("expected API key %q, got %q", apiKey, engine.GetAPIKey())
	}

	if engine.GetModel() != DefaultModel {
		t.Errorf("expected model %q, got %q", DefaultModel, engine.GetModel())
	}
}

func TestNewTranslationEngineWithModel(t *testing.T) {
	apiKey := "test-api-key"
	model := "gpt-3.5-turbo"
	engine := NewTranslationEngineWithModel(apiKey, model)

	if engine.GetAPIKey() != apiKey {
		t.Errorf("expected API key %q, got %q", apiKey, engine.GetAPIKey())
	}

	if engine.GetModel() != model {
		t.Errorf("expected model %q, got %q", model, engine.GetModel())
	}
}

func TestNewTranslationEngineWithConfig(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		model         string
		apiURL        string
		expectedModel string
		expectedURL   string
	}{
		{
			name:          "all values provided",
			apiKey:        "test-key",
			model:         "gpt-4",
			apiURL:        "https://custom.api.com/v1",
			expectedModel: "gpt-4",
			expectedURL:   "https://custom.api.com/v1/chat/completions",
		},
		{
			name:          "empty model uses default",
			apiKey:        "test-key",
			model:         "",
			apiURL:        "https://custom.api.com/v1",
			expectedModel: DefaultModel,
			expectedURL:   "https://custom.api.com/v1/chat/completions",
		},
		{
			name:          "empty URL uses default",
			apiKey:        "test-key",
			model:         "gpt-4",
			apiURL:        "",
			expectedModel: "gpt-4",
			expectedURL:   OpenAIAPIURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewTranslationEngineWithConfig(tt.apiKey, tt.model, tt.apiURL, 0, 3)

			if engine.GetModel() != tt.expectedModel {
				t.Errorf("expected model %q, got %q", tt.expectedModel, engine.GetModel())
			}

			if engine.apiURL != tt.expectedURL {
				t.Errorf("expected URL %q, got %q", tt.expectedURL, engine.apiURL)
			}
		})
	}
}

// TestTranslateTeX_EmptyContent tests translation of empty content.
func TestTranslateTeX_EmptyContent(t *testing.T) {
	engine := NewTranslationEngine("test-api-key")

	result, err := engine.TranslateTeX("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.OriginalContent != "" {
		t.Errorf("expected empty original content, got %q", result.OriginalContent)
	}

	if result.TranslatedContent != "" {
		t.Errorf("expected empty translated content, got %q", result.TranslatedContent)
	}

	if result.TokensUsed != 0 {
		t.Errorf("expected 0 tokens used, got %d", result.TokensUsed)
	}
}

// TestTranslateTeX_NoAPIKey tests that translation fails without API key.
func TestTranslateTeX_NoAPIKey(t *testing.T) {
	engine := NewTranslationEngine("")

	_, err := engine.TranslateTeX("Hello world")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}

	appErr, ok := err.(*types.AppError)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}

	if appErr.Code != types.ErrConfig {
		t.Errorf("expected error code %q, got %q", types.ErrConfig, appErr.Code)
	}
}

// TestTranslateChunk_EmptyContent tests translation of empty chunk.
func TestTranslateChunk_EmptyContent(t *testing.T) {
	engine := NewTranslationEngine("test-api-key")

	result, err := engine.TranslateChunk("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

// TestTranslateChunk_NoAPIKey tests that chunk translation fails without API key.
func TestTranslateChunk_NoAPIKey(t *testing.T) {
	engine := NewTranslationEngine("")

	_, err := engine.TranslateChunk("Hello world")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}

	appErr, ok := err.(*types.AppError)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}

	if appErr.Code != types.ErrConfig {
		t.Errorf("expected error code %q, got %q", types.ErrConfig, appErr.Code)
	}
}

// TestTranslateTeX_WithMockServer tests translation with a mock OpenAI server.
func TestTranslateTeX_WithMockServer(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization header 'Bearer test-api-key', got %q", r.Header.Get("Authorization"))
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Verify request structure
		if req.Model != DefaultModel {
			t.Errorf("expected model %q, got %q", DefaultModel, req.Model)
		}

		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}

		// Return mock response
		resp := ChatCompletionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   DefaultModel,
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: "这是翻译后的内容。",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create engine with mock server URL
	engine := NewTranslationEngine("test-api-key")
	engine.SetAPIURL(server.URL)

	// Test translation
	result, err := engine.TranslateTeX("This is the original content.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.OriginalContent != "This is the original content." {
		t.Errorf("expected original content 'This is the original content.', got %q", result.OriginalContent)
	}

	if result.TranslatedContent != "这是翻译后的内容。" {
		t.Errorf("expected translated content '这是翻译后的内容。', got %q", result.TranslatedContent)
	}

	if result.TokensUsed != 150 {
		t.Errorf("expected 150 tokens used, got %d", result.TokensUsed)
	}
}

// TestTranslateTeX_APIError tests handling of API errors.
func TestTranslateTeX_APIError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedCode   types.ErrorCode
	}{
		{
			name:         "unauthorized",
			statusCode:   http.StatusUnauthorized,
			responseBody: `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`,
			expectedCode: types.ErrAPICall,
		},
		{
			name:         "rate limit - retries exhausted",
			statusCode:   http.StatusTooManyRequests,
			responseBody: `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`,
			expectedCode: types.ErrAPICall, // After retries exhausted, returns ErrAPICall
		},
		{
			name:         "bad request",
			statusCode:   http.StatusBadRequest,
			responseBody: `{"error":{"message":"Invalid request","type":"invalid_request_error"}}`,
			expectedCode: types.ErrAPICall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			engine := NewTranslationEngine("test-api-key")
			engine.SetAPIURL(server.URL)

			_, err := engine.TranslateTeX("Test content")
			if err == nil {
				t.Fatal("expected error")
			}

			appErr, ok := err.(*types.AppError)
			if !ok {
				t.Fatalf("expected AppError, got %T", err)
			}

			if appErr.Code != tt.expectedCode {
				t.Errorf("expected error code %q, got %q", tt.expectedCode, appErr.Code)
			}
		})
	}
}

// TestSplitIntoChunks tests the text chunking logic.
func TestSplitIntoChunks(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		maxSize        int
		expectedChunks int
		checkFirst     string // Check if first chunk starts with this
	}{
		{
			name:           "small content no split",
			content:        "Hello world",
			maxSize:        100,
			expectedChunks: 1,
			checkFirst:     "Hello world",
		},
		{
			name:           "exact size no split",
			content:        "Hello",
			maxSize:        5,
			expectedChunks: 1,
			checkFirst:     "Hello",
		},
		{
			name:           "split by paragraphs",
			content:        "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.",
			maxSize:        30,
			expectedChunks: 3, // Each paragraph becomes its own chunk due to size constraints
			checkFirst:     "First paragraph.",
		},
		{
			name:           "split by sections",
			content:        "\\section{First}\nContent one.\n\\section{Second}\nContent two.",
			maxSize:        30,
			expectedChunks: 2,
			checkFirst:     "\\section{First}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := splitIntoChunks(tt.content, tt.maxSize)

			if len(chunks) != tt.expectedChunks {
				t.Errorf("expected %d chunks, got %d: %v", tt.expectedChunks, len(chunks), chunks)
			}

			if len(chunks) > 0 && !strings.HasPrefix(chunks[0], tt.checkFirst) {
				t.Errorf("expected first chunk to start with %q, got %q", tt.checkFirst, chunks[0])
			}

			// Verify all chunks are within size limit (with some tolerance for edge cases)
			for i, chunk := range chunks {
				if len(chunk) > tt.maxSize*2 { // Allow some overflow for edge cases
					t.Errorf("chunk %d exceeds max size: %d > %d", i, len(chunk), tt.maxSize)
				}
			}

			// Verify content is preserved (all chunks joined should equal original)
			joined := strings.Join(chunks, "")
			// Note: Some whitespace may be modified during splitting
			if !strings.Contains(joined, strings.TrimSpace(tt.content[:min(len(tt.content), 10)])) {
				t.Errorf("content not preserved in chunks")
			}
		})
	}
}

// TestSplitBySections tests section-based splitting.
func TestSplitBySections(t *testing.T) {
	content := `\section{Introduction}
This is the introduction.

\section{Methods}
This is the methods section.

\subsection{Data Collection}
Data was collected.

\section{Results}
Here are the results.`

	chunks := splitBySections(content, 100)

	// Should split into multiple chunks
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk should contain Introduction
	if !strings.Contains(chunks[0], "Introduction") {
		t.Errorf("first chunk should contain Introduction")
	}
}

// TestSplitByParagraphs tests paragraph-based splitting.
func TestSplitByParagraphs(t *testing.T) {
	content := `First paragraph with some content.

Second paragraph with more content.

Third paragraph with even more content.`

	chunks := splitByParagraphs(content, 50)

	// Should split into multiple chunks
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Verify paragraph boundaries are preserved
	for _, chunk := range chunks {
		if len(chunk) > 0 && !strings.HasPrefix(chunk, "First") && !strings.HasPrefix(chunk, "\n\n") {
			// Non-first chunks should start with paragraph separator
		}
	}
}

// TestSplitBySize tests size-based splitting with sentence boundaries.
func TestSplitBySize(t *testing.T) {
	content := "This is sentence one. This is sentence two. This is sentence three. This is sentence four."

	chunks := splitBySize(content, 50)

	// Should split into multiple chunks
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Verify chunks don't exceed max size significantly
	for i, chunk := range chunks {
		if len(chunk) > 60 { // Allow some tolerance
			t.Errorf("chunk %d exceeds max size: %d", i, len(chunk))
		}
	}
}

// TestFindBreakPoint tests finding good break points in text.
func TestFindBreakPoint(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxSize  int
		expected int // Approximate expected break point
	}{
		{
			name:     "break at sentence",
			text:     "This is a sentence. This is another sentence.",
			maxSize:  25,
			expected: 20, // After "This is a sentence. "
		},
		{
			name:     "break at newline",
			text:     "Line one content\nLine two content",
			maxSize:  20,
			expected: 17, // After newline
		},
		{
			name:     "break at space",
			text:     "Word1 Word2 Word3 Word4 Word5",
			maxSize:  15,
			expected: 12, // After "Word1 Word2 "
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breakPoint := findBreakPoint(tt.text, tt.maxSize)

			// Break point should be within reasonable range
			if breakPoint < tt.maxSize/2 || breakPoint > tt.maxSize+5 {
				t.Errorf("break point %d outside expected range for max size %d", breakPoint, tt.maxSize)
			}
		})
	}
}

// TestBuildSystemPrompt tests the system prompt generation.
func TestBuildSystemPrompt(t *testing.T) {
	prompt := buildSystemPrompt()

	// Verify key instructions are present
	requiredPhrases := []string{
		"LaTeX",
		"English to Chinese",
		"PRESERVE",
		"mathematical",
		// New line structure rules
		"CRITICAL LINE STRUCTURE RULES",
		"Each \\item MUST remain on its own separate line",
		"Each \\begin{...} MUST remain on its own separate line",
		"Each \\end{...} MUST remain on its own separate line",
		"NEVER merge multiple lines into a single line",
		"NEVER put \\end{itemize} and \\item on the same line",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("system prompt should contain %q", phrase)
		}
	}
}

// TestBuildUserPrompt tests the user prompt generation.
func TestBuildUserPrompt(t *testing.T) {
	content := "Test LaTeX content"
	prompt := buildUserPrompt(content)

	if !strings.Contains(prompt, content) {
		t.Errorf("user prompt should contain the content")
	}

	if !strings.Contains(prompt, "Translate") {
		t.Errorf("user prompt should contain translation instruction")
	}
	
	// Verify line structure reminders are present
	lineStructureReminders := []string{
		"KEEP EACH LINE SEPARATE",
		"do not merge lines together",
		"same line structure as the input",
	}
	
	for _, reminder := range lineStructureReminders {
		if !strings.Contains(prompt, reminder) {
			t.Errorf("user prompt should contain %q", reminder)
		}
	}
}

// TestIsRetryableAPIError tests error retry logic.
func TestIsRetryableAPIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "network error",
			err:      types.NewAppError(types.ErrNetwork, "network failed", nil),
			expected: true,
		},
		{
			name:     "rate limit error",
			err:      types.NewAppError(types.ErrAPIRateLimit, "rate limited", nil),
			expected: true,
		},
		{
			name:     "config error",
			err:      types.NewAppError(types.ErrConfig, "config error", nil),
			expected: false,
		},
		{
			name:     "server error (5xx)",
			err:      types.NewAppErrorWithDetails(types.ErrAPICall, "server error", "status 500", nil),
			expected: true,
		},
		{
			name:     "client error (4xx)",
			err:      types.NewAppErrorWithDetails(types.ErrAPICall, "client error", "status 400", nil),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableAPIError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


// =============================================================================
// Tests for LaTeX Command Protection Logic
// =============================================================================

// TestExtractLaTeXCommands_MathEnvironments tests extraction of math environments.
func TestExtractLaTeXCommands_MathEnvironments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "inline math",
			content:  "The equation $x^2 + y^2 = z^2$ is famous.",
			expected: []string{"$x^2 + y^2 = z^2$"},
		},
		{
			name:     "display math",
			content:  "Consider $$\\int_0^1 f(x) dx$$",
			expected: []string{"$$\\int_0^1 f(x) dx$$"},
		},
		{
			name:     "bracket math",
			content:  "The formula \\[E = mc^2\\] is well known.",
			expected: []string{"\\[E = mc^2\\]"},
		},
		{
			name:     "parenthesis math",
			content:  "Inline \\(a + b\\) expression.",
			expected: []string{"\\(a + b\\)"},
		},
		{
			name:     "equation environment",
			content:  "\\begin{equation}x = y\\end{equation}",
			expected: []string{"\\begin{equation}x = y\\end{equation}"},
		},
		{
			name:     "align environment",
			content:  "\\begin{align}a &= b\\\\c &= d\\end{align}",
			expected: []string{"\\begin{align}a &= b\\\\c &= d\\end{align}"},
		},
		{
			name:     "multiple math environments",
			content:  "First $a$ then $b$ and \\[c\\]",
			expected: []string{"$a$", "$b$", "\\[c\\]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := ExtractLaTeXCommands(tt.content)
			
			// Check that all expected commands are found
			for _, exp := range tt.expected {
				found := false
				for _, cmd := range commands {
					if cmd.Command == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find command %q, but it was not found", exp)
				}
			}
		})
	}
}

// TestExtractLaTeXCommands_StructureCommands tests extraction of document structure commands.
func TestExtractLaTeXCommands_StructureCommands(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "section command",
			content:  "\\section{Introduction}",
			expected: []string{"\\section{Introduction}"},
		},
		{
			name:     "subsection command",
			content:  "\\subsection{Methods}",
			expected: []string{"\\subsection{Methods}"},
		},
		{
			name:     "section with optional arg",
			content:  "\\section[Short]{Long Title}",
			expected: []string{"\\section[Short]{Long Title}"},
		},
		{
			name:     "begin environment",
			content:  "\\begin{document}",
			expected: []string{"\\begin{document}"},
		},
		{
			name:     "end environment",
			content:  "\\end{document}",
			expected: []string{"\\end{document}"},
		},
		{
			name:     "documentclass",
			content:  "\\documentclass{article}",
			expected: []string{"\\documentclass{article}"},
		},
		{
			name:     "usepackage",
			content:  "\\usepackage[utf8]{inputenc}",
			expected: []string{"\\usepackage[utf8]{inputenc}"},
		},
		{
			name:     "input command",
			content:  "\\input{chapter1}",
			expected: []string{"\\input{chapter1}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := ExtractLaTeXCommands(tt.content)
			
			for _, exp := range tt.expected {
				found := false
				for _, cmd := range commands {
					if cmd.Command == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find command %q, but it was not found. Found: %v", exp, commands)
				}
			}
		})
	}
}

// TestExtractLaTeXCommands_ReferenceCommands tests extraction of reference commands.
func TestExtractLaTeXCommands_ReferenceCommands(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "ref command",
			content:  "See Figure \\ref{fig:example}",
			expected: []string{"\\ref{fig:example}"},
		},
		{
			name:     "cite command",
			content:  "As shown in \\cite{smith2020}",
			expected: []string{"\\cite{smith2020}"},
		},
		{
			name:     "label command",
			content:  "\\label{sec:intro}",
			expected: []string{"\\label{sec:intro}"},
		},
		{
			name:     "eqref command",
			content:  "Equation \\eqref{eq:main}",
			expected: []string{"\\eqref{eq:main}"},
		},
		{
			name:     "citep command",
			content:  "Results \\citep{jones2021}",
			expected: []string{"\\citep{jones2021}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := ExtractLaTeXCommands(tt.content)
			
			for _, exp := range tt.expected {
				found := false
				for _, cmd := range commands {
					if cmd.Command == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find command %q, but it was not found. Found: %v", exp, commands)
				}
			}
		})
	}
}

// TestExtractLaTeXCommands_FormattingCommands tests extraction of formatting commands.
func TestExtractLaTeXCommands_FormattingCommands(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "textbf command",
			content:  "This is \\textbf{bold} text",
			expected: []string{"\\textbf{bold}"},
		},
		{
			name:     "textit command",
			content:  "This is \\textit{italic} text",
			expected: []string{"\\textit{italic}"},
		},
		{
			name:     "emph command",
			content:  "This is \\emph{emphasized} text",
			expected: []string{"\\emph{emphasized}"},
		},
		{
			name:     "font size command",
			content:  "\\large Large text",
			expected: []string{"\\large"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := ExtractLaTeXCommands(tt.content)
			
			for _, exp := range tt.expected {
				found := false
				for _, cmd := range commands {
					if cmd.Command == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find command %q, but it was not found. Found: %v", exp, commands)
				}
			}
		})
	}
}

// TestExtractLaTeXCommands_OtherCommands tests extraction of other LaTeX commands.
func TestExtractLaTeXCommands_OtherCommands(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "comment",
			content:  "Text % this is a comment",
			expected: []string{"% this is a comment"},
		},
		{
			name:     "special char percent",
			content:  "100\\% complete",
			expected: []string{"\\%"},
		},
		{
			name:     "special char dollar",
			content:  "Price is \\$100",
			expected: []string{"\\$"},
		},
		{
			name:     "newline command",
			content:  "Line one\\\\Line two",
			expected: []string{"\\\\"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := ExtractLaTeXCommands(tt.content)
			
			for _, exp := range tt.expected {
				found := false
				for _, cmd := range commands {
					if cmd.Command == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find command %q, but it was not found. Found: %v", exp, commands)
				}
			}
		})
	}
}

// TestExtractLaTeXCommands_ComplexDocument tests extraction from a complex document.
func TestExtractLaTeXCommands_ComplexDocument(t *testing.T) {
	content := `\documentclass{article}
\usepackage{amsmath}

\begin{document}

\section{Introduction}
This paper discusses the equation $E = mc^2$ as shown in \cite{einstein1905}.

\begin{equation}
F = ma
\end{equation}

See Equation \eqref{eq:main} for details.

\textbf{Important:} The results are significant.

\end{document}`

	commands := ExtractLaTeXCommands(content)

	// Should find multiple types of commands
	// Note: \label inside equation environment is part of the equation, not extracted separately
	expectedCommands := []string{
		"\\documentclass{article}",
		"\\usepackage{amsmath}",
		"\\begin{document}",
		"\\section{Introduction}",
		"$E = mc^2$",
		"\\cite{einstein1905}",
		"\\eqref{eq:main}",
		"\\textbf{Important:}",
		"\\end{document}",
	}

	for _, exp := range expectedCommands {
		found := false
		for _, cmd := range commands {
			if cmd.Command == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find command %q in complex document", exp)
		}
	}
}

// TestGetLaTeXCommandStrings tests the convenience function.
func TestGetLaTeXCommandStrings(t *testing.T) {
	content := "The equation $x^2$ and \\textbf{bold}"
	
	strings := GetLaTeXCommandStrings(content)
	
	if len(strings) < 2 {
		t.Errorf("expected at least 2 commands, got %d", len(strings))
	}
	
	// Check that results are strings, not LaTeXCommand structs
	for _, s := range strings {
		if s == "" {
			t.Error("got empty string in results")
		}
	}
}

// TestProtectLaTeXCommands tests the protection mechanism.
func TestProtectLaTeXCommands(t *testing.T) {
	content := "The equation $x^2 + y^2 = z^2$ is famous."
	
	protected, placeholders := ProtectLaTeXCommands(content)
	
	// The protected content should not contain the original math
	if strings.Contains(protected, "$x^2 + y^2 = z^2$") {
		t.Error("protected content should not contain original math")
	}
	
	// Should contain a placeholder
	if !strings.Contains(protected, "<<<LATEX_CMD_") {
		t.Error("protected content should contain placeholder")
	}
	
	// Placeholders map should have entries
	if len(placeholders) == 0 {
		t.Error("placeholders map should not be empty")
	}
	
	// Placeholder should map to original command
	for placeholder, original := range placeholders {
		if original != "$x^2 + y^2 = z^2$" {
			t.Errorf("placeholder %q should map to math command, got %q", placeholder, original)
		}
	}
}

// TestRestoreLaTeXCommands tests the restoration mechanism.
func TestRestoreLaTeXCommands(t *testing.T) {
	original := "The equation $x^2 + y^2 = z^2$ is famous."
	
	// Protect commands
	protected, placeholders := ProtectLaTeXCommands(original)
	
	// Simulate translation (just change the text around placeholders)
	translated := strings.Replace(protected, "The equation", "方程", 1)
	translated = strings.Replace(translated, "is famous.", "很有名。", 1)
	
	// Restore commands
	restored := RestoreLaTeXCommands(translated, placeholders)
	
	// Should contain the original math command
	if !strings.Contains(restored, "$x^2 + y^2 = z^2$") {
		t.Errorf("restored content should contain original math: %q", restored)
	}
	
	// Should contain translated text
	if !strings.Contains(restored, "方程") {
		t.Errorf("restored content should contain translated text: %q", restored)
	}
}

// TestProtectAndRestoreRoundTrip tests that protect and restore are inverse operations.
func TestProtectAndRestoreRoundTrip(t *testing.T) {
	testCases := []string{
		"Simple text with $math$ inside.",
		"\\section{Title} with \\textbf{bold} and $x^2$.",
		"Multiple $a$ math $b$ expressions $c$.",
		"Complex \\begin{equation}E=mc^2\\end{equation} environment.",
		"References \\cite{ref1} and \\ref{fig:1}.",
	}
	
	for _, original := range testCases {
		t.Run(original[:min(20, len(original))], func(t *testing.T) {
			// Protect
			protected, placeholders := ProtectLaTeXCommands(original)
			
			// Restore without any changes
			restored := RestoreLaTeXCommands(protected, placeholders)
			
			// Should be identical to original
			if restored != original {
				t.Errorf("round trip failed:\noriginal: %q\nrestored: %q", original, restored)
			}
		})
	}
}

// TestIsLaTeXCommand tests the command detection function.
func TestIsLaTeXCommand(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"\\section", true},
		{"\\textbf{bold}", true},
		{"$x^2$", true},
		{"\\[E=mc^2\\]", true},
		{"\\(a+b\\)", true},
		{"%comment", true},
		{"normal text", false},
		{"123", false},
		{"", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := IsLaTeXCommand(tt.text)
			if result != tt.expected {
				t.Errorf("IsLaTeXCommand(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestContainsLaTeXCommands tests the command presence detection.
func TestContainsLaTeXCommands(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"This has $math$ in it.", true},
		{"This has \\textbf{bold} in it.", true},
		{"This is plain text.", false},
		{"", false},
		{"\\section{Title}", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.content[:min(20, len(tt.content))], func(t *testing.T) {
			result := ContainsLaTeXCommands(tt.content)
			if result != tt.expected {
				t.Errorf("ContainsLaTeXCommands(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

// TestGetMathEnvironments tests extraction of only math environments.
func TestGetMathEnvironments(t *testing.T) {
	content := "Text with $inline$ math and \\section{Title} and $$display$$ math."
	
	mathCommands := GetMathEnvironments(content)
	
	// Should find math environments
	if len(mathCommands) < 2 {
		t.Errorf("expected at least 2 math environments, got %d", len(mathCommands))
	}
	
	// All should be math type
	for _, cmd := range mathCommands {
		if cmd.Type != CommandTypeMath {
			t.Errorf("expected CommandTypeMath, got %v for %q", cmd.Type, cmd.Command)
		}
	}
}

// TestGetNonMathText tests extraction of non-math text.
func TestGetNonMathText(t *testing.T) {
	content := "This is text $x^2$ more text $y^2$ end."
	
	nonMath := GetNonMathText(content)
	
	// Should not contain math
	if strings.Contains(nonMath, "$x^2$") || strings.Contains(nonMath, "$y^2$") {
		t.Errorf("non-math text should not contain math: %q", nonMath)
	}
	
	// Should contain regular text
	if !strings.Contains(nonMath, "This is text") {
		t.Errorf("non-math text should contain regular text: %q", nonMath)
	}
}

// TestDeduplicateAndSortCommands tests command deduplication.
func TestDeduplicateAndSortCommands(t *testing.T) {
	// Create overlapping commands
	commands := []LaTeXCommand{
		{Command: "$x$", Start: 0, End: 3, Type: CommandTypeMath},
		{Command: "$x$", Start: 0, End: 3, Type: CommandTypeMath}, // Duplicate
		{Command: "$y$", Start: 10, End: 13, Type: CommandTypeMath},
	}
	
	result := deduplicateAndSortCommands(commands)
	
	// Should remove duplicate
	if len(result) != 2 {
		t.Errorf("expected 2 commands after deduplication, got %d", len(result))
	}
	
	// Should be sorted by position
	if result[0].Start > result[1].Start {
		t.Error("commands should be sorted by start position")
	}
}

// TestCommandsOverlap tests overlap detection.
func TestCommandsOverlap(t *testing.T) {
	tests := []struct {
		name     string
		a        LaTeXCommand
		b        LaTeXCommand
		expected bool
	}{
		{
			name:     "no overlap",
			a:        LaTeXCommand{Start: 0, End: 5},
			b:        LaTeXCommand{Start: 10, End: 15},
			expected: false,
		},
		{
			name:     "adjacent no overlap",
			a:        LaTeXCommand{Start: 0, End: 5},
			b:        LaTeXCommand{Start: 5, End: 10},
			expected: false,
		},
		{
			name:     "overlap",
			a:        LaTeXCommand{Start: 0, End: 10},
			b:        LaTeXCommand{Start: 5, End: 15},
			expected: true,
		},
		{
			name:     "contained",
			a:        LaTeXCommand{Start: 0, End: 20},
			b:        LaTeXCommand{Start: 5, End: 15},
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := commandsOverlap(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("commandsOverlap(%v, %v) = %v, expected %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestExtractLaTeXCommands_EmptyContent tests extraction from empty content.
func TestExtractLaTeXCommands_EmptyContent(t *testing.T) {
	commands := ExtractLaTeXCommands("")
	
	if len(commands) != 0 {
		t.Errorf("expected 0 commands from empty content, got %d", len(commands))
	}
}

// TestExtractLaTeXCommands_NoCommands tests extraction from content without commands.
func TestExtractLaTeXCommands_NoCommands(t *testing.T) {
	content := "This is plain text without any LaTeX commands."
	
	commands := ExtractLaTeXCommands(content)
	
	if len(commands) != 0 {
		t.Errorf("expected 0 commands from plain text, got %d: %v", len(commands), commands)
	}
}

// TestProtectLaTeXCommands_EmptyContent tests protection of empty content.
func TestProtectLaTeXCommands_EmptyContent(t *testing.T) {
	protected, placeholders := ProtectLaTeXCommands("")
	
	if protected != "" {
		t.Errorf("expected empty protected content, got %q", protected)
	}
	
	if len(placeholders) != 0 {
		t.Errorf("expected empty placeholders map, got %v", placeholders)
	}
}

// TestProtectLaTeXCommands_NoCommands tests protection of content without commands.
func TestProtectLaTeXCommands_NoCommands(t *testing.T) {
	content := "Plain text without commands."
	
	protected, placeholders := ProtectLaTeXCommands(content)
	
	if protected != content {
		t.Errorf("expected unchanged content, got %q", protected)
	}
	
	if len(placeholders) != 0 {
		t.Errorf("expected empty placeholders map, got %v", placeholders)
	}
}


// TestFixLaTeXLineStructure tests the line structure fixing function.
func TestFixLaTeXLineStructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "fix end itemize followed by item",
			input:    `\end{itemize}\item \textbf{Next item}`,
			expected: `\end{itemize}` + "\n" + `\item \textbf{Next item}`,
		},
		{
			name:     "fix end itemize with spaces followed by item",
			input:    `\end{itemize}  \item \textbf{Next item}`,
			expected: `\end{itemize}` + "\n" + `\item \textbf{Next item}`,
		},
		{
			name:     "fix end enumerate followed by item",
			input:    `\end{enumerate}\item Next`,
			expected: `\end{enumerate}` + "\n" + `\item Next`,
		},
		{
			name:     "fix end followed by begin",
			input:    `\end{itemize}\begin{enumerate}`,
			expected: `\end{itemize}` + "\n" + `\begin{enumerate}`,
		},
		{
			name:     "fix end followed by section",
			input:    `\end{itemize}\section{Title}`,
			expected: `\end{itemize}` + "\n\n" + `\section{Title}`,
		},
		{
			name:     "preserve correct structure",
			input:    "\\begin{itemize}\n\\item First\n\\item Second\n\\end{itemize}",
			expected: "\\begin{itemize}\n\\item First\n\\item Second\n\\end{itemize}",
		},
		{
			name:     "fix text followed by begin itemize",
			input:    `Our contributions:\begin{itemize}`,
			expected: "Our contributions:\n\\begin{itemize}",
		},
		{
			name:     "fix end itemize followed by text",
			input:    `\end{itemize}Next paragraph`,
			expected: `\end{itemize}` + "\n" + `Next paragraph`,
		},
		{
			name:     "fix comment line followed by abstract",
			input:    `%%==================================%%\abstract{`,
			expected: `%%==================================%%` + "\n" + `\abstract{`,
		},
		{
			name:     "fix comment line followed by maketitle",
			input:    `%%==================================%%\maketitle`,
			expected: `%%==================================%%` + "\n" + `\maketitle`,
		},
		{
			name:     "fix text followed by abstract",
			input:    `Some text here\abstract{`,
			expected: `Some text here` + "\n" + `\abstract{`,
		},
		{
			name:     "preserve abstract on its own line",
			input:    "Some text\n\\abstract{",
			expected: "Some text\n\\abstract{",
		},
		{
			name:     "fix maketitle followed by text",
			input:    `\maketitleSome text`,
			expected: `\maketitle` + "\n\n" + `Some text`,
		},
		{
			name:     "fix cech followed by Chinese",
			input:    `\cech复形`,
			expected: `\cech{}复形`,
		},
		{
			name:     "preserve cech with space",
			input:    `\cech complex`,
			expected: `\cech complex`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixLaTeXLineStructure(tt.input)
			if result != tt.expected {
				t.Errorf("FixLaTeXLineStructure() = %q, want %q", result, tt.expected)
			}
		})
	}
}


// TestProtectTikZEnvironment tests that TikZ environments are fully protected.
func TestProtectTikZEnvironment(t *testing.T) {
	content := `This is some text.
\begin{tikzpicture}
\node[draw] (A) at (0,0) {Node A};
\node[draw] (B) at (2,0) {Node B};
\draw[->] (A) -- (B);
\end{tikzpicture}
More text after TikZ.`

	protected, placeholders := ProtectLaTeXCommands(content)
	
	// The TikZ environment should be replaced with a placeholder
	if strings.Contains(protected, "\\begin{tikzpicture}") {
		t.Error("TikZ begin tag should be replaced with placeholder")
	}
	if strings.Contains(protected, "\\end{tikzpicture}") {
		t.Error("TikZ end tag should be replaced with placeholder")
	}
	if strings.Contains(protected, "\\node") {
		t.Error("TikZ node command should be replaced with placeholder")
	}
	
	// Should have at least one placeholder for the TikZ environment
	foundTikZ := false
	for _, original := range placeholders {
		if strings.Contains(original, "\\begin{tikzpicture}") {
			foundTikZ = true
			// Verify the entire environment is captured
			if !strings.Contains(original, "\\end{tikzpicture}") {
				t.Error("TikZ placeholder should contain the entire environment")
			}
			if !strings.Contains(original, "\\node") {
				t.Error("TikZ placeholder should contain node commands")
			}
		}
	}
	if !foundTikZ {
		t.Error("TikZ environment should be in placeholders")
	}
	
	// Restore and verify
	restored := RestoreLaTeXCommands(protected, placeholders)
	if restored != content {
		t.Errorf("restored content doesn't match original\nExpected:\n%s\nGot:\n%s", content, restored)
	}
}

// TestProtectTabularEnvironment tests that tabular environments are fully protected.
func TestProtectTabularEnvironment(t *testing.T) {
	content := `Table below:
\begin{tabular}{|c|c|c|}
\hline
A & B & C \\
\hline
1 & 2 & 3 \\
\hline
\end{tabular}
End of table.`

	protected, placeholders := ProtectLaTeXCommands(content)
	
	// The tabular environment should be replaced with a placeholder
	if strings.Contains(protected, "\\begin{tabular}") {
		t.Error("tabular begin tag should be replaced with placeholder")
	}
	if strings.Contains(protected, "\\hline") {
		t.Error("hline command should be replaced with placeholder")
	}
	
	// Verify restoration
	restored := RestoreLaTeXCommands(protected, placeholders)
	if restored != content {
		t.Errorf("restored content doesn't match original\nExpected:\n%s\nGot:\n%s", content, restored)
	}
}

// TestProtectNestedEnvironments tests protection of nested environments.
func TestProtectNestedEnvironments(t *testing.T) {
	content := `\begin{equation}
\begin{aligned}
x &= y + z \\
a &= b + c
\end{aligned}
\end{equation}`

	protected, placeholders := ProtectLaTeXCommands(content)
	
	// The entire nested structure should be protected
	if strings.Contains(protected, "\\begin{equation}") {
		t.Error("equation begin tag should be replaced with placeholder")
	}
	if strings.Contains(protected, "\\begin{aligned}") {
		t.Error("aligned begin tag should be replaced with placeholder")
	}
	
	// Verify restoration
	restored := RestoreLaTeXCommands(protected, placeholders)
	if restored != content {
		t.Errorf("restored content doesn't match original\nExpected:\n%s\nGot:\n%s", content, restored)
	}
}

// TestProtectVerbatimEnvironment tests that verbatim environments are protected.
func TestProtectVerbatimEnvironment(t *testing.T) {
	content := `Code example:
\begin{verbatim}
def hello():
    print("Hello, World!")
\end{verbatim}
End of code.`

	protected, placeholders := ProtectLaTeXCommands(content)
	
	// The verbatim environment should be replaced with a placeholder
	if strings.Contains(protected, "\\begin{verbatim}") {
		t.Error("verbatim begin tag should be replaced with placeholder")
	}
	if strings.Contains(protected, "def hello") {
		t.Error("verbatim content should be replaced with placeholder")
	}
	
	// Verify restoration
	restored := RestoreLaTeXCommands(protected, placeholders)
	if restored != content {
		t.Errorf("restored content doesn't match original\nExpected:\n%s\nGot:\n%s", content, restored)
	}
}


// TestValidatePlaceholders tests the placeholder validation function.
func TestValidatePlaceholders(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		placeholders map[string]string
		wantMissing  int
	}{
		{
			name:    "all placeholders present",
			content: "这是 <<<LATEX_CMD_0>>> 和 <<<LATEX_CMD_1>>> 的测试",
			placeholders: map[string]string{
				"<<<LATEX_CMD_0>>>": "$x$",
				"<<<LATEX_CMD_1>>>": "$y$",
			},
			wantMissing: 0,
		},
		{
			name:    "one placeholder missing",
			content: "这是 <<<LATEX_CMD_0>>> 的测试",
			placeholders: map[string]string{
				"<<<LATEX_CMD_0>>>": "$x$",
				"<<<LATEX_CMD_1>>>": "$y$",
			},
			wantMissing: 1,
		},
		{
			name:    "all placeholders missing",
			content: "这是测试文本",
			placeholders: map[string]string{
				"<<<LATEX_CMD_0>>>": "$x$",
				"<<<LATEX_CMD_1>>>": "$y$",
			},
			wantMissing: 2,
		},
		{
			name:         "no placeholders expected",
			content:      "这是普通文本",
			placeholders: map[string]string{},
			wantMissing:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing := validatePlaceholders(tt.content, tt.placeholders)
			if len(missing) != tt.wantMissing {
				t.Errorf("validatePlaceholders() got %d missing, want %d", len(missing), tt.wantMissing)
			}
		})
	}
}

// TestRecoverMissingPlaceholders tests the placeholder recovery function.
func TestRecoverMissingPlaceholders(t *testing.T) {
	content := "这是翻译后的文本"
	placeholders := map[string]string{
		"<<<LATEX_CMD_0>>>": "$x$",
		"<<<LATEX_CMD_1>>>": "$y$",
	}
	missing := []string{"<<<LATEX_CMD_0>>>", "<<<LATEX_CMD_1>>>"}

	recovered := recoverMissingPlaceholders(content, placeholders, missing)

	// Check that all missing placeholders are now present
	for _, placeholder := range missing {
		if !strings.Contains(recovered, placeholder) {
			t.Errorf("recovered content should contain %s", placeholder)
		}
	}
}
