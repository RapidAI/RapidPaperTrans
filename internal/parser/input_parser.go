// Package parser provides input parsing and type identification for the LaTeX translator.
package parser

import (
	"regexp"
	"strings"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

var (
	// arXiv ID patterns
	// New format: YYMM.NNNNN (e.g., 2301.00001, 2301.12345)
	newArxivIDPattern = regexp.MustCompile(`^\d{4}\.\d{4,5}$`)
	// Old format: category/NNNNNNN (e.g., hep-th/9901001, math-ph/0001234)
	oldArxivIDPattern = regexp.MustCompile(`^[a-z-]+/\d{7}$`)
)

// ParseInput analyzes the input string and determines its type.
// It returns the SourceType and an error if the input is invalid.
//
// Input type rules:
// - Starts with http:// or https:// and contains "arxiv" → URL type
// - Matches arXiv ID format (YYMM.NNNNN or category/NNNNNNN) → ArxivID type
// - Ends with .zip (local path) → LocalZip type
// - Ends with .pdf (local path) → LocalPDF type
// - Otherwise → error (invalid input)
func ParseInput(input string) (types.SourceType, error) {
	logger.Debug("parsing input", logger.String("input", input))

	if input == "" {
		logger.Warn("parse input failed: empty input")
		return "", types.NewAppError(types.ErrInvalidInput, "输入不能为空", nil)
	}

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Check for URL type (starts with http:// or https:// and contains "arxiv")
	if isArxivURL(input) {
		logger.Info("input identified as arXiv URL", logger.String("input", input))
		return types.SourceTypeURL, nil
	}

	// Check for arXiv ID type
	if isArxivID(input) {
		logger.Info("input identified as arXiv ID", logger.String("input", input))
		return types.SourceTypeArxivID, nil
	}

	// Check for local zip file
	if isLocalZip(input) {
		logger.Info("input identified as local zip file", logger.String("input", input))
		return types.SourceTypeLocalZip, nil
	}

	// Check for local PDF file
	if isLocalPDF(input) {
		logger.Info("input identified as local PDF file", logger.String("input", input))
		return types.SourceTypeLocalPDF, nil
	}

	// Invalid input
	logger.Warn("invalid input format", logger.String("input", input))
	return "", types.NewAppError(types.ErrInvalidInput, "无效的输入格式", nil)
}

// isArxivURL checks if the input is an arXiv URL.
// A valid arXiv URL starts with http:// or https:// and contains "arxiv".
func isArxivURL(input string) bool {
	lowerInput := strings.ToLower(input)
	hasHTTPPrefix := strings.HasPrefix(lowerInput, "http://") || strings.HasPrefix(lowerInput, "https://")
	containsArxiv := strings.Contains(lowerInput, "arxiv")
	return hasHTTPPrefix && containsArxiv
}

// isArxivID checks if the input matches the arXiv ID format.
// Valid formats:
// - New format: YYMM.NNNNN (4 digits, dot, 4-5 digits)
// - Old format: category/NNNNNNN (lowercase letters and hyphens, slash, 7 digits)
func isArxivID(input string) bool {
	return newArxivIDPattern.MatchString(input) || oldArxivIDPattern.MatchString(input)
}

// isLocalZip checks if the input is a local zip file path.
// A valid local zip path ends with ".zip" (case-insensitive).
func isLocalZip(input string) bool {
	return strings.HasSuffix(strings.ToLower(input), ".zip")
}

// isLocalPDF checks if the input is a local PDF file path.
// A valid local PDF path ends with ".pdf" (case-insensitive).
func isLocalPDF(input string) bool {
	return strings.HasSuffix(strings.ToLower(input), ".pdf")
}

// ValidateArxivID validates the format of an arXiv ID.
// Returns true if the ID is valid, false otherwise.
func ValidateArxivID(id string) bool {
	return isArxivID(id)
}
