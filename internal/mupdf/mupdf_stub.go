//go:build !mupdf || !cgo

// Package mupdf provides Go bindings for the MuPDF library.
// This is a stub file for when MuPDF is not available.
// Build with -tags mupdf to enable full functionality.
package mupdf

import "errors"

var (
	ErrNotAvailable  = errors.New("mupdf: not available (build with -tags mupdf)")
	ErrOpenDocument  = errors.New("failed to open document")
	ErrInvalidPage   = errors.New("invalid page number")
	ErrExtractText   = errors.New("failed to extract text")
	ErrSaveDocument  = errors.New("failed to save document")
	ErrAddText       = errors.New("failed to add text")
	ErrContextCreate = errors.New("failed to create context")
)

// Context wraps MuPDF fz_context (stub)
type Context struct{}

// NewContext creates a new MuPDF context (stub - returns error)
func NewContext() (*Context, error) {
	return nil, ErrNotAvailable
}

// Close releases the context (stub)
func (c *Context) Close() {}

// Document wraps MuPDF fz_document for reading (stub)
type Document struct{}

// OpenDocument opens a document for reading (stub)
func (c *Context) OpenDocument(filename string) (*Document, error) {
	return nil, ErrNotAvailable
}

// Close releases the document (stub)
func (d *Document) Close() {}

// PageCount returns the number of pages (stub)
func (d *Document) PageCount() int { return 0 }

// PageBounds returns the bounds of a page (stub)
func (d *Document) PageBounds(pageNum int) (x0, y0, x1, y1 float64) {
	return 0, 0, 0, 0
}

// ExtractText extracts text from a page (stub)
func (d *Document) ExtractText(pageNum int) (string, error) {
	return "", ErrNotAvailable
}

// ExtractTextBlocks extracts text blocks with position info (stub)
func (d *Document) ExtractTextBlocks(pageNum int) ([]TextBlock, error) {
	return nil, ErrNotAvailable
}

// PDFDocument wraps MuPDF pdf_document for reading and writing (stub)
type PDFDocument struct{}

// OpenPDFDocument opens a PDF document for reading and writing (stub)
func (c *Context) OpenPDFDocument(filename string) (*PDFDocument, error) {
	return nil, ErrNotAvailable
}

// Close releases the PDF document (stub)
func (d *PDFDocument) Close() {}

// PageCount returns the number of pages (stub)
func (d *PDFDocument) PageCount() int { return 0 }

// Save saves the PDF document to a file (stub)
func (d *PDFDocument) Save(filename string) error {
	return ErrNotAvailable
}

// AddText adds text to a page at the specified position (stub)
func (d *PDFDocument) AddText(pageNum int, text string, x, y, fontSize float64, fontName string) error {
	return ErrNotAvailable
}

// AddRect adds a filled rectangle to a page (stub)
func (d *PDFDocument) AddRect(pageNum int, x, y, w, h float64, r, g, b float64) error {
	return ErrNotAvailable
}

// CoverAndAddText covers original text with white rectangle and adds translated text (stub)
func (d *PDFDocument) CoverAndAddText(pageNum int, x, y, w, h float64, translatedText string, fontSize float64) error {
	return ErrNotAvailable
}

// AddTextWithFont adds text with a specific font file (stub)
func (d *PDFDocument) AddTextWithFont(pageNum int, text string, x, y, fontSize float64, fontPath string) error {
	return ErrNotAvailable
}

// TextBlock represents extracted text with position
type TextBlock struct {
	Text     string
	X, Y     float64
	Width    float64
	Height   float64
	FontSize float64
	Page     int
}

// IsAvailable returns whether MuPDF is available
func IsAvailable() bool {
	return false
}
