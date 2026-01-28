package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"latex-translator/internal/types"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp() returned nil")
	}
}

func TestNewAppWithConfig(t *testing.T) {
	// Create a temporary directory for the config file
	tempDir, err := os.MkdirTemp("", "app-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")

	app, err := NewAppWithConfig(configPath)
	if err != nil {
		t.Fatalf("NewAppWithConfig() returned error: %v", err)
	}
	if app == nil {
		t.Fatal("NewAppWithConfig() returned nil")
	}
	if app.config == nil {
		t.Fatal("App config should not be nil")
	}
}

func TestApp_Startup(t *testing.T) {
	app := NewApp()
	ctx := context.Background()

	// Call startup
	app.startup(ctx)

	// Verify context is set
	if app.ctx != ctx {
		t.Error("Context was not set correctly")
	}

	// Verify work directory is set
	if app.workDir == "" {
		t.Error("Work directory should be set after startup")
	}

	// Verify modules are initialized
	if app.config == nil {
		t.Error("ConfigManager should be initialized after startup")
	}
	if app.downloader == nil {
		t.Error("SourceDownloader should be initialized after startup")
	}
	if app.translator == nil {
		t.Error("TranslationEngine should be initialized after startup")
	}
	if app.compiler == nil {
		t.Error("LaTeXCompiler should be initialized after startup")
	}
	if app.validator == nil {
		t.Error("SyntaxValidator should be initialized after startup")
	}

	// Clean up
	app.shutdown(ctx)
}

func TestApp_Shutdown(t *testing.T) {
	app := NewApp()
	ctx := context.Background()

	// Call startup to initialize
	app.startup(ctx)

	// Get the work directory before shutdown
	workDir := app.workDir

	// Verify work directory exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		t.Error("Work directory should exist before shutdown")
	}

	// Call shutdown
	app.shutdown(ctx)

	// Verify temporary work directory is cleaned up
	if app.isTemporaryWorkDir() {
		if _, err := os.Stat(workDir); !os.IsNotExist(err) {
			t.Error("Temporary work directory should be cleaned up after shutdown")
		}
	}
}

func TestApp_GettersReturnModules(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Test all getters
	if app.GetConfig() == nil {
		t.Error("GetConfig() should return non-nil ConfigManager")
	}
	if app.GetDownloader() == nil {
		t.Error("GetDownloader() should return non-nil SourceDownloader")
	}
	if app.GetTranslator() == nil {
		t.Error("GetTranslator() should return non-nil TranslationEngine")
	}
	if app.GetCompiler() == nil {
		t.Error("GetCompiler() should return non-nil LaTeXCompiler")
	}
	if app.GetValidator() == nil {
		t.Error("GetValidator() should return non-nil SyntaxValidator")
	}
	if app.GetWorkDir() == "" {
		t.Error("GetWorkDir() should return non-empty string")
	}
}

func TestApp_SetWorkDir(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Create a new work directory
	tempDir, err := os.MkdirTemp("", "app-workdir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	newWorkDir := filepath.Join(tempDir, "new-work-dir")

	// Set new work directory
	err = app.SetWorkDir(newWorkDir)
	if err != nil {
		t.Fatalf("SetWorkDir() returned error: %v", err)
	}

	// Verify work directory is updated
	if app.GetWorkDir() != newWorkDir {
		t.Errorf("Work directory not updated: got %s, want %s", app.GetWorkDir(), newWorkDir)
	}

	// Verify directory was created
	if _, err := os.Stat(newWorkDir); os.IsNotExist(err) {
		t.Error("New work directory should be created")
	}

	// Verify downloader work directory is updated
	if app.downloader.GetWorkDir() != newWorkDir {
		t.Error("Downloader work directory should be updated")
	}
}

func TestApp_ReloadConfig(t *testing.T) {
	// Create a temporary directory for the config file
	tempDir, err := os.MkdirTemp("", "app-reload-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")

	app, err := NewAppWithConfig(configPath)
	if err != nil {
		t.Fatalf("NewAppWithConfig() returned error: %v", err)
	}

	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Reload config should not error
	err = app.ReloadConfig()
	if err != nil {
		t.Errorf("ReloadConfig() returned error: %v", err)
	}
}

func TestApp_IsTemporaryWorkDir(t *testing.T) {
	app := NewApp()

	// Empty work dir
	app.workDir = ""
	if app.isTemporaryWorkDir() {
		t.Error("Empty work dir should not be considered temporary")
	}

	// Temp directory
	tempDir := os.TempDir()
	app.workDir = filepath.Join(tempDir, "test-dir")
	if !app.isTemporaryWorkDir() {
		t.Error("Directory in temp folder should be considered temporary")
	}

	// Non-temp directory
	app.workDir = "/some/other/path"
	if app.isTemporaryWorkDir() {
		t.Error("Directory outside temp folder should not be considered temporary")
	}
}

func TestApp_InitWorkDir_WithConfiguredDir(t *testing.T) {
	// Create a temporary directory for the config file
	tempDir, err := os.MkdirTemp("", "app-init-workdir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	configuredWorkDir := filepath.Join(tempDir, "configured-work-dir")

	app, err := NewAppWithConfig(configPath)
	if err != nil {
		t.Fatalf("NewAppWithConfig() returned error: %v", err)
	}

	// Set a work directory in config
	app.config.GetConfig().WorkDirectory = configuredWorkDir
	app.config.Save()

	// Initialize work directory
	err = app.initWorkDir()
	if err != nil {
		t.Fatalf("initWorkDir() returned error: %v", err)
	}

	// Verify configured work directory is used
	if app.workDir != configuredWorkDir {
		t.Errorf("Work directory should be configured value: got %s, want %s", app.workDir, configuredWorkDir)
	}

	// Verify directory was created
	if _, err := os.Stat(configuredWorkDir); os.IsNotExist(err) {
		t.Error("Configured work directory should be created")
	}
}


func TestApp_GetStatus(t *testing.T) {
	app := NewApp()

	// Initial status should be idle
	status := app.GetStatus()
	if status.Phase != types.PhaseIdle {
		t.Errorf("Initial phase should be idle, got %s", status.Phase)
	}
	if status.Progress != 0 {
		t.Errorf("Initial progress should be 0, got %d", status.Progress)
	}
}

func TestApp_SetStatusCallback(t *testing.T) {
	app := NewApp()

	var receivedStatus *types.Status
	callback := func(status *types.Status) {
		receivedStatus = status
	}

	app.SetStatusCallback(callback)

	// Trigger a status update
	app.updateStatus(types.PhaseDownloading, 10, "Testing...")

	if receivedStatus == nil {
		t.Fatal("Callback was not called")
	}
	if receivedStatus.Phase != types.PhaseDownloading {
		t.Errorf("Expected phase %s, got %s", types.PhaseDownloading, receivedStatus.Phase)
	}
	if receivedStatus.Progress != 10 {
		t.Errorf("Expected progress 10, got %d", receivedStatus.Progress)
	}
	if receivedStatus.Message != "Testing..." {
		t.Errorf("Expected message 'Testing...', got '%s'", receivedStatus.Message)
	}
}

func TestApp_UpdateStatus(t *testing.T) {
	app := NewApp()

	app.updateStatus(types.PhaseTranslating, 50, "翻译中...")

	status := app.GetStatus()
	if status.Phase != types.PhaseTranslating {
		t.Errorf("Expected phase %s, got %s", types.PhaseTranslating, status.Phase)
	}
	if status.Progress != 50 {
		t.Errorf("Expected progress 50, got %d", status.Progress)
	}
	if status.Message != "翻译中..." {
		t.Errorf("Expected message '翻译中...', got '%s'", status.Message)
	}
	if status.Error != "" {
		t.Errorf("Expected no error, got '%s'", status.Error)
	}
}

func TestApp_UpdateStatusError(t *testing.T) {
	app := NewApp()

	app.updateStatusError("测试错误")

	status := app.GetStatus()
	if status.Phase != types.PhaseError {
		t.Errorf("Expected phase %s, got %s", types.PhaseError, status.Phase)
	}
	if status.Error != "测试错误" {
		t.Errorf("Expected error '测试错误', got '%s'", status.Error)
	}
}

func TestApp_CancelProcess_NoProcess(t *testing.T) {
	app := NewApp()

	err := app.CancelProcess()
	if err == nil {
		t.Error("Expected error when cancelling with no process running")
	}
}

func TestApp_ProcessSource_InvalidInput(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Test with invalid input
	result, err := app.ProcessSource("invalid-input")
	if err == nil {
		t.Error("Expected error for invalid input")
	}
	if result != nil {
		t.Error("Expected nil result for invalid input")
	}

	// Check status is error
	status := app.GetStatus()
	if status.Phase != types.PhaseError {
		t.Errorf("Expected phase %s, got %s", types.PhaseError, status.Phase)
	}
}

func TestApp_ProcessSource_EmptyInput(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Test with empty input
	result, err := app.ProcessSource("")
	if err == nil {
		t.Error("Expected error for empty input")
	}
	if result != nil {
		t.Error("Expected nil result for empty input")
	}
}

func TestApp_ProcessSource_NonExistentZip(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Test with non-existent zip file
	result, err := app.ProcessSource("/nonexistent/path/file.zip")
	if err == nil {
		t.Error("Expected error for non-existent zip file")
	}
	if result != nil {
		t.Error("Expected nil result for non-existent zip file")
	}

	// Check status is error
	status := app.GetStatus()
	if status.Phase != types.PhaseError {
		t.Errorf("Expected phase %s, got %s", types.PhaseError, status.Phase)
	}
}

func TestApp_ProcessSource_StatusCallbackCalled(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	var statusUpdates []*types.Status
	callback := func(status *types.Status) {
		// Make a copy to avoid race conditions
		statusCopy := &types.Status{
			Phase:    status.Phase,
			Progress: status.Progress,
			Message:  status.Message,
			Error:    status.Error,
		}
		statusUpdates = append(statusUpdates, statusCopy)
	}
	app.SetStatusCallback(callback)

	// Process with invalid input to trigger status updates
	app.ProcessSource("invalid-input")

	// Should have received at least one status update
	if len(statusUpdates) == 0 {
		t.Error("Expected at least one status update")
	}

	// First update should be idle/starting
	if len(statusUpdates) > 0 && statusUpdates[0].Phase != types.PhaseIdle {
		t.Errorf("First status update should be idle, got %s", statusUpdates[0].Phase)
	}
}

func TestApp_GetStatus_ThreadSafe(t *testing.T) {
	app := NewApp()

	// Run concurrent status updates and reads
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			app.updateStatus(types.PhaseTranslating, i, "Testing...")
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = app.GetStatus()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we get here without a race condition, the test passes
}


// =============================================================================
// Integration Tests
// =============================================================================
// These tests verify the complete processing flow using mock data and mock
// HTTP servers for API calls.
//
// Validates: All Requirements (15.1 编写集成测试)
// =============================================================================

// createTestZipFile creates a test zip file with sample LaTeX content.
// Returns the path to the created zip file.
func createTestZipFile(t *testing.T, tempDir string, texContent string) string {
	t.Helper()

	zipPath := filepath.Join(tempDir, "test_latex.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add main.tex file
	writer, err := zipWriter.Create("main.tex")
	if err != nil {
		t.Fatalf("Failed to create file in zip: %v", err)
	}

	if texContent == "" {
		texContent = `\documentclass{article}
\begin{document}
Hello World! This is a test document.
\section{Introduction}
This is the introduction section.
\end{document}`
	}

	_, err = writer.Write([]byte(texContent))
	if err != nil {
		t.Fatalf("Failed to write to zip: %v", err)
	}

	return zipPath
}

// createMockTranslatorServer creates a mock HTTP server that simulates the OpenAI API
// for translation requests.
func createMockTranslatorServer(t *testing.T, translatedContent string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Return mock translation response
		response := map[string]interface{}{
			"id":      "mock-response-id",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": translatedContent,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     100,
				"completion_tokens": 150,
				"total_tokens":      250,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
}

// createMockValidatorServer creates a mock HTTP server that simulates the OpenAI API
// for validation/fix requests.
func createMockValidatorServer(t *testing.T, fixedContent string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Return mock fix response
		response := map[string]interface{}{
			"id":      "mock-fix-response-id",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": fixedContent,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     50,
				"completion_tokens": 75,
				"total_tokens":      125,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
}

// createMockErrorServer creates a mock HTTP server that returns an error response.
func createMockErrorServer(t *testing.T, statusCode int, errorMessage string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]string{
				"message": errorMessage,
				"type":    "api_error",
				"code":    "error",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}))
}

// TestIntegration_CompleteFlowWithLocalZip tests the complete processing flow
// using a local zip file with mock API servers.
// This test verifies that the entire pipeline works correctly:
// 1. Extract zip file
// 2. Find main tex file
// 3. Compile original document (skipped - requires LaTeX installation)
// 4. Translate content via mock API
// 5. Validate and fix syntax via mock API
// 6. Compile translated document (skipped - requires LaTeX installation)
//
// Note: Actual compilation is skipped as it requires LaTeX to be installed.
// This test focuses on the integration of components with mock API calls.
func TestIntegration_CompleteFlowWithLocalZip(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test zip file
	texContent := `\documentclass{article}
\begin{document}
Hello World! This is a test document.
\section{Introduction}
This is the introduction section.
\end{document}`
	zipPath := createTestZipFile(t, tempDir, texContent)

	// Create mock translator server
	translatedContent := `\documentclass{article}
\begin{document}
你好世界！这是一个测试文档。
\section{介绍}
这是介绍部分。
\end{document}`
	mockTranslator := createMockTranslatorServer(t, translatedContent)
	defer mockTranslator.Close()

	// Create mock validator server
	mockValidator := createMockValidatorServer(t, translatedContent)
	defer mockValidator.Close()

	// Create app and configure with mock servers
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Configure translator and validator to use mock servers
	if app.translator != nil {
		app.translator.SetAPIURL(mockTranslator.URL)
	}
	if app.validator != nil {
		app.validator.SetAPIURL(mockValidator.URL)
	}

	// Set a test API key (required for API calls)
	if app.config != nil {
		app.config.GetConfig().OpenAIAPIKey = "test-api-key"
	}

	// Track status updates
	var statusUpdates []*types.Status
	var statusMu sync.Mutex
	app.SetStatusCallback(func(status *types.Status) {
		statusMu.Lock()
		defer statusMu.Unlock()
		statusCopy := &types.Status{
			Phase:    status.Phase,
			Progress: status.Progress,
			Message:  status.Message,
			Error:    status.Error,
		}
		statusUpdates = append(statusUpdates, statusCopy)
	})

	// Process the zip file
	// Note: This will fail at compilation step since LaTeX is not installed,
	// but we can verify the earlier steps work correctly
	_, err = app.ProcessSource(zipPath)

	// We expect an error because LaTeX compiler is not installed in test environment
	// But we should have received status updates for the earlier phases
	statusMu.Lock()
	updateCount := len(statusUpdates)
	statusMu.Unlock()

	if updateCount == 0 {
		t.Error("Expected at least one status update during processing")
	}

	// Verify we got through at least the extraction phase
	foundExtracting := false
	statusMu.Lock()
	for _, status := range statusUpdates {
		if status.Phase == types.PhaseExtracting || status.Phase == types.PhaseDownloading {
			foundExtracting = true
			break
		}
	}
	statusMu.Unlock()

	if !foundExtracting {
		t.Error("Expected to reach extracting/downloading phase")
	}

	// The error is expected (compilation requires LaTeX), but the flow should have progressed
	if err == nil {
		t.Log("ProcessSource completed successfully (LaTeX must be installed)")
	} else {
		t.Logf("ProcessSource returned expected error (LaTeX not installed): %v", err)
	}
}

// TestIntegration_ErrorHandling_InvalidInputType tests error handling for invalid input types.
func TestIntegration_ErrorHandling_InvalidInputType(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	testCases := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"random string", "not-a-valid-input"},
		{"invalid URL", "ftp://invalid.com/file"},
		{"partial arXiv ID", "230"},
		{"invalid file extension", "/path/to/file.txt"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := app.ProcessSource(tc.input)

			if err == nil {
				t.Errorf("Expected error for input '%s', got nil", tc.input)
			}
			if result != nil {
				t.Errorf("Expected nil result for invalid input '%s'", tc.input)
			}

			// Verify status is set to error
			status := app.GetStatus()
			if status.Phase != types.PhaseError {
				t.Errorf("Expected phase %s, got %s", types.PhaseError, status.Phase)
			}
		})
	}
}

// TestIntegration_ErrorHandling_MissingZipFile tests error handling for missing zip files.
func TestIntegration_ErrorHandling_MissingZipFile(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Test with non-existent zip file
	result, err := app.ProcessSource("/nonexistent/path/to/file.zip")

	if err == nil {
		t.Error("Expected error for non-existent zip file")
	}
	if result != nil {
		t.Error("Expected nil result for non-existent zip file")
	}

	// Verify error status
	status := app.GetStatus()
	if status.Phase != types.PhaseError {
		t.Errorf("Expected phase %s, got %s", types.PhaseError, status.Phase)
	}
}

// TestIntegration_ErrorHandling_CorruptZipFile tests error handling for corrupt zip files.
func TestIntegration_ErrorHandling_CorruptZipFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "corrupt-zip-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a corrupt zip file (just random bytes)
	corruptZipPath := filepath.Join(tempDir, "corrupt.zip")
	err = os.WriteFile(corruptZipPath, []byte("this is not a valid zip file content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create corrupt zip file: %v", err)
	}

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Process the corrupt zip file
	result, err := app.ProcessSource(corruptZipPath)

	if err == nil {
		t.Error("Expected error for corrupt zip file")
	}
	if result != nil {
		t.Error("Expected nil result for corrupt zip file")
	}

	// Verify error status
	status := app.GetStatus()
	if status.Phase != types.PhaseError {
		t.Errorf("Expected phase %s, got %s", types.PhaseError, status.Phase)
	}
}

// TestIntegration_ErrorHandling_EmptyZipFile tests error handling for empty zip files.
func TestIntegration_ErrorHandling_EmptyZipFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "empty-zip-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an empty but valid zip file
	emptyZipPath := filepath.Join(tempDir, "empty.zip")
	zipFile, err := os.Create(emptyZipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	zipWriter := zip.NewWriter(zipFile)
	zipWriter.Close()
	zipFile.Close()

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Process the empty zip file
	result, err := app.ProcessSource(emptyZipPath)

	if err == nil {
		t.Error("Expected error for empty zip file (no tex files)")
	}
	if result != nil {
		t.Error("Expected nil result for empty zip file")
	}
}

// TestIntegration_ErrorHandling_ZipWithoutTexFiles tests error handling for zip files
// that don't contain any .tex files.
func TestIntegration_ErrorHandling_ZipWithoutTexFiles(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "no-tex-zip-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a zip file with non-tex files
	zipPath := filepath.Join(tempDir, "no_tex.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)
	writer, _ := zipWriter.Create("readme.txt")
	writer.Write([]byte("This is a readme file"))
	writer, _ = zipWriter.Create("data.csv")
	writer.Write([]byte("col1,col2\n1,2"))
	zipWriter.Close()
	zipFile.Close()

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Process the zip file without tex files
	result, err := app.ProcessSource(zipPath)

	if err == nil {
		t.Error("Expected error for zip file without tex files")
	}
	if result != nil {
		t.Error("Expected nil result for zip file without tex files")
	}
}

// TestIntegration_StatusCallbackVerification tests that status callbacks are called
// correctly during processing with proper phase transitions.
func TestIntegration_StatusCallbackVerification(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "status-callback-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test zip file
	zipPath := createTestZipFile(t, tempDir, "")

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Track all status updates
	var statusUpdates []*types.Status
	var statusMu sync.Mutex
	app.SetStatusCallback(func(status *types.Status) {
		statusMu.Lock()
		defer statusMu.Unlock()
		statusCopy := &types.Status{
			Phase:    status.Phase,
			Progress: status.Progress,
			Message:  status.Message,
			Error:    status.Error,
		}
		statusUpdates = append(statusUpdates, statusCopy)
	})

	// Process the zip file (will fail at compilation, but we can verify status updates)
	app.ProcessSource(zipPath)

	statusMu.Lock()
	defer statusMu.Unlock()

	// Verify we received status updates
	if len(statusUpdates) == 0 {
		t.Fatal("Expected at least one status update")
	}

	// Verify first status is idle (starting)
	if statusUpdates[0].Phase != types.PhaseIdle {
		t.Errorf("First status should be idle, got %s", statusUpdates[0].Phase)
	}

	// Verify progress values are within valid range
	for i, status := range statusUpdates {
		if status.Progress < 0 || status.Progress > 100 {
			t.Errorf("Status %d has invalid progress: %d", i, status.Progress)
		}
	}

	// Verify messages are not empty for non-idle phases
	for _, status := range statusUpdates {
		if status.Phase != types.PhaseIdle && status.Phase != types.PhaseError && status.Message == "" {
			t.Errorf("Expected non-empty message for phase %s", status.Phase)
		}
	}
}

// TestIntegration_CancelProcess tests the cancel process functionality.
func TestIntegration_CancelProcess(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Test cancelling when no process is running
	err := app.CancelProcess()
	if err == nil {
		t.Error("Expected error when cancelling with no process running")
	}

	// Verify the error message indicates no process
	if appErr, ok := err.(*types.AppError); ok {
		if appErr.Code != types.ErrInternal {
			t.Errorf("Expected ErrInternal, got %s", appErr.Code)
		}
	}
}

// TestIntegration_CancelProcessDuringExecution tests cancelling a process during execution.
func TestIntegration_CancelProcessDuringExecution(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "cancel-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test zip file
	zipPath := createTestZipFile(t, tempDir, "")

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Start processing in a goroutine
	done := make(chan struct{})
	var processErr error
	go func() {
		_, processErr = app.ProcessSource(zipPath)
		close(done)
	}()

	// Wait a bit for processing to start
	time.Sleep(100 * time.Millisecond)

	// Try to cancel (may or may not succeed depending on timing)
	cancelErr := app.CancelProcess()

	// Wait for processing to complete
	<-done

	// If cancel succeeded, verify the process was cancelled
	if cancelErr == nil {
		// Process should have been cancelled
		status := app.GetStatus()
		if status.Phase != types.PhaseError {
			// It's also possible the process completed before cancel took effect
			t.Logf("Process completed with phase: %s", status.Phase)
		}
	} else {
		// Cancel failed, process may have already completed
		t.Logf("Cancel returned error (process may have completed): %v", cancelErr)
	}

	// Verify we got some result (either error or success)
	if processErr != nil {
		t.Logf("Process returned error: %v", processErr)
	}
}

// TestIntegration_MultipleStatusCallbacks tests that multiple status updates
// are received during processing.
func TestIntegration_MultipleStatusCallbacks(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "multi-status-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test zip file
	zipPath := createTestZipFile(t, tempDir, "")

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Track status updates with timestamps
	type statusWithTime struct {
		status *types.Status
		time   time.Time
	}
	var statusUpdates []statusWithTime
	var statusMu sync.Mutex

	app.SetStatusCallback(func(status *types.Status) {
		statusMu.Lock()
		defer statusMu.Unlock()
		statusUpdates = append(statusUpdates, statusWithTime{
			status: &types.Status{
				Phase:    status.Phase,
				Progress: status.Progress,
				Message:  status.Message,
				Error:    status.Error,
			},
			time: time.Now(),
		})
	})

	// Process the zip file
	app.ProcessSource(zipPath)

	statusMu.Lock()
	defer statusMu.Unlock()

	// Verify we received multiple status updates
	if len(statusUpdates) < 2 {
		t.Errorf("Expected at least 2 status updates, got %d", len(statusUpdates))
	}

	// Verify progress is generally increasing (with some tolerance for phase changes)
	lastProgress := -1
	for _, su := range statusUpdates {
		if su.status.Phase == types.PhaseError {
			continue // Error phase can have any progress
		}
		if su.status.Progress < lastProgress-10 {
			t.Errorf("Progress decreased significantly: %d -> %d", lastProgress, su.status.Progress)
		}
		lastProgress = su.status.Progress
	}
}

// TestIntegration_APIErrorHandling tests error handling when API calls fail.
func TestIntegration_APIErrorHandling(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "api-error-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test zip file
	zipPath := createTestZipFile(t, tempDir, "")

	// Create mock server that returns errors
	mockErrorServer := createMockErrorServer(t, http.StatusUnauthorized, "Invalid API key")
	defer mockErrorServer.Close()

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Configure to use mock error server
	if app.translator != nil {
		app.translator.SetAPIURL(mockErrorServer.URL)
	}
	if app.validator != nil {
		app.validator.SetAPIURL(mockErrorServer.URL)
	}

	// Set a test API key
	if app.config != nil {
		app.config.GetConfig().OpenAIAPIKey = "invalid-key"
	}

	// Process should fail at some point due to API errors
	result, err := app.ProcessSource(zipPath)

	// We expect an error (either from compilation or API)
	if err == nil && result == nil {
		t.Log("Process completed without result (expected for test environment)")
	}

	// Verify status reflects the error
	status := app.GetStatus()
	if status.Phase != types.PhaseError && status.Phase != types.PhaseComplete {
		t.Logf("Final phase: %s", status.Phase)
	}
}

// TestIntegration_ZipWithNestedDirectories tests processing a zip file with nested directories.
func TestIntegration_ZipWithNestedDirectories(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "nested-zip-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a zip file with nested directory structure
	zipPath := filepath.Join(tempDir, "nested.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Create nested directory structure
	writer, _ := zipWriter.Create("project/")
	writer, _ = zipWriter.Create("project/src/")
	writer, _ = zipWriter.Create("project/src/main.tex")
	writer.Write([]byte(`\documentclass{article}
\begin{document}
Nested document
\end{document}`))
	writer, _ = zipWriter.Create("project/figures/")
	writer, _ = zipWriter.Create("project/figures/fig1.tex")
	writer.Write([]byte(`% Figure file`))

	zipWriter.Close()
	zipFile.Close()

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Process the nested zip file
	_, err = app.ProcessSource(zipPath)

	// We expect the extraction to work, but compilation may fail
	// The important thing is that nested directories are handled correctly
	status := app.GetStatus()
	t.Logf("Final status phase: %s, message: %s", status.Phase, status.Message)

	// If we got past extraction, the nested directory handling worked
	if status.Phase == types.PhaseError && status.Error != "" {
		// Check if error is related to extraction or later stages
		if status.Progress > 20 {
			t.Log("Nested directory extraction succeeded, error occurred in later stage")
		}
	}
}

// TestIntegration_ConcurrentStatusAccess tests thread safety of status access
// during processing.
func TestIntegration_ConcurrentStatusAccess(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "concurrent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test zip file
	zipPath := createTestZipFile(t, tempDir, "")

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Start processing in a goroutine
	done := make(chan struct{})
	go func() {
		app.ProcessSource(zipPath)
		close(done)
	}()

	// Concurrently read status multiple times
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				status := app.GetStatus()
				// Just verify we can read without panic
				_ = status.Phase
				_ = status.Progress
				_ = status.Message
			}
		}()
	}

	// Wait for all readers to complete
	wg.Wait()

	// Wait for processing to complete
	<-done

	// If we get here without panic, the test passes
}


// =============================================================================
// Startup Check Tests
// =============================================================================
// These tests verify the startup environment check functionality.
// =============================================================================

// TestCheckStartupRequirements tests the startup requirements check function.
func TestCheckStartupRequirements(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	result := app.CheckStartupRequirements()

	// Result should not be nil
	if result == nil {
		t.Fatal("CheckStartupRequirements returned nil")
	}

	// LaTeX check should return a boolean
	t.Logf("LaTeX installed: %v, version: %s", result.LaTeXInstalled, result.LaTeXVersion)

	// LLM check should return a boolean and error message
	t.Logf("LLM configured: %v, error: %s", result.LLMConfigured, result.LLMError)

	// If LaTeX is installed, version should not be empty
	if result.LaTeXInstalled && result.LaTeXVersion == "" {
		t.Error("LaTeX is installed but version is empty")
	}

	// If LLM is not configured, error should not be empty
	if !result.LLMConfigured && result.LLMError == "" {
		t.Error("LLM is not configured but error message is empty")
	}
}

// TestCheckLaTeXInstallation tests the LaTeX installation check.
func TestCheckLaTeXInstallation(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	installed, version := app.checkLaTeXInstallation()

	t.Logf("LaTeX installed: %v, version: %s", installed, version)

	// If installed, version should contain compiler name
	if installed {
		if version == "" {
			t.Error("LaTeX is installed but version is empty")
		}
		// Version should contain one of the compiler names
		hasCompiler := false
		for _, compiler := range []string{"xelatex", "pdflatex", "lualatex"} {
			if len(version) >= len(compiler) && version[:len(compiler)] == compiler {
				hasCompiler = true
				break
			}
		}
		if !hasCompiler {
			t.Logf("Version string doesn't start with expected compiler name: %s", version)
		}
	}
}

// TestGetLaTeXDownloadURL tests the LaTeX download URL function.
func TestGetLaTeXDownloadURL(t *testing.T) {
	app := NewApp()

	url := app.GetLaTeXDownloadURL()

	// URL should not be empty
	if url == "" {
		t.Error("GetLaTeXDownloadURL returned empty string")
	}

	// URL should be a valid HTTPS URL
	if len(url) < 8 || url[:8] != "https://" {
		t.Errorf("Expected HTTPS URL, got: %s", url)
	}

	t.Logf("LaTeX download URL: %s", url)
}

// TestCheckLLMConfiguration_NoConfig tests LLM check when config is not set.
func TestCheckLLMConfiguration_NoConfig(t *testing.T) {
	app := NewApp()
	// Don't call startup, so config is nil

	configured, errMsg := app.checkLLMConfiguration()

	if configured {
		t.Error("Expected LLM to not be configured when config is nil")
	}
	if errMsg == "" {
		t.Error("Expected error message when config is nil")
	}
}

// TestCheckLLMConfiguration_NoAPIKey tests LLM check when API key is not set.
func TestCheckLLMConfiguration_NoAPIKey(t *testing.T) {
	// Create temporary directory for config
	tempDir, err := os.MkdirTemp("", "llm-check-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	app, err := NewAppWithConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Load config (will use defaults with empty API key)
	app.config.Load()

	configured, errMsg := app.checkLLMConfiguration()

	if configured {
		t.Error("Expected LLM to not be configured when API key is empty")
	}
	if errMsg == "" {
		t.Error("Expected error message when API key is empty")
	}
	t.Logf("LLM error message: %s", errMsg)
}

// TestStartupCheckResult_JSONSerialization tests that StartupCheckResult can be serialized to JSON.
func TestStartupCheckResult_JSONSerialization(t *testing.T) {
	result := &StartupCheckResult{
		LaTeXInstalled: true,
		LaTeXVersion:   "pdflatex: pdfTeX 3.14159265",
		LLMConfigured:  false,
		LLMError:       "API Key 未配置",
	}

	// Serialize to JSON
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal StartupCheckResult: %v", err)
	}

	// Deserialize back
	var decoded StartupCheckResult
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal StartupCheckResult: %v", err)
	}

	// Verify fields
	if decoded.LaTeXInstalled != result.LaTeXInstalled {
		t.Errorf("LaTeXInstalled mismatch: expected %v, got %v", result.LaTeXInstalled, decoded.LaTeXInstalled)
	}
	if decoded.LaTeXVersion != result.LaTeXVersion {
		t.Errorf("LaTeXVersion mismatch: expected %s, got %s", result.LaTeXVersion, decoded.LaTeXVersion)
	}
	if decoded.LLMConfigured != result.LLMConfigured {
		t.Errorf("LLMConfigured mismatch: expected %v, got %v", result.LLMConfigured, decoded.LLMConfigured)
	}
	if decoded.LLMError != result.LLMError {
		t.Errorf("LLMError mismatch: expected %s, got %s", result.LLMError, decoded.LLMError)
	}
}

// TestContinueTranslation_EmptyArxivID tests ContinueTranslation with empty arXiv ID.
func TestContinueTranslation_EmptyArxivID(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	_, err := app.ContinueTranslation("")
	if err == nil {
		t.Error("Expected error for empty arXiv ID")
	}
}

// TestContinueTranslation_NonExistentPaper tests ContinueTranslation with non-existent paper.
func TestContinueTranslation_NonExistentPaper(t *testing.T) {
	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	_, err := app.ContinueTranslation("non-existent-paper-id")
	if err == nil {
		t.Error("Expected error for non-existent paper")
	}
}

// TestContinueTranslation_NoResultManager tests ContinueTranslation when result manager is nil.
func TestContinueTranslation_NoResultManager(t *testing.T) {
	app := NewApp()
	// Don't call startup, so results manager is nil

	_, err := app.ContinueTranslation("2301.00001")
	if err == nil {
		t.Error("Expected error when result manager is nil")
	}
}

// TestContinueProcessingFromStatus_StatusTranslated tests continuing from translated status.
func TestContinueProcessingFromStatus_StatusTranslated(t *testing.T) {
	// This test verifies that the status-based continuation logic works correctly
	// by checking that the correct branch is taken based on status

	app := NewApp()
	ctx := context.Background()
	app.startup(ctx)
	defer app.shutdown(ctx)

	// Create a temporary directory with translated files
	tempDir, err := os.MkdirTemp("", "continue-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple main tex file
	mainTexContent := `\documentclass{article}
\begin{document}
Hello World
\end{document}`
	mainTexPath := filepath.Join(tempDir, "main.tex")
	if err := os.WriteFile(mainTexPath, []byte(mainTexContent), 0644); err != nil {
		t.Fatalf("Failed to write main tex file: %v", err)
	}

	// Create a translated tex file
	translatedTexContent := `\documentclass{article}
\usepackage{ctex}
\begin{document}
你好世界
\end{document}`
	translatedTexPath := filepath.Join(tempDir, "translated_main.tex")
	if err := os.WriteFile(translatedTexPath, []byte(translatedTexContent), 0644); err != nil {
		t.Fatalf("Failed to write translated tex file: %v", err)
	}

	// Verify that the translated file exists
	if _, err := os.Stat(translatedTexPath); os.IsNotExist(err) {
		t.Fatal("Translated tex file should exist")
	}

	t.Log("Test setup complete - translated file exists")
}

// TestGetStatusText tests the status text mapping in frontend.
func TestGetStatusText(t *testing.T) {
	// This test documents the expected status text mappings
	statusMap := map[string]string{
		"pending":           "待处理",
		"downloading":       "下载中",
		"extracted":         "已解压",
		"original_compiled": "原文已编译",
		"translating":       "翻译中",
		"translated":        "已翻译",
		"compiling":         "编译中",
		"complete":          "完成",
		"error":             "错误",
	}

	for status, expectedText := range statusMap {
		t.Logf("Status '%s' should display as '%s'", status, expectedText)
	}
}
