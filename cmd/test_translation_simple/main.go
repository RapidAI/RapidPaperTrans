package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
)

func init() {
	logger.Init(&logger.Config{
		LogFilePath:   "test_translation_simple.log",
		Level:         logger.LevelDebug,
		EnableConsole: true,
	})
}

type Config struct {
	OpenAIAPIKey  string `json:"openai_api_key"`
	OpenAIBaseURL string `json:"openai_base_url"`
	OpenAIModel   string `json:"openai_model"`
}

func loadConfig() (*Config, error) {
	homeDir, _ := os.UserHomeDir()
	configPaths := []string{
		"latex-translator-config.json",
		filepath.Join(homeDir, "latex-translator-config.json"),
	}

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err == nil {
				return &cfg, nil
			}
		}
	}
	return &Config{}, nil
}

func main() {
	// Load config
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	apiKey := cfg.OpenAIAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		fmt.Println("❌ No API key configured")
		fmt.Println("Please set OPENAI_API_KEY or configure in latex-translator-config.json")
		os.Exit(1)
	}

	baseURL := cfg.OpenAIBaseURL
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	model := cfg.OpenAIModel
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	fmt.Printf("=== Simple Translation Test ===\n")
	fmt.Printf("Model: %s\n", model)
	if baseURL != "" {
		fmt.Printf("Base URL: %s\n", baseURL)
	}
	fmt.Println()

	// Test content with actual text and comments
	testContent := `\documentclass{article}

% This is a test document
\usepackage{amsmath}

\begin{document}

\section{Introduction}

This is a simple test to verify that translation works correctly.
We want to see if the text is translated to Chinese.

% This comment should also be translated
\subsection{Background}

Machine learning is a field of artificial intelligence.
It enables computers to learn from data.

\end{document}`

	fmt.Println("Original content:")
	fmt.Println("---")
	fmt.Println(testContent)
	fmt.Println("---")
	fmt.Println()

	// Translate
	fmt.Println("Translating...")
	engine := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, 1)
	result, err := engine.TranslateTeX(testContent)

	if err != nil {
		fmt.Printf("❌ Translation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Translation completed!")
	fmt.Printf("Tokens used: %d\n", result.TokensUsed)
	fmt.Println()
	fmt.Println("Translated content:")
	fmt.Println("---")
	fmt.Println(result.TranslatedContent)
	fmt.Println("---")
	fmt.Println()

	// Count Chinese characters
	chineseCount := 0
	for _, r := range result.TranslatedContent {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}

	fmt.Printf("Chinese character count: %d\n", chineseCount)
	if chineseCount > 0 {
		fmt.Println("✅ Translation successful - Chinese characters found!")
	} else {
		fmt.Println("❌ Translation failed - No Chinese characters found!")
		fmt.Println("\nThis indicates the API is not translating to Chinese.")
		fmt.Println("Possible causes:")
		fmt.Println("  1. The model doesn't support Chinese translation")
		fmt.Println("  2. The API key doesn't have proper permissions")
		fmt.Println("  3. The prompt is not being followed correctly")
	}
}
