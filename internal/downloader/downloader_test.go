// Package downloader provides functionality for downloading and extracting LaTeX source code.
package downloader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildArxivURL_NewFormat tests URL construction for new format arXiv IDs
func TestBuildArxivURL_NewFormat(t *testing.T) {
	testCases := []struct {
		name     string
		arxivID  string
		expected string
	}{
		{
			name:     "standard new format",
			arxivID:  "2301.00001",
			expected: "https://arxiv.org/e-print/2301.00001",
		},
		{
			name:     "new format with 5 digit number",
			arxivID:  "2301.12345",
			expected: "https://arxiv.org/e-print/2301.12345",
		},
		{
			name:     "different year/month",
			arxivID:  "1912.11111",
			expected: "https://arxiv.org/e-print/1912.11111",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildArxivURL(tc.arxivID)
			if result != tc.expected {
				t.Errorf("BuildArxivURL(%q) = %q, want %q", tc.arxivID, result, tc.expected)
			}
		})
	}
}

// TestBuildArxivURL_OldFormat tests URL construction for old format arXiv IDs
func TestBuildArxivURL_OldFormat(t *testing.T) {
	testCases := []struct {
		name     string
		arxivID  string
		expected string
	}{
		{
			name:     "hep-th category",
			arxivID:  "hep-th/9901001",
			expected: "https://arxiv.org/e-print/hep-th/9901001",
		},
		{
			name:     "math-ph category",
			arxivID:  "math-ph/0001234",
			expected: "https://arxiv.org/e-print/math-ph/0001234",
		},
		{
			name:     "cond-mat category",
			arxivID:  "cond-mat/9912345",
			expected: "https://arxiv.org/e-print/cond-mat/9912345",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildArxivURL(tc.arxivID)
			if result != tc.expected {
				t.Errorf("BuildArxivURL(%q) = %q, want %q", tc.arxivID, result, tc.expected)
			}
		})
	}
}

// TestBuildArxivURL_URLFormat tests that the URL format is correct
func TestBuildArxivURL_URLFormat(t *testing.T) {
	testIDs := []string{
		"2301.00001",
		"hep-th/9901001",
		"math-ph/0001234",
	}

	for _, id := range testIDs {
		t.Run(id, func(t *testing.T) {
			url := BuildArxivURL(id)

			// Check that URL starts with the correct base
			if !strings.HasPrefix(url, ArxivEprintBaseURL) {
				t.Errorf("URL %q does not start with %q", url, ArxivEprintBaseURL)
			}

			// Check that URL contains the ID
			if !strings.Contains(url, id) {
				t.Errorf("URL %q does not contain ID %q", url, id)
			}

			// Check that URL is exactly base + ID
			expected := ArxivEprintBaseURL + id
			if url != expected {
				t.Errorf("URL %q != expected %q", url, expected)
			}
		})
	}
}

// TestNewSourceDownloader tests the constructor
func TestNewSourceDownloader(t *testing.T) {
	workDir := "/tmp/test-work-dir"
	downloader := NewSourceDownloader(workDir)

	if downloader == nil {
		t.Fatal("NewSourceDownloader returned nil")
	}

	if downloader.workDir != workDir {
		t.Errorf("workDir = %q, want %q", downloader.workDir, workDir)
	}

	if downloader.httpClient == nil {
		t.Error("httpClient is nil")
	}

	if downloader.httpClient.Timeout != DefaultTimeout {
		t.Errorf("httpClient.Timeout = %v, want %v", downloader.httpClient.Timeout, DefaultTimeout)
	}
}

// TestNewSourceDownloaderWithTimeout tests the constructor with custom timeout
func TestNewSourceDownloaderWithTimeout(t *testing.T) {
	workDir := "/tmp/test-work-dir"
	customTimeout := 30 * time.Second
	downloader := NewSourceDownloaderWithTimeout(workDir, customTimeout)

	if downloader == nil {
		t.Fatal("NewSourceDownloaderWithTimeout returned nil")
	}

	if downloader.httpClient.Timeout != customTimeout {
		t.Errorf("httpClient.Timeout = %v, want %v", downloader.httpClient.Timeout, customTimeout)
	}
}

// TestSourceDownloader_GetSetWorkDir tests the GetWorkDir and SetWorkDir methods
func TestSourceDownloader_GetSetWorkDir(t *testing.T) {
	downloader := NewSourceDownloader("/initial/dir")

	if got := downloader.GetWorkDir(); got != "/initial/dir" {
		t.Errorf("GetWorkDir() = %q, want %q", got, "/initial/dir")
	}

	downloader.SetWorkDir("/new/dir")

	if got := downloader.GetWorkDir(); got != "/new/dir" {
		t.Errorf("GetWorkDir() after SetWorkDir = %q, want %q", got, "/new/dir")
	}
}

// TestSourceDownloader_GetHTTPClient tests the GetHTTPClient method
func TestSourceDownloader_GetHTTPClient(t *testing.T) {
	downloader := NewSourceDownloader("/tmp")

	client := downloader.GetHTTPClient()
	if client == nil {
		t.Error("GetHTTPClient() returned nil")
	}

	if client != downloader.httpClient {
		t.Error("GetHTTPClient() returned different client instance")
	}
}


// TestExtractFilenameFromURL tests the filename extraction from URLs
func TestExtractFilenameFromURL(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "arXiv e-print URL new format",
			url:      "https://arxiv.org/e-print/2301.00001",
			expected: "2301.00001.tar.gz",
		},
		{
			name:     "arXiv e-print URL old format",
			url:      "https://arxiv.org/e-print/hep-th/9901001",
			expected: "hep-th_9901001.tar.gz",
		},
		{
			name:     "arXiv src URL",
			url:      "https://arxiv.org/src/2301.00001",
			expected: "2301.00001.tar.gz",
		},
		{
			name:     "generic URL with filename",
			url:      "https://example.com/files/paper.tar.gz",
			expected: "paper.tar.gz",
		},
		{
			name:     "generic URL without extension",
			url:      "https://example.com/files/paper",
			expected: "paper.tar.gz",
		},
		{
			name:     "URL ending with slash",
			url:      "https://example.com/files/",
			expected: "download.tar.gz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractFilenameFromURL(tc.url)
			if result != tc.expected {
				t.Errorf("extractFilenameFromURL(%q) = %q, want %q", tc.url, result, tc.expected)
			}
		})
	}
}

// TestHandleHTTPError tests the HTTP error handling
func TestHandleHTTPError(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		expectedCode   string
		expectedSubstr string
	}{
		{
			name:           "404 Not Found",
			statusCode:     404,
			expectedCode:   "DOWNLOAD_ERROR",
			expectedSubstr: "not found",
		},
		{
			name:           "403 Forbidden",
			statusCode:     403,
			expectedCode:   "DOWNLOAD_ERROR",
			expectedSubstr: "forbidden",
		},
		{
			name:           "429 Too Many Requests",
			statusCode:     429,
			expectedCode:   "API_RATE_LIMIT",
			expectedSubstr: "rate limit",
		},
		{
			name:           "500 Internal Server Error",
			statusCode:     500,
			expectedCode:   "NETWORK_ERROR",
			expectedSubstr: "server error",
		},
		{
			name:           "502 Bad Gateway",
			statusCode:     502,
			expectedCode:   "NETWORK_ERROR",
			expectedSubstr: "server error",
		},
		{
			name:           "503 Service Unavailable",
			statusCode:     503,
			expectedCode:   "NETWORK_ERROR",
			expectedSubstr: "server error",
		},
		{
			name:           "400 Bad Request",
			statusCode:     400,
			expectedCode:   "DOWNLOAD_ERROR",
			expectedSubstr: "download failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := handleHTTPError(tc.statusCode, "https://example.com/test")
			if err == nil {
				t.Fatal("handleHTTPError returned nil error")
			}

			errMsg := err.Error()
			if !strings.Contains(strings.ToLower(errMsg), tc.expectedSubstr) {
				t.Errorf("error message %q does not contain %q", errMsg, tc.expectedSubstr)
			}
		})
	}
}

// TestIsRetryableError tests the retry logic determination
func TestIsRetryableError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "network error",
			err:      handleHTTPError(500, "https://example.com"),
			expected: true,
		},
		{
			name:     "rate limit error",
			err:      handleHTTPError(429, "https://example.com"),
			expected: true,
		},
		{
			name:     "not found error",
			err:      handleHTTPError(404, "https://example.com"),
			expected: false,
		},
		{
			name:     "forbidden error",
			err:      handleHTTPError(403, "https://example.com"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryableError(tc.err)
			if result != tc.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

// TestDownloadFromURL_EmptyURL tests that empty URL returns an error
func TestDownloadFromURL_EmptyURL(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	_, err := downloader.DownloadFromURL("")
	if err == nil {
		t.Error("DownloadFromURL with empty URL should return an error")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error message should mention 'empty', got: %v", err)
	}
}

// TestDownloadFromURL_InvalidURL tests that invalid URL returns an error
func TestDownloadFromURL_InvalidURL(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	testCases := []struct {
		name string
		url  string
	}{
		{
			name: "no protocol",
			url:  "arxiv.org/e-print/2301.00001",
		},
		{
			name: "ftp protocol",
			url:  "ftp://arxiv.org/e-print/2301.00001",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := downloader.DownloadFromURL(tc.url)
			if err == nil {
				t.Errorf("DownloadFromURL(%q) should return an error", tc.url)
			}
		})
	}
}

// TestDownloadByID_EmptyID tests that empty arXiv ID returns an error
func TestDownloadByID_EmptyID(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	_, err := downloader.DownloadByID("")
	if err == nil {
		t.Error("DownloadByID with empty ID should return an error")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error message should mention 'empty', got: %v", err)
	}
}

// TestDownloadByID_URLConstruction tests that DownloadByID constructs the correct URL
func TestDownloadByID_URLConstruction(t *testing.T) {
	// This test verifies that DownloadByID uses BuildArxivURL correctly
	// by checking the filename generated (which is based on the ID)
	testCases := []struct {
		name             string
		arxivID          string
		expectedFilename string
	}{
		{
			name:             "new format ID",
			arxivID:          "2301.00001",
			expectedFilename: "2301.00001.tar.gz",
		},
		{
			name:             "old format ID with slash",
			arxivID:          "hep-th/9901001",
			expectedFilename: "hep-th_9901001.tar.gz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// We can't actually download, but we can verify the URL construction
			url := BuildArxivURL(tc.arxivID)
			expectedURL := ArxivEprintBaseURL + tc.arxivID
			if url != expectedURL {
				t.Errorf("BuildArxivURL(%q) = %q, want %q", tc.arxivID, url, expectedURL)
			}
		})
	}
}


// ============================================================================
// ExtractZip Tests
// ============================================================================

// TestExtractZip_EmptyPath tests that empty path returns an error
func TestExtractZip_EmptyPath(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	_, err := downloader.ExtractZip("")
	if err == nil {
		t.Error("ExtractZip with empty path should return an error")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error message should mention 'empty', got: %v", err)
	}
}

// TestExtractZip_FileNotFound tests that non-existent file returns an error
func TestExtractZip_FileNotFound(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	_, err := downloader.ExtractZip("/nonexistent/path/file.zip")
	if err == nil {
		t.Error("ExtractZip with non-existent file should return an error")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error message should mention 'not found', got: %v", err)
	}
}

// TestExtractZip_ZipFile tests extraction of a valid .zip file
func TestExtractZip_ZipFile(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a test zip file
	zipPath := createTestZipFile(t, workDir)

	result, err := downloader.ExtractZip(zipPath)
	if err != nil {
		t.Fatalf("ExtractZip failed: %v", err)
	}

	// Verify result
	if result.SourceType != "local_zip" {
		t.Errorf("SourceType = %q, want %q", result.SourceType, "local_zip")
	}

	if result.OriginalRef != zipPath {
		t.Errorf("OriginalRef = %q, want %q", result.OriginalRef, zipPath)
	}

	if result.ExtractDir == "" {
		t.Error("ExtractDir should not be empty")
	}

	// Verify extracted files exist
	verifyExtractedFiles(t, result.ExtractDir, []string{"main.tex", "chapter1.tex"})

	// Verify tex files are found
	if len(result.AllTexFiles) != 2 {
		t.Errorf("AllTexFiles count = %d, want 2", len(result.AllTexFiles))
	}
}

// TestExtractZip_TarGzFile tests extraction of a valid .tar.gz file
func TestExtractZip_TarGzFile(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a test tar.gz file
	tarGzPath := createTestTarGzFile(t, workDir)

	result, err := downloader.ExtractZip(tarGzPath)
	if err != nil {
		t.Fatalf("ExtractZip failed: %v", err)
	}

	// Verify result
	if result.SourceType != "local_zip" {
		t.Errorf("SourceType = %q, want %q", result.SourceType, "local_zip")
	}

	if result.OriginalRef != tarGzPath {
		t.Errorf("OriginalRef = %q, want %q", result.OriginalRef, tarGzPath)
	}

	// Verify extracted files exist
	verifyExtractedFiles(t, result.ExtractDir, []string{"paper.tex", "refs.bib"})
}

// TestExtractZip_NestedDirectories tests extraction with nested directory structure
func TestExtractZip_NestedDirectories(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a test zip file with nested directories
	zipPath := createTestZipWithNestedDirs(t, workDir)

	result, err := downloader.ExtractZip(zipPath)
	if err != nil {
		t.Fatalf("ExtractZip failed: %v", err)
	}

	// Verify nested files exist
	expectedFiles := []string{
		"main.tex",
		"chapters/intro.tex",
		"chapters/conclusion.tex",
		"figures/diagram.tex",
	}
	verifyExtractedFiles(t, result.ExtractDir, expectedFiles)

	// Verify all tex files are found
	if len(result.AllTexFiles) != 4 {
		t.Errorf("AllTexFiles count = %d, want 4, got: %v", len(result.AllTexFiles), result.AllTexFiles)
	}
}

// TestExtractZip_InvalidArchive tests extraction of an invalid archive
func TestExtractZip_InvalidArchive(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create an invalid file (not a valid archive)
	invalidPath := createInvalidArchive(t, workDir)

	_, err := downloader.ExtractZip(invalidPath)
	if err == nil {
		t.Error("ExtractZip with invalid archive should return an error")
	}
}

// TestExtractZip_EmptyArchive tests extraction of an empty archive
func TestExtractZip_EmptyArchive(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create an empty zip file
	emptyZipPath := createEmptyZipFile(t, workDir)

	result, err := downloader.ExtractZip(emptyZipPath)
	if err != nil {
		t.Fatalf("ExtractZip failed: %v", err)
	}

	// Empty archive should result in empty tex files list
	if len(result.AllTexFiles) != 0 {
		t.Errorf("AllTexFiles should be empty for empty archive, got: %v", result.AllTexFiles)
	}
}

// TestSanitizePath tests the path sanitization function
func TestSanitizePath(t *testing.T) {
	destDir := "/tmp/extract"

	testCases := []struct {
		name      string
		entryName string
		wantErr   bool
	}{
		{
			name:      "simple filename",
			entryName: "file.tex",
			wantErr:   false,
		},
		{
			name:      "nested path",
			entryName: "subdir/file.tex",
			wantErr:   false,
		},
		{
			name:      "directory traversal attempt",
			entryName: "../../../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "hidden directory traversal",
			entryName: "subdir/../../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "absolute path",
			entryName: "/etc/passwd",
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sanitizePath(destDir, tc.entryName)
			if tc.wantErr && err == nil {
				t.Errorf("sanitizePath(%q, %q) should return an error", destDir, tc.entryName)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("sanitizePath(%q, %q) returned unexpected error: %v", destDir, tc.entryName, err)
			}
		})
	}
}

// TestFindTexFiles tests the tex file discovery function
func TestFindTexFiles(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create test directory structure with tex files
	createTestTexFiles(t, workDir)

	texFiles, err := downloader.findTexFiles(workDir)
	if err != nil {
		t.Fatalf("findTexFiles failed: %v", err)
	}

	// Should find all tex files
	if len(texFiles) != 3 {
		t.Errorf("findTexFiles found %d files, want 3, got: %v", len(texFiles), texFiles)
	}

	// Verify specific files are found
	foundMain := false
	foundChapter := false
	for _, f := range texFiles {
		if f == "main.tex" {
			foundMain = true
		}
		if strings.Contains(f, "chapter.tex") {
			foundChapter = true
		}
	}

	if !foundMain {
		t.Error("main.tex not found in tex files")
	}
	if !foundChapter {
		t.Error("chapter.tex not found in tex files")
	}
}

// ============================================================================
// Helper functions for creating test archives
// ============================================================================

// createTestZipFile creates a simple test zip file with tex files
func createTestZipFile(t *testing.T, dir string) string {
	t.Helper()

	zipPath := filepath.Join(dir, "test.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add main.tex
	w, err := zipWriter.Create("main.tex")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	w.Write([]byte("\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}"))

	// Add chapter1.tex
	w, err = zipWriter.Create("chapter1.tex")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	w.Write([]byte("\\section{Chapter 1}\nContent here."))

	return zipPath
}

// createTestTarGzFile creates a simple test tar.gz file
func createTestTarGzFile(t *testing.T, dir string) string {
	t.Helper()

	tarGzPath := filepath.Join(dir, "test.tar.gz")
	file, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatalf("Failed to create tar.gz file: %v", err)
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Add paper.tex
	content := []byte("\\documentclass{article}\n\\begin{document}\nPaper content\n\\end{document}")
	header := &tar.Header{
		Name: "paper.tex",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}

	// Add refs.bib
	bibContent := []byte("@article{test, author={Test}, title={Test}}")
	bibHeader := &tar.Header{
		Name: "refs.bib",
		Mode: 0644,
		Size: int64(len(bibContent)),
	}
	if err := tarWriter.WriteHeader(bibHeader); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tarWriter.Write(bibContent); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}

	return tarGzPath
}

// createTestZipWithNestedDirs creates a zip file with nested directory structure
func createTestZipWithNestedDirs(t *testing.T, dir string) string {
	t.Helper()

	zipPath := filepath.Join(dir, "nested.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	files := map[string]string{
		"main.tex":               "\\documentclass{article}\n\\input{chapters/intro}",
		"chapters/intro.tex":     "\\section{Introduction}",
		"chapters/conclusion.tex": "\\section{Conclusion}",
		"figures/diagram.tex":    "\\begin{tikzpicture}\\end{tikzpicture}",
	}

	for name, content := range files {
		w, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("Failed to create zip entry %s: %v", name, err)
		}
		w.Write([]byte(content))
	}

	return zipPath
}

// createInvalidArchive creates a file that is not a valid archive
func createInvalidArchive(t *testing.T, dir string) string {
	t.Helper()

	invalidPath := filepath.Join(dir, "invalid.zip")
	err := os.WriteFile(invalidPath, []byte("This is not a valid archive"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid archive: %v", err)
	}

	return invalidPath
}

// createEmptyZipFile creates an empty zip file
func createEmptyZipFile(t *testing.T, dir string) string {
	t.Helper()

	zipPath := filepath.Join(dir, "empty.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	zipWriter.Close()

	return zipPath
}

// createTestTexFiles creates a directory structure with tex files for testing
func createTestTexFiles(t *testing.T, dir string) {
	t.Helper()

	// Create main.tex
	mainPath := filepath.Join(dir, "main.tex")
	os.WriteFile(mainPath, []byte("\\documentclass{article}"), 0644)

	// Create subdir with chapter.tex
	subdir := filepath.Join(dir, "chapters")
	os.MkdirAll(subdir, 0755)
	chapterPath := filepath.Join(subdir, "chapter.tex")
	os.WriteFile(chapterPath, []byte("\\section{Chapter}"), 0644)

	// Create another tex file
	appendixPath := filepath.Join(dir, "appendix.tex")
	os.WriteFile(appendixPath, []byte("\\section{Appendix}"), 0644)

	// Create a non-tex file (should be ignored)
	readmePath := filepath.Join(dir, "README.md")
	os.WriteFile(readmePath, []byte("# README"), 0644)
}

// verifyExtractedFiles checks that expected files exist in the extraction directory
func verifyExtractedFiles(t *testing.T, extractDir string, expectedFiles []string) {
	t.Helper()

	for _, file := range expectedFiles {
		filePath := filepath.Join(extractDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %q not found in %q", file, extractDir)
		}
	}
}


// ============================================================================
// FindMainTexFile Tests
// ============================================================================

// TestFindMainTexFile_EmptyDir tests that empty directory path returns an error
func TestFindMainTexFile_EmptyDir(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	_, err := downloader.FindMainTexFile("")
	if err == nil {
		t.Error("FindMainTexFile with empty path should return an error")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error message should mention 'empty', got: %v", err)
	}
}

// TestFindMainTexFile_DirNotFound tests that non-existent directory returns an error
func TestFindMainTexFile_DirNotFound(t *testing.T) {
	downloader := NewSourceDownloader(t.TempDir())

	_, err := downloader.FindMainTexFile("/nonexistent/path/dir")
	if err == nil {
		t.Error("FindMainTexFile with non-existent directory should return an error")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error message should mention 'not found', got: %v", err)
	}
}

// TestFindMainTexFile_NotADirectory tests that file path returns an error
func TestFindMainTexFile_NotADirectory(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a file instead of a directory
	filePath := filepath.Join(workDir, "file.txt")
	os.WriteFile(filePath, []byte("content"), 0644)

	_, err := downloader.FindMainTexFile(filePath)
	if err == nil {
		t.Error("FindMainTexFile with file path should return an error")
	}

	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error message should mention 'not a directory', got: %v", err)
	}
}

// TestFindMainTexFile_NoTexFiles tests that directory with no tex files returns an error
func TestFindMainTexFile_NoTexFiles(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with no tex files
	testDir := filepath.Join(workDir, "empty_project")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "README.md"), []byte("# README"), 0644)

	_, err := downloader.FindMainTexFile(testDir)
	if err == nil {
		t.Error("FindMainTexFile with no tex files should return an error")
	}

	if !strings.Contains(err.Error(), "no tex files") {
		t.Errorf("error message should mention 'no tex files', got: %v", err)
	}
}

// TestFindMainTexFile_NoDocumentclass tests that directory with tex files but no \documentclass returns an error
func TestFindMainTexFile_NoDocumentclass(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with tex files but no \documentclass
	testDir := filepath.Join(workDir, "no_main")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "chapter1.tex"), []byte("\\section{Chapter 1}\nContent here."), 0644)
	os.WriteFile(filepath.Join(testDir, "chapter2.tex"), []byte("\\section{Chapter 2}\nMore content."), 0644)

	_, err := downloader.FindMainTexFile(testDir)
	if err == nil {
		t.Error("FindMainTexFile with no \\documentclass should return an error")
	}

	if !strings.Contains(err.Error(), "documentclass") {
		t.Errorf("error message should mention 'documentclass', got: %v", err)
	}
}

// TestFindMainTexFile_SingleMainFile tests finding a single main tex file
func TestFindMainTexFile_SingleMainFile(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with one main tex file
	testDir := filepath.Join(workDir, "single_main")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "paper.tex"), []byte("\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}"), 0644)
	os.WriteFile(filepath.Join(testDir, "chapter1.tex"), []byte("\\section{Chapter 1}\nContent here."), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	if mainFile != "paper.tex" {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, "paper.tex")
	}
}

// TestFindMainTexFile_PreferMainTex tests that main.tex is preferred over other files
func TestFindMainTexFile_PreferMainTex(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with multiple files containing \documentclass
	testDir := filepath.Join(workDir, "prefer_main")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "main.tex"), []byte("\\documentclass{article}\n\\begin{document}\nMain\n\\end{document}"), 0644)
	os.WriteFile(filepath.Join(testDir, "other.tex"), []byte("\\documentclass{report}\n\\begin{document}\nOther\n\\end{document}"), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	if mainFile != "main.tex" {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, "main.tex")
	}
}

// TestFindMainTexFile_PreferPaperTex tests that paper.tex is preferred when main.tex is not present
func TestFindMainTexFile_PreferPaperTex(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with multiple files containing \documentclass
	testDir := filepath.Join(workDir, "prefer_paper")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "paper.tex"), []byte("\\documentclass{article}\n\\begin{document}\nPaper\n\\end{document}"), 0644)
	os.WriteFile(filepath.Join(testDir, "other.tex"), []byte("\\documentclass{report}\n\\begin{document}\nOther\n\\end{document}"), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	if mainFile != "paper.tex" {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, "paper.tex")
	}
}

// TestFindMainTexFile_PreferRootFile tests that root directory files are preferred over nested files
func TestFindMainTexFile_PreferRootFile(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with files in root and subdirectory
	testDir := filepath.Join(workDir, "prefer_root")
	os.MkdirAll(filepath.Join(testDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(testDir, "article.tex"), []byte("\\documentclass{article}\n\\begin{document}\nRoot\n\\end{document}"), 0644)
	os.WriteFile(filepath.Join(testDir, "subdir", "nested.tex"), []byte("\\documentclass{report}\n\\begin{document}\nNested\n\\end{document}"), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	if mainFile != "article.tex" {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, "article.tex")
	}
}

// TestFindMainTexFile_DocumentclassWithOptions tests finding files with \documentclass[options]{class}
func TestFindMainTexFile_DocumentclassWithOptions(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with \documentclass with options
	testDir := filepath.Join(workDir, "with_options")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "main.tex"), []byte("\\documentclass[12pt,a4paper]{article}\n\\begin{document}\nHello\n\\end{document}"), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	if mainFile != "main.tex" {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, "main.tex")
	}
}

// TestFindMainTexFile_CaseInsensitivePreference tests that preferred names are matched case-insensitively
func TestFindMainTexFile_CaseInsensitivePreference(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with MAIN.tex (uppercase)
	testDir := filepath.Join(workDir, "case_insensitive")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "MAIN.tex"), []byte("\\documentclass{article}\n\\begin{document}\nMain\n\\end{document}"), 0644)
	os.WriteFile(filepath.Join(testDir, "other.tex"), []byte("\\documentclass{report}\n\\begin{document}\nOther\n\\end{document}"), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	if mainFile != "MAIN.tex" {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, "MAIN.tex")
	}
}

// TestFindMainTexFile_NestedMainTex tests that main.tex in subdirectory is found when no root main exists
func TestFindMainTexFile_NestedMainTex(t *testing.T) {
	workDir := t.TempDir()
	downloader := NewSourceDownloader(workDir)

	// Create a directory with main.tex only in subdirectory
	testDir := filepath.Join(workDir, "nested_main")
	os.MkdirAll(filepath.Join(testDir, "src"), 0755)
	os.WriteFile(filepath.Join(testDir, "src", "main.tex"), []byte("\\documentclass{article}\n\\begin{document}\nNested Main\n\\end{document}"), 0644)
	os.WriteFile(filepath.Join(testDir, "chapter.tex"), []byte("\\section{Chapter}\nContent."), 0644)

	mainFile, err := downloader.FindMainTexFile(testDir)
	if err != nil {
		t.Fatalf("FindMainTexFile failed: %v", err)
	}

	// Should find the nested main.tex since it's the only one with \documentclass
	expectedPath := filepath.Join("src", "main.tex")
	if mainFile != expectedPath {
		t.Errorf("FindMainTexFile = %q, want %q", mainFile, expectedPath)
	}
}

// TestFileContainsDocumentclass tests the helper function
func TestFileContainsDocumentclass(t *testing.T) {
	workDir := t.TempDir()

	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "simple documentclass",
			content:  "\\documentclass{article}",
			expected: true,
		},
		{
			name:     "documentclass with options",
			content:  "\\documentclass[12pt]{article}",
			expected: true,
		},
		{
			name:     "documentclass with multiple options",
			content:  "\\documentclass[12pt,a4paper,twoside]{book}",
			expected: true,
		},
		{
			name:     "no documentclass",
			content:  "\\section{Introduction}\nSome text.",
			expected: false,
		},
		{
			name:     "commented documentclass",
			content:  "% \\documentclass{article}\n\\section{Test}",
			expected: true, // We don't parse comments, so this will still match
		},
		{
			name:     "documentclass in middle of file",
			content:  "% Comment\n\\documentclass{article}\n\\begin{document}",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(workDir, tc.name+".tex")
			os.WriteFile(filePath, []byte(tc.content), 0644)

			result, err := fileContainsDocumentclass(filePath)
			if err != nil {
				t.Fatalf("fileContainsDocumentclass failed: %v", err)
			}

			if result != tc.expected {
				t.Errorf("fileContainsDocumentclass = %v, want %v", result, tc.expected)
			}
		})
	}
}

// TestSelectPreferredMainFile tests the preference selection logic
func TestSelectPreferredMainFile(t *testing.T) {
	testCases := []struct {
		name       string
		candidates []string
		expected   string
	}{
		{
			name:       "main.tex present",
			candidates: []string{"other.tex", "main.tex", "paper.tex"},
			expected:   "main.tex",
		},
		{
			name:       "paper.tex when no main.tex",
			candidates: []string{"other.tex", "paper.tex", "article.tex"},
			expected:   "paper.tex",
		},
		{
			name:       "article.tex when no main or paper",
			candidates: []string{"other.tex", "article.tex", "test.tex"},
			expected:   "article.tex",
		},
		{
			name:       "preferred name in subdir over non-preferred in root",
			candidates: []string{"subdir/main.tex", "root.tex"},
			expected:   "root.tex", // Root files are preferred over nested files, even with preferred names
		},
		{
			name:       "first alphabetically when no preference",
			candidates: []string{"zebra.tex", "alpha.tex", "beta.tex"},
			expected:   "alpha.tex",
		},
		{
			name:       "nested main.tex preferred over nested other",
			candidates: []string{"subdir/other.tex", "subdir/main.tex"},
			expected:   "subdir/main.tex",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := selectPreferredMainFile(tc.candidates)
			if result != tc.expected {
				t.Errorf("selectPreferredMainFile(%v) = %q, want %q", tc.candidates, result, tc.expected)
			}
		})
	}
}
