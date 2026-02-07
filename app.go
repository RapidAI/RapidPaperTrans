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
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"latex-translator/internal/compiler"
	"latex-translator/internal/config"
	"latex-translator/internal/downloader"
	"latex-translator/internal/errors"
	"latex-translator/internal/github"
	"latex-translator/internal/license"
	"latex-translator/internal/logger"
	"latex-translator/internal/parser"
	"latex-translator/internal/pdf"
	"latex-translator/internal/results"
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
	errorMgr   *errors.ErrorManager
	workDir    string

	// License client for commercial mode
	licenseClient *license.Client

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

	// Initialize license client for commercial mode (needed before checking work mode)
	a.licenseClient = license.NewClient()
	logger.Debug("license client initialized")

	// Check work mode status and apply license config BEFORE initializing translator
	// This ensures the translator uses the correct API key from the license
	// Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5, 3.4
	if a.config != nil {
		workModeStatus := a.config.GetWorkModeStatus()
		logger.Info("work mode status at startup",
			logger.Bool("hasWorkMode", workModeStatus.HasWorkMode),
			logger.String("workMode", string(workModeStatus.WorkMode)),
			logger.Bool("needsSelection", workModeStatus.NeedsSelection),
			logger.Bool("hasValidLicense", workModeStatus.HasValidLicense),
			logger.Bool("licenseExpired", workModeStatus.LicenseExpired),
			logger.Bool("licenseExpiringSoon", workModeStatus.LicenseExpiringSoon))

		// If commercial mode with valid license, update config with license LLM settings
		// This must happen BEFORE translator initialization so GetAPIKey() returns the correct value
		if workModeStatus.IsCommercial && workModeStatus.HasValidLicense {
			licenseInfo := a.config.GetLicenseInfo()
			if licenseInfo != nil && licenseInfo.ActivationData != nil {
				activationData := licenseInfo.ActivationData
				// Get effective base URL - derives from LLMType if LLMBaseURL is empty
				effectiveBaseURL := a.licenseClient.GetEffectiveBaseURL(activationData)
				logger.Info("applying license LLM config before translator init",
					logger.String("llmType", activationData.LLMType),
					logger.String("model", activationData.LLMModel),
					logger.Int("apiKeyLength", len(activationData.LLMAPIKey)),
					logger.String("originalBaseURL", activationData.LLMBaseURL),
					logger.String("effectiveBaseURL", effectiveBaseURL))
				
				// Update config with license values so GetAPIKey/GetModel/GetBaseURL return correct values
				if err := a.config.UpdateConfig(
					activationData.LLMAPIKey,
					effectiveBaseURL, // Use effective base URL instead of raw LLMBaseURL
					activationData.LLMModel,
					a.config.GetContextWindow(),
					a.config.GetDefaultCompiler(),
					a.config.GetWorkDirectory(),
					a.config.GetConcurrency(),
					a.config.GetLibraryPageSize(),
					true, // sharePromptEnabled
				); err != nil {
					logger.Warn("failed to apply license LLM config", logger.Err(err))
				}
			}
		}

		// Emit startup mode event to frontend
		// The frontend will handle showing mode selection or proceeding to main interface
		a.safeEmit("startup-mode-status", map[string]interface{}{
			"has_work_mode":         workModeStatus.HasWorkMode,
			"work_mode":             string(workModeStatus.WorkMode),
			"needs_selection":       workModeStatus.NeedsSelection,
			"is_commercial":         workModeStatus.IsCommercial,
			"is_open_source":        workModeStatus.IsOpenSource,
			"has_valid_license":     workModeStatus.HasValidLicense,
			"license_expired":       workModeStatus.LicenseExpired,
			"license_expiring_soon": workModeStatus.LicenseExpiringSoon,
			"days_until_expiry":     workModeStatus.DaysUntilExpiry,
		})
	}

	// Now initialize translator with API key from config (which now includes license values if applicable)
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
	// Use 10 minute timeout for large projects
	a.compiler = compiler.NewLaTeXCompiler(defaultCompiler, a.workDir, 10*time.Minute)
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
	
	// Set page complete callback for progressive PDF display
	a.pdfTranslator.SetPageCompleteCallback(func(currentPage, totalPages int, outputPath string) {
		// Emit event to frontend for progressive display
		a.safeEmit("pdf-page-translated", map[string]interface{}{
			"currentPage": currentPage,
			"totalPages":  totalPages,
			"outputPath":  outputPath,
		})
		logger.Debug("PDF page translated",
			logger.Int("currentPage", currentPage),
			logger.Int("totalPages", totalPages))
	})
	logger.Debug("PDF translator initialized")

	// Initialize result manager
	resultMgr, err := results.NewResultManager("")
	if err != nil {
		logger.Warn("failed to initialize result manager", logger.Err(err))
	} else {
		a.results = resultMgr
		logger.Debug("result manager initialized", logger.String("baseDir", resultMgr.GetBaseDir()))
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

// applyLicenseLLMConfig applies LLM configuration from the commercial license.
// This is called at startup when commercial mode has a valid license.
// It updates the translator, validator, and PDF translator with the license LLM settings.
// Validates: Requirements 3.4
func (a *App) applyLicenseLLMConfig() {
	if a.config == nil {
		return
	}

	licenseInfo := a.config.GetLicenseInfo()
	if licenseInfo == nil || licenseInfo.ActivationData == nil {
		return
	}

	activationData := licenseInfo.ActivationData
	// Get effective base URL - derives from LLMType if LLMBaseURL is empty
	effectiveBaseURL := a.licenseClient.GetEffectiveBaseURL(activationData)
	logger.Info("applying LLM config from license",
		logger.String("llmType", activationData.LLMType),
		logger.String("model", activationData.LLMModel),
		logger.String("originalBaseURL", activationData.LLMBaseURL),
		logger.String("effectiveBaseURL", effectiveBaseURL))

	// Get current config values for fields we don't want to change
	contextWindow := config.DefaultContextWindow
	compiler := config.DefaultCompiler
	workDir := ""
	concurrency := config.DefaultConcurrency
	libraryPageSize := config.DefaultLibraryPageSize
	sharePromptEnabled := true

	if a.config != nil {
		contextWindow = a.config.GetContextWindow()
		compiler = a.config.GetDefaultCompiler()
		workDir = a.config.GetWorkDirectory()
		concurrency = a.config.GetConcurrency()
		libraryPageSize = a.config.GetLibraryPageSize()
		// Get current share prompt setting from config
		cfg := a.config.GetConfig()
		if cfg != nil {
			sharePromptEnabled = cfg.SharePromptEnabled
		}
	}

	// Update config manager with license LLM settings using UpdateConfig
	if a.config != nil {
		if err := a.config.UpdateConfig(
			activationData.LLMAPIKey,
			effectiveBaseURL, // Use effective base URL instead of raw LLMBaseURL
			activationData.LLMModel,
			contextWindow,
			compiler,
			workDir,
			concurrency,
			libraryPageSize,
			sharePromptEnabled,
		); err != nil {
			logger.Warn("failed to save license LLM config", logger.Err(err))
		}
	}

	// Update translator with new config
	if a.translator != nil {
		a.translator = translator.NewTranslationEngineWithConfig(
			activationData.LLMAPIKey,
			activationData.LLMModel,
			effectiveBaseURL, // Use effective base URL
			0,
			concurrency,
		)
	}

	// Update validator with new config
	if a.validator != nil {
		a.validator = validator.NewSyntaxValidatorWithConfig(
			activationData.LLMAPIKey,
			activationData.LLMModel,
			effectiveBaseURL, // Use effective base URL
			0,
		)
	}

	// Update PDF translator with new config
	if a.pdfTranslator != nil {
		a.pdfTranslator.UpdateConfig(&types.Config{
			OpenAIAPIKey:  activationData.LLMAPIKey,
			OpenAIBaseURL: effectiveBaseURL, // Use effective base URL
			OpenAIModel:   activationData.LLMModel,
			ContextWindow: contextWindow,
			Concurrency:   concurrency,
		})
	}

	logger.Info("license LLM config applied successfully")
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

// IsPDFTranslating returns true if a PDF translation task is currently in progress.
// This method is thread-safe.
func (a *App) IsPDFTranslating() bool {
	if a.pdfTranslator == nil {
		return false
	}

	status := a.pdfTranslator.GetStatus()
	if status == nil {
		return false
	}

	// PDF translation is active if phase is not idle, complete, or error
	switch status.Phase {
	case pdf.PDFPhaseIdle, pdf.PDFPhaseComplete, pdf.PDFPhaseError:
		return false
	default:
		return true
	}
}

// IsAnyTranslationInProgress returns true if any translation task (LaTeX or PDF) is in progress.
// This is used for close confirmation dialog.
func (a *App) IsAnyTranslationInProgress() bool {
	return a.IsProcessing() || a.IsPDFTranslating()
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
// The force parameter behavior depends on the existing translation status:
// - If translation is complete: force=true means re-translate, force=false means return existing
// - If translation can continue: force=true means continue, force=false means restart
// - If translation failed: force=true means retry, force=false means give up
func (a *App) ProcessSourceWithForce(input string, force bool) (*types.ProcessResult, error) {
	logger.Info("processing source with force option",
		logger.String("input", input),
		logger.Bool("force", force))

	// Check if already processing to prevent duplicate calls
	if a.IsProcessing() {
		logger.Warn("ProcessSourceWithForce called while already processing, ignoring duplicate call")
		return nil, types.NewAppError(types.ErrInternal, "已有翻译任务正在进行中", nil)
	}

	// Check for existing translation
	existingInfo, err := a.CheckExistingTranslation(input)
	if err != nil {
		logger.Warn("failed to check existing translation", logger.Err(err))
		// Continue anyway - don't block translation due to check failure
	} else if existingInfo != nil && existingInfo.Exists {
		if existingInfo.IsComplete {
			// Translation is complete
			if force {
				// User wants to re-translate - delete existing and start fresh
				if existingInfo.PaperInfo != nil && existingInfo.PaperInfo.ArxivID != "" {
					logger.Info("deleting existing translation for force re-translation",
						logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
					if err := a.results.DeletePaper(existingInfo.PaperInfo.ArxivID); err != nil {
						logger.Warn("failed to delete existing paper", logger.Err(err))
					}
				}
				// Fall through to ProcessSource
			} else {
				// Return the existing result
				logger.Info("returning existing translation result",
					logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
				return &types.ProcessResult{
					OriginalPDFPath:   existingInfo.PaperInfo.OriginalPDF,
					TranslatedPDFPath: existingInfo.PaperInfo.TranslatedPDF,
					SourceID:          existingInfo.PaperInfo.ArxivID,
				}, nil
			}
		} else if existingInfo.CanContinue {
			// Translation can be continued (interrupted or error state)
			if force {
				// User wants to continue from where it left off
				logger.Info("continuing existing translation",
					logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
				return a.ContinueTranslation(existingInfo.PaperInfo.ArxivID)
			} else {
				// User wants to restart from scratch - delete existing
				if existingInfo.PaperInfo != nil && existingInfo.PaperInfo.ArxivID != "" {
					logger.Info("deleting existing translation for restart",
						logger.String("arxivID", existingInfo.PaperInfo.ArxivID))
					if err := a.results.DeletePaper(existingInfo.PaperInfo.ArxivID); err != nil {
						logger.Warn("failed to delete existing paper", logger.Err(err))
					}
				}
				// Fall through to ProcessSource
			}
		}
		// For other cases (e.g., pending), just start fresh
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

	// Check if already processing to prevent duplicate calls
	if a.IsProcessing() {
		logger.Warn("ProcessSource called while already processing, ignoring duplicate call")
		return nil, types.NewAppError(types.ErrInternal, "已有翻译任务正在进行中", nil)
	}

	// Create a cancellable context for this processing session
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFunc = cancel
	defer func() {
		a.cancelFunc = nil
	}()

	// Reset status to idle at the start
	a.updateStatus(types.PhaseIdle, 0, "开始处理...")

	// Step 1: Parse input to determine source type
	a.updateStatus(types.PhaseDownloading, 5, "解析输入...")
	logger.Debug("parsing input")

	sourceType, err := parser.ParseInput(input)
	if err != nil {
		logger.Error("input parsing failed", err, logger.String("input", input))
		a.updateStatusError(fmt.Sprintf("下载失败: %v", err))
		return nil, err
	}

	logger.Info("input parsed successfully", logger.String("sourceType", string(sourceType)))

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("已取消")
		return nil, types.NewAppError(types.ErrInternal, "已取消", ctx.Err())
	}

	// Step 2: Download/extract source code based on type
	var sourceInfo *types.SourceInfo

	switch sourceType {
	case types.SourceTypeURL:
		a.updateStatus(types.PhaseDownloading, 10, "下载 URL 源码...")
		logger.Info("downloading from URL", logger.String("url", input))
		sourceInfo, err = a.downloader.DownloadFromURL(input)
		if err != nil {
			logger.Error("download from URL failed", err, logger.String("url", input))
			a.updateStatusError(fmt.Sprintf("下载失败: %v", err))
			return nil, err
		}

		// Extract the downloaded archive
		a.updateStatus(types.PhaseExtracting, 20, "解压源码...")
		logger.Debug("extracting downloaded archive")
		sourceInfo, err = a.downloader.ExtractZip(sourceInfo.ExtractDir)
		if err != nil {
			logger.Error("extraction failed", err)
			a.updateStatusError(fmt.Sprintf("解压失败: %v", err))
			return nil, err
		}
		sourceInfo.SourceType = types.SourceTypeURL
		sourceInfo.OriginalRef = input

	case types.SourceTypeArxivID:
		a.updateStatus(types.PhaseDownloading, 10, "下载 arXiv 源码...")
		logger.Info("downloading by arXiv ID", logger.String("arxivID", input))
		sourceInfo, err = a.downloader.DownloadByID(input)
		if err != nil {
			logger.Error("download by ID failed", err, logger.String("arxivID", input))
			a.updateStatusError(fmt.Sprintf("下载失败: %v", err))
			// 记录下载错误
			arxivID := results.ExtractArxivID(input)
			if arxivID != "" {
				a.recordError(arxivID, arxivID, input, errors.StageDownload, err.Error())
			}
			return nil, err
		}

		// Extract the downloaded archive
		a.updateStatus(types.PhaseExtracting, 20, "解压源码...")
		logger.Debug("extracting downloaded archive")
		sourceInfo, err = a.downloader.ExtractZip(sourceInfo.ExtractDir)
		if err != nil {
			logger.Error("extraction failed", err)
			a.updateStatusError(fmt.Sprintf("解压失败: %v", err))
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
			a.updateStatusError(fmt.Sprintf("解压失败: %v", err))
			return nil, err
		}

	default:
		err := types.NewAppError(types.ErrInvalidInput, "不支持的输入类型", nil)
		logger.Error("unsupported input type", err)
		a.updateStatusError(err.Error())
		return nil, err
	}

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("已取消")
		return nil, types.NewAppError(types.ErrInternal, "已取消", ctx.Err())
	}

	// Step 2.5: Preprocess tex files to fix common issues
	a.updateStatus(types.PhaseExtracting, 22, "预处理 LaTeX 源文件...")
	logger.Info("preprocessing tex files", logger.String("extractDir", sourceInfo.ExtractDir))
	if err := compiler.PreprocessTexFiles(sourceInfo.ExtractDir); err != nil {
		// Log warning but don't fail - preprocessing is best-effort
		logger.Warn("preprocessing failed", logger.Err(err))
	}

	// Step 3: Find main tex file
	a.updateStatus(types.PhaseExtracting, 25, "查找主 tex 文件...")
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
		a.updateStatusError("已取消")
		return nil, types.NewAppError(types.ErrInternal, "已取消", ctx.Err())
	}

	// Detect project size and adjust timeout if needed
	texFileCount := len(sourceInfo.AllTexFiles)
	isLargeProject := texFileCount > 20
	
	if isLargeProject {
		logger.Info("detected large project", 
			logger.Int("texFiles", texFileCount))
		a.updateStatus(types.PhaseCompiling, 28, 
			fmt.Sprintf("检测到大型项目（%d 个文件），编译可能需要较长时间...", texFileCount))
		
		// For very large projects (50+ files), use even longer timeout
		if texFileCount > 50 {
			a.compiler = compiler.NewLaTeXCompiler(a.compiler.GetCompiler(), a.workDir, 15*time.Minute)
			logger.Info("using extended timeout for very large project", 
				logger.String("timeout", "15 minutes"))
		}
	}

	// Step 4: Compile original document to PDF
	a.updateStatus(types.PhaseCompiling, 30, "编译原始文档...")
	logger.Info("compiling original document", logger.String("texPath", mainTexPath))
	originalOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_original")
	originalResult, err := a.compiler.Compile(mainTexPath, originalOutputDir)
	if err != nil {
		logger.Error("original document compilation failed", err)
		a.updateStatusError(fmt.Sprintf("原始文档编译失败: %v", err))
		// Save intermediate result on error
		if arxivID != "" {
			a.saveIntermediateResult(arxivID, title, input, sourceInfo, results.StatusError, err.Error(), "", "")
			// 记录原始编译错误
			a.recordError(arxivID, title, input, errors.StageOriginalCompile, err.Error())
		}
		return nil, err
	}
	if !originalResult.Success {
		err := types.NewAppErrorWithDetails(types.ErrCompile, "原始文档编译失败", originalResult.ErrorMsg, nil)
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
		a.updateStatusError("已取消")
		return nil, types.NewAppError(types.ErrInternal, "已取消", ctx.Err())
	}

	// Step 5: Read and translate tex content
	a.updateStatus(types.PhaseTranslating, 40, "读取 tex 文件...")
	logger.Debug("reading tex files for translation")

	a.updateStatus(types.PhaseTranslating, 42, "开始翻译文档...")

	// Translate main file and all input files
	translatedFiles, totalTokens, err := a.translateAllTexFiles(mainTexPath, sourceInfo.ExtractDir, func(current, total int, message string) {
		// Calculate progress: translation phase is from 42% to 58%
		progressRange := 16 // 58 - 42
		progress := 42 + (current * progressRange / total)
		a.updateStatus(types.PhaseTranslating, progress, message)
	})
	if err != nil {
		logger.Error("translation failed", err)
		a.updateStatusError(fmt.Sprintf("下载失败: %v", err))
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
	// mainTexFile is the relative path (e.g., "deep-representation-learning-book-main\book-main.tex")
	// which matches the keys in translatedFiles
	mainFileName := mainTexFile
	translatedContent := translatedFiles[mainFileName]

	// Check for cancellation
	if ctx.Err() != nil {
		logger.Warn("processing cancelled")
		a.updateStatusError("已取消")
		return nil, types.NewAppError(types.ErrInternal, "已取消", ctx.Err())
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
			
			// Save original content before fix attempt
			originalTranslatedContent := translatedContent
			originalLineCount := strings.Count(originalTranslatedContent, "\n") + 1
			
			fixedContent, err := a.validator.Fix(translatedContent, validationResult.Errors)
			if err != nil {
				// For syntax fix failures, log warning but continue with compilation
				// The compile-fix loop will attempt to fix errors during compilation
				logger.Warn("syntax fix failed, will rely on compile-fix loop", logger.Err(err))
			} else {
				// Check if the fixed content was severely truncated by LLM
				fixedLineCount := strings.Count(fixedContent, "\n") + 1
				truncationRatio := float64(fixedLineCount) / float64(originalLineCount)
				
				// If content was truncated by more than 50%, reject the fix
				if truncationRatio < 0.5 {
					logger.Warn("LLM syntax fix returned severely truncated content, rejecting fix",
						logger.Int("originalLines", originalLineCount),
						logger.Int("fixedLines", fixedLineCount),
						logger.Float64("ratio", truncationRatio))
					// Keep original translated content, don't use truncated fix
				} else {
					// Accept the fix
					translatedContent = fixedContent
					logger.Info("syntax errors fixed successfully",
						logger.Int("originalLines", originalLineCount),
						logger.Int("fixedLines", fixedLineCount))
					
					// IMPORTANT: Re-apply reference-based fixes after LLM syntax fix
					// The LLM may have re-introduced wrongly commented environments
					// Read original content for reference-based fixes
					originalMainPath := filepath.Join(sourceInfo.ExtractDir, mainFileName)
					if originalContent, readErr := os.ReadFile(originalMainPath); readErr == nil {
						translatedContent = translator.ApplyReferenceBasedFixes(translatedContent, string(originalContent))
						logger.Info("re-applied reference-based fixes after syntax fix")
					}
					
					// Update the main file in translatedFiles
					translatedFiles[mainFileName] = translatedContent
				}
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
		a.updateStatusError("已取消")
		return nil, types.NewAppError(types.ErrInternal, "已取消", ctx.Err())
	}

	// Step 7: Save all translated tex files
	a.updateStatus(types.PhaseValidating, 70, "保存翻译文件...")

	// Ensure ctex package is included for Chinese support in main file
	// Get the latest content from translatedFiles (it may have been updated by syntax fix)
	translatedContent = translatedFiles[mainFileName]
	translatedContent = ensureCtexPackage(translatedContent)
	translatedFiles[mainFileName] = translatedContent

	logger.Info("saving translated files",
		logger.Int("fileCount", len(translatedFiles)),
		logger.String("mainFileName", mainFileName))

	// Save all translated files, applying QuickFixWithReference to each
	for relPath, content := range translatedFiles {
		// Read original content for reference-based fixes
		originalPath := filepath.Join(sourceInfo.ExtractDir, relPath)
		originalContent, err := os.ReadFile(originalPath)
		var originalStr string
		if err == nil {
			originalStr = string(originalContent)
		} else {
			logger.Debug("could not read original file for reference fix",
				logger.String("relPath", relPath),
				logger.String("error", err.Error()))
		}

		// Apply QuickFixWithReference to fix common translation issues
		// This includes fixing wrongly commented environments like \begin{abstract}
		fixedContent, wasFixed := compiler.QuickFixWithReference(content, originalStr)
		if wasFixed {
			logger.Info("applied QuickFixWithReference to translated file",
				logger.String("relPath", relPath))
			content = fixedContent
			translatedFiles[relPath] = content
		}

		// Check if content has thebibliography before fix
		hasBibBefore := strings.Contains(content, `\begin{thebibliography}`)
		logger.Debug("before fixDuplicateThebibliographyInPreamble",
			logger.String("relPath", relPath),
			logger.Bool("hasThebibliography", hasBibBefore))

		// Fix duplicate thebibliography in preamble (before \begin{document})
		// This can happen when LLM incorrectly inserts .bbl content during translation
		content = fixDuplicateThebibliographyInPreamble(content)
		translatedFiles[relPath] = content

		// Check if content has thebibliography after fix
		hasBibAfter := strings.Contains(content, `\begin{thebibliography}`)
		logger.Debug("after fixDuplicateThebibliographyInPreamble",
			logger.String("relPath", relPath),
			logger.Bool("hasThebibliography", hasBibAfter))

		// IMPORTANT: Fix incomplete tabular column specs AFTER all other fixes
		// This must be done last because QuickFix might remove the closing braces we add
		content = translator.FixIncompleteTabularColumnSpec(content)
		translatedFiles[relPath] = content

		// CRITICAL: Fix split comment lines in preamble AFTER all other fixes
		// This handles cases where LLM splits "% comment" into two lines:
		//   Line N:   %
		//   Line N+1: comment text (without %)
		// This must be done last because other fixes might re-introduce the problem
		content = fixSplitCommentLinesInPreamble(content)
		translatedFiles[relPath] = content

		// CRITICAL: Fix merged comment lines in preamble
		// This handles cases where LLM merges comment text with the next line:
		//   Original: "% comment text\n\usepackage{pkg}"
		//   Broken:   "% comment text\usepackage{pkg}" (on same line)
		// This must be done after fixSplitCommentLinesInPreamble
		content = fixMergedCommentLinesInPreamble(content, originalStr)
		translatedFiles[relPath] = content

		// Add Chinese font support for LuaLaTeX compilation
		// This is necessary because translated documents contain Chinese characters
		content = addChineseFontSupport(content)
		translatedFiles[relPath] = content

		var savePath string
		if relPath == mainFileName {
			// Main file gets "translated_" prefix (only to filename, not directory)
			dir := filepath.Dir(relPath)
			base := filepath.Base(relPath)
			translatedFileName := "translated_" + base
			if dir == "." {
				savePath = filepath.Join(sourceInfo.ExtractDir, translatedFileName)
			} else {
				savePath = filepath.Join(sourceInfo.ExtractDir, dir, translatedFileName)
			}
		} else {
			// Input files are saved in place (overwrite original)
			savePath = filepath.Join(sourceInfo.ExtractDir, relPath)
		}

		logger.Info("saving translated file",
			logger.String("relPath", relPath),
			logger.String("savePath", savePath),
			logger.Int("contentLength", len(content)),
			logger.Bool("isMainFile", relPath == mainFileName))

		// Ensure parent directory exists
		saveDir := filepath.Dir(savePath)
		if err := os.MkdirAll(saveDir, 0755); err != nil {
			logger.Error("failed to create directory for translated file", err, logger.String("dir", saveDir))
			a.updateStatusError(fmt.Sprintf("创建目录失败: %v", err))
			return nil, types.NewAppError(types.ErrInternal, "创建目录失败", err)
		}

		err = os.WriteFile(savePath, []byte(content), 0644)
		if err != nil {
			logger.Error("failed to save translated file", err, logger.String("path", savePath))
			a.updateStatusError(fmt.Sprintf("保存翻译文件失败: %v", err))
			return nil, types.NewAppError(types.ErrInternal, "保存翻译文件失败", err)
		}
	}

	// Update translatedContent with the fixed version
	translatedContent = translatedFiles[mainFileName]

	// Construct translated tex path (add "translated_" prefix to filename only)
	mainTexDir := filepath.Dir(mainTexFile)
	mainTexBase := filepath.Base(mainTexFile)
	translatedFileName := "translated_" + mainTexBase
	var translatedTexPath string
	if mainTexDir == "." {
		translatedTexPath = filepath.Join(sourceInfo.ExtractDir, translatedFileName)
	} else {
		translatedTexPath = filepath.Join(sourceInfo.ExtractDir, mainTexDir, translatedFileName)
	}
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

		// Construct translated tex file path (add "translated_" prefix to filename only)
		mainTexDir := filepath.Dir(mainTexFile)
		mainTexBase := filepath.Base(mainTexFile)
		translatedMainTexFile := filepath.Join(mainTexDir, "translated_"+mainTexBase)
		if mainTexDir == "." {
			translatedMainTexFile = "translated_" + mainTexBase
		}

		// Run hierarchical fix with progress callback
		fixResult, fixErr := fixer.HierarchicalFixCompilationErrors(
			sourceInfo.ExtractDir,
			translatedMainTexFile,
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

	// Step 9.5: Check page count difference (suspicious error detection)
	a.updateStatus(types.PhaseCompiling, 98, "检查页数差异...")
	pageCountResult := a.checkPageCountDifference(originalResult.PDFPath, translatedResult.PDFPath)
	if pageCountResult != nil && pageCountResult.IsSuspicious {
		// 记录可疑错误但不阻止流程
		errorMsg := pdf.FormatPageCountError(pageCountResult)
		logger.Warn("suspicious page count difference detected",
			logger.Int("originalPages", pageCountResult.OriginalPages),
			logger.Int("translatedPages", pageCountResult.TranslatedPages),
			logger.Float64("diffPercent", pageCountResult.DiffPercent*100))
		if arxivID != "" {
			a.recordError(arxivID, title, input, errors.StagePageCountMismatch, errorMsg)
		}
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
		// Add missing closing braces BEFORE \end{document} if it exists
		missingBraces := strings.Repeat("}", openBraces-closeBraces)
		endDocIdx := strings.LastIndex(fixed, "\\end{document}")
		if endDocIdx != -1 {
			// Insert before \end{document}
			fixed = fixed[:endDocIdx] + missingBraces + "\n" + fixed[endDocIdx:]
		} else {
			// No \end{document}, append at end
			fixed += missingBraces
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
		a.updateStatusError("已取消")
		logger.Info("process cancelled successfully")
		return nil
	}
	logger.Warn("no process to cancel")
	return types.NewAppError(types.ErrInternal, "没有正在进行的处理", nil)
}

// GetPaperCategories returns all available paper categories for the frontend.
func (a *App) GetPaperCategories() []types.PaperCategory {
	return types.GetPaperCategories()
}

// GetSettings returns the current application settings for the frontend.
// This method is exposed to the frontend via Wails bindings.
func (a *App) GetSettings() *types.Config {
	logger.Debug("getting settings for frontend")
	if a.config == nil {
		return &types.Config{
			OpenAIAPIKey:       "",
			OpenAIBaseURL:      "https://api.openai.com/v1",
			OpenAIModel:        "gpt-4",
			ContextWindow:      8192,
			DefaultCompiler:    "pdflatex",
			WorkDirectory:      "",
			Concurrency:        3,
			GitHubToken:        "",
			GitHubOwner:        "",
			GitHubRepo:         "",
			LibraryPageSize:    20,
			SharePromptEnabled: true,
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
		OpenAIAPIKey:       maskedKey,
		OpenAIBaseURL:      cfg.OpenAIBaseURL,
		OpenAIModel:        cfg.OpenAIModel,
		ContextWindow:      cfg.ContextWindow,
		DefaultCompiler:    cfg.DefaultCompiler,
		WorkDirectory:      cfg.WorkDirectory,
		Concurrency:        cfg.Concurrency,
		GitHubToken:        maskedGitHubToken,
		GitHubOwner:        cfg.GitHubOwner,
		GitHubRepo:         cfg.GitHubRepo,
		LibraryPageSize:    cfg.LibraryPageSize,
		SharePromptEnabled: cfg.SharePromptEnabled,
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
func (a *App) SaveSettings(apiKey, baseURL, model string, contextWindow int, compiler, workDir string, concurrency int, githubToken, githubOwner, githubRepo string, libraryPageSize int, sharePromptEnabled bool) error {
	logger.Info("saving settings from frontend",
		logger.String("baseURL", baseURL),
		logger.String("model", model),
		logger.Int("concurrency", concurrency),
		logger.Int("libraryPageSize", libraryPageSize),
		logger.Bool("sharePromptEnabled", sharePromptEnabled),
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
	if err := a.config.UpdateConfig(apiKey, baseURL, model, contextWindow, compiler, workDir, concurrency, libraryPageSize, sharePromptEnabled); err != nil {
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

// GetInputHistory returns the input history list.
func (a *App) GetInputHistory() []types.InputHistoryItem {
	if a.config == nil {
		return []types.InputHistoryItem{}
	}
	return a.config.GetInputHistory()
}

// AddInputHistory adds an input to the history list.
func (a *App) AddInputHistory(input string, inputType string) {
	if a.config == nil {
		return
	}
	a.config.AddInputHistory(input, inputType)
}

// RemoveInputHistory removes a specific input from history.
func (a *App) RemoveInputHistory(input string) {
	if a.config == nil {
		return
	}
	a.config.RemoveInputHistory(input)
}

// ClearInputHistory clears all input history.
func (a *App) ClearInputHistory() {
	if a.config == nil {
		return
	}
	a.config.ClearInputHistory()
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

	// Emit events to frontend with arXiv ID
	if info.OriginalPDF != "" {
		a.safeEmit(EventOriginalPDFReady, map[string]interface{}{
			"pdfPath": info.OriginalPDF,
			"arxivId": arxivID,
		})
	}
	if info.TranslatedPDF != "" {
		a.safeEmit(EventTranslatedPDFReady, map[string]interface{}{
			"pdfPath": info.TranslatedPDF,
			"arxivId": arxivID,
		})
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

// SaveTranslatedPDF saves the translated PDF to a user-selected location.
// This method is exposed to the frontend via Wails bindings.
func (a *App) SaveTranslatedPDF(sourcePath string) (string, error) {
	logger.Debug("SaveTranslatedPDF called", logger.String("sourcePath", sourcePath))

	if sourcePath == "" {
		return "", types.NewAppError(types.ErrInvalidInput, "源文件路径为空", nil)
	}

	// Check if source file exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return "", types.NewAppError(types.ErrFileNotFound, "源文件不存在", err)
	}

	// Generate default filename
	defaultFilename := filepath.Base(sourcePath)

	// Open save dialog
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存翻译后的 PDF",
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

	// Copy the file
	srcData, err := os.ReadFile(sourcePath)
	if err != nil {
		logger.Error("failed to read source PDF", err)
		return "", types.NewAppError(types.ErrFileNotFound, "读取源 PDF 失败", err)
	}

	if err := os.WriteFile(savePath, srcData, 0644); err != nil {
		logger.Error("failed to write PDF", err)
		return "", types.NewAppError(types.ErrInternal, "保存 PDF 失败", err)
	}

	logger.Info("Translated PDF saved", logger.String("path", savePath))
	return savePath, nil
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

// addChineseFontSupport adds Chinese font support to the preamble for LuaLaTeX compilation.
// This is necessary because translated documents contain Chinese characters that require
// proper font configuration.
func addChineseFontSupport(content string) string {
	// Find \begin{document} position
	beginDocIdx := strings.Index(content, `\begin{document}`)
	if beginDocIdx == -1 {
		logger.Debug("addChineseFontSupport: no \\begin{document} found")
		return content
	}

	preamble := content[:beginDocIdx]
	body := content[beginDocIdx:]

	// Check if Chinese font support is already present
	if strings.Contains(preamble, `luatexja-fontspec`) || strings.Contains(preamble, `\setCJKmainfont`) {
		logger.Debug("addChineseFontSupport: Chinese font support already present")
		return content
	}

	// Check if fontspec is already loaded
	hasFontspec := strings.Contains(preamble, `\usepackage{fontspec}`)

	// Chinese font support packages
	chineseFontSupport := `
% Chinese font support for LuaLaTeX (auto-added by translator)
`
	if !hasFontspec {
		chineseFontSupport += `\usepackage{fontspec}
`
	}
	chineseFontSupport += `\usepackage{luatexja-fontspec}
\setmainjfont{SimSun}[BoldFont=SimHei]
\setsansjfont{SimHei}
`

	// Find a good insertion point - after \documentclass or after geometry package
	insertIdx := -1

	// Try to insert after geometry package
	geometryIdx := strings.Index(preamble, `\usepackage`)
	if geometryIdx != -1 {
		// Find the end of the first \usepackage line
		lineEnd := strings.Index(preamble[geometryIdx:], "\n")
		if lineEnd != -1 {
			insertIdx = geometryIdx + lineEnd + 1
		}
	}

	// If no good insertion point found, insert right after \documentclass line
	if insertIdx == -1 {
		docclassIdx := strings.Index(preamble, `\documentclass`)
		if docclassIdx != -1 {
			lineEnd := strings.Index(preamble[docclassIdx:], "\n")
			if lineEnd != -1 {
				insertIdx = docclassIdx + lineEnd + 1
			}
		}
	}

	if insertIdx == -1 {
		// Fallback: insert at the beginning of preamble
		insertIdx = 0
	}

	newPreamble := preamble[:insertIdx] + chineseFontSupport + preamble[insertIdx:]
	logger.Info("added Chinese font support to preamble")

	return newPreamble + body
}

// fixDuplicateThebibliographyInPreamble removes thebibliography environment from preamble.
// The thebibliography environment should NEVER be in the preamble (before \begin{document}).
// This can happen when LLM incorrectly inserts .bbl content during translation.
func fixDuplicateThebibliographyInPreamble(content string) string {
	// Find \begin{document} position
	beginDocIdx := strings.Index(content, `\begin{document}`)
	if beginDocIdx == -1 {
		logger.Debug("fixDuplicateThebibliographyInPreamble: no \\begin{document} found")
		return content
	}

	preamble := content[:beginDocIdx]
	body := content[beginDocIdx:]

	// Check if thebibliography exists in preamble
	preambleHasBib := strings.Contains(preamble, `\begin{thebibliography}`)

	logger.Debug("fixDuplicateThebibliographyInPreamble: checking for thebibliography in preamble",
		logger.Bool("preambleHasBib", preambleHasBib))

	if !preambleHasBib {
		// No thebibliography in preamble, return as is
		return content
	}

	logger.Info("fixDuplicateThebibliographyInPreamble: found thebibliography in preamble, removing")

	// Remove thebibliography environment from preamble
	// Find the start and end of the thebibliography environment in preamble
	bibStartIdx := strings.Index(preamble, `\begin{thebibliography}`)
	if bibStartIdx == -1 {
		return content
	}

	bibEndIdx := strings.Index(preamble[bibStartIdx:], `\end{thebibliography}`)
	if bibEndIdx == -1 {
		logger.Warn("fixDuplicateThebibliographyInPreamble: no \\end{thebibliography} found in preamble")
		return content
	}
	bibEndIdx += bibStartIdx + len(`\end{thebibliography}`)

	// Remove the thebibliography environment from preamble
	// Also remove any trailing newlines
	for bibEndIdx < len(preamble) && (preamble[bibEndIdx] == '\n' || preamble[bibEndIdx] == '\r') {
		bibEndIdx++
	}

	newPreamble := preamble[:bibStartIdx] + preamble[bibEndIdx:]
	logger.Info("removed thebibliography from preamble",
		logger.Int("removedBytes", bibEndIdx-bibStartIdx))

	return newPreamble + body
}

// fixSplitCommentLinesInPreamble fixes cases where LLM translation incorrectly splits
// comment lines in the preamble, putting "%" on one line and the comment text on the next.
// This is critical because uncommented text in the preamble causes "Missing \begin{document}" errors.
//
// Pattern detected:
//
//	Line N:   % (or "% ")
//	Line N+1: comment text (without %)
//
// This function merges such split lines or comments the orphaned text.
func fixSplitCommentLinesInPreamble(content string) string {
	// Find \begin{document}
	beginDocIdx := strings.Index(content, `\begin{document}`)
	if beginDocIdx == -1 {
		return content
	}

	preamble := content[:beginDocIdx]
	body := content[beginDocIdx:]

	lines := strings.Split(preamble, "\n")
	fixed := false

	// Common comment indicators that suggest a line should be commented
	commentIndicators := []string{
		"recommended", "optional", "packages", "figures", "typesetting",
		"hyperref", "hyperlinks", "resulting", "build breaks",
		"comment out", "following", "attempt", "algorithmic",
		"initial blind", "submitted", "review", "preprint",
		"accepted", "camera-ready", "submission", "setting:",
		"above.", "better", "work together", "for the",
		"use the", "instead use", "if accepted", "for preprint",
		"spans a page", "please comment", "use the following",
		"with \\usepackage", "nohyperref",
	}

	// Process lines to fix split comments
	for i := 0; i < len(lines)-1; i++ {
		trimmed := strings.TrimSpace(lines[i])

		// Check if this line is just "%" or "% " (isolated percent sign)
		if trimmed == "%" || trimmed == "% " {
			// Check the next line
			nextLine := lines[i+1]
			nextTrimmed := strings.TrimSpace(nextLine)

			// Skip if next line is empty or already commented
			if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, "%") {
				continue
			}

			// Check if next line looks like comment text
			lowerNext := strings.ToLower(nextTrimmed)
			shouldComment := false

			for _, indicator := range commentIndicators {
				if strings.Contains(lowerNext, indicator) {
					shouldComment = true
					break
				}
			}

			// Special case: line starts with a LaTeX command but contains comment-like text
			// e.g., "\usepackage{icml2026} with \usepackage[nohyperref]{icml2026} above."
			// This should be fully commented
			if strings.HasPrefix(nextTrimmed, "\\") {
				// Check if it contains comment indicators
				for _, indicator := range commentIndicators {
					if strings.Contains(lowerNext, indicator) {
						shouldComment = true
						break
					}
				}
			}

			// Also check if the line doesn't look like valid LaTeX
			// (no backslash commands, no braces at start)
			if !shouldComment && !strings.HasPrefix(nextTrimmed, "\\") {
				if !strings.ContainsAny(nextTrimmed[:min(10, len(nextTrimmed))], "\\{}$") {
					// Line starts with regular text - likely a comment
					if len(nextTrimmed) > 5 {
						shouldComment = true
					}
				}
			}

			if shouldComment {
				// Comment the next line
				leadingWhitespace := nextLine[:len(nextLine)-len(strings.TrimLeft(nextLine, " \t"))]
				lines[i+1] = leadingWhitespace + "% " + nextTrimmed
				fixed = true
				logger.Info("fixSplitCommentLinesInPreamble: commented orphaned text",
					logger.Int("lineNum", i+1),
					logger.String("text", nextTrimmed[:min(50, len(nextTrimmed))]))
			}
		}
	}

	if fixed {
		return strings.Join(lines, "\n") + body
	}

	return content
}

// fixMergedCommentLinesInPreamble fixes cases where LLM translation incorrectly merges
// comment text with the next line in the preamble.
// This is critical because it can cause LaTeX commands to be commented out or syntax errors.
//
// Pattern detected:
//
//	Original: "% comment text\n\usepackage{pkg}"
//	Broken:   "% comment text\usepackage{pkg}" (merged on same line)
//
// This function splits such merged lines back into separate lines by comparing with original.
func fixMergedCommentLinesInPreamble(content, original string) string {
	if original == "" {
		return content
	}

	// Find \begin{document}
	beginDocIdx := strings.Index(content, `\begin{document}`)
	if beginDocIdx == -1 {
		return content
	}

	origBeginDocIdx := strings.Index(original, `\begin{document}`)
	if origBeginDocIdx == -1 {
		return content
	}

	preamble := content[:beginDocIdx]
	body := content[beginDocIdx:]
	origPreamble := original[:origBeginDocIdx]

	lines := strings.Split(preamble, "\n")
	origLines := strings.Split(origPreamble, "\n")
	fixed := false

	// Build a set of lines from original that are pure comment lines (not commenting out code)
	// A pure comment line is one where the text after % doesn't start with a LaTeX command
	origPureCommentLines := make(map[string]bool)
	for _, line := range origLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%") && !strings.HasPrefix(trimmed, "%%") {
			// Check if this is a pure comment (not commenting out a command)
			afterPercent := strings.TrimSpace(strings.TrimPrefix(trimmed, "%"))
			if afterPercent != "" && !strings.HasPrefix(afterPercent, "\\") {
				// This is a pure comment line (text, not a commented-out command)
				origPureCommentLines[trimmed] = true
			}
		}
	}

	// Build a set of uncommented lines from original (lines that should NOT be commented)
	origUncommentedLines := make(map[string]bool)
	for _, line := range origLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "%") {
			origUncommentedLines[trimmed] = true
		}
	}

	// Now check each line in the translated preamble
	var newLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		leadingWhitespace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

		// Case 1: Check for non-comment lines that have comment text merged with a command
		// Pattern: "above.\usepackage{...}" or "following:\newcommand{...}"
		// These are cases where comment continuation text lost its % and got merged with the next line
		if !strings.HasPrefix(trimmed, "%") && !strings.HasPrefix(trimmed, "\\") && len(trimmed) > 5 {
			// Look for text followed by a LaTeX command
			textCmdPattern := regexp.MustCompile(`^([^\\]+)(\\[a-zA-Z]+.*)$`)
			if matches := textCmdPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
				textPart := strings.TrimSpace(matches[1])
				commandPart := strings.TrimSpace(matches[2])

				// Check if the text part looks like comment continuation
				// (ends with punctuation or common comment words)
				lowerText := strings.ToLower(textPart)
				isCommentContinuation := false

				// Check for common comment endings
				commentEndings := []string{":", ".", ",", "above", "below", "following", "use", "instead"}
				for _, ending := range commentEndings {
					if strings.HasSuffix(lowerText, ending) {
						isCommentContinuation = true
						break
					}
				}

				// Also check if the command part exists as an uncommented line in original
				commandExistsInOrig := false
				for origLine := range origUncommentedLines {
					if strings.HasPrefix(origLine, commandPart) || strings.HasPrefix(commandPart, origLine) {
						commandExistsInOrig = true
						break
					}
				}

				if isCommentContinuation && commandExistsInOrig {
					// Split: comment the text part, keep command uncommented
					newLines = append(newLines, leadingWhitespace+"% "+textPart)
					newLines = append(newLines, leadingWhitespace+commandPart)
					fixed = true
					logger.Info("fixMergedCommentLinesInPreamble: split and commented merged line",
						logger.String("text", textPart),
						logger.String("command", commandPart))
					continue
				}
			}
		}

		// Case 2: Check for comment lines where comment text is merged with a command
		// Pattern: "% comment text\usepackage{...}"
		// But NOT: "% \usepackage{...}" (this is intentionally commented out)
		if strings.HasPrefix(trimmed, "%") && !strings.HasPrefix(trimmed, "%%") {
			afterPercent := strings.TrimSpace(strings.TrimPrefix(trimmed, "%"))

			// Skip if this is a commented-out command (starts with \)
			if strings.HasPrefix(afterPercent, "\\") {
				newLines = append(newLines, line)
				continue
			}

			// Look for pattern: "text \command" or "text\command"
			// The text part should have at least 3 characters of actual text
			mergedPattern := regexp.MustCompile(`^(.{3,}?)(\\[a-zA-Z]+.*)$`)
			if matches := mergedPattern.FindStringSubmatch(afterPercent); len(matches) == 3 {
				textPart := strings.TrimSpace(matches[1])
				commandPart := strings.TrimSpace(matches[2])

				// Verify this is a real merge by checking:
				// 1. The text part looks like comment text (not just whitespace or symbols)
				// 2. The command part exists as an uncommented line in original
				hasLetters := regexp.MustCompile(`[a-zA-Z]{2,}`).MatchString(textPart)
				commandExistsInOrig := false
				for origLine := range origUncommentedLines {
					if strings.HasPrefix(origLine, commandPart) || strings.HasPrefix(commandPart, origLine) {
						commandExistsInOrig = true
						break
					}
				}

				if hasLetters && commandExistsInOrig {
					// Split the line: keep comment part as comment, uncomment the command
					newLines = append(newLines, leadingWhitespace+"% "+textPart)
					newLines = append(newLines, leadingWhitespace+commandPart)
					fixed = true
					logger.Info("fixMergedCommentLinesInPreamble: split merged comment line",
						logger.String("comment", textPart),
						logger.String("command", commandPart))
					continue
				}
			}
		}

		newLines = append(newLines, line)
	}

	if fixed {
		return strings.Join(newLines, "\n") + body
	}

	return content
}

// ensureCtexPackage ensures the ctex package is included in the LaTeX document for Chinese support.
// It adds \usepackage{ctex} after \documentclass if not already present.
// It also fixes microtype compatibility issues with XeLaTeX and removes conflicting CJK packages.
func ensureCtexPackage(content string) string {
	// Check if ctex is already included (must be uncommented)
	// We need to check line by line to avoid matching commented lines like "% \usepackage{ctex}"
	hasUncommentedCtex := false
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip commented lines
		if strings.HasPrefix(trimmed, "%") {
			continue
		}
		// Check for ctex package (with or without options)
		if strings.Contains(line, "\\usepackage{ctex}") || 
		   (strings.Contains(line, "\\usepackage[") && strings.Contains(line, "ctex}")) {
			hasUncommentedCtex = true
			break
		}
	}
	
	if hasUncommentedCtex {
		logger.Debug("ctex package already present (uncommented)")
	} else {
		// Find the first uncommented \documentclass line and add \usepackage{ctex} after it
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

	// Remove CJKutf8 package which conflicts with ctex/xeCJK
	// When using XeLaTeX with ctex, xeCJK is used instead and CJKutf8 is incompatible
	if strings.Contains(content, "\\usepackage{CJKutf8}") {
		content = strings.Replace(content, "\\usepackage{CJKutf8}", "% \\usepackage{CJKutf8} % Commented out - using ctex instead", 1)
		logger.Info("commented out CJKutf8 package (conflicts with ctex)")
	}

	// Remove CJK* environment which is not defined when using xeCJK
	// Pattern: \begin{CJK*}{...}{...} ... \end{CJK*}
	cjkBeginPattern := regexp.MustCompile(`\\begin\{CJK\*\}\{[^}]*\}\{[^}]*\}\s*\n?`)
	if cjkBeginPattern.MatchString(content) {
		content = cjkBeginPattern.ReplaceAllString(content, "% CJK* environment removed - using ctex instead\n")
		logger.Info("removed \\begin{CJK*} (conflicts with ctex)")
	}

	cjkEndPattern := regexp.MustCompile(`\\end\{CJK\*\}\s*\n?`)
	if cjkEndPattern.MatchString(content) {
		content = cjkEndPattern.ReplaceAllString(content, "% \\end{CJK*} removed\n")
		logger.Info("removed \\end{CJK*} (conflicts with ctex)")
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

	// Fix nested tabular structures that were split across multiple lines
	content = fixNestedTabularStructure(content)

	return content
}

// fixNestedTabularStructure fixes nested tabular structures that were incorrectly split across multiple lines.
// This happens when the translator breaks \begin{tabular}...\end{tabular} into multiple lines,
// which causes LaTeX compilation errors like "Missing \cr inserted".
func fixNestedTabularStructure(content string) string {
	// Pattern to match nested tabular that should be on single line
	// e.g., \begin{tabular}[c]{@{}c@{}}...\end{tabular}}
	pattern := regexp.MustCompile(`(\\begin\{tabular\}\[[^\]]+\]\{[^}]+\})([\s\S]*?)(\\end\{tabular\}\})`)
	
	fixed := false
	content = pattern.ReplaceAllStringFunc(content, func(match string) string {
		// Check if the match spans multiple lines
		if strings.Contains(match, "\n") {
			// Merge into single line, replacing newlines with spaces
			result := strings.ReplaceAll(match, "\n", " ")
			result = strings.ReplaceAll(result, "\r", "")
			// Clean up multiple spaces
			for strings.Contains(result, "  ") {
				result = strings.ReplaceAll(result, "  ", " ")
			}
			fixed = true
			return result
		}
		return match
	})
	
	if fixed {
		logger.Debug("fixed nested tabular structures that were split across lines")
	}
	
	// Fix standalone } on a line after \end{tabular} or \end{tabular}}
	// Pattern: \end{tabular}\n}\n or \end{tabular}}\n}\n -> remove the extra }
	extraBracePattern := regexp.MustCompile(`(\\end\{tabular\}\}?)\s*\n\}\s*\n`)
	for extraBracePattern.MatchString(content) {
		content = extraBracePattern.ReplaceAllString(content, "$1\n")
		logger.Debug("removed extra closing braces after tabular")
	}
	
	// Fix \end{table without closing brace - this happens when LLM corrupts the structure
	// Pattern: \end{table followed by space or backslash (not })
	// e.g., \end{table \subsection -> \end{table} \subsection
	incompleteEndTablePattern := regexp.MustCompile(`\\end\{table([^}*])`)
	if incompleteEndTablePattern.MatchString(content) {
		content = incompleteEndTablePattern.ReplaceAllString(content, "\\end{table}$1")
		logger.Debug("fixed incomplete \\end{table} commands")
	}
	
	// Fix \end{tabular without closing brace
	incompleteEndTabularPattern := regexp.MustCompile(`\\end\{tabular([^}*])`)
	if incompleteEndTabularPattern.MatchString(content) {
		content = incompleteEndTabularPattern.ReplaceAllString(content, "\\end{tabular}$1")
		logger.Debug("fixed incomplete \\end{tabular} commands")
	}
	
	// Fix \multirow{ without proper closing - ensure nested tabular inside multirow is complete
	// Pattern: \multirow{...}{\begin{tabular}...\end{tabular} (missing final })
	// This is complex, so we use a simpler approach: ensure all \begin{tabular} have matching \end{tabular}}
	
	return content
}

// needsTranslation checks if a tex file contains translatable content.
// Files that only contain LaTeX command definitions or tables don't need translation.
func needsTranslation(content string) bool {
	// Count different types of content
	lines := strings.Split(content, "\n")
	
	commandDefCount := 0
	tableStructureCount := 0
	textContentCount := 0
	
	// Check if the file is primarily a table
	hasTableEnv := strings.Contains(content, "\\begin{table") || strings.Contains(content, "\\begin{tabular")
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "%") {
			continue
		}
		
		// Count command definitions (including the whole line)
		if strings.HasPrefix(trimmed, "\\newcommand") ||
			strings.HasPrefix(trimmed, "\\renewcommand") ||
			strings.HasPrefix(trimmed, "\\def") ||
			strings.HasPrefix(trimmed, "\\let") ||
			strings.HasPrefix(trimmed, "\\DeclareMathOperator") ||
			strings.HasPrefix(trimmed, "\\newlength") ||
			strings.HasPrefix(trimmed, "\\setlength") ||
			strings.HasPrefix(trimmed, "\\newcounter") ||
			strings.HasPrefix(trimmed, "\\setcounter") ||
			strings.HasPrefix(trimmed, "\\DeclareOption") ||
			strings.HasPrefix(trimmed, "\\ProcessOptions") ||
			strings.HasPrefix(trimmed, "\\RequirePackage") ||
			strings.HasPrefix(trimmed, "\\ProvidesPackage") ||
			strings.HasPrefix(trimmed, "\\ProvidesClass") {
			commandDefCount++
			continue
		}
		
		// Count table structure lines (tabular, toprule, midrule, etc.)
		if strings.Contains(trimmed, "\\begin{tabular") ||
			strings.Contains(trimmed, "\\end{tabular") ||
			strings.Contains(trimmed, "\\begin{table") ||
			strings.Contains(trimmed, "\\end{table") ||
			strings.Contains(trimmed, "\\toprule") ||
			strings.Contains(trimmed, "\\midrule") ||
			strings.Contains(trimmed, "\\bottomrule") ||
			strings.Contains(trimmed, "\\hline") ||
			strings.Contains(trimmed, "\\cline") ||
			strings.Contains(trimmed, "\\multicolumn") ||
			strings.Contains(trimmed, "\\resizebox") ||
			strings.Contains(trimmed, "\\centering") ||
			strings.Contains(trimmed, "\\caption") ||
			strings.Contains(trimmed, "\\label") ||
			strings.HasPrefix(trimmed, "&") ||
			strings.HasSuffix(trimmed, "\\\\") {
			tableStructureCount++
			continue
		}
		
		// Skip lines that are mostly LaTeX commands or symbols
		if strings.HasPrefix(trimmed, "\\") || 
			strings.HasPrefix(trimmed, "{") || 
			strings.HasPrefix(trimmed, "}") || 
			strings.HasPrefix(trimmed, "&") ||
			strings.HasPrefix(trimmed, "$") {
			continue
		}
		
		// Count lines that look like actual prose text content
		// Must have multiple words and not be just numbers/symbols
		wordCount := len(regexp.MustCompile(`[a-zA-Z]{3,}`).FindAllString(trimmed, -1))
		if wordCount >= 3 {
			textContentCount++
		}
	}
	
	// If the file is primarily a table with very little prose, skip translation
	if hasTableEnv && textContentCount < 5 {
		return false
	}
	
	// If the file is mostly command definitions with very little text, skip translation
	totalSignificant := commandDefCount + tableStructureCount + textContentCount
	if totalSignificant > 0 {
		nonTextRatio := float64(commandDefCount+tableStructureCount) / float64(totalSignificant)
		// If more than 80% of significant lines are non-text (commands or table structure), skip translation
		if nonTextRatio > 0.8 && textContentCount < 5 {
			return false
		}
	}
	
	// Also skip if there's very little text content overall
	if textContentCount < 3 {
		return false
	}
	
	return true
}

// findInputFiles finds all tex files referenced by \input or \include commands in the given content.
// It recursively scans all referenced files to find nested \input commands.
// It returns a list of relative file paths in the order they should be processed.
// baseDir should be the directory containing the main tex file (not the extraction root)
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
		return nil, 0, types.NewAppError(types.ErrFileNotFound, "读取主 tex 文件失败", err)
	}

	// Find all input files
	inputFiles := findInputFiles(string(mainContent), filepath.Dir(mainTexPath))
	logger.Info("found input files", logger.Int("count", len(inputFiles)))

	// Get the relative path of main file from baseDir
	mainFileRel, err := filepath.Rel(baseDir, mainTexPath)
	if err != nil {
		// If we can't get relative path, use the basename
		mainFileRel = filepath.Base(mainTexPath)
		logger.Warn("failed to get relative path for main file, using basename",
			logger.String("mainTexPath", mainTexPath),
			logger.String("baseDir", baseDir),
			logger.String("mainFileRel", mainFileRel))
	}

	// Collect all files to translate (main file + input files)
	allFiles := []string{mainFileRel}
	allFiles = append(allFiles, inputFiles...)

	totalFiles := len(allFiles)
	currentFile := 0

	// Translate each file
	for _, relPath := range allFiles {
		currentFile++
		
		// Construct full path - handle both relative paths from baseDir and from main file dir
		var fullPath string
		if filepath.IsAbs(relPath) {
			fullPath = relPath
		} else {
			// Try relative to baseDir first
			fullPath = filepath.Join(baseDir, relPath)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				// If not found, try relative to main file directory
				fullPath = filepath.Join(filepath.Dir(mainTexPath), relPath)
			}
		}

		logger.Info("processing file for translation",
			logger.String("relPath", relPath),
			logger.String("fullPath", fullPath),
			logger.Int("current", currentFile),
			logger.Int("total", totalFiles))

		content, err := os.ReadFile(fullPath)
		if err != nil {
			logger.Error("failed to read input file", err, 
				logger.String("file", relPath), 
				logger.String("fullPath", fullPath),
				logger.String("baseDir", baseDir),
				logger.String("mainTexPath", mainTexPath))
			// Don't skip - return error to fail fast
			return nil, 0, types.NewAppError(types.ErrFileNotFound, fmt.Sprintf("读取文件失败: %s", relPath), err)
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

		// Skip files that don't need translation (e.g., pure command definition files)
		if !needsTranslation(string(content)) {
			logger.Info("skipping file without translatable content", logger.String("file", relPath))
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

// CheckShareStatusWithCategory checks if files exist on GitHub for a specific arXiv ID
// This checks ALL files for the arXiv ID (compatible with old filename formats)
// categoryID is used to show the new filename format that will be used
func (a *App) CheckShareStatusWithCategory(categoryID string) (*ShareCheckResult, error) {
	logger.Debug("CheckShareStatusWithCategory called", logger.String("categoryID", categoryID))

	// Check if we have a result to share
	if a.lastResult == nil {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "没有可分享的翻译结果",
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

	// Extract arXiv ID
	arxivID := ""
	if a.lastResult.SourceID != "" {
		arxivID = results.ExtractArxivID(a.lastResult.SourceID)
		if arxivID == "" && strings.Contains(a.lastResult.SourceID, ".") && len(a.lastResult.SourceID) >= 9 {
			arxivID = a.lastResult.SourceID
		}
	}
	if arxivID == "" && a.lastResult.SourceInfo != nil {
		arxivID = results.ExtractArxivID(a.lastResult.SourceInfo.OriginalRef)
	}

	if arxivID == "" {
		return &ShareCheckResult{
			CanShare: false,
			Message:  "只能分享 arXiv 论文",
		}, nil
	}

	// Create uploader
	uploader := github.NewUploader(githubToken, owner, repo)

	// List all existing files for this arXiv ID (compatible with old filenames)
	existingFiles, err := uploader.ListFilesForArxivID(arxivID)
	if err != nil {
		logger.Warn("failed to list existing files", logger.Err(err))
	}

	// Check if Chinese and bilingual PDFs exist (any format)
	chineseExists := false
	bilingualExists := false
	var existingChineseName, existingBilingualName string
	for _, f := range existingFiles {
		if f.Type == "chinese" {
			chineseExists = true
			existingChineseName = f.Name
		} else if f.Type == "bilingual" {
			bilingualExists = true
			existingBilingualName = f.Name
		}
	}

	// Build new file paths with category
	safeID := strings.ReplaceAll(arxivID, "/", "_")
	var chinesePath, bilingualPath string
	if categoryID != "" {
		chinesePath = fmt.Sprintf("%s_%s_cn.pdf", safeID, categoryID)
		bilingualPath = fmt.Sprintf("%s_%s_bilingual.pdf", safeID, categoryID)
	} else {
		chinesePath = fmt.Sprintf("%s_cn.pdf", safeID)
		bilingualPath = fmt.Sprintf("%s_bilingual.pdf", safeID)
	}

	// Build message about existing files
	message := "可以分享"
	if chineseExists || bilingualExists {
		message = "已存在旧文件，上传后将被替换"
		if existingChineseName != "" {
			message += fmt.Sprintf(" (中文: %s)", existingChineseName)
		}
		if existingBilingualName != "" {
			message += fmt.Sprintf(" (双语: %s)", existingBilingualName)
		}
	}

	return &ShareCheckResult{
		CanShare:           true,
		ChinesePDFExists:   chineseExists,
		BilingualPDFExists: bilingualExists,
		ChinesePDFPath:     chinesePath,
		BilingualPDFPath:   bilingualPath,
		Message:            message,
	}, nil
}

// ShareToGitHub uploads the translated PDFs to GitHub
// categoryID: the category ID for the paper (e.g., "llm", "cv")
// uploadChinese: whether to upload Chinese PDF (will overwrite if exists)
// uploadBilingual: whether to upload bilingual PDF (will overwrite if exists)
// After uploading new files, old files for the same arXiv ID will be deleted
func (a *App) ShareToGitHub(categoryID string, uploadChinese, uploadBilingual bool) (*ShareResult, error) {
	logger.Info("ShareToGitHub called",
		logger.String("categoryID", categoryID),
		logger.Bool("uploadChinese", uploadChinese),
		logger.Bool("uploadBilingual", uploadBilingual))

	// Check if we have a result to share
	if a.lastResult == nil {
		return nil, types.NewAppError(types.ErrInvalidInput, "没有可分享的翻译结果", nil)
	}

	// Validate category ID
	if categoryID == "" {
		return nil, types.NewAppError(types.ErrInvalidInput, "请选择论文类别", nil)
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

	// List existing files for this arXiv ID before uploading
	existingFiles, err := uploader.ListFilesForArxivID(arxivID)
	if err != nil {
		logger.Warn("failed to list existing files", logger.Err(err))
		existingFiles = nil
	}

	// Track new file names to avoid deleting them
	newFileNames := make(map[string]bool)

	// Upload Chinese PDF if requested
	// File name format: arxivid_categoryid_cn.pdf
	if uploadChinese && a.lastResult.TranslatedPDFPath != "" {
		chinesePath := fmt.Sprintf("%s_%s_cn.pdf", safeID, categoryID)
		newFileNames[chinesePath] = true
		commitMsg := fmt.Sprintf("Add Chinese translation for %s [%s]", arxivID, categoryID)

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
	// File name format: arxivid_categoryid_bilingual.pdf
	if uploadBilingual && a.lastResult.OriginalPDFPath != "" && a.lastResult.TranslatedPDFPath != "" {
		bilingualPath := fmt.Sprintf("%s_%s_bilingual.pdf", safeID, categoryID)
		newFileNames[bilingualPath] = true
		commitMsg := fmt.Sprintf("Add bilingual PDF for %s [%s]", arxivID, categoryID)

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

	// Delete old files that are not the new files (clean up old format files)
	if uploadedCount > 0 && existingFiles != nil {
		for _, oldFile := range existingFiles {
			// Skip if this is one of the new files we just uploaded
			if newFileNames[oldFile.Name] {
				continue
			}

			// Delete old Chinese PDF if we uploaded a new one
			if uploadChinese && oldFile.Type == "chinese" {
				logger.Info("deleting old Chinese PDF", logger.String("file", oldFile.Name))
				commitMsg := fmt.Sprintf("Remove old Chinese PDF %s (replaced by new format)", oldFile.Name)
				if err := uploader.DeleteFile(oldFile.Path, oldFile.SHA, commitMsg); err != nil {
					logger.Warn("failed to delete old Chinese PDF", logger.String("file", oldFile.Name), logger.Err(err))
				}
			}

			// Delete old bilingual PDF if we uploaded a new one
			if uploadBilingual && oldFile.Type == "bilingual" {
				logger.Info("deleting old bilingual PDF", logger.String("file", oldFile.Name))
				commitMsg := fmt.Sprintf("Remove old bilingual PDF %s (replaced by new format)", oldFile.Name)
				if err := uploader.DeleteFile(oldFile.Path, oldFile.SHA, commitMsg); err != nil {
					logger.Warn("failed to delete old bilingual PDF", logger.String("file", oldFile.Name), logger.Err(err))
				}
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
// categoryID: the category ID for the paper (e.g., "llm", "cv")
// uploadChinese: whether to upload Chinese PDF (will overwrite if exists)
// uploadBilingual: whether to upload bilingual PDF (will overwrite if exists)
func (a *App) SharePaperToGitHub(arxivID string, categoryID string, uploadChinese, uploadBilingual bool) (*ShareResult, error) {
	logger.Info("SharePaperToGitHub called",
		logger.String("arxivID", arxivID),
		logger.String("categoryID", categoryID),
		logger.Bool("uploadChinese", uploadChinese),
		logger.Bool("uploadBilingual", uploadBilingual))

	if a.results == nil {
		return nil, types.NewAppError(types.ErrInternal, "结果管理器未初始化", nil)
	}

	// Validate category ID
	if categoryID == "" {
		return nil, types.NewAppError(types.ErrInvalidInput, "请选择论文类别", nil)
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
	// File name format: arxivid_categoryid_cn.pdf
	if uploadChinese && paper.TranslatedPDF != "" {
		chinesePath := fmt.Sprintf("%s_%s_cn.pdf", safeID, categoryID)
		commitMsg := fmt.Sprintf("Add Chinese translation for %s [%s]", arxivID, categoryID)

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
	// File name format: arxivid_categoryid_bilingual.pdf
	if uploadBilingual && paper.OriginalPDF != "" && paper.TranslatedPDF != "" {
		bilingualPath := fmt.Sprintf("%s_%s_bilingual.pdf", safeID, categoryID)
		commitMsg := fmt.Sprintf("Add bilingual PDF for %s [%s]", arxivID, categoryID)

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
	githubToken := ""

	if a.config != nil {
		cfg := a.config.GetConfig()
		if cfg.GitHubOwner != "" {
			owner = cfg.GitHubOwner
		}
		if cfg.GitHubRepo != "" {
			repo = cfg.GitHubRepo
		}
		githubToken = cfg.GitHubToken
	}

	logger.Info("using GitHub repository",
		logger.String("owner", owner),
		logger.String("repo", repo),
		logger.Bool("hasToken", githubToken != ""))

	// Create uploader with token for authenticated requests (higher rate limit)
	uploader := github.NewUploader(githubToken, owner, repo)

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

	// Get GitHub token for authenticated requests
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}

	// Search for the translation first
	uploader := github.NewUploader(githubToken, DefaultGitHubOwner, DefaultGitHubRepo)
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

// DownloadAndOpenResult represents the result of downloading and opening a paper
type DownloadAndOpenResult struct {
	Success       bool   `json:"success"`
	ChinesePath   string `json:"chinese_path,omitempty"`
	BilingualPath string `json:"bilingual_path,omitempty"`
	Message       string `json:"message,omitempty"`
}

// DownloadAndOpenGitHubTranslation downloads a translation to cache directory and optionally opens it
// fileType: "bilingual" or "chinese" - determines which file to open in external viewer (if openExternal is true)
// openExternal: if true, opens the file in external viewer; if false, only downloads and returns paths
func (a *App) DownloadAndOpenGitHubTranslation(arxivID, fileType string, openExternal bool) (*DownloadAndOpenResult, error) {
	logger.Info("downloading and opening GitHub translation",
		logger.String("arxivID", arxivID),
		logger.String("fileType", fileType),
		logger.Bool("openExternal", openExternal))

	// Normalize arXiv ID
	arxivID = strings.TrimPrefix(strings.ToLower(arxivID), "arxiv:")
	arxivID = strings.TrimSpace(arxivID)

	// Get cache directory
	cacheDir, err := a.getCacheDir()
	if err != nil {
		return nil, types.NewAppError(types.ErrInternal, "获取缓存目录失败: "+err.Error(), err)
	}

	// Create subdirectory for this paper
	paperCacheDir := filepath.Join(cacheDir, "github_papers", arxivID)
	if err := os.MkdirAll(paperCacheDir, 0755); err != nil {
		return nil, types.NewAppError(types.ErrInternal, "创建缓存目录失败: "+err.Error(), err)
	}

	// Get GitHub token for authenticated requests
	githubToken := ""
	if a.config != nil {
		cfg := a.config.GetConfig()
		githubToken = cfg.GitHubToken
	}

	// Search for the translation first
	uploader := github.NewUploader(githubToken, DefaultGitHubOwner, DefaultGitHubRepo)
	searchResult, err := uploader.SearchTranslation(arxivID)
	if err != nil {
		return nil, types.NewAppError(types.ErrAPICall, "搜索失败: "+err.Error(), err)
	}

	if !searchResult.Found {
		return nil, types.NewAppError(types.ErrFileNotFound, "GitHub 上未找到该论文的翻译", nil)
	}

	result := &DownloadAndOpenResult{Success: true}
	var fileToOpen string

	// Download both bilingual and chinese PDFs if available
	// This allows the viewer to display both files
	
	// Download bilingual PDF if available
	if searchResult.DownloadURLBi != "" && searchResult.BilingualPDFFilename != "" {
		bilingualPath := filepath.Join(paperCacheDir, searchResult.BilingualPDFFilename)
		
		// Check if file already exists
		if _, err := os.Stat(bilingualPath); err == nil {
			// File exists, skip download
			result.BilingualPath = bilingualPath
			logger.Info("bilingual PDF already exists in cache", logger.String("path", bilingualPath))
			
			// If user requested bilingual, this is the file to open
			if fileType == "bilingual" {
				fileToOpen = bilingualPath
			}
		} else {
			// File doesn't exist, download it
			// Update status
			a.updateStatus(types.PhaseDownloading, 20, "正在下载双语 PDF...")
			
			// Download
			if err := uploader.DownloadFile(searchResult.DownloadURLBi, bilingualPath); err != nil {
				logger.Error("download bilingual PDF failed", err, logger.String("url", searchResult.DownloadURLBi))
			} else {
				result.BilingualPath = bilingualPath
				logger.Info("downloaded bilingual PDF", logger.String("path", bilingualPath))
				
				// If user requested bilingual, this is the file to open
				if fileType == "bilingual" {
					fileToOpen = bilingualPath
				}
			}
		}
	}

	// Download chinese PDF if available
	if searchResult.DownloadURLCN != "" && searchResult.ChinesePDFFilename != "" {
		chinesePath := filepath.Join(paperCacheDir, searchResult.ChinesePDFFilename)
		
		// Check if file already exists
		if _, err := os.Stat(chinesePath); err == nil {
			// File exists, skip download
			result.ChinesePath = chinesePath
			logger.Info("chinese PDF already exists in cache", logger.String("path", chinesePath))
			
			// If user requested chinese, this is the file to open
			if fileType == "chinese" {
				fileToOpen = chinesePath
			}
		} else {
			// File doesn't exist, download it
			// Update status
			a.updateStatus(types.PhaseDownloading, 50, "正在下载中文 PDF...")
			
			// Download
			if err := uploader.DownloadFile(searchResult.DownloadURLCN, chinesePath); err != nil {
				logger.Error("download chinese PDF failed", err, logger.String("url", searchResult.DownloadURLCN))
			} else {
				result.ChinesePath = chinesePath
				logger.Info("downloaded chinese PDF", logger.String("path", chinesePath))
				
				// If user requested chinese, this is the file to open
				if fileType == "chinese" {
					fileToOpen = chinesePath
				}
			}
		}
	}

	// If no file was downloaded, return error
	if result.BilingualPath == "" && result.ChinesePath == "" {
		return nil, types.NewAppError(types.ErrFileNotFound, "未找到可下载的 PDF 文件", nil)
	}

	// Determine which file to open in external viewer
	// Priority: requested file type, then bilingual, then chinese
	if fileToOpen == "" {
		if result.BilingualPath != "" {
			fileToOpen = result.BilingualPath
		} else if result.ChinesePath != "" {
			fileToOpen = result.ChinesePath
		}
	}

	// Update status
	a.updateStatus(types.PhaseDownloading, 80, "正在打开文件...")

	// Open the file with system default application (only if openExternal is true)
	if openExternal && fileToOpen != "" {
		logger.Info("opening file in external viewer", logger.String("path", fileToOpen))
		if err := a.openFileWithDefaultApp(fileToOpen); err != nil {
			logger.Warn("failed to open file", logger.Err(err))
			result.Message = "文件已下载，但无法自动打开"
		} else {
			result.Message = "文件已下载并打开"
		}
	} else {
		result.Message = "文件已下载"
	}

	// Reset status
	a.updateStatus(types.PhaseIdle, 0, "")

	logger.Info("download and open completed",
		logger.String("bilingualPath", result.BilingualPath),
		logger.String("chinesePath", result.ChinesePath))
	return result, nil
}

// getCacheDir returns the cache directory path
func (a *App) getCacheDir() (string, error) {
	// Use user's cache directory
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	cacheDir := filepath.Join(userCacheDir, "latex-translator")
	return cacheDir, nil
}

// openFileWithDefaultApp opens a file with the system's default application
func (a *App) openFileWithDefaultApp(filePath string) error {
	var cmd *exec.Cmd

	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "linux":
		cmd = exec.Command("xdg-open", filePath)
	default:
		return fmt.Errorf("unsupported operating system: %s", goruntime.GOOS)
	}

	return cmd.Start()
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
	githubToken := ""

	if a.config != nil {
		cfg := a.config.GetConfig()
		if cfg.GitHubOwner != "" {
			owner = cfg.GitHubOwner
		}
		if cfg.GitHubRepo != "" {
			repo = cfg.GitHubRepo
		}
		githubToken = cfg.GitHubToken
	}

	logger.Info("using GitHub repository",
		logger.String("owner", owner),
		logger.String("repo", repo),
		logger.Bool("hasToken", githubToken != ""))

	// Create uploader with token for authenticated requests (higher rate limit)
	uploader := github.NewUploader(githubToken, owner, repo)

	// List recent translations
	papers, err := uploader.ListRecentTranslations(maxCount)
	if err != nil {
		logger.Error("failed to list GitHub translations", err)
		return nil, types.NewAppError(types.ErrAPICall, "获取列表失败: "+err.Error(), err)
	}

	logger.Info("listed GitHub translations", logger.Int("count", len(papers)))
	return papers, nil
}

// ListGitHubTranslationsByCategories lists translations filtered by category IDs
// categories: list of category IDs to filter by (e.g., ["llm", "cv", "nlp"])
// maxCount: maximum number of papers to return
// Returns papers whose filenames contain any of the specified category IDs
func (a *App) ListGitHubTranslationsByCategories(categories []string, maxCount int) ([]github.ArxivPaperInfo, error) {
	logger.Info("listing GitHub translations by categories",
		logger.Int("categoryCount", len(categories)),
		logger.Int("maxCount", maxCount))

	// Get GitHub repository settings from config
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo
	githubToken := ""

	if a.config != nil {
		cfg := a.config.GetConfig()
		if cfg.GitHubOwner != "" {
			owner = cfg.GitHubOwner
		}
		if cfg.GitHubRepo != "" {
			repo = cfg.GitHubRepo
		}
		githubToken = cfg.GitHubToken
	}

	// Create uploader with token for authenticated requests
	uploader := github.NewUploader(githubToken, owner, repo)

	// List all translations first
	allPapers, err := uploader.ListRecentTranslations(maxCount * 10) // Get more to filter
	if err != nil {
		logger.Error("failed to list GitHub translations", err)
		return nil, types.NewAppError(types.ErrAPICall, "获取列表失败: "+err.Error(), err)
	}

	// Filter by categories
	// Filename format: arxivid_categoryid_cn.pdf or arxivid_categoryid_bilingual.pdf
	filteredPapers := make([]github.ArxivPaperInfo, 0)
	categorySet := make(map[string]bool)
	for _, cat := range categories {
		categorySet[strings.ToLower(cat)] = true
	}

	for _, paper := range allPapers {
		// Extract category from Chinese PDF filename
		catID := extractCategoryFromFilename(paper.ChinesePDF)
		if catID == "" {
			// Try bilingual PDF
			catID = extractCategoryFromFilename(paper.BilingualPDF)
		}

		if catID != "" && categorySet[strings.ToLower(catID)] {
			filteredPapers = append(filteredPapers, paper)
		}
	}

	// Limit to maxCount
	if len(filteredPapers) > maxCount {
		filteredPapers = filteredPapers[:maxCount]
	}

	logger.Info("filtered GitHub translations by categories",
		logger.Int("totalCount", len(allPapers)),
		logger.Int("filteredCount", len(filteredPapers)))

	return filteredPapers, nil
}

// extractCategoryFromFilename extracts category ID from filename
// Format: arxivid_categoryid_cn.pdf or arxivid_categoryid_bilingual.pdf
func extractCategoryFromFilename(filename string) string {
	if filename == "" {
		return ""
	}

	// Match pattern: something_categoryid_cn.pdf or something_categoryid_bilingual.pdf
	// The category ID is between the last underscore before _cn or _bilingual
	filename = strings.ToLower(filename)

	var suffix string
	if strings.HasSuffix(filename, "_cn.pdf") {
		suffix = "_cn.pdf"
	} else if strings.HasSuffix(filename, "_bilingual.pdf") {
		suffix = "_bilingual.pdf"
	} else {
		return ""
	}

	// Remove suffix
	nameWithoutSuffix := strings.TrimSuffix(filename, suffix)

	// Find the last underscore
	lastUnderscore := strings.LastIndex(nameWithoutSuffix, "_")
	if lastUnderscore == -1 {
		return ""
	}

	// Extract category ID
	catID := nameWithoutSuffix[lastUnderscore+1:]
	return catID
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

// ReportErrorsToGitHub 上报错误列表到 GitHub Issue
// 将所有未上报的错误的 arXiv ID 和错误信息汇总到一个 Issue 中
func (a *App) ReportErrorsToGitHub() (*github.IssueCreateResult, error) {
	if a.errorMgr == nil {
		return nil, fmt.Errorf("error manager not initialized")
	}

	// 只获取未上报的错误
	errorList := a.errorMgr.ListUnreportedErrors()
	if len(errorList) == 0 {
		return nil, fmt.Errorf("no unreported errors to report")
	}

	// 获取 GitHub 配置
	token := ""
	owner := DefaultGitHubOwner
	repo := DefaultGitHubRepo

	if a.config != nil {
		cfg := a.config.GetConfig()
		token = cfg.GitHubToken
	}

	if token == "" {
		return nil, fmt.Errorf("GitHub token not configured, please set it in settings")
	}

	// 创建 GitHub uploader
	uploader := github.NewUploader(token, owner, repo)

	// 验证 token
	if err := uploader.ValidateToken(); err != nil {
		return nil, fmt.Errorf("GitHub token validation failed: %w", err)
	}

	// 构建 Issue 标题
	// 构建标题，包含 arXiv ID（最多5个）
	var titleBuilder strings.Builder
	titleBuilder.WriteString("翻译错误报告")
	
	// 添加 arXiv ID 到标题
	if len(errorList) > 0 {
		titleBuilder.WriteString(" - ")
		maxIDsInTitle := 5
		for i, record := range errorList {
			if i >= maxIDsInTitle {
				titleBuilder.WriteString("等")
				break
			}
			if i > 0 {
				titleBuilder.WriteString(", ")
			}
			titleBuilder.WriteString(record.ID)
		}
	}
	
	titleBuilder.WriteString(fmt.Sprintf(" (%d个, %s)", len(errorList), time.Now().Format("2006-01-02")))
	title := titleBuilder.String()

	// 构建 Issue 内容
	var bodyBuilder strings.Builder
	bodyBuilder.WriteString("## 翻译错误报告\n\n")
	bodyBuilder.WriteString(fmt.Sprintf("报告时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	bodyBuilder.WriteString(fmt.Sprintf("错误数量: %d\n\n", len(errorList)))
	
	// arXiv ID 列表（方便复制）
	bodyBuilder.WriteString("### arXiv ID 列表\n\n")
	bodyBuilder.WriteString("```\n")
	reportedIDs := make([]string, 0, len(errorList))
	for _, record := range errorList {
		bodyBuilder.WriteString(record.ID + "\n")
		reportedIDs = append(reportedIDs, record.ID)
	}
	bodyBuilder.WriteString("```\n\n")

	// 详细错误信息
	bodyBuilder.WriteString("### 详细错误信息\n\n")
	bodyBuilder.WriteString("| arXiv ID | 标题 | 阶段 | 错误信息 | 重试次数 |\n")
	bodyBuilder.WriteString("|----------|------|------|----------|----------|\n")
	
	for _, record := range errorList {
		// 截断过长的错误信息
		errorMsg := record.ErrorMsg
		if len(errorMsg) > 100 {
			errorMsg = errorMsg[:100] + "..."
		}
		// 转义 Markdown 表格中的特殊字符
		errorMsg = strings.ReplaceAll(errorMsg, "|", "\\|")
		errorMsg = strings.ReplaceAll(errorMsg, "\n", " ")
		
		recordTitle := record.Title
		if len(recordTitle) > 50 {
			recordTitle = recordTitle[:50] + "..."
		}
		recordTitle = strings.ReplaceAll(recordTitle, "|", "\\|")
		
		stageName := errors.GetStageDisplayName(record.Stage)
		
		bodyBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d |\n",
			record.ID, recordTitle, stageName, errorMsg, record.RetryCount))
	}

	// 创建 Issue
	result, err := uploader.CreateIssue(title, bodyBuilder.String(), []string{"bug", "translation-error"})
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub issue: %w", err)
	}

	// 上报成功后，标记这些错误为已上报
	if err := a.errorMgr.MarkAsReported(reportedIDs); err != nil {
		logger.Warn("failed to mark errors as reported", logger.Err(err))
		// 不返回错误，因为 Issue 已经创建成功
	}

	logger.Info("errors reported to GitHub issue",
		logger.String("issueURL", result.IssueURL),
		logger.Int("issueNumber", result.IssueNum),
		logger.Int("errorCount", len(errorList)))

	return result, nil
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

// checkPageCountDifference 检查翻译前后的页数差异
// 如果翻译后页数比原始页数少超过15%，返回可疑结果
func (a *App) checkPageCountDifference(originalPDFPath, translatedPDFPath string) *pdf.PageCountResult {
	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: a.workDir,
	})

	result, err := translator.CheckPageCountDifference(originalPDFPath, translatedPDFPath)
	if err != nil {
		logger.Warn("failed to check page count difference", logger.Err(err))
		return nil
	}

	return result
}

// ============================================================================
// License-related Wails binding methods
// Validates: Requirements 1.4, 1.5, 3.3, 3.4, 8.1, 8.2, 8.3
// ============================================================================

// ActivationResult represents the result of license activation
type ActivationResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// LicenseDisplayInfo represents license info for display in the about page
type LicenseDisplayInfo struct {
	WorkMode       string `json:"work_mode"`
	SerialNumber   string `json:"serial_number,omitempty"`   // 激活所用序列号
	ExpiresAt      string `json:"expires_at,omitempty"`
	DailyAnalysis  int    `json:"daily_analysis,omitempty"` // 每日分析次数限制，0 表示无限制
	DaysRemaining  int    `json:"days_remaining,omitempty"`
	IsExpiringSoon bool   `json:"is_expiring_soon,omitempty"`
}

// RequestSNResult represents the result of a serial number request
type RequestSNResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	SerialNumber string `json:"serial_number,omitempty"` // 分配的序列号（仅成功时返回）
}

// LicenseValidityResult represents the result of license validity check
type LicenseValidityResult struct {
	IsValid        bool   `json:"is_valid"`
	IsExpired      bool   `json:"is_expired"`
	IsExpiringSoon bool   `json:"is_expiring_soon"`
	Message        string `json:"message"`
}

// GetWorkMode returns the current work mode.
// Returns empty string if no work mode is configured.
// Validates: Requirements 1.4, 1.5
func (a *App) GetWorkMode() string {
	if a.config == nil {
		return ""
	}
	return string(a.config.GetWorkMode())
}

// SetWorkMode sets the work mode.
// Validates: Requirements 1.4, 1.5
func (a *App) SetWorkMode(mode string) error {
	if a.config == nil {
		return fmt.Errorf("配置管理器未初始化")
	}
	return a.config.SetWorkMode(license.WorkMode(mode))
}

// ActivateLicense activates a commercial license with the given serial number.
// On success, it saves the license info to config.
// Validates: Requirements 3.3, 3.4
func (a *App) ActivateLicense(serialNumber string) (*ActivationResult, error) {
	if a.licenseClient == nil {
		return nil, fmt.Errorf("授权客户端未初始化")
	}
	if a.config == nil {
		return nil, fmt.Errorf("配置管理器未初始化")
	}

	logger.Info("activating license", logger.String("serialNumber", "****-****-****-"+serialNumber[len(serialNumber)-4:]))

	// Call the license client to activate
	resp, activationData, err := a.licenseClient.Activate(serialNumber)
	if err != nil {
		logger.Error("license activation failed", err)
		return &ActivationResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Save license info to config
	licenseInfo := &license.LicenseInfo{
		WorkMode:       license.WorkModeCommercial,
		SerialNumber:   serialNumber,
		ActivationData: activationData,
		ActivatedAt:    time.Now(),
	}

	if err := a.config.SetLicenseInfo(licenseInfo); err != nil {
		logger.Error("failed to save license info", err)
		return &ActivationResult{
			Success: false,
			Message: "激活成功但保存配置失败: " + err.Error(),
		}, nil
	}

	// Set work mode to commercial
	if err := a.config.SetWorkMode(license.WorkModeCommercial); err != nil {
		logger.Error("failed to set work mode", err)
		return &ActivationResult{
			Success: false,
			Message: "激活成功但设置工作模式失败: " + err.Error(),
		}, nil
	}

	// Apply the LLM config from the license to translator, validator, and PDF translator
	// This is critical - without this, the translator will use old/empty API key
	a.applyLicenseLLMConfig()

	logger.Info("license activated successfully", logger.String("expiresAt", resp.ExpiresAt))

	return &ActivationResult{
		Success:   true,
		Message:   "激活成功",
		ExpiresAt: resp.ExpiresAt,
	}, nil
}

// RequestSerialNumber requests a serial number via email.
// Returns a structured result containing success status, message, and serial number if available.
// Validates: Requirements 3.8
func (a *App) RequestSerialNumber(email string) (*RequestSNResult, error) {
	if a.licenseClient == nil {
		return nil, fmt.Errorf("授权客户端未初始化")
	}

	logger.Info("requesting serial number", logger.String("email", email))

	result, err := a.licenseClient.RequestSN(email)
	if err != nil {
		logger.Error("serial number request failed", err)
		return nil, err
	}

	logger.Info("serial number request submitted", 
		logger.String("message", result.Message),
		logger.Bool("hasSN", result.SerialNumber != ""))
	
	return &RequestSNResult{
		Success:      result.Success,
		Message:      result.Message,
		SerialNumber: result.SerialNumber,
	}, nil
}

// GetLicenseInfo returns license information for display in the about page.
// Validates: Requirements 8.1, 8.2, 8.3, 10.1, 10.2, 10.3, 10.4, 10.5
func (a *App) GetLicenseInfo() (*LicenseDisplayInfo, error) {
	if a.config == nil {
		return nil, fmt.Errorf("配置管理器未初始化")
	}

	workMode := a.config.GetWorkMode()
	info := &LicenseDisplayInfo{
		WorkMode: string(workMode),
	}

	// For opensource mode, just return the work mode
	if workMode != license.WorkModeCommercial {
		return info, nil
	}

	// For commercial mode, get license details
	licenseInfo := a.config.GetLicenseInfo()
	if licenseInfo == nil || licenseInfo.ActivationData == nil {
		return info, nil
	}

	// Add serial number (masked for security - show only last 4 characters)
	if licenseInfo.SerialNumber != "" {
		sn := licenseInfo.SerialNumber
		if len(sn) > 4 {
			info.SerialNumber = "****-****-****-" + sn[len(sn)-4:]
		} else {
			info.SerialNumber = sn
		}
	}

	activationData := licenseInfo.ActivationData
	info.ExpiresAt = activationData.ExpiresAt
	info.DailyAnalysis = activationData.DailyAnalysis

	// Calculate days remaining and expiring soon status
	if a.licenseClient != nil {
		daysRemaining := a.licenseClient.DaysUntilExpiry(activationData)
		info.DaysRemaining = daysRemaining
		info.IsExpiringSoon = daysRemaining >= 0 && daysRemaining <= 7
	}

	return info, nil
}

// CheckLicenseValidity checks if the current license is valid.
// Validates: Requirements 8.1, 8.2, 8.3
func (a *App) CheckLicenseValidity() (*LicenseValidityResult, error) {
	if a.config == nil {
		return nil, fmt.Errorf("配置管理器未初始化")
	}

	// Get work mode status from config manager
	status := a.config.GetWorkModeStatus()

	result := &LicenseValidityResult{
		IsValid:        status.HasValidLicense,
		IsExpired:      status.LicenseExpired,
		IsExpiringSoon: status.LicenseExpiringSoon,
	}

	// Generate appropriate message
	if !status.HasWorkMode {
		result.Message = "未选择工作模式"
	} else if status.IsOpenSource {
		result.IsValid = true
		result.Message = "开源软件模式"
	} else if status.IsCommercial {
		if status.LicenseExpired {
			result.Message = "授权已过期，请重新激活或切换到开源模式"
		} else if status.LicenseExpiringSoon {
			result.Message = fmt.Sprintf("授权将在 %d 天后过期，请及时续费", status.DaysUntilExpiry)
		} else if status.HasValidLicense {
			result.Message = "授权有效"
		} else {
			result.Message = "授权无效，请重新激活"
		}
	}

	return result, nil
}

// RefreshLicense refreshes the license by re-activating with the stored serial number.
// This fetches the latest license data from the server.
// Returns the updated license display info on success.
func (a *App) RefreshLicense() (*LicenseDisplayInfo, error) {
	if a.config == nil {
		return nil, fmt.Errorf("配置管理器未初始化")
	}
	if a.licenseClient == nil {
		return nil, fmt.Errorf("授权客户端未初始化")
	}

	// Check if we're in commercial mode
	workMode := a.config.GetWorkMode()
	if workMode != license.WorkModeCommercial {
		return nil, fmt.Errorf("当前不是商业授权模式")
	}

	// Get the stored license info
	licenseInfo := a.config.GetLicenseInfo()
	if licenseInfo == nil {
		logger.Warn("license info is nil")
		return nil, fmt.Errorf("未找到已保存的授权信息")
	}
	
	if licenseInfo.SerialNumber == "" {
		logger.Warn("serial number is empty in license info")
		return nil, fmt.Errorf("未找到已保存的序列号")
	}

	serialNumber := licenseInfo.SerialNumber
	logger.Info("refreshing license", 
		logger.String("serialNumber", serialNumber),
		logger.Int("snLength", len(serialNumber)))

	// Re-activate with the stored serial number
	resp, activationData, err := a.licenseClient.Activate(serialNumber)
	if err != nil {
		logger.Error("license refresh failed", err)
		return nil, fmt.Errorf("刷新授权失败: %v", err)
	}

	// Update license info in config
	newLicenseInfo := &license.LicenseInfo{
		WorkMode:       license.WorkModeCommercial,
		SerialNumber:   serialNumber,
		ActivationData: activationData,
		ActivatedAt:    time.Now(),
	}

	if err := a.config.SetLicenseInfo(newLicenseInfo); err != nil {
		logger.Error("failed to save refreshed license info", err)
		return nil, fmt.Errorf("保存授权信息失败: %v", err)
	}

	// Apply the new LLM config from the refreshed license
	a.applyLicenseLLMConfig()

	logger.Info("license refreshed successfully", logger.String("expiresAt", resp.ExpiresAt))

	// Return the updated license display info
	return a.GetLicenseInfo()
}
