// Package pdf provides PDF to image conversion functionality
package pdf

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"

	"latex-translator/internal/logger"
)

// PDFToImageConverter handles PDF page to image conversion
type PDFToImageConverter struct {
	dpi        int
	format     string // png, jpg
	tempDir    string
	usePoppler bool
}

// NewPDFToImageConverter creates a new converter
func NewPDFToImageConverter(dpi int) *PDFToImageConverter {
	return &PDFToImageConverter{
		dpi:        dpi,
		format:     "png",
		usePoppler: checkPopplerAvailable(),
	}
}

// checkPopplerAvailable checks if pdftoppm is available
func checkPopplerAvailable() bool {
	cmd := exec.Command("pdftoppm", "-v")
	err := cmd.Run()
	return err == nil
}

// ConvertPage converts a PDF page to an image
func (c *PDFToImageConverter) ConvertPage(pdfPath string, pageNum int) (image.Image, error) {
	logger.Debug("converting PDF page to image",
		logger.String("pdf", filepath.Base(pdfPath)),
		logger.Int("page", pageNum),
		logger.Int("dpi", c.dpi))

	if c.usePoppler {
		return c.convertWithPoppler(pdfPath, pageNum)
	}

	// Fallback to Go-based conversion
	return c.convertWithGo(pdfPath, pageNum)
}

// convertWithPoppler uses pdftoppm for high-quality conversion
func (c *PDFToImageConverter) convertWithPoppler(pdfPath string, pageNum int) (image.Image, error) {
	// Create temp directory if not exists
	if c.tempDir == "" {
		tempDir, err := os.MkdirTemp("", "pdf2img_*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
		c.tempDir = tempDir
	}

	outputPrefix := filepath.Join(c.tempDir, fmt.Sprintf("page_%d", pageNum))

	// Build pdftoppm command
	args := []string{
		"-f", fmt.Sprintf("%d", pageNum),
		"-l", fmt.Sprintf("%d", pageNum),
		"-" + c.format,
		"-r", fmt.Sprintf("%d", c.dpi),
		"-singlefile",
		pdfPath,
		outputPrefix,
	}

	cmd := exec.Command("pdftoppm", args...)
	
	// Hide window on Windows
	hideWindowOnWindows(cmd)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %w, output: %s", err, string(output))
	}

	// Read the generated image
	imgPath := outputPrefix + "." + c.format
	img, err := loadImage(imgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load image: %w", err)
	}

	// Clean up temp file
	os.Remove(imgPath)

	logger.Debug("page converted successfully",
		logger.Int("width", img.Bounds().Dx()),
		logger.Int("height", img.Bounds().Dy()))

	return img, nil
}

// convertWithGo uses Go-based PDF rendering (fallback)
func (c *PDFToImageConverter) convertWithGo(pdfPath string, pageNum int) (image.Image, error) {
	// TODO: Implement pure Go PDF rendering
	// This is complex and may require additional libraries
	// For now, return an error suggesting to install poppler
	return nil, fmt.Errorf("poppler-utils not found, please install: " +
		"Ubuntu/Debian: apt-get install poppler-utils, " +
		"macOS: brew install poppler, " +
		"Windows: download from https://github.com/oschwartz10612/poppler-windows/releases")
}

// loadImage loads an image from file
func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return img, nil
}

// Cleanup removes temporary files
func (c *PDFToImageConverter) Cleanup() {
	if c.tempDir != "" {
		os.RemoveAll(c.tempDir)
		c.tempDir = ""
	}
}
