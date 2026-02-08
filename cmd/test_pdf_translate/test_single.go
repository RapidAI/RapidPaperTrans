// +build ignore

// Test program for PDF translation with single-test.pdf
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
)

func main() {
	inputPDF := "testdata/single-test.pdf"
	
	if _, err := os.Stat(inputPDF); err != nil {
		fmt.Printf("Test PDF not found: %s\n", inputPDF)
		os.Exit(1)
	}

	fmt.Printf("Testing PDF translation with: %s\n", inputPDF)

	// Create output directory
	outputDir := "testdata/output"
	os.MkdirAll(outputDir, 0755)

	// Create BabelDoc translator
	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: outputDir,
	})

	// Step 1: Extract text blocks
	fmt.Println("\n=== Step 1: Extracting text blocks ===")
	blocks, err := translator.ExtractBlocks(inputPDF)
	if err != nil {
		fmt.Printf("Error extracting blocks: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Extracted %d blocks\n", len(blocks))

	// Print all blocks with details
	fmt.Println("\nAll blocks:")
	for i, block := range blocks {
		fmt.Printf("  [%d] Page=%d, Type=%s\n", i, block.Page, block.BlockType)
		fmt.Printf("      Pos=(%.1f, %.1f), Size=(%.1f x %.1f), Font=%.1f\n",
			block.X, block.Y, block.Width, block.Height, block.FontSize)
		text := block.Text
		if len(text) > 100 {
			text = text[:100] + "..."
		}
		fmt.Printf("      Text: %s\n", text)
	}

	// Step 2: Create mock translations
	fmt.Println("\n=== Step 2: Creating mock translations ===")
	
	// Filter out non-translatable blocks
	translatableBlocks := pdf.FilterTranslatableBlocks(blocks)
	fmt.Printf("Filtered to %d translatable blocks (removed %d)\n", 
		len(translatableBlocks), len(blocks)-len(translatableBlocks))
	
	translations := make(map[string]string)
	for _, block := range translatableBlocks {
		// Add Chinese translation for testing
		translations[block.ID] = "【中文翻译】" + block.Text
	}
	// Keep formulas as-is
	for _, block := range blocks {
		if block.BlockType == "formula" {
			translations[block.ID] = block.Text
		}
	}

	// Step 3: Convert to translated blocks
	translatedBlocks := pdf.ConvertToTranslatedBlocks(blocks, translations)
	fmt.Printf("Created %d translated blocks\n", len(translatedBlocks))

	// Step 4: Generate translated PDF
	fmt.Println("\n=== Step 3: Generating translated PDF ===")
	outputPDF := filepath.Join(outputDir, "single_translated.pdf")
	
	err = translator.GenerateTranslatedPDF(inputPDF, translatedBlocks, outputPDF)
	if err != nil {
		fmt.Printf("Error generating PDF: %v\n", err)
		os.Exit(1)
	}

	// Verify output
	info, err := os.Stat(outputPDF)
	if err != nil {
		fmt.Printf("Error: output file not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSuccess! Generated: %s (%d bytes)\n", outputPDF, info.Size())
	fmt.Println("\nPlease open the PDF to verify Chinese text is displayed correctly.")
}
