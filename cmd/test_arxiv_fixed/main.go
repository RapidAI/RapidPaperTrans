// Test program for arXiv paper translation with automatic fixes
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"latex-translator/internal/compiler"
	"latex-translator/internal/downloader"
	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
)

func init() {
	logger.Init(&logger.Config{
		LogFilePath:   "test_arxiv.log",
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
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <arxiv-id>")
		fmt.Println("Example: go run main.go 2506.02153")
		os.Exit(1)
	}

	arxivID := os.Args[1]
	workDir := filepath.Join("testdata", "arxiv_test")

	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Printf("Failed to create work directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Testing arXiv Translation ===\n")
	fmt.Printf("arXiv ID: %s\n", arxivID)
	fmt.Printf("Work directory: %s\n", workDir)

	// Step 1: Download
	fmt.Println("\n=== Step 1: Download ===")
	dl := downloader.NewSourceDownloader(workDir)
	sourceInfo, err := dl.DownloadByID(arxivID)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Downloaded to: %s\n", sourceInfo.ExtractDir)

	// Step 2: Extract
	fmt.Println("\n=== Step 2: Extract ===")
	sourceInfo, err = dl.ExtractZip(sourceInfo.ExtractDir)
	if err != nil {
		fmt.Printf("Extract failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Extracted to: %s\n", sourceInfo.ExtractDir)

	// Step 3: Find main tex file
	fmt.Println("\n=== Step 3: Find Main TeX File ===")
	mainTexFile, err := dl.FindMainTexFile(sourceInfo.ExtractDir)
	if err != nil {
		fmt.Printf("Failed to find main tex file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Main tex file: %s\n", mainTexFile)

	mainTexPath := filepath.Join(sourceInfo.ExtractDir, mainTexFile)

	// Step 4: Preprocess
	fmt.Println("\n=== Step 4: Preprocess ===")
	if err := compiler.PreprocessTexFiles(sourceInfo.ExtractDir); err != nil {
		fmt.Printf("Preprocess warning: %v\n", err)
	}

	// Step 5: Compile original
	fmt.Println("\n=== Step 5: Compile Original ===")
	comp := compiler.NewLaTeXCompiler("pdflatex", workDir, 0)
	originalOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_original")
	originalResult, err := comp.Compile(mainTexPath, originalOutputDir)
	if err != nil {
		fmt.Printf("Original compilation failed: %v\n", err)
		fmt.Printf("Error message: %s\n", originalResult.ErrorMsg)
		fmt.Println("\n=== Compilation Log (last 100 lines) ===")
		lines := splitLines(originalResult.Log)
		start := len(lines) - 100
		if start < 0 {
			start = 0
		}
		for _, line := range lines[start:] {
			fmt.Println(line)
		}
		os.Exit(1)
	}
	fmt.Printf("Original PDF: %s\n", originalResult.PDFPath)

	// Step 6: Translate
	fmt.Println("\n=== Step 6: Translate ===")
	apiKey := getAPIKey()
	if apiKey == "" {
		fmt.Println("No API key configured, skipping translation")
		os.Exit(0)
	}

	baseURL := getBaseURL()
	model := getModel()
	fmt.Printf("Using model: %s\n", model)
	if baseURL != "" {
		fmt.Printf("Using base URL: %s\n", baseURL)
	}

	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		fmt.Printf("Failed to read tex file: %v\n", err)
		os.Exit(1)
	}

	engine := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, 3)
	result, err := engine.TranslateTeXWithProgress(string(content), func(current, total int, message string) {
		fmt.Printf("\r  Progress: %d/%d - %s", current, total, message)
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("Translation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Translation completed! Tokens used: %d\n", result.TokensUsed)

	// Add ctex package for Chinese support
	translatedContent := ensureCtexPackage(result.TranslatedContent)
	
	// Fix caption issues
	translatedContent = fixCaptionIssues(translatedContent)

	// Save translated file
	translatedTexPath := filepath.Join(sourceInfo.ExtractDir, "translated_"+mainTexFile)
	if err := os.WriteFile(translatedTexPath, []byte(translatedContent), 0644); err != nil {
		fmt.Printf("Failed to save translated file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved to: %s\n", translatedTexPath)

	// Step 7: Compile translated
	fmt.Println("\n=== Step 7: Compile Translated ===")
	translatedOutputDir := filepath.Join(sourceInfo.ExtractDir, "output_translated")
	translatedResult, err := comp.CompileWithXeLaTeX(translatedTexPath, translatedOutputDir)
	if err != nil || !translatedResult.Success {
		fmt.Printf("Translated compilation failed: %v\n", err)
		if translatedResult != nil {
			fmt.Printf("Error message: %s\n", translatedResult.ErrorMsg)
			fmt.Println("\n=== Compilation Log (last 100 lines) ===")
			lines := splitLines(translatedResult.Log)
			start := len(lines) - 100
			if start < 0 {
				start = 0
			}
			for _, line := range lines[start:] {
				fmt.Println(line)
			}
		}
		os.Exit(1)
	}
	fmt.Printf("Translated PDF: %s\n", translatedResult.PDFPath)

	fmt.Println("\n=== Success! ===")
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

// ensureCtexPackage adds ctex package for Chinese support if not already present.
// It also fixes microtype compatibility issues with XeLaTeX.
func ensureCtexPackage(content string) string {
	// Check if ctex is already included
	if strings.Contains(content, "\\usepackage{ctex}") || (strings.Contains(content, "\\usepackage[") && strings.Contains(content, "ctex")) {
		logger.Debug("ctex package already present")
	} else {
		// Find the first uncommented \documentclass line and add \usepackage{ctex} after it
		lines := strings.Split(content, "\n")
		var result []string
		added := false

		for _, line := range lines {
			result = append(result, line)
			// Skip if already added or if line is commented
			if added || strings.HasPrefix(strings.TrimSpace(line), "%") {
				continue
			}
			// Check if this line contains \documentclass
			if strings.Contains(line, "\\documentclass") {
				result = append(result, "\\usepackage{ctex}")
				added = true
				logger.Info("added ctex package for Chinese support")
			}
		}

		if added {
			content = strings.Join(result, "\n")
		} else {
			logger.Warn("could not find \\documentclass to add ctex package")
		}
	}

	// Fix microtype compatibility with XeLaTeX
	// microtype with expansion/protrusion can cause "Cannot use XeTeXglyph" errors
	// Replace \usepackage{microtype} with a XeLaTeX-compatible version
	if strings.Contains(content, "\\usepackage{microtype}") {
		content = strings.Replace(content, "\\usepackage{microtype}",
			"\\usepackage[protrusion=false,expansion=false]{microtype}", 1)
		logger.Info("fixed microtype package for XeLaTeX compatibility")
	}

	// Also handle microtype with options
	microtypePattern := regexp.MustCompile(`\\usepackage\[([^\]]*)\]\{microtype\}`)
	if microtypePattern.MatchString(content) {
		// Check if protrusion/expansion are already disabled
		if !strings.Contains(content, "protrusion=false") {
			content = microtypePattern.ReplaceAllString(content,
				"\\usepackage[protrusion=false,expansion=false]{microtype}")
			logger.Info("fixed microtype package options for XeLaTeX compatibility")
		}
	}

	return content
}

// fixCaptionIssues fixes common caption-related issues in translated content
func fixCaptionIssues(content string) string {
	// Fix unbalanced braces in \textbf{\textcolor{...}{\ARCH} patterns
	// Pattern: \textbf{\textcolor{color}{\ARCH} should be \textbf{\textcolor{color}{\ARCH}}
	pattern := regexp.MustCompile(`\\textbf\{\\textcolor\{([^}]+)\}\{\\ARCH\}\s+`)
	content = pattern.ReplaceAllString(content, `\textbf{\textcolor{$1}{\ARCH}} `)
	
	logger.Info("fixed caption brace issues")
	return content
}
