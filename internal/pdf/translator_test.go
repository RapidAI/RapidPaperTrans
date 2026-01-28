package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"latex-translator/internal/types"
)

// TestTranslatorCacheIntegration tests that the translator properly integrates with the cache
// Validates: Requirements 6.1, 6.2, 7.6
func TestTranslatorCacheIntegration(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "test_cache.json")

	// Create a translator with the test cache path
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:    nil, // No API config needed for cache tests
		WorkDir:   tempDir,
		CachePath: cachePath,
	})

	// Verify cache is initialized
	if translator.cache == nil {
		t.Fatal("Cache should be initialized")
	}

	// Verify cache path is set correctly
	if translator.cache.GetCachePath() != cachePath {
		t.Errorf("Cache path mismatch: got %s, want %s", translator.cache.GetCachePath(), cachePath)
	}
}

// TestTranslatorCacheLoadBeforeTranslation tests that cache is loaded before translation
// Validates: Requirements 6.1, 6.5
func TestTranslatorCacheLoadBeforeTranslation(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "test_cache.json")

	// Pre-populate the cache file with some entries
	preCache := NewTranslationCache(cachePath)
	preCache.Set("Hello", "你好")
	preCache.Set("World", "世界")
	if err := preCache.Save(); err != nil {
		t.Fatalf("Failed to save pre-cache: %v", err)
	}

	// Create a new translator - it should be able to load the cache
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:    nil,
		WorkDir:   tempDir,
		CachePath: cachePath,
	})

	// Load the cache
	if err := translator.cache.Load(); err != nil {
		t.Fatalf("Failed to load cache: %v", err)
	}

	// Verify the cache was loaded
	if translator.cache.Size() != 2 {
		t.Errorf("Cache size mismatch: got %d, want 2", translator.cache.Size())
	}

	// Verify specific entries
	if trans, ok := translator.cache.Get("Hello"); !ok || trans != "你好" {
		t.Errorf("Cache entry 'Hello' mismatch: got %s, ok=%v", trans, ok)
	}
	if trans, ok := translator.cache.Get("World"); !ok || trans != "世界" {
		t.Errorf("Cache entry 'World' mismatch: got %s, ok=%v", trans, ok)
	}
}

// TestTranslatorFilterCachedBlocks tests that FilterCached correctly separates cached and uncached blocks
// Validates: Requirements 6.2
func TestTranslatorFilterCachedBlocks(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "test_cache.json")

	// Create translator and add some cached entries
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:    nil,
		WorkDir:   tempDir,
		CachePath: cachePath,
	})

	// Add some entries to cache
	translator.cache.Set("Cached text 1", "缓存文本 1")
	translator.cache.Set("Cached text 2", "缓存文本 2")

	// Create test blocks - some cached, some not
	blocks := []TextBlock{
		{ID: "1", Page: 1, Text: "Cached text 1", BlockType: "paragraph"},
		{ID: "2", Page: 1, Text: "Uncached text 1", BlockType: "paragraph"},
		{ID: "3", Page: 2, Text: "Cached text 2", BlockType: "paragraph"},
		{ID: "4", Page: 2, Text: "Uncached text 2", BlockType: "paragraph"},
	}

	// Filter the blocks
	cached, uncached := translator.cache.FilterCached(blocks)

	// Verify counts
	if len(cached) != 2 {
		t.Errorf("Cached count mismatch: got %d, want 2", len(cached))
	}
	if len(uncached) != 2 {
		t.Errorf("Uncached count mismatch: got %d, want 2", len(uncached))
	}

	// Verify cached blocks have FromCache flag set
	for _, block := range cached {
		if !block.FromCache {
			t.Errorf("Cached block %s should have FromCache=true", block.ID)
		}
		if block.TranslatedText == "" {
			t.Errorf("Cached block %s should have TranslatedText set", block.ID)
		}
	}

	// Verify uncached blocks are the correct ones
	uncachedTexts := make(map[string]bool)
	for _, block := range uncached {
		uncachedTexts[block.Text] = true
	}
	if !uncachedTexts["Uncached text 1"] || !uncachedTexts["Uncached text 2"] {
		t.Error("Uncached blocks should contain 'Uncached text 1' and 'Uncached text 2'")
	}
}

// TestTranslatorCacheSaveOnCancel tests that cache is saved when translation is cancelled
// Validates: Requirements 7.6
func TestTranslatorCacheSaveOnCancel(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "test_cache.json")

	// Create translator
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:    nil,
		WorkDir:   tempDir,
		CachePath: cachePath,
	})

	// Simulate some translated blocks (as if translation was in progress)
	translator.translatedBlocks = []TranslatedBlock{
		{
			TextBlock:      TextBlock{ID: "1", Page: 1, Text: "Test text 1", BlockType: "paragraph"},
			TranslatedText: "测试文本 1",
			FromCache:      false, // This was newly translated
		},
		{
			TextBlock:      TextBlock{ID: "2", Page: 1, Text: "Test text 2", BlockType: "paragraph"},
			TranslatedText: "测试文本 2",
			FromCache:      false, // This was newly translated
		},
		{
			TextBlock:      TextBlock{ID: "3", Page: 1, Text: "Cached text", BlockType: "paragraph"},
			TranslatedText: "缓存文本",
			FromCache:      true, // This was from cache, should not be re-saved
		},
	}

	// Cancel the translation
	if err := translator.CancelTranslation(); err != nil {
		t.Fatalf("CancelTranslation failed: %v", err)
	}

	// Verify cache file was created
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("Cache file should exist after cancel")
	}

	// Load the cache in a new instance and verify entries were saved
	newCache := NewTranslationCache(cachePath)
	if err := newCache.Load(); err != nil {
		t.Fatalf("Failed to load cache: %v", err)
	}

	// Verify the newly translated blocks were saved
	if trans, ok := newCache.Get("Test text 1"); !ok || trans != "测试文本 1" {
		t.Errorf("Cache should contain 'Test text 1': got %s, ok=%v", trans, ok)
	}
	if trans, ok := newCache.Get("Test text 2"); !ok || trans != "测试文本 2" {
		t.Errorf("Cache should contain 'Test text 2': got %s, ok=%v", trans, ok)
	}
}

// TestTranslatorCacheSaveOnClose tests that cache is saved when translator is closed
// Validates: Requirements 6.4
func TestTranslatorCacheSaveOnClose(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "test_cache.json")

	// Create translator and add some cache entries
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:    nil,
		WorkDir:   tempDir,
		CachePath: cachePath,
	})

	// Add entries to cache
	translator.cache.Set("Close test", "关闭测试")

	// Close the translator
	if err := translator.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify cache file was created
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("Cache file should exist after close")
	}

	// Load the cache in a new instance and verify entries were saved
	newCache := NewTranslationCache(cachePath)
	if err := newCache.Load(); err != nil {
		t.Fatalf("Failed to load cache: %v", err)
	}

	if trans, ok := newCache.Get("Close test"); !ok || trans != "关闭测试" {
		t.Errorf("Cache should contain 'Close test': got %s, ok=%v", trans, ok)
	}
}

// TestTranslatorStatusTracking tests that status is properly tracked during operations
// Validates: Requirements 3.5, 5.5
func TestTranslatorStatusTracking(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create translator
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:  nil,
		WorkDir: tempDir,
	})

	// Initial status should be idle
	status := translator.GetStatus()
	if status.Phase != PDFPhaseIdle {
		t.Errorf("Initial phase should be idle, got %s", status.Phase)
	}
	if status.Progress != 0 {
		t.Errorf("Initial progress should be 0, got %d", status.Progress)
	}

	// Verify status is valid
	if !status.IsValidStatus() {
		t.Error("Status should be valid")
	}
}

// TestTranslatorCachedBlocksCount tests that cached blocks count is tracked correctly
// Validates: Requirements 6.2
func TestTranslatorCachedBlocksCount(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "test_cache.json")

	// Create translator and add some cached entries
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:    nil,
		WorkDir:   tempDir,
		CachePath: cachePath,
	})

	// Add entries to cache
	translator.cache.Set("Block 1", "块 1")
	translator.cache.Set("Block 2", "块 2")
	translator.cache.Set("Block 3", "块 3")

	// Create test blocks
	blocks := []TextBlock{
		{ID: "1", Page: 1, Text: "Block 1", BlockType: "paragraph"},
		{ID: "2", Page: 1, Text: "Block 2", BlockType: "paragraph"},
		{ID: "3", Page: 1, Text: "Block 3", BlockType: "paragraph"},
		{ID: "4", Page: 1, Text: "Block 4", BlockType: "paragraph"}, // Not cached
		{ID: "5", Page: 1, Text: "Block 5", BlockType: "paragraph"}, // Not cached
	}

	// Filter cached blocks
	cached, uncached := translator.cache.FilterCached(blocks)

	// Verify counts
	if len(cached) != 3 {
		t.Errorf("Should have 3 cached blocks, got %d", len(cached))
	}
	if len(uncached) != 2 {
		t.Errorf("Should have 2 uncached blocks, got %d", len(uncached))
	}

	// Verify total equals input
	if len(cached)+len(uncached) != len(blocks) {
		t.Errorf("Total blocks mismatch: cached(%d) + uncached(%d) != total(%d)",
			len(cached), len(uncached), len(blocks))
	}
}

// TestTranslatorSetWorkDir tests that SetWorkDir updates cache path correctly
// Validates: Requirements 6.4
func TestTranslatorSetWorkDir(t *testing.T) {
	// Create temporary directories for the test
	tempDir1, err := os.MkdirTemp("", "translator_test1")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "translator_test2")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	// Create translator with first work dir
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:  nil,
		WorkDir: tempDir1,
	})

	// Verify initial cache path
	expectedPath1 := filepath.Join(tempDir1, "pdf_translation_cache.json")
	if translator.cache.GetCachePath() != expectedPath1 {
		t.Errorf("Initial cache path mismatch: got %s, want %s",
			translator.cache.GetCachePath(), expectedPath1)
	}

	// Change work directory
	translator.SetWorkDir(tempDir2)

	// Verify cache path was updated
	expectedPath2 := filepath.Join(tempDir2, "pdf_translation_cache.json")
	if translator.cache.GetCachePath() != expectedPath2 {
		t.Errorf("Updated cache path mismatch: got %s, want %s",
			translator.cache.GetCachePath(), expectedPath2)
	}
}

// TestTranslatorUpdateConfig tests that UpdateConfig properly recreates the batch translator
func TestTranslatorUpdateConfig(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create translator without config
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:  nil,
		WorkDir: tempDir,
	})

	// Initially translator should be nil
	if translator.translator != nil {
		t.Error("Translator should be nil when no config provided")
	}

	// Update with a config
	config := &types.Config{
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.openai.com/v1",
		OpenAIModel:   "gpt-4",
		ContextWindow: 4000,
		Concurrency:   2,
	}
	translator.UpdateConfig(config)

	// Now translator should be created
	if translator.translator == nil {
		t.Error("Translator should be created after UpdateConfig")
	}
}

// TestTranslatorReset tests that Reset clears state properly
func TestTranslatorReset(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create translator
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:  nil,
		WorkDir: tempDir,
	})

	// Set some state
	translator.currentFile = "/path/to/test.pdf"
	translator.textBlocks = []TextBlock{{ID: "1", Text: "test"}}
	translator.translatedBlocks = []TranslatedBlock{{TextBlock: TextBlock{ID: "1"}, TranslatedText: "测试"}}
	translator.status.Phase = PDFPhaseTranslating
	translator.status.Progress = 50

	// Reset
	translator.Reset()

	// Verify state is cleared
	if translator.currentFile != "" {
		t.Error("currentFile should be empty after reset")
	}
	if translator.textBlocks != nil {
		t.Error("textBlocks should be nil after reset")
	}
	if translator.translatedBlocks != nil {
		t.Error("translatedBlocks should be nil after reset")
	}
	if translator.status.Phase != PDFPhaseIdle {
		t.Errorf("Phase should be idle after reset, got %s", translator.status.Phase)
	}
	if translator.status.Progress != 0 {
		t.Errorf("Progress should be 0 after reset, got %d", translator.status.Progress)
	}
}

// TestTranslatorProgressUpdate tests that progress is calculated correctly
func TestTranslatorProgressUpdate(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create translator
	translator := NewPDFTranslator(PDFTranslatorConfig{
		Config:  nil,
		WorkDir: tempDir,
	})

	// Test progress calculation
	testCases := []struct {
		completed int
		total     int
		minProg   int
		maxProg   int
	}{
		{0, 10, 10, 10},   // 0% completed -> 10% progress (minimum during translation)
		{5, 10, 40, 50},   // 50% completed -> ~50% progress
		{10, 10, 90, 90},  // 100% completed -> 90% progress (max during translation)
		{0, 0, 0, 0},      // Edge case: no blocks
	}

	for _, tc := range testCases {
		translator.mu.Lock()
		translator.updateProgressLocked(tc.completed, tc.total)
		progress := translator.status.Progress
		translator.mu.Unlock()

		if progress < tc.minProg || progress > tc.maxProg {
			t.Errorf("Progress for %d/%d should be between %d and %d, got %d",
				tc.completed, tc.total, tc.minProg, tc.maxProg, progress)
		}
	}
}
