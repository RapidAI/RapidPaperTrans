package results

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewResultManager(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "results_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create ResultManager: %v", err)
	}

	if manager.GetBaseDir() != tempDir {
		t.Errorf("Expected base dir %s, got %s", tempDir, manager.GetBaseDir())
	}
}

func TestSaveAndLoadPaperInfo(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "results_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create ResultManager: %v", err)
	}

	// Create test paper info
	info := &PaperInfo{
		ArxivID:        "2301.00001",
		Title:          "Test Paper Title",
		TranslatedAt:   time.Now(),
		OriginalPDF:    "/path/to/original.pdf",
		TranslatedPDF:  "/path/to/translated.pdf",
		HasLatexSource: true,
	}

	// Save
	if err := manager.SavePaperInfo(info); err != nil {
		t.Fatalf("Failed to save paper info: %v", err)
	}

	// Load
	loaded, err := manager.LoadPaperInfo("2301.00001")
	if err != nil {
		t.Fatalf("Failed to load paper info: %v", err)
	}

	if loaded.ArxivID != info.ArxivID {
		t.Errorf("ArxivID mismatch: expected %s, got %s", info.ArxivID, loaded.ArxivID)
	}
	if loaded.Title != info.Title {
		t.Errorf("Title mismatch: expected %s, got %s", info.Title, loaded.Title)
	}
}

func TestListPapers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "results_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create ResultManager: %v", err)
	}

	// Save multiple papers
	papers := []*PaperInfo{
		{ArxivID: "2301.00001", Title: "Paper 1", TranslatedAt: time.Now().Add(-time.Hour)},
		{ArxivID: "2301.00002", Title: "Paper 2", TranslatedAt: time.Now()},
	}

	for _, p := range papers {
		if err := manager.SavePaperInfo(p); err != nil {
			t.Fatalf("Failed to save paper: %v", err)
		}
	}

	// List
	list, err := manager.ListPapers()
	if err != nil {
		t.Fatalf("Failed to list papers: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("Expected 2 papers, got %d", len(list))
	}

	// Should be sorted by date, newest first
	if list[0].ArxivID != "2301.00002" {
		t.Errorf("Expected newest paper first, got %s", list[0].ArxivID)
	}
}

func TestDeletePaper(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "results_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create ResultManager: %v", err)
	}

	// Save a paper
	info := &PaperInfo{ArxivID: "2301.00001", Title: "Test Paper"}
	if err := manager.SavePaperInfo(info); err != nil {
		t.Fatalf("Failed to save paper: %v", err)
	}

	// Verify it exists
	if !manager.PaperExists("2301.00001") {
		t.Error("Paper should exist after saving")
	}

	// Delete
	if err := manager.DeletePaper("2301.00001"); err != nil {
		t.Fatalf("Failed to delete paper: %v", err)
	}

	// Verify it's gone
	if manager.PaperExists("2301.00001") {
		t.Error("Paper should not exist after deletion")
	}
}

func TestExtractArxivID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2301.00001", "2301.00001"},
		{"https://arxiv.org/abs/2301.00001", "2301.00001"},
		{"https://arxiv.org/pdf/2301.00001.pdf", "2301.00001"},
		{"hep-th/9901001", "hep-th/9901001"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := ExtractArxivID(tt.input)
		if result != tt.expected {
			t.Errorf("ExtractArxivID(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractTitleFromTeX(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{`\title{Simple Title}`, "Simple Title"},
		{`\title{Title with {nested} braces}`, "Title with {nested} braces"},
		{`\documentclass{article}\n\title{My Paper}\n\author{Author}`, "My Paper"},
		{`No title here`, ""},
	}

	for _, tt := range tests {
		result := ExtractTitleFromTeX(tt.content)
		if result != tt.expected {
			t.Errorf("ExtractTitleFromTeX() = %q, expected %q", result, tt.expected)
		}
	}
}

func TestSanitizeArxivID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2301.00001", "2301.00001"},
		{"hep-th/9901001", "hep-th_9901001"},
	}

	for _, tt := range tests {
		result := sanitizeArxivID(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeArxivID(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "results_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create ResultManager: %v", err)
	}

	arxivID := "2301.00001"
	
	paperDir := manager.GetPaperDir(arxivID)
	if !filepath.IsAbs(paperDir) || !contains(paperDir, arxivID) {
		t.Errorf("Invalid paper dir: %s", paperDir)
	}

	originalPDF := manager.GetOriginalPDFPath(arxivID)
	if !contains(originalPDF, "original.pdf") {
		t.Errorf("Invalid original PDF path: %s", originalPDF)
	}

	translatedPDF := manager.GetTranslatedPDFPath(arxivID)
	if !contains(translatedPDF, "translated.pdf") {
		t.Errorf("Invalid translated PDF path: %s", translatedPDF)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}


// TestUpdatePaperStatus tests updating paper status
func TestUpdatePaperStatus(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// First save a paper
	info := &PaperInfo{
		ArxivID:      "2301.00001",
		Title:        "Test Paper",
		TranslatedAt: time.Now(),
		Status:       StatusExtracted,
	}
	if err := manager.SavePaperInfo(info); err != nil {
		t.Fatalf("Failed to save paper info: %v", err)
	}

	// Update status
	if err := manager.UpdatePaperStatus("2301.00001", StatusComplete, ""); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Load and verify
	loaded, err := manager.LoadPaperInfo("2301.00001")
	if err != nil {
		t.Fatalf("Failed to load paper info: %v", err)
	}

	if loaded.Status != StatusComplete {
		t.Errorf("Expected status %s, got %s", StatusComplete, loaded.Status)
	}
}

// TestUpdatePaperStatusWithError tests updating paper status with error message
func TestUpdatePaperStatusWithError(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// First save a paper
	info := &PaperInfo{
		ArxivID:      "2301.00002",
		Title:        "Test Paper",
		TranslatedAt: time.Now(),
		Status:       StatusTranslating,
	}
	if err := manager.SavePaperInfo(info); err != nil {
		t.Fatalf("Failed to save paper info: %v", err)
	}

	// Update status with error
	errorMsg := "Compilation failed: missing package"
	if err := manager.UpdatePaperStatus("2301.00002", StatusError, errorMsg); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Load and verify
	loaded, err := manager.LoadPaperInfo("2301.00002")
	if err != nil {
		t.Fatalf("Failed to load paper info: %v", err)
	}

	if loaded.Status != StatusError {
		t.Errorf("Expected status %s, got %s", StatusError, loaded.Status)
	}
	if loaded.ErrorMessage != errorMsg {
		t.Errorf("Expected error message %s, got %s", errorMsg, loaded.ErrorMessage)
	}
}

// TestGetIncompleteTranslations tests getting incomplete translations
func TestGetIncompleteTranslations(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Save papers with different statuses
	papers := []struct {
		arxivID string
		status  TranslationStatus
	}{
		{"2301.00001", StatusComplete},
		{"2301.00002", StatusError},
		{"2301.00003", StatusTranslated},
		{"2301.00004", StatusComplete},
		{"2301.00005", StatusExtracted},
	}

	for _, p := range papers {
		info := &PaperInfo{
			ArxivID:      p.arxivID,
			Title:        "Test Paper " + p.arxivID,
			TranslatedAt: time.Now(),
			Status:       p.status,
		}
		if err := manager.SavePaperInfo(info); err != nil {
			t.Fatalf("Failed to save paper info: %v", err)
		}
	}

	// Get incomplete translations
	incomplete, err := manager.GetIncompleteTranslations()
	if err != nil {
		t.Fatalf("Failed to get incomplete translations: %v", err)
	}

	// Should have 3 incomplete (error, translated, extracted)
	if len(incomplete) != 3 {
		t.Errorf("Expected 3 incomplete translations, got %d", len(incomplete))
	}

	// Verify none are complete
	for _, p := range incomplete {
		if p.Status == StatusComplete {
			t.Errorf("Found complete paper in incomplete list: %s", p.ArxivID)
		}
	}
}

// TestTranslationStatusConstants tests that status constants are defined correctly
func TestTranslationStatusConstants(t *testing.T) {
	statuses := []TranslationStatus{
		StatusPending,
		StatusDownloading,
		StatusExtracted,
		StatusOriginalCompiled,
		StatusTranslating,
		StatusTranslated,
		StatusCompiling,
		StatusComplete,
		StatusError,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("Found empty status constant")
		}
	}
}


// TestCalculateFileMD5 tests MD5 calculation for files
func TestCalculateFileMD5(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a test file
	testContent := []byte("Hello, World!")
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Calculate MD5
	md5Hash, err := CalculateFileMD5(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate MD5: %v", err)
	}
	
	// Expected MD5 for "Hello, World!"
	expectedMD5 := "65a8e27d8879283831b664bd8b7f0ad4"
	if md5Hash != expectedMD5 {
		t.Errorf("Expected MD5 %s, got %s", expectedMD5, md5Hash)
	}
}

// TestCalculateFileMD5_NonExistent tests MD5 calculation for non-existent file
func TestCalculateFileMD5_NonExistent(t *testing.T) {
	_, err := CalculateFileMD5("/nonexistent/file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestFindByMD5 tests finding papers by MD5 hash
func TestFindByMD5(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	
	// Save a paper with MD5
	info := &PaperInfo{
		ArxivID:    "md5_1234567890abcdef",
		Title:      "Test Paper",
		SourceType: SourceTypeZip,
		SourceMD5:  "1234567890abcdef1234567890abcdef",
		Status:     StatusComplete,
	}
	if err := manager.SavePaperInfo(info); err != nil {
		t.Fatalf("Failed to save paper info: %v", err)
	}
	
	// Find by MD5
	found, err := manager.FindByMD5("1234567890abcdef1234567890abcdef")
	if err != nil {
		t.Fatalf("Failed to find by MD5: %v", err)
	}
	
	if found == nil {
		t.Fatal("Expected to find paper by MD5")
	}
	
	if found.ArxivID != info.ArxivID {
		t.Errorf("Expected arXiv ID %s, got %s", info.ArxivID, found.ArxivID)
	}
}

// TestFindByMD5_NotFound tests finding non-existent MD5
func TestFindByMD5_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	
	found, err := manager.FindByMD5("nonexistent_md5_hash")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if found != nil {
		t.Error("Expected nil for non-existent MD5")
	}
}

// TestCheckExistingTranslation_ArxivComplete tests checking existing complete arXiv translation
func TestCheckExistingTranslation_ArxivComplete(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	
	// Save a complete paper
	info := &PaperInfo{
		ArxivID:    "2301.00001",
		Title:      "Test Paper",
		SourceType: SourceTypeArxiv,
		Status:     StatusComplete,
	}
	if err := manager.SavePaperInfo(info); err != nil {
		t.Fatalf("Failed to save paper info: %v", err)
	}
	
	// Check existing translation
	existingInfo, err := manager.CheckExistingTranslation("2301.00001", SourceTypeArxiv)
	if err != nil {
		t.Fatalf("Failed to check existing translation: %v", err)
	}
	
	if !existingInfo.Exists {
		t.Error("Expected existing translation to be found")
	}
	
	if !existingInfo.IsComplete {
		t.Error("Expected translation to be complete")
	}
	
	if existingInfo.CanContinue {
		t.Error("Expected CanContinue to be false for complete translation")
	}
}

// TestCheckExistingTranslation_NotFound tests checking non-existent translation
func TestCheckExistingTranslation_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	manager, err := NewResultManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	
	existingInfo, err := manager.CheckExistingTranslation("nonexistent", SourceTypeArxiv)
	if err != nil {
		t.Fatalf("Failed to check existing translation: %v", err)
	}
	
	if existingInfo.Exists {
		t.Error("Expected no existing translation")
	}
}

// TestSourceTypeConstants tests that source type constants are defined correctly
func TestSourceTypeConstants(t *testing.T) {
	sourceTypes := []SourceType{
		SourceTypeArxiv,
		SourceTypeZip,
		SourceTypePDF,
	}
	
	for _, st := range sourceTypes {
		if st == "" {
			t.Error("Found empty source type constant")
		}
	}
}
