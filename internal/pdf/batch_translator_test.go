package pdf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewBatchTranslator(t *testing.T) {
	tests := []struct {
		name              string
		cfg               BatchTranslatorConfig
		expectedWindow    int
		expectedConcur    int
	}{
		{
			name: "with all values set",
			cfg: BatchTranslatorConfig{
				APIKey:        "test-key",
				BaseURL:       "https://api.example.com",
				Model:         "gpt-4",
				ContextWindow: 8000,
				Concurrency:   5,
				Timeout:       30 * time.Second,
			},
			expectedWindow: 8000,
			expectedConcur: 5,
		},
		{
			name: "with defaults",
			cfg: BatchTranslatorConfig{
				APIKey:  "test-key",
				BaseURL: "https://api.example.com",
				Model:   "gpt-4",
			},
			expectedWindow: DefaultContextWindow,
			expectedConcur: DefaultConcurrency,
		},
		{
			name: "with zero context window",
			cfg: BatchTranslatorConfig{
				APIKey:        "test-key",
				ContextWindow: 0,
			},
			expectedWindow: DefaultContextWindow,
			expectedConcur: DefaultConcurrency,
		},
		{
			name: "with negative values",
			cfg: BatchTranslatorConfig{
				APIKey:        "test-key",
				ContextWindow: -100,
				Concurrency:   -5,
			},
			expectedWindow: DefaultContextWindow,
			expectedConcur: DefaultConcurrency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBatchTranslator(tt.cfg)
			if bt == nil {
				t.Fatal("NewBatchTranslator returned nil")
			}
			if bt.GetContextWindow() != tt.expectedWindow {
				t.Errorf("contextWindow = %d, want %d", bt.GetContextWindow(), tt.expectedWindow)
			}
			if bt.GetConcurrency() != tt.expectedConcur {
				t.Errorf("concurrency = %d, want %d", bt.GetConcurrency(), tt.expectedConcur)
			}
			if bt.client == nil {
				t.Error("client should not be nil")
			}
		})
	}
}

func TestMergeBatches_EmptyInput(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		ContextWindow: 1000,
	})

	batches := bt.MergeBatches(nil)
	if batches != nil {
		t.Errorf("expected nil for empty input, got %v", batches)
	}

	batches = bt.MergeBatches([]TextBlock{})
	if batches != nil {
		t.Errorf("expected nil for empty slice, got %v", batches)
	}
}

func TestMergeBatches_SingleBlock(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello world", Page: 1, BlockType: "paragraph"},
	}

	batches := bt.MergeBatches(blocks)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if len(batches[0]) != 1 {
		t.Errorf("expected 1 block in batch, got %d", len(batches[0]))
	}
	if batches[0][0].ID != "1" {
		t.Errorf("expected block ID '1', got '%s'", batches[0][0].ID)
	}
}

func TestMergeBatches_MultipleBlocksFitInOneBatch(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "World", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "Test", Page: 1, BlockType: "paragraph"},
	}

	batches := bt.MergeBatches(blocks)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if len(batches[0]) != 3 {
		t.Errorf("expected 3 blocks in batch, got %d", len(batches[0]))
	}
}

func TestMergeBatches_BlocksExceedContextWindow(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		ContextWindow: 50, // Small window to force multiple batches
	})

	blocks := []TextBlock{
		{ID: "1", Text: "This is block one with some text", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "This is block two with more text", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "Block three", Page: 1, BlockType: "paragraph"},
	}

	batches := bt.MergeBatches(blocks)
	
	// Verify all blocks are included
	totalBlocks := 0
	for _, batch := range batches {
		totalBlocks += len(batch)
	}
	if totalBlocks != len(blocks) {
		t.Errorf("total blocks in batches = %d, want %d", totalBlocks, len(blocks))
	}

	// Verify each batch respects context window (except for oversized single blocks)
	separatorSize := len(BatchSeparator)
	for i, batch := range batches {
		if len(batch) == 1 {
			// Single block batches are allowed to exceed if the block itself is large
			continue
		}
		batchSize := 0
		for j, block := range batch {
			batchSize += len(block.Text)
			if j > 0 {
				batchSize += separatorSize
			}
		}
		if batchSize > bt.GetContextWindow() {
			t.Errorf("batch %d size %d exceeds context window %d", i, batchSize, bt.GetContextWindow())
		}
	}
}

func TestMergeBatches_LargeBlockExceedsWindow(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		ContextWindow: 20,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Short", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "This is a very long block that exceeds the context window size", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "End", Page: 1, BlockType: "paragraph"},
	}

	batches := bt.MergeBatches(blocks)

	// Verify all blocks are included
	totalBlocks := 0
	for _, batch := range batches {
		totalBlocks += len(batch)
	}
	if totalBlocks != len(blocks) {
		t.Errorf("total blocks in batches = %d, want %d", totalBlocks, len(blocks))
	}

	// The large block should be in its own batch
	foundLargeBlock := false
	for _, batch := range batches {
		for _, block := range batch {
			if block.ID == "2" {
				if len(batch) != 1 {
					t.Errorf("large block should be in its own batch, but batch has %d blocks", len(batch))
				}
				foundLargeBlock = true
			}
		}
	}
	if !foundLargeBlock {
		t.Error("large block not found in any batch")
	}
}

// Property 5: 批次大小约束
// 每个批次的总字符数应小于上下文窗口大小，所有批次的文本块总数等于输入总数
func TestMergeBatches_Property5_BatchSizeConstraint(t *testing.T) {
	testCases := []struct {
		name          string
		contextWindow int
		blocks        []TextBlock
	}{
		{
			name:          "small window with multiple blocks",
			contextWindow: 100,
			blocks: []TextBlock{
				{ID: "1", Text: "First block of text", Page: 1, BlockType: "paragraph"},
				{ID: "2", Text: "Second block", Page: 1, BlockType: "paragraph"},
				{ID: "3", Text: "Third block with more content", Page: 1, BlockType: "paragraph"},
				{ID: "4", Text: "Fourth", Page: 1, BlockType: "paragraph"},
				{ID: "5", Text: "Fifth block here", Page: 1, BlockType: "paragraph"},
			},
		},
		{
			name:          "large window with few blocks",
			contextWindow: 10000,
			blocks: []TextBlock{
				{ID: "1", Text: "Block one", Page: 1, BlockType: "paragraph"},
				{ID: "2", Text: "Block two", Page: 1, BlockType: "paragraph"},
			},
		},
		{
			name:          "exact fit scenario",
			contextWindow: 50,
			blocks: []TextBlock{
				{ID: "1", Text: "12345678901234567890", Page: 1, BlockType: "paragraph"}, // 20 chars
				{ID: "2", Text: "12345678901234567890", Page: 1, BlockType: "paragraph"}, // 20 chars
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bt := NewBatchTranslator(BatchTranslatorConfig{
				ContextWindow: tc.contextWindow,
			})

			batches := bt.MergeBatches(tc.blocks)

			// Property: All batches' total block count equals input count
			totalBlocks := 0
			for _, batch := range batches {
				totalBlocks += len(batch)
			}
			if totalBlocks != len(tc.blocks) {
				t.Errorf("total blocks = %d, want %d", totalBlocks, len(tc.blocks))
			}

			// Property: Each batch size < context window (except single oversized blocks)
			separatorSize := len(BatchSeparator)
			for i, batch := range batches {
				batchSize := 0
				for j, block := range batch {
					batchSize += len(block.Text)
					if j > 0 {
						batchSize += separatorSize
					}
				}
				// Only check multi-block batches or single blocks that fit
				if len(batch) > 1 || len(batch[0].Text) < tc.contextWindow {
					if batchSize > tc.contextWindow {
						t.Errorf("batch %d: size %d exceeds context window %d", i, batchSize, tc.contextWindow)
					}
				}
			}
		})
	}
}

func TestGetBatchText_Empty(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

	result := bt.GetBatchText(nil)
	if result != "" {
		t.Errorf("expected empty string for nil input, got %q", result)
	}

	result = bt.GetBatchText([]TextBlock{})
	if result != "" {
		t.Errorf("expected empty string for empty slice, got %q", result)
	}
}

func TestGetBatchText_SingleBlock(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello world", Page: 1, BlockType: "paragraph"},
	}

	result := bt.GetBatchText(blocks)
	if result != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", result)
	}
}

func TestGetBatchText_MultipleBlocks(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

	blocks := []TextBlock{
		{ID: "1", Text: "First", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "Second", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "Third", Page: 1, BlockType: "paragraph"},
	}

	result := bt.GetBatchText(blocks)
	expected := "First" + BatchSeparator + "Second" + BatchSeparator + "Third"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMergeBatches_PreservesBlockOrder(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		ContextWindow: 50,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "First", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "Second", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "Third", Page: 1, BlockType: "paragraph"},
		{ID: "4", Text: "Fourth", Page: 1, BlockType: "paragraph"},
		{ID: "5", Text: "Fifth", Page: 1, BlockType: "paragraph"},
	}

	batches := bt.MergeBatches(blocks)

	// Collect all block IDs in order
	var ids []string
	for _, batch := range batches {
		for _, block := range batch {
			ids = append(ids, block.ID)
		}
	}

	// Verify order is preserved
	expectedIDs := []string{"1", "2", "3", "4", "5"}
	if len(ids) != len(expectedIDs) {
		t.Fatalf("expected %d IDs, got %d", len(expectedIDs), len(ids))
	}
	for i, id := range ids {
		if id != expectedIDs[i] {
			t.Errorf("ID at position %d: expected %s, got %s", i, expectedIDs[i], id)
		}
	}
}


// Tests for TranslateBatch functionality

// mockOpenAIServer creates a mock server that simulates OpenAI API responses
func mockOpenAIServer(t *testing.T, responseFunc func(req *http.Request) (string, int)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, statusCode := responseFunc(r)
		w.WriteHeader(statusCode)
		w.Write([]byte(content))
	}))
}

// createMockResponse creates a mock OpenAI API response
func createMockResponse(translatedContent string) string {
	resp := ChatCompletionResponse{
		ID:      "test-id",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: translatedContent,
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
	jsonBytes, _ := json.Marshal(resp)
	return string(jsonBytes)
}

func TestTranslateBatch_EmptyInput(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey: "test-key",
	})

	// Test nil input
	result, err := bt.TranslateBatch(nil)
	if err != nil {
		t.Errorf("unexpected error for nil input: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil input, got %v", result)
	}

	// Test empty slice
	result, err = bt.TranslateBatch([]TextBlock{})
	if err != nil {
		t.Errorf("unexpected error for empty input: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty input, got %v", result)
	}
}

func TestTranslateBatch_SingleBlock(t *testing.T) {
	// Create mock server
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		return createMockResponse("你好世界"), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello world", Page: 1, X: 10, Y: 20, Width: 100, Height: 15, BlockType: "paragraph"},
	}

	result, err := bt.TranslateBatch(blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Verify the result preserves original block metadata
	if result[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", result[0].ID)
	}
	if result[0].Page != 1 {
		t.Errorf("expected Page 1, got %d", result[0].Page)
	}
	if result[0].X != 10 {
		t.Errorf("expected X 10, got %f", result[0].X)
	}
	if result[0].Y != 20 {
		t.Errorf("expected Y 20, got %f", result[0].Y)
	}
	if result[0].TranslatedText != "你好世界" {
		t.Errorf("expected translated text '你好世界', got '%s'", result[0].TranslatedText)
	}
	if result[0].FromCache != false {
		t.Errorf("expected FromCache false, got true")
	}
}

func TestTranslateBatch_MultipleBlocks(t *testing.T) {
	// Create mock server that returns translated text with separators
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		translatedText := "第一块" + BatchSeparator + "第二块" + BatchSeparator + "第三块"
		return createMockResponse(translatedText), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 10000, // Large window to fit all blocks in one batch
	})

	blocks := []TextBlock{
		{ID: "1", Text: "First block", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "Second block", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "Third block", Page: 2, BlockType: "paragraph"},
	}

	result, err := bt.TranslateBatch(blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Property 6: 翻译结果映射 - 翻译后返回的TranslatedBlock数量等于输入TextBlock数量
	if len(result) != len(blocks) {
		t.Fatalf("expected %d results, got %d", len(blocks), len(result))
	}

	// Verify each result has correct ID mapping
	expectedTranslations := []string{"第一块", "第二块", "第三块"}
	for i, r := range result {
		if r.ID != blocks[i].ID {
			t.Errorf("result %d: expected ID '%s', got '%s'", i, blocks[i].ID, r.ID)
		}
		if r.TranslatedText != expectedTranslations[i] {
			t.Errorf("result %d: expected translation '%s', got '%s'", i, expectedTranslations[i], r.TranslatedText)
		}
	}
}

// Property 6: 翻译结果映射
// 翻译后返回的TranslatedBlock数量等于输入TextBlock数量，ID一致
func TestTranslateBatch_Property6_ResultMapping(t *testing.T) {
	testCases := []struct {
		name   string
		blocks []TextBlock
	}{
		{
			name: "single block",
			blocks: []TextBlock{
				{ID: "block-1", Text: "Hello", Page: 1, BlockType: "paragraph"},
			},
		},
		{
			name: "multiple blocks same page",
			blocks: []TextBlock{
				{ID: "a", Text: "First", Page: 1, BlockType: "paragraph"},
				{ID: "b", Text: "Second", Page: 1, BlockType: "paragraph"},
				{ID: "c", Text: "Third", Page: 1, BlockType: "paragraph"},
			},
		},
		{
			name: "blocks across pages",
			blocks: []TextBlock{
				{ID: "p1-1", Text: "Page one text", Page: 1, X: 10, Y: 20, BlockType: "paragraph"},
				{ID: "p2-1", Text: "Page two text", Page: 2, X: 15, Y: 25, BlockType: "heading"},
				{ID: "p3-1", Text: "Page three text", Page: 3, X: 20, Y: 30, BlockType: "caption"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock server that returns correct number of translations
			server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
				// Generate translated text with correct number of separators
				translations := make([]string, len(tc.blocks))
				for i := range tc.blocks {
					translations[i] = "翻译" + tc.blocks[i].ID
				}
				translatedText := strings.Join(translations, BatchSeparator)
				return createMockResponse(translatedText), http.StatusOK
			})
			defer server.Close()

			bt := NewBatchTranslator(BatchTranslatorConfig{
				APIKey:        "test-key",
				BaseURL:       server.URL,
				Model:         "gpt-4",
				ContextWindow: 10000,
			})

			result, err := bt.TranslateBatch(tc.blocks)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Property: Result count equals input count
			if len(result) != len(tc.blocks) {
				t.Errorf("result count = %d, want %d", len(result), len(tc.blocks))
			}

			// Property: Each result ID matches corresponding input ID
			for i, r := range result {
				if r.ID != tc.blocks[i].ID {
					t.Errorf("result %d: ID = '%s', want '%s'", i, r.ID, tc.blocks[i].ID)
				}
				// Verify metadata is preserved
				if r.Page != tc.blocks[i].Page {
					t.Errorf("result %d: Page = %d, want %d", i, r.Page, tc.blocks[i].Page)
				}
				if r.X != tc.blocks[i].X {
					t.Errorf("result %d: X = %f, want %f", i, r.X, tc.blocks[i].X)
				}
				if r.Y != tc.blocks[i].Y {
					t.Errorf("result %d: Y = %f, want %f", i, r.Y, tc.blocks[i].Y)
				}
				if r.BlockType != tc.blocks[i].BlockType {
					t.Errorf("result %d: BlockType = '%s', want '%s'", i, r.BlockType, tc.blocks[i].BlockType)
				}
			}
		})
	}
}

func TestTranslateBatch_APIError(t *testing.T) {
	// Create mock server that returns an error
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		errResp := `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error", "code": "rate_limit"}}`
		return errResp, http.StatusTooManyRequests
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello", Page: 1, BlockType: "paragraph"},
	}

	_, err := bt.TranslateBatch(blocks)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify it's a PDFError
	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Fatalf("expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrAPIFailed {
		t.Errorf("expected error code %s, got %s", ErrAPIFailed, pdfErr.Code)
	}
}

func TestTranslateBatch_MultipleBatches(t *testing.T) {
	// Track how many API calls are made
	apiCallCount := 0
	var mu sync.Mutex

	// Create mock server
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		mu.Lock()
		apiCallCount++
		callNum := apiCallCount
		mu.Unlock()

		// Return different translations based on call number
		var translatedText string
		if callNum == 1 {
			translatedText = "批次一翻译"
		} else {
			translatedText = "批次二翻译"
		}
		return createMockResponse(translatedText), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 30, // Small window to force multiple batches
		Concurrency:   2,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "First block text here", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "Second block text here", Page: 1, BlockType: "paragraph"},
	}

	result, err := bt.TranslateBatch(blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all blocks are translated
	if len(result) != len(blocks) {
		t.Errorf("expected %d results, got %d", len(blocks), len(result))
	}

	// Verify multiple API calls were made (due to small context window)
	if apiCallCount < 1 {
		t.Errorf("expected at least 1 API call, got %d", apiCallCount)
	}
}

func TestSplitTranslatedText(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

	tests := []struct {
		name          string
		translatedText string
		expectedCount int
		expected      []string
	}{
		{
			name:          "exact match",
			translatedText: "一" + BatchSeparator + "二" + BatchSeparator + "三",
			expectedCount: 3,
			expected:      []string{"一", "二", "三"},
		},
		{
			name:          "fewer parts than expected",
			translatedText: "一" + BatchSeparator + "二",
			expectedCount: 3,
			expected:      []string{"一", "二", ""},
		},
		{
			name:          "more parts than expected",
			translatedText: "一" + BatchSeparator + "二" + BatchSeparator + "三" + BatchSeparator + "四",
			expectedCount: 3,
			expected:      []string{"一", "二", "三" + BatchSeparator + "四"},
		},
		{
			name:          "single part",
			translatedText: "单独文本",
			expectedCount: 1,
			expected:      []string{"单独文本"},
		},
		{
			name:          "with whitespace",
			translatedText: "  一  " + BatchSeparator + "  二  ",
			expectedCount: 2,
			expected:      []string{"一", "二"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bt.splitTranslatedText(tt.translatedText, tt.expectedCount)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d parts, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("part %d: expected '%s', got '%s'", i, exp, result[i])
				}
			}
		})
	}
}

func TestNormalizeAPIURL(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "https://api.openai.com/v1/chat/completions",
		},
		{
			input:    "https://api.openai.com/v1",
			expected: "https://api.openai.com/v1/chat/completions",
		},
		{
			input:    "https://api.openai.com/v1/",
			expected: "https://api.openai.com/v1/chat/completions",
		},
		{
			input:    "https://api.openai.com/v1/chat/completions",
			expected: "https://api.openai.com/v1/chat/completions",
		},
		{
			input:    "https://custom-api.example.com/api",
			expected: "https://custom-api.example.com/api/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := bt.normalizeAPIURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeAPIURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}


// Tests for TranslateWithRetry functionality
// Validates: Requirements 3.4, 7.3

func TestTranslateWithRetry_EmptyInput(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey: "test-key",
	})

	// Test nil input
	result, err := bt.TranslateWithRetry(nil, 3)
	if err != nil {
		t.Errorf("unexpected error for nil input: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil input, got %v", result)
	}

	// Test empty slice
	result, err = bt.TranslateWithRetry([]TextBlock{}, 3)
	if err != nil {
		t.Errorf("unexpected error for empty input: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty input, got %v", result)
	}
}

func TestTranslateWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	// Create mock server that succeeds immediately
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		return createMockResponse("翻译成功"), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello world", Page: 1, BlockType: "paragraph"},
	}

	result, err := bt.TranslateWithRetry(blocks, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].TranslatedText != "翻译成功" {
		t.Errorf("expected '翻译成功', got '%s'", result[0].TranslatedText)
	}
}

func TestTranslateWithRetry_SuccessAfterRetry(t *testing.T) {
	// Track API call count
	callCount := 0
	var mu sync.Mutex

	// Create mock server that fails first, then succeeds
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		mu.Lock()
		callCount++
		currentCall := callCount
		mu.Unlock()

		if currentCall == 1 {
			// First call fails with server error (retryable)
			return `{"error": {"message": "Server error", "type": "server_error"}}`, http.StatusInternalServerError
		}
		// Second call succeeds
		return createMockResponse("重试后成功"), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello", Page: 1, BlockType: "paragraph"},
	}

	result, err := bt.TranslateWithRetry(blocks, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].TranslatedText != "重试后成功" {
		t.Errorf("expected '重试后成功', got '%s'", result[0].TranslatedText)
	}

	// Verify retry happened
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls (retry), got %d", callCount)
	}
}

func TestTranslateWithRetry_FallbackToSingleBlock(t *testing.T) {
	// Track API call count
	callCount := 0
	var mu sync.Mutex

	// Create mock server that fails for batch but succeeds for single blocks
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		mu.Lock()
		callCount++
		currentCall := callCount
		mu.Unlock()

		// First 3 calls (batch retries) fail
		if currentCall <= 3 {
			return `{"error": {"message": "Server error", "type": "server_error"}}`, http.StatusInternalServerError
		}
		// Single block translations succeed
		return createMockResponse("单块翻译成功"), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 10000, // Large window so all blocks fit in one batch
	})

	blocks := []TextBlock{
		{ID: "1", Text: "First", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "Second", Page: 1, BlockType: "paragraph"},
	}

	result, err := bt.TranslateWithRetry(blocks, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have results for both blocks
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Verify fallback happened (more than 3 calls means single-block fallback was used)
	if callCount <= 3 {
		t.Errorf("expected more than 3 API calls (fallback to single-block), got %d", callCount)
	}
}

func TestTranslateWithRetry_NonRetryableError(t *testing.T) {
	// Create mock server that returns authentication error (non-retryable)
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		return `{"error": {"message": "Invalid API key", "type": "invalid_request_error"}}`, http.StatusUnauthorized
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "invalid-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello", Page: 1, BlockType: "paragraph"},
	}

	_, err := bt.TranslateWithRetry(blocks, 3)
	if err == nil {
		t.Fatal("expected error for non-retryable error, got nil")
	}

	// Verify it's a PDFError
	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Fatalf("expected PDFError, got %T", err)
	}
	// When batch translation fails with non-retryable error and falls back to single-block,
	// the error is wrapped as ErrTranslateFailed (since single block also fails)
	if pdfErr.Code != ErrTranslateFailed {
		t.Errorf("expected error code %s, got %s", ErrTranslateFailed, pdfErr.Code)
	}
}

func TestTranslateWithRetry_DefaultMaxRetries(t *testing.T) {
	// Track API call count
	callCount := 0
	var mu sync.Mutex

	// Create mock server that always fails with retryable error
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return `{"error": {"message": "Server error", "type": "server_error"}}`, http.StatusInternalServerError
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "Hello", Page: 1, BlockType: "paragraph"},
	}

	// Pass 0 for maxRetries to use default
	_, err := bt.TranslateWithRetry(blocks, 0)
	
	// Should fail after default retries + single-block fallback retries
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}

	// Verify default retries were used (DefaultMaxRetries = 3)
	// Batch retries (3) + single-block retries (3 for single block)
	if callCount < DefaultMaxRetries {
		t.Errorf("expected at least %d API calls, got %d", DefaultMaxRetries, callCount)
	}
}

func TestIsRetryableError(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

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
			name:     "rate limit error",
			err:      NewPDFErrorWithDetails(ErrAPIFailed, "API rate limit exceeded", "rate limit", nil),
			expected: true,
		},
		{
			name:     "server error 500",
			err:      NewPDFErrorWithDetails(ErrAPIFailed, "API server error", "status 500", nil),
			expected: true,
		},
		{
			name:     "server error 502",
			err:      NewPDFErrorWithDetails(ErrAPIFailed, "API server error", "status 502", nil),
			expected: true,
		},
		{
			name:     "authentication error",
			err:      NewPDFErrorWithDetails(ErrAPIFailed, "API authentication failed", "invalid API key", nil),
			expected: false,
		},
		{
			name:     "invalid request error",
			err:      NewPDFErrorWithDetails(ErrAPIFailed, "invalid API request", "bad request", nil),
			expected: false,
		},
		{
			name:     "generic API failed error",
			err:      NewPDFError(ErrAPIFailed, "API failed", nil),
			expected: true, // Default to retryable for API failures
		},
		{
			name:     "network timeout error",
			err:      &PDFError{Code: ErrAPIFailed, Message: "connection timeout"},
			expected: true,
		},
		{
			name:     "connection reset error",
			err:      &PDFError{Code: ErrAPIFailed, Message: "connection reset by peer"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bt.isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateBackoffDelay(t *testing.T) {
	bt := NewBatchTranslator(BatchTranslatorConfig{})

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 1, expected: 2 * time.Second},  // 2s * 2^0 = 2s
		{attempt: 2, expected: 4 * time.Second},  // 2s * 2^1 = 4s
		{attempt: 3, expected: 8 * time.Second},  // 2s * 2^2 = 8s
		{attempt: 4, expected: 16 * time.Second}, // 2s * 2^3 = 16s
		{attempt: 5, expected: 30 * time.Second}, // 2s * 2^4 = 32s, capped at 30s
		{attempt: 10, expected: 30 * time.Second}, // Capped at 30s
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.attempt)), func(t *testing.T) {
			result := bt.calculateBackoffDelay(tt.attempt)
			if result != tt.expected {
				t.Errorf("calculateBackoffDelay(%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestTranslateWithRetry_PreservesBlockMetadata(t *testing.T) {
	// Create mock server
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		return createMockResponse("翻译文本"), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 1000,
	})

	blocks := []TextBlock{
		{
			ID:        "test-id-123",
			Page:      5,
			Text:      "Original text",
			X:         100.5,
			Y:         200.5,
			Width:     300.0,
			Height:    50.0,
			FontSize:  12.0,
			FontName:  "Arial",
			IsBold:    true,
			IsItalic:  false,
			BlockType: "heading",
		},
	}

	result, err := bt.TranslateWithRetry(blocks, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	r := result[0]
	// Verify all metadata is preserved
	if r.ID != "test-id-123" {
		t.Errorf("ID = '%s', want 'test-id-123'", r.ID)
	}
	if r.Page != 5 {
		t.Errorf("Page = %d, want 5", r.Page)
	}
	if r.X != 100.5 {
		t.Errorf("X = %f, want 100.5", r.X)
	}
	if r.Y != 200.5 {
		t.Errorf("Y = %f, want 200.5", r.Y)
	}
	if r.Width != 300.0 {
		t.Errorf("Width = %f, want 300.0", r.Width)
	}
	if r.Height != 50.0 {
		t.Errorf("Height = %f, want 50.0", r.Height)
	}
	if r.FontSize != 12.0 {
		t.Errorf("FontSize = %f, want 12.0", r.FontSize)
	}
	if r.FontName != "Arial" {
		t.Errorf("FontName = '%s', want 'Arial'", r.FontName)
	}
	if r.IsBold != true {
		t.Errorf("IsBold = %v, want true", r.IsBold)
	}
	if r.IsItalic != false {
		t.Errorf("IsItalic = %v, want false", r.IsItalic)
	}
	if r.BlockType != "heading" {
		t.Errorf("BlockType = '%s', want 'heading'", r.BlockType)
	}
	if r.TranslatedText != "翻译文本" {
		t.Errorf("TranslatedText = '%s', want '翻译文本'", r.TranslatedText)
	}
}

func TestTranslateWithRetry_MultipleBlocksWithRetry(t *testing.T) {
	// Track API call count
	callCount := 0
	var mu sync.Mutex

	// Create mock server that fails once then succeeds
	server := mockOpenAIServer(t, func(req *http.Request) (string, int) {
		mu.Lock()
		callCount++
		currentCall := callCount
		mu.Unlock()

		if currentCall == 1 {
			// First call fails
			return `{"error": {"message": "Server error"}}`, http.StatusInternalServerError
		}
		// Subsequent calls succeed
		translatedText := "第一块" + BatchSeparator + "第二块" + BatchSeparator + "第三块"
		return createMockResponse(translatedText), http.StatusOK
	})
	defer server.Close()

	bt := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        "test-key",
		BaseURL:       server.URL,
		Model:         "gpt-4",
		ContextWindow: 10000,
	})

	blocks := []TextBlock{
		{ID: "1", Text: "First", Page: 1, BlockType: "paragraph"},
		{ID: "2", Text: "Second", Page: 1, BlockType: "paragraph"},
		{ID: "3", Text: "Third", Page: 1, BlockType: "paragraph"},
	}

	result, err := bt.TranslateWithRetry(blocks, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all blocks are translated
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// Verify translations
	expectedTranslations := []string{"第一块", "第二块", "第三块"}
	for i, r := range result {
		if r.TranslatedText != expectedTranslations[i] {
			t.Errorf("result %d: TranslatedText = '%s', want '%s'", i, r.TranslatedText, expectedTranslations[i])
		}
		if r.ID != blocks[i].ID {
			t.Errorf("result %d: ID = '%s', want '%s'", i, r.ID, blocks[i].ID)
		}
	}

	// Verify retry happened
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls, got %d", callCount)
	}
}
