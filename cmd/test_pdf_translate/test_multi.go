//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
	"latex-translator/internal/python"
)

func main() {
	// Test PDF translation with multi-test.pdf
	inputPDF := "testdata/multi-test.pdf"
	outputPDF := "testdata/output/multi_translated.pdf"

	// Check input exists
	if _, err := os.Stat(inputPDF); err != nil {
		fmt.Printf("Input PDF not found: %s\n", inputPDF)
		os.Exit(1)
	}

	// Ensure output directory exists
	os.MkdirAll(filepath.Dir(outputPDF), 0755)

	// Setup Python environment
	fmt.Println("Setting up Python environment...")
	if err := python.EnsureGlobalEnv(func(msg string) {
		fmt.Println("  ", msg)
	}); err != nil {
		fmt.Printf("Failed to setup Python: %v\n", err)
		os.Exit(1)
	}

	// Create translator
	workDir, _ := os.MkdirTemp("", "pdf-translate-test-*")
	defer os.RemoveAll(workDir)

	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: workDir,
	})

	// API config - using DeepSeek
	apiKey := "sk-b702fe39f0c746c08e6e3efc904b88f3"
	baseURL := "https://api.deepseek.com/v1"
	model := "deepseek-chat"

	fmt.Printf("\nTranslating: %s\n", inputPDF)
	fmt.Printf("Output: %s\n", outputPDF)
	fmt.Printf("Model: %s\n\n", model)

	// Translate with progress callback
	err := translator.TranslatePDFWithPyMuPDFProgressive(
		inputPDF,
		outputPDF,
		apiKey,
		baseURL,
		model,
		func(msg string) {
			fmt.Printf("Progress: %s\n", msg)
		},
		func(currentPage, totalPages int, outPath string) {
			fmt.Printf("Page %d/%d completed\n", currentPage, totalPages)
		},
	)

	if err != nil {
		fmt.Printf("\nTranslation failed: %v\n", err)
		os.Exit(1)
	}

	// Check output
	if info, err := os.Stat(outputPDF); err != nil {
		fmt.Printf("\nOutput PDF not created!\n")
		os.Exit(1)
	} else {
		fmt.Printf("\nSuccess! Output: %s (%.2f KB)\n", outputPDF, float64(info.Size())/1024)
	}
}
