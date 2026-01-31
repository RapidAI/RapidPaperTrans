// Package pdf provides PDF translation functionality including text extraction,
// batch translation, and PDF generation with translated content.
package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

// PDFTranslator 是 PDF 翻译功能的主控制器
// Requirements: 1.1, 3.5, 5.5 - 加载 PDF、执行翻译流程、显示处理状态
type PDFTranslator struct {
	ctx              context.Context
	cancel           context.CancelFunc
	config           *types.Config
	parser           *PDFParser
	translator       *BatchTranslator
	generator        *PDFGenerator
	cache            *TranslationCache
	workDir          string
	currentFile      string
	status           *PDFStatus
	textBlocks       []TextBlock
	translatedBlocks []TranslatedBlock
	mu               sync.RWMutex
	
	// Callback for page completion events (for progressive display)
	pageCompleteCallback func(currentPage, totalPages int, outputPath string)
}

// PDFTranslatorConfig holds configuration options for creating a PDFTranslator
type PDFTranslatorConfig struct {
	Config    *types.Config
	WorkDir   string
	CachePath string
}

// NewPDFTranslator creates a new PDFTranslator with the given configuration
func NewPDFTranslator(cfg PDFTranslatorConfig) *PDFTranslator {
	ctx, cancel := context.WithCancel(context.Background())

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = os.TempDir()
	}

	// Create cache path if not provided
	cachePath := cfg.CachePath
	if cachePath == "" {
		cachePath = filepath.Join(workDir, "pdf_translation_cache.json")
	}

	// Create components
	parser := NewPDFParser(workDir)
	cache := NewTranslationCache(cachePath)

	// Create batch translator with config
	var translator *BatchTranslator
	if cfg.Config != nil {
		translator = NewBatchTranslator(BatchTranslatorConfig{
			APIKey:        cfg.Config.OpenAIAPIKey,
			BaseURL:       cfg.Config.OpenAIBaseURL,
			Model:         cfg.Config.OpenAIModel,
			ContextWindow: cfg.Config.ContextWindow,
			Concurrency:   cfg.Config.Concurrency,
		})
	}

	generator := NewPDFGenerator(workDir)

	return &PDFTranslator{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg.Config,
		parser:    parser,
		translator: translator,
		generator: generator,
		cache:     cache,
		workDir:   workDir,
		status:    newIdleStatus(),
	}
}

// newIdleStatus creates a new idle status
func newIdleStatus() *PDFStatus {
	return &PDFStatus{
		Phase:           PDFPhaseIdle,
		Progress:        0,
		Message:         "",
		TotalBlocks:     0,
		CompletedBlocks: 0,
		CachedBlocks:    0,
	}
}

// LoadPDF 加载 PDF 文件并提取文本
// Requirements: 1.1 - 加载 PDF 文件并显示在左侧预览区域
// Property 1: PDF 加载有效性 - 对于有效的 PDF 文件路径，应成功返回 PDFInfo 对象
func (p *PDFTranslator) LoadPDF(filePath string) (*PDFInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger.Info("loading PDF file", logger.String("path", filePath))

	// Update status to loading
	p.updateStatusLocked(PDFPhaseLoading, 0, "正在加载 PDF 文件...")

	// Check if context is cancelled
	select {
	case <-p.ctx.Done():
		p.updateStatusLocked(PDFPhaseError, 0, "操作已取消")
		return nil, NewPDFError(ErrCancelled, "操作已取消", p.ctx.Err())
	default:
	}

	// Get PDF info
	pdfInfo, err := p.parser.GetPDFInfo(filePath)
	if err != nil {
		logger.Error("failed to get PDF info", err, logger.String("path", filePath))
		p.updateStatusLocked(PDFPhaseError, 0, err.Error())
		p.status.Error = err.Error()
		return nil, err
	}

	// Check if PDF contains extractable text
	if !pdfInfo.IsTextPDF {
		errMsg := "该 PDF 为扫描件，不支持直接翻译"
		logger.Warn(errMsg, logger.String("path", filePath))
		p.updateStatusLocked(PDFPhaseError, 0, errMsg)
		p.status.Error = errMsg
		return nil, NewPDFError(ErrPDFNoText, errMsg, nil)
	}

	// Update status to extracting
	p.updateStatusLocked(PDFPhaseExtracting, 10, "正在提取文本...")

	// Extract text blocks
	textBlocks, err := p.parser.ExtractText(filePath)
	if err != nil {
		logger.Error("failed to extract text", err, logger.String("path", filePath))
		p.updateStatusLocked(PDFPhaseError, 0, err.Error())
		p.status.Error = err.Error()
		return nil, err
	}

	// Store the extracted text blocks and file path
	p.currentFile = filePath
	p.textBlocks = textBlocks
	p.translatedBlocks = nil

	// Update status to idle (ready for translation)
	p.updateStatusLocked(PDFPhaseIdle, 100, "PDF 加载完成，准备翻译")
	p.status.TotalBlocks = len(textBlocks)

	logger.Info("PDF loaded successfully",
		logger.String("path", filePath),
		logger.Int("pageCount", pdfInfo.PageCount),
		logger.Int("textBlocks", len(textBlocks)))

	return pdfInfo, nil
}

// TranslatePDF 翻译 PDF 内容
// Requirements: 3.5 - 显示翻译进度（已完成块数/总块数）
// Property 7: 状态有效性 - PDFStatus的Phase应是有效枚举值，Progress在0-100范围内
// Uses PyMuPDF for extraction and overlay with progressive page display
func (p *PDFTranslator) TranslatePDF() (*TranslationResult, error) {
	p.mu.Lock()

	logger.Info("starting PDF translation", logger.String("file", p.currentFile))

	// Check if PDF is loaded
	if p.currentFile == "" {
		p.mu.Unlock()
		return nil, NewPDFError(ErrTranslateFailed, "请先加载 PDF 文件", nil)
	}

	// Check if config is available
	if p.config == nil || p.config.OpenAIAPIKey == "" {
		p.mu.Unlock()
		return nil, NewPDFError(ErrTranslateFailed, "翻译器未配置，请检查 API 设置", nil)
	}

	p.updateStatusLocked(PDFPhaseTranslating, 5, "正在准备翻译...")
	pageCallback := p.pageCompleteCallback
	p.mu.Unlock()

	outputPath := p.generator.GetOutputPath(p.currentFile)
	babelTranslator := NewBabelDocTranslator(BabelDocConfig{WorkDir: p.workDir})

	// Use PyMuPDF-based translation with progressive page updates
	err := babelTranslator.TranslatePDFWithPyMuPDFProgressive(
		p.currentFile,
		outputPath,
		p.config.OpenAIAPIKey,
		p.config.OpenAIBaseURL,
		p.config.OpenAIModel,
		func(msg string) {
			p.mu.Lock()
			p.status.Message = msg
			p.mu.Unlock()
		},
		func(currentPage, totalPages int, outPath string) {
			// Update progress based on page completion
			p.mu.Lock()
			progress := 10 + (currentPage * 80 / totalPages)
			if progress > 90 {
				progress = 90
			}
			p.status.Progress = progress
			p.status.Message = fmt.Sprintf("已翻译 %d/%d 页", currentPage, totalPages)
			p.mu.Unlock()
			
			// Call external page callback if set
			if pageCallback != nil {
				pageCallback(currentPage, totalPages, outPath)
			}
		},
	)

	if err != nil {
		p.mu.Lock()
		p.updateStatusLocked(PDFPhaseError, 0, err.Error())
		p.status.Error = err.Error()
		p.mu.Unlock()
		return nil, err
	}

	// Check page count difference
	var pageCountResult *PageCountResult
	if pcResult, err := babelTranslator.CheckPageCountDifference(p.currentFile, outputPath); err != nil {
		logger.Warn("failed to check page count", logger.Err(err))
	} else {
		pageCountResult = pcResult
	}

	// Validate content completeness (sections, appendices, etc.)
	p.mu.Lock()
	p.status.Message = "正在验证内容完整性..."
	p.mu.Unlock()

	var contentValidation *ContentValidationResult
	contentValidator := NewContentValidator(p.workDir)
	if cvResult, err := contentValidator.ValidateContent(p.currentFile, outputPath); err != nil {
		logger.Warn("failed to validate content completeness", logger.Err(err))
	} else {
		contentValidation = cvResult
		if !cvResult.IsComplete {
			logger.Warn("content validation detected missing sections",
				logger.Int("missingSections", len(cvResult.MissingSections)),
				logger.Float64("score", cvResult.Score))
		}
	}

	// Update status to complete
	p.mu.Lock()
	p.updateStatusLocked(PDFPhaseComplete, 100, "翻译完成")
	p.mu.Unlock()

	result := &TranslationResult{
		OriginalPDFPath:     p.currentFile,
		TranslatedPDFPath:   outputPath,
		TotalBlocks:         0,
		TranslatedBlocks:    0,
		CachedBlocks:        0,
		PageCountResult:     pageCountResult,
		ContentValidation:   contentValidation,
	}

	logger.Info("PDF translation completed",
		logger.String("output", outputPath))

	return result, nil
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// translateBlocksWithProgress translates blocks and updates progress
func (p *PDFTranslator) translateBlocksWithProgress(blocks []TextBlock, startCompleted, total int) ([]TranslatedBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	// Check context before starting
	select {
	case <-p.ctx.Done():
		return nil, NewPDFError(ErrCancelled, "翻译已取消", p.ctx.Err())
	default:
	}

	// Create progress callback that updates the translator's status
	progressCallback := func(completed, totalBlocks int) {
		p.mu.Lock()
		// completed here is relative to the uncached blocks being translated
		// We need to add startCompleted to get the absolute progress
		absoluteCompleted := startCompleted + completed
		p.status.CompletedBlocks = absoluteCompleted
		p.updateProgressLocked(absoluteCompleted, total)
		p.mu.Unlock()
	}

	// Use TranslateWithRetryAndProgress for robust translation with progress updates
	translatedBlocks, err := p.translator.TranslateWithRetryAndProgress(blocks, DefaultMaxRetries, progressCallback)
	if err != nil {
		return nil, err
	}

	return translatedBlocks, nil
}

// updateProgressLocked updates the progress percentage (must be called with lock held)
// Property 7: 状态有效性 - Progress应在0-100范围内
func (p *PDFTranslator) updateProgressLocked(completed, total int) {
	if total <= 0 {
		p.status.Progress = 0
		return
	}

	// Calculate progress (10-90% for translation phase, leaving room for loading and generating)
	progress := 10 + (completed * 80 / total)
	if progress > 90 {
		progress = 90
	}
	if progress < 0 {
		progress = 0
	}

	p.status.Progress = progress
	p.status.Message = fmt.Sprintf("正在翻译... (%d/%d)", completed, total)
}

// updateStatusLocked updates the status (must be called with lock held)
// Property 7: 状态有效性 - Phase应是有效的PDFPhase枚举值
func (p *PDFTranslator) updateStatusLocked(phase PDFPhase, progress int, message string) {
	// Validate phase
	if !IsValidPhase(phase) {
		logger.Warn("invalid phase, defaulting to error", logger.String("phase", string(phase)))
		phase = PDFPhaseError
	}

	// Clamp progress to 0-100
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	p.status.Phase = phase
	p.status.Progress = progress
	p.status.Message = message

	// Clear error if not in error phase
	if phase != PDFPhaseError {
		p.status.Error = ""
	}
}

// GetStatus 获取当前处理状态
// Requirements: 5.5 - 显示当前处理状态（加载中、提取中、翻译中、生成中等）
// Property 7: 状态有效性 - PDFStatus的Phase应是有效枚举值，Progress在0-100范围内
func (p *PDFTranslator) GetStatus() *PDFStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent external modification
	return &PDFStatus{
		Phase:           p.status.Phase,
		Progress:        p.status.Progress,
		Message:         p.status.Message,
		TotalBlocks:     p.status.TotalBlocks,
		CompletedBlocks: p.status.CompletedBlocks,
		CachedBlocks:    p.status.CachedBlocks,
		Error:           p.status.Error,
	}
}

// CancelTranslation 取消翻译
// Requirements: 7.6 - 如果翻译过程中断，保存已完成的翻译进度以便恢复
func (p *PDFTranslator) CancelTranslation() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger.Info("cancelling translation")

	// Cancel the context to stop ongoing operations
	p.cancel()

	// Save any completed translations to cache
	if len(p.translatedBlocks) > 0 {
		for _, block := range p.translatedBlocks {
			if block.TranslatedText != "" && !block.FromCache {
				p.cache.Set(block.Text, block.TranslatedText)
			}
		}

		if err := p.cache.Save(); err != nil {
			logger.Warn("failed to save cache on cancel", logger.Err(err))
		}
	}

	// Update status
	p.updateStatusLocked(PDFPhaseIdle, 0, "翻译已取消，进度已保存")

	// Create a new context for future operations
	p.ctx, p.cancel = context.WithCancel(context.Background())

	return nil
}

// GetTranslatedPDFPath 获取翻译后 PDF 路径
func (p *PDFTranslator) GetTranslatedPDFPath() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.currentFile == "" {
		return ""
	}

	return p.generator.GetOutputPath(p.currentFile)
}

// GetCurrentFile returns the currently loaded PDF file path
func (p *PDFTranslator) GetCurrentFile() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentFile
}

// GetTextBlocks returns the extracted text blocks from the current PDF
func (p *PDFTranslator) GetTextBlocks() []TextBlock {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent external modification
	if p.textBlocks == nil {
		return nil
	}

	blocks := make([]TextBlock, len(p.textBlocks))
	copy(blocks, p.textBlocks)
	return blocks
}

// GetTranslatedBlocks returns the translated text blocks
func (p *PDFTranslator) GetTranslatedBlocks() []TranslatedBlock {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent external modification
	if p.translatedBlocks == nil {
		return nil
	}

	blocks := make([]TranslatedBlock, len(p.translatedBlocks))
	copy(blocks, p.translatedBlocks)
	return blocks
}

// Reset resets the translator to its initial state
func (p *PDFTranslator) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger.Info("resetting PDF translator")

	// Cancel any ongoing operations
	p.cancel()

	// Clear state
	p.currentFile = ""
	p.textBlocks = nil
	p.translatedBlocks = nil
	p.status = newIdleStatus()

	// Create a new context
	p.ctx, p.cancel = context.WithCancel(context.Background())
}

// UpdateConfig updates the translator configuration
func (p *PDFTranslator) UpdateConfig(config *types.Config) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config

	// Recreate the batch translator with new config
	if config != nil {
		p.translator = NewBatchTranslator(BatchTranslatorConfig{
			APIKey:        config.OpenAIAPIKey,
			BaseURL:       config.OpenAIBaseURL,
			Model:         config.OpenAIModel,
			ContextWindow: config.ContextWindow,
			Concurrency:   config.Concurrency,
		})
	}
}

// SetWorkDir sets the working directory
func (p *PDFTranslator) SetWorkDir(workDir string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.workDir = workDir
	p.parser = NewPDFParser(workDir)
	p.generator = NewPDFGenerator(workDir)

	// Update cache path
	cachePath := filepath.Join(workDir, "pdf_translation_cache.json")
	p.cache.SetCachePath(cachePath)
}

// GetWorkDir returns the current working directory
func (p *PDFTranslator) GetWorkDir() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.workDir
}

// Close cleans up resources used by the translator
func (p *PDFTranslator) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger.Info("closing PDF translator")

	// Cancel any ongoing operations
	p.cancel()

	// Save cache
	if err := p.cache.Save(); err != nil {
		logger.Warn("failed to save cache on close", logger.Err(err))
	}

	return nil
}

// SetPageCompleteCallback sets the callback function for page completion events.
// This callback is called after each page is translated, allowing for progressive display.
// The callback receives: currentPage (1-based), totalPages, and outputPath.
func (p *PDFTranslator) SetPageCompleteCallback(callback func(currentPage, totalPages int, outputPath string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pageCompleteCallback = callback
}
