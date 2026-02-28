// Package pdf provides PDF overlay functionality using GoPDF2
// (github.com/VantageDataChat/GoPDF2)
//
// GoPDF2 supports:
//   - ImportPage / ImportPagesFromSource: import existing PDF pages
//   - InsertHTMLBox: render HTML content into a rectangular area
//   - RectFromUpperLeftWithStyle: draw filled rectangles
//   - AddTTFFont: load TTF fonts (with automatic subsetting for CJK)
//
// This replaces the previous Python + PyMuPDF implementation with pure Go.
package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	gopdf "github.com/VantageDataChat/GoPDF2"
	"latex-translator/internal/logger"
)

// GoPDF2Generator handles PDF translation overlay using GoPDF2 library
type GoPDF2Generator struct {
	workDir  string
	fontPath string // path to Chinese TTF font
}

// NewGoPDF2Generator creates a new GoPDF2 overlay generator
func NewGoPDF2Generator(workDir string) *GoPDF2Generator {
	return &GoPDF2Generator{
		workDir: workDir,
	}
}

// GenerateTranslatedPDF generates a translated PDF by:
// 1. Importing each page from the original PDF
// 2. For each translated block, drawing a white rectangle over the original text
// 3. Using InsertHTMLBox to render translated Chinese text
//
// Each block is overlaid independently to ensure complete page coverage.
func (g *GoPDF2Generator) GenerateTranslatedPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	logger.Info("generating translated PDF with GoPDF2",
		logger.String("original", originalPath),
		logger.String("output", outputPath),
		logger.Int("blocks", len(blocks)))

	p := gopdf.GoPdf{}
	p.Start(gopdf.Config{
		Unit:     gopdf.UnitPT,
		PageSize: *gopdf.PageSizeA4,
	})

	pageSizes := p.GetPageSizes(originalPath)
	if len(pageSizes) == 0 {
		return fmt.Errorf("failed to get page sizes from PDF: %s", originalPath)
	}
	numPages := len(pageSizes)

	fontFamily, err := g.loadChineseFont(&p)
	if err != nil {
		return fmt.Errorf("failed to load Chinese font: %w", err)
	}

	// Group blocks by page — each block is overlaid independently
	pageBlockMap := make(map[int][]TranslatedBlock)
	for _, b := range blocks {
		if b.TranslatedText == "" || b.BlockType == "formula" {
			continue
		}
		pageBlockMap[b.Page] = append(pageBlockMap[b.Page], b)
	}

	// Sort blocks on each page by Y descending (top of page first)
	for pg := range pageBlockMap {
		pbs := pageBlockMap[pg]
		sort.Slice(pbs, func(i, j int) bool {
			return pbs[i].Y > pbs[j].Y
		})
		pageBlockMap[pg] = pbs
	}

	for pageNum := 1; pageNum <= numPages; pageNum++ {
		mediaBox, ok := pageSizes[pageNum]["/MediaBox"]
		if !ok {
			mediaBox = map[string]float64{"w": 595, "h": 842}
		}
		pageW := mediaBox["w"]
		pageH := mediaBox["h"]

		p.AddPageWithOption(gopdf.PageOption{
			PageSize: &gopdf.Rect{W: pageW, H: pageH},
		})

		tpl := p.ImportPage(originalPath, pageNum, "/MediaBox")
		p.UseImportedTemplate(tpl, 0, 0, pageW, pageH)

		pbs, hasBlocks := pageBlockMap[pageNum]
		if !hasBlocks || len(pbs) == 0 {
			continue
		}

		// Pass 1: white rectangles to cover all original text
		for _, b := range pbs {
			g.drawWhiteRect(&p, b, pageH, pageW)
		}

		// Pass 2: render translated text
		overlaid := 0
		for _, b := range pbs {
			if err := g.overlayBlock(&p, b, fontFamily, pageH, pageW); err != nil {
				logger.Warn("overlay block failed",
					logger.Int("page", pageNum),
					logger.String("id", b.ID),
					logger.Err(err))
			} else {
				overlaid++
			}
		}

		fmt.Printf("  Page %d: done (%d blocks)\n", pageNum, overlaid)
	}

	if err := p.WritePdf(outputPath); err != nil {
		return fmt.Errorf("failed to write PDF: %w", err)
	}

	logger.Info("translated PDF generated successfully",
		logger.String("output", outputPath))
	return nil
}

// blockRect computes the GoPDF2 rectangle (top-left origin) for a TranslatedBlock.
// Returns x, y, w, h in GoPDF2 coordinates, and whether the block is valid.
func (g *GoPDF2Generator) blockRect(b TranslatedBlock, pageH, pageW float64) (x, y, w, h float64, ok bool) {
	// PDF coords: Y is from bottom. Block.Y is the bottom of the text line.
	// GoPDF2 coords: Y is from top.
	x = b.X
	h = b.Height
	if h <= 0 {
		h = b.FontSize * 1.2
	}
	if h <= 0 {
		h = 12
	}
	// top of block in PDF coords = b.Y + b.Height
	// convert to GoPDF2: y = pageH - (b.Y + b.Height)
	y = pageH - (b.Y + h)

	w = b.Width
	if w <= 0 {
		w = 200
	}

	// Clamp to page bounds
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+w > pageW {
		w = pageW - x
	}
	if y+h > pageH {
		h = pageH - y
	}
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, false
	}
	return x, y, w, h, true
}

// drawWhiteRect draws a white rectangle to cover the original text of a block.
func (g *GoPDF2Generator) drawWhiteRect(p *gopdf.GoPdf, b TranslatedBlock, pageH, pageW float64) {
	x, y, w, h, ok := g.blockRect(b, pageH, pageW)
	if !ok {
		return
	}
	// Add small padding to ensure full coverage
	pad := 1.0
	rx := x - pad
	ry := y - pad
	rw := w + pad*2
	rh := h + pad*2
	if rx < 0 {
		rx = 0
	}
	if ry < 0 {
		ry = 0
	}
	if rx+rw > pageW {
		rw = pageW - rx
	}
	if ry+rh > pageH {
		rh = pageH - ry
	}

	p.SetFillColor(255, 255, 255)
	p.RectFromUpperLeftWithStyle(rx, ry, rw, rh, "F")
}

// overlayBlock renders translated text into the block's position.
func (g *GoPDF2Generator) overlayBlock(p *gopdf.GoPdf, b TranslatedBlock, fontFamily string, pageH, pageW float64) error {
	x, y, w, h, ok := g.blockRect(b, pageH, pageW)
	if !ok {
		return nil
	}

	// Calculate Chinese font size (slightly smaller to fit denser text)
	fontSize := b.FontSize
	if fontSize <= 0 {
		fontSize = 10
	}
	cnFontSize := fontSize * 0.85
	if cnFontSize < 6 {
		cnFontSize = 6
	}
	if cnFontSize > 14 {
		cnFontSize = 14
	}

	// Estimate needed height for the translated text
	textLen := 0
	for _, r := range b.TranslatedText {
		if isChinese(r) {
			textLen += 2
		} else {
			textLen++
		}
	}
	charsPerLine := int(w / (cnFontSize * 0.55))
	if charsPerLine <= 0 {
		charsPerLine = 1
	}
	estimatedLines := float64(textLen) / float64(charsPerLine)
	if estimatedLines < 1 {
		estimatedLines = 1
	}
	neededH := estimatedLines * cnFontSize * 1.35

	// Use the larger of original height and needed height for the text box
	boxH := h
	if neededH > boxH {
		boxH = neededH
	}
	if y+boxH > pageH {
		boxH = pageH - y
	}

	htmlContent := g.buildHTML(b.TranslatedText, cnFontSize)

	_, err := p.InsertHTMLBox(x, y, w, boxH, htmlContent, gopdf.HTMLBoxOption{
		DefaultFontFamily: fontFamily,
		DefaultFontSize:   cnFontSize,
		DefaultColor:      [3]uint8{0, 0, 0},
	})
	if err != nil {
		return fmt.Errorf("InsertHTMLBox failed: %w", err)
	}
	return nil
}

// buildHTML wraps translated text in simple HTML for InsertHTMLBox.
func (g *GoPDF2Generator) buildHTML(text string, fontSize float64) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "\n", "<br/>")

	return fmt.Sprintf(`<span style="font-size:%.1fpt; color:#000000">%s</span>`, fontSize, text)
}

// loadChineseFont finds and loads a Chinese TTF font into the GoPDF2 instance.
func (g *GoPDF2Generator) loadChineseFont(pdf *gopdf.GoPdf) (string, error) {
	if g.fontPath != "" {
		if err := pdf.AddTTFFont("chinese", g.fontPath); err != nil {
			return "", fmt.Errorf("failed to load font %s: %w", g.fontPath, err)
		}
		return "chinese", nil
	}

	fontPaths := g.getSystemChineseFontPaths()
	for _, fp := range fontPaths {
		if _, err := os.Stat(fp); err == nil {
			if err := pdf.AddTTFFont("chinese", fp); err != nil {
				logger.Warn("failed to load font, trying next",
					logger.String("font", fp), logger.Err(err))
				continue
			}
			logger.Info("loaded Chinese font", logger.String("path", fp))
			return "chinese", nil
		}
	}

	localFonts := []string{
		filepath.Join(g.workDir, "fonts", "NotoSansSC-Regular.ttf"),
		filepath.Join(g.workDir, "NotoSansSC-Regular.ttf"),
	}
	for _, fp := range localFonts {
		if _, err := os.Stat(fp); err == nil {
			if err := pdf.AddTTFFont("chinese", fp); err == nil {
				logger.Info("loaded local Chinese font", logger.String("path", fp))
				return "chinese", nil
			}
		}
	}

	return "", fmt.Errorf("no Chinese font found; install a Chinese font (e.g. Microsoft YaHei, Noto Sans SC)")
}

// getSystemChineseFontPaths returns possible Chinese font paths for the current OS.
func (g *GoPDF2Generator) getSystemChineseFontPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Windows\Fonts\simhei.ttf`,
			`C:\Windows\Fonts\simkai.ttf`,
			`C:\Windows\Fonts\simfang.ttf`,
			`C:\Windows\Fonts\simsunb.ttf`,
			`C:\Windows\Fonts\msyh.ttc`,
			`C:\Windows\Fonts\msyhbd.ttc`,
			`C:\Windows\Fonts\simsun.ttc`,
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/STHeiti Light.ttc",
			"/Library/Fonts/Arial Unicode.ttf",
		}
	default:
		return []string{
			"/usr/share/fonts/truetype/noto/NotoSansSC-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		}
	}
}
