package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/pdf"
	"latex-translator/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: translate_single_pdf <input.pdf>")
		os.Exit(1)
	}

	inputPDF := os.Args[1]
	if _, err := os.Stat(inputPDF); err != nil {
		fmt.Printf("Error: PDF not found: %s\n", inputPDF)
		os.Exit(1)
	}

	// Load config from latex-translator-config.json
	cfg := &types.Config{}
	configPath := "latex-translator-config.json"
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, cfg)
	}

	// Override with environment variables if set
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAIAPIKey = key
	}
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		cfg.OpenAIBaseURL = url
	}
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		cfg.OpenAIModel = model
	}

	// Set defaults
	if cfg.OpenAIBaseURL == "" {
		cfg.OpenAIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.OpenAIModel == "" {
		cfg.OpenAIModel = "gpt-4"
	}
	if cfg.ContextWindow == 0 {
		cfg.ContextWindow = 8000
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 3
	}

	if cfg.OpenAIAPIKey == "" {
		fmt.Println("Error: OpenAI API key not configured")
		fmt.Println("Set OPENAI_API_KEY environment variable or configure in latex-translator-config.json")
		os.Exit(1)
	}

	// Create output path
	outputDir := filepath.Join(filepath.Dir(inputPDF), "output")
	os.MkdirAll(outputDir, 0755)
	
	baseName := filepath.Base(inputPDF)
	outputPDF := filepath.Join(outputDir, baseName[:len(baseName)-4]+"_translated.pdf")

	fmt.Printf("Input:  %s\n", inputPDF)
	fmt.Printf("Output: %s\n", outputPDF)
	fmt.Printf("API:    %s\n", cfg.OpenAIBaseURL)
	fmt.Printf("Model:  %s\n", cfg.OpenAIModel)
	fmt.Println()

	// Create translator
	translator := pdf.NewBabelDocPDFTranslator(pdf.BabelDocPDFTranslatorConfig{
		Config:  cfg,
		WorkDir: outputDir,
	})
	defer translator.Close()

	// Translate with progress
	result, err := translator.TranslatePDF(inputPDF, outputPDF, func(phase string, progress int) {
		fmt.Printf("\r[%3d%%] %s", progress, phase)
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Translation Complete ===\n")
	fmt.Printf("Total blocks:      %d\n", result.TotalBlocks)
	fmt.Printf("Translated:        %d\n", result.TranslatedBlocks)
	fmt.Printf("From cache:        %d\n", result.CachedBlocks)
	fmt.Printf("Formulas skipped:  %d\n", result.FormulaBlocks)
	fmt.Printf("Output:            %s\n", result.OutputPath)
}
