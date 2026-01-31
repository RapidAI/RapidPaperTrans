package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"latex-translator/internal/config"
	"latex-translator/internal/logger"
	"latex-translator/internal/translator"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

// Command line flags
var (
	urlFlag    = flag.String("url", "", "arXiv URL to download and process (e.g., https://arxiv.org/abs/2301.00001)")
	idFlag     = flag.String("id", "", "arXiv ID to download and process (e.g., 2301.00001)")
	fileFlag   = flag.String("file", "", "Local zip file path to process")
	pdfFlag    = flag.String("pdf", "", "PDF file path to translate directly")
	bookFlag   = flag.String("book", "", "Book directory or zip file to translate (LaTeX book project)")
	maxFiles   = flag.Int("max-files", 0, "Maximum number of files to translate (0 = all, for book mode)")
	outputDir  = flag.String("output", "", "Output directory for translated files (for book mode)")
	cliFlag    = flag.Bool("cli", false, "Run in CLI mode without GUI")
)

// printHelp displays the help information for command line usage.
func printHelp() {
	fmt.Println("LaTeX Translator - 将英文 LaTeX 文档翻译成中文并生成 PDF")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  latex-translator [选项]")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  --url <URL>        arXiv URL 地址 (例如: https://arxiv.org/abs/2301.00001)")
	fmt.Println("  --id <ID>          arXiv ID (例如: 2301.00001 或 hep-th/9901001)")
	fmt.Println("  --file <PATH>      本地 zip 文件路径 (LaTeX 源码)")
	fmt.Println("  --pdf <PATH>       PDF 文件路径 (直接翻译 PDF)")
	fmt.Println("  --book <PATH>      书籍目录或 zip 文件 (LaTeX 书籍项目)")
	fmt.Println("  --max-files <N>    最大翻译文件数 (0=全部, 用于书籍模式)")
	fmt.Println("  --output <PATH>    输出目录 (用于书籍模式)")
	fmt.Println("  --cli              命令行模式运行 (不启动 GUI)")
	fmt.Println("  -h, --help         显示帮助信息")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  latex-translator                           # 启动 GUI 界面")
	fmt.Println("  latex-translator --url https://arxiv.org/abs/2301.00001")
	fmt.Println("  latex-translator --id 2301.00001")
	fmt.Println("  latex-translator --file /path/to/paper.zip")
	fmt.Println("  latex-translator --pdf /path/to/paper.pdf --cli")
	fmt.Println("  latex-translator --book /path/to/book.zip --cli --max-files 5")
	fmt.Println("  latex-translator --book /path/to/book --output /path/to/output --cli")
	fmt.Println()
	fmt.Println("说明:")
	fmt.Println("  如果不提供任何参数，程序将启动图形界面。")
	fmt.Println("  如果提供了 --url、--id 或 --file 参数，程序将启动后自动开始处理。")
	fmt.Println("  使用 --pdf 和 --cli 可以在命令行模式下直接翻译 PDF 文件。")
	fmt.Println("  使用 --book 和 --cli 可以在命令行模式下翻译整本书籍。")
}

// getInputFromFlags returns the input string from command line flags.
// Returns empty string if no input flag is provided.
// Returns an error if multiple input flags are provided.
func getInputFromFlags() (string, string, error) {
	count := 0
	var input string
	var inputType string

	if *urlFlag != "" {
		count++
		input = *urlFlag
		inputType = "url"
	}
	if *idFlag != "" {
		count++
		input = *idFlag
		inputType = "id"
	}
	if *fileFlag != "" {
		count++
		input = *fileFlag
		inputType = "file"
	}
	if *pdfFlag != "" {
		count++
		input = *pdfFlag
		inputType = "pdf"
	}
	if *bookFlag != "" {
		count++
		input = *bookFlag
		inputType = "book"
	}

	if count > 1 {
		return "", "", fmt.Errorf("只能指定一个输入源 (--url, --id, --file, --pdf, 或 --book)")
	}

	return input, inputType, nil
}

// PDFHandler handles requests for PDF files from the local filesystem
type PDFHandler struct{}

func (h *PDFHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle /pdf/ requests
	if !strings.HasPrefix(r.URL.Path, "/pdf/") {
		http.NotFound(w, r)
		return
	}

	// Extract the file path from the URL
	// URL format: /pdf/C:/path/to/file.pdf or /pdf/path/to/file.pdf
	filePath := strings.TrimPrefix(r.URL.Path, "/pdf/")
	
	// URL decode the path
	filePath = strings.ReplaceAll(filePath, "%20", " ")
	filePath = strings.ReplaceAll(filePath, "%3A", ":")
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Serve the PDF file
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, filePath)
}

func main() {
	// Custom usage function for help
	flag.Usage = printHelp

	// Parse command line flags
	flag.Parse()

	// Get input from flags
	input, inputType, err := getInputFromFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		fmt.Println()
		printHelp()
		os.Exit(1)
	}

	// CLI mode for PDF translation
	if *cliFlag && inputType == "pdf" {
		runPDFTranslationCLI(input)
		return
	}

	// CLI mode for book translation
	if *cliFlag && inputType == "book" {
		runBookTranslationCLI(input, *outputDir, *maxFiles)
		return
	}

	// CLI mode for arXiv ID/URL/file translation
	if *cliFlag && (inputType == "id" || inputType == "url" || inputType == "file") {
		runArxivTranslationCLI(input)
		return
	}

	// Create an instance of the app structure
	app := NewApp()
	
	// Mark as running in Wails environment
	app.SetWailsRuntime(true)

	// Wrap the startup function to handle command line input
	startupFunc := func(ctx context.Context) {
		// Call the original startup
		app.startup(ctx)

		// If command line input is provided, start processing automatically
		if input != "" && inputType != "pdf" {
			// Use goroutine to avoid blocking the startup
			go func() {
				// Wait for the app to be fully initialized
				result, err := app.ProcessSource(input)
				if err != nil {
					// Emit error event to frontend
					runtime.EventsEmit(ctx, "process-error", err.Error())
					fmt.Fprintf(os.Stderr, "处理失败: %v\n", err)
				} else {
					// Emit success event to frontend
					runtime.EventsEmit(ctx, "process-complete", result)
					fmt.Printf("处理完成!\n")
					fmt.Printf("原始 PDF: %s\n", result.OriginalPDFPath)
					fmt.Printf("翻译 PDF: %s\n", result.TranslatedPDFPath)
				}
			}()
		}
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "论译",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: &PDFHandler{},
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        startupFunc,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			// Check if there's a translation task in progress
			if app.IsProcessing() {
				// Show confirmation dialog
				result, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
					Type:          runtime.QuestionDialog,
					Title:         "确认退出",
					Message:       "翻译任务正在进行中，确定要退出吗？\n退出后当前任务将被取消。",
					Buttons:       []string{"取消", "退出"},
					DefaultButton: "取消",
					CancelButton:  "取消",
				})
				if err != nil {
					// If dialog fails, allow close
					return false
				}
				// If user clicked "取消" (Cancel), prevent close
				if result == "取消" {
					return true
				}
				// User clicked "退出" (Exit), cancel the process and allow close
				app.CancelProcess()
			}
			return false
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		// Log error to file instead of console
		// println("Error:", err.Error())
	}
}

// runPDFTranslationCLI runs PDF translation in CLI mode without GUI
func runPDFTranslationCLI(pdfPath string) {
	fmt.Println("=== PDF 翻译 (CLI 模式) ===")
	fmt.Printf("输入文件: %s\n", pdfPath)

	// Check if file exists
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "错误: 文件不存在: %s\n", pdfPath)
		os.Exit(1)
	}

	// Create app and initialize
	app := NewApp()
	app.startup(context.Background())

	// Print config info for debugging
	if app.config != nil {
		fmt.Printf("API Base URL: %s\n", app.config.GetBaseURL())
		fmt.Printf("Model: %s\n", app.config.GetModel())
		fmt.Printf("Context Window: %d\n", app.config.GetContextWindow())
	}

	// Load PDF
	fmt.Println("正在加载 PDF...")
	pdfInfo, err := app.LoadPDF(pdfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 加载 PDF 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("PDF 信息: %d 页\n", pdfInfo.PageCount)

	// Start translation with progress monitoring
	fmt.Println("正在翻译...")
	
	// Start a goroutine to monitor progress
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				status := app.GetPDFStatus()
				if status != nil {
					fmt.Printf("  状态: %s - %s (进度: %d%%)\n", 
						status.Phase, status.Message, status.Progress)
				}
			}
		}
	}()

	result, err := app.TranslatePDF()
	close(done)

	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 翻译失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== 翻译完成 ===")
	fmt.Printf("原始 PDF: %s\n", result.OriginalPDFPath)
	fmt.Printf("翻译 PDF: %s\n", result.TranslatedPDFPath)
	fmt.Printf("总块数: %d\n", result.TotalBlocks)
	fmt.Printf("翻译块数: %d\n", result.TranslatedBlocks)
	fmt.Printf("缓存块数: %d\n", result.CachedBlocks)

	// Cleanup
	app.shutdown(context.Background())
}

// runArxivTranslationCLI runs arXiv LaTeX translation in CLI mode without GUI
func runArxivTranslationCLI(input string) {
	// Initialize logger with console output for CLI mode
	logger.Init(&logger.Config{
		LogFilePath:   "latex-translator-cli.log",
		Level:         logger.LevelInfo,
		EnableConsole: true,
	})
	defer logger.Close()

	fmt.Println("=== arXiv LaTeX 翻译 (CLI 模式) ===")
	fmt.Printf("输入: %s\n", input)

	// Create app and initialize
	app := NewApp()
	app.startup(context.Background())

	// Print config info for debugging
	if app.config != nil {
		fmt.Printf("API Base URL: %s\n", app.config.GetBaseURL())
		fmt.Printf("Model: %s\n", app.config.GetModel())
	}

	// Print work directory for debugging
	fmt.Printf("工作目录: %s\n", app.GetWorkDir())

	// Start a goroutine to monitor progress
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		lastProgress := -1
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				status := app.GetStatus()
				if status != nil && status.Progress != lastProgress {
					fmt.Printf("  [%d%%] %s: %s\n", status.Progress, status.Phase, status.Message)
					lastProgress = status.Progress
				}
			}
		}
	}()

	// Process the source
	result, err := app.ProcessSource(input)
	close(done)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n错误: 翻译失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "工作目录保留在: %s\n", app.GetWorkDir())
		// Don't cleanup on error so we can inspect the files
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== 翻译完成 ===")
	fmt.Printf("原始 PDF: %s\n", result.OriginalPDFPath)
	fmt.Printf("翻译 PDF: %s\n", result.TranslatedPDFPath)
	fmt.Printf("工作目录: %s\n", app.GetWorkDir())

	// Don't cleanup - keep the files for user to access
	// app.shutdown(context.Background())
}

// runBookTranslationCLI runs book translation in CLI mode without GUI
func runBookTranslationCLI(bookPath, outputPath string, maxFiles int) {
	// Initialize logger with console output for CLI mode
	logger.Init(&logger.Config{
		LogFilePath:   "latex-translator-book.log",
		Level:         logger.LevelInfo,
		EnableConsole: true,
	})
	defer logger.Close()

	fmt.Println("=== LaTeX 书籍翻译 (CLI 模式) ===")
	fmt.Printf("输入: %s\n", bookPath)

	// Load configuration
	configMgr, err := config.NewConfigManager("latex-translator-config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 无法加载配置: %v\n", err)
		os.Exit(1)
	}

	if err := configMgr.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// Get API key from config or environment
	apiKey := configMgr.GetAPIKey()
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "错误: API 密钥未配置\n")
		fmt.Fprintf(os.Stderr, "请在配置文件中设置 API 密钥: latex-translator-config.json\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量:\n")
		fmt.Fprintf(os.Stderr, "  Windows CMD: set OPENAI_API_KEY=your-key\n")
		fmt.Fprintf(os.Stderr, "  PowerShell:  $env:OPENAI_API_KEY=\"your-key\"\n")
		os.Exit(1)
	}

	// Get API configuration
	baseURL := configMgr.GetBaseURL()
	model := configMgr.GetModel()
	
	fmt.Printf("API Base URL: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)

	// Determine input directory
	inputDir := bookPath
	
	// If it's a zip file, extract it first
	if strings.HasSuffix(strings.ToLower(bookPath), ".zip") {
		fmt.Println("正在解压 ZIP 文件...")
		extractDir := strings.TrimSuffix(bookPath, ".zip") + "_extracted"
		
		// Create extract directory
		if err := os.MkdirAll(extractDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 创建解压目录失败: %v\n", err)
			os.Exit(1)
		}

		// Extract zip
		if err := extractZip(bookPath, extractDir); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 解压失败: %v\n", err)
			os.Exit(1)
		}

		// Find the actual book directory (might be nested)
		entries, err := os.ReadDir(extractDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 读取解压目录失败: %v\n", err)
			os.Exit(1)
		}

		// If there's only one directory, use it
		if len(entries) == 1 && entries[0].IsDir() {
			inputDir = filepath.Join(extractDir, entries[0].Name())
		} else {
			inputDir = extractDir
		}

		fmt.Printf("解压到: %s\n", inputDir)
	}

	// Check if directory exists
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "错误: 目录不存在: %s\n", inputDir)
		os.Exit(1)
	}

	// Set default output directory if not specified
	if outputPath == "" {
		outputPath = filepath.Join("testdata", "output", "book_translated")
	}

	fmt.Printf("输出目录: %s\n", outputPath)
	if maxFiles > 0 {
		fmt.Printf("最大文件数: %d\n", maxFiles)
	} else {
		fmt.Println("翻译所有文件")
	}

	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// Find all .tex files
	fmt.Println("\n正在扫描 LaTeX 文件...")
	texFiles, err := findTexFiles(inputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 扫描文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("找到 %d 个 .tex 文件\n", len(texFiles))

	// Limit files if specified
	if maxFiles > 0 && len(texFiles) > maxFiles {
		fmt.Printf("限制为前 %d 个文件\n", maxFiles)
		texFiles = texFiles[:maxFiles]
	}

	// Translate the book
	if err := translateBook(inputDir, outputPath, apiKey, baseURL, model, texFiles); err != nil {
		fmt.Fprintf(os.Stderr, "\n错误: 翻译失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== 翻译完成 ===")
	fmt.Printf("输出目录: %s\n", outputPath)
}

// extractZip extracts a zip file to the specified directory
func extractZip(zipPath, destDir string) error {
	// Use PowerShell Expand-Archive on Windows
	cmd := exec.Command("powershell", "-Command", 
		fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", zipPath, destDir))
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("解压失败: %v\n%s", err, string(output))
	}
	
	return nil
}

// findTexFiles finds all .tex files in a directory recursively
func findTexFiles(dir string) ([]string, error) {
	var texFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".tex") {
			// Skip already translated files
			if !strings.Contains(path, "_zh.tex") && !strings.Contains(path, "_ro.tex") {
				texFiles = append(texFiles, path)
			}
		}

		return nil
	})

	return texFiles, err
}

// translateBook translates all LaTeX files in the book
func translateBook(inputDir, outputDir, apiKey, baseURL, model string, texFiles []string) error {
	fmt.Println("\n=== 开始翻译 ===")
	
	// Create translator with custom configuration
	trans := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 120*time.Second, 3)

	// Track statistics
	startTime := time.Now()
	successCount := 0
	errorCount := 0
	skipCount := 0
	var errors []string

	// Translate each file
	for i, texFile := range texFiles {
		relPath, _ := filepath.Rel(inputDir, texFile)
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(texFiles), relPath)

		// Create output path first to check if already translated
		outputPath := filepath.Join(outputDir, relPath)
		outputPath = strings.TrimSuffix(outputPath, ".tex") + "_zh.tex"

		// Skip if already translated
		if _, err := os.Stat(outputPath); err == nil {
			fmt.Printf("  ⏭️  跳过 (已翻译)\n")
			skipCount++
			successCount++ // Count as success since it's already done
			continue
		}

		// Read file
		content, err := os.ReadFile(texFile)
		if err != nil {
			fmt.Printf("  ❌ 读取失败: %v\n", err)
			errorCount++
			errors = append(errors, fmt.Sprintf("%s: 读取失败", relPath))
			continue
		}

		// Skip if too small
		if len(content) < 50 {
			fmt.Printf("  ⏭️  跳过 (文件太小: %d 字节)\n", len(content))
			skipCount++
			continue
		}

		// Check if file is mostly TikZ/figure code (no translatable text)
		contentStr := string(content)
		if isMostlyCode(contentStr) {
			fmt.Printf("  ⏭️  跳过 (主要是代码/图形，无需翻译)\n")
			skipCount++
			// Copy original file as-is
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err == nil {
				os.WriteFile(outputPath, content, 0644)
			}
			continue
		}

		// Translate
		fmt.Printf("  📝 翻译中... (%d 字节)\n", len(content))
		translateStart := time.Now()

		result, err := trans.TranslateTeX(contentStr)
		if err != nil {
			// Check if it's a validation error for code-only files
			if strings.Contains(err.Error(), "中文字符过少") {
				fmt.Printf("  ⏭️  跳过 (无可翻译文本)\n")
				skipCount++
				// Copy original file as-is
				if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err == nil {
					os.WriteFile(outputPath, content, 0644)
				}
				continue
			}
			
			fmt.Printf("  ❌ 翻译失败: %v\n", err)
			errorCount++
			errors = append(errors, fmt.Sprintf("%s: 翻译失败 - %v", relPath, err))
			continue
		}

		elapsed := time.Since(translateStart)
		fmt.Printf("  ⏱️  耗时: %v\n", elapsed.Round(time.Millisecond))

		// Create output directory
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			fmt.Printf("  ❌ 创建目录失败: %v\n", err)
			errorCount++
			errors = append(errors, fmt.Sprintf("%s: 创建目录失败", relPath))
			continue
		}

		// Write translated file
		if err := os.WriteFile(outputPath, []byte(result.TranslatedContent), 0644); err != nil {
			fmt.Printf("  ❌ 写入失败: %v\n", err)
			errorCount++
			errors = append(errors, fmt.Sprintf("%s: 写入失败", relPath))
			continue
		}

		fmt.Printf("  ✅ 成功\n")
		successCount++

		// Progress update every 5 files
		if (i+1)%5 == 0 {
			totalElapsed := time.Since(startTime)
			avgTime := totalElapsed / time.Duration(i+1)
			remaining := avgTime * time.Duration(len(texFiles)-(i+1))
			fmt.Printf("\n📊 进度: %d/%d (%.1f%%), 预计剩余: %v\n", 
				i+1, len(texFiles), float64(i+1)/float64(len(texFiles))*100, 
				remaining.Round(time.Second))
		}
	}

	// Print summary
	totalElapsed := time.Since(startTime)
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 翻译摘要 ===")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("总文件数:    %d\n", len(texFiles))
	fmt.Printf("成功翻译:    %d\n", successCount)
	fmt.Printf("跳过:        %d\n", skipCount)
	fmt.Printf("错误:        %d\n", errorCount)
	fmt.Printf("总耗时:      %v\n", totalElapsed.Round(time.Second))

	if successCount > 0 {
		avgTime := totalElapsed / time.Duration(successCount)
		fmt.Printf("平均耗时:    %v/文件\n", avgTime.Round(time.Millisecond))
	}

	if len(errors) > 0 {
		fmt.Println("\n=== 错误列表 ===")
		for i, e := range errors {
			fmt.Printf("%d. %s\n", i+1, e)
		}
	}

	fmt.Println(strings.Repeat("=", 60))

	if errorCount > 0 {
		return fmt.Errorf("翻译完成，但有 %d 个错误", errorCount)
	}

	return nil
}

// isMostlyCode checks if a LaTeX file is mostly code/figures with little translatable text
func isMostlyCode(content string) bool {
	// Count lines with actual English text vs code/commands
	lines := strings.Split(content, "\n")
	textLines := 0
	codeLines := 0
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "%") {
			continue
		}
		
		// Check if line is mostly LaTeX commands (starts with \, or contains tikz/pgf commands)
		if strings.HasPrefix(trimmed, "\\") || 
		   strings.Contains(trimmed, "\\draw") ||
		   strings.Contains(trimmed, "\\node") ||
		   strings.Contains(trimmed, "\\path") ||
		   strings.Contains(trimmed, "\\coordinate") ||
		   strings.Contains(trimmed, "\\fill") ||
		   strings.Contains(trimmed, "\\shade") {
			codeLines++
			continue
		}
		
		// Check if line has English words (simple heuristic)
		hasEnglish := false
		words := strings.Fields(trimmed)
		for _, word := range words {
			// Remove LaTeX commands
			word = strings.TrimPrefix(word, "\\")
			// Check if word has letters
			for _, r := range word {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					hasEnglish = true
					break
				}
			}
			if hasEnglish {
				break
			}
		}
		
		if hasEnglish {
			textLines++
		} else {
			codeLines++
		}
	}
	
	// If more than 80% is code, consider it code-only
	total := textLines + codeLines
	if total == 0 {
		return true
	}
	
	codeRatio := float64(codeLines) / float64(total)
	return codeRatio > 0.8
}
