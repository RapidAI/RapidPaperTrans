//go:build mupdf && cgo

// Test program for MuPDF-based PDF translation
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
	"latex-translator/internal/types"
)

func main() {
	// Get API configuration
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")

	if apiKey == "" {
		fmt.Println("Error: OPENAI_API_KEY not set")
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

	// Input/output paths
	inputPDF := "testdata/single-test.pdf"
	outputDir := "testdata/output"
	os.MkdirAll(outputDir, 0755)
	outputPDF := filepath.Join(outputDir, "mupdf_translated.pdf")

	// Create translator
	translator := pdf.NewMuPDFTranslator(pdf.MuPDFTranslatorConfig{
		Config:    cfg,
		WorkDir:   outputDir,
		CachePath: filepath.Join(outputDir, "mupdf_cache.json"),
	})
	defer translator.Close()

	// Translate
	fmt.Println("\nStarting MuPDF translation...")
	result, err := translator.TranslatePDF(inputPDF, outputPDF, func(phase string, progress int) {
		fmt.Printf("\r[%s] Progress: %d%%   ", phase, progress)
	})

	fmt.Println()

	if err != nil {
		fmt.Printf("Translation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Translation Complete ===\n")
	fmt.Printf("  Total blocks: %d\n", result.TotalBlocks)
	fmt.Printf("  Translated: %d\n", result.TranslatedBlocks)
	fmt.Printf("  From cache: %d\n", result.CachedBlocks)
	fmt.Printf("  Formulas: %d\n", result.FormulaBlocks)
	fmt.Printf("  Output: %s\n", result.OutputPath)
}
