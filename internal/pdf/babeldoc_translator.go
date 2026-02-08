// Package pdf provides BabelDOC-style PDF translation functionality using pure Go.
// This implementation is inspired by funstory-ai/BabelDOC but uses pdfcpu for PDF manipulation.
package pdf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdftypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"latex-translator/internal/logger"
	"latex-translator/internal/python"
)

// BabelDocTranslator implements BabelDOC-style PDF translation in pure Go.
// It extracts text with position info, translates it, and overlays translations.
//
// Architecture (similar to BabelDOC):
// 1. PDF Parsing: Extract text blocks with position, font, and style info
// 2. Layout Analysis: Group text into paragraphs, detect formulas
// 3. Translation: Call LLM API to translate text
// 4. PDF Generation: Overlay translated text on original PDF
type BabelDocTranslator struct {
	workDir    string
	fontPath   string // Path to Chinese font (TTF/OTF)
	conf       *model.Configuration
}

// BabelDocConfig holds configuration for BabelDocTranslator
type BabelDocConfig struct {
	WorkDir  string
	FontPath string // Optional: path to Chinese font for better rendering
}

// NewBabelDocTranslator creates a new BabelDOC-style translator
func NewBabelDocTranslator(cfg BabelDocConfig) *BabelDocTranslator {
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = os.TempDir()
	}

	return &BabelDocTranslator{
		workDir:  workDir,
		fontPath: cfg.FontPath,
		conf:     model.NewDefaultConfiguration(),
	}
}

// BabelDocBlock represents a text block extracted from PDF (similar to BabelDOC's IL)
type BabelDocBlock struct {
	ID         string   `json:"id"`
	Page       int      `json:"page"`
	Text       string   `json:"text"`
	X          float64  `json:"x"`           // Left position in points
	Y          float64  `json:"y"`           // Bottom position in points
	Width      float64  `json:"width"`       // Width in points
	Height     float64  `json:"height"`      // Height in points
	FontSize   float64  `json:"font_size"`   // Font size in points
	FontName   string   `json:"font_name"`   // Original font name
	IsBold     bool     `json:"is_bold"`
	IsItalic   bool     `json:"is_italic"`
	BlockType  string   `json:"block_type"`  // paragraph, heading, formula, caption, etc.
	LineHeight float64  `json:"line_height"` // Line height for multi-line text
	Chars      []CharInfo `json:"chars,omitempty"` // Character-level info (optional)
}

// CharInfo holds character-level information (similar to BabelDOC's character extraction)
type CharInfo struct {
	Char     rune    `json:"char"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	FontSize float64 `json:"font_size"`
}

// TranslatedBabelDocBlock holds a block with its translation
type TranslatedBabelDocBlock struct {
	BabelDocBlock
	TranslatedText string `json:"translated_text"`
	FromCache      bool   `json:"from_cache"`
}

// ExtractBlocks extracts text blocks from PDF with position information.
// This is similar to BabelDOC's PDF parsing stage.
// It will use MuPDF if available, otherwise falls back to ledongthuc/pdf.
func (t *BabelDocTranslator) ExtractBlocks(pdfPath string) ([]BabelDocBlock, error) {
	logger.Info("extracting blocks from PDF (BabelDOC style)",
		logger.String("path", pdfPath))

	// Check if file exists
	if _, err := os.Stat(pdfPath); err != nil {
		if os.IsNotExist(err) {
			return nil, NewPDFError(ErrPDFNotFound, "PDF file not found", err)
		}
		return nil, NewPDFError(ErrPDFInvalid, "cannot access PDF file", err)
	}

	// Try MuPDF first if available (more accurate extraction)
	if IsMuPDFAvailable() {
		logger.Info("using MuPDF for text extraction")
		textBlocks, err := ExtractTextBlocksWithMuPDF(pdfPath)
		if err == nil && len(textBlocks) > 0 {
			// Convert TextBlock to BabelDocBlock
			blocks := make([]BabelDocBlock, len(textBlocks))
			for i, tb := range textBlocks {
				blocks[i] = BabelDocBlock{
					ID:        tb.ID,
					Page:      tb.Page,
					Text:      tb.Text,
					X:         tb.X,
					Y:         tb.Y,
					Width:     tb.Width,
					Height:    tb.Height,
					FontSize:  tb.FontSize,
					FontName:  tb.FontName,
					IsBold:    tb.IsBold,
					IsItalic:  tb.IsItalic,
					BlockType: tb.BlockType,
				}
			}
			logger.Info("extracted blocks with MuPDF",
				logger.Int("totalBlocks", len(blocks)))
			return blocks, nil
		}
		logger.Warn("MuPDF extraction failed, falling back to ledongthuc/pdf",
			logger.Err(err))
	}

	// Fallback to ledongthuc/pdf
	return t.extractBlocksWithLedongthuc(pdfPath)
}

// extractBlocksWithLedongthuc extracts blocks using ledongthuc/pdf library
func (t *BabelDocTranslator) extractBlocksWithLedongthuc(pdfPath string) ([]BabelDocBlock, error) {
	// Open PDF using ledongthuc/pdf
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return nil, NewPDFError(ErrPDFInvalid, "cannot open PDF file", err)
	}
	defer f.Close()

	var blocks []BabelDocBlock
	blockID := 0

	totalPages := r.NumPage()
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// Try GetTextByRow first (works well for most PDFs)
		rows, err := page.GetTextByRow()
		if err != nil {
			logger.Warn("failed to get text from page",
				logger.Int("page", pageNum),
				logger.Err(err))
			continue
		}

		// Check if row extraction gives good position data
		hasGoodPositions := false
		for _, row := range rows {
			for _, text := range row.Content {
				if text.X > 0 || text.Y > 0 {
					hasGoodPositions = true
					break
				}
			}
			if hasGoodPositions {
				break
			}
		}

		if hasGoodPositions {
			// Use row-based extraction
			for _, row := range rows {
				if len(row.Content) == 0 {
					continue
				}
				block := t.processRow(row, pageNum, &blockID)
				if block != nil {
					blocks = append(blocks, *block)
				}
			}
		} else {
			// Fallback: extract text and create synthetic blocks
			pageBlocks := t.extractBlocksFromPageContent(page, pageNum, &blockID)
			blocks = append(blocks, pageBlocks...)
		}
	}

	// Sort blocks by page and reading order
	t.sortBlocksByReadingOrder(blocks)

	// Re-assign IDs after sorting
	for i := range blocks {
		blocks[i].ID = fmt.Sprintf("block_%d", i+1)
	}

	logger.Info("extracted blocks from PDF",
		logger.Int("totalBlocks", len(blocks)),
		logger.Int("totalPages", totalPages))

	return blocks, nil
}

// extractBlocksFromPageContent extracts blocks by parsing page content directly
// This is a fallback when GetTextByRow doesn't provide good position data
func (t *BabelDocTranslator) extractBlocksFromPageContent(page pdf.Page, pageNum int, blockID *int) []BabelDocBlock {
	var blocks []BabelDocBlock

	// Get plain text content
	content, err := page.GetPlainText(nil)
	if err != nil || content == "" {
		return blocks
	}

	// Get page bounds for position estimation
	mediaBox := page.V.Key("MediaBox")
	pageWidth := 612.0  // Default letter width
	pageHeight := 792.0 // Default letter height
	if mediaBox.Kind() == pdf.Array && mediaBox.Len() >= 4 {
		pageWidth = mediaBox.Index(2).Float64() - mediaBox.Index(0).Float64()
		pageHeight = mediaBox.Index(3).Float64() - mediaBox.Index(1).Float64()
	}

	// Split content into paragraphs
	paragraphs := t.splitIntoParagraphs(content)

	// Create blocks with estimated positions
	// Estimate: typical academic paper has ~50 lines per page, ~80 chars per line
	linesPerPage := 50.0
	lineHeight := pageHeight / linesPerPage
	marginLeft := 72.0  // 1 inch margin
	marginTop := 72.0
	textWidth := pageWidth - marginLeft*2

	currentY := pageHeight - marginTop

	for _, para := range paragraphs {
		if len(strings.TrimSpace(para)) == 0 {
			continue
		}

		// Skip PostScript code
		if t.isPostScriptCode(para) || t.hasExcessiveNonPrintable(para) {
			continue
		}

		// Estimate number of lines for this paragraph
		charsPerLine := 80.0
		numLines := float64(len(para)) / charsPerLine
		if numLines < 1 {
			numLines = 1
		}

		blockHeight := numLines * lineHeight
		fontSize := 10.0

		// Determine block type
		blockType := t.determineBlockType(para, fontSize, false)

		*blockID++
		blocks = append(blocks, BabelDocBlock{
			ID:         fmt.Sprintf("block_%d_%d", pageNum, *blockID),
			Page:       pageNum,
			Text:       strings.TrimSpace(para),
			X:          marginLeft,
			Y:          currentY - blockHeight,
			Width:      textWidth,
			Height:     blockHeight,
			FontSize:   fontSize,
			BlockType:  blockType,
			LineHeight: lineHeight,
		})

		currentY -= blockHeight + lineHeight*0.5 // Add some spacing
	}

	return blocks
}

// splitIntoParagraphs splits text content into paragraphs
func (t *BabelDocTranslator) splitIntoParagraphs(content string) []string {
	// Split by double newlines or significant whitespace
	var paragraphs []string
	var current strings.Builder

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			// Empty line - end current paragraph
			if current.Len() > 0 {
				paragraphs = append(paragraphs, current.String())
				current.Reset()
			}
			continue
		}

		// Check if this looks like a new paragraph start
		isNewParagraph := false
		if i > 0 && current.Len() > 0 {
			// Check for sentence ending followed by capital letter
			prevText := current.String()
			if len(prevText) > 0 {
				lastChar := prevText[len(prevText)-1]
				if (lastChar == '.' || lastChar == '?' || lastChar == '!') && len(trimmed) > 0 {
					firstChar := rune(trimmed[0])
					if firstChar >= 'A' && firstChar <= 'Z' {
						// Could be new paragraph or just new sentence
						// Use heuristic: if line is short, likely new paragraph
						if len(strings.TrimSpace(lines[i-1])) < 60 {
							isNewParagraph = true
						}
					}
				}
			}
		}

		if isNewParagraph && current.Len() > 0 {
			paragraphs = append(paragraphs, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(trimmed)
	}

	// Don't forget the last paragraph
	if current.Len() > 0 {
		paragraphs = append(paragraphs, current.String())
	}

	return paragraphs
}

// processRow processes a row of text and creates a BabelDocBlock
func (t *BabelDocTranslator) processRow(row *pdf.Row, pageNum int, blockID *int) *BabelDocBlock {
	var textBuilder strings.Builder
	var minX, maxX, minY, maxY float64
	var totalFontSize float64
	var fontName string
	var isBold, isItalic bool
	var chars []CharInfo
	first := true
	var lastX float64
	var lastWidth float64

	for _, text := range row.Content {
		if text.S == "" {
			continue
		}

		// Filter out PostScript/PDF operator code
		if t.isPostScriptCode(text.S) {
			continue
		}

		// Add space between words if there's a gap
		if !first && text.X > lastX+lastWidth+2 {
			textBuilder.WriteString(" ")
		}

		textBuilder.WriteString(text.S)
		
		// Estimate width of this text segment
		lastX = text.X
		lastWidth = float64(len(text.S)) * text.FontSize * 0.5

		// Track position bounds
		if first {
			minX = text.X
			maxX = text.X
			minY = text.Y
			maxY = text.Y
			fontName = text.Font
			first = false
		} else {
			if text.X < minX {
				minX = text.X
			}
			if text.X > maxX {
				maxX = text.X
			}
			if text.Y < minY {
				minY = text.Y
			}
			if text.Y > maxY {
				maxY = text.Y
			}
		}

		totalFontSize += text.FontSize

		// Collect character info
		for _, ch := range text.S {
			chars = append(chars, CharInfo{
				Char:     ch,
				X:        text.X,
				Y:        text.Y,
				FontSize: text.FontSize,
			})
		}

		// Detect bold/italic from font name
		fontLower := strings.ToLower(text.Font)
		if strings.Contains(fontLower, "bold") {
			isBold = true
		}
		if strings.Contains(fontLower, "italic") || strings.Contains(fontLower, "oblique") {
			isItalic = true
		}
	}

	textStr := strings.TrimSpace(textBuilder.String())
	if textStr == "" {
		return nil
	}

	// Skip blocks that look like PostScript code
	if t.isPostScriptCode(textStr) || t.hasExcessiveNonPrintable(textStr) {
		return nil
	}

	// Calculate average font size
	avgFontSize := totalFontSize / float64(len(row.Content))
	if avgFontSize <= 0 {
		avgFontSize = 10.0
	}

	// Estimate dimensions
	estimatedWidth := float64(len(textStr)) * avgFontSize * 0.5
	if maxX > minX {
		actualWidth := maxX - minX + avgFontSize
		if actualWidth > estimatedWidth {
			estimatedWidth = actualWidth
		}
	}
	if estimatedWidth < avgFontSize {
		estimatedWidth = avgFontSize * float64(len(textStr)) * 0.5
	}

	estimatedHeight := avgFontSize * 1.2
	if estimatedHeight <= 0 {
		estimatedHeight = 12.0
	}

	// Determine block type
	blockType := t.determineBlockType(textStr, avgFontSize, isBold)

	*blockID++
	return &BabelDocBlock{
		ID:         fmt.Sprintf("block_%d_%d", pageNum, *blockID),
		Page:       pageNum,
		Text:       textStr,
		X:          minX,
		Y:          minY,
		Width:      estimatedWidth,
		Height:     estimatedHeight,
		FontSize:   avgFontSize,
		FontName:   fontName,
		IsBold:     isBold,
		IsItalic:   isItalic,
		BlockType:  blockType,
		LineHeight: avgFontSize * 1.2,
		Chars:      chars,
	}
}

// sortBlocksByReadingOrder sorts blocks by page and reading order (top to bottom, left to right)
func (t *BabelDocTranslator) sortBlocksByReadingOrder(blocks []BabelDocBlock) {
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].Page != blocks[j].Page {
			return blocks[i].Page < blocks[j].Page
		}
		// Higher Y values mean higher on the page in PDF coordinates
		yTolerance := 5.0
		if abs(blocks[i].Y-blocks[j].Y) < yTolerance {
			return blocks[i].X < blocks[j].X
		}
		return blocks[i].Y > blocks[j].Y
	})
}

// GenerateTranslatedPDF generates a PDF with translated text overlaid on the original.
// Priority: 1. Python PyMuPDF (best quality), 2. pdfcpu stamps (fallback)
func (t *BabelDocTranslator) GenerateTranslatedPDF(originalPath string, blocks []TranslatedBabelDocBlock, outputPath string) error {
	logger.Info("generating translated PDF (BabelDOC style)",
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
		return t.copyFile(originalPath, outputPath)
	}

	// Method 1: Try Python PyMuPDF overlay (best quality for Chinese)
	fmt.Println("正在使用 Python + PyMuPDF 方法覆盖文本...")
	if err := t.generatePythonOverlayPDF(originalPath, blocks, outputPath); err == nil {
		// Verify output
		if fileInfo, err := os.Stat(outputPath); err == nil && fileInfo.Size() > 0 {
			logger.Info("translated PDF generated successfully with Python PyMuPDF",
				logger.String("output", outputPath))
			return nil
		}
	} else {
		fmt.Printf("Warning: Python overlay failed: %v\n", err)
	}

	// Method 2: Fallback to pdfcpu stamps
	fmt.Println("回退到 pdfcpu stamp 方法...")
	
	// Copy original to output first
	if err := t.copyFile(originalPath, outputPath); err != nil {
		return NewPDFError(ErrGenerateFailed, "cannot copy original PDF", err)
	}

	// Group blocks by page
	pageBlocks := t.groupBlocksByPage(blocks)

	// Get page dimensions
	pageInfo, err := t.getPageInfo(originalPath)
	if err != nil {
		logger.Warn("cannot get page info, using A4 defaults", logger.Err(err))
	}

	// Apply stamps for each page
	for pageNum, pageBlockList := range pageBlocks {
		if err := t.applyPageStamps(outputPath, pageNum, pageBlockList, pageInfo); err != nil {
			logger.Warn("failed to apply stamps to page",
				logger.Int("page", pageNum),
				logger.Err(err))
		}
	}

	// Verify output
	fileInfo, err := os.Stat(outputPath)
	if err != nil || fileInfo.Size() == 0 {
		return NewPDFError(ErrGenerateFailed, "generated PDF is invalid", err)
	}

	logger.Info("translated PDF generated successfully",
		logger.String("output", outputPath))

	return nil
}

// PageInfo holds page dimension information
type PageInfo struct {
	Width  float64
	Height float64
}

// getPageInfo gets page dimensions from PDF
func (t *BabelDocTranslator) getPageInfo(pdfPath string) (map[int]PageInfo, error) {
	ctx, err := api.ReadContextFile(pdfPath)
	if err != nil {
		return nil, err
	}

	pageInfo := make(map[int]PageInfo)
	for i := 1; i <= ctx.PageCount; i++ {
		// Default to A4 dimensions
		pageInfo[i] = PageInfo{
			Width:  595.276, // A4 width in points
			Height: 841.890, // A4 height in points
		}
	}

	return pageInfo, nil
}

// applyPageStamps applies text stamps to a specific page
func (t *BabelDocTranslator) applyPageStamps(pdfPath string, pageNum int, blocks []TranslatedBabelDocBlock, pageInfo map[int]PageInfo) error {
	// Get page dimensions
	info, ok := pageInfo[pageNum]
	if !ok {
		info = PageInfo{Width: 595.276, Height: 841.890}
	}

	for _, block := range blocks {
		if block.TranslatedText == "" || block.BlockType == "formula" {
			continue
		}

		// Create stamp description for this block
		stampDesc := t.createStampDescription(block, info)

		// Apply stamp using pdfcpu
		pageSelection := []string{fmt.Sprintf("%d", pageNum)}
		wm, err := api.TextWatermark(block.TranslatedText, stampDesc, true, false, pdftypes.POINTS)
		if err != nil {
			logger.Warn("failed to create watermark",
				logger.String("text", truncateString(block.TranslatedText, 50)),
				logger.Err(err))
			continue
		}

		if err := api.AddWatermarksFile(pdfPath, "", pageSelection, wm, nil); err != nil {
			logger.Warn("failed to add watermark",
				logger.Int("page", pageNum),
				logger.Err(err))
		}
	}

	return nil
}

// createStampDescription creates pdfcpu stamp description string
func (t *BabelDocTranslator) createStampDescription(block TranslatedBabelDocBlock, pageInfo PageInfo) string {
	// Calculate position
	// pdfcpu uses position anchors and offsets
	// We need to convert from PDF coordinates (bottom-left origin) to pdfcpu positioning

	// Calculate offset from bottom-left corner
	offsetX := block.X
	offsetY := block.Y

	// Font size adjustment for Chinese
	fontSize := t.adjustFontSizeForChinese(block.TranslatedText, block.FontSize, block.Width)

	// Build description string
	// Format: "pos:bl, off:X Y, points:SIZE, fillc:white, strokec:black, op:1.0"
	desc := fmt.Sprintf("pos:bl, off:%.1f %.1f, points:%.1f, fillc:white, strokec:black, op:1.0, bgcol:white",
		offsetX, offsetY, fontSize)

	// Add font if specified
	if t.fontPath != "" {
		desc += fmt.Sprintf(", fontname:%s", filepath.Base(t.fontPath))
	}

	return desc
}

// adjustFontSizeForChinese adjusts font size for Chinese text
func (t *BabelDocTranslator) adjustFontSizeForChinese(text string, originalSize, width float64) float64 {
	// Chinese characters are typically wider than Latin characters
	// Adjust font size to fit within the original width

	chineseCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}

	if chineseCount == 0 {
		return originalSize
	}

	// Estimate required width for Chinese text
	estimatedWidth := float64(chineseCount) * originalSize

	// If text is too wide, reduce font size
	if estimatedWidth > width && width > 0 {
		ratio := width / estimatedWidth
		adjustedSize := originalSize * ratio * 0.9 // 10% margin
		if adjustedSize < 6 {
			adjustedSize = 6
		}
		return adjustedSize
	}

	return originalSize * 0.85 // Chinese typically needs slightly smaller font
}

// groupBlocksByPage groups blocks by page number
func (t *BabelDocTranslator) groupBlocksByPage(blocks []TranslatedBabelDocBlock) map[int][]TranslatedBabelDocBlock {
	pageBlocks := make(map[int][]TranslatedBabelDocBlock)
	for _, block := range blocks {
		pageBlocks[block.Page] = append(pageBlocks[block.Page], block)
	}
	return pageBlocks
}

// determineBlockType determines the type of text block
func (t *BabelDocTranslator) determineBlockType(text string, fontSize float64, isBold bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "paragraph"
	}

	// Check for mathematical formula first
	if t.isMathFormula(text) {
		return "formula"
	}

	// Check for caption (before heading, as "Fig." could match heading patterns)
	if t.isCaption(text) {
		return "caption"
	}

	// Check for heading
	if t.isHeading(text, fontSize, isBold) {
		return "heading"
	}

	return "paragraph"
}

// isMathFormula checks if text is a mathematical formula
func (t *BabelDocTranslator) isMathFormula(text string) bool {
	mathSymbols := "∫∑∏√∂∇±×÷≤≥≠≈∞∈∉⊂⊃∪∩∧∨¬∀∃αβγδεζηθικλμνξοπρστυφχψω²³⁰¹⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉"

	mathCount := 0
	total := 0
	hasEquals := false
	hasParens := false
	
	for _, r := range text {
		total++
		if strings.ContainsRune(mathSymbols, r) ||
			r == '+' || r == '-' || r == '*' || r == '/' ||
			r == '=' || r == '<' || r == '>' || r == '^' || r == '_' {
			mathCount++
		}
		if r == '=' {
			hasEquals = true
		}
		if r == '(' || r == ')' {
			hasParens = true
		}
	}

	// High ratio of math symbols
	if total > 0 && float64(mathCount)/float64(total) > 0.25 {
		return true
	}

	// Check for common formula patterns
	if strings.ContainsAny(text, "∫∑∏√∂∇") {
		return true
	}

	// Check for superscript/subscript patterns (like x² or E=mc²)
	if strings.ContainsAny(text, "²³⁰¹⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉") {
		return true
	}

	// Pattern like "f(x) = ..." - function notation with equals
	if hasEquals && hasParens && len(text) < 50 {
		// Count letters vs operators
		letterCount := 0
		for _, r := range text {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				letterCount++
			}
		}
		// If mostly single letters with operators, likely a formula
		if letterCount < 10 && mathCount > 2 {
			return true
		}
	}

	return false
}

// isHeading checks if text is a heading
func (t *BabelDocTranslator) isHeading(text string, fontSize float64, isBold bool) bool {
	isShort := len(text) < 100
	isLargeFont := fontSize > 12

	// Check for numbered sections like "1. Background" or "1.1 Introduction"
	textLower := strings.ToLower(text)
	
	// Check for common section keywords
	patterns := []string{"chapter", "section", "appendix", "abstract", "introduction",
		"conclusion", "references", "bibliography", "background", "method", "result", "discussion"}
	for _, pattern := range patterns {
		if strings.HasPrefix(textLower, pattern) || strings.Contains(textLower, " "+pattern) {
			return true
		}
	}

	// Check for numbered patterns like "1.", "1.1", "A."
	if len(text) >= 2 {
		first := text[0]
		if (first >= '0' && first <= '9') || (first >= 'A' && first <= 'Z') {
			// Look for pattern like "1. " or "1.1 "
			for i := 1; i < len(text) && i < 10; i++ {
				if text[i] == '.' || text[i] == ')' {
					if i+1 < len(text) && (text[i+1] == ' ' || text[i+1] == '\t') {
						// Check if remaining text is short (heading-like)
						remaining := strings.TrimSpace(text[i+1:])
						if len(remaining) < 80 && !strings.HasSuffix(remaining, ".") {
							return true
						}
					}
				}
			}
		}
	}

	if isBold && isShort && isLargeFont {
		return true
	}

	return false
}

// isCaption checks if text is a caption
func (t *BabelDocTranslator) isCaption(text string) bool {
	textLower := strings.ToLower(text)
	prefixes := []string{"figure", "table", "fig.", "tab.", "图", "表"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(textLower, prefix) {
			return true
		}
	}
	return false
}

// isPostScriptCode checks if text looks like PostScript code
func (t *BabelDocTranslator) isPostScriptCode(text string) bool {
	if len(text) == 0 {
		return false
	}

	textLower := strings.ToLower(text)

	// Check for PostScript patterns
	if strings.Contains(text, " def ") || strings.HasSuffix(text, " def") {
		if strings.Contains(text, "/") {
			return true
		}
	}

	psPatterns := []string{"currentpoint", "gsave", "grestore", "newpath", "closepath",
		"setrgbcolor", "setgray", "setlinewidth", "showpage", "moveto", "lineto"}
	for _, pattern := range psPatterns {
		if strings.Contains(textLower, pattern) {
			return true
		}
	}

	return false
}

// hasExcessiveNonPrintable checks for non-printable characters
func (t *BabelDocTranslator) hasExcessiveNonPrintable(text string) bool {
	if len(text) == 0 {
		return false
	}

	nonPrintable := 0
	for _, r := range text {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			nonPrintable++
		}
		if r >= 0x7F && r <= 0x9F {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(len(text)) > 0.1
}

// copyFile copies a file from src to dst
func (t *BabelDocTranslator) copyFile(src, dst string) error {
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

// generatePythonOverlayPDF generates a PDF with Chinese text overlaid using Python PyMuPDF
func (t *BabelDocTranslator) generatePythonOverlayPDF(originalPath string, blocks []TranslatedBabelDocBlock, outputPath string) error {
	// Ensure Python environment is set up with required packages
	fmt.Println("正在检查 Python 环境...")
	if err := python.EnsureGlobalEnv(func(msg string) {
		fmt.Println(msg)
	}); err != nil {
		return fmt.Errorf("failed to setup Python environment: %w", err)
	}

	// Get Python path from our managed environment
	pythonPath := python.GetPythonPath()
	if pythonPath == "" {
		return fmt.Errorf("Python environment not ready")
	}

	// Find Python script
	possiblePaths := []string{
		filepath.Join("internal", "pdf", "overlay_pdf.py"),
		filepath.Join("scripts", "pdf_translate.py"),
	}
	
	scriptPath := ""
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			scriptPath = path
			break
		}
	}
	
	if scriptPath == "" {
		return fmt.Errorf("Python overlay script not found")
	}
	
	// Create blocks JSON file
	blocksFile, err := t.createBlocksJSONFile(blocks)
	if err != nil {
		return fmt.Errorf("failed to create blocks JSON file: %v", err)
	}
	
	// Debug: also save to output directory
	debugFile := filepath.Join(t.workDir, "debug_blocks.json")
	if data, err := os.ReadFile(blocksFile); err == nil {
		os.WriteFile(debugFile, data, 0644)
	}
	
	defer os.Remove(blocksFile)
	
	// Run Python script using our managed Python environment
	cmd := exec.Command(pythonPath, scriptPath, originalPath, blocksFile, outputPath)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Python overlay failed: %v\nOutput: %s", err, string(output))
	}
	
	fmt.Printf("Python overlay output: %s\n", string(output))
	return nil
}

// createBlocksJSONFile creates a temporary JSON file with block data for Python script
func (t *BabelDocTranslator) createBlocksJSONFile(blocks []TranslatedBabelDocBlock) (string, error) {
	type BlockJSON struct {
		Page           int     `json:"page"`
		X              float64 `json:"x"`
		Y              float64 `json:"y"`
		Width          float64 `json:"width"`
		Height         float64 `json:"height"`
		FontSize       float64 `json:"font_size"`
		BlockType      string  `json:"block_type"`
		Text           string  `json:"text"`
		TranslatedText string  `json:"translated_text"`
	}
	
	jsonBlocks := make([]BlockJSON, 0, len(blocks))
	for _, block := range blocks {
		// Skip blocks without translation
		if block.TranslatedText == "" {
			continue
		}
		// Skip formula blocks - they should not be translated/overlaid
		if block.BlockType == "formula" {
			continue
		}
		jsonBlocks = append(jsonBlocks, BlockJSON{
			Page:           block.Page,
			X:              block.X,
			Y:              block.Y,
			Width:          block.Width,
			Height:         block.Height,
			FontSize:       block.FontSize,
			BlockType:      block.BlockType,
			Text:           block.Text,
			TranslatedText: block.TranslatedText,
		})
	}
	
	// Create temp file
	tmpFile, err := os.CreateTemp("", "blocks_*.json")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	
	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonBlocks); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	
	return tmpFile.Name(), nil
}

// ConvertToTranslatedBlocks converts BabelDocBlocks to TranslatedBabelDocBlocks
func ConvertToTranslatedBlocks(blocks []BabelDocBlock, translations map[string]string) []TranslatedBabelDocBlock {
	result := make([]TranslatedBabelDocBlock, len(blocks))
	for i, block := range blocks {
		result[i] = TranslatedBabelDocBlock{
			BabelDocBlock:  block,
			TranslatedText: translations[block.ID],
		}
	}
	return result
}

// FilterTranslatableBlocks filters out blocks that shouldn't be translated (formulas, etc.)
func FilterTranslatableBlocks(blocks []BabelDocBlock) []BabelDocBlock {
	var result []BabelDocBlock
	for _, block := range blocks {
		// Skip formulas
		if block.BlockType == "formula" {
			continue
		}
		
		// Skip empty blocks
		if len(strings.TrimSpace(block.Text)) == 0 {
			continue
		}
		
		// Skip blocks with invalid positions (likely metadata or artifacts)
		if block.X == 0 && block.Y == 0 && block.Width > 500 {
			continue
		}
		
		// Skip blocks with negative Y that are too far below page
		if block.Y < -100 {
			continue
		}
		
		// Skip very short blocks that are likely page numbers or artifacts
		if len(block.Text) < 3 && block.Width < 20 {
			continue
		}
		
		result = append(result, block)
	}
	return result
}

// Helper function
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Note: abs and isAllUpperCase are defined in parser.go

// PageCountResult 页数检测结果
type PageCountResult struct {
	OriginalPages   int     `json:"original_pages"`
	TranslatedPages int     `json:"translated_pages"`
	Difference      int     `json:"difference"`       // 差异页数
	DiffPercent     float64 `json:"diff_percent"`     // 差异百分比
	IsSuspicious    bool    `json:"is_suspicious"`    // 是否可疑（差异超过15%）
}

// PageCountThreshold 页数差异阈值（15%）
const PageCountThreshold = 0.15

// GetPDFPageCount 获取 PDF 文件的页数
func (t *BabelDocTranslator) GetPDFPageCount(pdfPath string) (int, error) {
	if _, err := os.Stat(pdfPath); err != nil {
		return 0, NewPDFError(ErrPDFNotFound, "PDF file not found", err)
	}

	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return 0, NewPDFError(ErrPDFInvalid, "cannot open PDF file", err)
	}
	defer f.Close()

	return r.NumPage(), nil
}

// CheckPageCountDifference 检查翻译前后的页数差异
// 如果翻译后页数比原始页数少超过15%，返回可疑结果
func (t *BabelDocTranslator) CheckPageCountDifference(originalPath, translatedPath string) (*PageCountResult, error) {
	originalPages, err := t.GetPDFPageCount(originalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get original PDF page count: %w", err)
	}

	translatedPages, err := t.GetPDFPageCount(translatedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get translated PDF page count: %w", err)
	}

	result := &PageCountResult{
		OriginalPages:   originalPages,
		TranslatedPages: translatedPages,
		Difference:      originalPages - translatedPages,
	}

	// 计算差异百分比（相对于原始页数）
	if originalPages > 0 {
		result.DiffPercent = float64(result.Difference) / float64(originalPages)
	}

	// 如果翻译后页数少于原始页数超过15%，标记为可疑
	result.IsSuspicious = result.DiffPercent > PageCountThreshold

	if result.IsSuspicious {
		logger.Warn("suspicious page count difference detected",
			logger.Int("originalPages", originalPages),
			logger.Int("translatedPages", translatedPages),
			logger.Float64("diffPercent", result.DiffPercent*100))
	}

	return result, nil
}

// FormatPageCountError 格式化页数差异错误信息
func FormatPageCountError(result *PageCountResult) string {
	return fmt.Sprintf("翻译后页数(%d)比原始页数(%d)少%.1f%%，超过15%%阈值，可能存在内容丢失",
		result.TranslatedPages, result.OriginalPages, result.DiffPercent*100)
}

// PyExtractedBlock represents a text block extracted by Python
type PyExtractedBlock struct {
	ID           string  `json:"id"`
	Page         int     `json:"page"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Text         string  `json:"text"`
	BlockType    string  `json:"block_type"`
	Translatable bool    `json:"translatable"`
	Rotation     int     `json:"rotation,omitempty"`
	TranslatedText string `json:"translated_text,omitempty"`
}

// ExtractTextWithPython extracts text blocks from PDF using Python PyMuPDF
func (t *BabelDocTranslator) ExtractTextWithPython(pdfPath string) ([]PyExtractedBlock, error) {
	logger.Info("extracting text with Python", logger.String("path", pdfPath))

	// Ensure Python environment
	if err := python.EnsureGlobalEnv(nil); err != nil {
		return nil, fmt.Errorf("failed to setup Python: %w", err)
	}

	pythonPath := python.GetPythonPath()
	if pythonPath == "" {
		return nil, fmt.Errorf("Python not ready")
	}

	// Write embedded script to temp file
	scriptFile := filepath.Join(t.workDir, "extract_text.py")
	if err := os.WriteFile(scriptFile, []byte(ExtractTextPyScript), 0644); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}

	// Create temp file for output
	outputFile := filepath.Join(t.workDir, "extracted_blocks.json")

	// Run extraction
	cmd := exec.Command(pythonPath, scriptFile, pdfPath, outputFile)
	hideWindowOnWindows(cmd) // Hide console window on Windows
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("Extract output: %s\n", string(output))

	// Read extracted blocks
	data, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read extracted blocks: %w", err)
	}

	var blocks []PyExtractedBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		return nil, fmt.Errorf("failed to parse extracted blocks: %w", err)
	}

	logger.Info("extracted blocks with Python", logger.Int("count", len(blocks)))
	return blocks, nil
}

// WriteTranslationsWithPython writes translated blocks to PDF using Python PyMuPDF
func (t *BabelDocTranslator) WriteTranslationsWithPython(inputPDF string, blocks []PyExtractedBlock, outputPDF string) error {
	logger.Info("writing translations with Python",
		logger.String("input", inputPDF),
		logger.String("output", outputPDF),
		logger.Int("blocks", len(blocks)))

	// Ensure Python environment
	if err := python.EnsureGlobalEnv(nil); err != nil {
		return fmt.Errorf("failed to setup Python: %w", err)
	}

	pythonPath := python.GetPythonPath()
	if pythonPath == "" {
		return fmt.Errorf("Python not ready")
	}

	// Write embedded script to temp file
	scriptFile := filepath.Join(t.workDir, "write_translations.py")
	if err := os.WriteFile(scriptFile, []byte(WriteTranslationsPyScript), 0644); err != nil {
		return fmt.Errorf("failed to write script: %w", err)
	}

	// Write blocks to temp JSON
	blocksFile := filepath.Join(t.workDir, "translated_blocks.json")
	data, err := json.MarshalIndent(blocks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal blocks: %w", err)
	}
	if err := os.WriteFile(blocksFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write blocks file: %w", err)
	}

	// Create output directory
	outputDir := filepath.Dir(outputPDF)
	if outputDir != "" && outputDir != "." {
		os.MkdirAll(outputDir, 0755)
	}

	// Run write script
	cmd := exec.Command(pythonPath, scriptFile, inputPDF, blocksFile, outputPDF)
	hideWindowOnWindows(cmd) // Hide console window on Windows
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write failed: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("Write output: %s\n", string(output))

	// Verify output
	if _, err := os.Stat(outputPDF); err != nil {
		return fmt.Errorf("output PDF not created")
	}

	return nil
}

// TranslatePDFWithPython translates a PDF using Python script that does everything in one pass.
// This is the recommended method as it uses PyMuPDF for both extraction and overlay,
// ensuring consistent text matching and font rendering.
//
// Parameters:
//   - inputPath: Path to the input PDF file
//   - outputPath: Path to the output translated PDF file
//   - apiKey: OpenAI API key
//   - baseURL: OpenAI API base URL
//   - model: Model name to use for translation
//   - cachePath: Optional path to cache file (can be empty)
//   - progressCallback: Optional callback for progress updates
//
// Returns error if translation fails.
func (t *BabelDocTranslator) TranslatePDFWithPython(inputPath, outputPath, apiKey, baseURL, model, cachePath string, progressCallback func(message string)) error {
	logger.Info("starting Python-based PDF translation",
		logger.String("input", inputPath),
		logger.String("output", outputPath))

	// Validate inputs
	if inputPath == "" || outputPath == "" {
		return NewPDFError(ErrGenerateFailed, "input and output paths are required", nil)
	}

	if _, err := os.Stat(inputPath); err != nil {
		return NewPDFError(ErrPDFNotFound, "input PDF not found", err)
	}

	if apiKey == "" {
		return NewPDFError(ErrGenerateFailed, "API key is required", nil)
	}

	// Ensure Python environment is set up
	if progressCallback != nil {
		progressCallback("正在检查 Python 环境...")
	}
	if err := python.EnsureGlobalEnv(func(msg string) {
		if progressCallback != nil {
			progressCallback(msg)
		}
	}); err != nil {
		return fmt.Errorf("failed to setup Python environment: %w", err)
	}

	// Get Python path
	pythonPath := python.GetPythonPath()
	if pythonPath == "" {
		return fmt.Errorf("Python environment not ready")
	}

	// Find Python script
	possiblePaths := []string{
		filepath.Join("internal", "pdf", "translate_pdf.py"),
		filepath.Join("scripts", "pdf_translate.py"),
	}

	// Also check relative to executable
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		possiblePaths = append(possiblePaths,
			filepath.Join(exeDir, "internal", "pdf", "translate_pdf.py"),
			filepath.Join(exeDir, "scripts", "pdf_translate.py"),
		)
	}

	scriptPath := ""
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			scriptPath = path
			break
		}
	}

	if scriptPath == "" {
		return fmt.Errorf("Python translate script not found")
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return NewPDFError(ErrGenerateFailed, "cannot create output directory", err)
		}
	}

	// Set default cache path if not provided
	if cachePath == "" {
		cachePath = filepath.Join(t.workDir, "pdf_translation_cache.json")
	}

	// Build command
	args := []string{scriptPath, inputPath, outputPath, cachePath}
	cmd := exec.Command(pythonPath, args...)

	// Set environment variables for API credentials
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+apiKey,
		"OPENAI_BASE_URL="+baseURL,
		"OPENAI_MODEL="+model,
	)

	if progressCallback != nil {
		progressCallback("正在翻译 PDF...")
	}

	// Run the script
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Python translation failed: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("Python translation output:\n%s\n", string(output))

	// Verify output file exists
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("output PDF not created")
	}

	if progressCallback != nil {
		progressCallback("翻译完成")
	}

	logger.Info("Python-based PDF translation completed",
		logger.String("output", outputPath))

	return nil
}

// TranslatePDFWithPdf2zh translates a PDF using pdf2zh (PDFMathTranslate).
// This provides professional-grade PDF translation with formula preservation.
//
// Parameters:
//   - inputPath: Path to the input PDF file
//   - outputPath: Path to the output translated PDF file
//   - apiKey: OpenAI API key
//   - baseURL: OpenAI API base URL
//   - model: Model name to use for translation
//   - progressCallback: Optional callback for progress updates
//
// Returns error if translation fails.
func (t *BabelDocTranslator) TranslatePDFWithPdf2zh(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string)) error {
	logger.Info("starting pdf2zh PDF translation",
		logger.String("input", inputPath),
		logger.String("output", outputPath))

	// Validate inputs
	if inputPath == "" || outputPath == "" {
		return NewPDFError(ErrGenerateFailed, "input and output paths are required", nil)
	}

	if _, err := os.Stat(inputPath); err != nil {
		return NewPDFError(ErrPDFNotFound, "input PDF not found", err)
	}

	// Ensure Python environment is set up
	if progressCallback != nil {
		progressCallback("正在检查 Python 环境...")
	}
	if err := python.EnsureGlobalEnv(func(msg string) {
		if progressCallback != nil {
			progressCallback(msg)
		}
	}); err != nil {
		return fmt.Errorf("failed to setup Python environment: %w", err)
	}

	// Get Python path
	pythonPath := python.GetPythonPath()
	if pythonPath == "" {
		return fmt.Errorf("Python environment not ready")
	}

	// Write embedded script to temp file
	scriptFile := filepath.Join(t.workDir, "pdf2zh_translate.py")
	if err := os.WriteFile(scriptFile, []byte(Pdf2zhTranslateScript), 0644); err != nil {
		return fmt.Errorf("failed to write script: %w", err)
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return NewPDFError(ErrGenerateFailed, "cannot create output directory", err)
		}
	}

	// Build command - use openai service
	args := []string{scriptFile, inputPath, outputPath, "openai"}
	cmd := exec.Command(pythonPath, args...)
	hideWindowOnWindows(cmd)

	// Set environment variables for API credentials
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+apiKey,
		"OPENAI_BASE_URL="+baseURL,
		"OPENAI_MODEL="+model,
	)

	if progressCallback != nil {
		progressCallback("正在使用 pdf2zh 翻译 PDF...")
	}

	// Run the script
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pdf2zh translation failed: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("pdf2zh output:\n%s\n", string(output))

	// Verify output file exists
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("output PDF not created")
	}

	if progressCallback != nil {
		progressCallback("翻译完成")
	}

	logger.Info("pdf2zh PDF translation completed",
		logger.String("output", outputPath))

	return nil
}

// PageCompleteCallback is called when a page translation is complete
type PageCompleteCallback func(currentPage, totalPages int, outputPath string)

// TranslatePDFWithPyMuPDF translates a PDF using PyMuPDF.
// This does extraction, translation, and overlay all in Python for consistency.
// Supports progressive display - calls pageCallback after each page is translated.
func (t *BabelDocTranslator) TranslatePDFWithPyMuPDF(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string)) error {
	return t.TranslatePDFWithPyMuPDFProgressive(inputPath, outputPath, apiKey, baseURL, model, progressCallback, nil)
}

// TranslatePDFWithPyMuPDFProgressive translates a PDF with progressive page updates.
func (t *BabelDocTranslator) TranslatePDFWithPyMuPDFProgressive(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string), pageCallback PageCompleteCallback) error {
	logger.Info("starting PyMuPDF PDF translation",
		logger.String("input", inputPath),
		logger.String("output", outputPath))

	if inputPath == "" || outputPath == "" {
		return NewPDFError(ErrGenerateFailed, "input and output paths are required", nil)
	}

	if _, err := os.Stat(inputPath); err != nil {
		return NewPDFError(ErrPDFNotFound, "input PDF not found", err)
	}

	if apiKey == "" {
		return NewPDFError(ErrGenerateFailed, "API key is required", nil)
	}

	// Ensure Python environment
	if progressCallback != nil {
		progressCallback("正在检查 Python 环境...")
	}
	if err := python.EnsureGlobalEnv(func(msg string) {
		if progressCallback != nil {
			progressCallback(msg)
		}
	}); err != nil {
		return fmt.Errorf("failed to setup Python: %w", err)
	}

	pythonPath := python.GetPythonPath()
	if pythonPath == "" {
		return fmt.Errorf("Python not ready")
	}

	// Write script to temp file
	scriptFile := filepath.Join(t.workDir, "pymupdf_translate.py")
	if err := os.WriteFile(scriptFile, []byte(PyMuPDFTranslateScript), 0644); err != nil {
		return fmt.Errorf("failed to write script: %w", err)
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		os.MkdirAll(outputDir, 0755)
	}

	// Cache path
	cachePath := filepath.Join(t.workDir, "translation_cache.json")

	// Build command
	cmd := exec.Command(pythonPath, scriptFile, inputPath, outputPath, cachePath)
	hideWindowOnWindows(cmd)

	// Get model path for layout detection
	modelPath := python.GetModelPath()

	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY="+apiKey,
		"OPENAI_BASE_URL="+baseURL,
		"OPENAI_MODEL="+model,
		"DOCLAYOUT_MODEL_PATH="+modelPath,
	)

	if progressCallback != nil {
		progressCallback("正在翻译 PDF...")
	}

	// Use real-time output parsing for progressive display
	return t.runWithProgressiveOutput(cmd, outputPath, progressCallback, pageCallback)
}

// runWithProgressiveOutput runs the command and parses output for page completion events in real-time
func (t *BabelDocTranslator) runWithProgressiveOutput(cmd *exec.Cmd, outputPath string, progressCallback func(message string), pageCallback PageCompleteCallback) error {
	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Read output in goroutines
	var outputLines []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	// Process stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		// Increase buffer size for long lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			outputLines = append(outputLines, line)
			mu.Unlock()

			// Check for page completion marker
			if strings.HasPrefix(line, "PAGE_COMPLETE:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 3 {
					currentPage := 0
					totalPages := 0
					fmt.Sscanf(parts[1], "%d", &currentPage)
					fmt.Sscanf(parts[2], "%d", &totalPages)
					
					if currentPage > 0 && totalPages > 0 {
						if progressCallback != nil {
							progressCallback(fmt.Sprintf("已翻译 %d/%d 页", currentPage, totalPages))
						}
						if pageCallback != nil {
							pageCallback(currentPage, totalPages, outputPath)
						}
					}
				}
			} else {
				fmt.Println(line)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("stdout scanner error: %v\n", err)
		}
	}()

	// Process stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			outputLines = append(outputLines, "[stderr] "+line)
			mu.Unlock()
			fmt.Println("[stderr]", line)
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("stderr scanner error: %v\n", err)
		}
	}()

	// Wait for both goroutines to finish reading
	wg.Wait()

	// Wait for command to complete
	err = cmd.Wait()

	if err != nil {
		mu.Lock()
		output := strings.Join(outputLines, "\n")
		mu.Unlock()
		return fmt.Errorf("translation failed: %v\nOutput: %s", err, output)
	}

	if _, err := os.Stat(outputPath); err != nil {
		mu.Lock()
		output := strings.Join(outputLines, "\n")
		mu.Unlock()
		return fmt.Errorf("output PDF not created\nOutput: %s", output)
	}

	if progressCallback != nil {
		progressCallback("翻译完成")
	}

	return nil
}
