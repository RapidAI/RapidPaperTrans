// Command babeldoc_test tests the BabelDOC-style PDF translation
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
)

func main() {
	inputPath := "testdata/single-test.pdf"
	outputDir := "testdata/output"
	outputPath := filepath.Join(outputDir, "single-test_translated.pdf")

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Create translator
	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: outputDir,
	})

	// Step 1: Extract blocks
	fmt.Println("=== Step 1: Extracting text blocks ===")
	blocks, err := translator.ExtractBlocks(inputPath)
	if err != nil {
		fmt.Printf("Error extracting blocks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Extracted %d blocks\n\n", len(blocks))

	// Print all blocks
	for i, block := range blocks {
		fmt.Printf("Block %d:\n", i+1)
		fmt.Printf("  ID: %s\n", block.ID)
		fmt.Printf("  Page: %d\n", block.Page)
		fmt.Printf("  Type: %s\n", block.BlockType)
		fmt.Printf("  Position: (%.1f, %.1f)\n", block.X, block.Y)
		fmt.Printf("  Size: %.1f x %.1f\n", block.Width, block.Height)
		fmt.Printf("  FontSize: %.1f\n", block.FontSize)
		fmt.Printf("  Text: %s\n", truncate(block.Text, 100))
		fmt.Println()
	}

	// Step 2: Create mock translations (for testing without API)
	fmt.Println("=== Step 2: Creating mock translations ===")
	translatedBlocks := make([]pdf.TranslatedBabelDocBlock, len(blocks))
	for i, block := range blocks {
		translatedBlocks[i] = pdf.TranslatedBabelDocBlock{
			BabelDocBlock: block,
		}

		// Skip formulas
		if block.BlockType == "formula" {
			translatedBlocks[i].TranslatedText = block.Text
			fmt.Printf("Block %d: [FORMULA - KEPT ORIGINAL]\n", i+1)
			continue
		}

		// Mock translation: add Chinese prefix
		translatedBlocks[i].TranslatedText = mockTranslate(block.Text)
		fmt.Printf("Block %d: %s -> %s\n", i+1, 
			truncate(block.Text, 30), 
			truncate(translatedBlocks[i].TranslatedText, 40))
	}
	fmt.Println()

	// Step 3: Generate translated PDF
	fmt.Println("=== Step 3: Generating translated PDF ===")
	err = translator.GenerateTranslatedPDF(inputPath, translatedBlocks, outputPath)
	if err != nil {
		fmt.Printf("Error generating PDF: %v\n", err)
		os.Exit(1)
	}

	// Verify output
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		fmt.Printf("Error: output file not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Result ===\n")
	fmt.Printf("Input:  %s\n", inputPath)
	fmt.Printf("Output: %s\n", outputPath)
	fmt.Printf("Size:   %d bytes\n", fileInfo.Size())
	fmt.Println("\nTranslation complete! Please check the output PDF.")
}

// mockTranslate creates a mock translation for testing
func mockTranslate(text string) string {
	// Simple mock: add Chinese prefix based on content type
	if len(text) < 20 {
		return "[标题] " + text
	}
	if len(text) < 50 {
		return "[短文] " + text
	}
	return "[翻译] " + text
}

// truncate truncates a string to maxLen
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
