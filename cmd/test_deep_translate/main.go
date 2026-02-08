// Test program for deep-representation-learning-book translation (full flow)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/downloader"
	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
)

func init() {
	logger.Init(&logger.Config{
		LogFilePath:   "test_deep_translate.log",
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
	zipPath := "testdata/deep-representation-learning-book-main.zip"
	workDir := filepath.Join("testdata", "deep_translate_test")

	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Printf("Failed to create work directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Testing Deep Learning Book Translation (Full Flow) ===\n")
	fmt.Printf("Zip file: %s\n", zipPath)
	fmt.Printf("Work directory: %s\n", workDir)

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
		fmt.Println("No API key configured. Set OPENAI_API_KEY or configure in latex-translator-config.json")
		fmt.Println("Skipping translation test.")
		os.Exit(0)
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

	fmt.Printf("Using model: %s\n", model)
	if baseURL != "" {
		fmt.Printf("Using base URL: %s\n", baseURL)
	}

	// Step 1: Extract
	fmt.Println("\n=== Step 1: Extract ===")
	dl := downloader.NewSourceDownloader(workDir)
	sourceInfo, err := dl.ExtractZip(zipPath)
	if err != nil {
		fmt.Printf("Extract failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Extracted to: %s\n", sourceInfo.ExtractDir)
	fmt.Printf("Found %d tex files\n", len(sourceInfo.AllTexFiles))

	// Step 2: Find main tex file
	fmt.Println("\n=== Step 2: Find Main TeX File ===")
	mainTexFile, err := dl.FindMainTexFile(sourceInfo.ExtractDir)
	if err != nil {
		fmt.Printf("Failed to find main tex file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Main tex file: %s\n", mainTexFile)

	mainTexPath := filepath.Join(sourceInfo.ExtractDir, mainTexFile)

	// Step 3: Read main file
	fmt.Println("\n=== Step 3: Read Main File ===")
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		fmt.Printf("Failed to read main tex file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("File size: %d bytes\n", len(content))
	fmt.Printf("Main file path: %s\n", mainTexPath)
	fmt.Printf("Extract dir: %s\n", sourceInfo.ExtractDir)

	// Step 4: Test translation (just first 1000 chars to verify path handling)
	fmt.Println("\n=== Step 4: Test Translation ===")
	fmt.Println("Testing with first 1000 characters only...")
	
	testContent := string(content)
	if len(testContent) > 1000 {
		testContent = testContent[:1000]
	}

	engine := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, 3)
	result, err := engine.TranslateTeXWithProgress(testContent, func(current, total int, message string) {
		fmt.Printf("\r  Progress: %d/%d - %s", current, total, message)
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("Translation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Translation completed! Tokens used: %d\n", result.TokensUsed)
	fmt.Printf("Translated content length: %d bytes\n", len(result.TranslatedContent))

	// Step 5: Save translated file
	fmt.Println("\n=== Step 5: Save Translated File ===")
	translatedTexPath := filepath.Join(filepath.Dir(mainTexPath), "translated_"+filepath.Base(mainTexPath))
	if err := os.WriteFile(translatedTexPath, []byte(result.TranslatedContent), 0644); err != nil {
		fmt.Printf("Failed to save translated file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved to: %s\n", translatedTexPath)

	fmt.Println("\n=== Test Complete ===")
	fmt.Printf("Extract directory: %s\n", sourceInfo.ExtractDir)
	fmt.Println("\nNote: This test only translated the first 1000 characters to verify path handling.")
	fmt.Println("For full translation, use the main application.")
}
