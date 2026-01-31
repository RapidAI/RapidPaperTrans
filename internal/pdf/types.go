// Package pdf provides PDF translation functionality including text extraction,
// batch translation, and PDF generation with translated content.
package pdf

import "time"

// PDFInfo PDF 文件信息
type PDFInfo struct {
	FilePath  string `json:"file_path"`
	FileName  string `json:"file_name"`
	PageCount int    `json:"page_count"`
	FileSize  int64  `json:"file_size"`
	IsTextPDF bool   `json:"is_text_pdf"`
}

// TextBlock 文本块
type TextBlock struct {
	ID        string  `json:"id"`
	Page      int     `json:"page"`
	Text      string  `json:"text"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	FontSize  float64 `json:"font_size"`
	FontName  string  `json:"font_name"`
	IsBold    bool    `json:"is_bold"`
	IsItalic  bool    `json:"is_italic"`
	BlockType string  `json:"block_type"` // paragraph, heading, caption, etc.
	Rotation  int     `json:"rotation"`   // text rotation in degrees (0, 90, 180, 270)
}

// TranslatedBlock 翻译后的文本块
type TranslatedBlock struct {
	TextBlock
	TranslatedText string `json:"translated_text"`
	FromCache      bool   `json:"from_cache"`
}

// TranslationBatch 翻译批次
type TranslationBatch struct {
	Blocks     []TextBlock `json:"blocks"`
	TotalChars int         `json:"total_chars"`
	BatchIndex int         `json:"batch_index"`
}

// PDFPhase PDF 处理阶段
type PDFPhase string

const (
	PDFPhaseIdle        PDFPhase = "idle"
	PDFPhaseLoading     PDFPhase = "loading"
	PDFPhaseExtracting  PDFPhase = "extracting"
	PDFPhaseTranslating PDFPhase = "translating"
	PDFPhaseGenerating  PDFPhase = "generating"
	PDFPhaseComplete    PDFPhase = "complete"
	PDFPhaseError       PDFPhase = "error"
)

// PDFStatus PDF 处理状态
type PDFStatus struct {
	Phase           PDFPhase `json:"phase"`
	Progress        int      `json:"progress"`
	Message         string   `json:"message"`
	TotalBlocks     int      `json:"total_blocks"`
	CompletedBlocks int      `json:"completed_blocks"`
	CachedBlocks    int      `json:"cached_blocks"`
	Error           string   `json:"error,omitempty"`
}

// TranslationResult 翻译结果
type TranslationResult struct {
	OriginalPDFPath   string                    `json:"original_pdf_path"`
	TranslatedPDFPath string                    `json:"translated_pdf_path"`
	TotalBlocks       int                       `json:"total_blocks"`
	TranslatedBlocks  int                       `json:"translated_blocks"`
	CachedBlocks      int                       `json:"cached_blocks"`
	TokensUsed        int                       `json:"tokens_used"`
	PageCountResult   *PageCountResult          `json:"page_count_result,omitempty"`   // 页数检测结果
	ContentValidation *ContentValidationResult  `json:"content_validation,omitempty"`  // 内容完整性检测结果
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Hash        string    `json:"hash"`
	Original    string    `json:"original"`
	Translation string    `json:"translation"`
	CreatedAt   time.Time `json:"created_at"`
}

// CacheFile 缓存文件结构
type CacheFile struct {
	Version string       `json:"version"`
	Entries []CacheEntry `json:"entries"`
}

// PDFErrorCode 错误代码枚举
type PDFErrorCode string

const (
	ErrPDFNotFound     PDFErrorCode = "PDF_NOT_FOUND"
	ErrPDFInvalid      PDFErrorCode = "PDF_INVALID"
	ErrPDFEncrypted    PDFErrorCode = "PDF_ENCRYPTED"
	ErrPDFCorrupted    PDFErrorCode = "PDF_CORRUPTED"
	ErrPDFNoText       PDFErrorCode = "PDF_NO_TEXT"
	ErrExtractFailed   PDFErrorCode = "EXTRACT_FAILED"
	ErrTranslateFailed PDFErrorCode = "TRANSLATE_FAILED"
	ErrGenerateFailed  PDFErrorCode = "GENERATE_FAILED"
	ErrCacheFailed     PDFErrorCode = "CACHE_FAILED"
	ErrAPIFailed       PDFErrorCode = "API_FAILED"
	ErrCancelled       PDFErrorCode = "CANCELLED"
)

// PDFError PDF 处理错误
type PDFError struct {
	Code    PDFErrorCode `json:"code"`
	Message string       `json:"message"`
	Details string       `json:"details,omitempty"`
	Page    int          `json:"page,omitempty"`
	Cause   error        `json:"-"`
}

// Error implements the error interface for PDFError
func (e *PDFError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// Unwrap returns the underlying cause of the error
func (e *PDFError) Unwrap() error {
	return e.Cause
}

// NewPDFError creates a new PDFError with the given code, message, and optional cause
func NewPDFError(code PDFErrorCode, message string, cause error) *PDFError {
	return &PDFError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewPDFErrorWithDetails creates a new PDFError with details
func NewPDFErrorWithDetails(code PDFErrorCode, message, details string, cause error) *PDFError {
	return &PDFError{
		Code:    code,
		Message: message,
		Details: details,
		Cause:   cause,
	}
}

// NewPDFErrorWithPage creates a new PDFError with page information
func NewPDFErrorWithPage(code PDFErrorCode, message string, page int, cause error) *PDFError {
	return &PDFError{
		Code:    code,
		Message: message,
		Page:    page,
		Cause:   cause,
	}
}

// IsValidPhase checks if the given phase is a valid PDFPhase
func IsValidPhase(phase PDFPhase) bool {
	switch phase {
	case PDFPhaseIdle, PDFPhaseLoading, PDFPhaseExtracting,
		PDFPhaseTranslating, PDFPhaseGenerating, PDFPhaseComplete, PDFPhaseError:
		return true
	default:
		return false
	}
}

// IsValidStatus checks if the PDFStatus has valid values
func (s *PDFStatus) IsValidStatus() bool {
	return IsValidPhase(s.Phase) &&
		s.Progress >= 0 && s.Progress <= 100 &&
		s.CompletedBlocks <= s.TotalBlocks
}

// IsValidTextBlock checks if the TextBlock has valid values
func (b *TextBlock) IsValidTextBlock() bool {
	return b.X >= 0 && b.Y >= 0 &&
		b.Width >= 0 && b.Height >= 0 &&
		b.Page > 0 && len(b.Text) > 0 &&
		len(b.BlockType) > 0
}
