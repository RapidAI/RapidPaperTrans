// Batch process arXiv papers: download, compile, translate
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"latex-translator/internal/compiler"
	"latex-translator/internal/downloader"
	"latex-translator/internal/logger"
	"latex-translator/internal/translator"
)

func init() {
	logger.Init(&logger.Config{
		LogFilePath:   "batch_process.log",
		Level:         logger.LevelInfo,
		EnableConsole: true,
	})
}

type Config struct {
	OpenAIAPIKey  string `json:"openai_api_key"`
	OpenAIBaseURL string `json:"openai_base_url"`
	OpenAIModel   string `json:"openai_model"`
}

type ProcessResult struct {
	ArxivID       string
	Downloaded    bool
	Compiled      bool
	Translated    bool
	PDFGenerated  bool
	Error         string
	FixesApplied  []string
}

var (
	goodIDs    []string
	badIDs     []string
	bugIDs     []string
	goodMutex  sync.Mutex
	badMutex   sync.Mutex
	bugMutex   sync.Mutex
	config     *Config
)

func loadConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	configPaths := []string{
		"latex-translator-config.json",
		filepath.Join(homeDir, "latex-translator-config.json"),
	}

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err == nil {
				return &cfg
			}
		}
	}
	return &Config{}
}

func readIDsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ids []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" && !strings.HasPrefix(id, "#") {
			ids = append(ids, id)
		}
	}
	return ids, scanner.Err()
}

func appendToFile(filename string, ids []string) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, id := range ids {
		if _, err := file.WriteString(id + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func writeIDsToFile(filename string, ids []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, id := range ids {
		if _, err := file.WriteString(id + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func addGoodID(id string) {
	goodMutex.Lock()
	defer goodMutex.Unlock()
	goodIDs = append(goodIDs, id)
}

func addBadID(id string) {
	badMutex.Lock()
	defer badMutex.Unlock()
	badIDs = append(badIDs, id)
}

func addBugID(id string) {
	bugMutex.Lock()
	defer bugMutex.Unlock()
	bugIDs = append(bugIDs, id)
}


// Phase 1: Download and compile original papers
func processOriginal(arxivID string, workDir string) *ProcessResult {
	result := &ProcessResult{ArxivID: arxivID}
	
	extractDir := filepath.Join(workDir, strings.ReplaceAll(arxivID, "/", "_")+"_extracted")
	
	// Check if already extracted
	if _, err := os.Stat(extractDir); err == nil {
		logger.Info("using existing extraction", logger.String("arxivID", arxivID))
	} else {
		// Download
		dl := downloader.NewSourceDownloader(workDir)
		sourceInfo, err := dl.DownloadByID(arxivID)
		if err != nil {
			result.Error = fmt.Sprintf("download failed: %v", err)
			return result
		}
		result.Downloaded = true

		// Extract
		sourceInfo, err = dl.ExtractZip(sourceInfo.ExtractDir)
		if err != nil {
			result.Error = fmt.Sprintf("extract failed: %v", err)
			return result
		}
		extractDir = sourceInfo.ExtractDir
	}

	// Find main tex file
	dl := downloader.NewSourceDownloader(workDir)
	mainTexFile, err := dl.FindMainTexFile(extractDir)
	if err != nil {
		result.Error = fmt.Sprintf("find main tex failed: %v", err)
		return result
	}

	mainTexPath := filepath.Join(extractDir, mainTexFile)

	// Preprocess
	compiler.PreprocessTexFiles(extractDir)

	// Compile original
	comp := compiler.NewLaTeXCompiler("pdflatex", workDir, 10*time.Minute)
	outputDir := filepath.Join(extractDir, "output_original")
	compResult, err := comp.Compile(mainTexPath, outputDir)
	if err != nil || !compResult.Success {
		result.Error = fmt.Sprintf("compile failed: %v", err)
		if compResult != nil && compResult.ErrorMsg != "" {
			result.Error = compResult.ErrorMsg
		}
		return result
	}

	result.Compiled = true
	result.Downloaded = true
	return result
}

// Phase 2: Translate and generate PDF
func processTranslate(arxivID string, workDir string) *ProcessResult {
	result := &ProcessResult{ArxivID: arxivID, Downloaded: true, Compiled: true}
	
	extractDir := filepath.Join(workDir, strings.ReplaceAll(arxivID, "/", "_")+"_extracted")
	
	// Find main tex file
	dl := downloader.NewSourceDownloader(workDir)
	mainTexFile, err := dl.FindMainTexFile(extractDir)
	if err != nil {
		result.Error = fmt.Sprintf("find main tex failed: %v", err)
		return result
	}

	mainTexPath := filepath.Join(extractDir, mainTexFile)

	// Read content
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		result.Error = fmt.Sprintf("read tex failed: %v", err)
		return result
	}

	// Translate
	apiKey := config.OpenAIAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		result.Error = "no API key configured"
		return result
	}

	baseURL := config.OpenAIBaseURL
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	model := config.OpenAIModel
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	engine := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 0, 3)
	transResult, err := engine.TranslateTeXWithProgress(string(content), func(current, total int, message string) {
		// Progress callback
	})
	if err != nil {
		result.Error = fmt.Sprintf("translate failed: %v", err)
		return result
	}
	result.Translated = true

	// Add ctex package
	translatedContent := ensureCtexPackage(transResult.TranslatedContent)

	// Save translated file
	translatedTexPath := filepath.Join(extractDir, "translated_"+mainTexFile)
	if err := os.WriteFile(translatedTexPath, []byte(translatedContent), 0644); err != nil {
		result.Error = fmt.Sprintf("save translated failed: %v", err)
		return result
	}

	// Compile translated
	comp := compiler.NewLaTeXCompiler("xelatex", workDir, 10*time.Minute)
	outputDir := filepath.Join(extractDir, "output_translated")
	compResult, err := comp.CompileWithXeLaTeX(translatedTexPath, outputDir)
	if err != nil || !compResult.Success {
		result.Error = fmt.Sprintf("compile translated failed: %v", err)
		if compResult != nil && compResult.ErrorMsg != "" {
			result.Error = compResult.ErrorMsg
		}
		return result
	}

	result.PDFGenerated = true
	return result
}

func ensureCtexPackage(content string) string {
	if strings.Contains(content, "\\usepackage{ctex}") || strings.Contains(content, "\\usepackage[") && strings.Contains(content, "ctex") {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	added := false

	for _, line := range lines {
		result = append(result, line)
		if added || strings.HasPrefix(strings.TrimSpace(line), "%") {
			continue
		}
		if strings.Contains(line, "\\documentclass") {
			result = append(result, "\\usepackage{ctex}")
			added = true
		}
	}

	if added {
		content = strings.Join(result, "\n")
	}

	// Fix microtype for XeLaTeX
	if strings.Contains(content, "\\usepackage{microtype}") {
		content = strings.Replace(content, "\\usepackage{microtype}",
			"\\usepackage[protrusion=false,expansion=false]{microtype}", 1)
	}

	return content
}


func main() {
	config = loadConfig()
	workDir := filepath.Join("testdata", "batch_arxiv")
	
	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		fmt.Printf("Failed to create work directory: %v\n", err)
		os.Exit(1)
	}

	// Determine which phase to run
	phase := "all"
	if len(os.Args) > 1 {
		phase = os.Args[1]
	}

	switch phase {
	case "compile", "1":
		runPhase1(workDir)
	case "translate", "2":
		runPhase2(workDir)
	case "fix", "3":
		runPhase3(workDir)
	case "all":
		runPhase1(workDir)
		runPhase2(workDir)
		runPhase3(workDir)
	default:
		fmt.Println("Usage: batch_process [compile|translate|fix|all]")
		fmt.Println("  compile/1  - Download and compile original papers")
		fmt.Println("  translate/2 - Translate good papers and generate PDFs")
		fmt.Println("  fix/3      - Fix and retry failed translations")
		fmt.Println("  all        - Run all phases")
		os.Exit(1)
	}
}

func runPhase1(workDir string) {
	fmt.Println("\n=== Phase 1: Download and Compile Original Papers ===")
	
	// Read all IDs
	allIDs, err := readIDsFromFile("arxiv_id.txt")
	if err != nil {
		fmt.Printf("Failed to read arxiv_id.txt: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Total papers to process: %d\n", len(allIDs))

	// Check for existing progress
	existingGood, _ := readIDsFromFile("arxiv_good_id.txt")
	existingBad, _ := readIDsFromFile("arxiv_bad_id.txt")
	processed := make(map[string]bool)
	for _, id := range existingGood {
		processed[id] = true
	}
	for _, id := range existingBad {
		processed[id] = true
	}

	// Filter out already processed
	var toProcess []string
	for _, id := range allIDs {
		if !processed[id] {
			toProcess = append(toProcess, id)
		}
	}
	fmt.Printf("Already processed: %d, Remaining: %d\n", len(processed), len(toProcess))

	// Process papers
	successCount := 0
	failCount := 0
	
	for i, id := range toProcess {
		fmt.Printf("\n[%d/%d] Processing %s...\n", i+1, len(toProcess), id)
		
		result := processOriginal(id, workDir)
		
		if result.Compiled {
			fmt.Printf("  ✓ Success\n")
			appendToFile("arxiv_good_id.txt", []string{id})
			successCount++
		} else {
			fmt.Printf("  ✗ Failed: %s\n", result.Error)
			appendToFile("arxiv_bad_id.txt", []string{id})
			failCount++
		}

		// Rate limiting for arXiv
		time.Sleep(3 * time.Second)
	}

	fmt.Printf("\n=== Phase 1 Complete ===\n")
	fmt.Printf("Success: %d, Failed: %d\n", successCount, failCount)
}

func runPhase2(workDir string) {
	fmt.Println("\n=== Phase 2: Translate and Generate PDFs ===")
	
	// Read good IDs
	goodIDList, err := readIDsFromFile("arxiv_good_id.txt")
	if err != nil {
		fmt.Printf("Failed to read arxiv_good_id.txt: %v\n", err)
		return
	}
	fmt.Printf("Papers to translate: %d\n", len(goodIDList))

	// Check for existing progress
	existingBug, _ := readIDsFromFile("arxiv_bug_id.txt")
	processed := make(map[string]bool)
	for _, id := range existingBug {
		processed[id] = true
	}

	// Check which ones already have translated PDFs
	var toTranslate []string
	for _, id := range goodIDList {
		if processed[id] {
			continue
		}
		extractDir := filepath.Join(workDir, strings.ReplaceAll(id, "/", "_")+"_extracted")
		translatedPDF := filepath.Join(extractDir, "output_translated")
		if _, err := os.Stat(translatedPDF); err != nil {
			toTranslate = append(toTranslate, id)
		}
	}
	fmt.Printf("Already translated: %d, Remaining: %d\n", len(goodIDList)-len(toTranslate), len(toTranslate))

	successCount := 0
	failCount := 0

	for i, id := range toTranslate {
		fmt.Printf("\n[%d/%d] Translating %s...\n", i+1, len(toTranslate), id)
		
		result := processTranslate(id, workDir)
		
		if result.PDFGenerated {
			fmt.Printf("  ✓ Success\n")
			successCount++
		} else {
			fmt.Printf("  ✗ Failed: %s\n", result.Error)
			appendToFile("arxiv_bug_id.txt", []string{id})
			failCount++
		}

		// Rate limiting for API
		time.Sleep(2 * time.Second)
	}

	fmt.Printf("\n=== Phase 2 Complete ===\n")
	fmt.Printf("Success: %d, Failed: %d\n", successCount, failCount)
}

func runPhase3(workDir string) {
	fmt.Println("\n=== Phase 3: Fix and Retry Failed Translations ===")
	
	// Read bug IDs
	bugIDList, err := readIDsFromFile("arxiv_bug_id.txt")
	if err != nil {
		fmt.Printf("No arxiv_bug_id.txt found or empty\n")
		return
	}
	fmt.Printf("Papers to fix: %d\n", len(bugIDList))

	fixedCount := 0
	stillBroken := []string{}

	for i, id := range bugIDList {
		fmt.Printf("\n[%d/%d] Fixing %s...\n", i+1, len(bugIDList), id)
		
		// Try to fix and recompile
		result := tryFixAndTranslate(id, workDir)
		
		if result.PDFGenerated {
			fmt.Printf("  ✓ Fixed successfully\n")
			fixedCount++
		} else {
			fmt.Printf("  ✗ Still broken: %s\n", result.Error)
			stillBroken = append(stillBroken, id)
		}

		time.Sleep(2 * time.Second)
	}

	// Update bug file with remaining broken IDs
	writeIDsToFile("arxiv_bug_id.txt", stillBroken)

	fmt.Printf("\n=== Phase 3 Complete ===\n")
	fmt.Printf("Fixed: %d, Still broken: %d\n", fixedCount, len(stillBroken))
}

func tryFixAndTranslate(arxivID string, workDir string) *ProcessResult {
	result := &ProcessResult{ArxivID: arxivID, Downloaded: true, Compiled: true}
	
	extractDir := filepath.Join(workDir, strings.ReplaceAll(arxivID, "/", "_")+"_extracted")
	
	// Find main tex file
	dl := downloader.NewSourceDownloader(workDir)
	mainTexFile, err := dl.FindMainTexFile(extractDir)
	if err != nil {
		result.Error = fmt.Sprintf("find main tex failed: %v", err)
		return result
	}

	translatedTexPath := filepath.Join(extractDir, "translated_"+mainTexFile)
	
	// Check if translated file exists
	if _, err := os.Stat(translatedTexPath); os.IsNotExist(err) {
		// Need to translate first
		return processTranslate(arxivID, workDir)
	}

	// Try to apply fixes to the translated file
	content, err := os.ReadFile(translatedTexPath)
	if err != nil {
		result.Error = fmt.Sprintf("read translated tex failed: %v", err)
		return result
	}

	// Apply various fixes
	fixedContent := applyTranslationFixes(string(content))
	
	if err := os.WriteFile(translatedTexPath, []byte(fixedContent), 0644); err != nil {
		result.Error = fmt.Sprintf("write fixed tex failed: %v", err)
		return result
	}

	// Try to compile again
	comp := compiler.NewLaTeXCompiler("xelatex", workDir, 10*time.Minute)
	outputDir := filepath.Join(extractDir, "output_translated")
	compResult, err := comp.CompileWithXeLaTeX(translatedTexPath, outputDir)
	if err != nil || !compResult.Success {
		result.Error = fmt.Sprintf("compile still failed: %v", err)
		if compResult != nil && compResult.ErrorMsg != "" {
			result.Error = compResult.ErrorMsg
		}
		return result
	}

	result.PDFGenerated = true
	result.Translated = true
	return result
}

func applyTranslationFixes(content string) string {
	// Fix 1: Ensure ctex package
	content = ensureCtexPackage(content)
	
	// Fix 2: Fix common XeLaTeX issues
	// Remove incompatible packages
	incompatiblePackages := []string{
		"\\usepackage{times}",
		"\\usepackage{mathptmx}",
	}
	for _, pkg := range incompatiblePackages {
		content = strings.Replace(content, pkg, "% "+pkg+" % disabled for XeLaTeX", 1)
	}

	// Fix 3: Fix fontenc for XeLaTeX
	if strings.Contains(content, "\\usepackage[T1]{fontenc}") {
		content = strings.Replace(content, "\\usepackage[T1]{fontenc}", 
			"% \\usepackage[T1]{fontenc} % disabled for XeLaTeX", 1)
	}

	// Fix 4: Fix inputenc for XeLaTeX
	if strings.Contains(content, "\\usepackage[utf8]{inputenc}") {
		content = strings.Replace(content, "\\usepackage[utf8]{inputenc}",
			"% \\usepackage[utf8]{inputenc} % disabled for XeLaTeX", 1)
	}

	return content
}
