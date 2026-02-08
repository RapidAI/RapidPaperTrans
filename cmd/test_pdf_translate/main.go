// Test program for PDF translation
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
	"latex-translator/internal/types"
)

func main() {
	// Find test PDF - prefer larger PDF for more comprehensive test
	testPDFs := []string{
		"testdata/2504.15895/original.pdf",
		"testdata/single-test.pdf",
	}

	var inputPDF string
	for _, p := range testPDFs {
		if _, err := os.Stat(p); err == nil {
			inputPDF = p
			break
		}
	}

	if inputPDF == "" {
		fmt.Println("No test PDF found")
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

	// Print first 10 blocks
	fmt.Println("\nFirst 10 blocks:")
	for i, block := range blocks {
		if i >= 10 {
			break
		}
		fmt.Printf("  [%d] Page=%d, Type=%s, Pos=(%.1f, %.1f), Size=(%.1f x %.1f), Font=%.1f\n",
			i, block.Page, block.BlockType, block.X, block.Y, block.Width, block.Height, block.FontSize)
		text := block.Text
		if len(text) > 80 {
			text = text[:80] + "..."
		}
		fmt.Printf("      Text: %s\n", text)
	}

	// Analyze block distribution
	fmt.Println("\nBlock distribution by page:")
	pageCount := make(map[int]int)
	for _, block := range blocks {
		pageCount[block.Page]++
	}
	for page := 1; page <= len(pageCount); page++ {
		if count, ok := pageCount[page]; ok {
			fmt.Printf("  Page %d: %d blocks\n", page, count)
		}
	}

	// Step 2: Filter translatable blocks
	fmt.Println("\n=== Step 2: Filtering translatable blocks ===")
	translatableBlocks := pdf.FilterTranslatableBlocks(blocks)
	fmt.Printf("Translatable blocks: %d (filtered out %d formulas/empty)\n",
		len(translatableBlocks), len(blocks)-len(translatableBlocks))

	// Step 3: Create mock translations (for testing without API)
	fmt.Println("\n=== Step 3: Creating mock translations ===")
	translations := make(map[string]string)
	for _, block := range blocks {
		if block.BlockType == "formula" {
			// Skip formulas - don't add translation
			continue
		}
		// Add Chinese prefix for testing
		translations[block.ID] = "[中文] " + block.Text
	}

	// Step 4: Convert to translated blocks
	translatedBlocks := pdf.ConvertToTranslatedBlocks(blocks, translations)
	fmt.Printf("Created %d translated blocks\n", len(translatedBlocks))

	// Step 5: Generate translated PDF
	fmt.Println("\n=== Step 4: Generating translated PDF ===")
	outputPDF := filepath.Join(outputDir, "translated_test.pdf")
	
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

	// Step 6: Check page count
	fmt.Println("\n=== Step 5: Checking page count ===")
	result, err := translator.CheckPageCountDifference(inputPDF, outputPDF)
	if err != nil {
		fmt.Printf("Warning: could not check page count: %v\n", err)
	} else {
		fmt.Printf("Original pages: %d\n", result.OriginalPages)
		fmt.Printf("Translated pages: %d\n", result.TranslatedPages)
		fmt.Printf("Difference: %.1f%%\n", result.DiffPercent*100)
		if result.IsSuspicious {
			fmt.Println("WARNING: Page count difference is suspicious!")
		}
	}

	// Step 7: Test with BabelDocPDFTranslator (high-level API)
	fmt.Println("\n=== Step 6: Testing BabelDocPDFTranslator ===")
	
	// Create config (without API key for mock test)
	config := &types.Config{
		OpenAIAPIKey:  "", // Empty for mock test
		OpenAIBaseURL: "",
		OpenAIModel:   "gpt-4",
		ContextWindow: 4000,
		Concurrency:   3,
	}

	babelTranslator := pdf.NewBabelDocPDFTranslator(pdf.BabelDocPDFTranslatorConfig{
		Config:  config,
		WorkDir: outputDir,
	})
	defer babelTranslator.Close()

	// Test extraction only (no API call)
	extractedBlocks, err := babelTranslator.ExtractBlocksOnly(inputPDF)
	if err != nil {
		fmt.Printf("Error in ExtractBlocksOnly: %v\n", err)
	} else {
		fmt.Printf("BabelDocPDFTranslator extracted %d blocks\n", len(extractedBlocks))
	}

	fmt.Println("\n=== Test Complete ===")
}
