// Package pdf provides clean PDF overlay functionality
// This implementation creates a clean translated PDF by:
// 1. Converting original PDF pages to images (removes text layer)
// 2. Adding translated text on top of the images
package pdf

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"latex-translator/internal/logger"
)

// CleanOverlayGenerator generates clean translated PDFs
// by converting original pages to images first
type CleanOverlayGenerator struct {
	workDir string
}

// NewCleanOverlayGenerator creates a new clean overlay generator
func NewCleanOverlayGenerator(workDir string) *CleanOverlayGenerator {
	return &CleanOverlayGenerator{
		workDir: workDir,
	}
}

// GenerateCleanOverlayPDF generates a clean PDF with translated text
// This method converts the original PDF to images first, removing the text layer,
// then overlays the translated Chinese text on top
func (g *CleanOverlayGenerator) GenerateCleanOverlayPDF(
	originalPDF string,
	blocks []TranslatedBlock,
	outputPath string,
) error {
	logger.Info("generating clean overlay PDF",
		logger.String("input", filepath.Base(originalPDF)),
		logger.String("output", filepath.Base(outputPath)))

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "clean-overlay-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy original PDF to temp directory
	originalCopy := filepath.Join(tempDir, "original.pdf")
	if err := copyFileSimple(originalPDF, originalCopy); err != nil {
		return fmt.Errorf("failed to copy original PDF: %w", err)
	}

	// Generate LaTeX that converts PDF to image and overlays text
	latexContent := g.generateCleanOverlayLatex("original.pdf", originalCopy, blocks)
	latexPath := filepath.Join(tempDir, "clean_overlay.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return fmt.Errorf("failed to write LaTeX file: %w", err)
	}

	// Compile LaTeX
	if err := compileLatexInDir(tempDir, "clean_overlay.tex"); err != nil {
		return fmt.Errorf("LaTeX compilation failed: %w", err)
	}

	// Copy output
	compiledPDF := filepath.Join(tempDir, "clean_overlay.pdf")
	if err := copyFileSimple(compiledPDF, outputPath); err != nil {
		return fmt.Errorf("failed to copy output PDF: %w", err)
	}

	logger.Info("clean overlay PDF generated successfully")
	return nil
}

// generateCleanOverlayLatex generates LaTeX that:
// 1. Uses includegraphics to render PDF as image (no text layer)
// 2. Draws white rectangles to cover original text areas
// 3. Adds translated Chinese text on top
func (g *CleanOverlayGenerator) generateCleanOverlayLatex(
	pdfFileName string,
	fullPath string,
	blocks []TranslatedBlock,
) string {
	var sb strings.Builder

	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{graphicx}
\usepackage{tikz}
\usepackage{eso-pic}
\usepackage[margin=0cm]{geometry}
\usepackage{xcolor}

% 关键：使用 graphicx 将 PDF 页面作为图像包含
% 这样原始的文本层就不会被保留

\begin{document}

`)

	// Group blocks by page
	pageBlocks := groupBlocksByPageSimple(blocks)

	// Get page count
	maxPage := getPageCountFromPDF(fullPath, blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))

		// Get blocks for this page
		pageBlockList := pageBlocks[page]

		// Use AddToShipoutPictureBG to add PDF page as background IMAGE
		sb.WriteString("\\AddToShipoutPictureBG*{%\n")
		// includegraphics renders PDF as image, removing text layer
		sb.WriteString(fmt.Sprintf("  \\includegraphics[page=%d,width=\\paperwidth,height=\\paperheight]{%s}%%\n",
			page, pdfFileName))
		sb.WriteString("}%\n")

		// Add foreground content (white boxes + translated text)
		if len(pageBlockList) > 0 {
			sb.WriteString("\\AddToShipoutPictureFG*{%\n")
			sb.WriteString("  \\begin{tikzpicture}[remember picture,overlay]\n")

			for _, block := range pageBlockList {
				if block.TranslatedText == "" {
					continue
				}

				// Escape LaTeX special characters
				text := escapeLatexChars(block.TranslatedText)

				// Calculate position (PDF coords to TikZ coords)
				// A4: 595.276pt x 841.890pt
				xShift := block.X - 595.276/2.0
				yShift := block.Y - 841.890/2.0

				// Calculate font size
				fontSize := block.FontSize
				if fontSize <= 0 {
					fontSize = 10
				}

				// Adjust for Chinese text
				adjustedSize := adjustFontForChinese(text, fontSize, block.Width, block.Height)
				if adjustedSize < 5 {
					adjustedSize = 5
				}
				if adjustedSize > 12 {
					adjustedSize = 12
				}

				// Calculate text dimensions
				textWidth := calculateTextWidth(text, adjustedSize, block.Width)
				textHeight := calculateTextHeight(text, adjustedSize, textWidth, block.Height)

				// 1. Draw white rectangle to cover original text area
				// Add padding to ensure complete coverage
				padding := 3.0
				sb.WriteString(fmt.Sprintf("    \\fill[white] ([xshift=%.2fpt,yshift=%.2fpt]current page.center) rectangle ([xshift=%.2fpt,yshift=%.2fpt]current page.center);\n",
					xShift-padding, yShift-padding,
					xShift+textWidth+padding, yShift+textHeight+padding))

				// 2. Add translated Chinese text
				sb.WriteString(fmt.Sprintf("    \\node[text=black,inner sep=1pt,outer sep=0pt,anchor=south west,font=\\fontsize{%.1f}{%.1f}\\selectfont,text width=%.2fpt,align=left] at ([xshift=%.2fpt,yshift=%.2fpt]current page.center) {%s};\n",
					adjustedSize, adjustedSize*1.2, textWidth-2, xShift+1, yShift+1, text))
			}

			sb.WriteString("  \\end{tikzpicture}%\n")
			sb.WriteString("}%\n")
		}

		// Create empty page to hold the content
		sb.WriteString("\\null\\clearpage\n\n")
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// Helper functions

func copyFileSimple(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func compileLatexInDir(dir, texFile string) error {
	cmd := exec.Command("xelatex", "-interaction=nonstopmode", texFile)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try again - sometimes LaTeX needs two passes
		cmd2 := exec.Command("xelatex", "-interaction=nonstopmode", texFile)
		cmd2.Dir = dir
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("LaTeX compilation failed: %v\nOutput: %s", err2, string(output2))
		}
	}
	_ = output
	return nil
}

func groupBlocksByPageSimple(blocks []TranslatedBlock) map[int][]TranslatedBlock {
	result := make(map[int][]TranslatedBlock)
	for _, block := range blocks {
		page := block.Page
		if page <= 0 {
			page = 1
		}
		result[page] = append(result[page], block)
	}
	return result
}

func getPageCountFromPDF(pdfPath string, blocks []TranslatedBlock) int {
	// Try to get page count from pdfcpu
	if pages, err := ExtractPDFInfoWithPDFCPU(pdfPath); err == nil && pages > 0 {
		return pages
	}

	// Fallback: get max page from blocks
	maxPage := 1
	for _, block := range blocks {
		if block.Page > maxPage {
			maxPage = block.Page
		}
	}
	return maxPage
}

func escapeLatexChars(text string) string {
	// Escape special LaTeX characters
	replacer := strings.NewReplacer(
		"\\", "\\textbackslash{}",
		"{", "\\{",
		"}", "\\}",
		"$", "\\$",
		"&", "\\&",
		"#", "\\#",
		"_", "\\_",
		"%", "\\%",
		"^", "\\textasciicircum{}",
		"~", "\\textasciitilde{}",
		"<", "\\textless{}",
		">", "\\textgreater{}",
	)
	return replacer.Replace(text)
}

func adjustFontForChinese(text string, originalSize, maxWidth, maxHeight float64) float64 {
	if maxWidth <= 0 || maxHeight <= 0 {
		return originalSize
	}

	// Count Chinese and Latin characters
	chineseCount := 0
	latinCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			latinCount++
		}
	}

	// Estimate text width
	estimatedWidth := float64(chineseCount)*originalSize + float64(latinCount)*originalSize*0.5

	// Scale down if needed
	if estimatedWidth > maxWidth {
		scaleFactor := maxWidth / estimatedWidth
		adjustedSize := originalSize * scaleFactor

		// Consider multi-line
		if adjustedSize < 6 {
			linesNeeded := estimatedWidth / maxWidth
			lineHeight := originalSize * 1.2
			if linesNeeded*lineHeight <= maxHeight {
				adjustedSize = maxHeight / (linesNeeded * 1.2)
				if adjustedSize > originalSize {
					adjustedSize = originalSize
				}
			} else {
				adjustedSize = 6
			}
		}
		return adjustedSize
	}

	return originalSize
}

func calculateTextWidth(text string, fontSize, originalWidth float64) float64 {
	// Count characters
	chineseCount := 0
	latinCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			latinCount++
		}
	}

	// Estimate width needed
	estimatedWidth := float64(chineseCount)*fontSize + float64(latinCount)*fontSize*0.5

	// Use larger of estimated and original
	if estimatedWidth > originalWidth {
		return estimatedWidth * 1.1 // Add 10% margin
	}

	// Limit max width
	maxWidth := 400.0
	if originalWidth > maxWidth {
		return maxWidth
	}

	if originalWidth <= 0 {
		return 150
	}

	return originalWidth
}

func calculateTextHeight(text string, fontSize, textWidth, originalHeight float64) float64 {
	// Count characters
	chineseCount := 0
	latinCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			latinCount++
		}
	}

	// Estimate lines needed
	estimatedWidth := float64(chineseCount)*fontSize + float64(latinCount)*fontSize*0.5
	linesNeeded := estimatedWidth / textWidth
	if linesNeeded < 1 {
		linesNeeded = 1
	}

	// Calculate height
	textHeight := fontSize * 1.3 * linesNeeded

	// Ensure at least original height
	if textHeight < originalHeight {
		textHeight = originalHeight * 1.2
	}

	return textHeight
}
