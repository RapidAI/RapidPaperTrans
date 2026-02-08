// +build ignore

// Test translation of test_full_paper.pdf
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
	"latex-translator/internal/python"
)

func main() {
	// Check API key
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

	fmt.Printf("API: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)

	// Setup Python environment
	fmt.Println("\n=== Setting up Python environment ===")
	err := python.EnsureGlobalEnv(func(msg string) {
		fmt.Printf("  %s\n", msg)
	})
	if err != nil {
		fmt.Printf("Error setting up Python: %v\n", err)
		os.Exit(1)
	}

	// Input/output paths
	inputPDF := "testdata/test_full_paper.pdf"
	outputDir := "testdata/output"
	outputPDF := filepath.Join(outputDir, "full_paper_translated.pdf")

	os.MkdirAll(outputDir, 0755)

	// Remove old output if exists
	os.Remove(outputPDF)
	os.Remove(outputPDF + ".tmp")

	// Create translator with output dir as work dir (cache will be stored there)
	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: outputDir,
	})

	// Translate
	fmt.Println("\n=== Translating PDF ===")
	fmt.Printf("Input: %s\n", inputPDF)
	fmt.Printf("Output: %s\n", outputPDF)
	fmt.Printf("Cache: %s\n", filepath.Join(outputDir, "translation_cache.json"))

	err = translator.TranslatePDFWithPyMuPDF(inputPDF, outputPDF, apiKey, baseURL, model, func(msg string) {
		fmt.Printf("  %s\n", msg)
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Check output
	info, err := os.Stat(outputPDF)
	if err != nil {
		fmt.Printf("Error: output not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Success ===\n")
	fmt.Printf("Output: %s (%.2f MB)\n", outputPDF, float64(info.Size())/(1024*1024))
}
