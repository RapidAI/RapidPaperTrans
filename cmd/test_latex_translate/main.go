// Test program for LaTeX translation with environment protection
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
)

func init() {
	// Initialize logger with console output
	logger.Init(&logger.Config{
		LogFilePath:   "test_translate.log",
		Level:         logger.LevelInfo,
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

func getAPIKey() string {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key
	}
	cfg, _ := loadConfig()
	return cfg.OpenAIAPIKey
}

func getBaseURL() string {
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		return url
	}
	cfg, _ := loadConfig()
	return cfg.OpenAIBaseURL
}

func getModel() string {
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		return model
	}
	cfg, _ := loadConfig()
	if cfg.OpenAIModel != "" {
		return cfg.OpenAIModel
	}
	return "gpt-4o-mini"
}

func main() {
	testDir := "testdata/2504.15895/latex"
	originalFile := filepath.Join(testDir, "arxiv.tex")
	outputFile := filepath.Join(testDir, "translated_arxiv_test.tex")

	if _, err := os.Stat(originalFile); err != nil {
		fmt.Printf("Original file not found: %s\n", originalFile)
		os.Exit(1)
	}

	content, err := os.ReadFile(originalFile)
	if err != nil {
		fmt.Printf("Failed to read file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== LaTeX Translation Test ===\n")
	fmt.Printf("Input file: %s\n", originalFile)
	fmt.Printf("Content size: %d bytes\n", len(content))

	fmt.Println("\n=== Test 1: Environment Boundary Detection ===")
	testEnvironmentDetection(string(content))

	fmt.Println("\n=== Test 2: Chunking Verification ===")
	fmt.Println("Run unit tests for accurate chunking verification:")
	fmt.Println("  go test ./internal/translator/... -v -run TestChunkingWithRealDocument")
	fmt.Println("The unit tests confirm that splitIntoChunks properly protects all environments.")

	apiKey := getAPIKey()
	if apiKey == "" {
		fmt.Println("\n=== Test 3: Full Translation (SKIPPED - no API key) ===")
		fmt.Println("Set OPENAI_API_KEY environment variable or configure latex-translator-config.json")
		return
	}

	fmt.Println("\n=== Test 3: Full Translation ===")
	testFullTranslation(string(content), apiKey, outputFile)
}

func testEnvironmentDetection(content string) {
	envCounts := map[string]int{
		"table":    strings.Count(content, "\\begin{table}") + strings.Count(content, "\\begin{table*}"),
		"tabular":  strings.Count(content, "\\begin{tabular}"),
		"figure":   strings.Count(content, "\\begin{figure}") + strings.Count(content, "\\begin{figure*}"),
		"equation": strings.Count(content, "\\begin{equation}"),
		"align":    strings.Count(content, "\\begin{align}"),
		"itemize":  strings.Count(content, "\\begin{itemize}"),
	}

	fmt.Println("Environment counts in original document:")
	for env, count := range envCounts {
		if count > 0 {
			fmt.Printf("  %s: %d\n", env, count)
		}
	}
}

func testFullTranslation(content string, apiKey string, outputFile string) {
	baseURL := getBaseURL()
	model := getModel()

	engine := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, 3)

	fmt.Println("Starting translation...")
	fmt.Printf("Using model: %s\n", model)
	if baseURL != "" {
		fmt.Printf("Using base URL: %s\n", baseURL)
	}

	result, err := engine.TranslateTeXWithProgress(content, func(current, total int, message string) {
		fmt.Printf("\r  Progress: %d/%d chunks - %s", current, total, message)
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("Translation failed: %v\n", err)
		return
	}

	fmt.Printf("Translation completed!\n")
	fmt.Printf("  Tokens used: %d\n", result.TokensUsed)
	fmt.Printf("  Original size: %d bytes\n", len(result.OriginalContent))
	fmt.Printf("  Translated size: %d bytes\n", len(result.TranslatedContent))

	if err := os.WriteFile(outputFile, []byte(result.TranslatedContent), 0644); err != nil {
		fmt.Printf("Failed to save output: %v\n", err)
		return
	}
	fmt.Printf("  Output saved to: %s\n", outputFile)

	fmt.Println("\nVerifying translated content...")
	verifyEnvironments(result.TranslatedContent)
}

func verifyEnvironments(content string) {
	envs := []string{"table", "table*", "tabular", "figure", "figure*", "equation", "align", "itemize", "enumerate"}

	allBalanced := true
	for _, env := range envs {
		begins := strings.Count(content, "\\begin{"+env+"}")
		ends := strings.Count(content, "\\end{"+env+"}")
		if begins != ends {
			fmt.Printf("  ✗ %s: begin=%d, end=%d (UNBALANCED)\n", env, begins, ends)
			allBalanced = false
		} else if begins > 0 {
			fmt.Printf("  ✓ %s: %d instances (balanced)\n", env, begins)
		}
	}

	if allBalanced {
		fmt.Println("\n✓ All environments are properly balanced!")
	} else {
		fmt.Println("\n✗ Some environments are unbalanced - translation may have issues")
	}
}
