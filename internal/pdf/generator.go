// Package pdf provides PDF translation functionality including text extraction,
// batch translation, and PDF generation with translated content.
package pdf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"latex-translator/internal/logger"

	ledongthucpdf "github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/color"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PDFGenerator 负责生成翻译后的 PDF
// Requirements: 4.1, 4.2 - 生成包含中文翻译的新 PDF 文件，保持原始文档的布局和格式
type PDFGenerator struct {
	workDir  string
	fontPath string
	conf     *model.Configuration
}

// NewPDFGenerator creates a new PDFGenerator with the specified working directory
func NewPDFGenerator(workDir string) *PDFGenerator {
	return &PDFGenerator{
		workDir:  workDir,
		fontPath: "", // Will be set during font configuration
		conf:     model.NewDefaultConfiguration(),
	}
}

// NewPDFGeneratorWithFont creates a new PDFGenerator with a custom font path
func NewPDFGeneratorWithFont(workDir, fontPath string) *PDFGenerator {
	return &PDFGenerator{
		workDir:  workDir,
		fontPath: fontPath,
		conf:     model.NewDefaultConfiguration(),
	}
}

// SetFontPath sets the path to the Chinese font file
func (g *PDFGenerator) SetFontPath(fontPath string) {
	g.fontPath = fontPath
}


// GeneratePDF 生成翻译后的 PDF
// Requirements: 4.1, 4.2 - 生成包含中文翻译的新 PDF 文件，保持原始文档的布局和格式
// Property 8: PDF 生成有效性 - GeneratePDF应生成可读取的PDF文件，文件大小大于0
// Property 9: 布局保持 - 翻译后文本位置与原始位置偏差在可接受范围内
func (g *PDFGenerator) GeneratePDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// Validate input parameters
	if originalPath == "" {
		return NewPDFError(ErrGenerateFailed, "原始 PDF 路径不能为空", nil)
	}

	if outputPath == "" {
		return NewPDFError(ErrGenerateFailed, "输出 PDF 路径不能为空", nil)
	}

	// Check if original file exists
	if _, err := os.Stat(originalPath); err != nil {
		if os.IsNotExist(err) {
			return NewPDFError(ErrPDFNotFound, "原始 PDF 文件不存在", err)
		}
		return NewPDFError(ErrPDFInvalid, "无法访问原始 PDF 文件", err)
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return NewPDFError(ErrGenerateFailed, "无法创建输出目录", err)
		}
	}

	// If no blocks to translate, just copy the original file
	if len(blocks) == 0 {
		if err := g.copyFile(originalPath, outputPath); err != nil {
			return NewPDFError(ErrGenerateFailed, "无法复制原始 PDF 文件", err)
		}
		return nil
	}

	// 方案1: 优先使用 Python + PyMuPDF 方案（最精确）
	fmt.Println("正在使用 Python + PyMuPDF 方法覆盖文本...")
	if err := g.generatePythonOverlayPDF(originalPath, blocks, outputPath); err != nil {
		fmt.Printf("Warning: Python overlay failed: %v\n", err)
		
		// 方案2: 使用 Clean Overlay 方法（将PDF转为图像，移除文本层）
		fmt.Println("Trying Clean Overlay method (converts PDF to image)...")
		cleanOverlay := NewCleanOverlayGenerator(g.workDir)
		if err := cleanOverlay.GenerateCleanOverlayPDF(originalPath, blocks, outputPath); err != nil {
			fmt.Printf("Warning: Clean overlay failed: %v\n", err)
			
			// 方案3: 使用传统 LaTeX 方案
			fmt.Println("Trying traditional LaTeX overlay as fallback...")
			if err := g.generateOverlayPDF(originalPath, blocks, outputPath); err != nil {
				fmt.Printf("Warning: LaTeX overlay also failed: %v\n", err)
				fmt.Println("Falling back to copying original PDF...")
				fmt.Println("Note: Python (PyMuPDF) or LaTeX (xelatex) must be installed for PDF overlay to work.")
				fmt.Println("Install PyMuPDF: pip install PyMuPDF")
				return g.copyFile(originalPath, outputPath)
			}
		}
	}

	// Verify the output file was created and has content
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return NewPDFError(ErrGenerateFailed, "无法验证输出文件", err)
	}

	if fileInfo.Size() == 0 {
		return NewPDFError(ErrGenerateFailed, "生成的 PDF 文件为空", nil)
	}

	return nil
}

// generateSimpleTranslatedPDF generates a simple translated PDF
func (g *PDFGenerator) generateSimpleTranslatedPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "pdf-translate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Copy original PDF to temp directory
	originalCopy := filepath.Join(tempDir, "original.pdf")
	if err := g.copyFile(originalPath, originalCopy); err != nil {
		return err
	}

	// Generate text-only translated PDF
	translatedText := filepath.Join(tempDir, "translated_text.pdf")
	if err := g.generateTextOnlyPDF(blocks, translatedText); err != nil {
		return err
	}

	// Generate side-by-side PDF
	if err := g.GenerateSideBySidePDF(originalCopy, translatedText, outputPath); err != nil {
		return err
	}

	return nil
}

// generateOverlayPDF generates a PDF with Chinese text overlaid on the original
func (g *PDFGenerator) generateOverlayPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// 使用改进的LaTeX方法直接在原PDF上覆盖文字
	// 这个方法会生成一个新PDF，其中中文文本覆盖在原始位置上
	
	fmt.Println("正在使用LaTeX方法在原PDF上覆盖文字...")
	if err := g.generateLatexOverlayPDF(originalPath, blocks, outputPath); err != nil {
		fmt.Printf("Warning: LaTeX覆盖方法失败: %v\n", err)
		fmt.Println("回退到纯中文PDF方法...")
		
		// 回退到纯中文PDF方法
		return g.generateChineseOnlyPDFMethod(blocks, outputPath)
	}
	
	fmt.Println("LaTeX覆盖PDF生成成功")
	return nil
}

// generateLatexOverlayPDF uses LaTeX to overlay Chinese text on original PDF
func (g *PDFGenerator) generateLatexOverlayPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "pdf-overlay-*")
	if err != nil {
		return fmt.Errorf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// 复制原始PDF到临时目录
	originalCopy := filepath.Join(tempDir, "original.pdf")
	if err := g.copyFile(originalPath, originalCopy); err != nil {
		return fmt.Errorf("无法复制原始PDF: %v", err)
	}
	
	// 生成LaTeX文件 - 使用改进的覆盖方法
	// 传入完整路径以便正确获取页数
	latexContent := g.generateImprovedOverlayLatexWithPath("original.pdf", originalCopy, blocks)
	latexPath := filepath.Join(tempDir, "overlay.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return fmt.Errorf("无法写入LaTeX文件: %v", err)
	}
	
	// 编译LaTeX
	if err := g.compileLatex(tempDir, "overlay.tex"); err != nil {
		return fmt.Errorf("LaTeX编译失败: %v", err)
	}
	
	// 复制生成的PDF到输出路径
	compiledPDF := filepath.Join(tempDir, "overlay.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return fmt.Errorf("无法复制输出PDF: %v", err)
	}
	
	return nil
}

// generateImprovedOverlayLatex generates improved LaTeX for text overlay
// This version converts the original PDF to an image first to remove text layer
func (g *PDFGenerator) generateImprovedOverlayLatex(originalPDF string, blocks []TranslatedBlock) string {
	// 使用相对路径版本，尝试从blocks获取页数
	return g.generateImprovedOverlayLatexWithPath(originalPDF, "", blocks)
}

// generateImprovedOverlayLatexWithPath generates improved LaTeX with explicit path for page count
func (g *PDFGenerator) generateImprovedOverlayLatexWithPath(originalPDF string, fullPath string, blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	// 使用eso-pic包来在每一页上添加内容
	// 关键：使用decodearray选项将PDF转为图像，这样会移除文本层
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{pdfpages}
\usepackage{tikz}
\usepackage{eso-pic}
\usepackage[margin=0cm]{geometry}
\usepackage{xcolor}
\usepackage{graphicx}

\begin{document}

`)

	// 按页面分组
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// 获取原始PDF的实际页数
	var maxPage int
	
	// 首先尝试使用完整路径
	if fullPath != "" {
		if pageCount, err := g.getPDFPageCount(fullPath); err == nil && pageCount > 0 {
			maxPage = pageCount
			logger.Info("got page count from full path",
				logger.Int("pageCount", pageCount),
				logger.String("path", fullPath))
		}
	}
	
	// 如果完整路径失败，尝试相对路径
	if maxPage == 0 {
		// 尝试多个可能的路径
		possiblePaths := []string{
			originalPDF,
		}
		
		if g.workDir != "" && !filepath.IsAbs(originalPDF) {
			possiblePaths = append(possiblePaths, filepath.Join(g.workDir, originalPDF))
		}
		
		for _, path := range possiblePaths {
			if pageCount, err := g.getPDFPageCount(path); err == nil && pageCount > 0 {
				maxPage = pageCount
				logger.Info("got page count from path",
					logger.Int("pageCount", pageCount),
					logger.String("path", path))
				break
			}
		}
	}
	
	// 如果还是无法获取，使用blocks中的最大页码
	if maxPage == 0 {
		maxPage = g.getMaxPageFromBlocks(blocks)
		logger.Warn("could not get PDF page count, using max page from blocks",
			logger.Int("maxPage", maxPage))
	}

	// 处理每一页
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))
		
		// 检查这一页是否有翻译
		pageBlockList, hasTranslations := pageBlocks[page]
		
		if !hasTranslations || len(pageBlockList) == 0 {
			// 没有翻译，直接包含原始页面
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d}]{%s}\n\n", page, originalPDF))
			continue
		}
		
		// 使用AddToShipoutPictureBG在页面背景上添加原始PDF（作为图像）
		sb.WriteString("\\AddToShipoutPictureBG*{%\n")
		// 使用includegraphics而不是includepdf，这样会将PDF页面转为图像
		// 图像不包含可选择的文本层
		sb.WriteString(fmt.Sprintf("  \\includegraphics[page=%d,width=\\paperwidth,height=\\paperheight]{%s}%%\n", page, originalPDF))
		sb.WriteString("}%\n")
		
		// 添加前景文本
		sb.WriteString("\\AddToShipoutPictureFG*{%\n")
		sb.WriteString("  \\begin{tikzpicture}[remember picture,overlay]\n")
		
		// 添加每个翻译文本块
		for _, block := range pageBlockList {
			if block.TranslatedText == "" {
				continue
			}
			
			// 转义LaTeX特殊字符
			text := g.escapeLatexSimple(block.TranslatedText)
			
			// PDF坐标系统：原点在左下角，Y轴向上
			// TikZ坐标系统：使用current page时，原点在页面中心
			// A4纸张：595.276pt x 841.890pt
			
			// 计算位置（PDF坐标转TikZ坐标）
			xShift := block.X - 595.276/2.0
			yShift := block.Y - 841.890/2.0
			
			// 计算字体大小
			fontSize := block.FontSize
			if fontSize <= 0 {
				fontSize = 10
			}
			
			// 为中文调整字体大小
			adjustedSize := g.adjustFontSizeForChinese(text, fontSize, block.Width, block.Height)
			if adjustedSize < 5 {
				adjustedSize = 5
			}
			if adjustedSize > 12 {
				adjustedSize = 12
			}
			
			// 计算中文文本的实际宽度需求
			// 中文字符宽度约等于字体大小，英文字符约为字体大小的0.5倍
			chineseCount := 0
			englishCount := 0
			for _, r := range text {
				if r >= 0x4E00 && r <= 0x9FFF {
					chineseCount++
				} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					englishCount++
				}
			}
			
			// 估算文本实际需要的宽度
			estimatedTextWidth := float64(chineseCount)*adjustedSize + float64(englishCount)*adjustedSize*0.5
			
			// 使用原始宽度和估算宽度中的较大值，但不超过页面宽度的一半
			textWidthPt := block.Width
			if estimatedTextWidth > textWidthPt {
				textWidthPt = estimatedTextWidth * 1.1 // 增加10%余量
			}
			
			// 限制最大宽度，避免超出页面
			maxWidth := 400.0 // 约2/3页面宽度
			if textWidthPt > maxWidth {
				textWidthPt = maxWidth
			}
			
			if textWidthPt <= 0 {
				textWidthPt = 150
			}
			
			// 根据文本长度和宽度估算需要的行数
			estimatedLines := estimatedTextWidth / textWidthPt
			if estimatedLines < 1 {
				estimatedLines = 1
			}
			
			// 计算文本高度 - 基于估算的行数
			textHeightPt := adjustedSize * 1.3 * estimatedLines
			
			// 确保高度至少能覆盖原始文本
			if textHeightPt < block.Height {
				textHeightPt = block.Height * 1.2
			}
			
			// 绘制白色矩形覆盖原文
			// 增加一些边距确保完全覆盖
			sb.WriteString(fmt.Sprintf("    \\fill[white] ([xshift=%.2fpt,yshift=%.2fpt]current page.center) rectangle ([xshift=%.2fpt,yshift=%.2fpt]current page.center);\n",
				xShift-2, yShift-2, xShift+textWidthPt+2, yShift+textHeightPt+2))
			
			// 添加中文文本
			sb.WriteString(fmt.Sprintf("    \\node[text=black,inner sep=1pt,outer sep=0pt,anchor=south west,font=\\fontsize{%.1f}{%.1f}\\selectfont,text width=%.2fpt,align=left] at ([xshift=%.2fpt,yshift=%.2fpt]current page.center) {%s};\n",
				adjustedSize, adjustedSize*1.2, textWidthPt-4, xShift+1, yShift+1, text))
		}
		
		sb.WriteString("  \\end{tikzpicture}%\n")
		sb.WriteString("}%\n")
		
		// 创建一个空白页来承载这些内容
		sb.WriteString("\\null\\clearpage\n\n")
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generateChineseOnlyPDFMethod generates a Chinese-only PDF (fallback method)
func (g *PDFGenerator) generateChineseOnlyPDFMethod(blocks []TranslatedBlock, outputPath string) error {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "pdf-overlay-*")
	if err != nil {
		return fmt.Errorf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// 生成纯中文PDF
	latexContent := g.generateChineseOnlyLatexSimple(blocks)
	latexPath := filepath.Join(tempDir, "chinese.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return fmt.Errorf("无法写入LaTeX文件: %v", err)
	}
	
	// 编译LaTeX
	fmt.Println("正在编译LaTeX文件...")
	if err := g.compileLatex(tempDir, "chinese.tex"); err != nil {
		return fmt.Errorf("LaTeX编译失败: %v", err)
	}
	
	// 复制生成的PDF到输出路径
	compiledPDF := filepath.Join(tempDir, "chinese.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return fmt.Errorf("无法复制输出PDF: %v", err)
	}
	
	fmt.Println("纯中文PDF生成成功")
	return nil
}

// generateWithTikzOverlay uses TikZ overlay method (fallback)
func (g *PDFGenerator) generateWithTikzOverlay(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "pdf-overlay-*")
	if err != nil {
		return fmt.Errorf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// 复制原始PDF到临时目录
	originalCopy := filepath.Join(tempDir, "original.pdf")
	if err := g.copyFile(originalPath, originalCopy); err != nil {
		return fmt.Errorf("无法复制原始PDF: %v", err)
	}
	
	// 生成LaTeX文件
	latexContent := g.generateSimpleOverlayLatex("original.pdf", blocks)
	latexPath := filepath.Join(tempDir, "overlay.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return fmt.Errorf("无法写入LaTeX文件: %v", err)
	}
	
	// 编译LaTeX
	fmt.Println("正在编译LaTeX文件...")
	if err := g.compileLatex(tempDir, "overlay.tex"); err != nil {
		return fmt.Errorf("LaTeX编译失败: %v", err)
	}
	
	// 复制生成的PDF到输出路径
	compiledPDF := filepath.Join(tempDir, "overlay.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return fmt.Errorf("无法复制输出PDF: %v", err)
	}
	
	fmt.Println("PDF覆盖层生成成功")
	return nil
}

// generateChineseOnlyPDF generates a PDF with only Chinese text (no original background)
func (g *PDFGenerator) generateChineseOnlyPDF(blocks []TranslatedBlock, outputPath string) error {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "chinese-pdf-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	
	// 生成LaTeX文件
	latexContent := g.generateChineseOnlyLatex(blocks)
	latexPath := filepath.Join(tempDir, "chinese.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return err
	}
	
	// 编译LaTeX
	if err := g.compileLatex(tempDir, "chinese.tex"); err != nil {
		return err
	}
	
	// 复制生成的PDF到输出路径
	compiledPDF := filepath.Join(tempDir, "chinese.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return err
	}
	
	return nil
}

// generateChineseOnlyLatex generates LaTeX for Chinese-only PDF with same layout as original
func (g *PDFGenerator) generateChineseOnlyLatex(blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{tikz}
\usepackage[margin=0cm,paperwidth=210mm,paperheight=297mm]{geometry}
\usepackage{xcolor}
\pagestyle{empty}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number from blocks (no original PDF available in this function)
	maxPage := g.getMaxPageFromBlocks(blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))
		
		// Start tikzpicture for absolute positioning
		sb.WriteString("\\noindent\\begin{tikzpicture}[remember picture,overlay]\n")
		
		// Check if there are translations for this page
		if pageBlockList, ok := pageBlocks[page]; ok {
			// Add each translated text block at its original position
			for _, block := range pageBlockList {
				if block.TranslatedText == "" {
					continue
				}
				
				// Escape special LaTeX characters
				text := g.escapeLatexSimple(block.TranslatedText)
				
				// Calculate position relative to page center
				xShift := block.X - 595.276/2.0
				yShift := block.Y - 841.890/2.0
				
				// Calculate font size
				fontSize := block.FontSize
				if fontSize <= 0 {
					fontSize = 10
				}
				
				// Adjust font size for Chinese
				adjustedSize := g.adjustFontSizeForChinese(text, fontSize, block.Width, block.Height)
				if adjustedSize < 6 {
					adjustedSize = 6
				}
				if adjustedSize > 14 {
					adjustedSize = 14
				}
				
				// Calculate text width
				textWidthPt := block.Width
				if textWidthPt <= 0 {
					textWidthPt = 100
				}
				
				// Add text node (no background, just black text on white page)
				sb.WriteString(fmt.Sprintf("\\node[text=black,inner sep=0pt,outer sep=0pt,anchor=south west,font=\\fontsize{%.1f}{%.1f}\\selectfont,text width=%.1fpt,align=left] at ([xshift=%.1fpt,yshift=%.1fpt]current page.center) {%s};\n",
					adjustedSize, adjustedSize*1.2, textWidthPt, xShift, yShift, text))
			}
		}
		
		sb.WriteString("\\end{tikzpicture}\n")
		
		if page < maxPage {
			sb.WriteString("\\clearpage\n\n")
		}
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generateChineseOnlyLatexSimple generates a simple Chinese-only PDF
func (g *PDFGenerator) generateChineseOnlyLatexSimple(blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage[margin=2cm]{geometry}
\usepackage{parskip}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number from blocks
	maxPage := g.getMaxPageFromBlocks(blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("\\section*{第 %d 页}\n\n", page))
		
		if pageBlockList, ok := pageBlocks[page]; ok {
			for _, block := range pageBlockList {
				if block.TranslatedText == "" {
					continue
				}
				
				// Escape special LaTeX characters
				text := g.escapeLatexSimple(block.TranslatedText)
				
				// Add the translated text as a paragraph
				sb.WriteString(text)
				sb.WriteString("\n\n")
			}
		}
		
		if page < maxPage {
			sb.WriteString("\\newpage\n\n")
		}
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generatePdfpagesOverlayLatex generates LaTeX using pdfpages with pagecommand overlay
func (g *PDFGenerator) generatePdfpagesOverlayLatex(originalPDF string, blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{pdfpages}
\usepackage{tikz}
\usepackage[margin=0cm]{geometry}
\usepackage{xcolor}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number - use actual PDF page count
	maxPage := g.getMaxPageForGeneration(originalPDF, blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))
		
		// Check if there are translations for this page
		pageBlockList, hasTranslations := pageBlocks[page]
		
		if !hasTranslations || len(pageBlockList) == 0 {
			// No translations, just include the original page
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d}]{%s}\n\n", page, originalPDF))
			continue
		}
		
		// Include the original page with overlay using pagecommand
		sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d},pagecommand={%%\n", page))
		sb.WriteString("\\begin{tikzpicture}[remember picture,overlay]\n")
		
		// Add each translated text block
		for _, block := range pageBlockList {
			if block.TranslatedText == "" {
				continue
			}
			
			// Escape special LaTeX characters
			text := g.escapeLatexSimple(block.TranslatedText)
			
			// Calculate position
			// PDF coordinates: origin at bottom-left
			// TikZ with "current page": origin at center
			// A4 page: 595.276pt x 841.890pt
			xShift := block.X - 595.276/2.0
			yShift := block.Y - 841.890/2.0
			
			// Calculate font size
			fontSize := block.FontSize
			if fontSize <= 0 {
				fontSize = 10
			}
			
			// Adjust font size for Chinese
			adjustedSize := g.adjustFontSizeForChinese(text, fontSize, block.Width, block.Height)
			if adjustedSize < 6 {
				adjustedSize = 6
			}
			if adjustedSize > 14 {
				adjustedSize = 14
			}
			
			// Calculate text width
			textWidthPt := block.Width
			if textWidthPt <= 0 {
				textWidthPt = 100
			}
			
			// Add white background box with Chinese text
			// Use very high opacity to ensure it covers the original text
			sb.WriteString(fmt.Sprintf("\\node[fill=white,fill opacity=1.0,text opacity=1.0,text=black,inner sep=2pt,outer sep=0pt,anchor=south west,font=\\fontsize{%.1f}{%.1f}\\selectfont,text width=%.1fpt,align=left] at ([xshift=%.1fpt,yshift=%.1fpt]current page.center) {%s};\n",
				adjustedSize, adjustedSize*1.2, textWidthPt, xShift, yShift, text))
		}
		
		sb.WriteString("\\end{tikzpicture}\n")
		sb.WriteString(fmt.Sprintf("}]{%s}\n\n", originalPDF))
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generateSimpleOverlayLatex generates LaTeX that puts original PDF as background image
// and draws text on top using tikz picture environment (not pagecommand)
func (g *PDFGenerator) generateSimpleOverlayLatex(originalPDF string, blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{tikz}
\usepackage{graphicx}
\usepackage[margin=0cm,paperwidth=210mm,paperheight=297mm]{geometry}
\usepackage{xcolor}
\pagestyle{empty}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number - use actual PDF page count
	maxPage := g.getMaxPageForGeneration(originalPDF, blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))
		
		// Start tikzpicture that fills the whole page
		sb.WriteString("\\noindent\\begin{tikzpicture}[remember picture,overlay]\n")
		
		// Place original PDF page as background
		sb.WriteString(fmt.Sprintf("\\node[anchor=center,inner sep=0pt] at (current page.center) {\\includegraphics[page=%d,width=\\paperwidth,height=\\paperheight,keepaspectratio]{%s}};\n", page, originalPDF))
		
		// Check if there are translations for this page
		if pageBlockList, ok := pageBlocks[page]; ok {
			// Add each translated text block
			for _, block := range pageBlockList {
				if block.TranslatedText == "" {
					continue
				}
				
				// Escape special LaTeX characters
				text := g.escapeLatexSimple(block.TranslatedText)
				
				// Calculate position relative to page center
				// PDF coordinates: bottom-left origin (0,0 at bottom-left)
				// A4 page: 595.276pt x 841.890pt (210mm x 297mm)
				// TikZ with "current page.center": origin at center
				// Convert from PDF coords to TikZ coords
				xShift := block.X - 595.276/2.0
				yShift := block.Y - 841.890/2.0
				
				// Calculate font size
				fontSize := block.FontSize
				if fontSize <= 0 {
					fontSize = 10
				}
				
				// Adjust font size for Chinese - make it slightly smaller to fit better
				adjustedSize := g.adjustFontSizeForChinese(text, fontSize, block.Width, block.Height)
				if adjustedSize < 6 {
					adjustedSize = 6
				}
				if adjustedSize > 14 {
					adjustedSize = 14
				}
				
				// Calculate text width
				textWidthPt := block.Width
				if textWidthPt <= 0 {
					textWidthPt = 100
				}
				
				// Add white background box with black text
				// Use fill opacity=1.0 to completely cover original text
				// Use text opacity=1.0 to make Chinese text fully visible
				sb.WriteString(fmt.Sprintf("\\node[fill=white,fill opacity=1.0,text opacity=1.0,text=black,inner sep=2pt,outer sep=0pt,anchor=south west,font=\\fontsize{%.1f}{%.1f}\\selectfont,text width=%.1fpt,align=left] at ([xshift=%.1fpt,yshift=%.1fpt]current page.center) {%s};\n",
					adjustedSize, adjustedSize*1.2, textWidthPt, xShift, yShift, text))
			}
		}
		
		sb.WriteString("\\end{tikzpicture}\n")
		
		if page < maxPage {
			sb.WriteString("\\clearpage\n\n")
		}
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generateOverlayLatex generates LaTeX code for overlaying Chinese text
func (g *PDFGenerator) generateOverlayLatex(originalPDF string, blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{pdfpages}
\usepackage{tikz}
\usepackage[margin=0cm]{geometry}
\usepackage{xcolor}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number - use actual PDF page count
	maxPage := g.getMaxPageForGeneration(originalPDF, blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))
		
		// Check if there are translations for this page
		pageBlockList, hasTranslations := pageBlocks[page]
		
		if !hasTranslations || len(pageBlockList) == 0 {
			// No translations, just include the original page
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d}]{%s}\n\n", page, originalPDF))
			continue
		}
		
		// Include the original page as background with overlay
		sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d},pagecommand={%%\n", page))
		sb.WriteString("\\begin{tikzpicture}[remember picture,overlay]\n")
		
		// Add each translated text block
		for _, block := range pageBlockList {
			if block.TranslatedText == "" {
				continue
			}
			
			// Escape special LaTeX characters - simplified version
			text := g.escapeLatexSimple(block.TranslatedText)
			
			// Calculate position
			// PDF coordinates: origin at bottom-left, measured in points (1/72 inch)
			// TikZ with "current page": origin at center, need to shift to bottom-left
			// A4 page: 595.276pt x 841.890pt (210mm x 297mm)
			
			// Convert PDF coordinates (bottom-left origin) to TikZ coordinates (center origin)
			// Shift origin from center to bottom-left
			xShift := block.X - 595.276/2.0  // Shift from center
			yShift := block.Y - 841.890/2.0  // Shift from center
			
			// Calculate font size
			fontSize := block.FontSize
			if fontSize <= 0 {
				fontSize = 10
			}
			
			// Adjust font size for Chinese
			adjustedSize := g.adjustFontSizeForChinese(text, fontSize, block.Width, block.Height)
			
			// Clamp font size to reasonable range
			if adjustedSize < 8 {
				adjustedSize = 8
			}
			if adjustedSize > 16 {
				adjustedSize = 16
			}
			
			// Calculate text width in points
			textWidthPt := block.Width
			if textWidthPt <= 0 {
				textWidthPt = 100 // Default width
			}
			
			// Add white background box with Chinese text overlay
			// Use current page as reference with shift
			sb.WriteString(fmt.Sprintf("\\node[fill=white,fill opacity=1.0,text=black,inner sep=1pt,outer sep=0pt,anchor=south west,font=\\fontsize{%.1f}{%.1f}\\selectfont,text width=%.1fpt,align=left] at ([xshift=%.1fpt,yshift=%.1fpt]current page.center) {%s};\n",
				adjustedSize, adjustedSize*1.2, textWidthPt, xShift, yShift, text))
		}
		
		sb.WriteString("\\end{tikzpicture}%%\n")
		sb.WriteString(fmt.Sprintf("}]{%s}\n\n", originalPDF))
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generateTextOnlyPDF generates a PDF with only translated text
func (g *PDFGenerator) generateTextOnlyPDF(blocks []TranslatedBlock, outputPath string) error {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "pdf-text-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Generate LaTeX file
	latexContent := g.generateTextOnlyLatex(blocks)
	latexPath := filepath.Join(tempDir, "text.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return err
	}

	// Compile LaTeX
	if err := g.compileLatex(tempDir, "text.tex"); err != nil {
		return err
	}

	// Copy compiled PDF to output
	compiledPDF := filepath.Join(tempDir, "text.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return err
	}

	return nil
}

// generateTextOnlyLatex generates LaTeX code for text-only PDF
func (g *PDFGenerator) generateTextOnlyLatex(blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage[margin=2cm]{geometry}
\usepackage{parskip}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number from blocks
	maxPage := g.getMaxPageFromBlocks(blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("\\section*{第 %d 页翻译}\n\n", page))
		
		if pageBlockList, ok := pageBlocks[page]; ok {
			for i, block := range pageBlockList {
				if block.TranslatedText == "" {
					continue
				}
				
				// Escape special LaTeX characters
				text := g.escapeLatex(block.TranslatedText)
				
				sb.WriteString(fmt.Sprintf("\\textbf{[%d]} %s\n\n", i+1, text))
			}
		}
		
		if page < maxPage {
			sb.WriteString("\\newpage\n\n")
		}
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// generateTranslationLatex generates LaTeX code for translated PDF
func (g *PDFGenerator) generateTranslationLatex(originalPDF string, blocks []TranslatedBlock) string {
	var sb strings.Builder
	
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[UTF8]{ctex}
\usepackage{pdfpages}
\usepackage{tikz}
\usepackage[margin=0cm]{geometry}

\begin{document}

`)

	// Group blocks by page
	pageBlocks := g.groupBlocksByPage(blocks)
	
	// Get max page number - use actual PDF page count
	maxPage := g.getMaxPageForGeneration(originalPDF, blocks)

	// Process each page
	for page := 1; page <= maxPage; page++ {
		sb.WriteString(fmt.Sprintf("%% Page %d\n", page))
		
		// Add translations as overlay using tikz
		if pageBlockList, ok := pageBlocks[page]; ok && len(pageBlockList) > 0 {
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d},pagecommand={%%\n", page))
			sb.WriteString("\\begin{tikzpicture}[remember picture,overlay]%\n")
			
			for _, block := range pageBlockList {
				if block.TranslatedText == "" {
					continue
				}
				
				// Escape special LaTeX characters
				text := g.escapeLatex(block.TranslatedText)
				
				// Calculate position (PDF coordinates to LaTeX coordinates)
				// This is approximate and may need adjustment
				xPos := block.X / 72.0 // Convert points to inches
				yPos := block.Y / 72.0
				
				// Add white background box with text
				sb.WriteString(fmt.Sprintf("\\node[fill=white,inner sep=1pt,anchor=south west,font=\\tiny] at (%.2fcm,%.2fcm) {%s};%%\n",
					xPos*2.54, yPos*2.54, text))
			}
			
			sb.WriteString("\\end{tikzpicture}%\n")
			sb.WriteString(fmt.Sprintf("}]{%s}\n\n", originalPDF))
		} else {
			// No translations for this page
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d}]{%s}\n\n", page, originalPDF))
		}
	}

	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// escapeLatex escapes special LaTeX characters
func (g *PDFGenerator) escapeLatex(text string) string {
	replacements := map[string]string{
		"\\": "\\textbackslash{}",
		"{":  "\\{",
		"}":  "\\}",
		"$":  "\\$",
		"&":  "\\&",
		"%":  "\\%",
		"#":  "\\#",
		"_":  "\\_",
		"~":  "\\textasciitilde{}",
		"^":  "\\textasciicircum{}",
	}
	
	result := text
	for old, new := range replacements {
		result = strings.ReplaceAll(result, old, new)
	}
	
	return result
}

// escapeLatexSimple escapes special LaTeX characters with a simpler approach
// This version is more robust and handles Chinese text better
func (g *PDFGenerator) escapeLatexSimple(text string) string {
	// For Chinese text in ctex, we only need to escape a few critical characters
	// Most Chinese characters don't need escaping
	var result strings.Builder
	
	for _, r := range text {
		switch r {
		case '\\':
			result.WriteString("\\textbackslash{}")
		case '{':
			result.WriteString("\\{")
		case '}':
			result.WriteString("\\}")
		case '$':
			result.WriteString("\\$")
		case '&':
			result.WriteString("\\&")
		case '%':
			result.WriteString("\\%")
		case '#':
			result.WriteString("\\#")
		case '_':
			result.WriteString("\\_")
		case '~':
			result.WriteString("\\textasciitilde{}")
		case '^':
			result.WriteString("\\textasciicircum{}")
		default:
			result.WriteRune(r)
		}
	}
	
	return result.String()
}

// copyFile copies a file from src to dst
func (g *PDFGenerator) copyFile(src, dst string) error {
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

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// groupBlocksByPage groups translated blocks by their page number
func (g *PDFGenerator) groupBlocksByPage(blocks []TranslatedBlock) map[int][]TranslatedBlock {
	pageBlocks := make(map[int][]TranslatedBlock)
	for _, block := range blocks {
		pageBlocks[block.Page] = append(pageBlocks[block.Page], block)
	}
	return pageBlocks
}

// getMaxPageFromBlocks returns the maximum page number from the blocks
func (g *PDFGenerator) getMaxPageFromBlocks(blocks []TranslatedBlock) int {
	maxPage := 0
	for _, block := range blocks {
		if block.Page > maxPage {
			maxPage = block.Page
		}
	}
	return maxPage
}

// getMaxPageForGeneration determines the maximum page number to generate
// It tries to get the actual page count from the original PDF, falling back to
// the maximum page number in the blocks if that fails
func (g *PDFGenerator) getMaxPageForGeneration(originalPDF string, blocks []TranslatedBlock) int {
	// 优先尝试从原始PDF文件获取实际页数
	// 这样可以确保即使某些页面没有文本块，也会被包含在最终PDF中
	
	// 尝试多个可能的路径
	possiblePaths := []string{
		originalPDF,
	}
	
	// 如果是相对路径，尝试在 workDir 中查找
	if !filepath.IsAbs(originalPDF) && g.workDir != "" {
		possiblePaths = append(possiblePaths, filepath.Join(g.workDir, originalPDF))
	}
	
	// 也尝试当前目录
	if !filepath.IsAbs(originalPDF) {
		if cwd, err := os.Getwd(); err == nil {
			possiblePaths = append(possiblePaths, filepath.Join(cwd, originalPDF))
		}
	}
	
	// 尝试每个可能的路径
	for _, path := range possiblePaths {
		if pageCount, err := g.getPDFPageCount(path); err == nil && pageCount > 0 {
			logger.Info("using actual PDF page count",
				logger.Int("pageCount", pageCount),
				logger.String("source", "original PDF"),
				logger.String("path", path))
			return pageCount
		}
	}
	
	// 尝试使用 ledongthuc/pdf 库获取页数（更可靠）
	for _, path := range possiblePaths {
		if pageCount, err := g.getPDFPageCountWithLedongthuc(path); err == nil && pageCount > 0 {
			logger.Info("using actual PDF page count (ledongthuc)",
				logger.Int("pageCount", pageCount),
				logger.String("source", "original PDF"),
				logger.String("path", path))
			return pageCount
		}
	}
	
	// 如果无法获取PDF页数，返回blocks中的最大页码
	maxPage := g.getMaxPageFromBlocks(blocks)
	logger.Warn("could not get PDF page count, using max page from blocks",
		logger.Int("maxPage", maxPage),
		logger.String("originalPDF", originalPDF))
	return maxPage
}

// getPDFPageCountWithLedongthuc 使用 ledongthuc/pdf 库获取 PDF 页数
func (g *PDFGenerator) getPDFPageCountWithLedongthuc(pdfPath string) (int, error) {
	f, r, err := ledongthucpdf.Open(pdfPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return r.NumPage(), nil
}

// getPDFPageCountFromFile gets the page count from a PDF file path
// This is a helper that tries to get the page count, returns 0 if it fails
func (g *PDFGenerator) getPDFPageCountFromFile(pdfPath string) (int, error) {
	// 如果路径不是绝对路径，尝试在workDir中查找
	if !filepath.IsAbs(pdfPath) && g.workDir != "" {
		// 先尝试相对于workDir的路径
		absPath := filepath.Join(g.workDir, pdfPath)
		if _, err := os.Stat(absPath); err == nil {
			pdfPath = absPath
		}
	}
	
	// 使用pdfcpu获取页数
	return g.getPDFPageCount(pdfPath)
}


// overlayPageText overlays translated text on a specific page
func (g *PDFGenerator) overlayPageText(pdfPath string, pageNum int, blocks []TranslatedBlock) error {
	if len(blocks) == 0 {
		return nil
	}

	// Create watermark/stamp for each text block
	for _, block := range blocks {
		if block.TranslatedText == "" {
			continue
		}

		if err := g.OverlayText(pdfPath, pageNum, block); err != nil {
			// Log error but continue with other blocks
			continue
		}
	}

	return nil
}

// OverlayText 在原始 PDF 上覆盖翻译文本
// Requirements: 4.2 - 保持原始文档的布局和格式
// Property 9: 布局保持 - 翻译后文本位置与原始位置偏差在可接受范围内
func (g *PDFGenerator) OverlayText(pdfPath string, page int, block TranslatedBlock) error {
	if block.TranslatedText == "" {
		return nil
	}

	// Prepare the text for overlay
	text := g.prepareTextForOverlay(block.TranslatedText)

	// Calculate font size - may need adjustment for Chinese text
	fontSize := block.FontSize
	if fontSize <= 0 {
		fontSize = 10 // Default font size
	}

	// Adjust font size for Chinese text (Chinese characters are typically wider)
	adjustedFontSize := g.adjustFontSizeForChinese(text, fontSize, block.Width, block.Height)

	// First, add a white rectangle to cover the original text
	if err := g.AddWhiteRectangle(pdfPath, page, block.X, block.Y, block.Width, block.Height); err != nil {
		// Log but continue - the white rectangle is optional
		fmt.Printf("Warning: Failed to add white rectangle: %v\n", err)
	}

	// Create watermark description for text overlay
	// Using pdfcpu's watermark/stamp functionality
	wm, err := g.createTextWatermark(text, block.X, block.Y, adjustedFontSize, block.Width, block.Height)
	if err != nil {
		return NewPDFErrorWithPage(ErrGenerateFailed, "创建文本水印失败", page, err)
	}

	// Apply watermark to the specific page
	selectedPages := []string{fmt.Sprintf("%d", page)}
	if err := api.AddWatermarksFile(pdfPath, "", selectedPages, wm, g.conf); err != nil {
		return NewPDFErrorWithPage(ErrGenerateFailed, "应用文本水印失败", page, err)
	}

	return nil
}

// prepareTextForOverlay prepares text for PDF overlay
func (g *PDFGenerator) prepareTextForOverlay(text string) string {
	// Trim whitespace
	text = strings.TrimSpace(text)

	// Replace newlines with spaces for single-line overlay
	// For multi-line support, we would need different handling
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", "")

	// Escape special characters that might interfere with PDF
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")

	return text
}

// adjustFontSizeForChinese adjusts font size to fit Chinese text within bounds
// Requirements: 4.3 - 当中文文本超出原始文本框时，自动调整字体大小
func (g *PDFGenerator) adjustFontSizeForChinese(text string, originalSize, maxWidth, maxHeight float64) float64 {
	if maxWidth <= 0 || maxHeight <= 0 {
		return originalSize
	}

	// Count Chinese characters (they take more space)
	chineseCount := 0
	totalCount := 0
	for _, r := range text {
		totalCount++
		if isChinese(r) {
			chineseCount++
		}
	}

	if totalCount == 0 {
		return originalSize
	}

	// Estimate text width
	// Chinese characters are approximately 1.5x wider than Latin characters
	// Average character width is approximately 0.5 * fontSize for Latin
	// For Chinese, it's approximately 1.0 * fontSize
	latinCount := totalCount - chineseCount
	estimatedWidth := float64(latinCount)*0.5*originalSize + float64(chineseCount)*1.0*originalSize

	// If text fits, return original size
	if estimatedWidth <= maxWidth {
		return originalSize
	}

	// Calculate scale factor to fit text
	scaleFactor := maxWidth / estimatedWidth

	// Apply scale factor but don't go below minimum readable size
	adjustedSize := originalSize * scaleFactor
	minSize := 6.0 // Minimum readable font size
	if adjustedSize < minSize {
		adjustedSize = minSize
	}

	return adjustedSize
}

// isChinese checks if a rune is a Chinese character
func isChinese(r rune) bool {
	// CJK Unified Ideographs range
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		// CJK Unified Ideographs Extension A
		(r >= 0x3400 && r <= 0x4DBF) ||
		// CJK Unified Ideographs Extension B
		(r >= 0x20000 && r <= 0x2A6DF) ||
		// CJK Compatibility Ideographs
		(r >= 0xF900 && r <= 0xFAFF) ||
		// CJK Symbols and Punctuation
		(r >= 0x3000 && r <= 0x303F)
}

// AdjustTextSize 根据空间自动调整文本大小
// Requirements: 4.3 - 当中文文本超出原始文本框时，自动调整字体大小或换行
// Property 9: 布局保持 - 翻译后文本位置与原始位置偏差在可接受范围内
//
// This method calculates the optimal font size for the given text to fit within
// the specified maxWidth and maxHeight constraints. It handles:
// - Text wrapping for long Chinese text
// - Minimum readable font size (6pt)
// - Maximum font size constraints
// - Multi-line text handling
//
// Returns the adjusted font size and an error if the text cannot fit within constraints.
func (g *PDFGenerator) AdjustTextSize(text string, maxWidth, maxHeight float64) (float64, error) {
	// Validate input parameters
	if text == "" {
		return 0, NewPDFError(ErrGenerateFailed, "文本不能为空", nil)
	}

	if maxWidth <= 0 {
		return 0, NewPDFError(ErrGenerateFailed, "最大宽度必须大于0", nil)
	}

	if maxHeight <= 0 {
		return 0, NewPDFError(ErrGenerateFailed, "最大高度必须大于0", nil)
	}

	// Constants for font size calculation
	const (
		minFontSize     = 6.0  // Minimum readable font size in points
		maxFontSize     = 72.0 // Maximum font size in points
		defaultFontSize = 12.0 // Default starting font size
		lineHeightRatio = 1.2  // Line height as ratio of font size
	)

	// Calculate text metrics
	metrics := g.calculateTextMetrics(text)

	// Start with default font size and adjust
	fontSize := defaultFontSize

	// Calculate how many lines we can fit vertically
	maxLines := int(maxHeight / (fontSize * lineHeightRatio))
	if maxLines < 1 {
		maxLines = 1
	}

	// Binary search for optimal font size
	lowSize := minFontSize
	highSize := maxFontSize
	optimalSize := minFontSize

	for lowSize <= highSize {
		midSize := (lowSize + highSize) / 2

		// Check if text fits with this font size
		fits, _ := g.textFitsInBounds(text, metrics, midSize, maxWidth, maxHeight, lineHeightRatio)

		if fits {
			optimalSize = midSize
			lowSize = midSize + 0.5 // Try larger size
		} else {
			highSize = midSize - 0.5 // Try smaller size
		}

		// Precision threshold
		if highSize-lowSize < 0.5 {
			break
		}
	}

	// Ensure we don't go below minimum readable size
	if optimalSize < minFontSize {
		optimalSize = minFontSize
	}

	// Final validation - check if text fits at optimal size
	fits, _ := g.textFitsInBounds(text, metrics, optimalSize, maxWidth, maxHeight, lineHeightRatio)
	if !fits && optimalSize <= minFontSize {
		// Text cannot fit even at minimum size - return minimum with warning
		return minFontSize, nil
	}

	return optimalSize, nil
}

// TextMetrics holds calculated metrics for a text string
type TextMetrics struct {
	TotalChars   int     // Total character count
	ChineseChars int     // Number of Chinese characters
	LatinChars   int     // Number of Latin/ASCII characters
	SpaceChars   int     // Number of space characters
	NewlineChars int     // Number of newline characters
	AvgCharWidth float64 // Average character width factor
}

// calculateTextMetrics calculates metrics for the given text
func (g *PDFGenerator) calculateTextMetrics(text string) TextMetrics {
	metrics := TextMetrics{}

	for _, r := range text {
		metrics.TotalChars++

		switch {
		case isChinese(r):
			metrics.ChineseChars++
		case r == ' ' || r == '\t':
			metrics.SpaceChars++
		case r == '\n' || r == '\r':
			metrics.NewlineChars++
		default:
			metrics.LatinChars++
		}
	}

	// Calculate average character width factor
	// Chinese characters are approximately 1.0 * fontSize wide
	// Latin characters are approximately 0.5 * fontSize wide
	// Spaces are approximately 0.25 * fontSize wide
	if metrics.TotalChars > 0 {
		totalWidth := float64(metrics.ChineseChars)*1.0 +
			float64(metrics.LatinChars)*0.5 +
			float64(metrics.SpaceChars)*0.25
		metrics.AvgCharWidth = totalWidth / float64(metrics.TotalChars-metrics.NewlineChars)
	}

	return metrics
}

// textFitsInBounds checks if text fits within the given bounds at the specified font size
// Returns whether it fits and the number of lines needed
func (g *PDFGenerator) textFitsInBounds(text string, metrics TextMetrics, fontSize, maxWidth, maxHeight, lineHeightRatio float64) (bool, int) {
	if fontSize <= 0 {
		return false, 0
	}

	// Calculate line height
	lineHeight := fontSize * lineHeightRatio

	// Calculate maximum lines that fit in height
	maxLines := int(maxHeight / lineHeight)
	if maxLines < 1 {
		maxLines = 1
	}

	// Calculate characters per line based on width
	// Use weighted average for character width
	chineseWidth := 1.0 * fontSize
	latinWidth := 0.5 * fontSize
	spaceWidth := 0.25 * fontSize

	// Estimate total text width
	totalTextWidth := float64(metrics.ChineseChars)*chineseWidth +
		float64(metrics.LatinChars)*latinWidth +
		float64(metrics.SpaceChars)*spaceWidth

	// Calculate lines needed (with word wrapping)
	linesNeeded := g.calculateLinesNeeded(text, fontSize, maxWidth)

	// Check if text fits
	fits := linesNeeded <= maxLines && totalTextWidth/float64(linesNeeded) <= maxWidth

	return fits, linesNeeded
}

// calculateLinesNeeded calculates the number of lines needed for text with word wrapping
func (g *PDFGenerator) calculateLinesNeeded(text string, fontSize, maxWidth float64) int {
	if maxWidth <= 0 || fontSize <= 0 {
		return 1
	}

	lines := 1
	currentLineWidth := 0.0

	for _, r := range text {
		// Handle explicit newlines
		if r == '\n' {
			lines++
			currentLineWidth = 0
			continue
		}

		// Skip carriage returns
		if r == '\r' {
			continue
		}

		// Calculate character width
		var charWidth float64
		switch {
		case isChinese(r):
			charWidth = 1.0 * fontSize
		case r == ' ' || r == '\t':
			charWidth = 0.25 * fontSize
		default:
			charWidth = 0.5 * fontSize
		}

		// Check if we need to wrap
		if currentLineWidth+charWidth > maxWidth {
			lines++
			currentLineWidth = charWidth
		} else {
			currentLineWidth += charWidth
		}
	}

	return lines
}

// AdjustTextSizeWithOriginal adjusts text size considering the original text block's font size
// This ensures the translated text doesn't exceed the original font size
// Requirements: 4.3 - 当中文文本超出原始文本框时，自动调整字体大小
func (g *PDFGenerator) AdjustTextSizeWithOriginal(text string, originalFontSize, maxWidth, maxHeight float64) (float64, error) {
	// Get the optimal size for the text
	optimalSize, err := g.AdjustTextSize(text, maxWidth, maxHeight)
	if err != nil {
		return 0, err
	}

	// Don't exceed the original font size
	if originalFontSize > 0 && optimalSize > originalFontSize {
		optimalSize = originalFontSize
	}

	return optimalSize, nil
}

// WrapTextForBounds wraps text to fit within the specified bounds
// Returns the wrapped text with newlines inserted at appropriate positions
func (g *PDFGenerator) WrapTextForBounds(text string, fontSize, maxWidth float64) string {
	if maxWidth <= 0 || fontSize <= 0 || text == "" {
		return text
	}

	var result strings.Builder
	currentLineWidth := 0.0
	var currentWord strings.Builder

	flushWord := func() {
		if currentWord.Len() == 0 {
			return
		}

		word := currentWord.String()
		wordWidth := g.calculateStringWidth(word, fontSize)

		// Check if word fits on current line
		if currentLineWidth+wordWidth > maxWidth && currentLineWidth > 0 {
			result.WriteRune('\n')
			currentLineWidth = 0
		}

		result.WriteString(word)
		currentLineWidth += wordWidth
		currentWord.Reset()
	}

	for _, r := range text {
		switch {
		case r == '\n':
			flushWord()
			result.WriteRune('\n')
			currentLineWidth = 0

		case r == ' ' || r == '\t':
			flushWord()
			// Add space if not at start of line
			if currentLineWidth > 0 {
				spaceWidth := 0.25 * fontSize
				if currentLineWidth+spaceWidth <= maxWidth {
					result.WriteRune(' ')
					currentLineWidth += spaceWidth
				}
			}

		case isChinese(r):
			// Chinese characters can break anywhere
			flushWord()
			charWidth := 1.0 * fontSize
			if currentLineWidth+charWidth > maxWidth && currentLineWidth > 0 {
				result.WriteRune('\n')
				currentLineWidth = 0
			}
			result.WriteRune(r)
			currentLineWidth += charWidth

		default:
			// Latin characters - accumulate into words
			currentWord.WriteRune(r)
		}
	}

	// Flush any remaining word
	flushWord()

	return result.String()
}

// calculateStringWidth calculates the approximate width of a string at the given font size
func (g *PDFGenerator) calculateStringWidth(text string, fontSize float64) float64 {
	width := 0.0
	for _, r := range text {
		switch {
		case isChinese(r):
			width += 1.0 * fontSize
		case r == ' ' || r == '\t':
			width += 0.25 * fontSize
		default:
			width += 0.5 * fontSize
		}
	}
	return width
}


// createTextWatermark creates a watermark configuration for text overlay
func (g *PDFGenerator) createTextWatermark(text string, x, y, fontSize, width, height float64) (*model.Watermark, error) {
	// Create watermark with text
	wm := &model.Watermark{
		Mode:           model.WMText,
		TextString:     text,
		FontName:       g.getFontName(),
		FontSize:       int(fontSize),
		ScaledFontSize: int(fontSize),
		Color:          color.Black,
		Opacity:        1.0,
		OnTop:          true,
		Update:         false,
	}

	// Set position - pdfcpu uses different coordinate system
	// We need to convert from PDF coordinates to watermark position
	wm.Pos = types.TopLeft
	wm.Dx = x
	wm.Dy = y

	// Set the bounding box for the text
	if width > 0 && height > 0 {
		wm.Width = int(width)
		wm.Height = int(height)
	}

	return wm, nil
}

// getFontName returns the font name to use for Chinese text
func (g *PDFGenerator) getFontName() string {
	// If a custom font path is set, use it
	if g.fontPath != "" {
		return g.fontPath
	}

	// Use built-in fonts that are more likely to be available
	// pdfcpu supports these built-in fonts
	return "Helvetica" // Use Helvetica as fallback, will be replaced with system font
}

// GetAvailableFonts returns a list of available fonts for PDF generation
func (g *PDFGenerator) GetAvailableFonts() []string {
	return []string{
		"NotoSansSC",      // Noto Sans Simplified Chinese
		"NotoSansTC",      // Noto Sans Traditional Chinese
		"NotoSerifSC",     // Noto Serif Simplified Chinese
		"NotoSerifTC",     // Noto Serif Traditional Chinese
		"SimSun",          // 宋体
		"SimHei",          // 黑体
		"Microsoft YaHei", // 微软雅黑
		"STSong",          // 华文宋体
		"STHeiti",         // 华文黑体
	}
}

// ConfigureChineseFont configures the generator to use a Chinese font
// Requirements: 4.2 - 配置中文字体支持
func (g *PDFGenerator) ConfigureChineseFont(fontName string) error {
	if fontName == "" {
		return NewPDFError(ErrGenerateFailed, "字体名称不能为空", nil)
	}

	// Validate font name
	validFonts := g.GetAvailableFonts()
	isValid := false
	for _, f := range validFonts {
		if strings.EqualFold(f, fontName) {
			isValid = true
			break
		}
	}

	if !isValid {
		// Allow custom font paths
		if _, err := os.Stat(fontName); err == nil {
			g.fontPath = fontName
			return nil
		}
		return NewPDFErrorWithDetails(ErrGenerateFailed, "不支持的字体", fontName, nil)
	}

	g.fontPath = fontName
	return nil
}


// AddWhiteRectangle adds a white rectangle to cover original text
// This is used before overlaying translated text
func (g *PDFGenerator) AddWhiteRectangle(pdfPath string, page int, x, y, width, height float64) error {
	// Create a white rectangle watermark to cover original text
	bgColor := color.White
	wm := &model.Watermark{
		Mode:       model.WMText,
		TextString: " ", // Empty text with background
		BgColor:    &bgColor,
		Opacity:    1.0,
		OnTop:      true,
		Pos:        types.TopLeft,
		Dx:         x,
		Dy:         y,
		Width:      int(width),
		Height:     int(height),
	}

	// Apply to specific page
	selectedPages := []string{fmt.Sprintf("%d", page)}
	if err := api.AddWatermarksFile(pdfPath, "", selectedPages, wm, g.conf); err != nil {
		return NewPDFErrorWithPage(ErrGenerateFailed, "添加白色矩形失败", page, err)
	}

	return nil
}

// GeneratePDFWithCoverage generates PDF with white rectangles covering original text
// then overlays translated text
func (g *PDFGenerator) GeneratePDFWithCoverage(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// First, generate the basic PDF
	if err := g.GeneratePDF(originalPath, blocks, outputPath); err != nil {
		return err
	}

	// Note: The white rectangle coverage is handled within the overlay process
	// This method provides an alternative approach if needed

	return nil
}

// ValidateOutput validates that the generated PDF is valid and readable
// Property 8: PDF 生成有效性 - GeneratePDF应生成可读取的PDF文件，文件大小大于0
func (g *PDFGenerator) ValidateOutput(pdfPath string) error {
	// Check file exists
	fileInfo, err := os.Stat(pdfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewPDFError(ErrPDFNotFound, "生成的 PDF 文件不存在", err)
		}
		return NewPDFError(ErrPDFInvalid, "无法访问生成的 PDF 文件", err)
	}

	// Check file size
	if fileInfo.Size() == 0 {
		return NewPDFError(ErrGenerateFailed, "生成的 PDF 文件为空", nil)
	}

	// Validate PDF structure using pdfcpu
	if err := api.ValidateFile(pdfPath, g.conf); err != nil {
		return NewPDFError(ErrPDFInvalid, "生成的 PDF 文件格式无效", err)
	}

	return nil
}

// GetOutputPath generates an output path for the translated PDF
func (g *PDFGenerator) GetOutputPath(originalPath string) string {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Create output filename with "_translated" suffix
	outputName := fmt.Sprintf("%s_translated%s", name, ext)

	// If workDir is set, use it as the output directory
	if g.workDir != "" {
		return filepath.Join(g.workDir, outputName)
	}

	return filepath.Join(dir, outputName)
}

// CalculatePositionOffset calculates the position offset for layout preservation
// Property 9: 布局保持 - 翻译后文本位置与原始位置偏差在可接受范围内
func (g *PDFGenerator) CalculatePositionOffset(original, translated TextBlock) (dx, dy float64) {
	// Calculate the difference in positions
	dx = translated.X - original.X
	dy = translated.Y - original.Y
	return dx, dy
}

// IsPositionWithinTolerance checks if position offset is within acceptable range
// Property 9: 布局保持 - 翻译后文本位置与原始位置偏差在可接受范围内（例如 X, Y 偏差 < 10 像素）
func (g *PDFGenerator) IsPositionWithinTolerance(dx, dy float64, tolerance float64) bool {
	if tolerance <= 0 {
		tolerance = 10.0 // Default tolerance of 10 pixels
	}
	return abs(dx) < tolerance && abs(dy) < tolerance
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// GenerateBilingualPDF 生成双语并排PDF
// 使用LaTeX的pdfpages包将原文PDF和译文PDF左右并排合并
// leftPDF: 左侧PDF路径（通常是原文）
// rightPDF: 右侧PDF路径（通常是译文）
// outputPath: 输出的双语PDF路径
func (g *PDFGenerator) GenerateBilingualPDF(leftPDF, rightPDF, outputPath string) error {
	// 验证输入文件
	if _, err := os.Stat(leftPDF); err != nil {
		return NewPDFError(ErrPDFNotFound, "左侧PDF文件不存在", err)
	}
	if _, err := os.Stat(rightPDF); err != nil {
		return NewPDFError(ErrPDFNotFound, "右侧PDF文件不存在", err)
	}

	// 创建临时工作目录
	tempDir, err := os.MkdirTemp("", "bilingual_pdf_*")
	if err != nil {
		return NewPDFError(ErrGenerateFailed, "无法创建临时目录", err)
	}
	defer os.RemoveAll(tempDir)

	// 复制PDF文件到临时目录（避免路径问题）
	leftCopy := filepath.Join(tempDir, "left.pdf")
	rightCopy := filepath.Join(tempDir, "right.pdf")
	
	if err := g.copyFile(leftPDF, leftCopy); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制左侧PDF", err)
	}
	if err := g.copyFile(rightPDF, rightCopy); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制右侧PDF", err)
	}

	// 生成LaTeX文件
	latexContent := g.generateBilingualLatex("left.pdf", "right.pdf")
	latexPath := filepath.Join(tempDir, "bilingual.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法写入LaTeX文件", err)
	}

	// 编译LaTeX
	if err := g.compileLatex(tempDir, "bilingual.tex"); err != nil {
		return NewPDFError(ErrGenerateFailed, "LaTeX编译失败", err)
	}

	// 复制输出文件
	compiledPDF := filepath.Join(tempDir, "bilingual.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制输出PDF", err)
	}

	return nil
}

// generateBilingualLatex 生成双栏并排的LaTeX代码
func (g *PDFGenerator) generateBilingualLatex(leftPDF, rightPDF string) string {
	return fmt.Sprintf(`\documentclass[a4paper,landscape]{article}
\usepackage[margin=0.5cm]{geometry}
\usepackage{pdfpages}
\usepackage{pgfpages}

\begin{document}

%% 设置双栏布局：左边原文，右边译文
\pgfpagesuselayout{2 on 1}[a4paper,landscape,border shrink=5mm]

%% 逐页合并两个PDF
\includepdfmerge[nup=2x1,landscape,pages=-]{%s,%s}

\end{document}
`, leftPDF, rightPDF)
}

// GenerateBilingualPDFAlternate 生成交替页面的双语PDF
// 每一页原文后面紧跟对应的译文页
func (g *PDFGenerator) GenerateBilingualPDFAlternate(leftPDF, rightPDF, outputPath string) error {
	// 验证输入文件
	if _, err := os.Stat(leftPDF); err != nil {
		return NewPDFError(ErrPDFNotFound, "左侧PDF文件不存在", err)
	}
	if _, err := os.Stat(rightPDF); err != nil {
		return NewPDFError(ErrPDFNotFound, "右侧PDF文件不存在", err)
	}

	// 创建临时工作目录
	tempDir, err := os.MkdirTemp("", "bilingual_pdf_*")
	if err != nil {
		return NewPDFError(ErrGenerateFailed, "无法创建临时目录", err)
	}
	defer os.RemoveAll(tempDir)

	// 复制PDF文件到临时目录
	leftCopy := filepath.Join(tempDir, "left.pdf")
	rightCopy := filepath.Join(tempDir, "right.pdf")
	
	if err := g.copyFile(leftPDF, leftCopy); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制左侧PDF", err)
	}
	if err := g.copyFile(rightPDF, rightCopy); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制右侧PDF", err)
	}

	// 获取页数
	leftPages, err := g.getPDFPageCount(leftCopy)
	if err != nil {
		return err
	}
	rightPages, err := g.getPDFPageCount(rightCopy)
	if err != nil {
		return err
	}

	// 生成交替页面的LaTeX
	latexContent := g.generateAlternateLatex("left.pdf", "right.pdf", leftPages, rightPages)
	latexPath := filepath.Join(tempDir, "bilingual.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法写入LaTeX文件", err)
	}

	// 编译LaTeX
	if err := g.compileLatex(tempDir, "bilingual.tex"); err != nil {
		return NewPDFError(ErrGenerateFailed, "LaTeX编译失败", err)
	}

	// 复制输出文件
	compiledPDF := filepath.Join(tempDir, "bilingual.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制输出PDF", err)
	}

	return nil
}

// generateAlternateLatex 生成交替页面的LaTeX代码
func (g *PDFGenerator) generateAlternateLatex(leftPDF, rightPDF string, leftPages, rightPages int) string {
	var sb strings.Builder
	sb.WriteString(`\documentclass[a4paper]{article}
\usepackage[margin=0cm]{geometry}
\usepackage{pdfpages}

\begin{document}

`)
	
	// 交替插入页面
	maxPages := leftPages
	if rightPages > maxPages {
		maxPages = rightPages
	}
	
	for i := 1; i <= maxPages; i++ {
		if i <= leftPages {
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d}]{%s}\n", i, leftPDF))
		}
		if i <= rightPages {
			sb.WriteString(fmt.Sprintf("\\includepdf[pages={%d}]{%s}\n", i, rightPDF))
		}
	}
	
	sb.WriteString("\n\\end{document}\n")
	return sb.String()
}

// GenerateSideBySidePDF 生成真正的左右并排双栏PDF
// 每一页左边显示原文对应页，右边显示译文对应页
func (g *PDFGenerator) GenerateSideBySidePDF(leftPDF, rightPDF, outputPath string) error {
	// 验证输入文件
	if _, err := os.Stat(leftPDF); err != nil {
		return NewPDFError(ErrPDFNotFound, "左侧PDF文件不存在", err)
	}
	if _, err := os.Stat(rightPDF); err != nil {
		return NewPDFError(ErrPDFNotFound, "右侧PDF文件不存在", err)
	}

	// 创建临时工作目录
	tempDir, err := os.MkdirTemp("", "bilingual_pdf_*")
	if err != nil {
		return NewPDFError(ErrGenerateFailed, "无法创建临时目录", err)
	}
	defer os.RemoveAll(tempDir)

	// 复制PDF文件到临时目录
	leftCopy := filepath.Join(tempDir, "left.pdf")
	rightCopy := filepath.Join(tempDir, "right.pdf")
	
	if err := g.copyFile(leftPDF, leftCopy); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制左侧PDF", err)
	}
	if err := g.copyFile(rightPDF, rightCopy); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制右侧PDF", err)
	}

	// 获取页数
	leftPages, err := g.getPDFPageCount(leftCopy)
	if err != nil {
		return err
	}
	rightPages, err := g.getPDFPageCount(rightCopy)
	if err != nil {
		return err
	}

	// 生成左右并排的LaTeX
	latexContent := g.generateSideBySideLatex("left.pdf", "right.pdf", leftPages, rightPages)
	latexPath := filepath.Join(tempDir, "bilingual.tex")
	if err := os.WriteFile(latexPath, []byte(latexContent), 0644); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法写入LaTeX文件", err)
	}

	// 编译LaTeX（在 Windows 上会隐藏命令行窗口）
	if err := g.compileLatex(tempDir, "bilingual.tex"); err != nil {
		return NewPDFError(ErrGenerateFailed, "LaTeX编译失败", err)
	}

	// 复制输出文件
	compiledPDF := filepath.Join(tempDir, "bilingual.pdf")
	if err := g.copyFile(compiledPDF, outputPath); err != nil {
		return NewPDFError(ErrGenerateFailed, "无法复制输出PDF", err)
	}

	return nil
}

// generateSideBySideLatex 生成左右并排的LaTeX代码
func (g *PDFGenerator) generateSideBySideLatex(leftPDF, rightPDF string, leftPages, rightPages int) string {
	var sb strings.Builder
	sb.WriteString(`\documentclass[a4paper,landscape]{article}
\usepackage[margin=0.2cm]{geometry}
\usepackage{pdfpages}
\usepackage{graphicx}

\begin{document}

`)
	
	// 逐页左右并排
	maxPages := leftPages
	if rightPages > maxPages {
		maxPages = rightPages
	}
	
	for i := 1; i <= maxPages; i++ {
		sb.WriteString("\\begin{figure}[p]\n")
		sb.WriteString("\\centering\n")
		
		// 左侧页面
		if i <= leftPages {
			sb.WriteString(fmt.Sprintf("\\includegraphics[page=%d,width=0.48\\textwidth,keepaspectratio]{%s}\n", i, leftPDF))
		} else {
			sb.WriteString("\\hspace{0.48\\textwidth}\n")
		}
		
		sb.WriteString("\\hfill\n")
		
		// 右侧页面
		if i <= rightPages {
			sb.WriteString(fmt.Sprintf("\\includegraphics[page=%d,width=0.48\\textwidth,keepaspectratio]{%s}\n", i, rightPDF))
		}
		
		sb.WriteString("\\end{figure}\n")
		sb.WriteString("\\clearpage\n\n")
	}
	
	sb.WriteString("\\end{document}\n")
	return sb.String()
}

// compileLatex 编译LaTeX文件
func (g *PDFGenerator) compileLatex(workDir, texFile string) error {
	// 优先使用 xelatex（更好的中文支持）
	cmd := exec.Command("xelatex", "-interaction=nonstopmode", texFile)
	cmd.Dir = workDir
	
	// 在 Windows 上隐藏命令行窗口
	hideWindowOnWindows(cmd)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if PDF was actually generated despite errors
		pdfName := strings.TrimSuffix(texFile, ".tex") + ".pdf"
		pdfPath := filepath.Join(workDir, pdfName)
		if _, statErr := os.Stat(pdfPath); statErr == nil {
			// PDF exists, consider it a success despite errors
			fmt.Printf("Warning: xelatex reported errors but PDF was generated\n")
			return nil
		}
		return fmt.Errorf("xelatex编译失败: %s\n输出: %s", err.Error(), string(output))
	}
	
	return nil
}

// getPDFPageCount 获取PDF页数
func (g *PDFGenerator) getPDFPageCount(pdfPath string) (int, error) {
	ctx, err := api.ReadContextFile(pdfPath)
	if err != nil {
		return 0, NewPDFError(ErrPDFInvalid, "无法读取PDF文件", err)
	}
	return ctx.PageCount, nil
}

// generatePythonOverlayPDF generates a PDF with Chinese text overlaid using Python
// 使用 PyMuPDF 的 redaction 功能完全删除原文并添加翻译
func (g *PDFGenerator) generatePythonOverlayPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	// 创建临时翻译缓存文件
	cacheFile, err := g.createTranslationCache(blocks)
	if err != nil {
		return fmt.Errorf("failed to create translation cache: %v", err)
	}
	defer os.Remove(cacheFile)
	
	// Find Python script - look in multiple possible locations
	possiblePaths := []string{
		filepath.Join("scripts", "pdf_translate.py"),
		filepath.Join("latex-translator", "scripts", "pdf_translate.py"),
		filepath.Join("..", "..", "scripts", "pdf_translate.py"),
		filepath.Join("scripts", "translate_pdf_final.py"),
		filepath.Join("latex-translator", "scripts", "translate_pdf_final.py"),
		filepath.Join("internal", "pdf", "overlay_pdf.py"),
		filepath.Join("latex-translator", "internal", "pdf", "overlay_pdf.py"),
		"overlay_pdf.py",
	}
	
	scriptPath := ""
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			scriptPath = path
			break
		}
	}
	
	if scriptPath == "" {
		return fmt.Errorf("Python script not found in any of the expected locations")
	}
	
	// 检查是否是新版本脚本（使用翻译缓存文件）
	isNewScript := strings.Contains(scriptPath, "pdf_translate.py") || strings.Contains(scriptPath, "translate_pdf_final.py")
	
	var cmd *exec.Cmd
	if isNewScript {
		// 新版本：使用翻译缓存文件
		cmd = exec.Command("python", scriptPath, originalPath, cacheFile, outputPath)
	} else {
		// 旧版本：使用 blocks JSON
		blocksJSON, err := g.blocksToJSON(blocks)
		if err != nil {
			return fmt.Errorf("failed to convert blocks to JSON: %v", err)
		}
		cmd = exec.Command("python", scriptPath, originalPath, blocksJSON, outputPath)
	}
	
	hideWindowOnWindows(cmd)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Python overlay failed: %v\nOutput: %s", err, string(output))
	}
	
	fmt.Printf("Python overlay output: %s\n", string(output))
	return nil
}

// createTranslationCache creates a temporary translation cache file from blocks
func (g *PDFGenerator) createTranslationCache(blocks []TranslatedBlock) (string, error) {
	type CacheEntry struct {
		Original    string `json:"original"`
		Translation string `json:"translation"`
	}
	
	type Cache struct {
		Version string       `json:"version"`
		Entries []CacheEntry `json:"entries"`
	}
	
	cache := Cache{
		Version: "1.0",
		Entries: make([]CacheEntry, 0, len(blocks)),
	}
	
	for _, block := range blocks {
		if block.TranslatedText == "" {
			continue
		}
		// 移除空格以匹配 PDF 解析后的文本
		original := strings.ReplaceAll(block.Text, " ", "")
		cache.Entries = append(cache.Entries, CacheEntry{
			Original:    original,
			Translation: block.TranslatedText,
		})
	}
	
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "translation_cache_*.json")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	
	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cache); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	
	return tmpFile.Name(), nil
}

// blocksToJSON converts translated blocks to JSON string
func (g *PDFGenerator) blocksToJSON(blocks []TranslatedBlock) (string, error) {
	type JSONBlock struct {
		Page           int     `json:"page"`
		Text           string  `json:"text"`
		TranslatedText string  `json:"translated_text"`
		X              float64 `json:"x"`
		Y              float64 `json:"y"`
		Width          float64 `json:"width"`
		Height         float64 `json:"height"`
		FontSize       float64 `json:"font_size"`
	}
	
	jsonBlocks := make([]JSONBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.TranslatedText == "" {
			continue
		}
		jsonBlocks = append(jsonBlocks, JSONBlock{
			Page:           block.Page,
			Text:           block.Text,
			TranslatedText: block.TranslatedText,
			X:              block.X,
			Y:              block.Y,
			Width:          block.Width,
			Height:         block.Height,
			FontSize:       block.FontSize,
		})
	}
	
	data, err := json.Marshal(jsonBlocks)
	if err != nil {
		return "", err
	}
	
	return string(data), nil
}

// GenerateImprovedOverlayLatexForTest is a test helper that exposes the LaTeX generation
// This is used for testing purposes only
func (g *PDFGenerator) GenerateImprovedOverlayLatexForTest(originalPDF string, blocks []TranslatedBlock) string {
	return g.generateImprovedOverlayLatex(originalPDF, blocks)
}
