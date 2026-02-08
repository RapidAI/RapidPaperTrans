package main

import (
	"fmt"
	"os"

	"latex-translator/internal/pdf"
	"latex-translator/internal/python"
)

func main() {
	// Test PDF file
	inputPDF := "testdata/2504.15895/original.pdf"
	outputPDF := "testdata/output/pymupdf_test.pdf"

	// Check input exists
	if _, err := os.Stat(inputPDF); err != nil {
		fmt.Printf("Input PDF not found: %s\n", inputPDF)
		os.Exit(1)
	}

	fmt.Println("=== Testing PyMuPDF Translation ===")
	fmt.Printf("Input: %s\n", inputPDF)
	fmt.Printf("Output: %s\n", outputPDF)

	// First ensure Python environment is ready
	fmt.Println("\n1. Setting up Python environment...")
	err := python.EnsureGlobalEnv(func(msg string) {
		fmt.Println("  " + msg)
	})
	if err != nil {
		fmt.Printf("Failed to setup Python: %v\n", err)
		os.Exit(1)
	}

	// Create translator
	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: "testdata/output",
	})

	// Get API config from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")

	if apiKey == "" {
		fmt.Println("\nError: OPENAI_API_KEY environment variable not set")
		fmt.Println("Please set it before running this test")
		os.Exit(1)
	}

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4"
	}

	fmt.Printf("\n2. API Config:\n")
	fmt.Printf("  Base URL: %s\n", baseURL)
	fmt.Printf("  Model: %s\n", model)

	// Run translation
	fmt.Println("\n3. Starting PyMuPDF translation...")
	err = translator.TranslatePDFWithPyMuPDF(
		inputPDF,
		outputPDF,
		apiKey,
		baseURL,
		model,
		func(msg string) {
			fmt.Println("  " + msg)
		},
	)

	if err != nil {
		fmt.Printf("\nTranslation failed: %v\n", err)
		os.Exit(1)
	}

	// Check output
	if info, err := os.Stat(outputPDF); err == nil {
		fmt.Printf("\n=== Success ===\n")
		fmt.Printf("Output file: %s\n", outputPDF)
		fmt.Printf("File size: %d bytes\n", info.Size())
	} else {
		fmt.Printf("\nOutput file not created\n")
		os.Exit(1)
	}
}
