// Test program for full translation of deep-representation-learning-book
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"latex-translator/internal/compiler"
	"latex-translator/internal/downloader"
	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
)

func init() {
	logger.Init(&logger.Config{
		LogFilePath:   "test_full_translation.log",
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

func main() {
	zipPath := "testdata/deep-representation-learning-book-main.zip"
	workDir := filepath.Join("testdata", "deep_full_test")

	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Printf("❌ Failed to create work directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== 完整翻译测试 (Full Translation Test) ===\n")
	fmt.Printf("Zip file: %s\n", zipPath)
	fmt.Printf("Work directory: %s\n", workDir)
	fmt.Println()

	// Load config
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
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

	fmt.Printf("Model: %s\n", model)
	if baseURL != "" {
		fmt.Printf("Base URL: %s\n", baseURL)
	}
	fmt.Println()

	// Step 1: Extract
	fmt.Println("=== Step 1: Extract ===")
	startTime := time.Now()
	dl := downloader.NewSourceDownloader(workDir)
	sourceInfo, err := dl.ExtractZip(zipPath)
	if err != nil {
		fmt.Printf("❌ Extract failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Extracted to: %s\n", sourceInfo.ExtractDir)
	fmt.Printf("   Found %d tex files\n", len(sourceInfo.AllTexFiles))
	fmt.Printf("   Time: %.1fs\n", time.Since(startTime).Seconds())
	fmt.Println()

	// Step 2: Find main tex file
	fmt.Println("=== Step 2: Find Main TeX File ===")
	mainTexFile, err := dl.FindMainTexFile(sourceInfo.ExtractDir)
	if err != nil {
		fmt.Printf("❌ Failed to find main tex file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Main tex file: %s\n", mainTexFile)
	mainTexPath := filepath.Join(sourceInfo.ExtractDir, mainTexFile)
	fmt.Println()

	// Step 3: Read main file
	fmt.Println("=== Step 3: Read Main File ===")
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		fmt.Printf("❌ Failed to read main tex file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ File size: %d bytes (%d KB)\n", len(content), len(content)/1024)
	fmt.Println()

	// Step 4: Translate
	fmt.Println("=== Step 4: Translate ===")
	fmt.Println("⚠️  This will translate the ENTIRE file and may take several minutes...")
	fmt.Println("⚠️  This will consume API tokens (estimated: 3000-5000 tokens)")
	fmt.Println()
	
	startTime = time.Now()
	engine := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, 3)
	
	var lastProgress int
	result, err := engine.TranslateTeXWithProgress(string(content), func(current, total int, message string) {
		progress := (current * 100) / total
		if progress != lastProgress {
			fmt.Printf("\r  Progress: %d%% (%d/%d chunks) - %s", progress, current, total, message)
			lastProgress = progress
		}
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("❌ Translation failed: %v\n", err)
		os.Exit(1)
	}

	translationTime := time.Since(startTime)
	fmt.Printf("✅ Translation completed!\n")
	fmt.Printf("   Tokens used: %d\n", result.TokensUsed)
	fmt.Printf("   Time: %.1fs (%.1f tokens/sec)\n", 
		translationTime.Seconds(), 
		float64(result.TokensUsed)/translationTime.Seconds())
	fmt.Printf("   Original length: %d bytes\n", len(result.OriginalContent))
	fmt.Printf("   Translated length: %d bytes\n", len(result.TranslatedContent))
	
	// Count Chinese characters
	chineseCount := 0
	for _, r := range result.TranslatedContent {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}
	fmt.Printf("   Chinese characters: %d (%.1f%%)\n", 
		chineseCount, 
		float64(chineseCount)*100/float64(len(result.TranslatedContent)))
	fmt.Println()

	// Step 5: Save translated file
	fmt.Println("=== Step 5: Save Translated File ===")
	translatedTexPath := filepath.Join(filepath.Dir(mainTexPath), "translated_"+filepath.Base(mainTexPath))
	if err := os.WriteFile(translatedTexPath, []byte(result.TranslatedContent), 0644); err != nil {
		fmt.Printf("❌ Failed to save translated file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Saved to: %s\n", translatedTexPath)
	fmt.Println()

	// Step 6: Compile translated version
	fmt.Println("=== Step 6: Compile Translated Version ===")
	fmt.Println("⚠️  This may take several minutes for large projects...")
	fmt.Println()
	
	startTime = time.Now()
	comp := compiler.NewLaTeXCompiler("pdflatex", workDir, 10*time.Minute)
	translatedOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_translated")
	translatedResult, err := comp.Compile(translatedTexPath, translatedOutputDir)
	
	compileTime := time.Since(startTime)
	
	if err != nil || !translatedResult.Success {
		fmt.Printf("⚠️  Compilation failed (this is common for complex documents)\n")
		if translatedResult != nil {
			fmt.Printf("   Error: %s\n", translatedResult.ErrorMsg)
		}
		fmt.Println()
		fmt.Println("Note: Compilation failure doesn't mean translation failed.")
		fmt.Println("The translated .tex file has been saved successfully.")
	} else {
		fmt.Printf("✅ Compilation successful!\n")
		fmt.Printf("   PDF: %s\n", translatedResult.PDFPath)
		fmt.Printf("   Time: %.1fs\n", compileTime.Seconds())
	}
	fmt.Println()

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Printf("✅ Translation completed successfully\n")
	fmt.Printf("   Extract directory: %s\n", sourceInfo.ExtractDir)
	fmt.Printf("   Translated file: %s\n", translatedTexPath)
	if translatedResult != nil && translatedResult.Success {
		fmt.Printf("   PDF file: %s\n", translatedResult.PDFPath)
	}
	fmt.Println()
	fmt.Println("You can now:")
	fmt.Println("  1. Review the translated .tex file")
	fmt.Println("  2. Manually compile if needed")
	fmt.Println("  3. Compare with the original")
}
