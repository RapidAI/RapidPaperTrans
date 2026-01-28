// Package pdf provides PDF reconstruction functionality
package pdf

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"latex-translator/internal/logger"
)

// PDFRebuilder handles complete PDF reconstruction with translated content
type PDFRebuilder struct {
	workDir  string
	fontPath string
}

// NewPDFRebuilder creates a new PDF rebuilder
func NewPDFRebuilder(workDir, fontPath string) *PDFRebuilder {
	return &PDFRebuilder{
		workDir:  workDir,
		fontPath: fontPath,
	}
}

// RebuildPDF completely rebuilds a PDF with translated content
// This is different from overlay - it creates a new PDF from scratch
func (r *PDFRebuilder) RebuildPDF(originalPath string, elements []LayoutElement, translatedBlocks []TranslatedBlock, outputPath string) error {
	logger.Info("rebuilding PDF with translated content",
		logger.String("original", filepath.Base(originalPath)),
		logger.String("output", filepath.Base(outputPath)))

	// Create element-to-translation mapping
	translationMap := r.createTranslationMap(elements, translatedBlocks)

	// Generate rebuilt PDF using Python script (similar to overlay but full rebuild)
	if err := r.rebuildWithPython(originalPath, elements, translationMap, outputPath); err != nil {
		logger.Warn("Python rebuild failed, trying LaTeX method", logger.Err(err))
		return r.rebuildWithLaTeX(originalPath, elements, translationMap, outputPath)
	}

	return nil
}

// createTranslationMap maps layout elements to their translations
func (r *PDFRebuilder) createTranslationMap(elements []LayoutElement, translatedBlocks []TranslatedBlock) map[string]string {
	translationMap := make(map[string]string)

	// Create a map from original text to translated text
	for _, block := range translatedBlocks {
		translationMap[block.Text] = block.TranslatedText
	}

	return translationMap
}

// rebuildWithPython uses Python + PyMuPDF to rebuild the PDF
func (r *PDFRebuilder) rebuildWithPython(originalPath string, elements []LayoutElement, translations map[string]string, outputPath string) error {
	// TODO: Implement Python-based PDF rebuilding
	// This is more sophisticated than overlay:
	// 1. Extract all images and graphics from original
	// 2. Create new PDF with same page sizes
	// 3. Place images and graphics in original positions
	// 4. Add translated text in appropriate positions
	// 5. Preserve formulas as images or vector graphics

	return fmt.Errorf("Python rebuild not yet implemented")
}

// rebuildWithLaTeX uses LaTeX to rebuild the PDF
func (r *PDFRebuilder) rebuildWithLaTeX(originalPath string, elements []LayoutElement, translations map[string]string, outputPath string) error {
	// TODO: Implement LaTeX-based PDF rebuilding
	return fmt.Errorf("LaTeX rebuild not yet implemented")
}

// GenerateDualPDF generates a bilingual side-by-side PDF
func (r *PDFRebuilder) GenerateDualPDF(originalPath, translatedPath, outputPath string) error {
	logger.Info("generating dual-language PDF",
		logger.String("output", filepath.Base(outputPath)))

	// Use the existing GenerateSideBySidePDF method
	generator := NewPDFGenerator(r.workDir)
	return generator.GenerateSideBySidePDF(originalPath, translatedPath, outputPath)
}

// EnhancedPDFTranslator extends PDFTranslator with AI layout detection
type EnhancedPDFTranslator struct {
	*PDFTranslator
	layoutDetector *LayoutDetector
	rebuilder      *PDFRebuilder
	useAILayout    bool
}

// EnhancedTranslatorConfig holds configuration for enhanced translator
type EnhancedTranslatorConfig struct {
	PDFTranslatorConfig
	LayoutModelPath string
	UseAILayout     bool
	FontPath        string
}

// NewEnhancedPDFTranslator creates a new enhanced PDF translator with AI layout detection
func NewEnhancedPDFTranslator(cfg EnhancedTranslatorConfig) (*EnhancedPDFTranslator, error) {
	// Create base translator
	baseTranslator := NewPDFTranslator(cfg.PDFTranslatorConfig)

	// Create layout detector
	layoutDetector, err := NewLayoutDetector(LayoutDetectorConfig{
		ModelPath: cfg.LayoutModelPath,
		Enabled:   cfg.UseAILayout,
	})
	if err != nil {
		logger.Warn("failed to create layout detector, using rule-based detection",
			logger.Err(err))
	}

	// Create rebuilder
	rebuilder := NewPDFRebuilder(cfg.WorkDir, cfg.FontPath)

	return &EnhancedPDFTranslator{
		PDFTranslator:  baseTranslator,
		layoutDetector: layoutDetector,
		rebuilder:      rebuilder,
		useAILayout:    cfg.UseAILayout && layoutDetector != nil,
	}, nil
}

// TranslatePDFEnhanced translates PDF using AI layout detection
func (e *EnhancedPDFTranslator) TranslatePDFEnhanced() (*EnhancedTranslationResult, error) {
	e.mu.Lock()

	logger.Info("starting enhanced PDF translation",
		logger.String("file", e.currentFile),
		logger.Bool("useAILayout", e.useAILayout))

	// Check if PDF is loaded
	if e.currentFile == "" || len(e.textBlocks) == 0 {
		e.mu.Unlock()
		return nil, NewPDFError(ErrTranslateFailed, "请先加载 PDF 文件", nil)
	}

	e.updateStatusLocked(PDFPhaseTranslating, 0, "正在分析文档布局...")
	e.mu.Unlock()

	// Step 1: Detect layout with AI (if enabled)
	var layoutElements []LayoutElement
	if e.useAILayout {
		elements, err := e.detectLayoutForAllPages()
		if err != nil {
			logger.Warn("AI layout detection failed, falling back to rule-based",
				logger.Err(err))
		} else {
			layoutElements = elements
		}
	}

	// Step 2: Merge layout elements with extracted text blocks
	enhancedBlocks := e.mergeLayoutAndText(layoutElements, e.textBlocks)

	// Step 3: Filter translatable blocks
	translatableBlocks := e.filterTranslatableBlocks(enhancedBlocks)

	logger.Info("layout analysis complete",
		logger.Int("totalElements", len(enhancedBlocks)),
		logger.Int("translatable", len(translatableBlocks)))

	// Step 4: Translate using base translator logic
	e.mu.Lock()
	e.updateStatusLocked(PDFPhaseTranslating, 10, "正在翻译...")
	e.mu.Unlock()

	// Use the existing translation logic
	result, err := e.PDFTranslator.TranslatePDF()
	if err != nil {
		return nil, err
	}

	// Step 5: Generate dual PDF if requested
	e.mu.Lock()
	e.updateStatusLocked(PDFPhaseGenerating, 95, "正在生成双语版本...")
	e.mu.Unlock()

	dualPath := e.getDualOutputPath(e.currentFile)
	if err := e.rebuilder.GenerateDualPDF(e.currentFile, result.TranslatedPDFPath, dualPath); err != nil {
		logger.Warn("failed to generate dual PDF", logger.Err(err))
		dualPath = "" // Clear dual path if generation failed
	}

	return &EnhancedTranslationResult{
		TranslationResult:  *result,
		DualPDFPath:        dualPath,
		LayoutElements:     layoutElements,
		UsedAILayout:       e.useAILayout && len(layoutElements) > 0,
		FormulaCount:       e.countElementsByType(layoutElements, ElementFormula),
		TableCount:         e.countElementsByType(layoutElements, ElementTable),
		ImageCount:         e.countElementsByType(layoutElements, ElementPicture),
	}, nil
}

// detectLayoutForAllPages detects layout for all pages in the PDF
func (e *EnhancedPDFTranslator) detectLayoutForAllPages() ([]LayoutElement, error) {
	if e.layoutDetector == nil {
		return nil, fmt.Errorf("layout detector not initialized")
	}

	var allElements []LayoutElement

	// Get page count from text blocks
	maxPage := 0
	for _, block := range e.textBlocks {
		if block.Page > maxPage {
			maxPage = block.Page
		}
	}

	logger.Info("detecting layout for all pages",
		logger.Int("totalPages", maxPage))

	// Detect layout for each page
	for page := 1; page <= maxPage; page++ {
		elements, err := e.layoutDetector.DetectLayout(e.currentFile, page)
		if err != nil {
			logger.Warn("failed to detect layout for page, skipping",
				logger.Int("page", page),
				logger.Err(err))
			continue
		}
		allElements = append(allElements, elements...)
		
		logger.Debug("page layout detected",
			logger.Int("page", page),
			logger.Int("elements", len(elements)))
	}

	logger.Info("layout detection complete",
		logger.Int("totalElements", len(allElements)))

	return allElements, nil
}

// mergeLayoutAndText merges AI-detected layout with extracted text
func (e *EnhancedPDFTranslator) mergeLayoutAndText(layoutElements []LayoutElement, textBlocks []TextBlock) []TextBlock {
	if len(layoutElements) == 0 {
		// No AI layout, return original text blocks
		logger.Info("no layout elements, using original text blocks")
		return textBlocks
	}

	logger.Info("merging layout with text",
		logger.Int("layoutElements", len(layoutElements)),
		logger.Int("textBlocks", len(textBlocks)))

	// Use the postprocessing function to merge
	merged := MergeLayoutWithText(layoutElements, textBlocks)

	// Count how many blocks were updated
	updated := 0
	for i := range merged {
		if merged[i].BlockType != textBlocks[i].BlockType {
			updated++
		}
	}

	logger.Info("merge complete",
		logger.Int("updatedBlocks", updated))

	return merged
}

// filterTranslatableBlocks filters blocks that should be translated
func (e *EnhancedPDFTranslator) filterTranslatableBlocks(blocks []TextBlock) []TextBlock {
	var translatable []TextBlock

	for _, block := range blocks {
		elementType := ElementType(block.BlockType)
		if elementType.IsTranslatable() {
			translatable = append(translatable, block)
		}
	}

	// Sort by priority
	sort.Slice(translatable, func(i, j int) bool {
		typeI := ElementType(translatable[i].BlockType)
		typeJ := ElementType(translatable[j].BlockType)
		return typeI.Priority() > typeJ.Priority()
	})

	return translatable
}

// countElementsByType counts elements of a specific type
func (e *EnhancedPDFTranslator) countElementsByType(elements []LayoutElement, elementType ElementType) int {
	count := 0
	for _, elem := range elements {
		if elem.Type == elementType {
			count++
		}
	}
	return count
}

// getDualOutputPath generates output path for dual PDF
func (e *EnhancedPDFTranslator) getDualOutputPath(originalPath string) string {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	outputName := fmt.Sprintf("%s-dual%s", name, ext)

	if e.workDir != "" {
		return filepath.Join(e.workDir, outputName)
	}

	return filepath.Join(dir, outputName)
}

// EnhancedTranslationResult extends TranslationResult with additional info
type EnhancedTranslationResult struct {
	TranslationResult
	DualPDFPath    string          `json:"dual_pdf_path,omitempty"`
	LayoutElements []LayoutElement `json:"layout_elements,omitempty"`
	UsedAILayout   bool            `json:"used_ai_layout"`
	FormulaCount   int             `json:"formula_count"`
	TableCount     int             `json:"table_count"`
	ImageCount     int             `json:"image_count"`
}

// OverlayWithPDFCPU uses pdfcpu (pure Go) for PDF operations
// This is a pure Go implementation that doesn't require external dependencies
func (r *PDFRebuilder) OverlayWithPDFCPU(
	originalPath string,
	outputPath string,
	watermarkText string,
) error {
	logger.Info("using pdfcpu for PDF operations",
		logger.String("input", filepath.Base(originalPath)),
		logger.String("output", filepath.Base(outputPath)))

	return AddWatermarkWithPDFCPU(originalPath, outputPath, watermarkText)
}

// ExtractPDFInfo extracts basic PDF information using pdfcpu
func (r *PDFRebuilder) ExtractPDFInfo(pdfPath string) (pages int, err error) {
	return ExtractPDFInfoWithPDFCPU(pdfPath)
}

// SplitPDF splits a PDF into individual pages using pdfcpu
func (r *PDFRebuilder) SplitPDF(pdfPath string, outputDir string) ([]string, error) {
	return SplitPDFWithPDFCPU(pdfPath, outputDir)
}

// MergePDFs merges multiple PDFs into one using pdfcpu
func (r *PDFRebuilder) MergePDFs(inputPaths []string, outputPath string) error {
	return MergePDFsWithPDFCPU(inputPaths, outputPath)
}
