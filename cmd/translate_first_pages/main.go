// Translate only first few pages for quick testing
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	gopdf "github.com/VantageDataChat/GoPDF2"
	"latex-translator/internal/pdf"
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

	baseName := filepath.Base(inputPDF)
	tempPDF := filepath.Join(outputDir, "temp_first_pages.pdf")
	outputPDF := filepath.Join(outputDir, baseName[:len(baseName)-4]+"_first_pages.pdf")

	fmt.Printf("Input:     %s\n", inputPDF)
	fmt.Printf("Max pages: %d\n", maxPages)
	fmt.Println("\nExtracting first pages with GoPDF2...")

	// Extract first N pages using GoPDF2
	if err := extractFirstPages(inputPDF, tempPDF, maxPages); err != nil {
		fmt.Printf("Error extracting pages: %v\n", err)
		os.Exit(1)
	}

	// Translate the temp PDF
	fmt.Printf("API:       %s\n", cfg.OpenAIBaseURL)
	fmt.Printf("Model:     %s\n", cfg.OpenAIModel)
	fmt.Println("\nTranslating...")

	translator := pdf.NewBabelDocTranslator(pdf.BabelDocConfig{
		WorkDir: outputDir,
	})

	err := translator.TranslatePDFWithGoPDF2(tempPDF, outputPDF, cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, cfg.OpenAIModel, func(msg string) {
		fmt.Printf("  %s\n", msg)
	})
	if err != nil {
		fmt.Printf("Error translating: %v\n", err)
		os.Exit(1)
	}

	// Clean up temp file
	os.Remove(tempPDF)

	fmt.Printf("\n=== Translation Complete ===\n")
	fmt.Printf("Output: %s\n", outputPDF)
	fmt.Println("\n请打开PDF文件检查：")
	fmt.Println("1. 字体大小是否合适")
	fmt.Println("2. 文字是否水平显示")
	fmt.Println("3. 中文是否清晰可读")
}

// extractFirstPages extracts the first N pages from a PDF using GoPDF2
func extractFirstPages(inputPath, outputPath string, maxPages int) error {
	src := gopdf.GoPdf{}
	src.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: *gopdf.PageSizeA4})

	pageSizes := src.GetPageSizes(inputPath)
	totalPages := len(pageSizes)
	if totalPages == 0 {
		return fmt.Errorf("cannot read pages from %s", inputPath)
	}

	if maxPages > totalPages {
		maxPages = totalPages
	}

	for i := 1; i <= maxPages; i++ {
		mediaBox, ok := pageSizes[i]["/MediaBox"]
		if !ok {
			mediaBox = map[string]float64{"w": 595, "h": 842}
		}
		w, h := mediaBox["w"], mediaBox["h"]

		src.AddPageWithOption(gopdf.PageOption{
			PageSize: &gopdf.Rect{W: w, H: h},
		})
		tpl := src.ImportPage(inputPath, i, "/MediaBox")
		src.UseImportedTemplate(tpl, 0, 0, w, h)
	}

	if err := src.WritePdf(outputPath); err != nil {
		return fmt.Errorf("failed to write extracted pages: %w", err)
	}

	fmt.Printf("Extracted %d pages\n", maxPages)
	return nil
}
