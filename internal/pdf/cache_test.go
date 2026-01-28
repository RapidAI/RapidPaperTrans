package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestComputeHashConsistency tests Property 12: 哈希一致性
// 相同文本多次ComputeHash返回相同值
// **Validates: Requirements 6.3**
func TestComputeHashConsistency(t *testing.T) {
	cache := NewTranslationCache("")

	testCases := []struct {
		name string
		text string
	}{
		{"empty string", ""},
		{"simple text", "Hello, World!"},
		{"chinese text", "你好，世界！"},
		{"special characters", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"unicode", "🎉🎊🎁"},
		{"long text", "This is a very long text that should still produce consistent hash values across multiple calls to ComputeHash function."},
		{"whitespace", "   \t\n\r   "},
		{"mixed content", "Hello 你好 123 !@# 🎉"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash1 := cache.ComputeHash(tc.text)
			hash2 := cache.ComputeHash(tc.text)
			hash3 := cache.ComputeHash(tc.text)

			if hash1 != hash2 || hash2 != hash3 {
				t.Errorf("ComputeHash not consistent for %q: got %s, %s, %s", tc.text, hash1, hash2, hash3)
			}

			// Verify hash is a valid hex string (64 chars for SHA256)
			if len(hash1) != 64 {
				t.Errorf("Expected hash length 64, got %d", len(hash1))
			}
		})
	}
}

// TestComputeHashDifferentTexts tests that different texts produce different hashes
// **Validates: Requirements 6.3**
func TestComputeHashDifferentTexts(t *testing.T) {
	cache := NewTranslationCache("")

	texts := []string{
		"Hello",
		"hello",
		"Hello ",
		" Hello",
		"Hello!",
		"World",
	}

	hashes := make(map[string]string)
	for _, text := range texts {
		hash := cache.ComputeHash(text)
		if existingText, exists := hashes[hash]; exists {
			t.Errorf("Hash collision: %q and %q both produce hash %s", text, existingText, hash)
		}
		hashes[hash] = text
	}
}

// TestCacheSetGet tests Property 10: 缓存往返 - Set后Get应返回相同值
// **Validates: Requirements 6.1, 6.3**
func TestCacheSetGet(t *testing.T) {
	cache := NewTranslationCache("")

	testCases := []struct {
		text        string
		translation string
	}{
		{"Hello", "你好"},
		{"World", "世界"},
		{"This is a test", "这是一个测试"},
		{"", "空字符串"},
		{"Special chars: !@#$%", "特殊字符：!@#$%"},
	}

	for _, tc := range testCases {
		t.Run(tc.text, func(t *testing.T) {
			cache.Set(tc.text, tc.translation)

			got, ok := cache.Get(tc.text)
			if !ok {
				t.Errorf("Get(%q) returned not found after Set", tc.text)
			}
			if got != tc.translation {
				t.Errorf("Get(%q) = %q, want %q", tc.text, got, tc.translation)
			}
		})
	}
}

// TestCacheGetNotFound tests Get returns false for non-existent keys
func TestCacheGetNotFound(t *testing.T) {
	cache := NewTranslationCache("")

	_, ok := cache.Get("non-existent")
	if ok {
		t.Error("Get should return false for non-existent key")
	}
}

// TestCacheOverwrite tests that Set overwrites existing values
func TestCacheOverwrite(t *testing.T) {
	cache := NewTranslationCache("")

	cache.Set("test", "translation1")
	cache.Set("test", "translation2")

	got, ok := cache.Get("test")
	if !ok {
		t.Error("Get should return true after Set")
	}
	if got != "translation2" {
		t.Errorf("Get = %q, want %q", got, "translation2")
	}
}


// TestCacheSaveLoad tests Property 10: Save后Load应保持内容不变
// **Validates: Requirements 6.4, 6.5**
func TestCacheSaveLoad(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cachePath := filepath.Join(tmpDir, "test_cache.json")

	// Create cache and add entries
	cache1 := NewTranslationCache(cachePath)
	testData := map[string]string{
		"Hello":           "你好",
		"World":           "世界",
		"This is a test":  "这是一个测试",
		"Special: !@#$%":  "特殊：!@#$%",
		"Unicode: 🎉🎊🎁": "表情符号",
	}

	for text, translation := range testData {
		cache1.Set(text, translation)
	}

	// Save the cache
	if err := cache1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Create a new cache and load from file
	cache2 := NewTranslationCache(cachePath)
	if err := cache2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify all entries are preserved
	for text, expectedTranslation := range testData {
		got, ok := cache2.Get(text)
		if !ok {
			t.Errorf("After Load, Get(%q) returned not found", text)
			continue
		}
		if got != expectedTranslation {
			t.Errorf("After Load, Get(%q) = %q, want %q", text, got, expectedTranslation)
		}
	}

	// Verify cache sizes match
	if cache1.Size() != cache2.Size() {
		t.Errorf("Cache sizes don't match: original=%d, loaded=%d", cache1.Size(), cache2.Size())
	}
}

// TestCacheLoadNonExistent tests Load with non-existent file
func TestCacheLoadNonExistent(t *testing.T) {
	cache := NewTranslationCache("/non/existent/path/cache.json")

	// Load should not return error for non-existent file
	if err := cache.Load(); err != nil {
		t.Errorf("Load should not error for non-existent file: %v", err)
	}

	// Cache should be empty
	if cache.Size() != 0 {
		t.Errorf("Cache should be empty after loading non-existent file, got size %d", cache.Size())
	}
}

// TestCacheLoadEmptyPath tests Load with empty path
func TestCacheLoadEmptyPath(t *testing.T) {
	cache := NewTranslationCache("")

	if err := cache.Load(); err != nil {
		t.Errorf("Load should not error for empty path: %v", err)
	}
}

// TestCacheSaveEmptyPath tests Save with empty path
func TestCacheSaveEmptyPath(t *testing.T) {
	cache := NewTranslationCache("")
	cache.Set("test", "translation")

	if err := cache.Save(); err != nil {
		t.Errorf("Save should not error for empty path: %v", err)
	}
}

// TestFilterCached tests Property 11: 缓存过滤正确性
// FilterCached返回的cached和uncached长度之和等于输入长度
// **Validates: Requirements 6.2**
func TestFilterCached(t *testing.T) {
	cache := NewTranslationCache("")

	// Pre-populate cache with some translations
	cache.Set("cached text 1", "缓存文本1")
	cache.Set("cached text 2", "缓存文本2")

	// Create test blocks - mix of cached and uncached
	blocks := []TextBlock{
		{ID: "1", Page: 1, Text: "cached text 1", X: 0, Y: 0, Width: 100, Height: 20, BlockType: "paragraph"},
		{ID: "2", Page: 1, Text: "uncached text 1", X: 0, Y: 20, Width: 100, Height: 20, BlockType: "paragraph"},
		{ID: "3", Page: 1, Text: "cached text 2", X: 0, Y: 40, Width: 100, Height: 20, BlockType: "paragraph"},
		{ID: "4", Page: 2, Text: "uncached text 2", X: 0, Y: 0, Width: 100, Height: 20, BlockType: "paragraph"},
		{ID: "5", Page: 2, Text: "uncached text 3", X: 0, Y: 20, Width: 100, Height: 20, BlockType: "paragraph"},
	}

	cached, uncached := cache.FilterCached(blocks)

	// Property 11: cached + uncached length should equal input length
	if len(cached)+len(uncached) != len(blocks) {
		t.Errorf("FilterCached: len(cached)=%d + len(uncached)=%d != len(blocks)=%d",
			len(cached), len(uncached), len(blocks))
	}

	// Verify cached count
	if len(cached) != 2 {
		t.Errorf("Expected 2 cached blocks, got %d", len(cached))
	}

	// Verify uncached count
	if len(uncached) != 3 {
		t.Errorf("Expected 3 uncached blocks, got %d", len(uncached))
	}

	// Verify cached blocks have correct translations
	for _, block := range cached {
		if !block.FromCache {
			t.Errorf("Cached block %s should have FromCache=true", block.ID)
		}
		expectedTranslation, _ := cache.Get(block.Text)
		if block.TranslatedText != expectedTranslation {
			t.Errorf("Cached block %s has wrong translation: got %q, want %q",
				block.ID, block.TranslatedText, expectedTranslation)
		}
	}
}

// TestFilterCachedEmpty tests FilterCached with empty input
func TestFilterCachedEmpty(t *testing.T) {
	cache := NewTranslationCache("")
	cache.Set("some text", "一些文本")

	cached, uncached := cache.FilterCached([]TextBlock{})

	if len(cached) != 0 {
		t.Errorf("Expected 0 cached blocks for empty input, got %d", len(cached))
	}
	if len(uncached) != 0 {
		t.Errorf("Expected 0 uncached blocks for empty input, got %d", len(uncached))
	}
}

// TestFilterCachedAllCached tests FilterCached when all blocks are cached
func TestFilterCachedAllCached(t *testing.T) {
	cache := NewTranslationCache("")
	cache.Set("text 1", "文本1")
	cache.Set("text 2", "文本2")

	blocks := []TextBlock{
		{ID: "1", Page: 1, Text: "text 1", X: 0, Y: 0, Width: 100, Height: 20, BlockType: "paragraph"},
		{ID: "2", Page: 1, Text: "text 2", X: 0, Y: 20, Width: 100, Height: 20, BlockType: "paragraph"},
	}

	cached, uncached := cache.FilterCached(blocks)

	if len(cached) != 2 {
		t.Errorf("Expected 2 cached blocks, got %d", len(cached))
	}
	if len(uncached) != 0 {
		t.Errorf("Expected 0 uncached blocks, got %d", len(uncached))
	}
}

// TestFilterCachedNoneCached tests FilterCached when no blocks are cached
func TestFilterCachedNoneCached(t *testing.T) {
	cache := NewTranslationCache("")

	blocks := []TextBlock{
		{ID: "1", Page: 1, Text: "text 1", X: 0, Y: 0, Width: 100, Height: 20, BlockType: "paragraph"},
		{ID: "2", Page: 1, Text: "text 2", X: 0, Y: 20, Width: 100, Height: 20, BlockType: "paragraph"},
	}

	cached, uncached := cache.FilterCached(blocks)

	if len(cached) != 0 {
		t.Errorf("Expected 0 cached blocks, got %d", len(cached))
	}
	if len(uncached) != 2 {
		t.Errorf("Expected 2 uncached blocks, got %d", len(uncached))
	}
}

// TestCacheSize tests the Size method
func TestCacheSize(t *testing.T) {
	cache := NewTranslationCache("")

	if cache.Size() != 0 {
		t.Errorf("New cache should have size 0, got %d", cache.Size())
	}

	cache.Set("text1", "translation1")
	if cache.Size() != 1 {
		t.Errorf("Cache should have size 1, got %d", cache.Size())
	}

	cache.Set("text2", "translation2")
	if cache.Size() != 2 {
		t.Errorf("Cache should have size 2, got %d", cache.Size())
	}

	// Overwriting should not increase size
	cache.Set("text1", "new translation")
	if cache.Size() != 2 {
		t.Errorf("Cache should still have size 2 after overwrite, got %d", cache.Size())
	}
}

// TestCacheClear tests the Clear method
func TestCacheClear(t *testing.T) {
	cache := NewTranslationCache("")
	cache.Set("text1", "translation1")
	cache.Set("text2", "translation2")

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Cache should be empty after Clear, got size %d", cache.Size())
	}

	_, ok := cache.Get("text1")
	if ok {
		t.Error("Get should return false after Clear")
	}
}

// TestCachePathMethods tests GetCachePath and SetCachePath
func TestCachePathMethods(t *testing.T) {
	cache := NewTranslationCache("/original/path")

	if cache.GetCachePath() != "/original/path" {
		t.Errorf("GetCachePath = %q, want %q", cache.GetCachePath(), "/original/path")
	}

	cache.SetCachePath("/new/path")
	if cache.GetCachePath() != "/new/path" {
		t.Errorf("After SetCachePath, GetCachePath = %q, want %q", cache.GetCachePath(), "/new/path")
	}
}
