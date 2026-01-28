// Package downloader provides functionality for downloading and extracting LaTeX source code.
package downloader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"latex-translator/internal/logger"
	"latex-translator/internal/types"
)

const (
	// DefaultTimeout is the default HTTP client timeout for downloads
	// Increased to 5 minutes to handle large files (e.g., 15MB+ arXiv sources)
	DefaultTimeout = 300 * time.Second
	// ArxivEprintBaseURL is the base URL for arXiv e-print downloads
	ArxivEprintBaseURL = "https://arxiv.org/e-print/"
	// MaxRetries is the maximum number of retry attempts for network errors
	MaxRetries = 3
	// BaseRetryDelay is the base delay between retries (will be multiplied by attempt number)
	BaseRetryDelay = 2 * time.Second
)

// SourceDownloader is responsible for downloading and extracting LaTeX source code.
type SourceDownloader struct {
	httpClient *http.Client
	workDir    string
}

// NewSourceDownloader creates a new SourceDownloader with the specified work directory.
// It configures an HTTP client with appropriate timeout and settings for downloading.
func NewSourceDownloader(workDir string) *SourceDownloader {
	return &SourceDownloader{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			// Don't follow redirects automatically to handle them manually if needed
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return types.NewAppError(types.ErrNetwork, "too many redirects", nil)
				}
				return nil
			},
		},
		workDir: workDir,
	}
}

// NewSourceDownloaderWithTimeout creates a new SourceDownloader with a custom timeout.
func NewSourceDownloaderWithTimeout(workDir string, timeout time.Duration) *SourceDownloader {
	return &SourceDownloader{
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return types.NewAppError(types.ErrNetwork, "too many redirects", nil)
				}
				return nil
			},
		},
		workDir: workDir,
	}
}

// BuildArxivURL constructs the download URL for an arXiv paper given its ID.
// The function supports both new format IDs (e.g., "2301.00001") and
// old format IDs (e.g., "hep-th/9901001").
//
// The returned URL follows the arXiv e-print source download format:
// https://arxiv.org/e-print/{id}
//
// Property 1: For any valid arXiv ID (new format like "2301.00001" or old format
// like "hep-th/9901001"), the constructed download URL should conform to arXiv's
// e-print source download format `https://arxiv.org/e-print/{id}`.
//
// Validates: Requirements 1.2
func BuildArxivURL(arxivID string) string {
	return fmt.Sprintf("%s%s", ArxivEprintBaseURL, arxivID)
}

// GetWorkDir returns the work directory for the downloader.
func (d *SourceDownloader) GetWorkDir() string {
	return d.workDir
}

// SetWorkDir sets the work directory for the downloader.
func (d *SourceDownloader) SetWorkDir(workDir string) {
	d.workDir = workDir
}

// GetHTTPClient returns the HTTP client used by the downloader.
func (d *SourceDownloader) GetHTTPClient() *http.Client {
	return d.httpClient
}

// DownloadFromURL downloads LaTeX source code from an arXiv URL.
// It validates the URL, downloads the content with retry logic, and saves it to the work directory.
//
// The function supports arXiv e-print URLs in the format:
// - https://arxiv.org/e-print/{id}
// - https://arxiv.org/src/{id}
// - https://export.arxiv.org/e-print/{id}
//
// Returns a SourceInfo with the downloaded file information, or an error if the download fails.
//
// Validates: Requirements 1.1, 1.4
func (d *SourceDownloader) DownloadFromURL(url string) (*types.SourceInfo, error) {
	logger.Info("downloading from URL", logger.String("url", url))

	if url == "" {
		logger.Warn("download failed: empty URL")
		return nil, types.NewAppError(types.ErrInvalidInput, "URL cannot be empty", nil)
	}

	// Validate URL format
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		logger.Warn("download failed: invalid URL format", logger.String("url", url))
		return nil, types.NewAppError(types.ErrInvalidInput, "invalid URL format: must start with http:// or https://", nil)
	}

	// Ensure work directory exists
	if err := os.MkdirAll(d.workDir, 0755); err != nil {
		logger.Error("failed to create work directory", err, logger.String("workDir", d.workDir))
		return nil, types.NewAppError(types.ErrInternal, "failed to create work directory", err)
	}

	// Extract filename from URL for saving
	filename := extractFilenameFromURL(url)
	destPath := filepath.Join(d.workDir, filename)
	logger.Debug("download destination", logger.String("path", destPath))

	// Download with retry logic
	if err := d.downloadWithRetry(url, destPath); err != nil {
		return nil, err
	}

	logger.Info("download completed successfully", logger.String("url", url), logger.String("destPath", destPath))
	return &types.SourceInfo{
		SourceType:  types.SourceTypeURL,
		OriginalRef: url,
		ExtractDir:  destPath,
	}, nil
}

// DownloadByID downloads LaTeX source code using an arXiv ID.
// It constructs the download URL using BuildArxivURL and delegates to DownloadFromURL.
//
// The function supports both new format IDs (e.g., "2301.00001") and
// old format IDs (e.g., "hep-th/9901001").
//
// Returns a SourceInfo with the downloaded file information, or an error if the download fails.
//
// Validates: Requirements 1.2, 1.4
func (d *SourceDownloader) DownloadByID(arxivID string) (*types.SourceInfo, error) {
	logger.Info("downloading by arXiv ID", logger.String("arxivID", arxivID))

	if arxivID == "" {
		logger.Warn("download failed: empty arXiv ID")
		return nil, types.NewAppError(types.ErrInvalidInput, "arXiv ID cannot be empty", nil)
	}

	// Build the download URL from the arXiv ID
	url := BuildArxivURL(arxivID)
	logger.Debug("constructed arXiv URL", logger.String("url", url))

	// Ensure work directory exists
	if err := os.MkdirAll(d.workDir, 0755); err != nil {
		logger.Error("failed to create work directory", err, logger.String("workDir", d.workDir))
		return nil, types.NewAppError(types.ErrInternal, "failed to create work directory", err)
	}

	// Generate filename from arXiv ID (replace / with _ for old format IDs)
	filename := strings.ReplaceAll(arxivID, "/", "_") + ".tar.gz"
	destPath := filepath.Join(d.workDir, filename)

	// Download with retry logic
	if err := d.downloadWithRetry(url, destPath); err != nil {
		return nil, err
	}

	logger.Info("download by ID completed successfully", logger.String("arxivID", arxivID), logger.String("destPath", destPath))
	return &types.SourceInfo{
		SourceType:  types.SourceTypeArxivID,
		OriginalRef: arxivID,
		ExtractDir:  destPath,
	}, nil
}

// downloadWithRetry performs an HTTP GET request with retry logic for network errors.
// It retries up to MaxRetries times with increasing delays between attempts.
//
// Validates: Requirements 1.4
func (d *SourceDownloader) downloadWithRetry(url, destPath string) error {
	var lastErr error

	for attempt := 1; attempt <= MaxRetries; attempt++ {
		logger.Debug("download attempt", logger.Int("attempt", attempt), logger.String("url", url))
		err := d.downloadFile(url, destPath)
		if err == nil {
			return nil
		}

		lastErr = err
		logger.Warn("download attempt failed", logger.Int("attempt", attempt), logger.Err(err))

		// Check if the error is retryable (network errors)
		if !isRetryableError(err) {
			logger.Error("non-retryable download error", err, logger.String("url", url))
			return err
		}

		// Don't sleep after the last attempt
		if attempt < MaxRetries {
			delay := BaseRetryDelay * time.Duration(attempt)
			logger.Debug("retrying after delay", logger.String("delay", delay.String()))
			time.Sleep(delay)
		}
	}

	logger.Error("download failed after all retries", lastErr, logger.String("url", url), logger.Int("maxRetries", MaxRetries))
	return types.NewAppErrorWithDetails(
		types.ErrNetwork,
		"download failed after multiple retries",
		fmt.Sprintf("attempted %d times", MaxRetries),
		lastErr,
	)
}

// downloadFile performs the actual HTTP download and saves the content to a file.
func (d *SourceDownloader) downloadFile(url, destPath string) error {
	// Create HTTP request
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return types.NewAppError(types.ErrInternal, "failed to create HTTP request", err)
	}

	// Set User-Agent header (arXiv may require this)
	req.Header.Set("User-Agent", "LaTeX-Translator/1.0")

	// Perform the request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return types.NewAppError(types.ErrNetwork, "network request failed", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return handleHTTPError(resp.StatusCode, url)
	}

	// Check if arXiv returned a PDF instead of source code
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "pdf") {
		logger.Warn("arXiv returned PDF instead of source code", 
			logger.String("url", url),
			logger.String("contentType", contentType))
		return types.NewAppErrorWithDetails(
			types.ErrDownload,
			"论文源码不可用",
			"arXiv 返回的是 PDF 文件而不是 LaTeX 源码。这篇论文可能没有上传源码，或者作者选择不公开源码。",
			nil,
		)
	}

	// Create the destination file
	file, err := os.Create(destPath)
	if err != nil {
		return types.NewAppError(types.ErrInternal, "failed to create destination file", err)
	}
	defer file.Close()

	// Copy the response body to the file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		// Clean up partial file on error
		os.Remove(destPath)
		return types.NewAppError(types.ErrNetwork, "failed to save downloaded content", err)
	}

	return nil
}

// extractFilenameFromURL extracts a filename from a URL.
// For arXiv URLs, it uses the paper ID as the filename.
func extractFilenameFromURL(url string) string {
	// Try to extract arXiv ID from URL
	if strings.Contains(url, "arxiv.org") {
		// Handle e-print URLs: https://arxiv.org/e-print/2301.00001
		if idx := strings.LastIndex(url, "/e-print/"); idx != -1 {
			id := url[idx+len("/e-print/"):]
			return strings.ReplaceAll(id, "/", "_") + ".tar.gz"
		}
		// Handle src URLs: https://arxiv.org/src/2301.00001
		if idx := strings.LastIndex(url, "/src/"); idx != -1 {
			id := url[idx+len("/src/"):]
			return strings.ReplaceAll(id, "/", "_") + ".tar.gz"
		}
	}

	// Default: use the last path segment
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		if filename != "" {
			// Add extension if not present
			if !strings.Contains(filename, ".") {
				filename += ".tar.gz"
			}
			return filename
		}
	}

	// Fallback filename
	return "download.tar.gz"
}

// handleHTTPError creates an appropriate AppError based on the HTTP status code.
func handleHTTPError(statusCode int, url string) error {
	switch statusCode {
	case http.StatusNotFound:
		return types.NewAppErrorWithDetails(
			types.ErrDownload,
			"resource not found",
			fmt.Sprintf("URL: %s returned 404", url),
			nil,
		)
	case http.StatusForbidden:
		return types.NewAppErrorWithDetails(
			types.ErrDownload,
			"access forbidden",
			fmt.Sprintf("URL: %s returned 403", url),
			nil,
		)
	case http.StatusTooManyRequests:
		return types.NewAppErrorWithDetails(
			types.ErrAPIRateLimit,
			"rate limit exceeded",
			"too many requests, please try again later",
			nil,
		)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return types.NewAppErrorWithDetails(
			types.ErrNetwork,
			"server error",
			fmt.Sprintf("URL: %s returned %d", url, statusCode),
			nil,
		)
	default:
		return types.NewAppErrorWithDetails(
			types.ErrDownload,
			"download failed",
			fmt.Sprintf("URL: %s returned status %d", url, statusCode),
			nil,
		)
	}
}

// isRetryableError determines if an error should trigger a retry.
// Network errors and server errors (5xx) are retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's an AppError
	if appErr, ok := err.(*types.AppError); ok {
		switch appErr.Code {
		case types.ErrNetwork:
			return true
		case types.ErrAPIRateLimit:
			return true
		default:
			return false
		}
	}

	// For other errors, assume they might be network-related
	return true
}


// ExtractZip extracts a local zip or tar.gz file to the work directory.
// It supports both .zip and .tar.gz formats (arXiv uses tar.gz).
// The function handles nested directory structures and returns information about
// the extracted files including all .tex files found.
//
// Property 2: For any zip file containing LaTeX source code, the extracted file set
// should be identical to the original zip contents (both filenames and content).
//
// Validates: Requirements 1.3
func (d *SourceDownloader) ExtractZip(zipPath string) (*types.SourceInfo, error) {
	logger.Info("extracting archive", logger.String("path", zipPath))

	if zipPath == "" {
		logger.Warn("extraction failed: empty path")
		return nil, types.NewAppError(types.ErrInvalidInput, "zip path cannot be empty", nil)
	}

	// Check if file exists
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		logger.Error("archive file not found", err, logger.String("path", zipPath))
		return nil, types.NewAppError(types.ErrFileNotFound, "zip file not found", err)
	}

	// Ensure work directory exists
	if err := os.MkdirAll(d.workDir, 0755); err != nil {
		logger.Error("failed to create work directory", err, logger.String("workDir", d.workDir))
		return nil, types.NewAppError(types.ErrInternal, "failed to create work directory", err)
	}

	// Create extraction directory based on the archive filename
	baseName := filepath.Base(zipPath)
	// Remove all extensions (.tar.gz, .zip, etc.)
	extractName := strings.TrimSuffix(strings.TrimSuffix(baseName, ".gz"), ".tar")
	extractName = strings.TrimSuffix(extractName, ".zip")
	extractDir := filepath.Join(d.workDir, extractName+"_extracted")
	logger.Debug("extraction directory", logger.String("extractDir", extractDir))

	// Clean up existing extraction directory if it exists
	if err := os.RemoveAll(extractDir); err != nil {
		logger.Error("failed to clean extraction directory", err, logger.String("extractDir", extractDir))
		return nil, types.NewAppError(types.ErrInternal, "failed to clean extraction directory", err)
	}

	// Create fresh extraction directory
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		logger.Error("failed to create extraction directory", err, logger.String("extractDir", extractDir))
		return nil, types.NewAppError(types.ErrInternal, "failed to create extraction directory", err)
	}

	// Determine archive type and extract accordingly
	var err error
	lowerPath := strings.ToLower(zipPath)
	if strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz") {
		logger.Debug("extracting tar.gz archive")
		err = d.extractTarGz(zipPath, extractDir)
	} else if strings.HasSuffix(lowerPath, ".zip") {
		logger.Debug("extracting zip archive")
		err = d.extractZipFile(zipPath, extractDir)
	} else {
		// Try to detect format by reading file header
		logger.Debug("detecting archive format by header")
		err = d.extractByDetection(zipPath, extractDir)
	}

	if err != nil {
		// Clean up on error
		os.RemoveAll(extractDir)
		logger.Error("extraction failed", err, logger.String("path", zipPath))
		return nil, err
	}

	// Find all .tex files in the extracted directory
	texFiles, err := d.findTexFiles(extractDir)
	if err != nil {
		logger.Error("failed to scan for tex files", err, logger.String("extractDir", extractDir))
		return nil, types.NewAppError(types.ErrInternal, "failed to scan for tex files", err)
	}

	logger.Info("extraction completed successfully",
		logger.String("extractDir", extractDir),
		logger.Int("texFilesFound", len(texFiles)))

	return &types.SourceInfo{
		SourceType:  types.SourceTypeLocalZip,
		OriginalRef: zipPath,
		ExtractDir:  extractDir,
		AllTexFiles: texFiles,
	}, nil
}

// extractTarGz extracts a .tar.gz archive to the destination directory.
func (d *SourceDownloader) extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to open archive", err)
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to create gzip reader", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return types.NewAppError(types.ErrExtract, "failed to read tar entry", err)
		}

		// Sanitize the path to prevent directory traversal attacks
		targetPath, err := sanitizePath(destDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return types.NewAppError(types.ErrExtract, "failed to create directory", err)
			}
		case tar.TypeReg:
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return types.NewAppError(types.ErrExtract, "failed to create parent directory", err)
			}

			// Create file
			outFile, err := os.Create(targetPath)
			if err != nil {
				return types.NewAppError(types.ErrExtract, "failed to create file", err)
			}

			// Copy content with size limit to prevent zip bombs
			_, err = io.Copy(outFile, io.LimitReader(tarReader, header.Size))
			outFile.Close()
			if err != nil {
				return types.NewAppError(types.ErrExtract, "failed to write file content", err)
			}

			// Set file permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				// Non-fatal: just log and continue
			}
		case tar.TypeSymlink:
			// Handle symlinks carefully to prevent security issues
			linkTarget, err := sanitizePath(destDir, header.Linkname)
			if err != nil {
				// Skip symlinks that point outside the extraction directory
				continue
			}
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return types.NewAppError(types.ErrExtract, "failed to create parent directory for symlink", err)
			}
			// Create relative symlink
			relTarget, err := filepath.Rel(filepath.Dir(targetPath), linkTarget)
			if err != nil {
				continue
			}
			if err := os.Symlink(relTarget, targetPath); err != nil {
				// Non-fatal: skip symlinks that fail
				continue
			}
		}
	}

	return nil
}

// extractZipFile extracts a .zip archive to the destination directory.
func (d *SourceDownloader) extractZipFile(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to open zip file", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		// Sanitize the path to prevent directory traversal attacks
		targetPath, err := sanitizePath(destDir, file.Name)
		if err != nil {
			return err
		}

		if file.FileInfo().IsDir() {
			// Create directory
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return types.NewAppError(types.ErrExtract, "failed to create directory", err)
			}
			continue
		}

		// Create parent directory if needed
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return types.NewAppError(types.ErrExtract, "failed to create parent directory", err)
		}

		// Extract file
		if err := d.extractZipEntry(file, targetPath); err != nil {
			return err
		}
	}

	return nil
}

// extractZipEntry extracts a single zip entry to the target path.
func (d *SourceDownloader) extractZipEntry(file *zip.File, targetPath string) error {
	srcFile, err := file.Open()
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to open zip entry", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(targetPath)
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to create destination file", err)
	}
	defer destFile.Close()

	// Copy with size limit to prevent zip bombs
	_, err = io.Copy(destFile, io.LimitReader(srcFile, int64(file.UncompressedSize64)))
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to write file content", err)
	}

	// Set file permissions
	if err := os.Chmod(targetPath, file.Mode()); err != nil {
		// Non-fatal: just continue
	}

	return nil
}

// extractByDetection tries to detect the archive format by reading the file header
// and extracts accordingly.
func (d *SourceDownloader) extractByDetection(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to open file", err)
	}
	defer file.Close()

	// Read first few bytes to detect format
	header := make([]byte, 4)
	_, err = file.Read(header)
	if err != nil {
		return types.NewAppError(types.ErrExtract, "failed to read file header", err)
	}
	file.Close()

	// Check for gzip magic number (1f 8b)
	if header[0] == 0x1f && header[1] == 0x8b {
		return d.extractTarGz(archivePath, destDir)
	}

	// Check for zip magic number (50 4b 03 04)
	if header[0] == 0x50 && header[1] == 0x4b && header[2] == 0x03 && header[3] == 0x04 {
		return d.extractZipFile(archivePath, destDir)
	}

	return types.NewAppError(types.ErrExtract, "unsupported archive format", nil)
}

// sanitizePath ensures the target path is within the destination directory
// to prevent directory traversal attacks (zip slip vulnerability).
func sanitizePath(destDir, entryName string) (string, error) {
	// Clean the entry name to remove any .. or other path manipulation
	cleanName := filepath.Clean(entryName)

	// Reject absolute paths (both Unix and Windows style)
	if filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "\\") {
		return "", types.NewAppError(types.ErrExtract, "invalid path: absolute paths not allowed", nil)
	}

	// Reject paths that try to escape the destination directory
	if strings.HasPrefix(cleanName, "..") || strings.Contains(cleanName, string(filepath.Separator)+"..") {
		return "", types.NewAppError(types.ErrExtract, "invalid path: directory traversal not allowed", nil)
	}

	targetPath := filepath.Join(destDir, cleanName)

	// Double-check that the resolved path is within destDir
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", types.NewAppError(types.ErrExtract, "failed to resolve path", err)
	}

	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return "", types.NewAppError(types.ErrExtract, "failed to resolve destination", err)
	}

	// Ensure the target is within the destination directory
	if !strings.HasPrefix(absTarget, absDest+string(filepath.Separator)) && absTarget != absDest {
		return "", types.NewAppError(types.ErrExtract, "invalid path: outside destination directory", nil)
	}

	return targetPath, nil
}

// findTexFiles recursively finds all .tex files in the given directory.
func (d *SourceDownloader) findTexFiles(dir string) ([]string, error) {
	var texFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".tex") {
			// Store relative path from the extraction directory
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				relPath = path
			}
			texFiles = append(texFiles, relPath)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return texFiles, nil
}


// preferredMainTexNames is a list of common main tex file names in order of preference.
var preferredMainTexNames = []string{
	"main.tex",
	"paper.tex",
	"article.tex",
	"manuscript.tex",
	"document.tex",
	"thesis.tex",
	"report.tex",
}

// FindMainTexFile finds the main tex file in a directory by searching for files
// containing the \documentclass command.
//
// The function searches all .tex files in the directory and its subdirectories
// for the \documentclass command. If multiple files contain this command,
// it prefers files with common main tex file names (main.tex, paper.tex, etc.).
//
// Property 3: For any set of tex files containing the \documentclass command,
// FindMainTexFile should return the path of a file containing that command.
//
// Validates: Requirements 1.5
func (d *SourceDownloader) FindMainTexFile(dir string) (string, error) {
	logger.Info("finding main tex file", logger.String("dir", dir))

	if dir == "" {
		logger.Warn("find main tex file failed: empty directory path")
		return "", types.NewAppError(types.ErrInvalidInput, "directory path cannot be empty", nil)
	}

	// Check if directory exists
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		logger.Error("directory not found", err, logger.String("dir", dir))
		return "", types.NewAppError(types.ErrFileNotFound, "directory not found", err)
	}
	if err != nil {
		logger.Error("failed to access directory", err, logger.String("dir", dir))
		return "", types.NewAppError(types.ErrInternal, "failed to access directory", err)
	}
	if !info.IsDir() {
		logger.Warn("path is not a directory", logger.String("path", dir))
		return "", types.NewAppError(types.ErrInvalidInput, "path is not a directory", nil)
	}

	// Find all tex files in the directory
	texFiles, err := d.findTexFiles(dir)
	if err != nil {
		logger.Error("failed to scan for tex files", err, logger.String("dir", dir))
		return "", types.NewAppError(types.ErrInternal, "failed to scan for tex files", err)
	}

	if len(texFiles) == 0 {
		logger.Warn("no tex files found in directory", logger.String("dir", dir))
		return "", types.NewAppError(types.ErrFileNotFound, "no tex files found in directory", nil)
	}

	logger.Debug("found tex files", logger.Int("count", len(texFiles)))

	// Find all files containing \documentclass
	var filesWithDocumentclass []string
	for _, texFile := range texFiles {
		fullPath := filepath.Join(dir, texFile)
		hasDocumentclass, err := fileContainsDocumentclass(fullPath)
		if err != nil {
			// Skip files that can't be read
			logger.Debug("skipping unreadable file", logger.String("file", texFile), logger.Err(err))
			continue
		}
		if hasDocumentclass {
			filesWithDocumentclass = append(filesWithDocumentclass, texFile)
		}
	}

	if len(filesWithDocumentclass) == 0 {
		logger.Warn("no main tex file found (no file contains \\documentclass)", logger.String("dir", dir))
		return "", types.NewAppError(types.ErrFileNotFound, "no main tex file found (no file contains \\documentclass)", nil)
	}

	// If only one file has \documentclass, return it
	if len(filesWithDocumentclass) == 1 {
		logger.Info("found main tex file", logger.String("file", filesWithDocumentclass[0]))
		return filesWithDocumentclass[0], nil
	}

	// Multiple files have \documentclass, prefer common main file names
	mainFile := selectPreferredMainFile(filesWithDocumentclass)
	logger.Info("selected main tex file from multiple candidates",
		logger.String("file", mainFile),
		logger.Int("candidates", len(filesWithDocumentclass)))
	return mainFile, nil
}

// fileContainsDocumentclass checks if a file contains the \documentclass command.
func fileContainsDocumentclass(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	// Search for \documentclass command
	// It can appear as \documentclass{...} or \documentclass[...]{...}
	contentStr := string(content)
	return strings.Contains(contentStr, "\\documentclass"), nil
}

// selectPreferredMainFile selects the preferred main file from a list of candidates.
// It prefers files with common main tex file names in the root directory first,
// then preferred names in subdirectories, then any root file, and finally
// returns the first file alphabetically.
func selectPreferredMainFile(candidates []string) string {
	// Sort candidates for consistent ordering
	sortedCandidates := make([]string, len(candidates))
	copy(sortedCandidates, candidates)
	sortStrings(sortedCandidates)

	// Separate root files and nested files
	var rootFiles []string
	var nestedFiles []string
	for _, candidate := range sortedCandidates {
		// Check for both Unix and Windows path separators
		if !strings.Contains(candidate, "/") && !strings.Contains(candidate, "\\") {
			rootFiles = append(rootFiles, candidate)
		} else {
			nestedFiles = append(nestedFiles, candidate)
		}
	}

	// First, check for preferred file names in root directory
	for _, preferredName := range preferredMainTexNames {
		for _, candidate := range rootFiles {
			baseName := filepath.Base(candidate)
			if strings.EqualFold(baseName, preferredName) {
				return candidate
			}
		}
	}

	// Second, prefer any root file over nested files
	if len(rootFiles) > 0 {
		// Return the first root file alphabetically (already sorted)
		return rootFiles[0]
	}

	// Third, check for preferred file names in nested directories
	for _, preferredName := range preferredMainTexNames {
		for _, candidate := range nestedFiles {
			baseName := filepath.Base(candidate)
			if strings.EqualFold(baseName, preferredName) {
				return candidate
			}
		}
	}

	// Return the first candidate alphabetically (already sorted)
	return sortedCandidates[0]
}

// sortStrings sorts a slice of strings in place alphabetically
func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
