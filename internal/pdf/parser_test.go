package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetPDFInfo_NonExistentFile tests that GetPDFInfo returns an error for non-existent files
func TestGetPDFInfo_NonExistentFile(t *testing.T) {
	parser := NewPDFParser("")
	_, err := parser.GetPDFInfo("/non/existent/file.pdf")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFNotFound {
		t.Errorf("Expected error code %s, got %s", ErrPDFNotFound, pdfErr.Code)
	}
}

// TestGetPDFInfo_Directory tests that GetPDFInfo returns an error when path is a directory
func TestGetPDFInfo_Directory(t *testing.T) {
	parser := NewPDFParser("")
	// Use the current directory as test input
	_, err := parser.GetPDFInfo(".")
	if err == nil {
		t.Error("Expected error for directory path, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFInvalid {
		t.Errorf("Expected error code %s, got %s", ErrPDFInvalid, pdfErr.Code)
	}
}

// TestGetPDFInfo_InvalidFile tests that GetPDFInfo returns an error for invalid PDF files
func TestGetPDFInfo_InvalidFile(t *testing.T) {
	// Create a temporary non-PDF file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.pdf")
	err := os.WriteFile(tmpFile, []byte("This is not a PDF file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	parser := NewPDFParser("")
	_, err = parser.GetPDFInfo(tmpFile)
	if err == nil {
		t.Error("Expected error for invalid PDF file, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFInvalid {
		t.Errorf("Expected error code %s, got %s", ErrPDFInvalid, pdfErr.Code)
	}
}

// TestIsTextPDF_NonExistentFile tests that IsTextPDF returns an error for non-existent files
func TestIsTextPDF_NonExistentFile(t *testing.T) {
	parser := NewPDFParser("")
	_, err := parser.IsTextPDF("/non/existent/file.pdf")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFNotFound {
		t.Errorf("Expected error code %s, got %s", ErrPDFNotFound, pdfErr.Code)
	}
}

// TestIsTextPDF_InvalidFile tests that IsTextPDF returns an error for invalid PDF files
func TestIsTextPDF_InvalidFile(t *testing.T) {
	// Create a temporary non-PDF file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.pdf")
	err := os.WriteFile(tmpFile, []byte("This is not a PDF file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	parser := NewPDFParser("")
	_, err = parser.IsTextPDF(tmpFile)
	if err == nil {
		t.Error("Expected error for invalid PDF file, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFInvalid {
		t.Errorf("Expected error code %s, got %s", ErrPDFInvalid, pdfErr.Code)
	}
}

// TestNewPDFParser tests the PDFParser constructor
func TestNewPDFParser(t *testing.T) {
	workDir := "/test/work/dir"
	parser := NewPDFParser(workDir)
	if parser == nil {
		t.Error("Expected non-nil parser")
	}
	if parser.workDir != workDir {
		t.Errorf("Expected workDir %s, got %s", workDir, parser.workDir)
	}
}


// TestExtractText_NonExistentFile tests that ExtractText returns an error for non-existent files
func TestExtractText_NonExistentFile(t *testing.T) {
	parser := NewPDFParser("")
	_, err := parser.ExtractText("/non/existent/file.pdf")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFNotFound {
		t.Errorf("Expected error code %s, got %s", ErrPDFNotFound, pdfErr.Code)
	}
}

// TestExtractText_InvalidFile tests that ExtractText returns an error for invalid PDF files
func TestExtractText_InvalidFile(t *testing.T) {
	// Create a temporary non-PDF file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.pdf")
	err := os.WriteFile(tmpFile, []byte("This is not a PDF file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	parser := NewPDFParser("")
	_, err = parser.ExtractText(tmpFile)
	if err == nil {
		t.Error("Expected error for invalid PDF file, got nil")
	}

	pdfErr, ok := err.(*PDFError)
	if !ok {
		t.Errorf("Expected PDFError, got %T", err)
	}
	if pdfErr.Code != ErrPDFInvalid {
		t.Errorf("Expected error code %s, got %s", ErrPDFInvalid, pdfErr.Code)
	}
}

// TestDetermineBlockType tests the block type determination logic
func TestDetermineBlockType(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		fontSize float64
		isBold   bool
		expected string
	}{
		{
			name:     "numbered heading",
			text:     "1. Introduction",
			fontSize: 14,
			isBold:   true,
			expected: "heading",
		},
		{
			name:     "chapter heading",
			text:     "Chapter 1: Getting Started",
			fontSize: 16,
			isBold:   true,
			expected: "heading",
		},
		{
			name:     "section heading",
			text:     "Section 2.1 Methods",
			fontSize: 14,
			isBold:   false,
			expected: "heading",
		},
		{
			name:     "figure caption",
			text:     "Figure 1: System Architecture",
			fontSize: 10,
			isBold:   false,
			expected: "caption",
		},
		{
			name:     "table caption",
			text:     "Table 2: Results Summary",
			fontSize: 10,
			isBold:   false,
			expected: "caption",
		},
		{
			name:     "regular paragraph",
			text:     "This is a regular paragraph with some text that describes the methodology used in this study.",
			fontSize: 11,
			isBold:   false,
			expected: "paragraph",
		},
		{
			name:     "abstract heading",
			text:     "Abstract",
			fontSize: 14,
			isBold:   true,
			expected: "heading",
		},
		{
			name:     "references heading",
			text:     "References",
			fontSize: 14,
			isBold:   true,
			expected: "heading",
		},
		{
			name:     "empty text",
			text:     "",
			fontSize: 11,
			isBold:   false,
			expected: "paragraph",
		},
		{
			name:     "whitespace only",
			text:     "   ",
			fontSize: 11,
			isBold:   false,
			expected: "paragraph",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineBlockType(tt.text, tt.fontSize, tt.isBold)
			if result != tt.expected {
				t.Errorf("determineBlockType(%q, %f, %v) = %q, want %q",
					tt.text, tt.fontSize, tt.isBold, result, tt.expected)
			}
		})
	}
}

// TestIsNumberedHeading tests the numbered heading detection
func TestIsNumberedHeading(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"1. Introduction", true},
		{"1.1 Background", true},
		{"1.1.1 Details", true},
		{"Chapter 1", true},
		{"Section 2", true},
		{"Appendix A", true},
		{"Abstract", true},
		{"Introduction", true},
		{"Conclusion", true},
		{"References", true},
		{"A. First Item", true},
		{"I. Roman Numeral", true},
		{"This is a regular sentence.", false},
		{"The quick brown fox jumps over the lazy dog.", false},
		{"", false},
		{"   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := isNumberedHeading(tt.text)
			if result != tt.expected {
				t.Errorf("isNumberedHeading(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestIsAllUpperCase tests the uppercase detection
func TestIsAllUpperCase(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"ABSTRACT", true},
		{"INTRODUCTION", true},
		{"CHAPTER ONE", true},
		{"Abstract", false},
		{"introduction", false},
		{"Chapter One", false},
		{"123", false},
		{"", false},
		{"A", true},
		{"a", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := isAllUpperCase(tt.text)
			if result != tt.expected {
				t.Errorf("isAllUpperCase(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestIsListItem tests the list item detection
func TestIsListItem(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"• First item", true},
		{"- Second item", true},
		{"* Third item", true},
		{"● Fourth item", true},
		{"○ Fifth item", true},
		{"1) Numbered item", true},
		{"a) Lettered item", true},
		{"(1) Parenthesized number", true},
		{"(a) Parenthesized letter", true},
		{"1. Numbered with dot", true},
		{"a. Lettered with dot", true},
		{"Regular text without bullet", false},
		{"", false},
		{"A", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := isListItem(tt.text)
			if result != tt.expected {
				t.Errorf("isListItem(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestTextBlockValidation tests that TextBlock validation works correctly
func TestTextBlockValidation(t *testing.T) {
	tests := []struct {
		name     string
		block    TextBlock
		expected bool
	}{
		{
			name: "valid block",
			block: TextBlock{
				ID:        "block_1",
				Page:      1,
				Text:      "Hello World",
				X:         10.0,
				Y:         20.0,
				Width:     100.0,
				Height:    15.0,
				FontSize:  12.0,
				FontName:  "Helvetica",
				BlockType: "paragraph",
			},
			expected: true,
		},
		{
			name: "invalid - negative X",
			block: TextBlock{
				ID:        "block_2",
				Page:      1,
				Text:      "Hello",
				X:         -10.0,
				Y:         20.0,
				Width:     100.0,
				Height:    15.0,
				BlockType: "paragraph",
			},
			expected: false,
		},
		{
			name: "invalid - zero page",
			block: TextBlock{
				ID:        "block_3",
				Page:      0,
				Text:      "Hello",
				X:         10.0,
				Y:         20.0,
				Width:     100.0,
				Height:    15.0,
				BlockType: "paragraph",
			},
			expected: false,
		},
		{
			name: "invalid - empty text",
			block: TextBlock{
				ID:        "block_4",
				Page:      1,
				Text:      "",
				X:         10.0,
				Y:         20.0,
				Width:     100.0,
				Height:    15.0,
				BlockType: "paragraph",
			},
			expected: false,
		},
		{
			name: "invalid - empty block type",
			block: TextBlock{
				ID:        "block_5",
				Page:      1,
				Text:      "Hello",
				X:         10.0,
				Y:         20.0,
				Width:     100.0,
				Height:    15.0,
				BlockType: "",
			},
			expected: false,
		},
		{
			name: "valid - zero width and height",
			block: TextBlock{
				ID:        "block_6",
				Page:      1,
				Text:      "Hello",
				X:         10.0,
				Y:         20.0,
				Width:     0.0,
				Height:    0.0,
				BlockType: "paragraph",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.block.IsValidTextBlock()
			if result != tt.expected {
				t.Errorf("IsValidTextBlock() = %v, want %v for block %+v", result, tt.expected, tt.block)
			}
		})
	}
}

// TestSpecialCharacterPreservation tests that special characters are preserved
func TestSpecialCharacterPreservation(t *testing.T) {
	// Test that the determineBlockType function doesn't modify special characters
	specialTexts := []string{
		"α + β = γ",
		"∫f(x)dx",
		"∑_{i=1}^{n} x_i",
		"√(x² + y²)",
		"∞ → ∅",
		"≤ ≥ ≠ ≈",
		"∈ ∉ ⊂ ⊃",
		"∧ ∨ ¬ ⇒",
		"π ≈ 3.14159",
		"E = mc²",
	}

	for _, text := range specialTexts {
		t.Run(text, func(t *testing.T) {
			// The function should not panic or modify the text
			blockType := determineBlockType(text, 11.0, false)
			if blockType == "" {
				t.Errorf("determineBlockType returned empty string for special text: %q", text)
			}
		})
	}
}


// TestIsPostScriptCode tests the PostScript code detection
func TestIsPostScriptCode(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "PostScript with def",
			text:     "/burl@stx null def /BU.S /burl@stx null def def",
			expected: true,
		},
		{
			name:     "PostScript with currentpoint",
			text:     "/BU.SS currentpoint /burl@",
			expected: true,
		},
		{
			name:     "PostScript with multiple operators",
			text:     "gsave newpath moveto lineto stroke grestore",
			expected: true,
		},
		{
			name:     "PostScript with @stx marker",
			text:     "some text @stx more text",
			expected: true,
		},
		{
			name:     "PostScript with many slashes",
			text:     "/Name1 /Name2 /Name3 /Name4 /Name5",
			expected: true,
		},
		{
			name:     "PostScript with null def",
			text:     "/something null def",
			expected: true,
		},
		{
			name:     "PostScript with burl",
			text:     "/burl@stx some content",
			expected: true,
		},
		{
			name:     "Real garbage from PDF",
			text:     "/burl@stx null def /BU.S /burl@stx null def def /BU.SS currentpoint /burl@l",
			expected: true,
		},
		{
			name:     "Normal English text",
			text:     "This is a normal paragraph of text.",
			expected: false,
		},
		{
			name:     "Normal Chinese text",
			text:     "这是一段正常的中文文本。",
			expected: false,
		},
		{
			name:     "Math formula",
			text:     "E = mc² where m is mass",
			expected: false,
		},
		{
			name:     "Abstract heading",
			text:     "Abstract",
			expected: false,
		},
		{
			name:     "Section heading",
			text:     "1. Introduction",
			expected: false,
		},
		{
			name:     "Empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "URL with slashes",
			text:     "Visit https://example.com/path/to/page for more info",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPostScriptCode(tt.text)
			if result != tt.expected {
				t.Errorf("isPostScriptCode(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestHasExcessiveNonPrintable tests the non-printable character detection
func TestHasExcessiveNonPrintable(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "Normal text",
			text:     "This is normal text.",
			expected: false,
		},
		{
			name:     "Text with newlines",
			text:     "Line 1\nLine 2\nLine 3",
			expected: false,
		},
		{
			name:     "Text with tabs",
			text:     "Column1\tColumn2\tColumn3",
			expected: false,
		},
		{
			name:     "Empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "Text with control characters",
			text:     "Text\x00\x01\x02\x03\x04\x05more",
			expected: true,
		},
		{
			name:     "Chinese text",
			text:     "这是中文文本",
			expected: false,
		},
		{
			name:     "Mixed content",
			text:     "Hello 你好 World 世界",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasExcessiveNonPrintable(tt.text)
			if result != tt.expected {
				t.Errorf("hasExcessiveNonPrintable(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}
