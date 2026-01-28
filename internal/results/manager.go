// Package results provides translation result management functionality.
// It handles storing, listing, and managing translated papers by arXiv ID.
package results

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TranslationStatus represents the status of a translation
type TranslationStatus string

const (
	// StatusPending indicates the translation has not started
	StatusPending TranslationStatus = "pending"
	// StatusDownloading indicates the source is being downloaded
	StatusDownloading TranslationStatus = "downloading"
	// StatusExtracted indicates the source has been extracted
	StatusExtracted TranslationStatus = "extracted"
	// StatusOriginalCompiled indicates the original document has been compiled
	StatusOriginalCompiled TranslationStatus = "original_compiled"
	// StatusTranslating indicates the document is being translated
	StatusTranslating TranslationStatus = "translating"
	// StatusTranslated indicates the translation is complete but not compiled
	StatusTranslated TranslationStatus = "translated"
	// StatusCompiling indicates the translated document is being compiled
	StatusCompiling TranslationStatus = "compiling"
	// StatusComplete indicates the translation is fully complete
	StatusComplete TranslationStatus = "complete"
	// StatusError indicates an error occurred during translation
	StatusError TranslationStatus = "error"
)

// SourceType represents the type of source for translation
type SourceType string

const (
	// SourceTypeArxiv indicates the source is from arXiv
	SourceTypeArxiv SourceType = "arxiv"
	// SourceTypeZip indicates the source is a local zip file
	SourceTypeZip SourceType = "zip"
	// SourceTypePDF indicates the source is a PDF file
	SourceTypePDF SourceType = "pdf"
)

// PaperInfo represents metadata about a translated paper
type PaperInfo struct {
	ArxivID        string            `json:"arxiv_id"`
	Title          string            `json:"title"`
	TranslatedAt   time.Time         `json:"translated_at"`
	OriginalPDF    string            `json:"original_pdf"`
	TranslatedPDF  string            `json:"translated_pdf"`
	BilingualPDF   string            `json:"bilingual_pdf,omitempty"`
	SourceDir      string            `json:"source_dir"`
	HasLatexSource bool              `json:"has_latex_source"`
	// Status tracking fields
	Status         TranslationStatus `json:"status"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	LastPhase      string            `json:"last_phase,omitempty"`
	OriginalInput  string            `json:"original_input,omitempty"`
	MainTexFile    string            `json:"main_tex_file,omitempty"`
	// Source identification fields
	SourceType     SourceType        `json:"source_type,omitempty"`
	SourceMD5      string            `json:"source_md5,omitempty"`      // MD5 hash of source file (zip or PDF)
	SourceFileName string            `json:"source_file_name,omitempty"` // Original file name
}

// ResultManager manages translation results stored in user directory
type ResultManager struct {
	baseDir string // Base directory for storing results (e.g., ~/latex-translator-results)
}

// NewResultManager creates a new ResultManager with the specified base directory
// If baseDir is empty, uses default location in user's home directory
func NewResultManager(baseDir string) (*ResultManager, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		baseDir = filepath.Join(homeDir, "latex-translator-results")
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}

	return &ResultManager{baseDir: baseDir}, nil
}

// GetBaseDir returns the base directory for results
func (m *ResultManager) GetBaseDir() string {
	return m.baseDir
}

// GetPaperDir returns the directory path for a specific paper
func (m *ResultManager) GetPaperDir(arxivID string) string {
	// Sanitize arxiv ID for use as directory name
	safeID := sanitizeArxivID(arxivID)
	return filepath.Join(m.baseDir, safeID)
}

// SavePaperInfo saves paper metadata to the paper's directory
func (m *ResultManager) SavePaperInfo(info *PaperInfo) error {
	paperDir := m.GetPaperDir(info.ArxivID)
	
	// Ensure paper directory exists
	if err := os.MkdirAll(paperDir, 0755); err != nil {
		return err
	}

	// Save metadata as JSON
	metaPath := filepath.Join(paperDir, "metadata.json")
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
}

// LoadPaperInfo loads paper metadata from the paper's directory
func (m *ResultManager) LoadPaperInfo(arxivID string) (*PaperInfo, error) {
	paperDir := m.GetPaperDir(arxivID)
	metaPath := filepath.Join(paperDir, "metadata.json")

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var info PaperInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// ListPapers returns a list of all translated papers
func (m *ResultManager) ListPapers() ([]*PaperInfo, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*PaperInfo{}, nil
		}
		return nil, err
	}

	var papers []*PaperInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metaPath := filepath.Join(m.baseDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // Skip directories without metadata
		}

		var info PaperInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		papers = append(papers, &info)
	}

	// Sort by translation date, newest first
	sort.Slice(papers, func(i, j int) bool {
		return papers[i].TranslatedAt.After(papers[j].TranslatedAt)
	})

	return papers, nil
}

// DeletePaper deletes a paper and all its associated files
func (m *ResultManager) DeletePaper(arxivID string) error {
	paperDir := m.GetPaperDir(arxivID)
	return os.RemoveAll(paperDir)
}

// PaperExists checks if a paper with the given arXiv ID exists
func (m *ResultManager) PaperExists(arxivID string) bool {
	paperDir := m.GetPaperDir(arxivID)
	metaPath := filepath.Join(paperDir, "metadata.json")
	_, err := os.Stat(metaPath)
	return err == nil
}

// UpdatePaperStatus updates the status of a paper
func (m *ResultManager) UpdatePaperStatus(arxivID string, status TranslationStatus, errorMsg string) error {
	info, err := m.LoadPaperInfo(arxivID)
	if err != nil {
		return err
	}
	
	info.Status = status
	info.ErrorMessage = errorMsg
	info.TranslatedAt = time.Now()
	
	return m.SavePaperInfo(info)
}

// UpdatePaperPaths updates the PDF paths for a paper
func (m *ResultManager) UpdatePaperPaths(arxivID string, originalPDF, translatedPDF string) error {
	info, err := m.LoadPaperInfo(arxivID)
	if err != nil {
		return err
	}
	
	if originalPDF != "" {
		info.OriginalPDF = originalPDF
	}
	if translatedPDF != "" {
		info.TranslatedPDF = translatedPDF
	}
	info.TranslatedAt = time.Now()
	
	return m.SavePaperInfo(info)
}

// GetIncompleteTranslations returns papers that are not complete
func (m *ResultManager) GetIncompleteTranslations() ([]*PaperInfo, error) {
	papers, err := m.ListPapers()
	if err != nil {
		return nil, err
	}
	
	var incomplete []*PaperInfo
	for _, paper := range papers {
		if paper.Status != StatusComplete {
			incomplete = append(incomplete, paper)
		}
	}
	
	return incomplete, nil
}

// GetOriginalPDFPath returns the path to the original PDF
func (m *ResultManager) GetOriginalPDFPath(arxivID string) string {
	return filepath.Join(m.GetPaperDir(arxivID), "original.pdf")
}

// GetTranslatedPDFPath returns the path to the translated PDF
func (m *ResultManager) GetTranslatedPDFPath(arxivID string) string {
	return filepath.Join(m.GetPaperDir(arxivID), "translated.pdf")
}

// GetBilingualPDFPath returns the path to the bilingual PDF
func (m *ResultManager) GetBilingualPDFPath(arxivID string) string {
	return filepath.Join(m.GetPaperDir(arxivID), "bilingual.pdf")
}

// GetLatexSourceDir returns the path to the LaTeX source directory
func (m *ResultManager) GetLatexSourceDir(arxivID string) string {
	return filepath.Join(m.GetPaperDir(arxivID), "latex")
}

// sanitizeArxivID converts an arXiv ID to a safe directory name
func sanitizeArxivID(arxivID string) string {
	// Replace characters that might cause issues in file paths
	safe := strings.ReplaceAll(arxivID, "/", "_")
	safe = strings.ReplaceAll(safe, ":", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	return safe
}

// ExtractArxivID extracts arXiv ID from various input formats
func ExtractArxivID(input string) string {
	input = strings.TrimSpace(input)
	
	if input == "" {
		return ""
	}
	
	// Handle arXiv URLs
	// https://arxiv.org/abs/2301.00001
	// https://arxiv.org/pdf/2301.00001.pdf
	if strings.Contains(input, "arxiv.org") {
		parts := strings.Split(input, "/")
		for i, part := range parts {
			if part == "abs" || part == "pdf" {
				if i+1 < len(parts) {
					id := parts[i+1]
					// Remove .pdf extension if present
					id = strings.TrimSuffix(id, ".pdf")
					return id
				}
			}
		}
	}

	// Handle direct arXiv ID (e.g., 2301.00001 or hep-th/9901001)
	if isArxivID(input) {
		return input
	}

	return ""
}

// isArxivID checks if a string looks like an arXiv ID
func isArxivID(s string) bool {
	if s == "" {
		return false
	}
	// New format: YYMM.NNNNN (e.g., 2301.00001, 2310.06824)
	// Length should be at least 10 (YYMM.NNNNN) and have a dot at position 4
	if len(s) >= 10 && s[4] == '.' {
		// Check that first 4 chars are digits
		for i := 0; i < 4; i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		// Check that chars after dot are digits
		for i := 5; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	// Old format: category/YYMMNNN (e.g., hep-th/9901001)
	if strings.Contains(s, "/") && len(s) >= 10 {
		return true
	}
	return false
}

// ExtractTitleFromTeX attempts to extract the paper title from LaTeX source
func ExtractTitleFromTeX(texContent string) string {
	// Look for \title{...} command
	titleStart := strings.Index(texContent, "\\title{")
	if titleStart == -1 {
		titleStart = strings.Index(texContent, "\\title [")
		if titleStart == -1 {
			return ""
		}
	}

	// Find the opening brace
	braceStart := strings.Index(texContent[titleStart:], "{")
	if braceStart == -1 {
		return ""
	}
	braceStart += titleStart

	// Find matching closing brace
	depth := 1
	braceEnd := braceStart + 1
	for braceEnd < len(texContent) && depth > 0 {
		switch texContent[braceEnd] {
		case '{':
			depth++
		case '}':
			depth--
		}
		braceEnd++
	}

	if depth != 0 {
		return ""
	}

	title := texContent[braceStart+1 : braceEnd-1]
	
	// Clean up the title
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", "")
	title = strings.TrimSpace(title)
	
	// Remove LaTeX commands (simple cleanup)
	// This is a basic cleanup, might need more sophisticated handling
	for strings.Contains(title, "\\") {
		cmdStart := strings.Index(title, "\\")
		cmdEnd := cmdStart + 1
		// Find end of command (space or non-letter)
		for cmdEnd < len(title) && ((title[cmdEnd] >= 'a' && title[cmdEnd] <= 'z') || (title[cmdEnd] >= 'A' && title[cmdEnd] <= 'Z')) {
			cmdEnd++
		}
		// Remove the command
		title = title[:cmdStart] + title[cmdEnd:]
	}
	
	title = strings.TrimSpace(title)
	
	// Limit length
	if len(title) > 200 {
		title = title[:200] + "..."
	}

	return title
}

// CalculateFileMD5 calculates the MD5 hash of a file
func CalculateFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// FindByMD5 finds a paper by its source file MD5 hash
func (m *ResultManager) FindByMD5(md5Hash string) (*PaperInfo, error) {
	papers, err := m.ListPapers()
	if err != nil {
		return nil, err
	}

	for _, paper := range papers {
		if paper.SourceMD5 == md5Hash {
			return paper, nil
		}
	}

	return nil, nil // Not found, but not an error
}

// FindBySourceType finds all papers of a specific source type
func (m *ResultManager) FindBySourceType(sourceType SourceType) ([]*PaperInfo, error) {
	papers, err := m.ListPapers()
	if err != nil {
		return nil, err
	}

	var result []*PaperInfo
	for _, paper := range papers {
		if paper.SourceType == sourceType {
			result = append(result, paper)
		}
	}

	return result, nil
}

// GetPaperDirByMD5 returns the directory path for a paper identified by MD5
// This is used for zip and PDF sources that don't have an arXiv ID
func (m *ResultManager) GetPaperDirByMD5(md5Hash string) string {
	return filepath.Join(m.baseDir, "md5_"+md5Hash[:16]) // Use first 16 chars of MD5
}

// SavePaperInfoByMD5 saves paper info using MD5 as the identifier
func (m *ResultManager) SavePaperInfoByMD5(info *PaperInfo) error {
	if info.SourceMD5 == "" {
		return os.ErrInvalid
	}

	// Use MD5 as the directory name if no arXiv ID
	var paperDir string
	if info.ArxivID != "" {
		paperDir = m.GetPaperDir(info.ArxivID)
	} else {
		paperDir = m.GetPaperDirByMD5(info.SourceMD5)
		// Set a pseudo arXiv ID for consistency
		info.ArxivID = "md5_" + info.SourceMD5[:16]
	}

	// Ensure paper directory exists
	if err := os.MkdirAll(paperDir, 0755); err != nil {
		return err
	}

	// Save metadata as JSON
	metaPath := filepath.Join(paperDir, "metadata.json")
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
}

// CheckDuplicate checks if a translation already exists for the given input
// Returns the existing PaperInfo if found, nil otherwise
// For arXiv: checks by arXiv ID
// For zip/PDF: checks by MD5 hash
func (m *ResultManager) CheckDuplicate(input string, sourceType SourceType) (*PaperInfo, error) {
	switch sourceType {
	case SourceTypeArxiv:
		arxivID := ExtractArxivID(input)
		if arxivID != "" && m.PaperExists(arxivID) {
			return m.LoadPaperInfo(arxivID)
		}
	case SourceTypeZip, SourceTypePDF:
		// Calculate MD5 of the file
		md5Hash, err := CalculateFileMD5(input)
		if err != nil {
			return nil, err
		}
		return m.FindByMD5(md5Hash)
	}
	return nil, nil
}

// ExistingTranslationInfo contains information about an existing translation
type ExistingTranslationInfo struct {
	Exists       bool              `json:"exists"`
	PaperInfo    *PaperInfo        `json:"paper_info,omitempty"`
	IsComplete   bool              `json:"is_complete"`
	CanContinue  bool              `json:"can_continue"`
	Message      string            `json:"message"`
}

// CheckExistingTranslation checks if a translation already exists and returns detailed info
func (m *ResultManager) CheckExistingTranslation(input string, sourceType SourceType) (*ExistingTranslationInfo, error) {
	info := &ExistingTranslationInfo{
		Exists:      false,
		IsComplete:  false,
		CanContinue: false,
	}

	paper, err := m.CheckDuplicate(input, sourceType)
	if err != nil {
		return nil, err
	}

	if paper == nil {
		info.Message = "未找到已有翻译"
		return info, nil
	}

	info.Exists = true
	info.PaperInfo = paper

	switch paper.Status {
	case StatusComplete:
		info.IsComplete = true
		info.CanContinue = false
		info.Message = fmt.Sprintf("该文档已于 %s 翻译完成", paper.TranslatedAt.Format("2006-01-02 15:04"))
	case StatusError:
		info.IsComplete = false
		info.CanContinue = true
		info.Message = fmt.Sprintf("该文档翻译失败: %s，可以继续尝试", paper.ErrorMessage)
	default:
		info.IsComplete = false
		info.CanContinue = true
		info.Message = fmt.Sprintf("该文档翻译未完成 (状态: %s)，可以继续", paper.Status)
	}

	return info, nil
}
