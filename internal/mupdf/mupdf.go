//go:build mupdf && cgo

// Package mupdf provides Go bindings for the MuPDF library.
// This package enables PDF reading, text extraction with position info,
// and PDF modification including text overlay.
//
// Build with: go build -tags mupdf
//
// Requires MuPDF development libraries to be installed.
package mupdf

/*
#cgo CFLAGS: -IC:/msys64/mingw64/include
#cgo LDFLAGS: -LC:/msys64/mingw64/lib -lmupdf -lfreetype -lharfbuzz -lgumbo -lopenjp2 -ljbig2dec -ljpeg -lz -lgdi32 -luser32

#include <stdlib.h>
#include <string.h>
#include <mupdf/fitz.h>
#include <mupdf/pdf.h>

// Helper function to create context with document handlers registered
static fz_context* new_context() {
    fz_context *ctx = fz_new_context(NULL, NULL, FZ_STORE_DEFAULT);
    if (ctx) {
        fz_try(ctx) {
            fz_register_document_handlers(ctx);
        }
        fz_catch(ctx) {
            // Ignore registration errors
        }
    }
    return ctx;
}

// Helper function to open document
static fz_document* open_document(fz_context *ctx, const char *filename) {
    fz_document *doc = NULL;
    fz_try(ctx) {
        doc = fz_open_document(ctx, filename);
    }
    fz_catch(ctx) {
        return NULL;
    }
    return doc;
}

// Helper function to open PDF document
static pdf_document* open_pdf_document(fz_context *ctx, const char *filename) {
    pdf_document *doc = NULL;
    fz_try(ctx) {
        doc = pdf_open_document(ctx, filename);
    }
    fz_catch(ctx) {
        return NULL;
    }
    return doc;
}

// Helper function to get page count
static int get_page_count(fz_context *ctx, fz_document *doc) {
    int count = 0;
    fz_try(ctx) {
        count = fz_count_pages(ctx, doc);
    }
    fz_catch(ctx) {
        return -1;
    }
    return count;
}

// Helper function to get page bounds
static fz_rect get_page_bounds(fz_context *ctx, fz_document *doc, int page_num) {
    fz_rect bounds = fz_empty_rect;
    fz_try(ctx) {
        fz_page *page = fz_load_page(ctx, doc, page_num);
        bounds = fz_bound_page(ctx, page);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        // Return empty rect on error
    }
    return bounds;
}

// Text block structure for extraction
typedef struct {
    char *text;
    float x, y, w, h;
    float font_size;
    int page;
} text_block_t;

// Helper to extract text from a page with position info
static char* extract_page_text(fz_context *ctx, fz_document *doc, int page_num) {
    char *text = NULL;
    fz_try(ctx) {
        fz_page *page = fz_load_page(ctx, doc, page_num);
        fz_stext_page *stext = fz_new_stext_page_from_page(ctx, page, NULL);
        
        // Get text as string
        fz_buffer *buf = fz_new_buffer(ctx, 1024);
        fz_output *out = fz_new_output_with_buffer(ctx, buf);
        fz_print_stext_page_as_text(ctx, out, stext);
        fz_close_output(ctx, out);
        fz_drop_output(ctx, out);
        
        // Copy to result - new API: fz_buffer_extract returns size and takes data pointer
        unsigned char *data = NULL;
        size_t len = fz_buffer_extract(ctx, buf, &data);
        text = (char*)malloc(len + 1);
        if (text) {
            memcpy(text, data, len);
            text[len] = '\0';
        }
        fz_free(ctx, data);
        fz_drop_buffer(ctx, buf);
        
        fz_drop_stext_page(ctx, stext);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        return NULL;
    }
    return text;
}

// Helper to save PDF document
static int save_pdf_document(fz_context *ctx, pdf_document *doc, const char *filename) {
    fz_try(ctx) {
        pdf_save_document(ctx, doc, filename, NULL);
    }
    fz_catch(ctx) {
        return -1;
    }
    return 0;
}

// Helper to add text to PDF page with CJK font support
static int add_text_to_page(fz_context *ctx, pdf_document *doc, int page_num,
                            const char *text, float x, float y, float font_size,
                            const char *font_path) {
    fz_try(ctx) {
        pdf_page *page = pdf_load_page(ctx, doc, page_num);
        
        // Create font - use CJK font if font_path provided, otherwise Base14
        fz_font *font = NULL;
        pdf_obj *font_obj = NULL;
        
        if (font_path && font_path[0] != '\0') {
            // Load CJK font from file
            font = fz_new_font_from_file(ctx, NULL, font_path, 0, 0);
            font_obj = pdf_add_cjk_font(ctx, doc, font, FZ_ADOBE_GB, 0, 1);
        } else {
            // Use Base14 font
            font = fz_new_base14_font(ctx, "Helvetica");
            font_obj = pdf_add_simple_font(ctx, doc, font, PDF_SIMPLE_ENCODING_LATIN);
        }
        
        // Create content stream for text
        fz_buffer *buf = fz_new_buffer(ctx, 1024);
        fz_append_printf(ctx, buf, "BT\n");
        fz_append_printf(ctx, buf, "/F0 %.2f Tf\n", font_size);
        fz_append_printf(ctx, buf, "%.2f %.2f Td\n", x, y);
        
        // For CJK text, we need to encode as hex string
        if (font_path && font_path[0] != '\0') {
            fz_append_string(ctx, buf, "<");
            const unsigned char *p = (const unsigned char *)text;
            while (*p) {
                // Get UTF-8 codepoint
                int c = 0;
                if ((*p & 0x80) == 0) {
                    c = *p++;
                } else if ((*p & 0xE0) == 0xC0) {
                    c = (*p++ & 0x1F) << 6;
                    c |= (*p++ & 0x3F);
                } else if ((*p & 0xF0) == 0xE0) {
                    c = (*p++ & 0x0F) << 12;
                    c |= (*p++ & 0x3F) << 6;
                    c |= (*p++ & 0x3F);
                } else if ((*p & 0xF8) == 0xF0) {
                    c = (*p++ & 0x07) << 18;
                    c |= (*p++ & 0x3F) << 12;
                    c |= (*p++ & 0x3F) << 6;
                    c |= (*p++ & 0x3F);
                } else {
                    p++;
                    continue;
                }
                // Encode as 4-digit hex (CID)
                fz_append_printf(ctx, buf, "%04X", c);
            }
            fz_append_string(ctx, buf, "> Tj\n");
        } else {
            fz_append_printf(ctx, buf, "(%s) Tj\n", text);
        }
        
        fz_append_printf(ctx, buf, "ET\n");
        
        // Add content to page
        pdf_obj *resources = pdf_dict_get(ctx, page->obj, PDF_NAME(Resources));
        if (!resources) {
            resources = pdf_new_dict(ctx, doc, 1);
            pdf_dict_put(ctx, page->obj, PDF_NAME(Resources), resources);
        }
        
        pdf_obj *fonts = pdf_dict_get(ctx, resources, PDF_NAME(Font));
        if (!fonts) {
            fonts = pdf_new_dict(ctx, doc, 1);
            pdf_dict_put(ctx, resources, PDF_NAME(Font), fonts);
        }
        
        pdf_obj *font_name_obj = pdf_new_name(ctx, "F0");
        pdf_dict_put(ctx, fonts, font_name_obj, font_obj);
        
        // Create new content stream
        pdf_obj *contents = pdf_add_stream(ctx, doc, buf, NULL, 0);
        
        // Get existing contents
        pdf_obj *old_contents = pdf_dict_get(ctx, page->obj, PDF_NAME(Contents));
        if (old_contents) {
            pdf_obj *arr = pdf_new_array(ctx, doc, 2);
            if (pdf_is_array(ctx, old_contents)) {
                int n = pdf_array_len(ctx, old_contents);
                for (int i = 0; i < n; i++) {
                    pdf_array_push(ctx, arr, pdf_array_get(ctx, old_contents, i));
                }
            } else {
                pdf_array_push(ctx, arr, old_contents);
            }
            pdf_array_push(ctx, arr, contents);
            pdf_dict_put(ctx, page->obj, PDF_NAME(Contents), arr);
        } else {
            pdf_dict_put(ctx, page->obj, PDF_NAME(Contents), contents);
        }
        
        fz_drop_buffer(ctx, buf);
        if (font) fz_drop_font(ctx, font);
        fz_drop_page(ctx, (fz_page*)page);
    }
    fz_catch(ctx) {
        return -1;
    }
    return 0;
}

// Structure for text block with position
typedef struct {
    char *text;
    float x0, y0, x1, y1;
    float font_size;
} stext_block_info;

// Helper to extract structured text blocks from a page
// Returns array of text blocks with position info
static int extract_stext_blocks(fz_context *ctx, fz_document *doc, int page_num,
                                stext_block_info **blocks_out, int *count_out) {
    *blocks_out = NULL;
    *count_out = 0;
    
    fz_try(ctx) {
        fz_page *page = fz_load_page(ctx, doc, page_num);
        fz_stext_page *stext = fz_new_stext_page_from_page(ctx, page, NULL);
        
        // Count blocks first
        int block_count = 0;
        fz_stext_block *block;
        for (block = stext->first_block; block; block = block->next) {
            if (block->type == FZ_STEXT_BLOCK_TEXT) {
                block_count++;
            }
        }
        
        if (block_count == 0) {
            fz_drop_stext_page(ctx, stext);
            fz_drop_page(ctx, page);
            return 0;
        }
        
        // Allocate array
        stext_block_info *blocks = (stext_block_info*)malloc(block_count * sizeof(stext_block_info));
        if (!blocks) {
            fz_drop_stext_page(ctx, stext);
            fz_drop_page(ctx, page);
            return -1;
        }
        
        // Fill blocks
        int i = 0;
        for (block = stext->first_block; block && i < block_count; block = block->next) {
            if (block->type != FZ_STEXT_BLOCK_TEXT) {
                continue;
            }
            
            // Get block bounds
            blocks[i].x0 = block->bbox.x0;
            blocks[i].y0 = block->bbox.y0;
            blocks[i].x1 = block->bbox.x1;
            blocks[i].y1 = block->bbox.y1;
            
            // Extract text from lines
            fz_buffer *buf = fz_new_buffer(ctx, 256);
            fz_stext_line *line;
            float max_font_size = 0;
            
            for (line = block->u.t.first_line; line; line = line->next) {
                fz_stext_char *ch;
                for (ch = line->first_char; ch; ch = ch->next) {
                    char utf8[8];
                    int len = fz_runetochar(utf8, ch->c);
                    fz_append_data(ctx, buf, utf8, len);
                    if (ch->size > max_font_size) {
                        max_font_size = ch->size;
                    }
                }
                fz_append_byte(ctx, buf, ' ');
            }
            
            // Copy text - new API
            unsigned char *data = NULL;
            size_t len = fz_buffer_extract(ctx, buf, &data);
            blocks[i].text = (char*)malloc(len + 1);
            if (blocks[i].text) {
                memcpy(blocks[i].text, data, len);
                blocks[i].text[len] = '\0';
            }
            blocks[i].font_size = max_font_size > 0 ? max_font_size : 10.0f;
            
            fz_free(ctx, data);
            fz_drop_buffer(ctx, buf);
            i++;
        }
        
        *blocks_out = blocks;
        *count_out = i;
        
        fz_drop_stext_page(ctx, stext);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        return -1;
    }
    return 0;
}

// Helper to free stext blocks
static void free_stext_blocks(stext_block_info *blocks, int count) {
    if (blocks) {
        for (int i = 0; i < count; i++) {
            if (blocks[i].text) {
                free(blocks[i].text);
            }
        }
        free(blocks);
    }
}

// Helper to add rectangle (for covering original text)
static int add_rect_to_page(fz_context *ctx, pdf_document *doc, int page_num,
                            float x, float y, float w, float h,
                            float r, float g, float b) {
    fz_try(ctx) {
        pdf_page *page = pdf_load_page(ctx, doc, page_num);
        
        // Create content stream for rectangle
        fz_buffer *buf = fz_new_buffer(ctx, 128);
        fz_append_printf(ctx, buf, "q\n");
        fz_append_printf(ctx, buf, "%.3f %.3f %.3f rg\n", r, g, b);
        fz_append_printf(ctx, buf, "%.2f %.2f %.2f %.2f re\n", x, y, w, h);
        fz_append_printf(ctx, buf, "f\n");
        fz_append_printf(ctx, buf, "Q\n");
        
        // Add content to page (prepend to draw under text)
        pdf_obj *contents = pdf_add_stream(ctx, doc, buf, NULL, 0);
        pdf_obj *old_contents = pdf_dict_get(ctx, page->obj, PDF_NAME(Contents));
        
        pdf_obj *arr = pdf_new_array(ctx, doc, 2);
        pdf_array_push(ctx, arr, contents);  // Rectangle first (background)
        
        if (old_contents) {
            if (pdf_is_array(ctx, old_contents)) {
                int n = pdf_array_len(ctx, old_contents);
                for (int i = 0; i < n; i++) {
                    pdf_array_push(ctx, arr, pdf_array_get(ctx, old_contents, i));
                }
            } else {
                pdf_array_push(ctx, arr, old_contents);
            }
        }
        
        pdf_dict_put(ctx, page->obj, PDF_NAME(Contents), arr);
        
        fz_drop_buffer(ctx, buf);
        fz_drop_page(ctx, (fz_page*)page);
    }
    fz_catch(ctx) {
        return -1;
    }
    return 0;
}

*/
import "C"

import (
	"errors"
	"unsafe"
)

var (
	ErrOpenDocument  = errors.New("failed to open document")
	ErrInvalidPage   = errors.New("invalid page number")
	ErrExtractText   = errors.New("failed to extract text")
	ErrSaveDocument  = errors.New("failed to save document")
	ErrAddText       = errors.New("failed to add text")
	ErrContextCreate = errors.New("failed to create context")
)

// Context wraps MuPDF fz_context
type Context struct {
	ctx *C.fz_context
}

// NewContext creates a new MuPDF context
func NewContext() (*Context, error) {
	ctx := C.new_context()
	if ctx == nil {
		return nil, ErrContextCreate
	}
	return &Context{ctx: ctx}, nil
}

// Close releases the context
func (c *Context) Close() {
	if c.ctx != nil {
		C.fz_drop_context(c.ctx)
		c.ctx = nil
	}
}

// Document wraps MuPDF fz_document for reading
type Document struct {
	ctx *Context
	doc *C.fz_document
}

// OpenDocument opens a document for reading
func (c *Context) OpenDocument(filename string) (*Document, error) {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	doc := C.open_document(c.ctx, cFilename)
	if doc == nil {
		return nil, ErrOpenDocument
	}

	return &Document{ctx: c, doc: doc}, nil
}

// Close releases the document
func (d *Document) Close() {
	if d.doc != nil {
		C.fz_drop_document(d.ctx.ctx, d.doc)
		d.doc = nil
	}
}

// PageCount returns the number of pages
func (d *Document) PageCount() int {
	return int(C.get_page_count(d.ctx.ctx, d.doc))
}

// PageBounds returns the bounds of a page
func (d *Document) PageBounds(pageNum int) (x0, y0, x1, y1 float64) {
	rect := C.get_page_bounds(d.ctx.ctx, d.doc, C.int(pageNum))
	return float64(rect.x0), float64(rect.y0), float64(rect.x1), float64(rect.y1)
}

// ExtractText extracts text from a page
func (d *Document) ExtractText(pageNum int) (string, error) {
	if pageNum < 0 || pageNum >= d.PageCount() {
		return "", ErrInvalidPage
	}

	cText := C.extract_page_text(d.ctx.ctx, d.doc, C.int(pageNum))
	if cText == nil {
		return "", ErrExtractText
	}
	defer C.free(unsafe.Pointer(cText))

	return C.GoString(cText), nil
}

// PDFDocument wraps MuPDF pdf_document for reading and writing
type PDFDocument struct {
	ctx *Context
	doc *C.pdf_document
}

// OpenPDFDocument opens a PDF document for reading and writing
func (c *Context) OpenPDFDocument(filename string) (*PDFDocument, error) {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	doc := C.open_pdf_document(c.ctx, cFilename)
	if doc == nil {
		return nil, ErrOpenDocument
	}

	return &PDFDocument{ctx: c, doc: doc}, nil
}

// Close releases the PDF document
func (d *PDFDocument) Close() {
	if d.doc != nil {
		C.pdf_drop_document(d.ctx.ctx, d.doc)
		d.doc = nil
	}
}

// PageCount returns the number of pages
func (d *PDFDocument) PageCount() int {
	return int(C.pdf_count_pages(d.ctx.ctx, d.doc))
}

// Save saves the PDF document to a file
func (d *PDFDocument) Save(filename string) error {
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	if C.save_pdf_document(d.ctx.ctx, d.doc, cFilename) != 0 {
		return ErrSaveDocument
	}
	return nil
}

// AddText adds text to a page at the specified position
// fontPath can be empty for ASCII text, or path to TTF/OTF for CJK
func (d *PDFDocument) AddText(pageNum int, text string, x, y, fontSize float64, fontPath string) error {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	var cFontPath *C.char
	if fontPath != "" {
		cFontPath = C.CString(fontPath)
		defer C.free(unsafe.Pointer(cFontPath))
	} else {
		cFontPath = C.CString("")
		defer C.free(unsafe.Pointer(cFontPath))
	}

	if C.add_text_to_page(d.ctx.ctx, d.doc, C.int(pageNum),
		cText, C.float(x), C.float(y), C.float(fontSize), cFontPath) != 0 {
		return ErrAddText
	}
	return nil
}

// AddRect adds a filled rectangle to a page (useful for covering original text)
func (d *PDFDocument) AddRect(pageNum int, x, y, w, h float64, r, g, b float64) error {
	if C.add_rect_to_page(d.ctx.ctx, d.doc, C.int(pageNum),
		C.float(x), C.float(y), C.float(w), C.float(h),
		C.float(r), C.float(g), C.float(b)) != 0 {
		return ErrAddText
	}
	return nil
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

// ExtractTextBlocks extracts text blocks with position info from a page
// This provides structured text extraction similar to BabelDOC's approach
func (d *Document) ExtractTextBlocks(pageNum int) ([]TextBlock, error) {
	if pageNum < 0 || pageNum >= d.PageCount() {
		return nil, ErrInvalidPage
	}

	var blocksPtr *C.stext_block_info
	var count C.int

	result := C.extract_stext_blocks(d.ctx.ctx, d.doc, C.int(pageNum), &blocksPtr, &count)
	if result != 0 {
		return nil, ErrExtractText
	}

	if count == 0 || blocksPtr == nil {
		return nil, nil
	}
	defer C.free_stext_blocks(blocksPtr, count)

	// Convert C blocks to Go blocks
	blocks := make([]TextBlock, int(count))
	blockSize := unsafe.Sizeof(C.stext_block_info{})

	for i := 0; i < int(count); i++ {
		blockPtr := (*C.stext_block_info)(unsafe.Pointer(uintptr(unsafe.Pointer(blocksPtr)) + uintptr(i)*blockSize))
		blocks[i] = TextBlock{
			Text:     C.GoString(blockPtr.text),
			X:        float64(blockPtr.x0),
			Y:        float64(blockPtr.y0),
			Width:    float64(blockPtr.x1 - blockPtr.x0),
			Height:   float64(blockPtr.y1 - blockPtr.y0),
			FontSize: float64(blockPtr.font_size),
			Page:     pageNum,
		}
	}

	return blocks, nil
}

// CoverAndAddText covers original text with white rectangle and adds translated text
// This is the main function for PDF translation overlay
func (d *PDFDocument) CoverAndAddText(pageNum int, x, y, w, h float64, translatedText string, fontSize float64) error {
	// First, add white rectangle to cover original text
	if err := d.AddRect(pageNum, x, y, w, h, 1.0, 1.0, 1.0); err != nil {
		return err
	}

	// Then add translated text
	// Adjust Y position to account for font baseline
	textY := y + h - fontSize
	return d.AddText(pageNum, translatedText, x, textY, fontSize, "")
}

// AddTextWithFont adds text with a specific font file (for CJK support)
func (d *PDFDocument) AddTextWithFont(pageNum int, text string, x, y, fontSize float64, fontPath string) error {
	// For CJK fonts, we need to embed the font
	// This is a simplified version - full implementation would load TTF/OTF
	return d.AddText(pageNum, text, x, y, fontSize, "")
}

// IsAvailable returns whether MuPDF is available
func IsAvailable() bool {
	return true
}
