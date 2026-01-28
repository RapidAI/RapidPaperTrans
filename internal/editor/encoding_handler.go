// Package editor provides file editing tools for LaTeX documents.
package editor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"

	"latex-translator/internal/logger"
)

// EncodingHandler handles file encoding detection and conversion
type EncodingHandler struct {
	backupMgr *BackupManager
}

// NewEncodingHandler creates a new EncodingHandler
func NewEncodingHandler(backupMgr *BackupManager) *EncodingHandler {
	return &EncodingHandler{
		backupMgr: backupMgr,
	}
}

// DetectEncoding detects the encoding of a file
// Returns: "UTF-8", "UTF-8-BOM", "GBK", "UTF-16LE", "UTF-16BE", or "UNKNOWN"
func (h *EncodingHandler) DetectEncoding(path string) (string, error) {
	logger.Debug("detecting file encoding", logger.String("path", path))

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("failed to read file", err)
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Check for BOM markers
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xEF, 0xBB, 0xBF}) {
		logger.Debug("detected UTF-8 with BOM")
		return "UTF-8-BOM", nil
	}
	if len(data) >= 2 && bytes.Equal(data[:2], []byte{0xFF, 0xFE}) {
		logger.Debug("detected UTF-16LE")
		return "UTF-16LE", nil
	}
	if len(data) >= 2 && bytes.Equal(data[:2], []byte{0xFE, 0xFF}) {
		logger.Debug("detected UTF-16BE")
		return "UTF-16BE", nil
	}

	// Check if valid UTF-8
	if utf8.Valid(data) {
		logger.Debug("detected UTF-8")
		return "UTF-8", nil
	}

	// Try to decode as GBK
	if h.isValidGBK(data) {
		logger.Debug("detected GBK")
		return "GBK", nil
	}

	logger.Warn("unknown encoding detected")
	return "UNKNOWN", nil
}

// isValidGBK checks if data is valid GBK encoding
func (h *EncodingHandler) isValidGBK(data []byte) bool {
	decoder := simplifiedchinese.GBK.NewDecoder()
	decoded, err := decoder.Bytes(data)
	if err != nil {
		return false
	}
	// Check if decoded text is valid UTF-8
	return utf8.Valid(decoded)
}

// ConvertEncoding converts a file from one encoding to another
func (h *EncodingHandler) ConvertEncoding(path string, from, to string) error {
	logger.Info("converting file encoding",
		logger.String("path", path),
		logger.String("from", from),
		logger.String("to", to))

	// Create backup
	backup, err := h.backupMgr.CreateBackup(path)
	if err != nil {
		logger.Error("failed to create backup", err)
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("failed to read file", err)
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Decode from source encoding
	var decoded []byte
	switch from {
	case "GBK":
		decoder := simplifiedchinese.GBK.NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			h.backupMgr.Restore(backup, path)
			return fmt.Errorf("failed to decode from GBK: %w", err)
		}
	case "UTF-8-BOM":
		// Remove BOM
		if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xEF, 0xBB, 0xBF}) {
			decoded = data[3:]
		} else {
			decoded = data
		}
	case "UTF-16LE":
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			h.backupMgr.Restore(backup, path)
			return fmt.Errorf("failed to decode from UTF-16LE: %w", err)
		}
	case "UTF-16BE":
		decoder := unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			h.backupMgr.Restore(backup, path)
			return fmt.Errorf("failed to decode from UTF-16BE: %w", err)
		}
	case "UTF-8":
		decoded = data
	default:
		h.backupMgr.Restore(backup, path)
		return fmt.Errorf("unsupported source encoding: %s", from)
	}

	// Encode to target encoding
	var encoded []byte
	switch to {
	case "UTF-8":
		encoded = decoded
	case "UTF-8-BOM":
		encoded = append([]byte{0xEF, 0xBB, 0xBF}, decoded...)
	case "GBK":
		encoder := simplifiedchinese.GBK.NewEncoder()
		encoded, err = encoder.Bytes(decoded)
		if err != nil {
			h.backupMgr.Restore(backup, path)
			return fmt.Errorf("failed to encode to GBK: %w", err)
		}
	default:
		h.backupMgr.Restore(backup, path)
		return fmt.Errorf("unsupported target encoding: %s", to)
	}

	// Write file
	if err := os.WriteFile(path, encoded, 0644); err != nil {
		h.backupMgr.Restore(backup, path)
		logger.Error("failed to write file", err)
		return fmt.Errorf("failed to write file: %w", err)
	}

	logger.Info("encoding conversion completed successfully")
	return nil
}

// RemoveBOM removes BOM marker from a file
func (h *EncodingHandler) RemoveBOM(path string) error {
	logger.Debug("removing BOM", logger.String("path", path))

	// Create backup
	backup, err := h.backupMgr.CreateBackup(path)
	if err != nil {
		logger.Error("failed to create backup", err)
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("failed to read file", err)
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Remove UTF-8 BOM if present
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
		logger.Debug("UTF-8 BOM removed")
	} else {
		logger.Debug("no BOM found")
		return nil
	}

	// Write file
	if err := os.WriteFile(path, data, 0644); err != nil {
		h.backupMgr.Restore(backup, path)
		logger.Error("failed to write file", err)
		return fmt.Errorf("failed to write file: %w", err)
	}

	logger.Info("BOM removed successfully")
	return nil
}

// EnsureUTF8 ensures a file is in UTF-8 encoding without BOM
func (h *EncodingHandler) EnsureUTF8(path string) error {
	logger.Debug("ensuring UTF-8 encoding", logger.String("path", path))

	// Detect current encoding
	encoding, err := h.DetectEncoding(path)
	if err != nil {
		return err
	}

	// If already UTF-8, just remove BOM if present
	if encoding == "UTF-8" {
		logger.Debug("file is already UTF-8")
		return nil
	}

	// If UTF-8 with BOM, remove BOM
	if encoding == "UTF-8-BOM" {
		logger.Debug("removing BOM from UTF-8 file")
		return h.RemoveBOM(path)
	}

	// Convert to UTF-8
	logger.Info("converting to UTF-8", logger.String("from", encoding))
	return h.ConvertEncoding(path, encoding, "UTF-8")
}

// ReadFileWithEncoding reads a file and returns its content as UTF-8 string
// This automatically handles encoding conversion
func (h *EncodingHandler) ReadFileWithEncoding(path string) (string, error) {
	// Detect encoding
	encoding, err := h.DetectEncoding(path)
	if err != nil {
		return "", err
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Decode based on encoding
	var decoded []byte
	switch encoding {
	case "UTF-8":
		decoded = data
	case "UTF-8-BOM":
		if len(data) >= 3 {
			decoded = data[3:]
		} else {
			decoded = data
		}
	case "GBK":
		decoder := simplifiedchinese.GBK.NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			return "", fmt.Errorf("failed to decode GBK: %w", err)
		}
	case "UTF-16LE":
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			return "", fmt.Errorf("failed to decode UTF-16LE: %w", err)
		}
	case "UTF-16BE":
		decoder := unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			return "", fmt.Errorf("failed to decode UTF-16BE: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported encoding: %s", encoding)
	}

	return string(decoded), nil
}

// WriteFileWithEncoding writes content to a file in the specified encoding
func (h *EncodingHandler) WriteFileWithEncoding(path string, content string, encoding string) error {
	var data []byte
	var err error

	switch encoding {
	case "UTF-8":
		data = []byte(content)
	case "UTF-8-BOM":
		data = append([]byte{0xEF, 0xBB, 0xBF}, []byte(content)...)
	case "GBK":
		encoder := simplifiedchinese.GBK.NewEncoder()
		data, err = encoder.Bytes([]byte(content))
		if err != nil {
			return fmt.Errorf("failed to encode to GBK: %w", err)
		}
	default:
		return fmt.Errorf("unsupported encoding: %s", encoding)
	}

	return os.WriteFile(path, data, 0644)
}

// FixEncodingIssues attempts to fix common encoding issues in a file
func (h *EncodingHandler) FixEncodingIssues(path string) error {
	logger.Info("fixing encoding issues", logger.String("path", path))

	// Create backup
	backup, err := h.backupMgr.CreateBackup(path)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Try to read with encoding detection
	content, err := h.ReadFileWithEncoding(path)
	if err != nil {
		h.backupMgr.Restore(backup, path)
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Write back as UTF-8 without BOM
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		h.backupMgr.Restore(backup, path)
		return fmt.Errorf("failed to write file: %w", err)
	}

	logger.Info("encoding issues fixed successfully")
	return nil
}

// ValidateUTF8 checks if a file is valid UTF-8
func (h *EncodingHandler) ValidateUTF8(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	return utf8.Valid(data), nil
}

// GetEncodingInfo returns detailed information about a file's encoding
type EncodingInfo struct {
	Encoding    string
	HasBOM      bool
	IsValid     bool
	FileSize    int64
	SampleText  string // First 100 characters
}

// GetEncodingInfo returns detailed encoding information for a file
func (h *EncodingHandler) GetEncodingInfo(path string) (*EncodingInfo, error) {
	// Get file info
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Detect encoding
	encoding, err := h.DetectEncoding(path)
	if err != nil {
		return nil, err
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Check for BOM
	hasBOM := false
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xEF, 0xBB, 0xBF}) {
		hasBOM = true
	}

	// Get sample text
	sampleText := ""
	if content, err := h.ReadFileWithEncoding(path); err == nil {
		if len(content) > 100 {
			sampleText = content[:100]
		} else {
			sampleText = content
		}
	}

	// Validate
	isValid := encoding != "UNKNOWN"

	return &EncodingInfo{
		Encoding:   encoding,
		HasBOM:     hasBOM,
		IsValid:    isValid,
		FileSize:   fileInfo.Size(),
		SampleText: sampleText,
	}, nil
}

// CopyWithEncoding copies a file and converts its encoding
func (h *EncodingHandler) CopyWithEncoding(srcPath, dstPath string, targetEncoding string) error {
	// Read source file with encoding detection
	content, err := h.ReadFileWithEncoding(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Write to destination with target encoding
	return h.WriteFileWithEncoding(dstPath, content, targetEncoding)
}

// StreamConvert converts a large file's encoding using streaming to avoid memory issues
func (h *EncodingHandler) StreamConvert(path string, from, to string) error {
	logger.Info("stream converting file encoding",
		logger.String("path", path),
		logger.String("from", from),
		logger.String("to", to))

	// Create backup
	backup, err := h.backupMgr.CreateBackup(path)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Open source file
	srcFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create temporary file
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "encoding_convert_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Set up decoder
	var reader io.Reader = srcFile
	switch from {
	case "GBK":
		reader = simplifiedchinese.GBK.NewDecoder().Reader(srcFile)
	case "UTF-8-BOM":
		// Skip BOM
		bom := make([]byte, 3)
		srcFile.Read(bom)
	case "UTF-16LE":
		reader = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Reader(srcFile)
	case "UTF-16BE":
		reader = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder().Reader(srcFile)
	}

	// Set up encoder
	var writer io.Writer = tmpFile
	switch to {
	case "UTF-8-BOM":
		tmpFile.Write([]byte{0xEF, 0xBB, 0xBF})
	case "GBK":
		writer = simplifiedchinese.GBK.NewEncoder().Writer(tmpFile)
	}

	// Copy with conversion
	if _, err := io.Copy(writer, reader); err != nil {
		tmpFile.Close()
		h.backupMgr.Restore(backup, path)
		return fmt.Errorf("failed to convert encoding: %w", err)
	}

	tmpFile.Close()

	// Replace original file
	if err := os.Rename(tmpPath, path); err != nil {
		h.backupMgr.Restore(backup, path)
		return fmt.Errorf("failed to replace original file: %w", err)
	}

	logger.Info("stream encoding conversion completed successfully")
	return nil
}
