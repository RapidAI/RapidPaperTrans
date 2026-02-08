//go:build mupdf && cgo

// Package pdf provides MuPDF-based PDF generation for translation overlay.
// This implementation uses the native MuPDF binding for better quality and performance.
package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"latex-translator/internal/logger"
	"latex-translator/internal/mupdf"
)

// MuPDFGenerator generates translated PDFs using MuPDF library.
// This provides better quality than LaTeX/pdfcpu approaches.
type MuPDFGenerator struct {
	workDir  string
	fontPath string // Path to CJK font for Chinese text
}

// NewMuPDFGenerator creates a new MuPDF-based PDF generator
func NewMuPDFGenerator(workDir string) *MuPDFGenerator {
	return &MuPDFGenerator{
		workDir: workDir,
	}
}

// SetFontPath sets the path to a CJK font file for Chinese text rendering
func (g *MuPDFGenerator) SetFontPath(fontPath string) {
	g.fontPath = fontPath
}

// GenerateTranslatedPDF generates a PDF with translated text overlaid on the original.
// It covers original text with white rectangles and adds translated text on top.
func (g *MuPDFGenerator) GenerateTranslatedPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	logger.Info("generating translated PDF with MuPDF",
		logger.String("input", originalPath),
		logger.String("output", outputPath),
		logger.Int("blocks", len(blocks)))

	// Validate inputs
	if originalPath == "" || outputPath == "" {
		return NewPDFError(ErrGenerateFailed, "input and output paths are required", nil)
	}

	if _, err := os.Stat(originalPath); err != nil {
		return NewPDFError(ErrPDFNotFound, "original PDF not found", err)
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return NewPDFError(ErrGenerateFailed, "cannot create output directory", err)
		}
	}

	// If no blocks, just copy the original
	if len(blocks) == 0 {
		return g.copyFile(originalPath, outputPath)
	}

	// Create MuPDF context
	ctx, err := mupdf.NewContext()
	if err != nil {
		return NewPDFError(ErrGenerateFailed, "failed to create MuPDF context", err)
	}
	defer ctx.Close()

	// Copy original to output first (we'll modify the copy)
	if err := g.copyFile(originalPath, outputPath); err != nil {
		return NewPDFError(ErrGenerateFailed, "failed to copy original PDF", err)
	}

	// Open the copy for modification
	doc, err := ctx.OpenPDFDocument(outputPath)
	if err != nil {
		return NewPDFError(ErrGenerateFailed, "failed to open PDF for modification", err)
	}
	defer doc.Close()

	// Get page count for validation
	pageCount := doc.PageCount()
	logger.Debug("PDF page count", logger.Int("pages", pageCount))

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)

	// Process each page
	for pageNum, pageBlockList := range pageBlocks {
		// MuPDF uses 0-based page numbers
		mupdfPageNum := pageNum - 1
		if mupdfPageNum < 0 || mupdfPageNum >= pageCount {
			logger.Warn("skipping invalid page number",
				logger.Int("page", pageNum),
				logger.Int("totalPages", pageCount))
			continue
		}

		if err := g.processPage(doc, mupdfPageNum, pageBlockList); err != nil {
			logger.Warn("failed to process page",
				logger.Int("page", pageNum),
				logger.Err(err))
			// Continue with other pages
		}
	}

	// Save the modified document
	if err := doc.Save(outputPath); err != nil {
		return NewPDFError(ErrGenerateFailed, "failed to save translated PDF", err)
	}

	// Verify output
	fileInfo, err := os.Stat(outputPath)
	if err != nil || fileInfo.Size() == 0 {
		return NewPDFError(ErrGenerateFailed, "generated PDF is invalid", err)
	}

	logger.Info("translated PDF generated successfully with MuPDF",
		logger.String("output", outputPath),
		logger.Int64("size", fileInfo.Size()))

	return nil
}

// processPage processes a single page, adding translated text overlays
func (g *MuPDFGenerator) processPage(doc *mupdf.PDFDocument, pageNum int, blocks []TranslatedBlock) error {
	for _, block := range blocks {
		if block.TranslatedText == "" {
			continue
		}

		// Skip formula blocks
		if block.BlockType == "formula" {
			continue
		}

		// Calculate dimensions
		x := block.X
		y := block.Y
		w := block.Width
		h := block.Height

		// Ensure minimum dimensions
		if w < 10 {
			w = float64(len(block.TranslatedText)) * block.FontSize * 0.6
		}
		if h < block.FontSize {
			h = block.FontSize * 1.5
		}

		// Adjust font size for Chinese text
		fontSize := g.adjustFontSizeForChinese(block.TranslatedText, block.FontSize, w)

		// Add white rectangle to cover original text
		// Add some padding
		padding := 2.0
		if err := doc.AddRect(pageNum, x-padding, y-padding, w+padding*2, h+padding*2, 1.0, 1.0, 1.0); err != nil {
			logger.Warn("failed to add cover rectangle",
				logger.Int("page", pageNum),
				logger.Err(err))
			continue
		}

		// Add translated text
		// Note: MuPDF Y coordinate is from bottom, adjust for text baseline
		textY := y + h - fontSize
		
		// For now, use ASCII-safe text (MuPDF base14 fonts don't support CJK)
		// TODO: Add CJK font embedding support
		displayText := g.toASCIISafe(block.TranslatedText)
		
		if err := doc.AddText(pageNum, displayText, x, textY, fontSize, ""); err != nil {
			logger.Warn("failed to add translated text",
				logger.Int("page", pageNum),
				logger.Err(err))
		}
	}

	return nil
}

// groupBlocksByPage groups translated blocks by page number
func (g *MuPDFGenerator) groupBlocksByPage(blocks []TranslatedBlock) map[int][]TranslatedBlock {
	pageBlocks := make(map[int][]TranslatedBlock)
	for _, block := range blocks {
		pageBlocks[block.Page] = append(pageBlocks[block.Page], block)
	}
	return pageBlocks
}

// adjustFontSizeForChinese adjusts font size for Chinese text to fit width
func (g *MuPDFGenerator) adjustFontSizeForChinese(text string, originalSize, width float64) float64 {
	// Count Chinese characters
	chineseCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}

	if chineseCount == 0 {
		return originalSize
	}

	// Chinese characters are wider, estimate required width
	estimatedWidth := float64(chineseCount) * originalSize

	// If text is too wide, reduce font size
	if estimatedWidth > width && width > 0 {
		ratio := width / estimatedWidth
		adjustedSize := originalSize * ratio * 0.9
		if adjustedSize < 6 {
			adjustedSize = 6
		}
		return adjustedSize
	}

	return originalSize * 0.85
}

// toASCIISafe converts text to ASCII-safe representation
// This is a temporary workaround until CJK font support is added
func (g *MuPDFGenerator) toASCIISafe(text string) string {
	var result strings.Builder
	for _, r := range text {
		if r < 128 {
			result.WriteRune(r)
		} else {
			// Replace non-ASCII with placeholder
			result.WriteString("[?]")
		}
	}
	return result.String()
}

// copyFile copies a file from src to dst
func (g *MuPDFGenerator) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

// ExtractTextBlocksWithMuPDF extracts text blocks from PDF using MuPDF
// This provides more accurate extraction than ledongthuc/pdf
func ExtractTextBlocksWithMuPDF(pdfPath string) ([]TextBlock, error) {
	ctx, err := mupdf.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create MuPDF context: %w", err)
	}
	defer ctx.Close()

	doc, err := ctx.OpenDocument(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open document: %w", err)
	}
	defer doc.Close()

	var allBlocks []TextBlock
	blockID := 0

	pageCount := doc.PageCount()
	for pageNum := 0; pageNum < pageCount; pageNum++ {
		mupdfBlocks, err := doc.ExtractTextBlocks(pageNum)
		if err != nil {
			logger.Warn("failed to extract blocks from page",
				logger.Int("page", pageNum),
				logger.Err(err))
			continue
		}

		for _, mb := range mupdfBlocks {
			blockID++
			
			// Determine block type
			blockType := determineBlockTypeFromText(mb.Text, mb.FontSize)

			allBlocks = append(allBlocks, TextBlock{
				ID:        fmt.Sprintf("block_%d", blockID),
				Page:      pageNum + 1, // Convert to 1-based
				Text:      strings.TrimSpace(mb.Text),
				X:         mb.X,
				Y:         mb.Y,
				Width:     mb.Width,
				Height:    mb.Height,
				FontSize:  mb.FontSize,
				BlockType: blockType,
			})
		}
	}

	logger.Info("extracted text blocks with MuPDF",
		logger.Int("totalBlocks", len(allBlocks)),
		logger.Int("totalPages", pageCount))

	return allBlocks, nil
}

// determineBlockTypeFromText determines block type based on text content
func determineBlockTypeFromText(text string, fontSize float64) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "paragraph"
	}

	// Check for formula
	if isMathFormula(text) {
		return "formula"
	}

	// Check for heading
	if isNumberedHeading(text) {
		return "heading"
	}

	// Check for caption
	textLower := strings.ToLower(text)
	if strings.HasPrefix(textLower, "figure") ||
		strings.HasPrefix(textLower, "table") ||
		strings.HasPrefix(textLower, "fig.") {
		return "caption"
	}

	// Large font usually indicates heading
	if fontSize > 14 && len(text) < 100 {
		return "heading"
	}

	return "paragraph"
}

// IsMuPDFAvailable checks if MuPDF is available
func IsMuPDFAvailable() bool {
	return mupdf.IsAvailable()
}
