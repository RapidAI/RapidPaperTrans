// Package pdf provides PDF overlay functionality using pdfcpu (pure Go)
package pdf

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"latex-translator/internal/logger"
)

// PDFCPUOverlayGenerator handles PDF operations using pdfcpu (pure Go)
type PDFCPUOverlayGenerator struct {
	workDir string
}

// NewPDFCPUOverlayGenerator creates a new pdfcpu overlay generator
func NewPDFCPUOverlayGenerator(workDir string) *PDFCPUOverlayGenerator {
	return &PDFCPUOverlayGenerator{
		workDir: workDir,
	}
}

// ExtractPDFInfoWithPDFCPU extracts basic PDF information using pdfcpu
func ExtractPDFInfoWithPDFCPU(pdfPath string) (pages int, err error) {
	ctx, err := api.ReadContextFile(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PDF: %w", err)
	}

	return ctx.PageCount, nil
}

// MergePDFsWithPDFCPU merges multiple PDFs into one using pdfcpu
func MergePDFsWithPDFCPU(inputPaths []string, outputPath string) error {
	logger.Info("merging PDFs with pdfcpu",
		logger.Int("count", len(inputPaths)),
		logger.String("output", filepath.Base(outputPath)))

	if err := api.MergeCreateFile(inputPaths, outputPath, false, nil); err != nil {
		return fmt.Errorf("failed to merge PDFs: %w", err)
	}

	logger.Info("PDFs merged successfully")
	return nil
}

// SplitPDFWithPDFCPU splits a PDF into individual pages using pdfcpu
func SplitPDFWithPDFCPU(inputPath, outputDir string) ([]string, error) {
	logger.Info("splitting PDF with pdfcpu",
		logger.String("input", filepath.Base(inputPath)),
		logger.String("outputDir", outputDir))

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Split PDF
	if err := api.SplitFile(inputPath, outputDir, 1, nil); err != nil {
		return nil, fmt.Errorf("failed to split PDF: %w", err)
	}

	// List generated files
	files, err := filepath.Glob(filepath.Join(outputDir, "*.pdf"))
	if err != nil {
		return nil, fmt.Errorf("failed to list split files: %w", err)
	}

	logger.Info("PDF split successfully", logger.Int("pages", len(files)))
	return files, nil
}

// AddWatermarkWithPDFCPU adds a watermark to a PDF using pdfcpu
func AddWatermarkWithPDFCPU(inputPath, outputPath, watermarkText string) error {
	logger.Info("adding watermark with pdfcpu",
		logger.String("input", filepath.Base(inputPath)),
		logger.String("watermark", watermarkText))

	// Create watermark configuration
	// Using simple text watermark with default settings
	wm, err := api.TextWatermark(watermarkText, "", false, false, 0)
	if err != nil {
		return fmt.Errorf("failed to create watermark: %w", err)
	}

	// Add watermark
	if err := api.AddWatermarksFile(inputPath, outputPath, nil, wm, nil); err != nil {
		return fmt.Errorf("failed to add watermark: %w", err)
	}

	logger.Info("watermark added successfully")
	return nil
}
