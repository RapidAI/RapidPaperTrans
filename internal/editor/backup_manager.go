// Package editor provides file editing tools for LaTeX documents.
package editor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"latex-translator/internal/logger"
)

// BackupManager manages file backups for safe editing
type BackupManager struct {
	backupDir string
}

// NewBackupManager creates a new BackupManager
// If backupDir is empty, backups are created in the same directory as the original file
func NewBackupManager(backupDir string) *BackupManager {
	return &BackupManager{
		backupDir: backupDir,
	}
}

// CreateBackup creates a backup of the specified file
// Returns the path to the backup file
func (m *BackupManager) CreateBackup(path string) (string, error) {
	logger.Debug("creating backup", logger.String("path", path))

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", path)
	}

	// Generate backup filename
	timestamp := time.Now().Format("20060102_150405")
	basename := filepath.Base(path)
	backupName := fmt.Sprintf("%s.backup_%s", basename, timestamp)

	// Determine backup directory
	var backupPath string
	if m.backupDir != "" {
		// Ensure backup directory exists
		if err := os.MkdirAll(m.backupDir, 0755); err != nil {
			logger.Error("failed to create backup directory", err)
			return "", fmt.Errorf("failed to create backup directory: %w", err)
		}
		backupPath = filepath.Join(m.backupDir, backupName)
	} else {
		// Use same directory as original file
		backupPath = path + ".backup_" + timestamp
	}

	// Copy file
	if err := copyFile(path, backupPath); err != nil {
		logger.Error("failed to copy file", err)
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	logger.Info("backup created successfully", logger.String("backupPath", backupPath))
	return backupPath, nil
}

// Restore restores a file from its backup
func (m *BackupManager) Restore(backupPath string, originalPath string) error {
	logger.Debug("restoring from backup",
		logger.String("backupPath", backupPath),
		logger.String("originalPath", originalPath))

	// Check if backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", backupPath)
	}

	// Copy backup to original location
	if err := copyFile(backupPath, originalPath); err != nil {
		logger.Error("failed to restore backup", err)
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	logger.Info("file restored from backup successfully")
	return nil
}

// ListBackups lists all backups for a given file
func (m *BackupManager) ListBackups(path string) ([]string, error) {
	basename := filepath.Base(path)
	var searchDir string

	if m.backupDir != "" {
		searchDir = m.backupDir
	} else {
		searchDir = filepath.Dir(path)
	}

	// Read directory
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Find matching backups
	var backups []string
	prefix := basename + ".backup_"

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			backupPath := filepath.Join(searchDir, entry.Name())
			backups = append(backups, backupPath)
		}
	}

	// Sort by timestamp (newest first)
	sort.Sort(sort.Reverse(sort.StringSlice(backups)))

	return backups, nil
}

// CleanupBackups removes old backups, keeping only the most recent N backups
func (m *BackupManager) CleanupBackups(path string, keepCount int) error {
	logger.Debug("cleaning up backups",
		logger.String("path", path),
		logger.Int("keepCount", keepCount))

	backups, err := m.ListBackups(path)
	if err != nil {
		return err
	}

	// Remove old backups
	if len(backups) > keepCount {
		for i := keepCount; i < len(backups); i++ {
			if err := os.Remove(backups[i]); err != nil {
				logger.Warn("failed to remove backup", logger.Err(err), logger.String("path", backups[i]))
			} else {
				logger.Debug("removed old backup", logger.String("path", backups[i]))
			}
		}
	}

	logger.Info("backup cleanup completed",
		logger.Int("totalBackups", len(backups)),
		logger.Int("kept", min(len(backups), keepCount)),
		logger.Int("removed", max(0, len(backups)-keepCount)))

	return nil
}

// DeleteBackup deletes a specific backup file
func (m *BackupManager) DeleteBackup(backupPath string) error {
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	logger.Info("backup deleted", logger.String("backupPath", backupPath))
	return nil
}

// GetLatestBackup returns the path to the most recent backup for a file
func (m *BackupManager) GetLatestBackup(path string) (string, error) {
	backups, err := m.ListBackups(path)
	if err != nil {
		return "", err
	}

	if len(backups) == 0 {
		return "", fmt.Errorf("no backups found for file: %s", path)
	}

	return backups[0], nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Sync to ensure data is written to disk
	if err := destFile.Sync(); err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
