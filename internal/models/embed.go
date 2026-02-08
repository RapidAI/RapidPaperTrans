// Package models provides embedded model files for layout detection.
package models

import (
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed doclayout_yolo.onnx.gz
var modelFS embed.FS

const (
	// ModelFileName is the name of the model file (after decompression)
	ModelFileName = "doclayout_yolo.onnx"
	// CompressedModelFileName is the name of the embedded compressed model
	CompressedModelFileName = "doclayout_yolo.onnx.gz"
)

// EnsureModelExtracted ensures the model file is extracted to the target directory.
// Returns the path to the extracted model file.
func EnsureModelExtracted(targetDir string) (string, error) {
	modelPath := filepath.Join(targetDir, ModelFileName)

	// Check if already extracted
	if info, err := os.Stat(modelPath); err == nil && info.Size() > 0 {
		return modelPath, nil
	}

	// Create target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create model directory: %w", err)
	}

	// Open compressed file from embedded FS
	compressedFile, err := modelFS.Open(CompressedModelFileName)
	if err != nil {
		return "", fmt.Errorf("failed to open embedded model: %w", err)
	}
	defer compressedFile.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(compressedFile)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create output file
	dstFile, err := os.Create(modelPath)
	if err != nil {
		return "", fmt.Errorf("failed to create model file: %w", err)
	}
	defer dstFile.Close()

	// Decompress and write
	if _, err := io.Copy(dstFile, gzReader); err != nil {
		os.Remove(modelPath)
		return "", fmt.Errorf("failed to extract model: %w", err)
	}

	return modelPath, nil
}

// GetModelPath returns the path where the model should be located.
func GetModelPath(baseDir string) string {
	return filepath.Join(baseDir, "models", ModelFileName)
}
