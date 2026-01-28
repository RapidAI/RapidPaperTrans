// Package parser provides input parsing and type identification for the LaTeX translator.
package parser

import (
	"testing"

	"latex-translator/internal/types"
)

func TestParseInput_EmptyInput(t *testing.T) {
	_, err := ParseInput("")
	if err == nil {
		t.Error("Expected error for empty input, got nil")
	}
	appErr, ok := err.(*types.AppError)
	if !ok {
		t.Errorf("Expected *types.AppError, got %T", err)
	}
	if appErr.Code != types.ErrInvalidInput {
		t.Errorf("Expected error code %s, got %s", types.ErrInvalidInput, appErr.Code)
	}
}

func TestParseInput_WhitespaceOnlyInput(t *testing.T) {
	_, err := ParseInput("   ")
	if err == nil {
		t.Error("Expected error for whitespace-only input, got nil")
	}
}

func TestParseInput_ArxivURL(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"HTTPS URL", "https://arxiv.org/abs/2301.00001"},
		{"HTTP URL", "http://arxiv.org/abs/2301.00001"},
		{"E-print URL", "https://arxiv.org/e-print/2301.00001"},
		{"PDF URL", "https://arxiv.org/pdf/2301.00001.pdf"},
		{"URL with subdomain", "https://export.arxiv.org/abs/2301.00001"},
		{"URL with uppercase", "https://ARXIV.org/abs/2301.00001"},
		{"URL with mixed case", "https://ArXiv.org/abs/2301.00001"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceType, err := ParseInput(tc.input)
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.input, err)
			}
			if sourceType != types.SourceTypeURL {
				t.Errorf("Expected SourceTypeURL for %s, got %s", tc.input, sourceType)
			}
		})
	}
}

func TestParseInput_ArxivID_NewFormat(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Standard new format", "2301.00001"},
		{"5-digit number", "2301.12345"},
		{"Different year/month", "1912.11111"},
		{"With whitespace", "  2301.00001  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceType, err := ParseInput(tc.input)
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.input, err)
			}
			if sourceType != types.SourceTypeArxivID {
				t.Errorf("Expected SourceTypeArxivID for %s, got %s", tc.input, sourceType)
			}
		})
	}
}

func TestParseInput_ArxivID_OldFormat(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"hep-th category", "hep-th/9901001"},
		{"math-ph category", "math-ph/0001234"},
		{"Simple category", "astro/1234567"},
		{"With whitespace", "  hep-th/9901001  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceType, err := ParseInput(tc.input)
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.input, err)
			}
			if sourceType != types.SourceTypeArxivID {
				t.Errorf("Expected SourceTypeArxivID for %s, got %s", tc.input, sourceType)
			}
		})
	}
}

func TestParseInput_LocalZip(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Simple zip", "paper.zip"},
		{"Path with directory", "/home/user/papers/paper.zip"},
		{"Relative path", "./downloads/paper.zip"},
		{"Windows path", "C:\\Users\\paper.zip"},
		{"Uppercase extension", "paper.ZIP"},
		{"Mixed case extension", "paper.Zip"},
		{"With whitespace", "  paper.zip  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceType, err := ParseInput(tc.input)
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.input, err)
			}
			if sourceType != types.SourceTypeLocalZip {
				t.Errorf("Expected SourceTypeLocalZip for %s, got %s", tc.input, sourceType)
			}
		})
	}
}

func TestParseInput_InvalidInput(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Random text", "hello world"},
		{"Non-arxiv URL", "https://google.com"},
		{"Invalid arXiv ID format", "12345"},
		{"Partial arXiv ID", "2301."},
		{"Invalid old format", "hep-th/123"},
		{"File without extension", "paper"},
		{"Invalid characters in ID", "abc.12345"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInput(tc.input)
			if err == nil {
				t.Errorf("Expected error for invalid input %s, got nil", tc.input)
			}
			appErr, ok := err.(*types.AppError)
			if !ok {
				t.Errorf("Expected *types.AppError, got %T", err)
			}
			if appErr.Code != types.ErrInvalidInput {
				t.Errorf("Expected error code %s, got %s", types.ErrInvalidInput, appErr.Code)
			}
		})
	}
}

func TestParseInput_LocalPDF(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Simple PDF file", "paper.pdf"},
		{"Path with directory", "/path/to/paper.pdf"},
		{"Relative path", "./documents/paper.pdf"},
		{"Windows path", "C:\\Users\\test\\paper.pdf"},
		{"Uppercase extension", "PAPER.PDF"},
		{"Mixed case extension", "Paper.Pdf"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceType, err := ParseInput(tc.input)
			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", tc.input, err)
			}
			if sourceType != types.SourceTypeLocalPDF {
				t.Errorf("Expected SourceTypeLocalPDF for input %s, got %s", tc.input, sourceType)
			}
		})
	}
}

func TestValidateArxivID(t *testing.T) {
	validIDs := []string{
		"2301.00001",
		"2301.12345",
		"hep-th/9901001",
		"math-ph/0001234",
	}

	for _, id := range validIDs {
		if !ValidateArxivID(id) {
			t.Errorf("Expected %s to be a valid arXiv ID", id)
		}
	}

	invalidIDs := []string{
		"",
		"12345",
		"2301.",
		"hep-th/123",
		"abc.12345",
		"https://arxiv.org/abs/2301.00001",
	}

	for _, id := range invalidIDs {
		if ValidateArxivID(id) {
			t.Errorf("Expected %s to be an invalid arXiv ID", id)
		}
	}
}

func TestIsArxivURL(t *testing.T) {
	// Test valid URLs
	validURLs := []string{
		"https://arxiv.org/abs/2301.00001",
		"http://arxiv.org/abs/2301.00001",
		"https://export.arxiv.org/abs/2301.00001",
	}

	for _, url := range validURLs {
		if !isArxivURL(url) {
			t.Errorf("Expected %s to be recognized as arXiv URL", url)
		}
	}

	// Test invalid URLs
	invalidURLs := []string{
		"https://google.com",
		"ftp://arxiv.org/abs/2301.00001",
		"arxiv.org/abs/2301.00001",
	}

	for _, url := range invalidURLs {
		if isArxivURL(url) {
			t.Errorf("Expected %s to NOT be recognized as arXiv URL", url)
		}
	}
}

func TestIsArxivID(t *testing.T) {
	// Test valid IDs
	validIDs := []string{
		"2301.00001",
		"2301.12345",
		"hep-th/9901001",
	}

	for _, id := range validIDs {
		if !isArxivID(id) {
			t.Errorf("Expected %s to be recognized as arXiv ID", id)
		}
	}

	// Test invalid IDs
	invalidIDs := []string{
		"12345",
		"2301.",
		"hep-th/123",
	}

	for _, id := range invalidIDs {
		if isArxivID(id) {
			t.Errorf("Expected %s to NOT be recognized as arXiv ID", id)
		}
	}
}

func TestIsLocalZip(t *testing.T) {
	// Test valid zip paths
	validPaths := []string{
		"paper.zip",
		"/path/to/paper.zip",
		"paper.ZIP",
	}

	for _, path := range validPaths {
		if !isLocalZip(path) {
			t.Errorf("Expected %s to be recognized as local zip", path)
		}
	}

	// Test invalid paths
	invalidPaths := []string{
		"paper.pdf",
		"paper.tar.gz",
		"paper",
	}

	for _, path := range invalidPaths {
		if isLocalZip(path) {
			t.Errorf("Expected %s to NOT be recognized as local zip", path)
		}
	}
}
