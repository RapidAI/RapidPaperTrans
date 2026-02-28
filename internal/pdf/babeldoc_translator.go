// Package pdf provides BabelDOC-style PDF translation functionality using pure Go.
// This implementation is inspired by funstory-ai/BabelDOC but uses pdfcpu for PDF manipulation.
package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdftypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"latex-translator/internal/logger"
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
	var minX, maxX, minY, maxY float64
	var totalFontSize float64
	var fontName string
	var isBold, isItalic bool
	var chars []CharInfo
	first := true

	// First pass: collect all non-empty text fragments
	var fragments []string
	for _, text := range row.Content {
		if text.S == "" {
			continue
		}
		if t.isPostScriptCode(text.S) {
			continue
		}
		fragments = append(fragments, text.S)
	}

	// Second pass: join fragments with smart spacing.
	// ledongthuc/pdf splits text at font/encoding boundaries (ligatures like fi, fl, ff).
	// Since we can't reliably distinguish ligature splits from word boundaries
	// without actual X position data, we add spaces between all fragments.
	// The LLM translator can handle minor spacing artifacts like "Lar ge" or "v arious".
	var textBuilder strings.Builder
	for i, frag := range fragments {
		if i > 0 && textBuilder.Len() > 0 {
			prev := fragments[i-1]
			// Only skip space for clear ligature/encoding patterns:
			skipSpace := false
			// 1-char fragment that's a letter (ligature split or small-caps encoding)
			if len(prev) == 1 && ((prev[0] >= 'a' && prev[0] <= 'z') || (prev[0] >= 'A' && prev[0] <= 'Z')) {
				skipSpace = true
			}
			if len(frag) == 1 && ((frag[0] >= 'a' && frag[0] <= 'z') || (frag[0] >= 'A' && frag[0] <= 'Z')) {
				skipSpace = true
			}
			// Don't add space before punctuation
			if len(frag) > 0 && (frag[0] == ',' || frag[0] == '.' || frag[0] == ';' || frag[0] == ':' || frag[0] == ')' || frag[0] == ']') {
				skipSpace = true
			}
			// Don't add space after opening bracket
			if len(prev) > 0 && (prev[len(prev)-1] == '(' || prev[len(prev)-1] == '[') {
				skipSpace = true
			}
			// Don't add space if prev ends with hyphen (line-break or compound word)
			if len(prev) > 0 && prev[len(prev)-1] == '-' {
				skipSpace = true
			}
			if !skipSpace {
				textBuilder.WriteString(" ")
			}
		}
		textBuilder.WriteString(frag)
	}

	// Track positions from row content
	for _, text := range row.Content {
		if text.S == "" || t.isPostScriptCode(text.S) {
			continue
		}

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
	// Cap width to reasonable page bounds:
	// For US Letter (612pt), max text width is ~500pt (with margins)
	// For A4 (595pt), similar constraint
	maxTextWidth := 500.0
	if estimatedWidth > maxTextWidth {
		estimatedWidth = maxTextWidth
	}
	// Also ensure width doesn't extend past right edge of page
	// (assuming ~72pt right margin on a 612pt page)
	maxRight := 540.0
	if minX+estimatedWidth > maxRight {
		estimatedWidth = maxRight - minX
		if estimatedWidth < 50 {
			estimatedWidth = 50
		}
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
// Priority: 1. GoPDF2 overlay (best quality), 2. pdfcpu stamps (fallback)
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

	// Method 1: Try GoPDF2 overlay (pure Go, best quality for Chinese)
	fmt.Println("正在使用 GoPDF2 方法覆盖文本...")
	if err := t.generateGoPDF2OverlayPDF(originalPath, blocks, outputPath); err == nil {
		// Verify output
		if fileInfo, err := os.Stat(outputPath); err == nil && fileInfo.Size() > 0 {
			logger.Info("translated PDF generated successfully with GoPDF2",
				logger.String("output", outputPath))
			return nil
		}
	} else {
		fmt.Printf("Warning: GoPDF2 overlay failed: %v\n", err)
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

// isWordBoundary determines if two adjacent text fragments should have a space between them.
// ledongthuc/pdf splits text at font/encoding boundaries (e.g., ligatures fi, fl, ff, ffi).
// Examples of ligature splits: "v"+"arious"="various", "e"+"ven"="even", "Ho"+"we"+"v"+"er"
// Examples of word boundaries: "language"+"models", "consistently"+"benefit"
func (t *BabelDocTranslator) isWordBoundary(prev, curr string) bool {
	if len(prev) == 0 || len(curr) == 0 {
		return false
	}

	lastChar := prev[len(prev)-1]
	firstChar := curr[0]

	// If prev ends with hyphen at end of line, no space (line-break hyphen)
	if lastChar == '-' {
		return false
	}

	// If current starts with punctuation that attaches to previous word, no space
	if firstChar == ',' || firstChar == '.' || firstChar == ';' || firstChar == ':' ||
		firstChar == ')' || firstChar == ']' || firstChar == '}' || firstChar == '\'' {
		return false
	}

	// If prev ends with opening bracket, no space
	if lastChar == '(' || lastChar == '[' || lastChar == '{' {
		return false
	}

	// Ligature split detection:
	// PDF fonts commonly split at ligature boundaries, producing 1-char fragments
	// like "v", "e", "w" that are parts of words like "various", "even", "shadow".
	// Rule: if EITHER fragment is exactly 1 character and it's a lowercase letter,
	// it's almost certainly a ligature split.
	if len(prev) == 1 && lastChar >= 'a' && lastChar <= 'z' {
		return false
	}
	if len(curr) == 1 && firstChar >= 'a' && firstChar <= 'z' {
		return false
	}

	// Special case: "Lar"+"ge" type splits where a short fragment (2-3 chars)
	// continues a word. If prev is short AND ends lowercase AND curr starts lowercase
	// AND curr is also short, likely a split word.
	// But "we"+"observe" should be two words. The difference is that "ge" is not
	// a common English word while "we" is.
	// Since we can't use a dictionary, use a simpler rule:
	// If prev is exactly 2 chars, both lowercase, and curr starts lowercase,
	// check if prev looks like a common 2-letter word.
	if len(prev) == 2 && lastChar >= 'a' && lastChar <= 'z' && prev[0] >= 'a' && prev[0] <= 'z' {
		// Common 2-letter words that should NOT be joined to next fragment
		commonWords := map[string]bool{
			"an": true, "as": true, "at": true, "be": true, "by": true,
			"do": true, "go": true, "he": true, "if": true, "in": true,
			"is": true, "it": true, "me": true, "my": true, "no": true,
			"of": true, "on": true, "or": true, "so": true, "to": true,
			"up": true, "us": true, "we": true,
		}
		if commonWords[prev] {
			// It's a real word — add space if next also looks like a word start
			if firstChar >= 'a' && firstChar <= 'z' && len(curr) >= 2 {
				return true
			}
		} else {
			// Not a common word — likely a ligature fragment (e.g., "ge", "er", "ar")
			if firstChar >= 'a' && firstChar <= 'z' {
				return false
			}
		}
	}

	// If both fragments are 3+ chars, it's a word boundary
	if len(prev) >= 2 && len(curr) >= 2 {
		return true
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

// generateGoPDF2OverlayPDF generates a PDF with Chinese text overlaid using GoPDF2
func (t *BabelDocTranslator) generateGoPDF2OverlayPDF(originalPath string, blocks []TranslatedBabelDocBlock, outputPath string) error {
	// Convert TranslatedBabelDocBlock to TranslatedBlock for GoPDF2Generator
	var translatedBlocks []TranslatedBlock
	for _, b := range blocks {
		if b.TranslatedText == "" || b.BlockType == "formula" {
			continue
		}
		translatedBlocks = append(translatedBlocks, TranslatedBlock{
			TextBlock: TextBlock{
				ID:        b.ID,
				Page:      b.Page,
				Text:      b.Text,
				X:         b.X,
				Y:         b.Y,
				Width:     b.Width,
				Height:    b.Height,
				FontSize:  b.FontSize,
				FontName:  b.FontName,
				IsBold:    b.IsBold,
				IsItalic:  b.IsItalic,
				BlockType: b.BlockType,
			},
			TranslatedText: b.TranslatedText,
			FromCache:      b.FromCache,
		})
	}

	gen := NewGoPDF2Generator(t.workDir)
	if t.fontPath != "" {
		gen.fontPath = t.fontPath
	}
	return gen.GenerateTranslatedPDF(originalPath, translatedBlocks, outputPath)
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
		if block.X == 0 && block.Y == 0 {
			continue
		}
		
		// Skip blocks with negative Y (below page, e.g., arXiv ID watermarks)
		if block.Y < 0 {
			continue
		}
		
		// Skip very short blocks that are likely page numbers or artifacts
		if len(block.Text) < 3 && block.Width < 20 {
			continue
		}

		// Skip blocks that look like page identifiers (e.g., "Preprint", "arXiv:...")
		textLower := strings.ToLower(strings.TrimSpace(block.Text))
		if textLower == "preprint" || strings.HasPrefix(textLower, "arxiv:") {
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

// PageCompleteCallback is called when a page translation is complete
type PageCompleteCallback func(currentPage, totalPages int, outputPath string)

// TranslatePDFWithGoPDF2 translates a PDF using pure Go (GoPDF2).
// Extract text → translate via API → overlay with GoPDF2.
func (t *BabelDocTranslator) TranslatePDFWithGoPDF2(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string)) error {
	return t.TranslatePDFWithGoPDF2Progressive(inputPath, outputPath, apiKey, baseURL, model, progressCallback, nil)
}

// TranslatePDFWithPyMuPDF is an alias for TranslatePDFWithGoPDF2 (kept for backward compatibility).
func (t *BabelDocTranslator) TranslatePDFWithPyMuPDF(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string)) error {
	return t.TranslatePDFWithGoPDF2(inputPath, outputPath, apiKey, baseURL, model, progressCallback)
}

// TranslatePDFWithPyMuPDFProgressive is an alias for TranslatePDFWithGoPDF2Progressive (kept for backward compatibility).
func (t *BabelDocTranslator) TranslatePDFWithPyMuPDFProgressive(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string), pageCallback PageCompleteCallback) error {
	return t.TranslatePDFWithGoPDF2Progressive(inputPath, outputPath, apiKey, baseURL, model, progressCallback, pageCallback)
}

// TranslatePDFWithGoPDF2Progressive translates a PDF with progressive page updates.
// Pure Go implementation: extract text → translate via API → overlay with GoPDF2.
func (t *BabelDocTranslator) TranslatePDFWithGoPDF2Progressive(inputPath, outputPath, apiKey, baseURL, model string, progressCallback func(message string), pageCallback PageCompleteCallback) error {
	logger.Info("starting Go PDF translation",
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

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		os.MkdirAll(outputDir, 0755)
	}

	// Phase 1: Extract text blocks using Go
	if progressCallback != nil {
		progressCallback("正在提取文本...")
	}

	blocks, err := t.ExtractBlocks(inputPath)
	if err != nil {
		return fmt.Errorf("text extraction failed: %w", err)
	}

	// Filter translatable blocks
	translatableBlocks := FilterTranslatableBlocks(blocks)
	totalBlocks := len(translatableBlocks)

	logger.Info("extracted blocks",
		logger.Int("total", len(blocks)),
		logger.Int("translatable", totalBlocks))

	if totalBlocks == 0 {
		// No translatable blocks, just copy original
		if progressCallback != nil {
			progressCallback("没有可翻译的文本块，复制原始文件...")
		}
		return t.copyFile(inputPath, outputPath)
	}

	// Get total pages for progress reporting
	totalPages, _ := t.GetPDFPageCount(inputPath)
	if totalPages <= 0 {
		totalPages = 1
	}

	// Phase 2: Translate text blocks using Go BatchTranslator
	if progressCallback != nil {
		progressCallback("正在翻译文本...")
	}

	batchTranslator := NewBatchTranslator(BatchTranslatorConfig{
		APIKey:        apiKey,
		BaseURL:       baseURL,
		Model:         model,
		ContextWindow: DefaultContextWindow,
		Concurrency:   DefaultConcurrency,
	})

	// Load cache
	cachePath := filepath.Join(t.workDir, "translation_cache.json")
	cache := NewTranslationCache(cachePath)
	if err := cache.Load(); err != nil {
		logger.Warn("failed to load cache", logger.Err(err))
	}

	// Convert to TextBlock for batch translator
	var textBlocks []TextBlock
	for _, b := range translatableBlocks {
		textBlocks = append(textBlocks, TextBlock{
			ID:        b.ID,
			Page:      b.Page,
			Text:      b.Text,
			X:         b.X,
			Y:         b.Y,
			Width:     b.Width,
			Height:    b.Height,
			FontSize:  b.FontSize,
			FontName:  b.FontName,
			IsBold:    b.IsBold,
			IsItalic:  b.IsItalic,
			BlockType: b.BlockType,
		})
	}

	// Check cache and separate cached vs uncached
	var uncachedBlocks []TextBlock
	translations := make(map[string]string)
	cachedCount := 0

	for _, tb := range textBlocks {
		if cached, ok := cache.Get(tb.Text); ok && cached != "" {
			translations[tb.ID] = cached
			cachedCount++
		} else {
			uncachedBlocks = append(uncachedBlocks, tb)
		}
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("缓存命中 %d/%d 块，需翻译 %d 块...", cachedCount, totalBlocks, len(uncachedBlocks)))
	}

	// Translate uncached blocks
	if len(uncachedBlocks) > 0 {
		translated, err := batchTranslator.TranslateWithRetryAndProgress(uncachedBlocks, DefaultMaxRetries, func(completed, total int) {
			if progressCallback != nil {
				progressCallback(fmt.Sprintf("正在翻译... (%d/%d)", completed+cachedCount, totalBlocks))
			}
			// Report page progress
			if pageCallback != nil && totalPages > 0 {
				// Estimate current page based on progress
				currentPage := (completed + cachedCount) * totalPages / totalBlocks
				if currentPage < 1 {
					currentPage = 1
				}
				if currentPage > totalPages {
					currentPage = totalPages
				}
				pageCallback(currentPage, totalPages, outputPath)
			}
		})
		if err != nil {
			logger.Warn("batch translation had errors", logger.Err(err))
		}

		for _, tb := range translated {
			if tb.TranslatedText != "" {
				translations[tb.ID] = tb.TranslatedText
				cache.Set(tb.Text, tb.TranslatedText)
			}
		}
	}

	// Save cache
	if err := cache.Save(); err != nil {
		logger.Warn("failed to save cache", logger.Err(err))
	}

	// Phase 3: Generate translated PDF using GoPDF2
	if progressCallback != nil {
		progressCallback("正在生成翻译后的 PDF...")
	}

	// Convert to TranslatedBlock for GoPDF2
	var translatedBlocks []TranslatedBlock
	for _, b := range translatableBlocks {
		translatedText, ok := translations[b.ID]
		if !ok || translatedText == "" {
			continue
		}
		translatedBlocks = append(translatedBlocks, TranslatedBlock{
			TextBlock: TextBlock{
				ID:        b.ID,
				Page:      b.Page,
				Text:      b.Text,
				X:         b.X,
				Y:         b.Y,
				Width:     b.Width,
				Height:    b.Height,
				FontSize:  b.FontSize,
				FontName:  b.FontName,
				IsBold:    b.IsBold,
				IsItalic:  b.IsItalic,
				BlockType: b.BlockType,
			},
			TranslatedText: translatedText,
		})
	}

	gen := NewGoPDF2Generator(t.workDir)
	if t.fontPath != "" {
		gen.fontPath = t.fontPath
	}

	if err := gen.GenerateTranslatedPDF(inputPath, translatedBlocks, outputPath); err != nil {
		return fmt.Errorf("PDF generation failed: %w", err)
	}

	// Final page callback
	if pageCallback != nil {
		pageCallback(totalPages, totalPages, outputPath)
	}

	if progressCallback != nil {
		progressCallback("翻译完成")
	}

	logger.Info("Go PDF translation completed",
		logger.String("output", outputPath),
		logger.Int("translated", len(translatedBlocks)),
		logger.Int("cached", cachedCount))

	return nil
}
