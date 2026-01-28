package github

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Uploader handles file uploads to GitHub
type Uploader struct {
	token  string
	owner  string
	repo   string
	client *http.Client
}

// UploadResult represents the result of an upload operation
type UploadResult struct {
	Success bool
	URL     string
	Message string
}

// FileExistsResult represents the result of a file existence check
type FileExistsResult struct {
	Exists bool
	URL    string
	SHA    string // SHA of the existing file (needed for updates)
}

// NewUploader creates a new GitHub uploader
func NewUploader(token, owner, repo string) *Uploader {
	return &Uploader{
		token: token,
		owner: owner,
		repo:  repo,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ValidateToken validates the GitHub token by making a test API call
func (u *Uploader) ValidateToken() error {
	if u.token == "" {
		return errors.New("GitHub token is not configured")
	}

	// Test the token by getting the authenticated user
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+u.token)
req.Header.Set("Accept", "application/vnd.github.v3+json")

resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to validate token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return errors.New("GitHub token is invalid or expired")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %s", string(body))
	}

	return nil
}

// CheckFileExists checks if a file exists in the repository
func (u *Uploader) CheckFileExists(path string) (*FileExistsResult, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", u.owner, u.repo, path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+u.token)
req.Header.Set("Accept", "application/vnd.github.v3+json")

resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &FileExistsResult{Exists: false}, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s", string(body))
	}

	// Parse response to get SHA and download URL
	var fileInfo struct {
		SHA         string `json:"sha"`
		DownloadURL string `json:"download_url"`
		HTMLURL     string `json:"html_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fileInfo); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &FileExistsResult{
		Exists: true,
		URL:    fileInfo.HTMLURL,
		SHA:    fileInfo.SHA,
	}, nil
}

// UploadFile uploads a file to the repository
func (u *Uploader) UploadFile(localPath, remotePath, commitMsg string, overwrite bool) (*UploadResult, error) {
	// Read the local file
	content, err := os.ReadFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Check if file already exists
	existsResult, err := u.CheckFileExists(remotePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing file: %w", err)
	}

	if existsResult.Exists && !overwrite {
		return &UploadResult{
			Success: false,
			URL:     existsResult.URL,
			Message: "File already exists",
		}, nil
	}

	// Prepare the request body
	requestBody := map[string]interface{}{
		"message": commitMsg,
		"content": base64.StdEncoding.EncodeToString(content),
	}

	// If updating existing file, include the SHA
	if existsResult.Exists && overwrite {
		requestBody["sha"] = existsResult.SHA
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the API request
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", u.owner, u.repo, remotePath)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+u.token)
req.Header.Set("Accept", "application/vnd.github.v3+json")

req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response to get the file URL
	var uploadResp struct {
		Content struct {
			HTMLURL     string `json:"html_url"`
			DownloadURL string `json:"download_url"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &UploadResult{
		Success: true,
		URL:     uploadResp.Content.HTMLURL,
		Message: "File uploaded successfully",
	}, nil
}

// TranslationSearchResult represents the search result for a translation
type TranslationSearchResult struct {
	Found          bool   `json:"found"`
	ArxivID        string `json:"arxiv_id"`
	ChinesePDF     string `json:"chinese_pdf"`      // URL to Chinese PDF
	BilingualPDF   string `json:"bilingual_pdf"`    // URL to Bilingual PDF
	LatexZip       string `json:"latex_zip"`        // URL to LaTeX zip file
	DownloadURLCN  string `json:"download_url_cn"`  // Direct download URL for Chinese PDF
	DownloadURLBi  string `json:"download_url_bi"`  // Direct download URL for Bilingual PDF
	DownloadURLZip string `json:"download_url_zip"` // Direct download URL for LaTeX zip
	// Full filenames for matched files
	ChinesePDFFilename   string `json:"chinese_pdf_filename"`
	BilingualPDFFilename string `json:"bilingual_pdf_filename"`
	LatexZipFilename     string `json:"latex_zip_filename"`
}

// RepoFile represents a file in the GitHub repository
type RepoFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	HTMLURL     string `json:"html_url"`
}

// SearchTranslation searches for an existing translation by arXiv ID
// It lists all files in the repository and matches by arXiv ID prefix
func (u *Uploader) SearchTranslation(arxivID string) (*TranslationSearchResult, error) {
	result := &TranslationSearchResult{
		Found:   false,
		ArxivID: arxivID,
	}

	// List all files in the repository root
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/", u.owner, u.repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers - no auth needed for public repos
	req.Header.Set("Accept", "application/vnd.github.v3+json")

resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("GitHub 仓库 %s/%s 不存在或无法访问，请在设置中配置正确的仓库信息", u.owner, u.repo)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response to get file list
	var files []RepoFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Match files by arXiv ID prefix
	for _, file := range files {
		// Skip directories
		if file.Type != "file" {
			continue
		}

		// Check if filename starts with the arXiv ID
		if !strings.HasPrefix(file.Name, arxivID) {
			continue
		}

		// Get file extension
		ext := strings.ToLower(filepath.Ext(file.Name))
		nameWithoutExt := strings.TrimSuffix(file.Name, ext)

		// Classify file by name pattern and extension
		if ext == ".pdf" {
			// Check if it's a Chinese PDF (contains _cn or similar)
			if strings.Contains(strings.ToLower(nameWithoutExt), "_cn") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "-cn") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "_chinese") {
				result.Found = true
				result.ChinesePDF = file.HTMLURL
				result.DownloadURLCN = file.DownloadURL
				result.ChinesePDFFilename = file.Name
			} else if strings.Contains(strings.ToLower(nameWithoutExt), "_bilingual") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "-bilingual") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "_bi") {
				// Bilingual PDF
				result.Found = true
				result.BilingualPDF = file.HTMLURL
				result.DownloadURLBi = file.DownloadURL
				result.BilingualPDFFilename = file.Name
			}
		} else if ext == ".zip" {
			// LaTeX zip file
			result.Found = true
			result.LatexZip = file.HTMLURL
			result.DownloadURLZip = file.DownloadURL
			result.LatexZipFilename = file.Name
		}
	}

	return result, nil
}

// DownloadFile downloads a file from GitHub to a local path
func (u *Uploader) DownloadFile(downloadURL, localPath string) error {
	// Create HTTP request
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to download file (status %d): %s", resp.StatusCode, string(body))
	}

	// Create local file
	outFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer outFile.Close()

	// Copy content
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ArxivPaperInfo represents basic information about an arXiv paper in the repository
type ArxivPaperInfo struct {
	ArxivID          string `json:"arxiv_id"`
	ChinesePDF       string `json:"chinese_pdf"`
	BilingualPDF     string `json:"bilingual_pdf"`
	LatexZip         string `json:"latex_zip"`
	ChinesePDFURL    string `json:"chinese_pdf_url"`
	BilingualPDFURL  string `json:"bilingual_pdf_url"`
	LatexZipURL      string `json:"latex_zip_url"`
	HasChinese       bool   `json:"has_chinese"`
	HasBilingual     bool   `json:"has_bilingual"`
	HasLatex         bool   `json:"has_latex"`
}

// ListRecentTranslations lists the most recent translations in the repository
// Returns up to maxCount unique arXiv IDs with their available files
func (u *Uploader) ListRecentTranslations(maxCount int) ([]ArxivPaperInfo, error) {
	// List all files in the repository root
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/", u.owner, u.repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers - no auth needed for public repos
	req.Header.Set("Accept", "application/vnd.github.v3+json")

resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("GitHub 仓库 %s/%s 不存在或无法访问", u.owner, u.repo)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response to get file list
	var files []RepoFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract arXiv IDs and group files by arXiv ID
	arxivMap := make(map[string]*ArxivPaperInfo)
	arxivIDPattern := regexp.MustCompile(`^(\d{4}\.\d{4,5}(v\d+)?)`)

	for _, file := range files {
		// Skip directories
		if file.Type != "file" {
			continue
		}

		// Extract arXiv ID from filename
		matches := arxivIDPattern.FindStringSubmatch(file.Name)
		if len(matches) < 2 {
			continue
		}

		arxivID := matches[1]

		// Initialize paper info if not exists
		if _, exists := arxivMap[arxivID]; !exists {
			arxivMap[arxivID] = &ArxivPaperInfo{
				ArxivID: arxivID,
			}
		}

		paper := arxivMap[arxivID]

		// Get file extension
		ext := strings.ToLower(filepath.Ext(file.Name))
		nameWithoutExt := strings.TrimSuffix(file.Name, ext)

		// Classify file by name pattern and extension
		if ext == ".pdf" {
			// Check if it's a Chinese PDF
			if strings.Contains(strings.ToLower(nameWithoutExt), "_cn") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "-cn") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "_chinese") {
				paper.ChinesePDF = file.Name
				paper.ChinesePDFURL = file.DownloadURL
				paper.HasChinese = true
			} else if strings.Contains(strings.ToLower(nameWithoutExt), "_bilingual") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "-bilingual") ||
				strings.Contains(strings.ToLower(nameWithoutExt), "_bi") {
				// Bilingual PDF
				paper.BilingualPDF = file.Name
				paper.BilingualPDFURL = file.DownloadURL
				paper.HasBilingual = true
			}
		} else if ext == ".zip" {
			// LaTeX zip file
			paper.LatexZip = file.Name
			paper.LatexZipURL = file.DownloadURL
			paper.HasLatex = true
		}
	}

	// Convert map to slice and limit to maxCount
	result := make([]ArxivPaperInfo, 0, len(arxivMap))
	for _, paper := range arxivMap {
		// Only include papers that have at least one file
		if paper.HasChinese || paper.HasBilingual || paper.HasLatex {
			result = append(result, *paper)
		}
	}

	// Limit to maxCount
	if len(result) > maxCount {
		result = result[:maxCount]
	}

	return result, nil
}


