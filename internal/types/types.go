// Package types defines core data types and enums for the LaTeX translator application.
package types

// Config 应用配置
type Config struct {
	OpenAIAPIKey    string `json:"openai_api_key"`
	OpenAIBaseURL   string `json:"openai_base_url"`   // OpenAI 兼容 API 的 Base URL
	OpenAIModel     string `json:"openai_model"`
	ContextWindow   int    `json:"context_window"`    // 上下文窗口大小（tokens），用于 LaTeX 和 PDF 翻译的批次大小控制
	DefaultCompiler string `json:"default_compiler"`  // "pdflatex" 或 "xelatex"
	WorkDirectory   string `json:"work_directory"`
	LastInput       string `json:"last_input"`        // 最后一次输入的 ID/URL/路径
	Concurrency     int    `json:"concurrency"`       // 翻译并发数，用于 LaTeX 和 PDF 翻译的并发批次处理，默认为 3
	// GitHub 分享配置
	GitHubToken     string `json:"github_token"`      // GitHub Personal Access Token
	GitHubOwner     string `json:"github_owner"`      // GitHub 仓库所有者
	GitHubRepo      string `json:"github_repo"`       // GitHub 仓库名称
	// 浏览库配置
	LibraryPageSize int    `json:"library_page_size"` // 浏览库每页显示数量，默认20
}

// SourceType 源码类型枚举
type SourceType string

const (
	SourceTypeURL      SourceType = "url"
	SourceTypeArxivID  SourceType = "arxiv_id"
	SourceTypeLocalZip SourceType = "local_zip"
	SourceTypeLocalPDF SourceType = "local_pdf"
)

// SourceInfo 源码信息
type SourceInfo struct {
	SourceType  SourceType `json:"source_type"` // URL, ArxivID, LocalZip
	OriginalRef string     `json:"original_ref"`
	ExtractDir  string     `json:"extract_dir"`
	MainTexFile string     `json:"main_tex_file"`
	AllTexFiles []string   `json:"all_tex_files"`
}

// ProcessPhase 处理阶段枚举
type ProcessPhase string

const (
	PhaseIdle        ProcessPhase = "idle"
	PhaseDownloading ProcessPhase = "downloading"
	PhaseExtracting  ProcessPhase = "extracting"
	PhaseCompiling   ProcessPhase = "compiling"
	PhaseTranslating ProcessPhase = "translating"
	PhaseValidating  ProcessPhase = "validating"
	PhaseComplete    ProcessPhase = "complete"
	PhaseError       ProcessPhase = "error"
)

// Status 处理状态
type Status struct {
	Phase    ProcessPhase `json:"phase"`
	Progress int          `json:"progress"` // 0-100
	Message  string       `json:"message"`
	Error    string       `json:"error,omitempty"`
}

// ProcessResult 处理结果
type ProcessResult struct {
	OriginalPDFPath   string      `json:"original_pdf_path"`
	TranslatedPDFPath string      `json:"translated_pdf_path"`
	BilingualPDFPath  string      `json:"bilingual_pdf_path"` // 双语并排 PDF 路径
	SourceInfo        *SourceInfo `json:"source_info"`
	SourceID          string      `json:"source_id"` // arXiv ID 或 zip 文件名（不含扩展名）
}

// TranslationResult 翻译结果
type TranslationResult struct {
	OriginalContent   string `json:"original_content"`
	TranslatedContent string `json:"translated_content"`
	TokensUsed        int    `json:"tokens_used"`
}

// ValidationResult 语法验证结果
type ValidationResult struct {
	IsValid bool          `json:"is_valid"`
	Errors  []SyntaxError `json:"errors"`
}

// SyntaxError 语法错误
type SyntaxError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// CompileResult 编译结果
type CompileResult struct {
	Success  bool   `json:"success"`
	PDFPath  string `json:"pdf_path"`
	Log      string `json:"log"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// ErrorCode 错误代码枚举
type ErrorCode string

const (
	ErrNetwork      ErrorCode = "NETWORK_ERROR"
	ErrDownload     ErrorCode = "DOWNLOAD_ERROR"
	ErrExtract      ErrorCode = "EXTRACT_ERROR"
	ErrFileNotFound ErrorCode = "FILE_NOT_FOUND"
	ErrInvalidInput ErrorCode = "INVALID_INPUT"
	ErrAPICall      ErrorCode = "API_CALL_ERROR"
	ErrAPIRateLimit ErrorCode = "API_RATE_LIMIT"
	ErrCompile      ErrorCode = "COMPILE_ERROR"
	ErrConfig       ErrorCode = "CONFIG_ERROR"
	ErrInternal     ErrorCode = "INTERNAL_ERROR"
)

// AppError 应用错误
type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
	Cause   error     `json:"-"`
}

// Error implements the error interface for AppError
func (e *AppError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// Unwrap returns the underlying cause of the error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// NewAppError creates a new AppError with the given code, message, and optional cause
func NewAppError(code ErrorCode, message string, cause error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewAppErrorWithDetails creates a new AppError with details
func NewAppErrorWithDetails(code ErrorCode, message, details string, cause error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Details: details,
		Cause:   cause,
	}
}
