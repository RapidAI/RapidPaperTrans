// Package pdf provides PDF translation functionality including text extraction,
// batch translation, and PDF generation with translated content.
package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// PDFParser 负责解析 PDF 并提取文本
type PDFParser struct {
	workDir string
}

// NewPDFParser creates a new PDFParser with the specified working directory
func NewPDFParser(workDir string) *PDFParser {
	return &PDFParser{
		workDir: workDir,
	}
}

// GetPDFInfo 获取 PDF 基本信息（页数、文件大小）
// Requirements: 1.1, 1.4 - 加载 PDF 文件并显示文件基本信息
func (p *PDFParser) GetPDFInfo(pdfPath string) (*PDFInfo, error) {
	// Check if file exists
	fileInfo, err := os.Stat(pdfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewPDFError(ErrPDFNotFound, "文件不存在，请检查路径", err)
		}
		return nil, NewPDFError(ErrPDFInvalid, "无法访问文件", err)
	}

	// Check if it's a regular file
	if fileInfo.IsDir() {
		return nil, NewPDFError(ErrPDFInvalid, "路径指向目录而非文件", nil)
	}

	// Use ledongthuc/pdf to get page count (more reliable than pdfcpu for some PDFs)
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return nil, NewPDFError(ErrPDFInvalid, "无法打开 PDF 文件", err)
	}
	defer f.Close()

	pageCount := r.NumPage()

	// Check if PDF contains extractable text
	isTextPDF, err := p.IsTextPDF(pdfPath)
	if err != nil {
		// If we can't determine text status, default to false but don't fail
		isTextPDF = false
	}

	return &PDFInfo{
		FilePath:  pdfPath,
		FileName:  filepath.Base(pdfPath),
		PageCount: pageCount,
		FileSize:  fileInfo.Size(),
		IsTextPDF: isTextPDF,
	}, nil
}

// IsTextPDF 检查 PDF 是否包含可提取的文本
// Requirements: 2.4 - 如果 PDF 包含扫描图像而非可选文本，提示用户该文件不支持直接翻译
func (p *PDFParser) IsTextPDF(pdfPath string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(pdfPath); err != nil {
		if os.IsNotExist(err) {
			return false, NewPDFError(ErrPDFNotFound, "文件不存在，请检查路径", err)
		}
		return false, NewPDFError(ErrPDFInvalid, "无法访问文件", err)
	}

	// Use ledongthuc/pdf library to actually try extracting text
	// This is more reliable than checking font resources
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return false, NewPDFError(ErrPDFInvalid, "无法打开 PDF 文件", err)
	}
	defer f.Close()

	// Try to extract text from the first few pages
	// If we can extract any meaningful text, it's a text PDF
	maxPagesToCheck := 3
	if r.NumPage() < maxPagesToCheck {
		maxPagesToCheck = r.NumPage()
	}

	totalTextLength := 0
	for pageNum := 1; pageNum <= maxPagesToCheck; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// Try to get text content
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		// Count non-whitespace characters
		for _, r := range content {
			if !unicode.IsSpace(r) {
				totalTextLength++
			}
		}

		// If we found enough text, it's definitely a text PDF
		if totalTextLength > 50 {
			return true, nil
		}
	}

	// If we found some text (even a small amount), consider it a text PDF
	// This handles PDFs with minimal text content
	return totalTextLength > 0, nil
}


// ExtractText 从 PDF 中提取文本块
// Requirements: 2.1, 2.2, 2.3, 2.5 - 提取文本内容、位置信息、文本块类型，保留数学公式和特殊符号
func (p *PDFParser) ExtractText(pdfPath string) ([]TextBlock, error) {
	// Check if file exists
	if _, err := os.Stat(pdfPath); err != nil {
		if os.IsNotExist(err) {
			return nil, NewPDFError(ErrPDFNotFound, "文件不存在，请检查路径", err)
		}
		return nil, NewPDFError(ErrPDFInvalid, "无法访问文件", err)
	}

	// Open PDF file using ledongthuc/pdf library
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return nil, NewPDFError(ErrPDFInvalid, "无法打开 PDF 文件", err)
	}
	defer f.Close()

	var textBlocks []TextBlock
	blockID := 0

	totalPages := r.NumPage()
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// Check if page has content
		if page.V.Key("Contents").Kind() == pdf.Null {
			continue
		}

		// Get text by row for better grouping
		rows, err := page.GetTextByRow()
		if err != nil {
			// Log error but continue with other pages
			continue
		}

		// Process each row
		for _, row := range rows {
			if len(row.Content) == 0 {
				continue
			}

			// Merge text content from the row
			var textBuilder strings.Builder
			var minX, maxX, minY, maxY float64
			var totalFontSize float64
			var fontName string
			var isBold, isItalic bool
			first := true

			for _, text := range row.Content {
				if text.S == "" {
					continue
				}

				// Filter out PostScript/PDF operator code (garbage text)
				if isPostScriptCode(text.S) {
					continue
				}

				// Preserve original text including math formulas and special symbols
				textBuilder.WriteString(text.S)

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

				// Detect bold/italic from font name
				fontLower := strings.ToLower(text.Font)
				if strings.Contains(fontLower, "bold") {
					isBold = true
				}
				if strings.Contains(fontLower, "italic") || strings.Contains(fontLower, "oblique") {
					isItalic = true
				}
			}

			text := strings.TrimSpace(textBuilder.String())
			if text == "" {
				continue
			}

			// Skip blocks that look like PostScript code
			if isPostScriptCode(text) {
				continue
			}

			// Skip blocks with too many non-printable or control characters
			if hasExcessiveNonPrintable(text) {
				continue
			}

			// Calculate average font size
			avgFontSize := totalFontSize / float64(len(row.Content))
			if avgFontSize <= 0 {
				avgFontSize = 10.0 // Default font size if not available
			}

			// Estimate width based on text length and font size
			// This is an approximation; actual width depends on font metrics
			estimatedWidth := float64(len(text)) * avgFontSize * 0.5
			if maxX > minX {
				actualWidth := maxX - minX + avgFontSize
				// Use the larger of estimated and actual width
				if actualWidth > estimatedWidth {
					estimatedWidth = actualWidth
				}
			}
			
			// Ensure minimum width
			if estimatedWidth < avgFontSize {
				estimatedWidth = avgFontSize * float64(len(text)) * 0.5
			}

			// Estimate height based on font size
			estimatedHeight := avgFontSize * 1.2
			if estimatedHeight <= 0 {
				estimatedHeight = 12.0 // Default height
			}

			// Determine block type based on font size and content
			blockType := determineBlockType(text, avgFontSize, isBold)

			blockID++
			textBlocks = append(textBlocks, TextBlock{
				ID:        fmt.Sprintf("block_%d_%d", pageNum, blockID),
				Page:      pageNum,
				Text:      text,
				X:         minX,
				Y:         minY,
				Width:     estimatedWidth,
				Height:    estimatedHeight,
				FontSize:  avgFontSize,
				FontName:  fontName,
				IsBold:    isBold,
				IsItalic:  isItalic,
				BlockType: blockType,
			})
		}
	}

	// Sort blocks by page and then by Y position (top to bottom)
	// Note: PDF coordinate system has origin at bottom-left, so higher Y = higher on page
	// We need to sort in descending Y order to get top-to-bottom reading order
	sort.Slice(textBlocks, func(i, j int) bool {
		if textBlocks[i].Page != textBlocks[j].Page {
			return textBlocks[i].Page < textBlocks[j].Page
		}
		// Higher Y values mean higher on the page in PDF coordinates
		// Sort descending by Y for top-to-bottom reading order
		// Use a tolerance for Y comparison to handle slight variations in the same line
		yTolerance := 5.0
		if abs(textBlocks[i].Y-textBlocks[j].Y) < yTolerance {
			// Same line, sort by X (left to right)
			return textBlocks[i].X < textBlocks[j].X
		}
		return textBlocks[i].Y > textBlocks[j].Y
	})

	// Re-assign IDs after sorting
	for i := range textBlocks {
		textBlocks[i].ID = fmt.Sprintf("block_%d", i+1)
	}

	return textBlocks, nil
}

// isPostScriptCode checks if text looks like PostScript/PDF operator code
// These are internal PDF commands that should not be extracted as text
func isPostScriptCode(text string) bool {
	if len(text) == 0 {
		return false
	}

	textLower := strings.ToLower(text)

	// Check for specific PostScript syntax patterns that are very unlikely in normal text
	
	// Pattern like "/name def" or "/name { ... } def" - this is the most reliable indicator
	if strings.Contains(text, " def ") || strings.HasSuffix(text, " def") {
		// Make sure it's not just the word "def" in normal text
		// PostScript def is usually preceded by a name starting with /
		if strings.Contains(text, "/") {
			return true
		}
	}

	// Pattern like "null def" - common in PostScript
	if strings.Contains(textLower, "null def") {
		return true
	}

	// Pattern like "@stx" or "@etx" - internal markers
	if strings.Contains(text, "@stx") || strings.Contains(text, "@etx") {
		return true
	}

	// Pattern like "/burl" - hyperlink related PostScript
	if strings.Contains(textLower, "/burl") || strings.Contains(textLower, "burl@") {
		return true
	}

	// Check for PostScript-specific patterns
	psSpecificPatterns := []string{
		"currentpoint", "gsave", "grestore", "newpath", "closepath",
		"setrgbcolor", "setgray", "setlinewidth", "showpage",
		"moveto", "lineto", "curveto", "stroke", "fill",
	}

	for _, pattern := range psSpecificPatterns {
		if strings.Contains(textLower, pattern) {
			// If any of these specific patterns appear, it's likely PS code
			return true
		}
	}

	// Check for excessive forward slashes with PostScript name syntax
	// PostScript names look like /Name followed by operators
	// But URLs also have slashes, so we need to be careful
	if !strings.Contains(text, "://") && !strings.Contains(textLower, "http") {
		// Count patterns like "/word " (PostScript name followed by space)
		slashNameCount := 0
		words := strings.Fields(text)
		for _, word := range words {
			if len(word) > 1 && word[0] == '/' {
				// Check if it looks like a PostScript name (alphanumeric after /)
				isName := true
				for _, c := range word[1:] {
					if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '@') {
						isName = false
						break
					}
				}
				if isName {
					slashNameCount++
				}
			}
		}
		// If there are many PostScript-style names, it's likely PS code
		if slashNameCount >= 3 {
			return true
		}
	}

	return false
}

// hasExcessiveNonPrintable checks if text has too many non-printable characters
func hasExcessiveNonPrintable(text string) bool {
	if len(text) == 0 {
		return false
	}

	nonPrintableCount := 0
	for _, r := range text {
		// Check for control characters (except common whitespace)
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			nonPrintableCount++
		}
		// Check for other non-printable ranges
		if r >= 0x7F && r <= 0x9F {
			nonPrintableCount++
		}
	}

	// If more than 10% of characters are non-printable, skip this block
	ratio := float64(nonPrintableCount) / float64(len(text))
	return ratio > 0.1
}

// determineBlockType determines the type of text block based on its characteristics
// Requirements: 2.3 - 将文本按逻辑块（段落、标题等）进行分组
func determineBlockType(text string, fontSize float64, isBold bool) string {
	// Trim and check text length
	text = strings.TrimSpace(text)
	if text == "" {
		return "paragraph"
	}

	// Check if it's a mathematical formula
	// Mathematical formulas typically contain special characters and symbols
	if isMathFormula(text) {
		return "formula"
	}

	// Check for heading characteristics
	// Headings are typically: short, bold, larger font, or numbered sections
	isShort := len(text) < 100
	isNumberedSection := isNumberedHeading(text)
	isAllCaps := isAllUpperCase(text) && len(text) > 2

	// Large font size typically indicates heading (threshold varies by document)
	isLargeFont := fontSize > 12

	// Determine block type
	if isNumberedSection {
		return "heading"
	}

	if isBold && isShort && (isLargeFont || isAllCaps) {
		return "heading"
	}

	if isLargeFont && isShort && !strings.Contains(text, ".") {
		return "heading"
	}

	// Check for caption characteristics (typically short, starts with "Figure", "Table", etc.)
	textLower := strings.ToLower(text)
	if strings.HasPrefix(textLower, "figure") ||
		strings.HasPrefix(textLower, "table") ||
		strings.HasPrefix(textLower, "fig.") ||
		strings.HasPrefix(textLower, "tab.") ||
		strings.HasPrefix(textLower, "图") ||
		strings.HasPrefix(textLower, "表") {
		return "caption"
	}

	// Check for footnote characteristics (typically starts with number or symbol)
	if len(text) > 0 && (text[0] >= '0' && text[0] <= '9') && len(text) < 200 {
		// Could be a footnote or list item
		if strings.Contains(text, ".") && !strings.HasSuffix(text, ".") {
			return "footnote"
		}
	}

	// Check for list item
	if isListItem(text) {
		return "list_item"
	}

	// Default to paragraph
	return "paragraph"
}

// isMathFormula checks if text looks like a mathematical formula
func isMathFormula(text string) bool {
	if len(text) == 0 {
		return false
	}

	// Count mathematical symbols and operators
	mathSymbolCount := 0
	totalChars := 0
	
	// Common mathematical symbols
	mathSymbols := "∫∑∏√∂∇±×÷≤≥≠≈∞∈∉⊂⊃∪∩∧∨¬∀∃αβγδεζηθικλμνξοπρστυφχψω"
	
	for _, r := range text {
		totalChars++
		
		// Check for mathematical operators
		if r == '+' || r == '-' || r == '*' || r == '/' || r == '=' || 
		   r == '<' || r == '>' || r == '^' || r == '_' || r == '~' {
			mathSymbolCount++
		}
		
		// Check for Greek letters and mathematical symbols
		if strings.ContainsRune(mathSymbols, r) {
			mathSymbolCount++
		}
		
		// Check for parentheses, brackets (common in formulas)
		if r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' {
			mathSymbolCount++
		}
	}
	
	// If more than 30% of characters are mathematical symbols, it's likely a formula
	if totalChars > 0 && float64(mathSymbolCount)/float64(totalChars) > 0.3 {
		return true
	}
	
	// Check for common formula patterns
	// Pattern like "x = y + z" or "f(x) = ..."
	if strings.Contains(text, "=") && (strings.Contains(text, "(") || strings.Contains(text, "+") || strings.Contains(text, "-")) {
		// Check if it's mostly symbols and numbers with few words
		wordCount := len(strings.Fields(text))
		if wordCount <= 5 && len(text) < 100 {
			return true
		}
	}
	
	// Pattern like "∫f(x)dx" or "∑i=1"
	if strings.ContainsAny(text, "∫∑∏√∂∇") {
		return true
	}
	
	// Pattern with lots of subscripts/superscripts indicators
	underscoreCount := strings.Count(text, "_")
	caretCount := strings.Count(text, "^")
	if underscoreCount+caretCount > 2 && len(text) < 100 {
		return true
	}
	
	return false
}

// isNumberedHeading checks if text looks like a numbered section heading
func isNumberedHeading(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return false
	}

	// Check for patterns like "1.", "1.1", "1.1.1", "Chapter 1", "Section 1", etc.
	patterns := []string{
		"chapter", "section", "appendix", "abstract", "introduction",
		"conclusion", "references", "bibliography", "acknowledgment",
	}

	textLower := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.HasPrefix(textLower, pattern) {
			return true
		}
	}

	// Check for numbered patterns like "1.", "1.1", "1.1.1", "I.", "A."
	if len(text) >= 2 {
		// Check for patterns starting with digit or uppercase letter
		if (text[0] >= '0' && text[0] <= '9') || (text[0] >= 'A' && text[0] <= 'Z') {
			// Find the end of the numbering part (e.g., "1.1.1" or "A.")
			i := 0
			for i < len(text) && i < 15 {
				ch := text[i]
				// Allow digits, dots, and uppercase letters in the numbering
				if (ch >= '0' && ch <= '9') || ch == '.' || (ch >= 'A' && ch <= 'Z') {
					i++
				} else {
					break
				}
			}

			// Check if we found a valid numbering pattern followed by space
			if i > 0 && i < len(text) {
				numberPart := text[:i]
				// Must contain at least one dot or be followed by ) or space
				hasDot := strings.Contains(numberPart, ".")
				nextChar := text[i]

				if hasDot && (nextChar == ' ' || nextChar == '\t') {
					// Pattern like "1.1 " or "1.1.1 "
					remainingText := strings.TrimSpace(text[i:])
					// Headings are typically short and don't end with period
					if len(remainingText) < 80 {
						return true
					}
				} else if !hasDot && (nextChar == '.' || nextChar == ')') {
					// Pattern like "1." or "A)" - check if followed by space
					if i+1 < len(text) && (text[i+1] == ' ' || text[i+1] == '\t') {
						remainingText := strings.TrimSpace(text[i+1:])
						if len(remainingText) < 80 {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

// isAllUpperCase checks if text is all uppercase letters
func isAllUpperCase(text string) bool {
	hasLetter := false
	for _, r := range text {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}

// isListItem checks if text looks like a list item
func isListItem(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 2 {
		return false
	}

	// Check for bullet points
	firstRune := []rune(text)[0]
	bulletChars := []rune{'•', '◦', '▪', '▫', '●', '○', '■', '□', '-', '*', '–', '—'}
	for _, bullet := range bulletChars {
		if firstRune == bullet {
			return true
		}
	}

	// Check for numbered list items like "1)", "(1)", "a)", "(a)"
	if len(text) >= 3 {
		if text[0] == '(' && (text[2] == ')' || (len(text) > 3 && text[3] == ')')) {
			return true
		}
		if (text[0] >= '0' && text[0] <= '9') || (text[0] >= 'a' && text[0] <= 'z') {
			if text[1] == ')' || text[1] == '.' {
				return true
			}
		}
	}

	return false
}
