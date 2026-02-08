// Command check_pages validates content completeness between original and translated PDFs.
// It compares section structure, appendices, and page counts to detect missing content.
//
// Usage:
//
//	go run cmd/check_pages/main.go <original.pdf> <translated.pdf>
package main

import (
	"fmt"
	"os"

	"latex-translator/internal/pdf"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: check_pages <original.pdf> <translated.pdf>")
		fmt.Println()
		fmt.Println("This tool validates content completeness between original and translated PDFs.")
		fmt.Println("It compares:")
		fmt.Println("  - Page counts (warns if translated has >15% fewer pages)")
		fmt.Println("  - Section structure (numbered sections like 1, 1.1, 2, etc.)")
		fmt.Println("  - Appendices (Appendix A, B, etc.)")
		fmt.Println("  - Special sections (Abstract, References, Acknowledgments)")
		os.Exit(1)
	}

	originalPath := os.Args[1]
	translatedPath := os.Args[2]

	// Check if files exist
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		fmt.Printf("Error: Original PDF not found: %s\n", originalPath)
		os.Exit(1)
	}
	if _, err := os.Stat(translatedPath); os.IsNotExist(err) {
		fmt.Printf("Error: Translated PDF not found: %s\n", translatedPath)
		os.Exit(1)
	}

	fmt.Printf("Validating content completeness...\n")
	fmt.Printf("  Original:   %s\n", originalPath)
	fmt.Printf("  Translated: %s\n\n", translatedPath)

	// Create validator
	validator := pdf.NewContentValidator("")

	// Run validation
	result, err := validator.ValidateContent(originalPath, translatedPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Print formatted result
	fmt.Println(pdf.FormatValidationResult(result))

	// Exit with error code if content is incomplete
	if !result.IsComplete {
		os.Exit(2)
	}
}
