// Package pdf provides BabelDOC-style PDF translation integration.
// This file provides high-level integration functions for the BabelDOC translator.
package pdf

import (
	"context"
	"fmt"
	"sync"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

// BabelDocPDFTranslator is a high-level PDF translator using BabelDOC approach.
// It combines text extraction, translation, and PDF generation in pure Go.
type BabelDocPDFTranslator struct {
	ctx            context.Context
	cancel         context.CancelFunc
	config         *types.Config
	translator     *BabelDocTranslator
	batchTranslator *BatchTranslator
	cache          *TranslationCache
	workDir        string
	mu             sync.RWMutex
}

// BabelDocPDFTranslatorConfig holds configuration for BabelDocPDFTranslator
type BabelDocPDFTranslatorConfig struct {
	Config    *types.Config
	WorkDir   string
	CachePath string
	FontPath  string // Optional: path to Chinese font
}

// NewBabelDocPDFTranslator creates a new BabelDOC-style PDF translator
func NewBabelDocPDFTranslator(cfg BabelDocPDFTranslatorConfig) *BabelDocPDFTranslator {
	ctx, cancel := context.WithCancel(context.Background())

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "."
	}

	cachePath := cfg.CachePath
	if cachePath == "" {
		cachePath = workDir + "/babeldoc_cache.json"
	}

	// Create BabelDoc translator
	translator := NewBabelDocTranslator(BabelDocConfig{
		WorkDir:  workDir,
		FontPath: cfg.FontPath,
	})

	// Create batch translator for API calls
	var batchTranslator *BatchTranslator
	if cfg.Config != nil {
		batchTranslator = NewBatchTranslator(BatchTranslatorConfig{
			APIKey:        cfg.Config.OpenAIAPIKey,
			BaseURL:       cfg.Config.OpenAIBaseURL,
			Model:         cfg.Config.OpenAIModel,
			ContextWindow: cfg.Config.ContextWindow,
			Concurrency:   cfg.Config.Concurrency,
		})
	}

	cache := NewTranslationCache(cachePath)

	return &BabelDocPDFTranslator{
		ctx:             ctx,
		cancel:          cancel,
		config:          cfg.Config,
		translator:      translator,
		batchTranslator: batchTranslator,
		cache:           cache,
		workDir:         workDir,
	}
}

// TranslateResult holds the result of a BabelDOC translation
type TranslateResult struct {
	InputPath      string `json:"input_path"`
	OutputPath     string `json:"output_path"`
	TotalBlocks    int    `json:"total_blocks"`
	TranslatedBlocks int  `json:"translated_blocks"`
	CachedBlocks   int    `json:"cached_blocks"`
	FormulaBlocks  int    `json:"formula_blocks"`
}

// TranslatePDF translates a PDF file using BabelDOC approach.
// This is the main entry point for PDF translation.
// Flow: Python extract → Go translate → Python write
func (t *BabelDocPDFTranslator) TranslatePDF(inputPath, outputPath string, progressCallback func(phase string, progress int)) (*TranslateResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	logger.Info("starting BabelDOC PDF translation",
		logger.String("input", inputPath),
		logger.String("output", outputPath))

	// Check if we have API config
	if t.config == nil || t.config.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("API configuration is required for PDF translation")
	}

	// Phase 1: Python extracts text blocks
	if progressCallback != nil {
		progressCallback("extracting", 10)
	}
	fmt.Println("Phase 1: Python 提取文本...")

	pyBlocks, err := t.translator.ExtractTextWithPython(inputPath)
	if err != nil {
		return nil, fmt.Errorf("Python extraction failed: %w", err)
	}

	logger.Info("extracted blocks", logger.Int("count", len(pyBlocks)))

	// Count translatable blocks
	translatableCount := 0
	for _, b := range pyBlocks {
		if b.Translatable {
			translatableCount++
		}
	}
	fmt.Printf("Total blocks: %d, Translatable: %d\n", len(pyBlocks), translatableCount)

	// Phase 2: Go translates text blocks
	if progressCallback != nil {
		progressCallback("translating", 20)
	}
	fmt.Println("Phase 2: Go 翻译文本...")

	// Load cache
	if err := t.cache.Load(); err != nil {
		logger.Warn("failed to load cache", logger.Err(err))
	}

	// Translate blocks
	translatedCount := 0
	cachedCount := 0
	
	for i := range pyBlocks {
		if !pyBlocks[i].Translatable {
			continue
		}

		text := pyBlocks[i].Text

		// Check cache first
		if cached, ok := t.cache.Get(text); ok && cached != "" {
			pyBlocks[i].TranslatedText = cached
			cachedCount++
			translatedCount++
			continue
		}

		// Translate via API
		if t.batchTranslator != nil {
			// Create a single-block slice for translation
			textBlock := TextBlock{
				ID:   pyBlocks[i].ID,
				Text: text,
			}
			
			translated, err := t.batchTranslator.TranslateBatch([]TextBlock{textBlock})
			if err != nil {
				logger.Warn("translation failed for block", logger.String("id", pyBlocks[i].ID), logger.Err(err))
				continue
			}
			
			if len(translated) > 0 && translated[0].TranslatedText != "" {
				pyBlocks[i].TranslatedText = translated[0].TranslatedText
				t.cache.Set(text, translated[0].TranslatedText)
				translatedCount++
			}
		}

		// Update progress
		if progressCallback != nil {
			progress := 20 + (translatedCount * 60 / translatableCount)
			progressCallback("translating", progress)
		}
	}

	// Save cache
	if err := t.cache.Save(); err != nil {
		logger.Warn("failed to save cache", logger.Err(err))
	}

	fmt.Printf("Translated: %d, From cache: %d\n", translatedCount, cachedCount)

	// Phase 3: Python writes translations
	if progressCallback != nil {
		progressCallback("generating", 85)
	}
	fmt.Println("Phase 3: Python 写入翻译...")

	if err := t.translator.WriteTranslationsWithPython(inputPath, pyBlocks, outputPath); err != nil {
		return nil, fmt.Errorf("Python write failed: %w", err)
	}

	if progressCallback != nil {
		progressCallback("complete", 100)
	}

	result := &TranslateResult{
		InputPath:        inputPath,
		OutputPath:       outputPath,
		TotalBlocks:      len(pyBlocks),
		TranslatedBlocks: translatedCount - cachedCount,
		CachedBlocks:     cachedCount,
		FormulaBlocks:    len(pyBlocks) - translatableCount,
	}

	logger.Info("BabelDOC translation complete",
		logger.Int("total", result.TotalBlocks),
		logger.Int("translated", result.TranslatedBlocks),
		logger.Int("cached", result.CachedBlocks))

	return result, nil
}

// ExtractBlocksOnly extracts text blocks without translation.
// Useful for previewing what will be translated.
func (t *BabelDocPDFTranslator) ExtractBlocksOnly(inputPath string) ([]BabelDocBlock, error) {
	return t.translator.ExtractBlocks(inputPath)
}

// UpdateConfig updates the translator configuration
func (t *BabelDocPDFTranslator) UpdateConfig(config *types.Config) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.config = config
	if config != nil {
		t.batchTranslator = NewBatchTranslator(BatchTranslatorConfig{
			APIKey:        config.OpenAIAPIKey,
			BaseURL:       config.OpenAIBaseURL,
			Model:         config.OpenAIModel,
			ContextWindow: config.ContextWindow,
			Concurrency:   config.Concurrency,
		})
	}
}

// Cancel cancels any ongoing translation
func (t *BabelDocPDFTranslator) Cancel() {
	t.cancel()
	// Create new context for future operations
	t.ctx, t.cancel = context.WithCancel(context.Background())
}

// Close cleans up resources
func (t *BabelDocPDFTranslator) Close() error {
	t.cancel()
	return t.cache.Save()
}
