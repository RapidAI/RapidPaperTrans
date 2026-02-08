package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"latex-translator/internal/translator"
)

// TranslateBook translates all LaTeX files in the book
func TranslateBook(inputDir, outputDir string, apiKey string, maxFiles int) error {
	log.Printf("=== Starting Book Translation ===")
	log.Printf("Input: %s", inputDir)
	log.Printf("Output: %s", outputDir)
	
	// Find all tex files
	texFiles, err := findTexFiles(inputDir)
	if err != nil {
		return fmt.Errorf("failed to find tex files: %w", err)
	}

	log.Printf("Found %d .tex files to translate", len(texFiles))

	// Limit files if specified
	if maxFiles > 0 && len(texFiles) > maxFiles {
		log.Printf("Limiting to first %d files", maxFiles)
		texFiles = texFiles[:maxFiles]
	}

	// Create translator
	trans := translator.NewTranslationEngine(apiKey)

	// Track statistics
	stats := &TranslationStats{
		StartTime: time.Now(),
	}

	// Translate each file
	for i, texFile := range texFiles {
		log.Printf("\n[%d/%d] Processing: %s", i+1, len(texFiles), texFile)
		
		if err := translateFile(trans, texFile, inputDir, outputDir, stats); err != nil {
			log.Printf("  ERROR: %v", err)
			stats.ErrorCount++
			stats.Errors = append(stats.Errors, FileError{
				File:  texFile,
				Error: err.Error(),
			})
		} else {
			stats.SuccessCount++
		}

		// Progress update
		if (i+1)%5 == 0 {
			elapsed := time.Since(stats.StartTime)
			avgTime := elapsed / time.Duration(i+1)
			remaining := avgTime * time.Duration(len(texFiles)-(i+1))
			log.Printf("Progress: %d/%d (%.1f%%), Est. remaining: %v", 
				i+1, len(texFiles), float64(i+1)/float64(len(texFiles))*100, remaining.Round(time.Second))
		}
	}

	// Print summary
	printSummary(stats, len(texFiles))

	return nil
}

type TranslationStats struct {
	StartTime    time.Time
	SuccessCount int
	ErrorCount   int
	SkipCount    int
	Errors       []FileError
}

type FileError struct {
	File  string
	Error string
}

func translateFile(trans *translator.TranslationEngine, texFile, inputDir, outputDir string, stats *TranslationStats) error {
	// Read file
	content, err := os.ReadFile(texFile)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Skip if already Chinese or very small
	if strings.Contains(texFile, "_zh.tex") || strings.Contains(texFile, "_ro.tex") {
		log.Printf("  SKIP: Already translated version")
		stats.SkipCount++
		return nil
	}

	if len(content) < 50 {
		log.Printf("  SKIP: File too small (%d bytes)", len(content))
		stats.SkipCount++
		return nil
	}

	// Translate
	log.Printf("  Translating... (%d bytes)", len(content))
	startTime := time.Now()
	
	result, err := trans.TranslateTeX(string(content))
	if err != nil {
		return fmt.Errorf("translation failed: %w", err)
	}

	elapsed := time.Since(startTime)
	log.Printf("  Translation completed in %v", elapsed.Round(time.Millisecond))

	// Create output path
	relPath, err := filepath.Rel(inputDir, texFile)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}

	outputPath := filepath.Join(outputDir, relPath)
	outputPath = strings.TrimSuffix(outputPath, ".tex") + "_zh.tex"

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write translated file
	if err := os.WriteFile(outputPath, []byte(result.TranslatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	log.Printf("  SUCCESS: Saved to %s", outputPath)

	// Validate
	if err := validateTranslation(outputPath); err != nil {
		log.Printf("  WARNING: Validation issues: %v", err)
	}

	return nil
}

func printSummary(stats *TranslationStats, totalFiles int) {
	elapsed := time.Since(stats.StartTime)
	
	log.Printf("\n" + strings.Repeat("=", 60))
	log.Printf("=== Translation Summary ===")
	log.Printf(strings.Repeat("=", 60))
	log.Printf("Total files found:    %d", totalFiles)
	log.Printf("Successfully translated: %d", stats.SuccessCount)
	log.Printf("Skipped:              %d", stats.SkipCount)
	log.Printf("Errors:               %d", stats.ErrorCount)
	log.Printf("Total time:           %v", elapsed.Round(time.Second))
	
	if stats.SuccessCount > 0 {
		avgTime := elapsed / time.Duration(stats.SuccessCount)
		log.Printf("Average time per file: %v", avgTime.Round(time.Millisecond))
	}

	if len(stats.Errors) > 0 {
		log.Printf("\n=== Errors ===")
		for i, e := range stats.Errors {
			log.Printf("%d. %s: %s", i+1, filepath.Base(e.File), e.Error)
		}
	}
	
	log.Printf(strings.Repeat("=", 60))
}
