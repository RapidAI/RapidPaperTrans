package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

// Command line flags
var (
	urlFlag  = flag.String("url", "", "arXiv URL to download and process (e.g., https://arxiv.org/abs/2301.00001)")
	idFlag   = flag.String("id", "", "arXiv ID to download and process (e.g., 2301.00001)")
	fileFlag = flag.String("file", "", "Local zip file path to process")
	pdfFlag  = flag.String("pdf", "", "PDF file path to translate directly")
	cliFlag  = flag.Bool("cli", false, "Run in CLI mode without GUI")
)

// printHelp displays the help information for command line usage.
func printHelp() {
	fmt.Println("LaTeX Translator - 将英文 LaTeX 文档翻译成中文并生成 PDF")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  latex-translator [选项]")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  --url <URL>     arXiv URL 地址 (例如: https://arxiv.org/abs/2301.00001)")
	fmt.Println("  --id <ID>       arXiv ID (例如: 2301.00001 或 hep-th/9901001)")
	fmt.Println("  --file <PATH>   本地 zip 文件路径 (LaTeX 源码)")
	fmt.Println("  --pdf <PATH>    PDF 文件路径 (直接翻译 PDF)")
	fmt.Println("  --cli           命令行模式运行 (不启动 GUI)")
	fmt.Println("  -h, --help      显示帮助信息")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  latex-translator                           # 启动 GUI 界面")
	fmt.Println("  latex-translator --url https://arxiv.org/abs/2301.00001")
	fmt.Println("  latex-translator --id 2301.00001")
	fmt.Println("  latex-translator --file /path/to/paper.zip")
	fmt.Println("  latex-translator --pdf /path/to/paper.pdf --cli  # CLI 模式翻译 PDF")
	fmt.Println()
	fmt.Println("说明:")
	fmt.Println("  如果不提供任何参数，程序将启动图形界面。")
	fmt.Println("  如果提供了 --url、--id 或 --file 参数，程序将启动后自动开始处理。")
	fmt.Println("  使用 --pdf 和 --cli 可以在命令行模式下直接翻译 PDF 文件。")
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

	if count > 1 {
		return "", "", fmt.Errorf("只能指定一个输入源 (--url, --id, --file, 或 --pdf)")
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
