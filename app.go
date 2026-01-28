package main

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"latex-translator/internal/compiler"
	"latex-translator/internal/config"
	"latex-translator/internal/downloader"
	"latex-translator/internal/errors"
	"latex-translator/internal/github"
	"latex-translator/internal/logger"
	"latex-translator/internal/parser"
	"latex-translator/internal/pdf"
	"latex-translator/internal/results"
	"latex-translator/internal/settings"
	"latex-translator/internal/translator"
	"latex-translator/internal/types"
	"latex-translator/internal/validator"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Event names for frontend communication
const (
	EventOriginalPDFReady   = "original-pdf-ready"
	EventTranslatedPDFReady = "translated-pdf-ready"
)

// Default GitHub repository settings
const (
	DefaultGitHubOwner = "rapidaicoder"
	DefaultGitHubRepo  = "chinesepaper"
)

// StatusCallback is a function type for status update callbacks.
// It is called whenever the processing status changes.
type StatusCallback func(status *types.Status)

// App is the main Wails application controller.
// It integrates all modules (ConfigManager, SourceDownloader, TranslationEngine,
// LaTeXCompiler, SyntaxValidator) and manages the application lifecycle.
type App struct {
	ctx        context.Context
	config     *config.ConfigManager
	downloader *downloader.SourceDownloader
	translator *translator.TranslationEngine
	compiler   *compiler.LaTeXCompiler
	validator  *validator.SyntaxValidator
	results    *results.ResultManager
	settings   *settings.Manager
	errorMgr   *errors.ErrorManager
	workDir    string

	// Status tracking
	status         *types.Status
	statusMu       sync.RWMutex
	statusCallback StatusCallback

	// Cancellation support
	cancelFunc context.CancelFunc

	// Last process result for download
	lastResult *types.ProcessResult

	// PDF translation support
	pdfTranslator *pdf.PDFTranslator

	// isWailsRuntime indicates if the app is running in a Wails environment
	// This is used to safely skip EventsEmit calls during tests
	isWailsRuntime bool
}

// safeEmit safely emits an event to the frontend.
// It only emits events when running in a Wails environment.
func (a *App) safeEmit(eventName string, data ...interface{}) {
	if !a.isWailsRuntime {
		logger.Debug("event emit skipped (not in Wails runtime)",
			logger.String("event", eventName))
		return
	}
	runtime.EventsEmit(a.ctx, eventName, data...)
}

// SetWailsRuntime sets the Wails runtime flag.
// This should be called from main.go when the app is started in Wails mode.
func (a *App) SetWailsRuntime(isWails bool) {
	a.isWailsRuntime = isWails
}

// NewApp creates a new App application struct with all dependencies initialized.
// It sets up the work directory and creates instances of all required modules.
func NewApp() *App {
	return &App{
		status: &types.Status{
			Phase:    types.PhaseIdle,
			Progress: 0,
			Message:  "",
		},
	}
}

// NewAppWithConfig creates a new App with a custom config path.
// This is useful for testing or when a specific configuration location is needed.
func NewAppWithConfig(configPath string) (*App, error) {
	app := &App{
		status: &types.Status{
			Phase:    types.PhaseIdle,
			Progress: 0,
			Message:  "",
		},
	}

	// Initialize config manager
	configMgr, err := config.NewConfigManager(configPath)
	if err != nil {
		return nil, err
	}
	app.config = configMgr

	return app, nil
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods. It initializes all modules
// and prepares the application for use.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logger.Info("application starting up")

	// Initialize config manager if not already set
	if a.config == nil {
		configMgr, err := config.NewConfigManager("")
		if err != nil {
			logger.Error("failed to create config manager", err)
			return
		}
		a.config = configMgr
	}

	// Load configuration
	if err := a.config.Load(); err != nil {
		// Continue with defaults if config load fails
		logger.Warn("failed to load config, using defaults", logger.Err(err))
	}

	// Initialize work directory (after config is loaded so we can use configured work dir)
	if err := a.initWorkDir(); err != nil {
		// Log error but continue - work dir can be set later
		logger.Error("failed to initialize work directory", err)
	}

	// Initialize downloader with work directory
	a.downloader = downloader.NewSourceDownloader(a.workDir)
	logger.Debug("downloader initialized", logger.String("workDir", a.workDir))

	// Initialize translator with API key from config
	apiKey := a.config.GetAPIKey()
	model := a.config.GetModel()
	baseURL := a.config.GetBaseURL()
	concurrency := a.config.GetConcurrency()
	logger.Info("initializing translator",
		logger.Int("apiKeyLength", len(apiKey)),
		logger.String("model", model),
		logger.String("baseURL", baseURL),
		logger.Int("concurrency", concurrency))
	a.translator = translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, concurrency)

	// Initialize compiler with default compiler from config
	defaultCompiler := a.config.GetDefaultCompiler()
	a.compiler = compiler.NewLaTeXCompiler(defaultCompiler, a.workDir, 0)
	logger.Debug("compiler initialized", logger.String("compiler", defaultCompiler))

	// Initialize validator with API key and base URL from config
	a.validator = validator.NewSyntaxValidatorWithConfig(apiKey, model, baseURL, 0)
	logger.Debug("validator initialized", logger.String("baseURL", baseURL))

	// Initialize PDF translator with config
	pdfConfig := pdf.PDFTranslatorConfig{
		Config: &types.Config{
			OpenAIAPIKey:  apiKey,
			OpenAIBaseURL: baseURL,
			OpenAIModel:   model,
			ContextWindow: a.config.GetContextWindow(),
			Concurrency:   concurrency,
		},
		WorkDir: a.workDir,
	}
	a.pdfTranslator = pdf.NewPDFTranslator(pdfConfig)
	logger.Debug("PDF translator initialized")

	// Initialize result manager
	resultMgr, err := results.NewResultManager("")
	if err != nil {
		logger.Warn("failed to initialize result manager", logger.Err(err))
	} else {
		a.results = resultMgr
		logger.Debug("result manager initialized", logger.String("baseDir", resultMgr.GetBaseDir()))
	}

	// Initialize settings manager for local settings (GitHub token, etc.)
	settingsMgr, err := settings.NewManager()
	if err != nil {
		logger.Warn("failed to initialize settings manager", logger.Err(err))
	} else {
		a.settings = settingsMgr
		logger.Debug("settings manager initialized", logger.String("filePath", settingsMgr.GetFilePath()))
	}

	// Initialize error manager
	errorMgr, err := errors.NewErrorManager("")
	if err != nil {
		logger.Warn("failed to initialize error manager", logger.Err(err))
	} else {
		a.errorMgr = errorMgr
		logger.Debug("error manager initialized")
	}

	// Ensure GitHub token is available
	if err := a.EnsureGitHubToken(); err != nil {
		logger.Warn("failed to ensure GitHub token", logger.Err(err))
	}

	logger.Info("application startup complete")
}

// shutdown is called when the app is closing. It performs cleanup
// of temporary files and resources.
func (a *App) shutdown(ctx context.Context) {
	logger.Info("application shutting down")

	// Close PDF translator to save cache
	if a.pdfTranslator != nil {
		if err := a.pdfTranslator.Close(); err != nil {
			logger.Warn("failed to close PDF translator", logger.Err(err))
		}
	}

	// Clean up work directory if it exists and is a temp directory
	if a.workDir != "" && a.isTemporaryWorkDir() {
		// Attempt to clean up, but don't fail if it doesn't work
		if err := os.RemoveAll(a.workDir); err != nil {
			logger.Warn("failed to clean up work directory", logger.String("workDir", a.workDir), logger.Err(err))
		} else {
			logger.Debug("cleaned up temporary work directory", logger.String("workDir", a.workDir))
		}
	}

	logger.Info("application shutdown complete")
}

// initWorkDir initializes the work directory for temporary files.
// It first checks if a work directory is configured, otherwise creates
// a temporary directory.
func (a *App) initWorkDir() error {
	// Check if config has a work directory set
	if a.config != nil {
		configWorkDir := a.config.GetWorkDirectory()
		if configWorkDir != "" {
			a.workDir = configWorkDir
			return os.MkdirAll(a.workDir, 0755)
		}
	}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "latex-translator-*")
	if err != nil {
		return err
	}
	a.workDir = tempDir
	return nil
}

// isTemporaryWorkDir checks if the current work directory is a temporary directory.
func (a *App) isTemporaryWorkDir() bool {
	if a.workDir == "" {
		return false
	}

	// Check if it's in the system temp directory
	tempDir := os.TempDir()
	return filepath.HasPrefix(a.workDir, tempDir)
}

// GetConfig returns the config manager.
func (a *App) GetConfig() *config.ConfigManager {
	return a.config
}

// GetDownloader returns the source downloader.
func (a *App) GetDownloader() *downloader.SourceDownloader {
	return a.downloader
}

// GetTranslator returns the translation engine.
func (a *App) GetTranslator() *translator.TranslationEngine {
	return a.translator
}

// GetCompiler returns the LaTeX compiler.
func (a *App) GetCompiler() *compiler.LaTeXCompiler {
	return a.compiler
}

// GetValidator returns the syntax validator.
func (a *App) GetValidator() *validator.SyntaxValidator {
	return a.validator
}

// GetWorkDir returns the current work directory.
func (a *App) GetWorkDir() string {
	return a.workDir
}

// SetWorkDir sets the work directory and updates all modules.
func (a *App) SetWorkDir(workDir string) error {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	a.workDir = workDir

	// Update downloader work directory
	if a.downloader != nil {
		a.downloader.SetWorkDir(workDir)
	}

	return nil
}

// ReloadConfig reloads the configuration and updates all modules with new settings.
func (a *App) ReloadConfig() error {
	if a.config == nil {
		return nil
	}

	// Reload configuration
	if err := a.config.Load(); err != nil {
		return err
	}

	// Update translator with new API key and model
	apiKey := a.config.GetAPIKey()
	model := a.config.GetModel()
	baseURL := a.config.GetBaseURL()
	concurrency := a.config.GetConcurrency()
	logger.Info("ReloadConfig: updating translator",
		logger.Int("apiKeyLength", len(apiKey)),
		logger.String("model", model),
		logger.String("baseURL", baseURL),
		logger.Int("concurrency", concurrency))
	if a.translator != nil {
		a.translator = translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, concurrency)
	}

	// Update validator with new API key and base URL
	if a.validator != nil {
		a.validator = validator.NewSyntaxValidatorWithConfig(apiKey, model, baseURL, 0)
	}

	// Update compiler with new default compiler
	defaultCompiler := a.config.GetDefaultCompiler()
	if a.compiler != nil {
		a.compiler.SetCompiler(defaultCompiler)
	}

	// Update work directory if configured
	configWorkDir := a.config.GetWorkDirectory()
	if configWorkDir != "" && configWorkDir != a.workDir {
		if err := a.SetWorkDir(configWorkDir); err != nil {
			return err
		}
	}

	// Update PDF translator with new config
	if a.pdfTranslator != nil {
		a.pdfTranslator.UpdateConfig(&types.Config{
			OpenAIAPIKey:  apiKey,
			OpenAIBaseURL: baseURL,
			OpenAIModel:   model,
			ContextWindow: a.config.GetContextWindow(),
			Concurrency:   concurrency,
		})
		// Also update work directory for PDF translator
		if configWorkDir != "" {
			a.pdfTranslator.SetWorkDir(configWorkDir)
		}
	}

	return nil
}

// SetStatusCallback sets the callback function for status updates.
// The callback will be called whenever the processing status changes.
func (a *App) SetStatusCallback(callback StatusCallback) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.statusCallback = callback
}

// GetStatus returns the current processing status.
// This method is thread-safe.
func (a *App) GetStatus() *types.Status {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()

	// Return a copy to prevent external modification
	return &types.Status{
		Phase:    a.status.Phase,
		Progress: a.status.Progress,
		Message:  a.status.Message,
		Error:    a.status.Error,
	}
}

// IsProcessing returns true if a translation task is currently in progress.
// This method is thread-safe.
func (a *App) IsProcessing() bool {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()

	// Processing is active if phase is not idle, complete, or error
	switch a.status.Phase {
	case types.PhaseIdle, types.PhaseComplete, types.PhaseError:
		return false
	default:
		return true
	}
}

// updateStatus updates the current status and notifies the callback.
func (a *App) updateStatus(phase types.ProcessPhase, progress int, message string) {
	a.statusMu.Lock()
	a.status.Phase = phase
	a.status.Progress = progress
	a.status.Message = message
	a.status.Error = ""

	// Get callback while holding lock
	callback := a.statusCallback
	// Make a copy for the callback
	statusCopy := &types.Status{
		Phase:    a.status.Phase,
		Progress: a.status.Progress,
		Message:  a.status.Message,
		Error:    a.status.Error,
	}
	a.statusMu.Unlock()

	// Call callback outside of lock to prevent deadlocks
	if callback != nil {
		callback(statusCopy)
	}
}

// updateStatusError updates the status with an error.
func (a *App) updateStatusError(errorMsg string) {
	a.statusMu.Lock()
	a.status.Phase = types.PhaseError
	a.status.Error = errorMsg

	// Get callback while holding lock
	callback := a.statusCallback
	// Make a copy for the callback
	statusCopy := &types.Status{
		Phase:    a.status.Phase,
		Progress: a.status.Progress,
		Message:  a.status.Message,
		Error:    a.status.Error,
	}
	a.statusMu.Unlock()

	// Call callback outside of lock to prevent deadlocks
	if callback != nil {
		callback(statusCopy)
	}
}

// CheckExistingTranslation checks if a translation already exists for the given input
// This is called by the frontend before starting a translation to avoid duplicates
func (a *App) CheckExistingTranslation(input string) (*results.ExistingTranslationInfo, error) {
	logger.Info("checking existing translation", logger.String("input", input))

	if a.results == nil {
		return &results.ExistingTranslationInfo{
			Exists:  false,
			Message: "结果管理器未初始化",
		}, nil
	}

	// Determine source type
	sourceType, err := parser.ParseInput(input)
	if err != nil {
		return nil, err
	}

	var resultSourceType results.SourceType
	switch sourceType {
	case types.SourceTypeArxivID, types.SourceTypeURL:
		resultSourceType = results.SourceTypeArxiv
	case types.SourceTypeLocalZip:
		resultSourceType = results.SourceTypeZip
	case types.SourceTypeLocalPDF:
		resultSourceType = results.SourceTypePDF
	default:
		return &results.ExistingTranslationInfo{
			Exists:  false,
			Message: "未知的输入类型",
		}, nil
	}

	return a.results.CheckExistingTranslation(input, resultSourceType)
}

// ProcessSourceWithForce processes the input source, optionally forcing re-translation
// If force is false and a translation already exists, returns an error
// If force is true, deletes existing translation and starts fresh
func (a *App) ProcessSourceWithForce(input string, force bool) (*types.ProcessResult, error) {
	logger.Info("processing source with force option",
		logger.String("input", input),
		logger.Bool("force", force))

	// Check for existing translation
	existingInfo, err := a.CheckExistingTranslation(input)
	if err != nil {
		logger.Warn("failed to check existing translation", logger.Err(err))
		// Continue anyway - don't block translation due to check failure
	} else if existingInfo != nil && existingInfo.Exists {
		if !force {
			// Return the existing result instead of re-translating
			if existingInfo.IsComplete && existingInfo.PaperInfo != nil {
				logger.Info("returning existing translation result",
					logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
				return &types.ProcessResult{
					OriginalPDFPath:   existingInfo.PaperInfo.OriginalPDF,
					TranslatedPDFPath: existingInfo.PaperInfo.TranslatedPDF,
					SourceID:          existingInfo.PaperInfo.ArxivID,
				}, nil
			}
			// If not complete, we can continue
			if existingInfo.CanContinue && existingInfo.PaperInfo != nil {
				logger.Info("continuing existing translation",
					logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
				return a.ContinueTranslation(existingInfo.PaperInfo.ArxivID)
			}
		} else {
			// Force re-translation - delete existing
			if existingInfo.PaperInfo != nil && existingInfo.PaperInfo.ArxivID != "" {
				logger.Info("deleting existing translation for force re-translation",
					logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
				if err := a.results.DeletePaper(existingInfo.PaperInfo.ArxivID); err != nil {
					logger.Warn("failed to delete existing paper", logger.Err(err))
				}
			}
		}
	}

	return a.ProcessSource(input)
}

// ProcessSource processes the input source and executes the complete translation flow.
// The flow is:
//  1. Parse input (URL, ArxivID, or LocalZip)
//  2. Download/extract source code
//  3. Find main tex file
//  4. Compile original document to PDF
//  5. Translate tex content
//  6. Validate and fix syntax errors
//  7. Compile translated document to PDF
//  8. Return ProcessResult with both PDF paths
//
// Validates: Requirements 1.1-1.5, 2.1-2.5, 3.1-3.5
func (a *App) ProcessSource(input string) (*types.ProcessResult, error) {
	logger.Info("starting source processing", logger.String("input", input))

	// Create a cancellable context for this processing session
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFunc = cancel
	defer func() {
		a.cancelFunc = nil
	}()

	// Reset status to idle at the start
	a.updateStatus(types.PhaseIdle, 0, "开始处?..")

	// Step 1: Parse input to determine source type
	a.updateStatus(types.PhaseDownloading, 5, "...")
	logger.Debug("parsing input")

	sourceType, err := parser.ParseInput(input)
	if err != nil {
		logger.Error("input parsing failed", err, logger.String("input", input))
		a.updateStatusError(fmt.Sprintf("ʧ: %v", err))
		return nil, err
	}

	logger.Info("input parsed successfully", logger.String("sourceType", string(sourceType)))

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("ȡ")
		return nil, types.NewAppError(types.ErrInternal, "ȡ", ctx.Err())
	}

	// Step 2: Download/extract source code based on type
	var sourceInfo *types.SourceInfo

	switch sourceType {
	case types.SourceTypeURL:
		a.updateStatus(types.PhaseDownloading, 10, " URL Դ...")
		logger.Info("downloading from URL", logger.String("url", input))
		sourceInfo, err = a.downloader.DownloadFromURL(input)
		if err != nil {
			logger.Error("download from URL failed", err, logger.String("url", input))
			a.updateStatusError(fmt.Sprintf("ʧ: %v", err))
			return nil, err
		}

		// Extract the downloaded archive
		a.updateStatus(types.PhaseExtracting, 20, "ѹԴ...")
		logger.Debug("extracting downloaded archive")
		sourceInfo, err = a.downloader.ExtractZip(sourceInfo.ExtractDir)
		if err != nil {
			logger.Error("extraction failed", err)
			a.updateStatusError(fmt.Sprintf("ѹʧ: %v", err))
			return nil, err
		}
		sourceInfo.SourceType = types.SourceTypeURL
		sourceInfo.OriginalRef = input

	case types.SourceTypeArxivID:
		a.updateStatus(types.PhaseDownloading, 10, " arXiv Դ...")
		logger.Info("downloading by arXiv ID", logger.String("arxivID", input))
		sourceInfo, err = a.downloader.DownloadByID(input)
		if err != nil {
			logger.Error("download by ID failed", err, logger.String("arxivID", input))
			a.updateStatusError(fmt.Sprintf("ʧ: %v", err))
			// 记录下载错误
			arxivID := results.ExtractArxivID(input)
			if arxivID != "" {
				a.recordError(arxivID, arxivID, input, errors.StageDownload, err.Error())
			}
			return nil, err
		}

		// Extract the downloaded archive
		a.updateStatus(types.PhaseExtracting, 20, "ѹԴ...")
		logger.Debug("extracting downloaded archive")
		sourceInfo, err = a.downloader.ExtractZip(sourceInfo.ExtractDir)
		if err != nil {
			logger.Error("extraction failed", err)
			a.updateStatusError(fmt.Sprintf("ѹʧ: %v", err))
			// 记录解压错误
			arxivID := results.ExtractArxivID(input)
			if arxivID != "" {
				a.recordError(arxivID, arxivID, input, errors.StageExtract, err.Error())
			}
			return nil, err
		}
		sourceInfo.SourceType = types.SourceTypeArxivID
		sourceInfo.OriginalRef = input

	case types.SourceTypeLocalZip:
		a.updateStatus(types.PhaseExtracting, 15, "解压本地文件...")
		logger.Info("extracting local zip file", logger.String("path", input))
		sourceInfo, err = a.downloader.ExtractZip(input)
		if err != nil {
			logger.Error("extraction failed", err, logger.String("path", input))
			a.updateStatusError(fmt.Sprintf("ѹʧ: %v", err))
			return nil, err
		}

	default:
		err := types.NewAppError(types.ErrInvalidInput, "ֵ֧", nil)
		logger.Error("unsupported input type", err)
		a.updateStatusError(err.Error())
		return nil, err
	}

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("ȡ")
		return nil, types.NewAppError(types.ErrInternal, "ȡ", ctx.Err())
	}

	// Step 2.5: Preprocess tex files to fix common issues
	a.updateStatus(types.PhaseExtracting, 22, "预处理 LaTeX 源文件...")
	logger.Info("preprocessing tex files", logger.String("extractDir", sourceInfo.ExtractDir))
	if err := compiler.PreprocessTexFiles(sourceInfo.ExtractDir); err != nil {
		// Log warning but don't fail - preprocessing is best-effort
		logger.Warn("preprocessing failed", logger.Err(err))
	}

	// Step 3: Find main tex file
	a.updateStatus(types.PhaseExtracting, 25, " tex ļ...")
	logger.Debug("finding main tex file", logger.String("extractDir", sourceInfo.ExtractDir))
	mainTexFile, err := a.downloader.FindMainTexFile(sourceInfo.ExtractDir)
	if err != nil {
		logger.Error("failed to find main tex file", err)
		a.updateStatusError(fmt.Sprintf("未找到主 tex 文件: %v", err))
		// Save intermediate result on error
		arxivID := results.ExtractArxivID(input)
		if arxivID != "" {
			a.saveIntermediateResult(arxivID, arxivID, input, sourceInfo, results.StatusError, err.Error(), "", "")
			// 记录解压/查找文件错误
			a.recordError(arxivID, arxivID, input, errors.StageExtract, err.Error())
		}
		return nil, err
	}
	sourceInfo.MainTexFile = mainTexFile
	logger.Info("found main tex file", logger.String("mainTexFile", mainTexFile))

	mainTexPath := filepath.Join(sourceInfo.ExtractDir, mainTexFile)

	// Extract arXiv ID and title for saving intermediate results
	arxivID := results.ExtractArxivID(input)

	// Extract source ID for file naming (arXiv ID or zip filename without extension)
	sourceID := arxivID
	if sourceID == "" && sourceType == types.SourceTypeLocalZip {
		// For local zip files, use the filename without extension
		sourceID = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	}

	title := ""
	if arxivID != "" {
		mainTexContent, readErr := os.ReadFile(mainTexPath)
		if readErr == nil {
			title = results.ExtractTitleFromTeX(string(mainTexContent))
		}
		if title == "" {
			title = arxivID
		}
		// Save intermediate result after extraction
		a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusExtracted, "", "", "")
	}

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("ȡ")
		return nil, types.NewAppError(types.ErrInternal, "ȡ", ctx.Err())
	}

	// Step 4: Compile original document to PDF
	a.updateStatus(types.PhaseCompiling, 30, "编译原始文档...")
	logger.Info("compiling original document", logger.String("texPath", mainTexPath))
	originalOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_original")
	originalResult, err := a.compiler.Compile(mainTexPath, originalOutputDir)
	if err != nil {
		logger.Error("original document compilation failed", err)
		a.updateStatusError(fmt.Sprintf("原始文档ʧ: %v", err))
		// Save intermediate result on error
		if arxivID != "" {
			a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusError, err.Error(), "", "")
			// 记录原始编译错误
			a.recordError(arxivID, title, input, errors.StageOriginalCompile, err.Error())
		}
		return nil, err
	}
	if !originalResult.Success {
		err := types.NewAppErrorWithDetails(types.ErrCompile, "原始文档ʧ", originalResult.ErrorMsg, nil)
		logger.Error("original document compilation failed", err, logger.String("errorMsg", originalResult.ErrorMsg))
		a.updateStatusError(err.Error())
		// Save intermediate result on error
		if arxivID != "" {
			a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusError, originalResult.ErrorMsg, "", "")
			// 记录原始编译错误
			a.recordError(arxivID, title, input, errors.StageOriginalCompile, originalResult.ErrorMsg)
		}
		return nil, err
	}
	logger.Info("original document compiled successfully", logger.String("pdfPath", originalResult.PDFPath))

	// Save intermediate result after original compilation
	if arxivID != "" {
		a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusOriginalCompiled, "", originalResult.PDFPath, "")
	}

	// Emit event to frontend to display original PDF
	a.safeEmit(EventOriginalPDFReady, originalResult.PDFPath)

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("ȡ")
		return nil, types.NewAppError(types.ErrInternal, "ȡ", ctx.Err())
	}

	// Step 5: Read and translate tex content
	a.updateStatus(types.PhaseTranslating, 40, "ȡ tex ļ...")
	logger.Debug("reading tex files for translation")

	a.updateStatus(types.PhaseTranslating, 42, "ʼĵ...")

	// Translate main file and all input files
	translatedFiles, totalTokens, err := a.translateAllTexFiles(mainTexPath, sourceInfo.ExtractDir, func(current, total int, message string) {
		// Calculate progress: translation phase is from 42% to 58%
		progressRange := 16 // 58 - 42
		progress := 42 + (current * progressRange / total)
		a.updateStatus(types.PhaseTranslating, progress, message)
	})
	if err != nil {
		logger.Error("translation failed", err)
		a.updateStatusError(fmt.Sprintf("ʧ: %v", err))
		// Save intermediate result on translation error
		if arxivID != "" {
			a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusError, err.Error(), originalResult.PDFPath, "")
			// 记录翻译错误
			a.recordError(arxivID, title, input, errors.StageTranslation, err.Error())
		}
		return nil, err
	}
	logger.Info("translation completed", logger.Int("tokensUsed", totalTokens), logger.Int("filesTranslated", len(translatedFiles)))

	// Save intermediate result after translation
	if arxivID != "" {
		a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusTranslated, "", originalResult.PDFPath, "")
	}

	// Get the translated main file content
	mainFileName := filepath.Base(mainTexPath)
	translatedContent := translatedFiles[mainFileName]

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("ȡ")
		return nil, types.NewAppError(types.ErrInternal, "ȡ", ctx.Err())
	}

	// Step 6: Validate and fix syntax errors (only for main file)
	// Skip syntax validation for large files based on context window setting
	// The compile-fix loop in step 8 will handle any remaining errors
	// Estimate: 1 token ?4 bytes for English/LaTeX, use conservative 3 bytes/token
	// Also reserve 50% of context for system prompt and response
	contextWindow := a.config.GetContextWindow()
	maxSyntaxFixSize := contextWindow * 3 / 2 // contextWindow * 3 bytes/token * 50% reserve
	logger.Debug("syntax fix size threshold",
		logger.Int("contextWindow", contextWindow),
		logger.Int("maxSyntaxFixSize", maxSyntaxFixSize),
		logger.Int("contentSize", len(translatedContent)))

	if len(translatedContent) <= maxSyntaxFixSize {
		a.updateStatus(types.PhaseValidating, 60, "验证翻译后的语法...")
		logger.Debug("validating translated content")
		validationResult, err := a.validator.Validate(translatedContent)
		if err != nil {
			logger.Error("syntax validation failed", err)
			a.updateStatusError(fmt.Sprintf("语法验证失败: %v", err))
			return nil, err
		}

		if !validationResult.IsValid {
			a.updateStatus(types.PhaseValidating, 65, "修正语法错误...")
			logger.Info("fixing syntax errors", logger.Int("errorCount", len(validationResult.Errors)))
			translatedContent, err = a.validator.Fix(translatedContent, validationResult.Errors)
			if err != nil {
				// For syntax fix failures, log warning but continue with compilation
				// The compile-fix loop will attempt to fix errors during compilation
				logger.Warn("syntax fix failed, will rely on compile-fix loop", logger.Err(err))
			} else {
				logger.Info("syntax errors fixed successfully")
				// Update the main file in translatedFiles
				translatedFiles[mainFileName] = translatedContent
			}
		}
	} else {
		logger.Info("skipping syntax validation for large file, will rely on compile-fix loop",
			logger.Int("contentSize", len(translatedContent)),
			logger.Int("threshold", maxSyntaxFixSize),
			logger.Int("contextWindow", contextWindow))
		a.updateStatus(types.PhaseValidating, 65, "大文件跳过语法验证，依赖编译修复...")
	}

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("ȡ")
		return nil, types.NewAppError(types.ErrInternal, "ȡ", ctx.Err())
	}

	// Step 7: Save all translated tex files
	a.updateStatus(types.PhaseValidating, 70, "保存翻译文件...")

	// Ensure ctex package is included for Chinese support in main file
	translatedContent = ensureCtexPackage(translatedContent)
	translatedFiles[mainFileName] = translatedContent

	logger.Info("saving translated files",
		logger.Int("fileCount", len(translatedFiles)),
		logger.String("mainFileName", mainFileName))

	// Save all translated files, applying QuickFix to each
	for relPath, content := range translatedFiles {
		// Apply QuickFix to fix common translation issues (like lonely \item)
		fixedContent, wasFixed := compiler.QuickFix(content)
		if wasFixed {
			logger.Info("applied QuickFix to translated file",
				logger.String("relPath", relPath))
			content = fixedContent
			translatedFiles[relPath] = content
		}

		var savePath string
		if relPath == mainFileName {
			// Main file gets "translated_" prefix
			savePath = filepath.Join(sourceInfo.ExtractDir, "translated_"+relPath)
		} else {
			// Input files are saved in place (overwrite original)
			savePath = filepath.Join(sourceInfo.ExtractDir, relPath)
		}

		logger.Info("saving translated file",
			logger.String("relPath", relPath),
			logger.String("savePath", savePath),
			logger.Int("contentLength", len(content)),
			logger.Bool("isMainFile", relPath == mainFileName))

		err = os.WriteFile(savePath, []byte(content), 0644)
		if err != nil {
			logger.Error("failed to save translated file", err, logger.String("path", savePath))
			a.updateStatusError(fmt.Sprintf("保存翻译文件失败: %v", err))
			return nil, types.NewAppError(types.ErrInternal, "保存翻译文件失败", err)
		}
	}

	// Update translatedContent with the fixed version
	translatedContent = translatedFiles[mainFileName]

	translatedTexPath := filepath.Join(sourceInfo.ExtractDir, "translated_"+mainTexFile)
	translatedOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_translated")

	// Step 8: Compile translated document with hierarchical auto-fix
	// Strategy (3-level hierarchical fix):
	// Level 1: Rule-based fixes (fast, reliable for common issues)
	// Level 2: Simple LLM fixes (moderate cost, handles most errors)
	// Level 3: Agent-based fixes (higher cost, handles complex errors)
	var translatedResult *types.CompileResult

	// First attempt: compile without fixes
	a.updateStatus(types.PhaseCompiling, 75, "编译中文文档...")
	logger.Info("compiling translated document", logger.String("texPath", translatedTexPath))
	translatedResult, err = a.compiler.CompileWithXeLaTeX(translatedTexPath, translatedOutputDir)

	// If compilation failed, use hierarchical fix strategy
	if err != nil || !translatedResult.Success {
		logger.Warn("initial compilation failed, starting hierarchical fix process",
			logger.String("error", translatedResult.ErrorMsg))

		// Create fixer with API credentials
		apiKey := a.config.GetAPIKey()
		baseURL := a.config.GetBaseURL()
		model := a.config.GetModel()
		fixer := compiler.NewLaTeXFixerWithAgent(apiKey, baseURL, model, model, true)

		// Run hierarchical fix with progress callback
		fixResult, fixErr := fixer.HierarchicalFixCompilationErrors(
			sourceInfo.ExtractDir,
			"translated_"+mainTexFile,
			translatedResult.Log,
			a.compiler,
			translatedOutputDir,
			func(level compiler.FixLevel, attempt int, message string) {
				// Calculate progress based on fix level
				var progress int
				switch level {
				case compiler.FixLevelRule:
					progress = 78 + attempt
				case compiler.FixLevelLLM:
					progress = 82 + attempt*2
				case compiler.FixLevelAgent:
					progress = 90 + attempt*3
				}
				if progress > 98 {
					progress = 98
				}
				a.updateStatus(types.PhaseValidating, progress, message)
			},
		)

		if fixErr != nil {
			logger.Error("hierarchical fix process failed", fixErr)
		}

		if fixResult != nil && fixResult.Success {
			logger.Info("hierarchical fix succeeded",
				logger.Int("totalIterations", fixResult.TotalIterations),
				logger.Int("ruleAttempts", fixResult.RuleFixAttempts),
				logger.Int("llmAttempts", fixResult.LLMFixAttempts),
				logger.Int("agentAttempts", fixResult.AgentFixAttempts))

			// Compile one more time to get the final result
			translatedResult, err = a.compiler.CompileWithXeLaTeX(translatedTexPath, translatedOutputDir)
		} else {
			logger.Warn("hierarchical fix did not succeed",
				logger.String("description", fixResult.Description))
		}
	}

	// Final check - if still failed after all fix attempts
	if err != nil || !translatedResult.Success {
		errMsg := "中文文档编译失败"
		if translatedResult != nil && translatedResult.ErrorMsg != "" {
			errMsg = translatedResult.ErrorMsg
		}
		err := types.NewAppErrorWithDetails(types.ErrCompile, "中文文档编译失败", errMsg, nil)
		logger.Error("translated document compilation failed after all fix attempts", err)
		a.updateStatusError(err.Error())
		// Save intermediate result on compilation error - still save the source for later retry
		if arxivID != "" {
			a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusError, errMsg, originalResult.PDFPath, "")
			// 记录翻译后编译错误
			a.recordError(arxivID, title, input, errors.StageTranslatedCompile, errMsg)
		}
		return nil, err
	}

	logger.Info("translated document compiled successfully", logger.String("pdfPath", translatedResult.PDFPath))
	// Emit event to frontend to display translated PDF
	a.safeEmit(EventTranslatedPDFReady, translatedResult.PDFPath)

	// Step 9: Generate bilingual PDF
	a.updateStatus(types.PhaseCompiling, 95, "生成双语对照 PDF...")
	bilingualPDFPath := ""
	bilingualOutputPath := filepath.Join(sourceInfo.ExtractDir, "bilingual_"+sourceID+".pdf")
	generator := pdf.NewPDFGenerator(sourceInfo.ExtractDir)
	if err := generator.GenerateSideBySidePDF(originalResult.PDFPath, translatedResult.PDFPath, bilingualOutputPath); err != nil {
		logger.Warn("failed to generate bilingual PDF", logger.Err(err))
		// 双语 PDF 生成失败不影响主流程，但记录错误
		if arxivID != "" {
			a.recordError(arxivID, title, input, errors.StagePDFGeneration, err.Error())
		}
	} else {
		bilingualPDFPath = bilingualOutputPath
		logger.Info("bilingual PDF generated", logger.String("path", bilingualPDFPath))
	}

	// Step 10: Complete
	a.updateStatus(types.PhaseComplete, 100, "翻译完成")
	logger.Info("source processing completed successfully",
		logger.String("originalPDF", originalResult.PDFPath),
		logger.String("translatedPDF", translatedResult.PDFPath),
		logger.String("bilingualPDF", bilingualPDFPath))

	result := &types.ProcessResult{
		OriginalPDFPath:   originalResult.PDFPath,
		TranslatedPDFPath: translatedResult.PDFPath,
		BilingualPDFPath:  bilingualPDFPath,
		SourceInfo:        sourceInfo,
		SourceID:          sourceID,
	}

	// Store result for download
	a.lastResult = result

	// Save to permanent storage if this is an arXiv paper (arxivID and title already extracted earlier)
	if arxivID != "" {
		if err := a.saveResultToPermanentStorage(result, arxivID, title); err != nil {
			logger.Warn("failed to save result to permanent storage", logger.Err(err))
		}
		// 翻译成功，移除错误记录
		if a.errorMgr != nil {
			if err := a.errorMgr.RemoveError(arxivID); err != nil {
				logger.Warn("failed to remove error record after successful translation", logger.Err(err))
			}
		}
	}

	return result, nil
}

// extractCompileErrors extracts error information from LaTeX compilation log
func extractCompileErrors(log string) []types.SyntaxError {
	var errors []types.SyntaxError
	lines := strings.Split(log, "\n")

	// Common LaTeX error patterns - more specific patterns first
	errorPatterns := []struct {
		pattern *regexp.Regexp
		errType string
	}{
		{regexp.MustCompile(`Undefined control sequence`), "undefined_command"},
		{regexp.MustCompile(`Missing \$ inserted`), "math_mode"},
		{regexp.MustCompile(`Missing \{ inserted`), "brace"},
		{regexp.MustCompile(`Missing \} inserted`), "brace"},
		{regexp.MustCompile(`Extra \}`), "brace"},
		{regexp.MustCompile(`Runaway argument`), "runaway"},
		{regexp.MustCompile(`Package .+ Error`), "package_error"},
		{regexp.MustCompile(`LaTeX Error`), "latex_error"},
		{regexp.MustCompile(`^! `), "latex_error"},
		{regexp.MustCompile(`^l\.(\d+)`), "line_error"},
	}

	for i, line := range lines {
		for _, ep := range errorPatterns {
			if ep.pattern.MatchString(line) {
				// Try to extract line number
				lineNum := i + 1
				linePattern := regexp.MustCompile(`l\.(\d+)`)
				if matches := linePattern.FindStringSubmatch(line); len(matches) > 1 {
					fmt.Sscanf(matches[1], "%d", &lineNum)
				}

				errors = append(errors, types.SyntaxError{
					Line:    lineNum,
					Column:  1,
					Message: strings.TrimSpace(line),
					Type:    ep.errType,
				})
				break
			}
		}
	}

	return errors
}

// extractErrorFile extracts the file name where the error occurred from the compilation log
func extractErrorFile(log string, baseDir string) string {
	lines := strings.Split(log, "\n")

	// Pattern to match file being processed: (./path/to/file.tex or ./file.tex
	filePattern := regexp.MustCompile(`\(\.?/?([^\s\(\)]+\.tex)`)

	// Track the most recently opened file before an error
	var currentFile string

	for _, line := range lines {
		// Check for file opening
		matches := filePattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				currentFile = match[1]
				// Normalize path separators
				currentFile = filepath.Clean(currentFile)
			}
		}

		// Check for error indicators
		if strings.HasPrefix(line, "!") || strings.Contains(line, "Error") {
			if currentFile != "" {
				return currentFile
			}
		}
	}

	return ""
}

// fixCompileErrors uses LLM to fix compilation errors
// For large files, it only sends the relevant code sections around the errors
func (a *App) fixCompileErrors(content string, errors []types.SyntaxError, compileLog string) (string, error) {
	if a.translator == nil {
		return "", types.NewAppError(types.ErrConfig, "translator not initialized", nil)
	}

	// Calculate max content size based on context window
	contextWindow := a.config.GetContextWindow()
	maxContentSize := contextWindow * 3 / 2 // Same calculation as syntax validation

	// Build error descriptions
	var errorDescriptions []string
	for _, err := range errors {
		errorDescriptions = append(errorDescriptions, fmt.Sprintf("- Line %d: %s (%s)", err.Line, err.Message, err.Type))
	}

	// Truncate log if too long
	logExcerpt := compileLog
	if len(logExcerpt) > 2000 {
		logExcerpt = logExcerpt[len(logExcerpt)-2000:]
	}

	// For large files, extract only the relevant sections around errors
	contentToFix := content
	isPartialFix := false

	if len(content) > maxContentSize && len(errors) > 0 {
		isPartialFix = true
		contentToFix = extractErrorSections(content, errors, maxContentSize)
		logger.Info("using partial content for compile fix",
			logger.Int("originalSize", len(content)),
			logger.Int("extractedSize", len(contentToFix)),
			logger.Int("errorCount", len(errors)))
	}

	var prompt string
	if isPartialFix {
		prompt = fmt.Sprintf(`Fix the following LaTeX compilation errors. Only the relevant sections around the errors are shown.

COMPILATION ERRORS:
%s

COMPILATION LOG (last part):
%s

IMPORTANT RULES:
1. Fix ONLY the compilation errors in the sections shown
2. Output ONLY the corrected sections in the same format (with line markers)
3. Keep the "=== SECTION: lines X-Y ===" markers in your output
4. Common fixes include:
   - Adding missing packages
   - Fixing undefined control sequences
   - Fixing unmatched braces or brackets
   - Fixing math mode issues
5. Preserve all Chinese text

SECTIONS TO FIX:
%s`, strings.Join(errorDescriptions, "\n"), logExcerpt, contentToFix)
	} else {
		prompt = fmt.Sprintf(`Fix the following LaTeX compilation errors. The document failed to compile with XeLaTeX.

COMPILATION ERRORS:
%s

COMPILATION LOG (last part):
%s

IMPORTANT RULES:
1. Fix ONLY the compilation errors - do not change anything else
2. Common fixes include:
   - Adding missing packages
   - Fixing undefined control sequences by replacing with valid commands
   - Fixing unmatched braces or brackets
   - Fixing math mode issues
   - Removing or replacing incompatible commands
3. Preserve all Chinese text and translations
4. Keep the ctex package for Chinese support
5. Output ONLY the corrected LaTeX content - no explanations

DOCUMENT TO FIX:
%s`, strings.Join(errorDescriptions, "\n"), logExcerpt, contentToFix)
	}

	// Use translator to fix (it has the LLM connection)
	result, err := a.translator.TranslateChunk(prompt)
	if err != nil {
		return "", err
	}

	// For partial fixes, merge the fixed sections back into the original content
	if isPartialFix {
		return mergeFixedSections(content, result, errors), nil
	}

	return result, nil
}

// extractErrorSections extracts code sections around error lines
func extractErrorSections(content string, errors []types.SyntaxError, maxSize int) string {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Collect unique error line numbers
	errorLines := make(map[int]bool)
	for _, err := range errors {
		if err.Line > 0 && err.Line <= totalLines {
			errorLines[err.Line] = true
		}
	}

	// If no valid line numbers, return truncated content
	if len(errorLines) == 0 {
		if len(content) > maxSize {
			return content[:maxSize] + "\n... [truncated]"
		}
		return content
	}

	// Calculate context lines per error based on available space
	contextPerError := maxSize / (len(errorLines) * 100) // rough estimate: 100 chars per line
	if contextPerError < 10 {
		contextPerError = 10
	}
	if contextPerError > 50 {
		contextPerError = 50
	}

	// Extract sections around each error
	var sections []string
	processedRanges := make(map[string]bool)

	for lineNum := range errorLines {
		startLine := lineNum - contextPerError
		if startLine < 1 {
			startLine = 1
		}
		endLine := lineNum + contextPerError
		if endLine > totalLines {
			endLine = totalLines
		}

		// Skip if this range overlaps with already processed range
		rangeKey := fmt.Sprintf("%d-%d", startLine, endLine)
		if processedRanges[rangeKey] {
			continue
		}
		processedRanges[rangeKey] = true

		// Extract the section
		sectionLines := lines[startLine-1 : endLine]
		section := fmt.Sprintf("=== SECTION: lines %d-%d ===\n%s\n=== END SECTION ===",
			startLine, endLine, strings.Join(sectionLines, "\n"))
		sections = append(sections, section)
	}

	return strings.Join(sections, "\n\n")
}

// mergeFixedSections merges fixed sections back into the original content
func mergeFixedSections(original string, fixed string, errors []types.SyntaxError) string {
	// Parse the fixed sections
	sectionPattern := regexp.MustCompile(`=== SECTION: lines (\d+)-(\d+) ===\n([\s\S]*?)\n=== END SECTION ===`)
	matches := sectionPattern.FindAllStringSubmatch(fixed, -1)

	if len(matches) == 0 {
		// No section markers found, return fixed content as-is (might be full content)
		logger.Warn("no section markers found in fixed content, using as-is")
		return fixed
	}

	lines := strings.Split(original, "\n")

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		var startLine, endLine int
		fmt.Sscanf(match[1], "%d", &startLine)
		fmt.Sscanf(match[2], "%d", &endLine)
		fixedContent := match[3]

		// Validate line numbers
		if startLine < 1 || endLine > len(lines) || startLine > endLine {
			logger.Warn("invalid section line numbers",
				logger.Int("startLine", startLine),
				logger.Int("endLine", endLine),
				logger.Int("totalLines", len(lines)))
			continue
		}

		// Replace the section
		fixedLines := strings.Split(fixedContent, "\n")

		// Build new lines array
		newLines := make([]string, 0, len(lines))
		newLines = append(newLines, lines[:startLine-1]...)
		newLines = append(newLines, fixedLines...)
		if endLine < len(lines) {
			newLines = append(newLines, lines[endLine:]...)
		}
		lines = newLines
	}

	return strings.Join(lines, "\n")
}

// applyRuleBasedFixes applies rule-based fixes for common LaTeX errors caused by translation
// Returns the fixed content and a boolean indicating if any fixes were applied
func applyRuleBasedFixes(content string, compileLog string) (string, bool) {
	fixed := content
	anyFixed := false

	// Rule 1: Fix translated LaTeX commands (Chinese -> English)
	// These are common LaTeX commands that might get translated to Chinese
	translatedCommands := map[string]string{
		`\\引用`:  `\cite`,
		`\\参考`:  `\ref`,
		`\\标签`:  `\label`,
		`\\章节`:  `\section`,
		`\\小节`:  `\subsection`,
		`\\段落`:  `\paragraph`,
		`\\图`:   `\figure`,
		`\\表`:   `\table`,
		`\\公式`:  `\equation`,
		`\\开始`:  `\begin`,
		`\\结束`:  `\end`,
		`\\文本`:  `\text`,
		`\\粗体`:  `\textbf`,
		`\\斜体`:  `\textit`,
		`\\下划线`: `\underline`,
		`\\脚注`:  `\footnote`,
		`\\项目`:  `\item`,
	}

	for pattern, replacement := range translatedCommands {
		re := regexp.MustCompile(pattern)
		if re.MatchString(fixed) {
			fixed = re.ReplaceAllString(fixed, replacement)
			anyFixed = true
			logger.Debug("rule-based fix: replaced translated command",
				logger.String("pattern", pattern),
				logger.String("replacement", replacement))
		}
	}

	// Rule 2: Fix common undefined control sequences from compile log
	if strings.Contains(compileLog, "Undefined control sequence") {
		undefinedPattern := regexp.MustCompile(`Undefined control sequence.*\\([a-zA-Z\x{4e00}-\x{9fff}]+)`)
		matches := undefinedPattern.FindAllStringSubmatch(compileLog, -1)
		for _, match := range matches {
			if len(match) > 1 {
				undefinedCmd := match[1]
				if containsChinese(undefinedCmd) {
					cmdPattern := regexp.MustCompile(`\\` + regexp.QuoteMeta(undefinedCmd) + `(\{[^}]*\})?`)
					if cmdPattern.MatchString(fixed) {
						fixed = cmdPattern.ReplaceAllString(fixed, "$1")
						anyFixed = true
						logger.Debug("rule-based fix: removed undefined Chinese command",
							logger.String("command", undefinedCmd))
					}
				}
			}
		}
	}

	// Rule 3: Fix unmatched braces
	openBraces := strings.Count(fixed, "{") - strings.Count(fixed, "\\{")
	closeBraces := strings.Count(fixed, "}") - strings.Count(fixed, "\\}")
	if openBraces > closeBraces {
		for i := 0; i < openBraces-closeBraces; i++ {
			fixed += "}"
		}
		anyFixed = true
		logger.Debug("rule-based fix: added missing closing braces",
			logger.Int("count", openBraces-closeBraces))
	} else if closeBraces > openBraces {
		for i := 0; i < closeBraces-openBraces; i++ {
			lastBrace := strings.LastIndex(fixed, "}")
			if lastBrace > 0 && (lastBrace == 0 || fixed[lastBrace-1] != '\\') {
				fixed = fixed[:lastBrace] + fixed[lastBrace+1:]
				anyFixed = true
			}
		}
		if anyFixed {
			logger.Debug("rule-based fix: removed extra closing braces",
				logger.Int("count", closeBraces-openBraces))
		}
	}

	// Rule 4: Fix missing $ for math mode errors
	if strings.Contains(compileLog, "Missing $ inserted") {
		standalonePattern := regexp.MustCompile(`([^$\\])([_^])(\{[^}]+\}|[a-zA-Z0-9])([^$])`)
		if standalonePattern.MatchString(fixed) {
			fixed = standalonePattern.ReplaceAllString(fixed, "$1$$2$3$$4")
			anyFixed = true
			logger.Debug("rule-based fix: wrapped standalone subscript/superscript in math mode")
		}
	}

	// Rule 5: Fix "Lonely \item" errors - \end{itemize}\item should be on separate lines
	if strings.Contains(compileLog, "Lonely \\item") || strings.Contains(compileLog, "missing list environment") {
		endItemizeItemPattern := regexp.MustCompile(`(\\end\{itemize\})\s*(\\item\b)`)
		if endItemizeItemPattern.MatchString(fixed) {
			fixed = endItemizeItemPattern.ReplaceAllString(fixed, "$1\n$2")
			anyFixed = true
			logger.Debug("rule-based fix: separated \\end{itemize} and \\item onto different lines")
		}

		endEnumerateItemPattern := regexp.MustCompile(`(\\end\{enumerate\})\s*(\\item\b)`)
		if endEnumerateItemPattern.MatchString(fixed) {
			fixed = endEnumerateItemPattern.ReplaceAllString(fixed, "$1\n$2")
			anyFixed = true
			logger.Debug("rule-based fix: separated \\end{enumerate} and \\item onto different lines")
		}

		endDescriptionItemPattern := regexp.MustCompile(`(\\end\{description\})\s*(\\item\b)`)
		if endDescriptionItemPattern.MatchString(fixed) {
			fixed = endDescriptionItemPattern.ReplaceAllString(fixed, "$1\n$2")
			anyFixed = true
			logger.Debug("rule-based fix: separated \\end{description} and \\item onto different lines")
		}
	}

	// Rule 6: Fix \end{...}\begin{...} on same line
	endBeginPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\begin\{[^}]+\})`)
	if endBeginPattern.MatchString(fixed) {
		fixed = endBeginPattern.ReplaceAllString(fixed, "$1\n$2")
		anyFixed = true
		logger.Debug("rule-based fix: separated \\end and \\begin onto different lines")
	}

	// Rule 7: Fix \end{...}\section on same line
	endSectionPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\b)`)
	if endSectionPattern.MatchString(fixed) {
		fixed = endSectionPattern.ReplaceAllString(fixed, "$1\n\n$2")
		anyFixed = true
		logger.Debug("rule-based fix: separated \\end and section command onto different lines")
	}

	// Rule 8: Fix \begin{} without matching \end{}
	beginPattern := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endPattern := regexp.MustCompile(`\\end\{([^}]+)\}`)
	beginMatches := beginPattern.FindAllStringSubmatch(fixed, -1)
	endMatches := endPattern.FindAllStringSubmatch(fixed, -1)

	beginCounts := make(map[string]int)
	endCounts := make(map[string]int)
	for _, m := range beginMatches {
		beginCounts[m[1]]++
	}
	for _, m := range endMatches {
		endCounts[m[1]]++
	}

	for env, count := range beginCounts {
		if endCounts[env] < count {
			for i := 0; i < count-endCounts[env]; i++ {
				fixed += fmt.Sprintf("\n\\end{%s}", env)
				anyFixed = true
				logger.Debug("rule-based fix: added missing \\end", logger.String("env", env))
			}
		}
	}

	return fixed, anyFixed
}

// containsChinese checks if a string contains Chinese characters
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// CancelProcess cancels the current processing operation.
// It signals the processing goroutine to stop at the next cancellation check point.
func (a *App) CancelProcess() error {
	logger.Info("cancel process requested")
	if a.cancelFunc != nil {
		a.cancelFunc()
		a.updateStatusError("ȡ")
		logger.Info("process cancelled successfully")
		return nil
	}
	logger.Warn("no process to cancel")
	return types.NewAppError(types.ErrInternal, "ûڽеĴ", nil)
}

// GetSettings returns the current application settings for the frontend.
// This method is exposed to the frontend via Wails bindings.
func (a *App) GetSettings() *types.Config {
	logger.Debug("getting settings for frontend")
	if a.config == nil {
		return &types.Config{
			OpenAIAPIKey:    "",
			OpenAIBaseURL:   "https://api.openai.com/v1",
			OpenAIModel:     "gpt-4",
			ContextWindow:   8192,
			DefaultCompiler: "pdflatex",
			WorkDirectory:   "",
			Concurrency:     3,
			GitHubToken:     "",
			GitHubOwner:     "",
			GitHubRepo:      "",
			LibraryPageSize: 20,
		}
	}
	cfg := a.config.GetConfig()

	// Log the actual key length for debugging
	logger.Debug("actual API key info", logger.Int("keyLength", len(cfg.OpenAIAPIKey)))

	// Mask API key for display - use fixed length mask to avoid revealing key length
	// Format: "****" + last 4 chars, or just "****" if key is too short
	maskedKey := ""
	if cfg.OpenAIAPIKey != "" {
		if len(cfg.OpenAIAPIKey) > 4 {
			maskedKey = "********" + cfg.OpenAIAPIKey[len(cfg.OpenAIAPIKey)-4:]
		} else {
			maskedKey = "********"
		}
	}

	// Get GitHub token from config
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}
	
	// If no token in settings.json, use token from config
	if githubToken == "" {
		githubToken = cfg.GitHubToken
	}

	// Mask GitHub token for display
	maskedGitHubToken := ""
	if githubToken != "" {
		if len(githubToken) > 4 {
			maskedGitHubToken = "********" + githubToken[len(githubToken)-4:]
		} else {
			maskedGitHubToken = "********"
		}
	}

	logger.Debug("returning masked key", logger.String("maskedKey", maskedKey))

	return &types.Config{
		OpenAIAPIKey:    maskedKey,
		OpenAIBaseURL:   cfg.OpenAIBaseURL,
		OpenAIModel:     cfg.OpenAIModel,
		ContextWindow:   cfg.ContextWindow,
		DefaultCompiler: cfg.DefaultCompiler,
		WorkDirectory:   cfg.WorkDirectory,
		Concurrency:     cfg.Concurrency,
		GitHubToken:     maskedGitHubToken,
		GitHubOwner:     cfg.GitHubOwner,
		GitHubRepo:      cfg.GitHubRepo,
		LibraryPageSize: cfg.LibraryPageSize,
	}
}

// TestAPIConnection tests the API connection with the provided settings.
// Returns nil if successful, or an error message if the test fails.
func (a *App) TestAPIConnection(apiKey, baseURL, model string) error {
	logger.Info("testing API connection", logger.String("baseURL", baseURL), logger.String("model", model))

	if apiKey == "" {
		return types.NewAppError(types.ErrConfig, "API Key 不能为空", nil)
	}
	if baseURL == "" {
		return types.NewAppError(types.ErrConfig, "API Base URL 不能为空", nil)
	}
	if model == "" {
		return types.NewAppError(types.ErrConfig, "模型名称不能为空", nil)
	}

	// If apiKey starts with asterisks (masked key), use the existing key from config
	actualKey := apiKey
	if strings.HasPrefix(apiKey, "****") {
		if a.config != nil {
			actualKey = a.config.GetAPIKey()
		}
		if actualKey == "" {
			return types.NewAppError(types.ErrConfig, "请输入新?API Key", nil)
		}
		logger.Debug("using existing API key for test", logger.Int("keyLength", len(actualKey)))
	}

	// Create a test translator with the provided settings (30 second timeout)
	testTranslator := translator.NewTranslationEngineWithConfig(actualKey, model, baseURL, 30*time.Second, 1)

	// Use the dedicated test connection method
	err := testTranslator.TestConnection()
	if err != nil {
		logger.Error("API connection test failed", err)
		return err
	}

	logger.Info("API connection test successful")
	return nil
}

// SaveSettings saves the application settings from the frontend.
// This method is exposed to the frontend via Wails bindings.
func (a *App) SaveSettings(apiKey, baseURL, model string, contextWindow int, compiler, workDir string, concurrency int, githubToken, githubOwner, githubRepo string, libraryPageSize int) error {
	logger.Info("saving settings from frontend",
		logger.String("baseURL", baseURL),
		logger.String("model", model),
		logger.Int("concurrency", concurrency),
		logger.Int("libraryPageSize", libraryPageSize),
	)

	if a.config == nil {
		configMgr, err := config.NewConfigManager("")
		if err != nil {
			logger.Error("failed to create config manager", err)
			return err
		}
		a.config = configMgr
	}

	// If apiKey starts with asterisks (masked value), don't update it - preserve existing key
	if strings.HasPrefix(apiKey, "****") {
		logger.Debug("API key is masked, preserving existing key")
		apiKey = "" // Empty string means don't update in UpdateConfig
	} else if apiKey != "" {
		logger.Info("Updating API key", logger.Int("keyLength", len(apiKey)))
	}

	// Update config
	if err := a.config.UpdateConfig(apiKey, baseURL, model, contextWindow, compiler, workDir, concurrency, libraryPageSize); err != nil {
		logger.Error("failed to update config", err)
		return err
	}

	// Save GitHub token to config (not settings.json anymore)
	if githubToken != "" && !strings.HasPrefix(githubToken, "****") {
		cfg := a.config.GetConfig()
		cfg.GitHubToken = githubToken
		if err := a.config.Save(); err != nil {
			logger.Error("failed to save GitHub token to config", err)
			return err
		}
		logger.Info("GitHub token saved to config")
	}

	// Note: GitHubOwner and GitHubRepo are no longer saved - they use defaults

	// Reload modules with new settings
	if err := a.ReloadConfig(); err != nil {
		logger.Error("failed to reload config", err)
		return err
	}

	logger.Info("settings saved successfully")
	return nil
}

// OpenFileDialog opens a file selection dialog for selecting zip files.
// Returns the selected file path or empty string if cancelled.
func (a *App) OpenFileDialog() string {
	logger.Debug("opening file dialog")
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 LaTeX 源码 zip 文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Zip 文件 (*.zip)",
				Pattern:     "*.zip",
			},
			{
				DisplayName: "所有文?(*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		logger.Error("file dialog error", err)
		return ""
	}
	logger.Debug("file selected", logger.String("path", selection))
	return selection
}

// OpenDirectoryDialog opens a directory selection dialog.
// Returns the selected directory path or empty string if cancelled.
func (a *App) OpenDirectoryDialog() string {
	logger.Debug("opening directory dialog")
	selection, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择工作目录",
	})
	if err != nil {
		logger.Error("directory dialog error", err)
		return ""
	}
	logger.Debug("directory selected", logger.String("path", selection))
	return selection
}

// GetLastInput returns the last input (arXiv ID, URL, or zip path) from config.
// This is used to restore the input field when the app starts.
func (a *App) GetLastInput() string {
	if a.config == nil {
		return ""
	}
	cfg := a.config.GetConfig()
	return cfg.LastInput
}

// SaveLastInput saves the last input to config for later restoration.
func (a *App) SaveLastInput(input string) {
	if a.config == nil {
		return
	}
	a.config.SetLastInput(input)
}

// GetPDFDataURL reads a PDF file and returns it as a data URL for display in iframe.
// This is needed because Wails WebView doesn't allow loading file:// URLs directly.
func (a *App) GetPDFDataURL(pdfPath string) (string, error) {
	if pdfPath == "" {
		return "", types.NewAppError(types.ErrInvalidInput, "PDF 路径为空", nil)
	}

	// Read the PDF file
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		logger.Error("failed to read PDF file", err, logger.String("path", pdfPath))
		return "", types.NewAppError(types.ErrFileNotFound, "无法读取 PDF 文件", err)
	}

	// Encode as base64 data URL
	encoded := base64.StdEncoding.EncodeToString(data)
	dataURL := "data:application/pdf;base64," + encoded

	logger.Debug("PDF converted to data URL", logger.String("path", pdfPath), logger.Int("size", len(data)))
	return dataURL, nil
}

// OpenPDFInSystem opens a PDF file using the system's default PDF viewer.
func (a *App) OpenPDFInSystem(pdfPath string) error {
	if pdfPath == "" {
		return types.NewAppError(types.ErrInvalidInput, "PDF 路径为空", nil)
	}

	// Check if file exists
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return types.NewAppError(types.ErrFileNotFound, "PDF ļ", err)
	}

	// Open with system default application
	logger.Info("opening PDF in system viewer", logger.String("path", pdfPath))
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(pdfPath))
	return nil
}

// OpenURLInBrowser opens a URL in the system's default browser.
func (a *App) OpenURLInBrowser(url string) {
	logger.Info("opening URL in browser", logger.String("url", url))
	runtime.BrowserOpenURL(a.ctx, url)
}

// StartupCheckResult contains the results of startup environment checks.
type StartupCheckResult struct {
	LaTeXInstalled bool   `json:"latex_installed"`
	LaTeXVersion   string `json:"latex_version"`
	LLMConfigured  bool   `json:"llm_configured"`
	LLMError       string `json:"llm_error"`
}

// CheckStartupRequirements checks if LaTeX is installed and LLM is properly configured.
// This should be called at startup to verify the environment is ready.
func (a *App) CheckStartupRequirements() *StartupCheckResult {
	logger.Info("checking startup requirements")
	result := &StartupCheckResult{}

	// Check LaTeX installation
	result.LaTeXInstalled, result.LaTeXVersion = a.checkLaTeXInstallation()
	logger.Info("LaTeX check result",
		logger.Bool("installed", result.LaTeXInstalled),
		logger.String("version", result.LaTeXVersion))

	// Check LLM configuration
	result.LLMConfigured, result.LLMError = a.checkLLMConfiguration()
	logger.Info("LLM check result",
		logger.Bool("configured", result.LLMConfigured),
		logger.String("error", result.LLMError))

	return result
}

// checkLaTeXInstallation checks if LaTeX (pdflatex or xelatex) is installed.
func (a *App) checkLaTeXInstallation() (bool, string) {
	// Try xelatex first (preferred for Chinese support)
	compilers := []string{"xelatex", "pdflatex", "lualatex"}

	for _, compiler := range compilers {
		version, err := a.getCompilerVersion(compiler)
		if err == nil && version != "" {
			return true, fmt.Sprintf("%s: %s", compiler, version)
		}
	}

	return false, ""
}

// getCompilerVersion gets the version string of a LaTeX compiler.
func (a *App) getCompilerVersion(compiler string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
		cmd = exec.CommandContext(ctx, "cmd", "/c", compiler, "--version")
	} else {
		cmd = exec.CommandContext(ctx, compiler, "--version")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Extract first line as version info
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}

	return "", fmt.Errorf("no version output")
}

// checkLLMConfiguration checks if LLM is properly configured and can connect.
func (a *App) checkLLMConfiguration() (bool, string) {
	if a.config == nil {
		return false, "配置未初始化"
	}

	apiKey := a.config.GetAPIKey()
	if apiKey == "" {
		return false, "API Key 未配置"
	}

	baseURL := a.config.GetBaseURL()
	if baseURL == "" {
		return false, "API Base URL 未配置"
	}

	model := a.config.GetModel()
	if model == "" {
		return false, "模型未配置"
	}

	// Test the connection
	err := a.TestAPIConnection(apiKey, baseURL, model)
	if err != nil {
		return false, err.Error()
	}

	return true, ""
}

// GetLaTeXDownloadURL returns the appropriate LaTeX download URL based on the operating system.
func (a *App) GetLaTeXDownloadURL() string {
	osName := strings.ToLower(os.Getenv("OS"))

	if strings.Contains(osName, "windows") {
		// MiKTeX is recommended for Windows
		return "https://miktex.org/download"
	}

	// For other systems, check runtime.GOOS
	switch goos := strings.ToLower(fmt.Sprintf("%s", os.Getenv("GOOS"))); {
	case strings.Contains(goos, "darwin"):
		// MacTeX for macOS
		return "https://www.tug.org/mactex/mactex-download.html"
	default:
		// TeX Live for Linux and others
		return "https://www.tug.org/texlive/acquire-netinstall.html"
	}
}

// DownloadChinesePDF saves the translated Chinese PDF to a user-selected location.
func (a *App) DownloadChinesePDF() (string, error) {
	if a.lastResult == nil || a.lastResult.TranslatedPDFPath == "" {
		return "", types.NewAppError(types.ErrInvalidInput, "没有可下载的中文 PDF", nil)
	}

	// Generate default filename based on source ID
	defaultFilename := "translated.pdf"
	if a.lastResult.SourceID != "" {
		defaultFilename = a.lastResult.SourceID + ".pdf"
	}

	// Open save dialog
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存中文 PDF",
		DefaultFilename: defaultFilename,
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF ļ (*.pdf)", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		logger.Error("save dialog error", err)
		return "", types.NewAppError(types.ErrInternal, "򿪱Իʧ", err)
	}
	if savePath == "" {
		return "", nil // User cancelled
	}

	// Copy the file
	srcData, err := os.ReadFile(a.lastResult.TranslatedPDFPath)
	if err != nil {
		logger.Error("failed to read source PDF", err)
		return "", types.NewAppError(types.ErrFileNotFound, "ȡԴ PDF ʧ", err)
	}

	if err := os.WriteFile(savePath, srcData, 0644); err != nil {
		logger.Error("failed to write PDF", err)
		return "", types.NewAppError(types.ErrInternal, "保存 PDF 失败", err)
	}

	logger.Info("Chinese PDF saved", logger.String("path", savePath))
	return savePath, nil
}

// DownloadBilingualPDF saves the bilingual PDF (English left, Chinese right) to a user-selected location.
// If the bilingual PDF was already generated during translation, it will be copied directly.
// Otherwise, it will be generated on-demand using LaTeX.
func (a *App) DownloadBilingualPDF() (string, error) {
	if a.lastResult == nil {
		return "", types.NewAppError(types.ErrInvalidInput, "没有可下载的内容", nil)
	}
	if a.lastResult.OriginalPDFPath == "" || a.lastResult.TranslatedPDFPath == "" {
		return "", types.NewAppError(types.ErrInvalidInput, "原始或翻译 PDF 不存在", nil)
	}

	// Generate default filename based on source ID with _biling suffix
	defaultFilename := "bilingual.pdf"
	if a.lastResult.SourceID != "" {
		defaultFilename = a.lastResult.SourceID + "_biling.pdf"
	}

	// Open save dialog
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存中英对照 PDF",
		DefaultFilename: defaultFilename,
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF 文件 (*.pdf)", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		logger.Error("save dialog error", err)
		return "", types.NewAppError(types.ErrInternal, "打开保存对话框失败", err)
	}
	if savePath == "" {
		return "", nil // User cancelled
	}

	// Check if bilingual PDF already exists (generated during translation)
	if a.lastResult.BilingualPDFPath != "" {
		if _, err := os.Stat(a.lastResult.BilingualPDFPath); err == nil {
			// Copy the existing bilingual PDF
			srcData, err := os.ReadFile(a.lastResult.BilingualPDFPath)
			if err != nil {
				logger.Error("failed to read bilingual PDF", err)
				return "", types.NewAppError(types.ErrFileNotFound, "读取双语 PDF 失败", err)
			}
			if err := os.WriteFile(savePath, srcData, 0644); err != nil {
				logger.Error("failed to write bilingual PDF", err)
				return "", types.NewAppError(types.ErrInternal, "保存双语 PDF 失败", err)
			}
			logger.Info("Bilingual PDF saved (from cache)", logger.String("path", savePath))
			return savePath, nil
		}
	}

	// Bilingual PDF not available, generate it on-demand
	logger.Info("generating bilingual PDF on-demand")
	generator := pdf.NewPDFGenerator(a.workDir)

	// Generate side-by-side PDF: English (original) on left, Chinese (translated) on right
	err = generator.GenerateSideBySidePDF(
		a.lastResult.OriginalPDFPath,   // Left: English original
		a.lastResult.TranslatedPDFPath, // Right: Chinese translation
		savePath,
	)
	if err != nil {
		logger.Error("failed to generate bilingual PDF", err)
		// Fallback to zip if LaTeX compilation fails
		return a.downloadBilingualPDFAsZip(savePath)
	}

	logger.Info("Bilingual PDF saved", logger.String("path", savePath))
	return savePath, nil
}

// downloadBilingualPDFAsZip is a fallback method that creates a zip with both PDFs
// when LaTeX-based side-by-side generation fails (e.g., LaTeX not installed)
func (a *App) downloadBilingualPDFAsZip(originalSavePath string) (string, error) {
	// Change extension from .pdf to .zip
	savePath := strings.TrimSuffix(originalSavePath, ".pdf") + ".zip"

	logger.Warn("falling back to zip format for bilingual PDF", logger.String("path", savePath))

	// Create a zip file containing both PDFs
	zipFile, err := os.Create(savePath)
	if err != nil {
		logger.Error("failed to create zip file", err)
		return "", types.NewAppError(types.ErrInternal, "创建 zip 文件失败", err)
	}
	defer zipFile.Close()

	zipWriter := NewZipWriter(zipFile)
	defer zipWriter.Close()

	// Add Chinese PDF
	chinesePDF, err := os.ReadFile(a.lastResult.TranslatedPDFPath)
	if err != nil {
		logger.Error("failed to read Chinese PDF", err)
		return "", types.NewAppError(types.ErrFileNotFound, "读取中文 PDF 失败", err)
	}
	if err := zipWriter.AddFile("chinese.pdf", chinesePDF); err != nil {
		return "", types.NewAppError(types.ErrInternal, "添加中文 PDF ?zip 失败", err)
	}

	// Add English PDF
	englishPDF, err := os.ReadFile(a.lastResult.OriginalPDFPath)
	if err != nil {
		logger.Error("failed to read English PDF", err)
		return "", types.NewAppError(types.ErrFileNotFound, "读取英文 PDF 失败", err)
	}
	if err := zipWriter.AddFile("english.pdf", englishPDF); err != nil {
		return "", types.NewAppError(types.ErrInternal, "添加英文 PDF ?zip 失败", err)
	}

	logger.Info("Bilingual PDF zip saved (fallback)", logger.String("path", savePath))
	return savePath, nil
}

// DownloadLatexZip packages all translated LaTeX files into a zip archive.
func (a *App) DownloadLatexZip() (string, error) {
	if a.lastResult == nil || a.lastResult.SourceInfo == nil {
		return "", types.NewAppError(types.ErrInvalidInput, "没有可下载的 LaTeX 文件", nil)
	}

	extractDir := a.lastResult.SourceInfo.ExtractDir
	if extractDir == "" {
		return "", types.NewAppError(types.ErrInvalidInput, "源码目录不存在", nil)
	}

	// Generate default filename based on source ID
	defaultFilename := "translated_latex.zip"
	if a.lastResult.SourceID != "" {
		defaultFilename = a.lastResult.SourceID + "_latex.zip"
	}

	// Open save dialog
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存翻译后的 LaTeX 文件",
		DefaultFilename: defaultFilename,
		Filters: []runtime.FileFilter{
			{DisplayName: "Zip 文件 (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		logger.Error("save dialog error", err)
		return "", types.NewAppError(types.ErrInternal, "打开保存对话框失败", err)
	}
	if savePath == "" {
		return "", nil // User cancelled
	}

	// Create zip file
	zipFile, err := os.Create(savePath)
	if err != nil {
		logger.Error("failed to create zip file", err)
		return "", types.NewAppError(types.ErrInternal, "创建 zip 文件失败", err)
	}
	defer zipFile.Close()

	zipWriter := NewZipWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the extract directory and add all files
	err = filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and output directories
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), "output_") {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(extractDir, path)
		if err != nil {
			return err
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Add to zip
		return zipWriter.AddFile(relPath, content)
	})

	if err != nil {
		logger.Error("failed to create LaTeX zip", err)
		return "", types.NewAppError(types.ErrInternal, "打包 LaTeX 文件失败", err)
	}

	logger.Info("LaTeX zip saved", logger.String("path", savePath))
	return savePath, nil
}

// =============================================================================
// PDF Translation Methods (Wails Bindings)
// =============================================================================

// LoadPDF loads a PDF file and extracts text for translation.
// This method is exposed to the frontend via Wails bindings.
// Requirements: 1.1 - 加载 PDF 文件并显示在左侧预览区域
// Requirements: 5.1 - 提供 PDF 翻译模式入口
func (a *App) LoadPDF(filePath string) (*pdf.PDFInfo, error) {
	logger.Info("LoadPDF called", logger.String("path", filePath))

	if a.pdfTranslator == nil {
		return nil, types.NewAppError(types.ErrInternal, "PDF 翻译器未初始化", nil)
	}

	return a.pdfTranslator.LoadPDF(filePath)
}

// TranslatePDF executes the PDF translation process.
// This method is exposed to the frontend via Wails bindings.
// Requirements: 3.5 - 显示翻译进度（已完成块数/总块数）
func (a *App) TranslatePDF() (*pdf.TranslationResult, error) {
	logger.Info("TranslatePDF called")

	if a.pdfTranslator == nil {
		return nil, types.NewAppError(types.ErrInternal, "PDF 翻译器未初始化", nil)
	}

	return a.pdfTranslator.TranslatePDF()
}

// GetPDFStatus returns the current PDF translation status.
// This method is exposed to the frontend via Wails bindings.
// Requirements: 5.5 - 显示当前处理状态（加载中、提取中、翻译中、生成中等）
func (a *App) GetPDFStatus() *pdf.PDFStatus {
	logger.Debug("GetPDFStatus called")

	if a.pdfTranslator == nil {
		return &pdf.PDFStatus{
			Phase:   pdf.PDFPhaseError,
			Message: "PDF 翻译器未初始化",
		}
	}

	return a.pdfTranslator.GetStatus()
}

// ==================== Result Management Methods ====================

// PaperListItem represents a paper in the list for frontend display
type PaperListItem struct {
	ArxivID      string `json:"arxiv_id"`
	Title        string `json:"title"`
	TranslatedAt string `json:"translated_at"`
}

// ListTranslatedPapers returns a list of all translated papers
func (a *App) ListTranslatedPapers() ([]PaperListItem, error) {
	logger.Debug("ListTranslatedPapers called")

	if a.results == nil {
		return nil, types.NewAppError(types.ErrInternal, "结果管理器未初始化", nil)
	}

	papers, err := a.results.ListPapers()
	if err != nil {
		logger.Error("failed to list papers", err)
		return nil, types.NewAppError(types.ErrInternal, "获取论文列表失败", err)
	}

	items := make([]PaperListItem, len(papers))
	for i, p := range papers {
		items[i] = PaperListItem{
			ArxivID:      p.ArxivID,
			Title:        p.Title,
			TranslatedAt: p.TranslatedAt.Format("2006-01-02 15:04"),
		}
	}

	logger.Info("listed papers", logger.Int("count", len(items)))
	return items, nil
}

// DeleteTranslatedPaper deletes a translated paper and all its files
func (a *App) DeleteTranslatedPaper(arxivID string) error {
	logger.Info("DeleteTranslatedPaper called", logger.String("arxivID", arxivID))

	if a.results == nil {
		return types.NewAppError(types.ErrInternal, "结果管理器未初始化", nil)
	}

	if arxivID == "" {
		return types.NewAppError(types.ErrInvalidInput, "arXiv ID 不能为空", nil)
	}

	if err := a.results.DeletePaper(arxivID); err != nil {
		logger.Error("failed to delete paper", err, logger.String("arxivID", arxivID))
		return types.NewAppError(types.ErrInternal, "删除论文失败", err)
	}

	logger.Info("paper deleted successfully", logger.String("arxivID", arxivID))
	return nil
}

// RetranslateFromArxiv re-downloads and re-translates a paper from arXiv
func (a *App) RetranslateFromArxiv(arxivID string) (*types.ProcessResult, error) {
	logger.Info("RetranslateFromArxiv called", logger.String("arxivID", arxivID))

	if arxivID == "" {
		return nil, types.NewAppError(types.ErrInvalidInput, "arXiv ID 不能为空", nil)
	}

	// Delete existing result first
	if a.results != nil && a.results.PaperExists(arxivID) {
		if err := a.results.DeletePaper(arxivID); err != nil {
			logger.Warn("failed to delete existing paper before retranslation", logger.Err(err))
		}
	}

	// Process the arXiv ID (this will download fresh source from arXiv)
	return a.ProcessSource(arxivID)
}

// ContinueTranslation continues a previously failed or incomplete translation
// It uses the saved source files and tries to continue from where it left off
// The method intelligently resumes from the last successful phase based on saved status
func (a *App) ContinueTranslation(arxivID string) (*types.ProcessResult, error) {
	logger.Info("ContinueTranslation called", logger.String("arxivID", arxivID))

	if arxivID == "" {
		return nil, types.NewAppError(types.ErrInvalidInput, "arXiv ID 不能为空", nil)
	}

	if a.results == nil {
		return nil, types.NewAppError(types.ErrInternal, "结果管理器未初始化", nil)
	}

	// Load paper info
	info, err := a.results.LoadPaperInfo(arxivID)
	if err != nil {
		logger.Error("failed to load paper info", err, logger.String("arxivID", arxivID))
		return nil, types.NewAppError(types.ErrFileNotFound, "论文不存在", err)
	}

	// Check if we have source files to continue from
	if !info.HasLatexSource || info.SourceDir == "" {
		logger.Info("no source files available, re-downloading from original input")
		// If we have the original input, use it to re-download
		if info.OriginalInput != "" {
			return a.ProcessSource(info.OriginalInput)
		}
		// Otherwise try using the arXiv ID
		return a.ProcessSource(arxivID)
	}

	// Check if source directory exists
	if _, err := os.Stat(info.SourceDir); os.IsNotExist(err) {
		logger.Info("source directory no longer exists, re-downloading")
		if info.OriginalInput != "" {
			return a.ProcessSource(info.OriginalInput)
		}
		return a.ProcessSource(arxivID)
	}

	// Continue from saved source
	logger.Info("continuing translation from saved source",
		logger.String("sourceDir", info.SourceDir),
		logger.String("status", string(info.Status)))

	// Copy source to work directory for processing
	workDir := filepath.Join(a.workDir, fmt.Sprintf("continue_%s_%d", arxivID, time.Now().Unix()))
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, types.NewAppError(types.ErrInternal, "创建工作目录失败", err)
	}

	// Copy source files
	if err := copyDir(info.SourceDir, workDir); err != nil {
		return nil, types.NewAppError(types.ErrInternal, "复制源文件失败", err)
	}

	// Create source info
	sourceInfo := &types.SourceInfo{
		ExtractDir:  workDir,
		MainTexFile: info.MainTexFile,
		SourceType:  types.SourceTypeArxivID,
		OriginalRef: arxivID,
	}

	// If we don't have the main tex file, find it
	if sourceInfo.MainTexFile == "" {
		mainTexFile, err := a.downloader.FindMainTexFile(workDir)
		if err != nil {
			return nil, types.NewAppError(types.ErrFileNotFound, "未找到主 tex 文件", err)
		}
		sourceInfo.MainTexFile = mainTexFile
	}

	// Continue processing based on status - intelligently resume from last successful phase
	return a.continueProcessingFromStatus(sourceInfo, arxivID, info.Title, info.Status, info.OriginalPDF)
}

// continueProcessingFromStatus continues processing based on the saved status
// This allows intelligent resumption from the last successful phase
func (a *App) continueProcessingFromStatus(sourceInfo *types.SourceInfo, arxivID, title string, status results.TranslationStatus, savedOriginalPDF string) (*types.ProcessResult, error) {
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFunc = cancel
	defer func() {
		a.cancelFunc = nil
	}()

	mainTexPath := filepath.Join(sourceInfo.ExtractDir, sourceInfo.MainTexFile)
	translatedTexPath := filepath.Join(sourceInfo.ExtractDir, "translated_"+sourceInfo.MainTexFile)
	originalOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_original")
	translatedOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_translated")

	var originalPDFPath string

	logger.Info("continuing from status",
		logger.String("status", string(status)),
		logger.String("savedOriginalPDF", savedOriginalPDF))

	// Determine starting point based on status
	switch status {
	case results.StatusTranslated, results.StatusCompiling:
		// Translation is complete, just need to compile
		// Check if translated file exists
		if _, statErr := os.Stat(translatedTexPath); statErr == nil {
			logger.Info("resuming from translated status - skipping translation phase")
			// Use saved original PDF if available, otherwise compile original first
			if savedOriginalPDF != "" {
				originalPDFPath = savedOriginalPDF
				a.safeEmit(EventOriginalPDFReady, originalPDFPath)
			} else {
				// Compile original document first
				a.updateStatus(types.PhaseCompiling, 30, "编译原始文档...")
				originalResult, err := a.compiler.Compile(mainTexPath, originalOutputDir)
				if err != nil || !originalResult.Success {
					errMsg := "原始文档编译失败"
					if originalResult != nil && originalResult.ErrorMsg != "" {
						errMsg = originalResult.ErrorMsg
					}
					a.updateStatusError(errMsg)
					return nil, types.NewAppError(types.ErrCompile, errMsg, err)
				}
				originalPDFPath = originalResult.PDFPath
				a.safeEmit(EventOriginalPDFReady, originalPDFPath)
			}

			// Go directly to compile translated document
			return a.compileTranslatedDocument(sourceInfo, arxivID, title, originalPDFPath, translatedTexPath, translatedOutputDir)
		}
		// Translated file doesn't exist, fall through to full processing
		logger.Warn("translated file not found, starting from beginning")
		fallthrough

	case results.StatusOriginalCompiled, results.StatusTranslating:
		// Original is compiled, need to translate and compile
		// Check if we have the original PDF
		if savedOriginalPDF != "" {
			originalPDFPath = savedOriginalPDF
			a.safeEmit(EventOriginalPDFReady, originalPDFPath)
			logger.Info("resuming from original_compiled status - skipping original compilation")

			// Start from translation
			return a.translateAndCompile(sourceInfo, arxivID, title, originalPDFPath, mainTexPath, translatedTexPath, translatedOutputDir)
		}
		// No saved original PDF, fall through to full processing
		logger.Warn("original PDF not found, starting from beginning")
		fallthrough

	default:
		// Start from the beginning (extracted, pending, error, etc.)
		logger.Info("starting from beginning")

		// Preprocess tex files
		a.updateStatus(types.PhaseExtracting, 22, "预处理 LaTeX 源文件...")
		if err := compiler.PreprocessTexFiles(sourceInfo.ExtractDir); err != nil {
			logger.Warn("preprocessing failed", logger.Err(err))
		}

		// Compile original document
		a.updateStatus(types.PhaseCompiling, 30, "编译原始文档...")
		originalResult, err := a.compiler.Compile(mainTexPath, originalOutputDir)
		if err != nil || !originalResult.Success {
			errMsg := "原始文档编译失败"
			if originalResult != nil && originalResult.ErrorMsg != "" {
				errMsg = originalResult.ErrorMsg
			}
			a.updateStatusError(errMsg)
			a.saveIntermediateResult(arxivID, title, arxivID, sourceInfo, results.StatusError, errMsg, "", "")
			return nil, types.NewAppError(types.ErrCompile, errMsg, err)
		}

		originalPDFPath = originalResult.PDFPath
		a.safeEmit(EventOriginalPDFReady, originalPDFPath)
		a.saveIntermediateResult(arxivID, title, arxivID, sourceInfo, results.StatusOriginalCompiled, "", originalPDFPath, "")

		// Check for cancellation
		if ctx.Err() != nil {
			return nil, types.NewAppError(types.ErrInternal, "处理已取消", ctx.Err())
		}

		// Continue with translation and compilation
		return a.translateAndCompile(sourceInfo, arxivID, title, originalPDFPath, mainTexPath, translatedTexPath, translatedOutputDir)
	}
}

// translateAndCompile handles the translation and compilation phases
func (a *App) translateAndCompile(sourceInfo *types.SourceInfo, arxivID, title, originalPDFPath, mainTexPath, translatedTexPath, translatedOutputDir string) (*types.ProcessResult, error) {
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFunc = cancel
	defer func() {
		a.cancelFunc = nil
	}()

	mainFileName := filepath.Base(mainTexPath)

	// Translate
	a.updateStatus(types.PhaseTranslating, 42, "开始翻译文档...")
	translatedFiles, _, err := a.translateAllTexFiles(mainTexPath, sourceInfo.ExtractDir, func(current, total int, message string) {
		progress := 42 + (current * 16 / total)
		a.updateStatus(types.PhaseTranslating, progress, message)
	})
	if err != nil {
		a.updateStatusError(fmt.Sprintf("翻译失败: %v", err))
		a.saveIntermediateResult(arxivID, title, arxivID, sourceInfo, results.StatusError, err.Error(), originalPDFPath, "")
		return nil, err
	}

	a.saveIntermediateResult(arxivID, title, arxivID, sourceInfo, results.StatusTranslated, "", originalPDFPath, "")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, types.NewAppError(types.ErrInternal, "处理已取消", ctx.Err())
	}

	// Save translated files
	a.updateStatus(types.PhaseValidating, 60, "保存翻译文件...")
	translatedContent := translatedFiles[mainFileName]
	translatedContent = ensureCtexPackage(translatedContent)
	translatedFiles[mainFileName] = translatedContent

	for relPath, content := range translatedFiles {
		fixedContent, _ := compiler.QuickFix(content)
		var savePath string
		if relPath == mainFileName {
			savePath = filepath.Join(sourceInfo.ExtractDir, "translated_"+relPath)
		} else {
			savePath = filepath.Join(sourceInfo.ExtractDir, relPath)
		}
		if err := os.WriteFile(savePath, []byte(fixedContent), 0644); err != nil {
			logger.Warn("failed to save translated file", logger.String("path", savePath), logger.Err(err))
		}
	}

	// Compile translated document
	return a.compileTranslatedDocument(sourceInfo, arxivID, title, originalPDFPath, translatedTexPath, translatedOutputDir)
}

// compileTranslatedDocument handles the final compilation phase
func (a *App) compileTranslatedDocument(sourceInfo *types.SourceInfo, arxivID, title, originalPDFPath, translatedTexPath, translatedOutputDir string) (*types.ProcessResult, error) {
	a.updateStatus(types.PhaseCompiling, 75, "编译中文文档...")
	a.saveIntermediateResult(arxivID, title, arxivID, sourceInfo, results.StatusCompiling, "", originalPDFPath, "")

	translatedResult, err := a.compiler.CompileWithXeLaTeX(translatedTexPath, translatedOutputDir)

	if err != nil || !translatedResult.Success {
		// Try hierarchical fix
		apiKey := a.config.GetAPIKey()
		baseURL := a.config.GetBaseURL()
		model := a.config.GetModel()
		fixer := compiler.NewLaTeXFixerWithAgent(apiKey, baseURL, model, model, true)

		fixResult, _ := fixer.HierarchicalFixCompilationErrors(
			sourceInfo.ExtractDir,
			filepath.Base(translatedTexPath),
			translatedResult.Log,
			a.compiler,
			translatedOutputDir,
			func(level compiler.FixLevel, attempt int, message string) {
				progress := 78 + attempt*3
				if progress > 98 {
					progress = 98
				}
				a.updateStatus(types.PhaseValidating, progress, message)
			},
		)

		if fixResult != nil && fixResult.Success {
			translatedResult, err = a.compiler.CompileWithXeLaTeX(translatedTexPath, translatedOutputDir)
		}
	}

	if err != nil || !translatedResult.Success {
		errMsg := "中文文档编译失败"
		if translatedResult != nil && translatedResult.ErrorMsg != "" {
			errMsg = translatedResult.ErrorMsg
		}
		a.updateStatusError(errMsg)
		a.saveIntermediateResult(arxivID, title, arxivID, sourceInfo, results.StatusError, errMsg, originalPDFPath, "")
		return nil, types.NewAppError(types.ErrCompile, errMsg, err)
	}

	a.safeEmit(EventTranslatedPDFReady, translatedResult.PDFPath)

	// Complete
	a.updateStatus(types.PhaseComplete, 100, "翻译完成")

	result := &types.ProcessResult{
		OriginalPDFPath:   originalPDFPath,
		TranslatedPDFPath: translatedResult.PDFPath,
		SourceInfo:        sourceInfo,
		SourceID:          arxivID, // Use arxivID as source ID for continued translations
	}

	a.lastResult = result
	a.saveResultToPermanentStorage(result, arxivID, title)

	return result, nil
}

// continueProcessingFromSource continues processing from extracted source (legacy method for compatibility)
func (a *App) continueProcessingFromSource(sourceInfo *types.SourceInfo, arxivID, title string) (*types.ProcessResult, error) {
	// Use the new status-aware method with default status
	return a.continueProcessingFromStatus(sourceInfo, arxivID, title, results.StatusExtracted, "")
}

// OpenPaperResult opens a previously translated paper for viewing
func (a *App) OpenPaperResult(arxivID string) (*types.ProcessResult, error) {
	logger.Info("OpenPaperResult called", logger.String("arxivID", arxivID))

	if a.results == nil {
		return nil, types.NewAppError(types.ErrInternal, "结果管理器未初始化", nil)
	}

	info, err := a.results.LoadPaperInfo(arxivID)
	if err != nil {
		logger.Error("failed to load paper info", err, logger.String("arxivID", arxivID))
		return nil, types.NewAppError(types.ErrFileNotFound, "论文不存在", err)
	}

	result := &types.ProcessResult{
		OriginalPDFPath:   info.OriginalPDF,
		TranslatedPDFPath: info.TranslatedPDF,
		BilingualPDFPath:  info.BilingualPDF,
		SourceID:          arxivID,
	}

	// Store result for download
	a.lastResult = result

	// Emit events to frontend
	if info.OriginalPDF != "" {
		a.safeEmit(EventOriginalPDFReady, info.OriginalPDF)
	}
	if info.TranslatedPDF != "" {
		a.safeEmit(EventTranslatedPDFReady, info.TranslatedPDF)
	}

	logger.Info("paper result opened", logger.String("arxivID", arxivID))
	return result, nil
}

// GetResultsDirectory returns the base directory for translation results
func (a *App) GetResultsDirectory() string {
	if a.results == nil {
		return ""
	}
	return a.results.GetBaseDir()
}

// saveIntermediateResult saves an intermediate result during translation process
// This allows resuming translation if it fails or is interrupted
func (a *App) saveIntermediateResult(arxivID, title, originalInput string, sourceInfo *types.SourceInfo, status results.TranslationStatus, errorMsg string, originalPDF, translatedPDF string) error {
	if a.results == nil {
		return nil
	}

	// Determine source type and calculate MD5 if needed
	var sourceType results.SourceType
	var sourceMD5 string
	var sourceFileName string

	parsedType, _ := parser.ParseInput(originalInput)
	switch parsedType {
	case types.SourceTypeArxivID, types.SourceTypeURL:
		sourceType = results.SourceTypeArxiv
	case types.SourceTypeLocalZip:
		sourceType = results.SourceTypeZip
		// Calculate MD5 for zip files
		if md5Hash, err := results.CalculateFileMD5(originalInput); err == nil {
			sourceMD5 = md5Hash
		}
		sourceFileName = filepath.Base(originalInput)
	case types.SourceTypeLocalPDF:
		sourceType = results.SourceTypePDF
		// Calculate MD5 for PDF files
		if md5Hash, err := results.CalculateFileMD5(originalInput); err == nil {
			sourceMD5 = md5Hash
		}
		sourceFileName = filepath.Base(originalInput)
	}

	// For non-arXiv sources without an arXiv ID, generate one from MD5
	if arxivID == "" && sourceMD5 != "" {
		arxivID = "md5_" + sourceMD5[:16]
	}

	if arxivID == "" {
		return nil // Can't save without identifier
	}

	logger.Info("saving intermediate result",
		logger.String("arxivID", arxivID),
		logger.String("status", string(status)),
		logger.String("sourceType", string(sourceType)))

	paperDir := a.results.GetPaperDir(arxivID)
	if err := os.MkdirAll(paperDir, 0755); err != nil {
		return err
	}

	// Copy original PDF if available
	originalDst := ""
	if originalPDF != "" {
		originalDst = a.results.GetOriginalPDFPath(arxivID)
		if err := copyFile(originalPDF, originalDst); err != nil {
			logger.Warn("failed to copy original PDF", logger.Err(err))
			originalDst = ""
		}
	}

	// Copy translated PDF if available
	translatedDst := ""
	if translatedPDF != "" {
		translatedDst = a.results.GetTranslatedPDFPath(arxivID)
		if err := copyFile(translatedPDF, translatedDst); err != nil {
			logger.Warn("failed to copy translated PDF", logger.Err(err))
			translatedDst = ""
		}
	}

	// Copy LaTeX source directory if available
	latexDst := a.results.GetLatexSourceDir(arxivID)
	hasLatexSource := false
	mainTexFile := ""
	if sourceInfo != nil && sourceInfo.ExtractDir != "" {
		if err := copyDir(sourceInfo.ExtractDir, latexDst); err != nil {
			logger.Warn("failed to copy LaTeX source", logger.Err(err))
		} else {
			hasLatexSource = true
			mainTexFile = sourceInfo.MainTexFile
		}
	}

	// Save metadata
	info := &results.PaperInfo{
		ArxivID:        arxivID,
		Title:          title,
		TranslatedAt:   time.Now(),
		OriginalPDF:    originalDst,
		TranslatedPDF:  translatedDst,
		SourceDir:      latexDst,
		HasLatexSource: hasLatexSource,
		Status:         status,
		ErrorMessage:   errorMsg,
		OriginalInput:  originalInput,
		MainTexFile:    mainTexFile,
		SourceType:     sourceType,
		SourceMD5:      sourceMD5,
		SourceFileName: sourceFileName,
	}

	if err := a.results.SavePaperInfo(info); err != nil {
		logger.Error("failed to save paper info", err)
		return err
	}

	logger.Info("intermediate result saved", logger.String("arxivID", arxivID), logger.String("status", string(status)))
	return nil
}

// saveResultToPermanentStorage saves the translation result to permanent storage
func (a *App) saveResultToPermanentStorage(result *types.ProcessResult, arxivID, title string) error {
	if a.results == nil {
		return nil // Silently skip if result manager not initialized
	}

	if arxivID == "" {
		return nil // Can't save without arXiv ID
	}

	logger.Info("saving result to permanent storage", logger.String("arxivID", arxivID))

	paperDir := a.results.GetPaperDir(arxivID)
	if err := os.MkdirAll(paperDir, 0755); err != nil {
		return err
	}

	// Copy original PDF
	originalDst := a.results.GetOriginalPDFPath(arxivID)
	if result.OriginalPDFPath != "" {
		if err := copyFile(result.OriginalPDFPath, originalDst); err != nil {
			logger.Warn("failed to copy original PDF", logger.Err(err))
		}
	}

	// Copy translated PDF
	translatedDst := a.results.GetTranslatedPDFPath(arxivID)
	if result.TranslatedPDFPath != "" {
		if err := copyFile(result.TranslatedPDFPath, translatedDst); err != nil {
			logger.Warn("failed to copy translated PDF", logger.Err(err))
		}
	}

	// Copy bilingual PDF
	bilingualDst := a.results.GetBilingualPDFPath(arxivID)
	if result.BilingualPDFPath != "" {
		if err := copyFile(result.BilingualPDFPath, bilingualDst); err != nil {
			logger.Warn("failed to copy bilingual PDF", logger.Err(err))
		}
	}

	// Copy LaTeX source directory
	latexDst := a.results.GetLatexSourceDir(arxivID)
	hasLatexSource := false
	mainTexFile := ""
	if result.SourceInfo != nil && result.SourceInfo.ExtractDir != "" {
		if err := copyDir(result.SourceInfo.ExtractDir, latexDst); err != nil {
			logger.Warn("failed to copy LaTeX source", logger.Err(err))
		} else {
			hasLatexSource = true
			mainTexFile = result.SourceInfo.MainTexFile
		}
	}

	// Save metadata with complete status
	info := &results.PaperInfo{
		ArxivID:        arxivID,
		Title:          title,
		TranslatedAt:   time.Now(),
		OriginalPDF:    originalDst,
		TranslatedPDF:  translatedDst,
		BilingualPDF:   bilingualDst,
		SourceDir:      latexDst,
		HasLatexSource: hasLatexSource,
		Status:         results.StatusComplete,
		MainTexFile:    mainTexFile,
	}

	if err := a.results.SavePaperInfo(info); err != nil {
		logger.Error("failed to save paper info", err)
		return err
	}

	logger.Info("result saved to permanent storage", logger.String("arxivID", arxivID))
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip output directories
		if info.IsDir() && strings.HasPrefix(info.Name(), "output_") {
			return filepath.SkipDir
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		return copyFile(path, dstPath)
	})
}

// CancelPDFTranslation cancels the current PDF translation operation.
// This method is exposed to the frontend via Wails bindings.
// Requirements: 7.6 - 如果翻译过程中断，保存已完成的翻译进度以便恢复
func (a *App) CancelPDFTranslation() error {
	logger.Info("CancelPDFTranslation called")

	if a.pdfTranslator == nil {
		return types.NewAppError(types.ErrInternal, "PDF 翻译器未初始化", nil)
	}

	return a.pdfTranslator.CancelTranslation()
}

// GetTranslatedPDFPath returns the path to the translated PDF file.
// This method is exposed to the frontend via Wails bindings.
func (a *App) GetTranslatedPDFPath() string {
	logger.Debug("GetTranslatedPDFPath called")

	if a.pdfTranslator == nil {
		return ""
	}

	return a.pdfTranslator.GetTranslatedPDFPath()
}

// OpenPDFFileDialog opens a file selection dialog for selecting PDF files.
// Returns the selected file path or empty string if cancelled.
// This method is exposed to the frontend via Wails bindings.
// Requirements: 1.1 - 用户选择本地 PDF 文件
func (a *App) OpenPDFFileDialog() string {
	logger.Debug("opening PDF file dialog")
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 PDF 文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "PDF ļ (*.pdf)",
				Pattern:     "*.pdf",
			},
			{
				DisplayName: "所有文?(*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		logger.Error("PDF file dialog error", err)
		return ""
	}
	logger.Debug("PDF file selected", logger.String("path", selection))
	return selection
}

// ZipWriter is a helper for creating zip archives
type ZipWriter struct {
	writer *zip.Writer
}

// NewZipWriter creates a new ZipWriter
func NewZipWriter(w io.Writer) *ZipWriter {
	return &ZipWriter{writer: zip.NewWriter(w)}
}

// AddFile adds a file to the zip archive
func (z *ZipWriter) AddFile(name string, content []byte) error {
	f, err := z.writer.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	return err
}

// Close closes the zip writer
func (z *ZipWriter) Close() error {
	return z.writer.Close()
}

// ensureCtexPackage ensures the ctex package is included in the LaTeX document for Chinese support.
// It adds \usepackage{ctex} after \documentclass if not already present.
// It also fixes microtype compatibility issues with XeLaTeX.
func ensureCtexPackage(content string) string {
	// Check if ctex is already included
	if strings.Contains(content, "\\usepackage{ctex}") || strings.Contains(content, "\\usepackage[") && strings.Contains(content, "ctex") {
		logger.Debug("ctex package already present")
	} else {
		// Find the first uncommented \documentclass line and add \usepackage{ctex} after it
		lines := strings.Split(content, "\n")
		var result []string
		added := false

		for _, line := range lines {
			result = append(result, line)
			// Skip if already added or if line is commented
			if added || strings.HasPrefix(strings.TrimSpace(line), "%") {
				continue
			}
			// Check if this line contains \documentclass
			if strings.Contains(line, "\\documentclass") {
				result = append(result, "\\usepackage{ctex}")
				added = true
				logger.Info("added ctex package for Chinese support")
			}
		}

		if added {
			content = strings.Join(result, "\n")
		} else {
			logger.Warn("could not find \\documentclass to add ctex package")
		}
	}

	// Fix microtype compatibility with XeLaTeX
	// microtype with expansion/protrusion can cause "Cannot use XeTeXglyph" errors
	// Replace \usepackage{microtype} with a XeLaTeX-compatible version
	if strings.Contains(content, "\\usepackage{microtype}") {
		content = strings.Replace(content, "\\usepackage{microtype}",
			"\\usepackage[protrusion=false,expansion=false]{microtype}", 1)
		logger.Info("fixed microtype package for XeLaTeX compatibility")
	}

	// Also handle microtype with options
	microtypePattern := regexp.MustCompile(`\\usepackage\[([^\]]*)\]\{microtype\}`)
	if microtypePattern.MatchString(content) {
		// Check if protrusion/expansion are already disabled
		if !strings.Contains(content, "protrusion=false") {
			content = microtypePattern.ReplaceAllString(content,
				"\\usepackage[protrusion=false,expansion=false]{microtype}")
			logger.Info("fixed microtype package options for XeLaTeX compatibility")
		}
	}

	return content
}

// findInputFiles finds all tex files referenced by \input or \include commands in the given content.
// It recursively scans all referenced files to find nested \input commands.
// It returns a list of relative file paths in the order they should be processed.
func findInputFiles(content string, baseDir string) []string {
	seen := make(map[string]bool)
	var files []string

	// Use recursive helper function
	findInputFilesRecursive(content, baseDir, seen, &files, "")

	return files
}

// findInputFilesRecursive recursively finds all tex files referenced by \input or \include commands.
// It handles nested \input commands by scanning each found file for additional references.
func findInputFilesRecursive(content string, baseDir string, seen map[string]bool, files *[]string, currentDir string) {
	// Pattern to match \input{...} and \include{...}
	// Also handles \input{path/file} without .tex extension
	inputPattern := regexp.MustCompile(`\\(?:input|include)\s*\{([^}]+)\}`)

	matches := inputPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			filePath := strings.TrimSpace(match[1])
			// Add .tex extension if not present
			if !strings.HasSuffix(filePath, ".tex") {
				filePath = filePath + ".tex"
			}

			// Handle relative paths from the current file's directory
			if currentDir != "" && !filepath.IsAbs(filePath) && !strings.HasPrefix(filePath, currentDir) {
				// Check if file exists relative to current directory first
				relToCurrentPath := filepath.Join(currentDir, filePath)
				fullPathFromCurrent := filepath.Join(baseDir, relToCurrentPath)
				if _, err := os.Stat(fullPathFromCurrent); err == nil {
					filePath = relToCurrentPath
				}
			}

			// Normalize path separators
			filePath = filepath.Clean(filePath)

			// Check if file exists
			fullPath := filepath.Join(baseDir, filePath)
			if _, err := os.Stat(fullPath); err == nil {
				if !seen[filePath] {
					seen[filePath] = true
					*files = append(*files, filePath)
					logger.Debug("found input file", logger.String("file", filePath))

					// Recursively scan this file for more \input commands
					nestedContent, err := os.ReadFile(fullPath)
					if err == nil {
						// Get the directory of the current file for relative path resolution
						fileDir := filepath.Dir(filePath)
						findInputFilesRecursive(string(nestedContent), baseDir, seen, files, fileDir)
					}
				}
			} else {
				logger.Debug("input file not found", logger.String("file", filePath), logger.String("fullPath", fullPath))
			}
		}
	}
}

// translateAllTexFiles translates the main tex file and all referenced input files.
// It returns a map of file paths to their translated content.
func (a *App) translateAllTexFiles(mainTexPath string, baseDir string, progressCallback func(current, total int, message string)) (map[string]string, int, error) {
	results := make(map[string]string)
	originalContents := make(map[string]string) // Store original content for reference-based fixes
	totalTokens := 0

	// Read main tex file
	mainContent, err := os.ReadFile(mainTexPath)
	if err != nil {
		return nil, 0, types.NewAppError(types.ErrFileNotFound, "读取?tex 文件失败", err)
	}

	// Find all input files
	inputFiles := findInputFiles(string(mainContent), baseDir)
	logger.Info("found input files", logger.Int("count", len(inputFiles)))

	// Collect all files to translate (main file + input files)
	allFiles := []string{filepath.Base(mainTexPath)}
	allFiles = append(allFiles, inputFiles...)

	totalFiles := len(allFiles)
	currentFile := 0

	// Translate each file
	for _, relPath := range allFiles {
		currentFile++
		fullPath := filepath.Join(baseDir, relPath)

		logger.Info("processing file for translation",
			logger.String("relPath", relPath),
			logger.String("fullPath", fullPath),
			logger.Int("current", currentFile),
			logger.Int("total", totalFiles))

		content, err := os.ReadFile(fullPath)
		if err != nil {
			logger.Error("failed to read input file", err, logger.String("file", relPath), logger.String("fullPath", fullPath))
			// Don't skip - return error to fail fast
			return nil, 0, types.NewAppError(types.ErrFileNotFound, fmt.Sprintf("ȡļʧ: %s", relPath), err)
		}

		// Store original content for reference-based fixes
		originalContents[relPath] = string(content)

		logger.Debug("file read successfully",
			logger.String("file", relPath),
			logger.Int("contentLength", len(content)))

		// Skip empty files
		if len(strings.TrimSpace(string(content))) == 0 {
			logger.Debug("skipping empty file", logger.String("file", relPath))
			results[relPath] = string(content)
			continue
		}

		logger.Info("translating file", logger.String("file", relPath), logger.Int("current", currentFile), logger.Int("total", totalFiles))

		// Translate with progress callback
		result, err := a.translator.TranslateTeXWithProgress(string(content), func(chunkCurrent, chunkTotal int, message string) {
			if progressCallback != nil {
				// Calculate overall progress
				fileProgress := float64(currentFile-1) / float64(totalFiles)
				chunkProgress := float64(chunkCurrent) / float64(chunkTotal) / float64(totalFiles)
				overallProgress := int((fileProgress + chunkProgress) * 100)
				progressCallback(overallProgress, 100, fmt.Sprintf("翻译 %s (%d/%d 文件, %d/%d 分块)...", relPath, currentFile, totalFiles, chunkCurrent, chunkTotal))
			}
		})

		if err != nil {
			logger.Error("failed to translate file", err, logger.String("file", relPath))
			return nil, 0, err
		}

		// Apply reference-based fixes using original content
		translatedContent := result.TranslatedContent
		if original, ok := originalContents[relPath]; ok {
			fixedContent, wasFixed := compiler.QuickFixWithReference(translatedContent, original)
			if wasFixed {
				logger.Info("applied reference-based fixes to translated file",
					logger.String("relPath", relPath))
				translatedContent = fixedContent
			}
		}

		results[relPath] = translatedContent
		totalTokens += result.TokensUsed
		logger.Info("file translated", logger.String("file", relPath), logger.Int("tokens", result.TokensUsed))
	}

	return results, totalTokens, nil
}

// ==================== GitHub Share Methods ====================

// ShareCheckResult represents the result of checking if files can be shared
type ShareCheckResult struct {
	CanShare           bool   `json:"can_share"`
	ChinesePDFExists   bool   `json:"chinese_pdf_exists"`
	BilingualPDFExists bool   `json:"bilingual_pdf_exists"`
	ChinesePDFPath     string `json:"chinese_pdf_path"`
	BilingualPDFPath   string `json:"bilingual_pdf_path"`
	Message            string `json:"message"`
}

// ShareResult represents the result of a share operation
type ShareResult struct {
	Success         bool   `json:"success"`
	ChinesePDFURL   string `json:"chinese_pdf_url,omitempty"`
	BilingualPDFURL string `json:"bilingual_pdf_url,omitempty"`
	Message         string `json:"message"`
}

// CheckShareStatus checks if the current result can be shared and if files already exist on GitHub
func (a *App) CheckShareStatus() (*ShareCheckResult, error) {
	logger.Debug("CheckShareStatus called")

	// Check if we have a result to share
	if a.lastResult == nil {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "没有可分享的翻译结果",
		}, nil
	}

	// Log lastResult for debugging
	logger.Debug("CheckShareStatus lastResult",
		logger.String("SourceID", a.lastResult.SourceID),
		logger.String("OriginalPDFPath", a.lastResult.OriginalPDFPath),
		logger.String("TranslatedPDFPath", a.lastResult.TranslatedPDFPath))
	if a.lastResult.SourceInfo != nil {
		logger.Debug("CheckShareStatus SourceInfo",
			logger.String("OriginalRef", a.lastResult.SourceInfo.OriginalRef))
	}

	// Get GitHub token from config
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}
	if githubToken == "" {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "请先在设置中配置 GitHub Token",
		}, nil
	}

	// Use default owner and repo
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	// Extract arXiv ID - try multiple sources
	arxivID := ""
	// First try SourceID (set by OpenPaperResult)
	if a.lastResult.SourceID != "" {
		logger.Debug("Trying to extract arXiv ID from SourceID", logger.String("SourceID", a.lastResult.SourceID))
		arxivID = results.ExtractArxivID(a.lastResult.SourceID)
		logger.Debug("ExtractArxivID result", logger.String("arxivID", arxivID))
		// If SourceID is already a valid arXiv ID format, use it directly
		if arxivID == "" {
			// Check if it looks like an arXiv ID directly
			if strings.Contains(a.lastResult.SourceID, ".") && len(a.lastResult.SourceID) >= 9 {
				arxivID = a.lastResult.SourceID
				logger.Debug("Using SourceID directly as arXiv ID", logger.String("arxivID", arxivID))
			}
		}
	}
	// Then try SourceInfo.OriginalRef
	if arxivID == "" && a.lastResult.SourceInfo != nil {
		logger.Debug("Trying to extract arXiv ID from SourceInfo.OriginalRef", logger.String("OriginalRef", a.lastResult.SourceInfo.OriginalRef))
		arxivID = results.ExtractArxivID(a.lastResult.SourceInfo.OriginalRef)
	}

	logger.Debug("Final arXiv ID", logger.String("arxivID", arxivID))

	if arxivID == "" {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "只能分享 arXiv 论文",
		}, nil
	}

	// Create uploader and validate token
	uploader := github.NewUploader(githubToken, owner, repo)
	if err := uploader.ValidateToken(); err != nil {
		return &ShareCheckResult{
			CanShare: false,
			Message:  err.Error(),
		}, nil
	}

	// Check if files exist on GitHub
	safeID := strings.ReplaceAll(arxivID, "/", "_")
	chinesePath := fmt.Sprintf("%s_cn.pdf", safeID)
	bilingualPath := fmt.Sprintf("%s_bilingual.pdf", safeID)

	chineseExists, _ := uploader.CheckFileExists(chinesePath)
	bilingualExists, _ := uploader.CheckFileExists(bilingualPath)

	return &ShareCheckResult{
		CanShare:           true,
		ChinesePDFExists:   chineseExists != nil && chineseExists.Exists,
		BilingualPDFExists: bilingualExists != nil && bilingualExists.Exists,
		ChinesePDFPath:     chinesePath,
		BilingualPDFPath:   bilingualPath,
		Message:            "可以分享",
	}, nil
}

// ShareToGitHub uploads the translated PDFs to GitHub
// uploadChinese: whether to upload Chinese PDF (will overwrite if exists)
// uploadBilingual: whether to upload bilingual PDF (will overwrite if exists)
func (a *App) ShareToGitHub(uploadChinese, uploadBilingual bool) (*ShareResult, error) {
	logger.Info("ShareToGitHub called",
		logger.Bool("uploadChinese", uploadChinese),
		logger.Bool("uploadBilingual", uploadBilingual))

	// Check if we have a result to share
	if a.lastResult == nil {
		return nil, types.NewAppError(types.ErrInvalidInput, "没有可分享的翻译结果", nil)
	}

	// Get GitHub token from config
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}
	if githubToken == "" {
		return nil, types.NewAppError(types.ErrConfig, "请先在设置中配置 GitHub Token", nil)
	}

	// Use default owner and repo
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	// Extract arXiv ID - try multiple sources
	arxivID := ""
	// First try SourceID (set by OpenPaperResult)
	if a.lastResult.SourceID != "" {
		arxivID = results.ExtractArxivID(a.lastResult.SourceID)
		// If SourceID is already a valid arXiv ID format, use it directly
		if arxivID == "" && (strings.Contains(a.lastResult.SourceID, ".") || len(a.lastResult.SourceID) > 4) {
			arxivID = a.lastResult.SourceID
		}
	}
	// Then try SourceInfo.OriginalRef
	if arxivID == "" && a.lastResult.SourceInfo != nil {
		arxivID = results.ExtractArxivID(a.lastResult.SourceInfo.OriginalRef)
	}

	if arxivID == "" {
		return nil, types.NewAppError(types.ErrInvalidInput, "只能分享 arXiv 论文", nil)
	}

	// Create uploader
	uploader := github.NewUploader(githubToken, owner, repo)

	safeID := strings.ReplaceAll(arxivID, "/", "_")
	result := &ShareResult{Success: true}
	var uploadedCount int

	// Upload Chinese PDF if requested
	if uploadChinese && a.lastResult.TranslatedPDFPath != "" {
		chinesePath := fmt.Sprintf("%s_cn.pdf", safeID)
		commitMsg := fmt.Sprintf("Add Chinese translation for %s", arxivID)

		// Always overwrite if user chose to upload (they've been warned about existing files)
		uploadResult, err := uploader.UploadFile(a.lastResult.TranslatedPDFPath, chinesePath, commitMsg, true)
		if err != nil {
			logger.Error("failed to upload Chinese PDF", err)
			return nil, types.NewAppError(types.ErrInternal, "上传中文 PDF 失败: "+err.Error(), err)
		}

		if uploadResult.Success {
			result.ChinesePDFURL = uploadResult.URL
			uploadedCount++
		}
	}

	// Upload bilingual PDF if requested
	if uploadBilingual && a.lastResult.OriginalPDFPath != "" && a.lastResult.TranslatedPDFPath != "" {
		bilingualPath := fmt.Sprintf("%s_bilingual.pdf", safeID)
		commitMsg := fmt.Sprintf("Add bilingual PDF for %s", arxivID)

		// Check if bilingual PDF already exists (generated during translation)
		bilingualLocalPath := a.lastResult.BilingualPDFPath
		needsGeneration := bilingualLocalPath == ""
		if bilingualLocalPath != "" {
			if _, err := os.Stat(bilingualLocalPath); err != nil {
				needsGeneration = true
			}
		}

		// Generate bilingual PDF if not available
		if needsGeneration {
			logger.Info("generating bilingual PDF for sharing")
			tempDir, err := os.MkdirTemp("", "share_bilingual_*")
			if err != nil {
				logger.Warn("failed to create temp dir for bilingual PDF", logger.Err(err))
			} else {
				defer os.RemoveAll(tempDir)
				bilingualLocalPath = filepath.Join(tempDir, "bilingual.pdf")
				generator := pdf.NewPDFGenerator(tempDir)
				if err := generator.GenerateSideBySidePDF(a.lastResult.OriginalPDFPath, a.lastResult.TranslatedPDFPath, bilingualLocalPath); err != nil {
					logger.Warn("failed to generate bilingual PDF for sharing", logger.Err(err))
					bilingualLocalPath = ""
				}
			}
		}

		// Upload if we have a bilingual PDF
		if bilingualLocalPath != "" {
			uploadResult, err := uploader.UploadFile(bilingualLocalPath, bilingualPath, commitMsg, true)
			if err != nil {
				logger.Warn("failed to upload bilingual PDF", logger.Err(err))
			} else if uploadResult.Success {
				result.BilingualPDFURL = uploadResult.URL
				uploadedCount++
			}
		}
	}

	if uploadedCount > 0 {
		result.Message = "分享成功"
	} else {
		result.Success = false
		result.Message = "没有文件被上传"
	}

	logger.Info("share completed",
		logger.Int("uploadedCount", uploadedCount),
		logger.String("chineseURL", result.ChinesePDFURL),
		logger.String("bilingualURL", result.BilingualPDFURL))

	return result, nil
}

// CheckShareStatusForPaper checks if a specific paper can be shared and if files already exist on GitHub
func (a *App) CheckShareStatusForPaper(arxivID string) (*ShareCheckResult, error) {
	logger.Debug("CheckShareStatusForPaper called", logger.String("arxivID", arxivID))

	if a.results == nil {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "结果管理器未初始化",
		}, nil
	}

	// Get paper info
	paper, err := a.results.LoadPaperInfo(arxivID)
	if err != nil {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "未找到该论文: " + err.Error(),
		}, nil
	}

	// Check if translation is complete
	if paper.Status != results.StatusComplete {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "只能分享已完成翻译的论文",
		}, nil
	}

	// Check if translated PDF exists
	if paper.TranslatedPDF == "" {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "翻译后的 PDF 不存在",
		}, nil
	}

	// Get GitHub token from config
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}
	if githubToken == "" {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "请先在设置中配置 GitHub Token",
		}, nil
	}

	// Use default owner and repo
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	// Create uploader and validate token
	uploader := github.NewUploader(githubToken, owner, repo)
	if err := uploader.ValidateToken(); err != nil {
		return &ShareCheckResult{
			CanShare: false,
			Message:  err.Error(),
		}, nil
	}

	// Check if files exist on GitHub
	safeID := strings.ReplaceAll(arxivID, "/", "_")
	chinesePath := fmt.Sprintf("%s_cn.pdf", safeID)
	bilingualPath := fmt.Sprintf("%s_bilingual.pdf", safeID)

	chineseExists, _ := uploader.CheckFileExists(chinesePath)
	bilingualExists, _ := uploader.CheckFileExists(bilingualPath)

	return &ShareCheckResult{
		CanShare:           true,
		ChinesePDFExists:   chineseExists != nil && chineseExists.Exists,
		BilingualPDFExists: bilingualExists != nil && bilingualExists.Exists,
		ChinesePDFPath:     chinesePath,
		BilingualPDFPath:   bilingualPath,
		Message:            "可以分享",
	}, nil
}

// SharePaperToGitHub uploads a specific paper's PDFs to GitHub
// arxivID: the arXiv ID of the paper to share
// uploadChinese: whether to upload Chinese PDF (will overwrite if exists)
// uploadBilingual: whether to upload bilingual PDF (will overwrite if exists)
func (a *App) SharePaperToGitHub(arxivID string, uploadChinese, uploadBilingual bool) (*ShareResult, error) {
	logger.Info("SharePaperToGitHub called",
		logger.String("arxivID", arxivID),
		logger.Bool("uploadChinese", uploadChinese),
		logger.Bool("uploadBilingual", uploadBilingual))

	if a.results == nil {
		return nil, types.NewAppError(types.ErrInternal, "结果管理器未初始化", nil)
	}

	// Get paper info
	paper, err := a.results.LoadPaperInfo(arxivID)
	if err != nil {
		return nil, types.NewAppError(types.ErrInvalidInput, "未找到该论文: "+err.Error(), err)
	}

	// Check if translation is complete
	if paper.Status != results.StatusComplete {
		return nil, types.NewAppError(types.ErrInvalidInput, "只能分享已完成翻译的论文", nil)
	}

	// Get GitHub token from config
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}
	if githubToken == "" {
		return nil, types.NewAppError(types.ErrConfig, "请先在设置中配置 GitHub Token", nil)
	}

	// Use default owner and repo
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	// Create uploader
	uploader := github.NewUploader(githubToken, owner, repo)

	safeID := strings.ReplaceAll(arxivID, "/", "_")
	result := &ShareResult{Success: true}
	var uploadedCount int

	// Upload Chinese PDF if requested
	if uploadChinese && paper.TranslatedPDF != "" {
		chinesePath := fmt.Sprintf("%s_cn.pdf", safeID)
		commitMsg := fmt.Sprintf("Add Chinese translation for %s", arxivID)

		uploadResult, err := uploader.UploadFile(paper.TranslatedPDF, chinesePath, commitMsg, true)
		if err != nil {
			logger.Error("failed to upload Chinese PDF", err)
			return nil, types.NewAppError(types.ErrInternal, "上传中文 PDF 失败: "+err.Error(), err)
		}

		if uploadResult.Success {
			result.ChinesePDFURL = uploadResult.URL
			uploadedCount++
		}
	}

	// Generate and upload bilingual PDF if requested
	if uploadBilingual && paper.OriginalPDF != "" && paper.TranslatedPDF != "" {
		bilingualPath := fmt.Sprintf("%s_bilingual.pdf", safeID)
		commitMsg := fmt.Sprintf("Add bilingual PDF for %s", arxivID)

		// Check if bilingual PDF already exists
		bilingualLocalPath := paper.BilingualPDF
		needsGeneration := bilingualLocalPath == ""
		if bilingualLocalPath != "" {
			if _, err := os.Stat(bilingualLocalPath); err != nil {
				needsGeneration = true
			}
		}

		// Generate bilingual PDF if not available
		if needsGeneration {
			logger.Info("generating bilingual PDF for sharing paper")
			tempDir, err := os.MkdirTemp("", "share_bilingual_*")
			if err != nil {
				logger.Warn("failed to create temp dir for bilingual PDF", logger.Err(err))
			} else {
				defer os.RemoveAll(tempDir)
				bilingualLocalPath = filepath.Join(tempDir, "bilingual.pdf")
				generator := pdf.NewPDFGenerator(tempDir)
				if err := generator.GenerateSideBySidePDF(paper.OriginalPDF, paper.TranslatedPDF, bilingualLocalPath); err != nil {
					logger.Warn("failed to generate bilingual PDF for sharing", logger.Err(err))
					bilingualLocalPath = ""
				}
			}
		}

		// Upload if we have a bilingual PDF
		if bilingualLocalPath != "" {
			uploadResult, err := uploader.UploadFile(bilingualLocalPath, bilingualPath, commitMsg, true)
			if err != nil {
				logger.Warn("failed to upload bilingual PDF", logger.Err(err))
			} else if uploadResult.Success {
				result.BilingualPDFURL = uploadResult.URL
				uploadedCount++
			}
		}
	}

	if uploadedCount > 0 {
		result.Message = "分享成功"
	} else {
		result.Success = false
		result.Message = "没有文件被上传"
	}

	logger.Info("share paper completed",
		logger.String("arxivID", arxivID),
		logger.Int("uploadedCount", uploadedCount),
		logger.String("chineseURL", result.ChinesePDFURL),
		logger.String("bilingualURL", result.BilingualPDFURL))

	return result, nil
}

// TestGitHubConnection tests the GitHub connection with the provided token
// Uses default owner and repo settings
func (a *App) TestGitHubConnection(token string) error {
	logger.Debug("TestGitHubConnection called")

	// If token is masked, get the actual token from config
	actualToken := token
	if strings.HasPrefix(token, "****") {
		if a.config != nil {
			cfg := a.config.GetConfig()
			actualToken = cfg.GitHubToken
		}
		if actualToken == "" {
			return types.NewAppError(types.ErrInvalidInput, "请输入新的 GitHub Token", nil)
		}
	}

	if actualToken == "" {
		return types.NewAppError(types.ErrInvalidInput, "请填写 GitHub Token", nil)
	}

	// Use default owner and repo
	uploader := github.NewUploader(actualToken, DefaultGitHubOwner, DefaultGitHubRepo)
	if err := uploader.ValidateToken(); err != nil {
		return types.NewAppError(types.ErrAPICall, err.Error(), err)
	}

	return nil
}

// SearchGitHubTranslation searches for an existing translation on GitHub by arXiv ID
// Returns translation search result with URLs to Chinese PDF, Bilingual PDF, and LaTeX zip
func (a *App) SearchGitHubTranslation(arxivID string) (*github.TranslationSearchResult, error) {
	logger.Info("searching GitHub for translation", logger.String("arxivID", arxivID))

	// Normalize arXiv ID (remove 'arxiv:' prefix if present)
	arxivID = strings.TrimPrefix(strings.ToLower(arxivID), "arxiv:")
	arxivID = strings.TrimSpace(arxivID)

	// Validate arXiv ID format
	if arxivID == "" {
		return nil, types.NewAppError(types.ErrInvalidInput, "请输入有效的 arXiv ID", nil)
	}

	// Get GitHub repository settings from config (same as SharePaperToGitHub)
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	if a.config != nil {
		cfg := a.config.GetConfig()
		if cfg.GitHubOwner != "" {
			owner = cfg.GitHubOwner
		}
		if cfg.GitHubRepo != "" {
			repo = cfg.GitHubRepo
		}
	}

	logger.Info("using GitHub repository",
		logger.String("owner", owner),
		logger.String("repo", repo))

	// Create uploader (no token needed for public searches)
	uploader := github.NewUploader("", owner, repo)

	// Search for translation
	result, err := uploader.SearchTranslation(arxivID)
	if err != nil {
		logger.Error("GitHub search failed", err, logger.String("arxivID", arxivID))
		return nil, types.NewAppError(types.ErrAPICall, "搜索失败: "+err.Error(), err)
	}

	if result.Found {
		logger.Info("translation found on GitHub",
			logger.String("arxivID", arxivID),
			logger.String("chinesePDF", result.ChinesePDF),
			logger.String("bilingualPDF", result.BilingualPDF))
	} else {
		logger.Info("no translation found on GitHub", logger.String("arxivID", arxivID))
	}

	return result, nil
}

// DownloadGitHubTranslation downloads a translation file from GitHub to a user-selected directory
// fileType can be "chinese", "bilingual", or "latex"
func (a *App) DownloadGitHubTranslation(arxivID, fileType, saveDir string) (string, error) {
	logger.Info("downloading GitHub translation",
		logger.String("arxivID", arxivID),
		logger.String("fileType", fileType),
		logger.String("saveDir", saveDir))

	// Normalize arXiv ID
	arxivID = strings.TrimPrefix(strings.ToLower(arxivID), "arxiv:")
	arxivID = strings.TrimSpace(arxivID)

	// Search for the translation first
	uploader := github.NewUploader("", DefaultGitHubOwner, DefaultGitHubRepo)
	searchResult, err := uploader.SearchTranslation(arxivID)
	if err != nil {
		return "", types.NewAppError(types.ErrAPICall, "搜索失败: "+err.Error(), err)
	}

	if !searchResult.Found {
		return "", types.NewAppError(types.ErrFileNotFound, "GitHub 上未找到该论文的翻译", nil)
	}

	// Determine download URL and filename based on file type
	// Use the actual filename from the search result
	var downloadURL, filename string
	switch fileType {
	case "chinese":
		if searchResult.DownloadURLCN == "" || searchResult.ChinesePDFFilename == "" {
			return "", types.NewAppError(types.ErrFileNotFound, "未找到中文 PDF", nil)
		}
		downloadURL = searchResult.DownloadURLCN
		filename = searchResult.ChinesePDFFilename

	case "bilingual":
		if searchResult.DownloadURLBi == "" || searchResult.BilingualPDFFilename == "" {
			return "", types.NewAppError(types.ErrFileNotFound, "未找到双语 PDF", nil)
		}
		downloadURL = searchResult.DownloadURLBi
		filename = searchResult.BilingualPDFFilename

	case "latex":
		if searchResult.DownloadURLZip == "" || searchResult.LatexZipFilename == "" {
			return "", types.NewAppError(types.ErrFileNotFound, "未找到 LaTeX 源码", nil)
		}
		downloadURL = searchResult.DownloadURLZip
		filename = searchResult.LatexZipFilename

	default:
		return "", types.NewAppError(types.ErrInvalidInput, "无效的文件类型: "+fileType, nil)
	}

	// Ensure save directory exists
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return "", types.NewAppError(types.ErrInternal, "创建目录失败: "+err.Error(), err)
	}

	// Full path for the downloaded file (using actual filename from GitHub)
	savePath := filepath.Join(saveDir, filename)

	// Download the file
	logger.Info("downloading file",
		logger.String("url", downloadURL),
		logger.String("filename", filename),
		logger.String("savePath", savePath))
	if err := uploader.DownloadFile(downloadURL, savePath); err != nil {
		logger.Error("download failed", err, logger.String("url", downloadURL))
		return "", types.NewAppError(types.ErrAPICall, "下载失败: "+err.Error(), err)
	}

	logger.Info("download completed", logger.String("savePath", savePath))
	return savePath, nil
}

// ArxivPaperMetadata represents metadata for an arXiv paper
type ArxivPaperMetadata struct {
	ArxivID  string `json:"arxiv_id"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Authors  string `json:"authors"`
}

// ListRecentGitHubTranslations lists the most recent translations from GitHub repository
// Returns up to maxCount papers with their arXiv IDs
func (a *App) ListRecentGitHubTranslations(maxCount int) ([]github.ArxivPaperInfo, error) {
	logger.Info("listing recent GitHub translations", logger.Int("maxCount", maxCount))

	// Get GitHub repository settings from config
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	if a.config != nil {
		cfg := a.config.GetConfig()
		if cfg.GitHubOwner != "" {
			owner = cfg.GitHubOwner
		}
		if cfg.GitHubRepo != "" {
			repo = cfg.GitHubRepo
		}
	}

	logger.Info("using GitHub repository",
		logger.String("owner", owner),
		logger.String("repo", repo))

	// Create uploader (no token needed for public searches)
	uploader := github.NewUploader("", owner, repo)

	// List recent translations
	papers, err := uploader.ListRecentTranslations(maxCount)
	if err != nil {
		logger.Error("failed to list GitHub translations", err)
		return nil, types.NewAppError(types.ErrAPICall, "获取列表失败: "+err.Error(), err)
	}

	logger.Info("listed GitHub translations", logger.Int("count", len(papers)))
	return papers, nil
}

// GetArxivPaperMetadata fetches paper metadata from arXiv API
func (a *App) GetArxivPaperMetadata(arxivID string) (*ArxivPaperMetadata, error) {
	logger.Info("fetching arXiv metadata", logger.String("arxivID", arxivID))

	// Normalize arXiv ID
	arxivID = strings.TrimPrefix(strings.ToLower(arxivID), "arxiv:")
	arxivID = strings.TrimSpace(arxivID)

	// arXiv API URL
	apiURL := fmt.Sprintf("http://export.arxiv.org/api/query?id_list=%s", arxivID)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make request
	resp, err := client.Get(apiURL)
	if err != nil {
		logger.Error("failed to fetch arXiv metadata", err)
		return nil, types.NewAppError(types.ErrAPICall, "获取论文信息失败: "+err.Error(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("arXiv API error", nil, logger.Int("status", resp.StatusCode), logger.String("body", string(body)))
		return nil, types.NewAppError(types.ErrAPICall, fmt.Sprintf("arXiv API 错误 (状态 %d)", resp.StatusCode), nil)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "读取响应失败: "+err.Error(), err)
	}

	// Parse XML response
	// arXiv API returns Atom XML format
	// We need to extract title, abstract, and authors
	bodyStr := string(body)

	// Extract title
	titleRegex := regexp.MustCompile(`<title>([^<]+)</title>`)
	titleMatches := titleRegex.FindAllStringSubmatch(bodyStr, -1)
	title := ""
	if len(titleMatches) >= 2 {
		// First match is the feed title, second is the entry title
		title = strings.TrimSpace(titleMatches[1][1])
	}

	// Extract abstract (summary)
	abstractRegex := regexp.MustCompile(`<summary>([^<]+)</summary>`)
	abstractMatches := abstractRegex.FindStringSubmatch(bodyStr)
	abstract := ""
	if len(abstractMatches) >= 2 {
		abstract = strings.TrimSpace(abstractMatches[1])
		// Clean up whitespace
		abstract = regexp.MustCompile(`\s+`).ReplaceAllString(abstract, " ")
	}

	// Extract authors
	authorRegex := regexp.MustCompile(`<name>([^<]+)</name>`)
	authorMatches := authorRegex.FindAllStringSubmatch(bodyStr, -1)
	authors := make([]string, 0)
	for _, match := range authorMatches {
		if len(match) >= 2 {
			authors = append(authors, strings.TrimSpace(match[1]))
		}
	}
	authorsStr := strings.Join(authors, ", ")

	if title == "" {
		return nil, types.NewAppError(types.ErrFileNotFound, "未找到论文信息", nil)
	}

	metadata := &ArxivPaperMetadata{
		ArxivID:  arxivID,
		Title:    title,
		Abstract: abstract,
		Authors:  authorsStr,
	}

	logger.Info("fetched arXiv metadata",
		logger.String("arxivID", arxivID),
		logger.String("title", title))

	return metadata, nil
}

// getTokenPrefix returns the first 20 characters of a token for logging purposes
func getTokenPrefix(token string) string {
	if len(token) <= 20 {
		return token
	}
	return token[:20]
}

// FetchAndDecodeGitHubToken fetches the GitHub token from the remote URL and decodes it
func (a *App) FetchAndDecodeGitHubToken() (string, error) {
	logger.Info("fetching GitHub token from remote URL")

	// Add timestamp to bypass cache
	tokenURL := fmt.Sprintf("https://raw.githubusercontent.com/rapidaicoder/msg/main/papertrans.md?t=%d", time.Now().Unix())

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Fetch the content
	resp, err := client.Get(tokenURL)
	if err != nil {
		logger.Error("failed to fetch token URL", err)
		return "", types.NewAppError(types.ErrAPICall, "获取 Token 失败: "+err.Error(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("token URL returned error", nil, logger.Int("status", resp.StatusCode), logger.String("body", string(body)))
		return "", types.NewAppError(types.ErrAPICall, fmt.Sprintf("获取 Token 失败 (状态 %d)", resp.StatusCode), nil)
	}

	// Read the content
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", types.NewAppError(types.ErrInternal, "读取 Token 内容失败: "+err.Error(), err)
	}

	encodedToken := strings.TrimSpace(string(body))

	// Check if the content looks like a GitHub token (starts with "github_pat_" or "ghp_")
	// If so, use it directly without decoding
	if strings.HasPrefix(encodedToken, "github_pat_") || strings.HasPrefix(encodedToken, "ghp_") {
		logger.Info("fetched GitHub token directly (no decoding needed)", logger.Int("tokenLength", len(encodedToken)))
		return encodedToken, nil
	}

	// Otherwise, try to decode 3 times (for backward compatibility)
	token := encodedToken
	for i := 0; i < 3; i++ {
		// Clean the token: remove any whitespace, newlines, or carriage returns
		token = strings.TrimSpace(token)
		token = strings.ReplaceAll(token, "\n", "")
		token = strings.ReplaceAll(token, "\r", "")
		token = strings.ReplaceAll(token, " ", "")
		
		// Remove any invalid base64 characters (like '[', ']', '{', '}', '|')
		// These might be artifacts from the encoding process
		token = strings.ReplaceAll(token, "[", "")
		token = strings.ReplaceAll(token, "]", "")
		token = strings.ReplaceAll(token, "{", "")
		token = strings.ReplaceAll(token, "}", "")
		token = strings.ReplaceAll(token, "|", "")
		
		decoded, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			// Try with RawStdEncoding (without padding) as fallback
			decoded, err = base64.RawStdEncoding.DecodeString(token)
			if err != nil {
				// If decoding fails, the token might already be decoded
				// Check if it looks like a valid GitHub token
				if strings.HasPrefix(token, "github_pat_") || strings.HasPrefix(token, "ghp_") {
					logger.Info("token appears to be already decoded (has github prefix)", logger.Int("tokenLength", len(token)), logger.Int("iteration", i+1))
					break
				}
				// If it's a reasonable length and doesn't have spaces, assume it's valid
				if len(token) > 20 && !strings.Contains(token, " ") {
					logger.Info("token appears to be already decoded, using as-is", logger.Int("tokenLength", len(token)), logger.Int("iteration", i+1))
					break
				}
				logger.Error("failed to decode token", err, logger.Int("iteration", i+1), logger.String("tokenPrefix", getTokenPrefix(token)))
				return "", types.NewAppError(types.ErrInternal, fmt.Sprintf("解码 Token 失败 (第 %d 次): %v", i+1, err), err)
			}
		}
		token = string(decoded)
		
		// Clean up any non-printable characters from the decoded token
		// GitHub tokens should only contain printable ASCII characters
		cleanToken := ""
		for _, c := range token {
			if c >= 32 && c <= 126 {
				cleanToken += string(c)
			}
		}
		token = cleanToken
	}

	logger.Info("successfully fetched and decoded GitHub token", logger.Int("tokenLength", len(token)))
	return token, nil
}

// UpdateGitHubToken fetches the latest GitHub token and saves it to config
func (a *App) UpdateGitHubToken() error {
	logger.Info("updating GitHub token")

	// Fetch and decode token
	token, err := a.FetchAndDecodeGitHubToken()
	if err != nil {
		return err
	}

	// Save to config
	if a.config == nil {
		configMgr, err := config.NewConfigManager("")
		if err != nil {
			logger.Error("failed to create config manager", err)
			return err
		}
		a.config = configMgr
	}

	// Update config with new token
	cfg := a.config.GetConfig()
	cfg.GitHubToken = token

	// Save config
	if err := a.config.Save(); err != nil {
		logger.Error("failed to save config with new token", err)
		return err
	}

	logger.Info("GitHub token updated successfully")
	return nil
}

// EnsureGitHubToken ensures a GitHub token is available, fetching it if necessary
func (a *App) EnsureGitHubToken() error {
	if a.config == nil {
		configMgr, err := config.NewConfigManager("")
		if err != nil {
			return err
		}
		a.config = configMgr
		if err := a.config.Load(); err != nil {
			logger.Warn("failed to load config", logger.Err(err))
		}
	}

	cfg := a.config.GetConfig()
	
	// If token is empty or very short (likely invalid), fetch a new one
	if cfg.GitHubToken == "" || len(cfg.GitHubToken) < 10 {
		logger.Info("GitHub token not found or invalid, fetching from remote")
		return a.UpdateGitHubToken()
	}

	// Token exists, validate it
	logger.Info("validating existing GitHub token")
	uploader := github.NewUploader(cfg.GitHubToken, DefaultGitHubOwner, DefaultGitHubRepo)
	if err := uploader.ValidateToken(); err != nil {
		logger.Warn("existing GitHub token is invalid, fetching new token", logger.Err(err))
		// Token is invalid, try to fetch a new one
		if updateErr := a.UpdateGitHubToken(); updateErr != nil {
			logger.Error("failed to fetch new token", updateErr)
			return fmt.Errorf("existing token invalid and failed to fetch new token: %w", updateErr)
		}
		logger.Info("successfully fetched and updated new GitHub token")
		return nil
	}

	logger.Info("existing GitHub token is valid")
	
	// Token is valid, but still try to fetch a new one in the background (non-blocking)
	// This ensures we always have the latest token
	go func() {
		logger.Debug("attempting to fetch latest GitHub token in background")
		if err := a.UpdateGitHubToken(); err != nil {
			logger.Debug("background token update failed (this is normal if server token hasn't changed)", logger.Err(err))
		} else {
			logger.Info("successfully updated to latest GitHub token")
		}
	}()

	return nil
}

// ListErrors 列出所有错误记录
func (a *App) ListErrors() ([]*errors.ErrorRecord, error) {
	if a.errorMgr == nil {
		return nil, fmt.Errorf("error manager not initialized")
	}
	return a.errorMgr.ListErrors(), nil
}

// RetryFromError 从错误记录重试翻译
func (a *App) RetryFromError(id string) (*types.ProcessResult, error) {
	if a.errorMgr == nil {
		return nil, fmt.Errorf("error manager not initialized")
	}

	record, ok := a.errorMgr.GetError(id)
	if !ok {
		return nil, fmt.Errorf("error record not found: %s", id)
	}

	logger.Info("retrying translation from error record",
		logger.String("id", id),
		logger.String("input", record.Input),
		logger.String("stage", string(record.Stage)))

	// 增加重试次数
	if err := a.errorMgr.IncrementRetry(id); err != nil {
		logger.Warn("failed to increment retry count", logger.Err(err))
	}

	// 重新开始完整的翻译流程
	result, err := a.ProcessSource(record.Input)
	if err != nil {
		// 重试失败，更新错误记录
		stage := errors.StageDownload
		if record.Stage != "" {
			stage = record.Stage
		}
		if recordErr := a.errorMgr.RecordError(id, record.Title, record.Input, stage, err.Error()); recordErr != nil {
			logger.Warn("failed to update error record", logger.Err(recordErr))
		}
		return nil, err
	}

	// 重试成功，移除错误记录
	if err := a.errorMgr.RemoveError(id); err != nil {
		logger.Warn("failed to remove error record after successful retry", logger.Err(err))
	}

	return result, nil
}

// ClearError 清除特定错误记录
func (a *App) ClearError(id string) error {
	if a.errorMgr == nil {
		return fmt.Errorf("error manager not initialized")
	}
	return a.errorMgr.RemoveError(id)
}

// ClearAllErrors 清除所有错误记录
func (a *App) ClearAllErrors() error {
	if a.errorMgr == nil {
		return fmt.Errorf("error manager not initialized")
	}
	return a.errorMgr.ClearAll()
}

// ExportErrorsToFile 导出错误列表到文本文件
func (a *App) ExportErrorsToFile() (string, error) {
	if a.errorMgr == nil {
		return "", fmt.Errorf("error manager not initialized")
	}

	errorList := a.errorMgr.ListErrors()
	if len(errorList) == 0 {
		return "", fmt.Errorf("no errors to export")
	}

	// 生成文件内容
	var content strings.Builder
	content.WriteString("=== RapidPaperTrans 错误报告 ===\n")
	content.WriteString(fmt.Sprintf("导出时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString(fmt.Sprintf("错误数量: %d\n", len(errorList)))
	content.WriteString("\n")

	for i, err := range errorList {
		content.WriteString(fmt.Sprintf("--- 错误 %d ---\n", i+1))
		content.WriteString(fmt.Sprintf("arXiv ID: %s\n", err.ID))
		content.WriteString(fmt.Sprintf("标题: %s\n", err.Title))
		content.WriteString(fmt.Sprintf("输入: %s\n", err.Input))
		content.WriteString(fmt.Sprintf("阶段: %s (%s)\n", err.Stage, errors.GetStageDisplayName(err.Stage)))
		content.WriteString(fmt.Sprintf("错误信息: %s\n", err.ErrorMsg))
		content.WriteString(fmt.Sprintf("发生时间: %s\n", err.Timestamp.Format("2006-01-02 15:04:05")))
		content.WriteString(fmt.Sprintf("重试次数: %d\n", err.RetryCount))
		if !err.LastRetry.IsZero() {
			content.WriteString(fmt.Sprintf("最后重试: %s\n", err.LastRetry.Format("2006-01-02 15:04:05")))
		}
		content.WriteString("\n")
	}

	// 添加 arXiv ID 列表（方便复制）
	content.WriteString("=== arXiv ID 列表 ===\n")
	for _, err := range errorList {
		content.WriteString(fmt.Sprintf("%s\n", err.ID))
	}

	// 使用文件对话框让用户选择保存位置
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: fmt.Sprintf("error_report_%s.txt", time.Now().Format("20060102_150405")),
		Title:           "导出错误报告",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "文本文件 (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to show save dialog: %w", err)
	}

	if savePath == "" {
		return "", fmt.Errorf("save cancelled")
	}

	// 写入文件
	if err := os.WriteFile(savePath, []byte(content.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	logger.Info("errors exported to file", logger.String("path", savePath))
	return savePath, nil
}

// ExportErrorIDsToFile 导出错误的 arXiv ID 列表到文本文件（每行一个 ID）
func (a *App) ExportErrorIDsToFile() (string, error) {
	if a.errorMgr == nil {
		return "", fmt.Errorf("error manager not initialized")
	}

	errorList := a.errorMgr.ListErrors()
	if len(errorList) == 0 {
		return "", fmt.Errorf("no errors to export")
	}

	// 使用文件对话框让用户选择保存位置
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: fmt.Sprintf("error_arxiv_ids_%s.txt", time.Now().Format("20060102_150405")),
		Title:           "导出错误 arXiv ID 列表",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "文本文件 (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to show save dialog: %w", err)
	}

	if savePath == "" {
		return "", fmt.Errorf("save cancelled")
	}

	// 使用 ErrorManager 的导出方法
	if err := a.errorMgr.ExportErrorIDs(savePath); err != nil {
		return "", fmt.Errorf("failed to export error IDs: %w", err)
	}

	logger.Info("error IDs exported to file", 
		logger.String("path", savePath),
		logger.Int("count", len(errorList)))
	return savePath, nil
}

// recordError 内部方法：记录错误到错误管理器
func (a *App) recordError(id, title, input string, stage errors.ErrorStage, errorMsg string) {
	if a.errorMgr == nil {
		logger.Warn("error manager not initialized, cannot record error")
		return
	}

	if err := a.errorMgr.RecordError(id, title, input, stage, errorMsg); err != nil {
		logger.Warn("failed to record error", logger.Err(err))
	} else {
		logger.Info("error recorded",
			logger.String("id", id),
			logger.String("stage", string(stage)))
	}
}
