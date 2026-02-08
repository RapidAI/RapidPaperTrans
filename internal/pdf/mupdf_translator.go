//go:build mupdf && cgo

// Package pdf provides MuPDF-based PDF translation.
package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"latex-translator/internal/logger"
	"latex-translator/internal/mupdf"
	"latex-translator/internal/types"
)

// MuPDFTranslator translates PDFs using native MuPDF bindings.
// This provides the same quality as PyMuPDF but in pure Go.
type MuPDFTranslator struct {
	config    *types.Config
	cache     *TranslationCache
	workDir   string
	fontPath  string
}

// MuPDFTranslatorConfig holds configuration
type MuPDFTranslatorConfig struct {
	Config    *types.Config
	WorkDir   string
	CachePath string
	FontPath  string // Path to Chinese font (TTF/OTF)
}

// NewMuPDFTranslator creates a new MuPDF-based translator
func NewMuPDFTranslator(cfg MuPDFTranslatorConfig) *MuPDFTranslator {
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "."
	}

	cachePath := cfg.CachePath
	if cachePath == "" {
		cachePath = filepath.Join(workDir, "mupdf_cache.json")
	}

	// Find Chinese font if not specified
	fontPath := cfg.FontPath
	if fontPath == "" {
		fontPath = findChineseFont()
	}

	return &MuPDFTranslator{
		config:   cfg.Config,
		cache:    NewTranslationCache(cachePath),
		workDir:  workDir,
		fontPath: fontPath,
	}
}

// findChineseFont finds a suitable Chinese font on the system
func findChineseFont() string {
	paths := []string{
		"C:/Windows/Fonts/msyh.ttc",    // Microsoft YaHei
		"C:/Windows/Fonts/simsun.ttc",  // SimSun
		"C:/Windows/Fonts/simhei.ttf",  // SimHei
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TranslatePDF translates a PDF file using MuPDF
func (t *MuPDFTranslator) TranslatePDF(inputPath, outputPath string, progressCallback func(phase string, progress int)) (*TranslateResult, error) {
	logger.Info("starting MuPDF PDF translation",
		logger.String("input", inputPath),
		logger.String("output", outputPath))

	// Create MuPDF context
	ctx, err := mupdf.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create MuPDF context: %w", err)
	}
	defer ctx.Close()

	// Open document for reading
	doc, err := ctx.OpenDocument(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer doc.Close()

	// Load cache
	if err := t.cache.Load(); err != nil {
		logger.Warn("failed to load cache", logger.Err(err))
	}

	// Extract all text blocks
	if progressCallback != nil {
		progressCallback("extracting", 10)
	}

	var allBlocks []mupdf.TextBlock
	pageCount := doc.PageCount()
	
	for pageNum := 0; pageNum < pageCount; pageNum++ {
		blocks, err := doc.ExtractTextBlocks(pageNum)
		if err != nil {
			logger.Warn("failed to extract blocks from page",
				logger.Int("page", pageNum),
				logger.Err(err))
			continue
		}
		
		for i := range blocks {
			blocks[i].Page = pageNum
		}
		allBlocks = append(allBlocks, blocks...)
	}

	logger.Info("extracted blocks", logger.Int("count", len(allBlocks)))

	// Filter translatable blocks
	var translatableBlocks []mupdf.TextBlock
	formulaCount := 0
	
	for _, block := range allBlocks {
		if !shouldTranslateBlock(block.Text) {
			if isFormula(block.Text) {
				formulaCount++
			}
			continue
		}
		translatableBlocks = append(translatableBlocks, block)
	}

	logger.Info("filtered blocks",
		logger.Int("translatable", len(translatableBlocks)),
		logger.Int("formulas", formulaCount))

	// Translate blocks
	if progressCallback != nil {
		progressCallback("translating", 30)
	}

	translations := make(map[int]string) // block index -> translation
	cachedCount := 0
	translatedCount := 0

	// Check cache first
	var uncachedBlocks []mupdf.TextBlock
	var uncachedIndices []int
	
	for i, block := range translatableBlocks {
		if cached, ok := t.cache.Get(block.Text); ok {
			translations[i] = cached
			cachedCount++
		} else {
			uncachedBlocks = append(uncachedBlocks, block)
			uncachedIndices = append(uncachedIndices, i)
		}
	}

	// Translate uncached blocks using batch translator
	if len(uncachedBlocks) > 0 {
		batchTranslator := NewBatchTranslator(BatchTranslatorConfig{
			APIKey:        t.config.OpenAIAPIKey,
			BaseURL:       t.config.OpenAIBaseURL,
			Model:         t.config.OpenAIModel,
			ContextWindow: t.config.ContextWindow,
			Concurrency:   t.config.Concurrency,
		})

		// Convert to TextBlock for batch translator
		textBlocks := make([]TextBlock, len(uncachedBlocks))
		for i, b := range uncachedBlocks {
			textBlocks[i] = TextBlock{
				ID:   fmt.Sprintf("block_%d", uncachedIndices[i]),
				Text: b.Text,
			}
		}

		// Translate with progress
		translatedBlocks, err := batchTranslator.TranslateWithRetryAndProgress(
			textBlocks,
			DefaultMaxRetries,
			func(completed, total int) {
				if progressCallback != nil {
					progress := 30 + (completed * 50 / total)
					progressCallback("translating", progress)
				}
			},
		)
		if err != nil {
			logger.Warn("batch translation failed", logger.Err(err))
		}

		// Store translations
		for j, tb := range translatedBlocks {
			if tb.TranslatedText != "" {
				idx := uncachedIndices[j]
				translations[idx] = tb.TranslatedText
				t.cache.Set(uncachedBlocks[j].Text, tb.TranslatedText)
				translatedCount++
			}
		}
	}

	// Save cache
	if err := t.cache.Save(); err != nil {
		logger.Warn("failed to save cache", logger.Err(err))
	}

	// Generate output PDF
	if progressCallback != nil {
		progressCallback("generating", 85)
	}

	// Open original for modification (we'll save to a temp file first)
	pdfDoc, err := ctx.OpenPDFDocument(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF for modification: %w", err)
	}

	// Apply translations
	for i, block := range translatableBlocks {
		translated, ok := translations[i]
		if !ok || translated == "" {
			continue
		}

		// Cover original text with white rectangle
		if err := pdfDoc.AddRect(block.Page, block.X, block.Y, block.Width, block.Height, 1, 1, 1); err != nil {
			logger.Warn("failed to add rect",
				logger.Int("page", block.Page),
				logger.Err(err))
			continue
		}

		// Add translated text
		fontSize := block.FontSize * 0.85 // Slightly smaller for Chinese
		if fontSize < 6 {
			fontSize = 6
		}
		
		textY := block.Y + block.Height - fontSize
		if err := pdfDoc.AddText(block.Page, translated, block.X, textY, fontSize, t.fontPath); err != nil {
			logger.Warn("failed to add text",
				logger.Int("page", block.Page),
				logger.Err(err))
		}
	}

	// Save to output path
	if err := pdfDoc.Save(outputPath); err != nil {
		pdfDoc.Close()
		return nil, fmt.Errorf("failed to save PDF: %w", err)
	}
	pdfDoc.Close()

	if progressCallback != nil {
		progressCallback("complete", 100)
	}

	result := &TranslateResult{
		InputPath:        inputPath,
		OutputPath:       outputPath,
		TotalBlocks:      len(allBlocks),
		TranslatedBlocks: translatedCount,
		CachedBlocks:     cachedCount,
		FormulaBlocks:    formulaCount,
	}

	logger.Info("MuPDF translation complete",
		logger.Int("total", result.TotalBlocks),
		logger.Int("translated", result.TranslatedBlocks),
		logger.Int("cached", result.CachedBlocks))

	return result, nil
}

// shouldTranslateBlock determines if a text block should be translated
func shouldTranslateBlock(text string) bool {
	text = strings.TrimSpace(text)
	
	if len(text) < 3 {
		return false
	}

	// Skip line numbers
	if isLineNumbers(text) {
		return false
	}

	// Skip formulas
	if isFormula(text) {
		return false
	}

	// Skip if mostly numbers/symbols
	alphaCount := 0
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			alphaCount++
		}
	}
	if len(text) > 0 && float64(alphaCount)/float64(len(text)) < 0.3 {
		return false
	}

	return true
}

// isLineNumbers checks if text is just line numbers
func isLineNumbers(text string) bool {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 3 {
		return false
	}

	numberLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		isNum := true
		for _, r := range line {
			if r < '0' || r > '9' {
				isNum = false
				break
			}
		}
		if isNum && line != "" {
			numberLines++
		}
	}

	return float64(numberLines)/float64(len(lines)) > 0.7
}

// isFormula checks if text is a mathematical formula
func isFormula(text string) bool {
	mathSymbols := "∫∑∏√∂∇±×÷≤≥≠≈∞∈∉⊂⊃∪∩∧∨¬∀∃αβγδεζηθικλμνξοπρστυφχψω"
	
	mathCount := 0
	for _, r := range text {
		if strings.ContainsRune(mathSymbols, r) || r == '+' || r == '-' || r == '*' || r == '/' || r == '=' || r == '<' || r == '>' || r == '^' || r == '_' {
			mathCount++
		}
	}

	if len(text) > 0 && float64(mathCount)/float64(len(text)) > 0.25 {
		return true
	}

	if strings.ContainsAny(text, "∫∑∏√∂∇") {
		return true
	}

	return false
}

// copyFile copies a file
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// Close cleans up resources
func (t *MuPDFTranslator) Close() error {
	return t.cache.Save()
}
