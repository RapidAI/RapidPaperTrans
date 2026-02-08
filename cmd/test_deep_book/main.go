// Test program for deep-representation-learning-book translation
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
)

func init() {
	logger.Init(&logger.Config{
		LogFilePath:   "test_deep_book.log",
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
	workDir := filepath.Join("testdata", "deep_book_test")

	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Printf("Failed to create work directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Testing Deep Learning Book Translation ===\n")
	fmt.Printf("Zip file: %s\n", zipPath)
	fmt.Printf("Work directory: %s\n", workDir)

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
		fmt.Println("\nAll tex files found:")
		for i, f := range sourceInfo.AllTexFiles {
			fmt.Printf("  %d. %s\n", i+1, f)
		}
		os.Exit(1)
	}
	fmt.Printf("Main tex file: %s\n", mainTexFile)

	mainTexPath := filepath.Join(sourceInfo.ExtractDir, mainTexFile)

	// Step 3: Check file content
	fmt.Println("\n=== Step 3: Check Main File ===")
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		fmt.Printf("Failed to read main tex file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("File size: %d bytes\n", len(content))
	fmt.Printf("First 500 characters:\n%s\n", string(content[:min(500, len(content))]))
	
	// Check project size
	texFileCount := len(sourceInfo.AllTexFiles)
	fmt.Printf("\nProject size: %d tex files\n", texFileCount)
	if texFileCount > 20 {
		fmt.Printf("⚠️  Large project detected! Compilation may take several minutes.\n")
	}
	if texFileCount > 50 {
		fmt.Printf("⚠️  Very large project! Compilation may take 5-10 minutes or more.\n")
	}
	// Step 4: Preprocess
	fmt.Println("\n=== Step 4: Preprocess ===")
	if err := compiler.PreprocessTexFiles(sourceInfo.ExtractDir); err != nil {
		fmt.Printf("Preprocess warning: %v\n", err)
	}

	// Step 5: Compile original
	fmt.Println("\n=== Step 5: Compile Original ===")
	// Use 10 minute timeout for large projects
	comp := compiler.NewLaTeXCompiler("pdflatex", workDir, 10*time.Minute)
	originalOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_original")
	originalResult, err := comp.Compile(mainTexPath, originalOutputDir)
	if err != nil || !originalResult.Success {
		fmt.Printf("Original compilation failed: %v\n", err)
		if originalResult != nil {
			fmt.Printf("Error message: %s\n", originalResult.ErrorMsg)
			fmt.Println("\n=== Compilation Log (last 50 lines) ===")
			lines := splitLines(originalResult.Log)
			start := len(lines) - 50
			if start < 0 {
				start = 0
			}
			for _, line := range lines[start:] {
				fmt.Println(line)
			}
		}
		fmt.Println("\nNote: Compilation may fail due to missing dependencies or complex structure.")
		fmt.Println("This is expected for some papers. Continuing to translation test...")
	} else {
		fmt.Printf("Original PDF: %s\n", originalResult.PDFPath)
	}

	fmt.Println("\n=== Test Complete ===")
	fmt.Printf("Extract directory: %s\n", sourceInfo.ExtractDir)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
