//go:build !mupdf || !cgo

// Package pdf provides stub implementations when MuPDF is not available.
package pdf

import "errors"

// MuPDFGenerator stub for when MuPDF is not available
type MuPDFGenerator struct {
	workDir  string
	fontPath string
}

// NewMuPDFGenerator creates a stub generator
func NewMuPDFGenerator(workDir string) *MuPDFGenerator {
	return &MuPDFGenerator{workDir: workDir}
}

// SetFontPath is a no-op in stub
func (g *MuPDFGenerator) SetFontPath(fontPath string) {
	g.fontPath = fontPath
}

// GenerateTranslatedPDF returns error when MuPDF is not available
func (g *MuPDFGenerator) GenerateTranslatedPDF(originalPath string, blocks []TranslatedBlock, outputPath string) error {
	return errors.New("MuPDF not available: build with -tags mupdf")
}

// ExtractTextBlocksWithMuPDF returns error when MuPDF is not available
func ExtractTextBlocksWithMuPDF(pdfPath string) ([]TextBlock, error) {
	return nil, errors.New("MuPDF not available: build with -tags mupdf")
}

// IsMuPDFAvailable returns false when MuPDF is not compiled in
func IsMuPDFAvailable() bool {
	return false
}
