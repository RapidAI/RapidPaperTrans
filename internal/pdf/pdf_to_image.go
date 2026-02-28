// Package pdf provides PDF to image conversion using GoPDF2's built-in renderer.
package pdf

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	gopdf "github.com/VantageDataChat/GoPDF2"
	"latex-translator/internal/logger"
)

// ConvertPDFPageToImage converts a single PDF page to an image using GoPDF2.
// pageNum is 1-based (PDF convention). DPI controls resolution (150 recommended for layout detection).
func ConvertPDFPageToImage(pdfPath string, pageNum int, dpi float64) (image.Image, error) {
	logger.Debug("converting PDF page to image via GoPDF2",
		logger.String("pdf", filepath.Base(pdfPath)),
		logger.Int("page", pageNum),
		logger.Float64("dpi", dpi))

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	// GoPDF2 uses 0-based page index
	pageIndex := pageNum - 1

	img, err := gopdf.RenderPageToImage(data, pageIndex, gopdf.RenderOption{
		DPI: dpi,
	})
	if err != nil {
		return nil, fmt.Errorf("GoPDF2 render page %d failed: %w", pageNum, err)
	}

	bounds := img.Bounds()
	logger.Debug("page converted successfully",
		logger.Int("width", bounds.Dx()),
		logger.Int("height", bounds.Dy()))

	return img, nil
}
