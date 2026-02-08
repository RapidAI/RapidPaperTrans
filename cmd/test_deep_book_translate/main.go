package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Parse command line flags
	inputDir := flag.String("input", "testdata/deep_book_test/deep-representation-learning-book-main", "Input directory containing LaTeX files")
	outputDir := flag.String("output", "testdata/output/deep_book_translated", "Output directory for translated files")
	mainFile := flag.String("main", "book-main.tex", "Main LaTeX file")
	dryRun := flag.Bool("dry-run", false, "Dry run - only analyze without translating")
	maxFiles := flag.Int("max-files", 0, "Maximum number of files to translate (0 = all)")
	flag.Parse()

	log.Printf("Starting deep book translation...")
	log.Printf("Input directory: %s", *inputDir)
	log.Printf("Output directory: %s", *outputDir)
	log.Printf("Main file: %s", *mainFile)

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Find all .tex files
	texFiles, err := findTexFiles(*inputDir)
	if err != nil {
		log.Fatalf("Failed to find tex files: %v", err)
	}

	log.Printf("Found %d .tex files", len(texFiles))

	// Analyze the main file structure
	mainPath := filepath.Join(*inputDir, *mainFile)
	if err := analyzeMainFile(mainPath); err != nil {
		log.Printf("Warning: Failed to analyze main file: %v", err)
	}

	if *dryRun {
		log.Printf("Dry run complete. Found %d files to translate.", len(texFiles))
		return
	}

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable not set")
	}

	// Translate the book
	if err := TranslateBook(*inputDir, *outputDir, apiKey, *maxFiles); err != nil {
		log.Fatalf("Translation failed: %v", err)
	}

	log.Printf("\n✓ Translation complete! Output directory: %s", *outputDir)
}

func findTexFiles(dir string) ([]string, error) {
	var texFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".tex") {
			// Skip already translated files
			if !strings.Contains(path, "_zh.tex") && !strings.Contains(path, "_ro.tex") {
				texFiles = append(texFiles, path)
			}
		}

		return nil
	})

	return texFiles, err
}

func analyzeMainFile(mainPath string) error {
	content, err := os.ReadFile(mainPath)
	if err != nil {
		return err
	}

	log.Printf("\n=== Main File Analysis ===")
	log.Printf("File: %s", mainPath)

	// Count subfiles
	subfileCount := strings.Count(string(content), "\\subfile{")
	log.Printf("Subfiles referenced: %d", subfileCount)

	// Count input files
	inputCount := strings.Count(string(content), "\\input{")
	log.Printf("Input files referenced: %d", inputCount)

	// Check for bibliography
	if strings.Contains(string(content), "\\addbibresource") {
		log.Printf("Bibliography: Found")
	}

	// Check for custom packages
	customPackages := []string{"math-macros", "math-theorems", "book-macros"}
	for _, pkg := range customPackages {
		if strings.Contains(string(content), pkg) {
			log.Printf("Custom package: %s", pkg)
		}
	}

	return nil
}

func validateTranslation(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Check for common LaTeX errors
	issues := []string{}

	// Check for unmatched braces
	openBraces := strings.Count(string(content), "{")
	closeBraces := strings.Count(string(content), "}")
	if openBraces != closeBraces {
		issues = append(issues, fmt.Sprintf("Unmatched braces: %d open, %d close", openBraces, closeBraces))
	}

	// Check for unmatched $
	dollarCount := strings.Count(string(content), "$")
	if dollarCount%2 != 0 {
		issues = append(issues, fmt.Sprintf("Unmatched $: %d found", dollarCount))
	}

	// Check for Chinese content
	hasChinese := false
	for _, r := range string(content) {
		if r >= 0x4E00 && r <= 0x9FFF {
			hasChinese = true
			break
		}
	}
	if !hasChinese {
		issues = append(issues, "No Chinese characters found in translation")
	}

	if len(issues) > 0 {
		return fmt.Errorf("%s", strings.Join(issues, "; "))
	}

	return nil
}

func compileLatex(mainFile string) error {
	log.Printf("Compiling: %s", mainFile)

	// Read the file
	content, err := os.ReadFile(mainFile)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Basic LaTeX validation
	if err := validateLatexSyntax(string(content)); err != nil {
		return fmt.Errorf("LaTeX validation failed: %w", err)
	}

	log.Printf("LaTeX validation passed")
	return nil
}

func validateLatexSyntax(content string) error {
	// Check for unmatched braces
	openBraces := strings.Count(content, "{")
	closeBraces := strings.Count(content, "}")
	if openBraces != closeBraces {
		return fmt.Errorf("unmatched braces: %d open, %d close", openBraces, closeBraces)
	}

	// Check for unmatched environments
	beginCount := strings.Count(content, "\\begin{")
	endCount := strings.Count(content, "\\end{")
	if beginCount != endCount {
		return fmt.Errorf("unmatched environments: %d begin, %d end", beginCount, endCount)
	}

	return nil
}
