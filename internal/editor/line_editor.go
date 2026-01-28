// Package editor provides file editing tools for LaTeX documents.
package editor

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"latex-translator/internal/logger"
)

// LineEditor provides line-level file editing functionality
type LineEditor struct {
	backupMgr *BackupManager
}

// NewLineEditor creates a new LineEditor instance
func NewLineEditor(backupMgr *BackupManager) *LineEditor {
	return &LineEditor{
		backupMgr: backupMgr,
	}
}

// ReadLines reads a range of lines from a file
// start: starting line number (1-based)
// end: ending line number (-1 means read to end of file)
func (e *LineEditor) ReadLines(path string, start, end int) ([]string, error) {
	logger.Debug("reading lines from file",
		logger.String("path", path),
		logger.Int("start", start),
		logger.Int("end", end))

	file, err := os.Open(path)
	if err != nil {
		logger.Error("failed to open file", err, logger.String("path", path))
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 1

	for scanner.Scan() {
		if lineNum >= start && (end == -1 || lineNum <= end) {
			lines = append(lines, scanner.Text())
		}
		if end != -1 && lineNum > end {
			break
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		logger.Error("error reading file", err, logger.String("path", path))
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	logger.Debug("read lines successfully", logger.Int("lineCount", len(lines)))
	return lines, nil
}

// ReplaceLine replaces a single line in the file
func (e *LineEditor) ReplaceLine(path string, lineNum int, newContent string) error {
	logger.Debug("replacing line",
		logger.String("path", path),
		logger.Int("lineNum", lineNum))

	// Create backup
	backup, err := e.backupMgr.CreateBackup(path)
	if err != nil {
		logger.Error("failed to create backup", err)
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read all lines
	lines, err := e.readAllLines(path)
	if err != nil {
		return err
	}

	// Validate line number
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line number %d out of range (file has %d lines)", lineNum, len(lines))
	}

	// Replace the line
	lines[lineNum-1] = newContent

	// Write back
	if err := e.writeAllLines(path, lines); err != nil {
		// Restore backup on failure
		e.backupMgr.Restore(backup, path)
		return err
	}

	logger.Info("line replaced successfully", logger.Int("lineNum", lineNum))
	return nil
}

// InsertLine inserts a new line at the specified position
func (e *LineEditor) InsertLine(path string, lineNum int, content string) error {
	logger.Debug("inserting line",
		logger.String("path", path),
		logger.Int("lineNum", lineNum))

	// Create backup
	backup, err := e.backupMgr.CreateBackup(path)
	if err != nil {
		logger.Error("failed to create backup", err)
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read all lines
	lines, err := e.readAllLines(path)
	if err != nil {
		return err
	}

	// Validate line number (can be 1 to len(lines)+1 for append)
	if lineNum < 1 || lineNum > len(lines)+1 {
		return fmt.Errorf("line number %d out of range (file has %d lines)", lineNum, len(lines))
	}

	// Insert the line
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:lineNum-1]...)
	newLines = append(newLines, content)
	newLines = append(newLines, lines[lineNum-1:]...)

	// Write back
	if err := e.writeAllLines(path, newLines); err != nil {
		// Restore backup on failure
		e.backupMgr.Restore(backup, path)
		return err
	}

	logger.Info("line inserted successfully", logger.Int("lineNum", lineNum))
	return nil
}

// DeleteLine deletes a line from the file
func (e *LineEditor) DeleteLine(path string, lineNum int) error {
	logger.Debug("deleting line",
		logger.String("path", path),
		logger.Int("lineNum", lineNum))

	// Create backup
	backup, err := e.backupMgr.CreateBackup(path)
	if err != nil {
		logger.Error("failed to create backup", err)
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read all lines
	lines, err := e.readAllLines(path)
	if err != nil {
		return err
	}

	// Validate line number
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line number %d out of range (file has %d lines)", lineNum, len(lines))
	}

	// Delete the line
	newLines := append(lines[:lineNum-1], lines[lineNum:]...)

	// Write back
	if err := e.writeAllLines(path, newLines); err != nil {
		// Restore backup on failure
		e.backupMgr.Restore(backup, path)
		return err
	}

	logger.Info("line deleted successfully", logger.Int("lineNum", lineNum))
	return nil
}

// ReplaceLines replaces a range of lines with new content
func (e *LineEditor) ReplaceLines(path string, start, end int, newContent []string) error {
	logger.Debug("replacing lines",
		logger.String("path", path),
		logger.Int("start", start),
		logger.Int("end", end),
		logger.Int("newLineCount", len(newContent)))

	// Create backup
	backup, err := e.backupMgr.CreateBackup(path)
	if err != nil {
		logger.Error("failed to create backup", err)
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read all lines
	lines, err := e.readAllLines(path)
	if err != nil {
		return err
	}

	// Validate line range
	if start < 1 || end > len(lines) || start > end {
		return fmt.Errorf("invalid line range: %d-%d (file has %d lines)", start, end, len(lines))
	}

	// Replace the lines
	newLines := make([]string, 0, len(lines)-(end-start+1)+len(newContent))
	newLines = append(newLines, lines[:start-1]...)
	newLines = append(newLines, newContent...)
	newLines = append(newLines, lines[end:]...)

	// Write back
	if err := e.writeAllLines(path, newLines); err != nil {
		// Restore backup on failure
		e.backupMgr.Restore(backup, path)
		return err
	}

	logger.Info("lines replaced successfully",
		logger.Int("start", start),
		logger.Int("end", end),
		logger.Int("newLineCount", len(newContent)))
	return nil
}

// readAllLines reads all lines from a file
func (e *LineEditor) readAllLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		logger.Error("failed to open file", err, logger.String("path", path))
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		logger.Error("error reading file", err, logger.String("path", path))
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return lines, nil
}

// writeAllLines writes all lines to a file
func (e *LineEditor) writeAllLines(path string, lines []string) error {
	file, err := os.Create(path)
	if err != nil {
		logger.Error("failed to create file", err, logger.String("path", path))
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			logger.Error("error writing line", err)
			return fmt.Errorf("error writing line: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		logger.Error("error flushing writer", err)
		return fmt.Errorf("error flushing writer: %w", err)
	}

	return nil
}

// CountLines returns the number of lines in a file
func (e *LineEditor) CountLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading file: %w", err)
	}

	return count, nil
}

// SearchLines searches for lines containing the specified text
func (e *LineEditor) SearchLines(path string, searchText string) ([]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var matchingLines []int
	scanner := bufio.NewScanner(file)
	lineNum := 1

	for scanner.Scan() {
		if strings.Contains(scanner.Text(), searchText) {
			matchingLines = append(matchingLines, lineNum)
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return matchingLines, nil
}
