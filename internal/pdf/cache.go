package pdf

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// TranslationCache 负责缓存翻译结果
type TranslationCache struct {
	cachePath string
	cache     map[string]CacheEntry // hash -> CacheEntry
	mu        sync.RWMutex
}

// NewTranslationCache 创建新的翻译缓存实例
func NewTranslationCache(cachePath string) *TranslationCache {
	return &TranslationCache{
		cachePath: cachePath,
		cache:     make(map[string]CacheEntry),
	}
}

// ComputeHash 计算文本哈希（使用 SHA256）
func (c *TranslationCache) ComputeHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

// Get 获取缓存的翻译
func (c *TranslationCache) Get(text string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hash := c.ComputeHash(text)
	entry, ok := c.cache[hash]
	if !ok {
		return "", false
	}
	return entry.Translation, true
}

// Set 设置翻译缓存
func (c *TranslationCache) Set(text, translation string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := c.ComputeHash(text)
	c.cache[hash] = CacheEntry{
		Hash:        hash,
		Original:    text,
		Translation: translation,
		CreatedAt:   time.Now(),
	}
}


// Load 从文件加载缓存
func (c *TranslationCache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If cache path is empty, nothing to load
	if c.cachePath == "" {
		return nil
	}

	// Check if file exists
	if _, err := os.Stat(c.cachePath); os.IsNotExist(err) {
		// File doesn't exist, start with empty cache
		return nil
	}

	// Read the file
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return NewPDFError(ErrCacheFailed, "failed to read cache file", err)
	}

	// Parse the cache file
	var cacheFile CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return NewPDFError(ErrCacheFailed, "failed to parse cache file", err)
	}

	// Rebuild the cache map from entries
	c.cache = make(map[string]CacheEntry)
	for _, entry := range cacheFile.Entries {
		c.cache[entry.Hash] = entry
	}

	return nil
}

// Save 保存缓存到文件
func (c *TranslationCache) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// If cache path is empty, nothing to save
	if c.cachePath == "" {
		return nil
	}

	// Convert cache map to entries slice
	entries := make([]CacheEntry, 0, len(c.cache))
	for _, entry := range c.cache {
		entries = append(entries, entry)
	}

	// Create cache file structure
	cacheFile := CacheFile{
		Version: "1.0",
		Entries: entries,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(cacheFile, "", "  ")
	if err != nil {
		return NewPDFError(ErrCacheFailed, "failed to marshal cache", err)
	}

	// Write to file
	if err := os.WriteFile(c.cachePath, data, 0644); err != nil {
		return NewPDFError(ErrCacheFailed, "failed to write cache file", err)
	}

	return nil
}

// FilterCached 过滤出已缓存和未缓存的文本块
func (c *TranslationCache) FilterCached(blocks []TextBlock) (cached []TranslatedBlock, uncached []TextBlock) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached = make([]TranslatedBlock, 0)
	uncached = make([]TextBlock, 0)

	for _, block := range blocks {
		hash := c.ComputeHash(block.Text)
		if entry, ok := c.cache[hash]; ok {
			// Block is cached
			cached = append(cached, TranslatedBlock{
				TextBlock:      block,
				TranslatedText: entry.Translation,
				FromCache:      true,
			})
		} else {
			// Block is not cached
			uncached = append(uncached, block)
		}
	}

	return cached, uncached
}

// Size 返回缓存中的条目数量
func (c *TranslationCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear 清空缓存
func (c *TranslationCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]CacheEntry)
}

// GetCachePath 返回缓存文件路径
func (c *TranslationCache) GetCachePath() string {
	return c.cachePath
}

// SetCachePath 设置缓存文件路径
func (c *TranslationCache) SetCachePath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cachePath = path
}
