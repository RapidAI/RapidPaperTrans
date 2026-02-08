// Translate only first few pages for quick testing
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: translate_first_pages <input.pdf> [max_pages]")
		os.Exit(1)
	}

	inputPDF := os.Args[1]
	maxPages := 2 // Default to first 2 pages
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &maxPages)
	}

	// Load config
	type Config struct {
		OpenAIAPIKey  string `json:"openai_api_key"`
		OpenAIBaseURL string `json:"openai_base_url"`
		OpenAIModel   string `json:"openai_model"`
	}
	
	cfg := &Config{}
	configPath := "latex-translator-config.json"
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, cfg)
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAIAPIKey = key
	}
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		cfg.OpenAIBaseURL = url
	}
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		cfg.OpenAIModel = model
	}

	if cfg.OpenAIBaseURL == "" {
		cfg.OpenAIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.OpenAIModel == "" {
		cfg.OpenAIModel = "gpt-4"
	}

	if cfg.OpenAIAPIKey == "" {
		fmt.Println("Error: OpenAI API key not configured")
		os.Exit(1)
	}

	outputDir := filepath.Join(filepath.Dir(inputPDF), "output")
	os.MkdirAll(outputDir, 0755)
	
	// Extract first N pages to a temp PDF
	baseName := filepath.Base(inputPDF)
	tempPDF := filepath.Join(outputDir, "temp_first_pages.pdf")
	outputPDF := filepath.Join(outputDir, baseName[:len(baseName)-4]+"_first_pages.pdf")

	fmt.Printf("Input:     %s\n", inputPDF)
	fmt.Printf("Max pages: %d\n", maxPages)
	fmt.Println("\nExtracting first pages...")

	// Use Python to extract first pages
	extractScript := filepath.Join(outputDir, "extract_pages.py")
	scriptContent := fmt.Sprintf(`#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import sys
try:
    import pymupdf
except ImportError:
    import fitz as pymupdf

doc = pymupdf.open(r'%s')
new_doc = pymupdf.open()
for i in range(min(%d, len(doc))):
    new_doc.insert_pdf(doc, from_page=i, to_page=i)
new_doc.save(r'%s')
new_doc.close()
doc.close()
print(f'Extracted {{min(%d, len(doc))}} pages')
`, inputPDF, maxPages, tempPDF, maxPages)

	if err := os.WriteFile(extractScript, []byte(scriptContent), 0644); err != nil {
		fmt.Printf("Error writing script: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command("python", extractScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error extracting pages: %v\n%s\n", err, output)
		os.Exit(1)
	}
	fmt.Println(string(output))

	// Now translate the temp PDF
	fmt.Printf("API:       %s\n", cfg.OpenAIBaseURL)
	fmt.Printf("Model:     %s\n", cfg.OpenAIModel)
	fmt.Println("\nTranslating...")

	// Set environment variables for translate_pdf.exe
	os.Setenv("OPENAI_API_KEY", cfg.OpenAIAPIKey)
	os.Setenv("OPENAI_BASE_URL", cfg.OpenAIBaseURL)
	os.Setenv("OPENAI_MODEL", cfg.OpenAIModel)

	translateCmd := exec.Command("./translate_pdf.exe", tempPDF)
	translateCmd.Stdout = os.Stdout
	translateCmd.Stderr = os.Stderr
	
	if err := translateCmd.Run(); err != nil {
		fmt.Printf("Error translating: %v\n", err)
		os.Exit(1)
	}

	// Rename output
	tempOutput := filepath.Join(outputDir, "temp_first_pages_translated.pdf")
	if _, err := os.Stat(tempOutput); err == nil {
		os.Rename(tempOutput, outputPDF)
		os.Remove(tempPDF) // Clean up temp file
		
		fmt.Printf("\n=== Translation Complete ===\n")
		fmt.Printf("Output: %s\n", outputPDF)
		fmt.Println("\n请打开PDF文件检查：")
		fmt.Println("1. 字体大小是否合适（应该≥12pt）")
		fmt.Println("2. 文字是否水平显示（不是竖向）")
		fmt.Println("3. 中文是否清晰可读")
	}
}
