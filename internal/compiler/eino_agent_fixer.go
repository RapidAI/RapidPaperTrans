// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino-ext/components/model/openai"

	"latex-translator/internal/logger"
)

// EinoAgentFixer uses the eino framework's ReAct agent for intelligent LaTeX error fixing
type EinoAgentFixer struct {
	apiKey    string
	baseURL   string
	model     string
	texDir    string
	compiler  *LaTeXCompiler
	outputDir string
	maxSteps  int
}

// NewEinoAgentFixer creates a new eino-based agent fixer
func NewEinoAgentFixer(apiKey, baseURL, model string) *EinoAgentFixer {
	if model == "" {
		model = "gpt-4o"
	}
	return &EinoAgentFixer{
		apiKey:   apiKey,
		baseURL:  baseURL,
		model:    model,
		maxSteps: 20, // ReAct agent steps (each loop = 2 steps)
	}
}

// Tool parameter structs with jsonschema tags for eino's InferTool

// ReadFileParams parameters for read_file tool
type ReadFileParams struct {
	Filename string `json:"filename" jsonschema:"description=The name of the file to read (relative to the tex directory)"`
}

// WriteFileParams parameters for write_file tool
type WriteFileParams struct {
	Filename string `json:"filename" jsonschema:"description=The name of the file to write (relative to the tex directory)"`
	Content  string `json:"content" jsonschema:"description=The complete content to write to the file"`
}

// CompileLatexParams parameters for compile_latex tool
type CompileLatexParams struct {
	MainFile string `json:"main_file" jsonschema:"description=The main tex file to compile"`
}

// SearchParams parameters for search_in_files tool
type SearchParams struct {
	Pattern string `json:"pattern" jsonschema:"description=The pattern to search for (supports regex)"`
}

// FixCompleteParams parameters for fix_complete tool
type FixCompleteParams struct {
	Summary string `json:"summary" jsonschema:"description=A summary of all fixes applied"`
}

// createTools creates the tools for the eino agent
func (f *EinoAgentFixer) createTools() ([]tool.BaseTool, error) {
	// read_file tool
	readFileTool, err := utils.InferTool(
		"read_file",
		"Read the content of a LaTeX file. Use this to understand the document structure and find errors.",
		func(ctx context.Context, params *ReadFileParams) (string, error) {
			return f.toolReadFile(params.Filename)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create read_file tool: %w", err)
	}

	// write_file tool
	writeFileTool, err := utils.InferTool(
		"write_file",
		"Write content to a LaTeX file. Use this to apply fixes to the document.",
		func(ctx context.Context, params *WriteFileParams) (string, error) {
			return f.toolWriteFile(params.Filename, params.Content)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create write_file tool: %w", err)
	}

	// compile_latex tool
	compileLatexTool, err := utils.InferTool(
		"compile_latex",
		"Compile the LaTeX document and return the result. Use this to test if your fixes work.",
		func(ctx context.Context, params *CompileLatexParams) (string, error) {
			return f.toolCompileLatex(params.MainFile)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create compile_latex tool: %w", err)
	}

	// list_files tool
	listFilesTool, err := utils.InferTool(
		"list_files",
		"List all tex files in the directory. Use this to discover the document structure.",
		func(ctx context.Context, params *struct{}) (string, error) {
			return f.toolListFiles()
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create list_files tool: %w", err)
	}

	// search_in_files tool
	searchTool, err := utils.InferTool(
		"search_in_files",
		"Search for a pattern in all tex files. Use this to find specific LaTeX commands or content.",
		func(ctx context.Context, params *SearchParams) (string, error) {
			return f.toolSearchInFiles(params.Pattern)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create search_in_files tool: %w", err)
	}

	// fix_complete tool
	fixCompleteTool, err := utils.InferTool(
		"fix_complete",
		"Call this when you have successfully fixed all errors and compilation succeeds.",
		func(ctx context.Context, params *FixCompleteParams) (string, error) {
			return fmt.Sprintf("Fix complete: %s", params.Summary), nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create fix_complete tool: %w", err)
	}

	return []tool.BaseTool{
		readFileTool,
		writeFileTool,
		compileLatexTool,
		listFilesTool,
		searchTool,
		fixCompleteTool,
	}, nil
}

// Tool implementations

func (f *EinoAgentFixer) toolReadFile(filename string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filename, err)
	}
	// Truncate if too large
	if len(content) > 30000 {
		return string(content[:15000]) + "\n...[truncated]...\n" + string(content[len(content)-15000:]), nil
	}
	return string(content), nil
}

func (f *EinoAgentFixer) toolWriteFile(filename, content string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filename, err)
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filename), nil
}

func (f *EinoAgentFixer) toolCompileLatex(mainFile string) (string, error) {
	mainPath := filepath.Join(f.texDir, mainFile)
	result, err := f.compiler.CompileWithXeLaTeX(mainPath, f.outputDir)
	if err != nil {
		return fmt.Sprintf("Compilation error: %v", err), nil
	}
	if result.Success {
		return "Compilation successful! PDF generated.", nil
	}
	// Return relevant portion of log
	logExcerpt := extractRelevantLogPortion(result.Log, 5000)
	return fmt.Sprintf("Compilation failed.\n\nError log:\n%s", logExcerpt), nil
}

func (f *EinoAgentFixer) toolListFiles() (string, error) {
	var files []string
	err := filepath.Walk(f.texDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tex") || strings.HasSuffix(path, ".sty") || strings.HasSuffix(path, ".bbl") {
			relPath, _ := filepath.Rel(f.texDir, path)
			files = append(files, relPath)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Files found:\n%s", strings.Join(files, "\n")), nil
}

func (f *EinoAgentFixer) toolSearchInFiles(pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
	}

	var results []string
	filepath.Walk(f.texDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".tex") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		relPath, _ := filepath.Rel(f.texDir, path)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
				if len(results) > 50 {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if len(results) == 0 {
		return "No matches found", nil
	}
	return fmt.Sprintf("Found %d matches:\n%s", len(results), strings.Join(results, "\n")), nil
}

// EinoAgentFixResult represents the result of eino agent-based fixing
type EinoAgentFixResult struct {
	Success     bool              `json:"success"`
	FixedFiles  map[string]string `json:"fixed_files"`
	Summary     string            `json:"summary"`
	Steps       int               `json:"steps"`
}

// FixWithEinoAgent uses the eino framework's ReAct agent to fix LaTeX compilation errors
func (f *EinoAgentFixer) FixWithEinoAgent(
	ctx context.Context,
	texDir string,
	mainTexFile string,
	compileLog string,
	compiler *LaTeXCompiler,
	outputDir string,
	progressCallback func(step int, message string),
) (*EinoAgentFixResult, error) {
	f.texDir = texDir
	f.compiler = compiler
	f.outputDir = outputDir

	logger.Info("starting eino agent-based LaTeX fix",
		logger.String("texDir", texDir),
		logger.String("mainTexFile", mainTexFile),
		logger.String("model", f.model))

	result := &EinoAgentFixResult{
		Success:    false,
		FixedFiles: make(map[string]string),
	}

	// Create tools
	tools, err := f.createTools()
	if err != nil {
		return result, fmt.Errorf("failed to create tools: %w", err)
	}

	// Create OpenAI chat model
	chatModelConfig := &openai.ChatModelConfig{
		Model:  f.model,
		APIKey: f.apiKey,
	}
	if f.baseURL != "" {
		chatModelConfig.BaseURL = f.baseURL
	}

	chatModel, err := openai.NewChatModel(ctx, chatModelConfig)
	if err != nil {
		return result, fmt.Errorf("failed to create chat model: %w", err)
	}

	// Create ReAct agent
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MaxStep: f.maxSteps,
		MessageModifier: func(ctx context.Context, input []*schema.Message) []*schema.Message {
			// Add system message
			systemMsg := schema.SystemMessage(f.buildSystemPrompt())
			return append([]*schema.Message{systemMsg}, input...)
		},
	})
	if err != nil {
		return result, fmt.Errorf("failed to create ReAct agent: %w", err)
	}

	// Build initial user message
	userMessage := f.buildInitialUserMessage(mainTexFile, compileLog)

	// Run the agent
	if progressCallback != nil {
		progressCallback(1, "Eino Agent 开始分析...")
	}

	response, err := agent.Generate(ctx, []*schema.Message{
		schema.UserMessage(userMessage),
	})
	if err != nil {
		logger.Error("eino agent failed", err)
		return result, fmt.Errorf("agent execution failed: %w", err)
	}

	// Check if compilation succeeded
	mainPath := filepath.Join(texDir, mainTexFile)
	compileResult, _ := compiler.CompileWithXeLaTeX(mainPath, outputDir)
	if compileResult != nil && compileResult.Success {
		result.Success = true
		result.Summary = "Eino Agent 成功修复了编译错误"
		if response != nil && response.Content != "" {
			result.Summary = response.Content
		}
		logger.Info("eino agent fix succeeded")
	} else {
		result.Summary = "Eino Agent 未能完全修复编译错误"
		if response != nil && response.Content != "" {
			result.Summary = response.Content
		}
		logger.Warn("eino agent fix did not fully succeed")
	}

	// Collect modified files
	// Note: In a real implementation, we would track file modifications during tool execution
	// For now, we'll re-read the main file to see if it was modified
	if content, err := os.ReadFile(mainPath); err == nil {
		result.FixedFiles[mainTexFile] = string(content)
	}

	return result, nil
}

func (f *EinoAgentFixer) buildSystemPrompt() string {
	return `You are a LaTeX debugging agent. Your job is to fix compilation errors by locating the problem, understanding it, and making precise fixes.

TOOLS:
- read_file(filename): Read file content
- write_file(filename, content): Write file
- compile_latex(main_file): Compile and get error log
- search_in_files(pattern): Regex search across files
- list_files(): List all tex files
- fix_complete(summary): Call when compilation succeeds

WORKFLOW - Follow this process for each error:

STEP 1: PARSE THE ERROR
From the compilation log, extract:
- Error type (e.g., "Too many }'s", "Undefined control sequence", "Missing $ inserted")
- File name (from lines like "(./filename.tex" or the file context)
- Line number (from "l.XXX")

STEP 2: READ THE ERROR LOCATION
Use read_file to see the problematic area. Read at least 10-20 lines around the error line to understand the context.

STEP 3: ANALYZE AND FIX
Based on what you see:

For "Too many }'s" or "Extra }":
- Look for a standalone } on its own line that doesn't match any {
- Check if there's a } right before \end{document} that shouldn't be there
- Count braces in the surrounding context to find the mismatch

For "Missing } inserted" or "Missing { inserted":
- Find the unclosed brace or environment
- Look for \begin{} without matching \end{} or vice versa

For "Undefined control sequence":
- Check if a command name got corrupted (e.g., Chinese characters in command)
- Check if a package is missing

For "Missing $ inserted":
- Look for unescaped special characters like _, ^, & outside math mode
- Check for broken math delimiters

STEP 4: APPLY THE FIX
Make the minimal change needed. When writing the fixed file:
- Keep ALL existing content
- Only change what's necessary to fix the error
- Preserve all Chinese translations

STEP 5: VERIFY
Compile again. If there are more errors, repeat from Step 1.

IMPORTANT RULES:
1. Always read the file before fixing - don't guess
2. Make minimal changes - don't rewrite sections unnecessarily  
3. Never delete translated content
4. Fix one error at a time, then recompile
5. The first error in the log is usually the root cause

When compilation succeeds (PDF generated), call fix_complete.`
}


func (f *EinoAgentFixer) buildInitialUserMessage(mainTexFile, compileLog string) string {
	var sb strings.Builder
	sb.WriteString("Please fix the LaTeX compilation errors in this document.\n\n")
	sb.WriteString(fmt.Sprintf("Main file: %s\n\n", mainTexFile))
	sb.WriteString("Compilation log (relevant portion):\n")
	sb.WriteString("```\n")
	sb.WriteString(extractRelevantLogPortion(compileLog, 4000))
	sb.WriteString("\n```\n\n")
	sb.WriteString("Start by listing the files and reading the main tex file to understand the document structure. Then analyze the errors and fix them one by one.")
	return sb.String()
}
