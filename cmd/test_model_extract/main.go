package main

import (
	"fmt"
	"os"
	"path/filepath"

	"latex-translator/internal/models"
)

func main() {
	// Use temp directory
	tempDir := filepath.Join(os.TempDir(), "test_model_extract")
	defer os.RemoveAll(tempDir)

	fmt.Println("Extracting model...")
	modelPath, err := models.EnsureModelExtracted(tempDir)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Check file size
	info, err := os.Stat(modelPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Model extracted to: %s\n", modelPath)
	fmt.Printf("Model size: %.2f MB\n", float64(info.Size())/(1024*1024))
}
