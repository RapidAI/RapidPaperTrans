// +build ignore

// Test program for PDF translation with real API
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
	"latex-translator/internal/types"
)

func main() {
	// Get API configuration from environment variables
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")
	
	if apiKey == "" {
		fmt.Println("Error: OPENAI_API_KEY environment variable not set")
		os.Exit(1)
	}
	
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4"
	}
	
	cfg := &types.Config{
		OpenAIAPIKey:  apiKey,
		OpenAIBaseURL: baseURL,
		OpenAIModel:   model,
		ContextWindow: 8192,
		Concurrency:   3,
	}
	
	fmt.Printf("Using API: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("API Key: %s...%s\n", apiKey[:8], apiKey[len(apiKey)-4:])

	// Find test PDF - use single page for faster testing
	inputPDF := "testdata/single-test.pdf"
	if _, err := os.Stat(inputPDF); err != nil {
		fmt.Println("Test PDF not found")
		os.Exit(1)
	}

	fmt.Printf("\nTranslating: %s\n", inputPDF)

	// Create output directory
	outputDir := "testdata/output"
	os.MkdirAll(outputDir, 0755)
	outputPDF := filepath.Join(outputDir, "real_translated.pdf")

	// Create BabelDocPDFTranslator
	translator := pdf.NewBabelDocPDFTranslator(pdf.BabelDocPDFTranslatorConfig{
		Config:    cfg,
		WorkDir:   outputDir,
		CachePath: filepath.Join(outputDir, "translation_cache.json"),
	})
	defer translator.Close()

	// Translate with progress callback
	fmt.Println("\nStarting translation...")
	result, err := translator.TranslatePDF(inputPDF, outputPDF, func(phase string, progress int) {
		fmt.Printf("\r[%s] Progress: %d%%   ", phase, progress)
	})

	fmt.Println() // New line after progress

	if err != nil {
		fmt.Printf("\nTranslation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Translation Complete ===\n")
	fmt.Printf("  Input: %s\n", result.InputPath)
	fmt.Printf("  Output: %s\n", result.OutputPath)
	fmt.Printf("  Total blocks: %d\n", result.TotalBlocks)
	fmt.Printf("  Translated: %d\n", result.TranslatedBlocks)
	fmt.Printf("  From cache: %d\n", result.CachedBlocks)
	fmt.Printf("  Formulas (skipped): %d\n", result.FormulaBlocks)
	
	// Verify output
	if info, err := os.Stat(outputPDF); err == nil {
		fmt.Printf("  Output size: %d bytes\n", info.Size())
	}
	
	fmt.Println("\nPlease open the output PDF to verify the translation.")
}
