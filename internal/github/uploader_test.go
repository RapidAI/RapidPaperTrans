package github

import (
	"testing"
)

func TestSearchTranslation(t *testing.T) {
	// Create uploader with default settings (no token needed for public search)
	uploader := NewUploader("", "rapidaicoder", "chinesepaper")

	// Test with a known arXiv ID that should exist
	// Using a test ID - you can replace with an actual ID from your repo
	arxivID := "2405.04304"

	result, err := uploader.SearchTranslation(arxivID)
	if err != nil {
		t.Logf("Search error (this is expected if the paper doesn't exist): %v", err)
		// Don't fail the test if the paper doesn't exist
		return
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("Search result for %s:", arxivID)
	t.Logf("  Found: %v", result.Found)
	if result.Found {
		t.Logf("  Chinese PDF: %s", result.ChinesePDF)
		t.Logf("  Bilingual PDF: %s", result.BilingualPDF)
		t.Logf("  LaTeX Zip: %s", result.LatexZip)
		t.Logf("  Download URL (CN): %s", result.DownloadURLCN)
		t.Logf("  Download URL (Bi): %s", result.DownloadURLBi)
		t.Logf("  Download URL (Zip): %s", result.DownloadURLZip)
	}
}

func TestSearchTranslationNotFound(t *testing.T) {
	uploader := NewUploader("", "rapidaicoder", "chinesepaper")

	// Use a non-existent arXiv ID
	arxivID := "9999.99999"

	result, err := uploader.SearchTranslation(arxivID)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Found {
		t.Error("Expected not found for non-existent arXiv ID")
	}
}

func TestSearchTranslationInvalidRepo(t *testing.T) {
	// Test with invalid repository
	uploader := NewUploader("", "nonexistent", "nonexistent")

	arxivID := "2405.04304"

	_, err := uploader.SearchTranslation(arxivID)
	if err == nil {
		t.Error("Expected error for invalid repository")
	}

	t.Logf("Got expected error: %v", err)
}
