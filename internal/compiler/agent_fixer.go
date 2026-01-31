// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"latex-translator/internal/editor"
	"latex-translator/internal/logger"
)

// AgentTool represents a tool that the agent can use
type AgentTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// AgentToolCall represents a tool call made by the agent
type AgentToolCall struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Arguments string `json:"arguments"`
}

// AgentMessage represents a message in the agent conversation
type AgentMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []AgentToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// LaTeXAgentFixer uses an agent-based approach to fix complex LaTeX errors
// It implements a ReAct-like pattern where the agent can:
// 1. Read files to understand the document structure
// 2. Analyze compilation errors
// 3. Make targeted fixes
// 4. Test compilation
// 5. Iterate until success or max attempts
type LaTeXAgentFixer struct {
	apiKey     string
	apiURL     string
	model      string
	client     *http.Client
	maxSteps   int
	texDir     string
	compiler   *LaTeXCompiler
	outputDir  string
	// Editor tools
	lineEditor      *editor.LineEditor
	encodingHandler *editor.EncodingHandler
	validator       *editor.LaTeXValidator
	backupMgr       *editor.BackupManager
}

// NewLaTeXAgentFixer creates a new agent-based LaTeX fixer
func NewLaTeXAgentFixer(apiKey, apiURL, model string) *LaTeXAgentFixer {
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1/chat/completions"
	} else if !strings.HasSuffix(apiURL, "/chat/completions") {
		apiURL = strings.TrimSuffix(apiURL, "/") + "/chat/completions"
	}
	if model == "" {
		model = "gpt-4o"
	}
	
	// Initialize editor tools
	backupMgr := editor.NewBackupManager("")
	lineEditor := editor.NewLineEditor(backupMgr)
	encodingHandler := editor.NewEncodingHandler(backupMgr)
	validator := editor.NewLaTeXValidator()
	
	return &LaTeXAgentFixer{
		apiKey:          apiKey,
		apiURL:          apiURL,
		model:           model,
		client:          &http.Client{Timeout: 300 * time.Second},
		maxSteps:        10,
		lineEditor:      lineEditor,
		encodingHandler: encodingHandler,
		validator:       validator,
		backupMgr:       backupMgr,
	}
}

// getTools returns the tools available to the agent
func (f *LaTeXAgentFixer) getTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read_file",
				"description": "Read the content of a LaTeX file. Use this to understand the document structure and find errors.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file to read (relative to the tex directory)",
						},
					},
					"required": []string{"filename"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read_lines",
				"description": "Read specific lines from a file. Use this to examine a specific section of a file.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file to read (relative to the tex directory)",
						},
						"start": map[string]interface{}{
							"type":        "integer",
							"description": "Starting line number (1-based)",
						},
						"end": map[string]interface{}{
							"type":        "integer",
							"description": "Ending line number (-1 for end of file)",
						},
					},
					"required": []string{"filename", "start", "end"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "write_file",
				"description": "Write content to a LaTeX file. Use this to apply fixes to the document.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file to write (relative to the tex directory)",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The complete content to write to the file",
						},
					},
					"required": []string{"filename", "content"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "replace_line",
				"description": "Replace a single line in a file. Use this for targeted fixes.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
						"line_number": map[string]interface{}{
							"type":        "integer",
							"description": "Line number to replace (1-based)",
						},
						"new_content": map[string]interface{}{
							"type":        "string",
							"description": "New content for the line",
						},
					},
					"required": []string{"filename", "line_number", "new_content"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "insert_line",
				"description": "Insert a new line at a specific position in a file.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
						"line_number": map[string]interface{}{
							"type":        "integer",
							"description": "Position to insert the line (1-based)",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Content to insert",
						},
					},
					"required": []string{"filename", "line_number", "content"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "delete_line",
				"description": "Delete a line from a file.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
						"line_number": map[string]interface{}{
							"type":        "integer",
							"description": "Line number to delete (1-based)",
						},
					},
					"required": []string{"filename", "line_number"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "detect_encoding",
				"description": "Detect the encoding of a file. Use this when you suspect encoding issues.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
					},
					"required": []string{"filename"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "fix_encoding",
				"description": "Fix encoding issues by converting a file to UTF-8. Use this when you detect encoding problems.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
					},
					"required": []string{"filename"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "validate_latex",
				"description": "Validate LaTeX syntax in a file. Use this to check for common errors.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
					},
					"required": []string{"filename"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "create_backup",
				"description": "Create a backup of a file before making changes.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The name of the file (relative to the tex directory)",
						},
					},
					"required": []string{"filename"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "compile_latex",
				"description": "Compile the LaTeX document and return the result. Use this to test if your fixes work.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"main_file": map[string]interface{}{
							"type":        "string",
							"description": "The main tex file to compile",
						},
					},
					"required": []string{"main_file"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_files",
				"description": "List all tex files in the directory. Use this to discover the document structure.",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "search_in_files",
				"description": "Search for a pattern in all tex files. Use this to find specific LaTeX commands or content.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "The pattern to search for (supports regex)",
						},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "fix_complete",
				"description": "Call this when you have successfully fixed all errors and compilation succeeds.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"summary": map[string]interface{}{
							"type":        "string",
							"description": "A summary of all fixes applied",
						},
					},
					"required": []string{"summary"},
				},
			},
		},
	}
}

// executeTool executes a tool call and returns the result
func (f *LaTeXAgentFixer) executeTool(name string, arguments string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	switch name {
	case "read_file":
		filename, _ := args["filename"].(string)
		return f.toolReadFile(filename)
	case "read_lines":
		filename, _ := args["filename"].(string)
		start, _ := args["start"].(float64)
		end, _ := args["end"].(float64)
		return f.toolReadLines(filename, int(start), int(end))
	case "write_file":
		filename, _ := args["filename"].(string)
		content, _ := args["content"].(string)
		return f.toolWriteFile(filename, content)
	case "replace_line":
		filename, _ := args["filename"].(string)
		lineNum, _ := args["line_number"].(float64)
		newContent, _ := args["new_content"].(string)
		return f.toolReplaceLine(filename, int(lineNum), newContent)
	case "insert_line":
		filename, _ := args["filename"].(string)
		lineNum, _ := args["line_number"].(float64)
		content, _ := args["content"].(string)
		return f.toolInsertLine(filename, int(lineNum), content)
	case "delete_line":
		filename, _ := args["filename"].(string)
		lineNum, _ := args["line_number"].(float64)
		return f.toolDeleteLine(filename, int(lineNum))
	case "detect_encoding":
		filename, _ := args["filename"].(string)
		return f.toolDetectEncoding(filename)
	case "fix_encoding":
		filename, _ := args["filename"].(string)
		return f.toolFixEncoding(filename)
	case "validate_latex":
		filename, _ := args["filename"].(string)
		return f.toolValidateLaTeX(filename)
	case "create_backup":
		filename, _ := args["filename"].(string)
		return f.toolCreateBackup(filename)
	case "compile_latex":
		mainFile, _ := args["main_file"].(string)
		return f.toolCompileLatex(mainFile)
	case "list_files":
		return f.toolListFiles()
	case "search_in_files":
		pattern, _ := args["pattern"].(string)
		return f.toolSearchInFiles(pattern)
	case "fix_complete":
		summary, _ := args["summary"].(string)
		return fmt.Sprintf("Fix complete: %s", summary), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (f *LaTeXAgentFixer) toolReadFile(filename string) (string, error) {
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

func (f *LaTeXAgentFixer) toolWriteFile(filename, content string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filename, err)
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filename), nil
}

func (f *LaTeXAgentFixer) toolCompileLatex(mainFile string) (string, error) {
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

func (f *LaTeXAgentFixer) toolListFiles() (string, error) {
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

func (f *LaTeXAgentFixer) toolSearchInFiles(pattern string) (string, error) {
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

// toolReadLines reads specific lines from a file
func (f *LaTeXAgentFixer) toolReadLines(filename string, start, end int) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	lines, err := f.lineEditor.ReadLines(filePath, start, end)
	if err != nil {
		return "", fmt.Errorf("failed to read lines from %s: %w", filename, err)
	}
	return fmt.Sprintf("Lines %d-%d from %s:\n%s", start, end, filename, strings.Join(lines, "\n")), nil
}

// toolReplaceLine replaces a single line in a file
func (f *LaTeXAgentFixer) toolReplaceLine(filename string, lineNum int, newContent string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	if err := f.lineEditor.ReplaceLine(filePath, lineNum, newContent); err != nil {
		return "", fmt.Errorf("failed to replace line %d in %s: %w", lineNum, filename, err)
	}
	return fmt.Sprintf("Successfully replaced line %d in %s", lineNum, filename), nil
}

// toolInsertLine inserts a new line in a file
func (f *LaTeXAgentFixer) toolInsertLine(filename string, lineNum int, content string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	if err := f.lineEditor.InsertLine(filePath, lineNum, content); err != nil {
		return "", fmt.Errorf("failed to insert line at %d in %s: %w", lineNum, filename, err)
	}
	return fmt.Sprintf("Successfully inserted line at %d in %s", lineNum, filename), nil
}

// toolDeleteLine deletes a line from a file
func (f *LaTeXAgentFixer) toolDeleteLine(filename string, lineNum int) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	if err := f.lineEditor.DeleteLine(filePath, lineNum); err != nil {
		return "", fmt.Errorf("failed to delete line %d from %s: %w", lineNum, filename, err)
	}
	return fmt.Sprintf("Successfully deleted line %d from %s", lineNum, filename), nil
}

// toolDetectEncoding detects the encoding of a file
func (f *LaTeXAgentFixer) toolDetectEncoding(filename string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	info, err := f.encodingHandler.GetEncodingInfo(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to detect encoding of %s: %w", filename, err)
	}
	
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Encoding information for %s:\n", filename))
	result.WriteString(fmt.Sprintf("  Encoding: %s\n", info.Encoding))
	result.WriteString(fmt.Sprintf("  Has BOM: %v\n", info.HasBOM))
	result.WriteString(fmt.Sprintf("  Is Valid: %v\n", info.IsValid))
	result.WriteString(fmt.Sprintf("  File Size: %d bytes\n", info.FileSize))
	if info.SampleText != "" {
		result.WriteString(fmt.Sprintf("  Sample: %s\n", info.SampleText))
	}
	
	return result.String(), nil
}

// toolFixEncoding fixes encoding issues in a file
func (f *LaTeXAgentFixer) toolFixEncoding(filename string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	
	// First detect the encoding
	encoding, err := f.encodingHandler.DetectEncoding(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to detect encoding of %s: %w", filename, err)
	}
	
	// If already UTF-8 without BOM, nothing to do
	if encoding == "UTF-8" {
		return fmt.Sprintf("File %s is already in UTF-8 encoding", filename), nil
	}
	
	// Fix encoding issues
	if err := f.encodingHandler.EnsureUTF8(filePath); err != nil {
		return "", fmt.Errorf("failed to fix encoding of %s: %w", filename, err)
	}
	
	return fmt.Sprintf("Successfully converted %s from %s to UTF-8", filename, encoding), nil
}

// toolValidateLaTeX validates LaTeX syntax in a file
func (f *LaTeXAgentFixer) toolValidateLaTeX(filename string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	report, err := f.validator.ValidateAndReport(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to validate %s: %w", filename, err)
	}
	return report, nil
}

// toolCreateBackup creates a backup of a file
func (f *LaTeXAgentFixer) toolCreateBackup(filename string) (string, error) {
	filePath := filepath.Join(f.texDir, filename)
	backupPath, err := f.backupMgr.CreateBackup(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup of %s: %w", filename, err)
	}
	return fmt.Sprintf("Backup created: %s", backupPath), nil
}

// AgentFixResult represents the result of agent-based fixing
type AgentFixResult struct {
	Success     bool              `json:"success"`
	FixedFiles  map[string]string `json:"fixed_files"`
	Summary     string            `json:"summary"`
	Steps       int               `json:"steps"`
	Conversation []AgentMessage   `json:"conversation,omitempty"`
}

// FixWithAgent uses an agent-based approach to fix LaTeX compilation errors
func (f *LaTeXAgentFixer) FixWithAgent(
	ctx context.Context,
	texDir string,
	mainTexFile string,
	compileLog string,
	compiler *LaTeXCompiler,
	outputDir string,
	progressCallback func(step int, message string),
) (*AgentFixResult, error) {
	f.texDir = texDir
	f.compiler = compiler
	f.outputDir = outputDir

	logger.Info("starting agent-based LaTeX fix",
		logger.String("texDir", texDir),
		logger.String("mainTexFile", mainTexFile))

	result := &AgentFixResult{
		Success:    false,
		FixedFiles: make(map[string]string),
	}

	// Build initial system prompt
	systemPrompt := f.buildSystemPrompt()

	// Build initial user message with error context
	userMessage := f.buildInitialUserMessage(mainTexFile, compileLog)

	// Initialize conversation
	messages := []map[string]interface{}{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userMessage},
	}

	// Agent loop
	for step := 1; step <= f.maxSteps; step++ {
		result.Steps = step

		if progressCallback != nil {
			progressCallback(step, fmt.Sprintf("Agent 修复步骤 %d/%d...", step, f.maxSteps))
		}

		// Call LLM
		response, err := f.callLLM(ctx, messages)
		if err != nil {
			logger.Error("agent LLM call failed", err)
			return result, err
		}

		// Check if there are tool calls
		if len(response.ToolCalls) > 0 {
			// Add assistant message with tool calls
			assistantMsg := map[string]interface{}{
				"role":       "assistant",
				"content":    response.Content,
				"tool_calls": response.ToolCalls,
			}
			messages = append(messages, assistantMsg)

			// Execute each tool call
			for _, toolCall := range response.ToolCalls {
				logger.Info("agent executing tool",
					logger.String("tool", toolCall.Name),
					logger.Int("step", step))

				toolResult, toolErr := f.executeTool(toolCall.Name, toolCall.Arguments)
				if toolErr != nil {
					toolResult = fmt.Sprintf("Error: %v", toolErr)
				}

				// Check for fix_complete
				if toolCall.Name == "fix_complete" {
					var args map[string]interface{}
					json.Unmarshal([]byte(toolCall.Arguments), &args)
					result.Summary, _ = args["summary"].(string)
					
					// Verify compilation actually succeeds
					mainPath := filepath.Join(texDir, mainTexFile)
					compileResult, _ := compiler.CompileWithXeLaTeX(mainPath, outputDir)
					if compileResult != nil && compileResult.Success {
						result.Success = true
						logger.Info("agent fix completed successfully", logger.String("summary", result.Summary))
						return result, nil
					}
					// If compilation still fails, continue
					toolResult = "Compilation still fails. Please continue fixing."
				}

				// Track modified files
				if toolCall.Name == "write_file" {
					var args map[string]interface{}
					json.Unmarshal([]byte(toolCall.Arguments), &args)
					if filename, ok := args["filename"].(string); ok {
						if content, ok := args["content"].(string); ok {
							result.FixedFiles[filename] = content
						}
					}
				}

				// Add tool result message
				toolMsg := map[string]interface{}{
					"role":         "tool",
					"tool_call_id": toolCall.ID,
					"content":      toolResult,
				}
				messages = append(messages, toolMsg)
			}
		} else {
			// No tool calls - agent is done or stuck
			if response.Content != "" {
				logger.Info("agent response without tool calls", logger.String("content", response.Content[:min(200, len(response.Content))]))
			}
			break
		}
	}

	logger.Warn("agent fix reached max steps without success", logger.Int("maxSteps", f.maxSteps))
	result.Summary = "Agent reached maximum steps without fully resolving errors"
	return result, nil
}

func (f *LaTeXAgentFixer) buildSystemPrompt() string {
	return `You are a LaTeX debugging agent. Your job is to fix compilation errors by locating the problem, understanding it, and making precise fixes.

TOOLS:
- read_file(filename): Read complete file
- read_lines(filename, start, end): Read specific lines (use for large files)
- write_file(filename, content): Write complete file
- replace_line(filename, line_number, new_content): Replace single line
- delete_line(filename, line_number): Delete a line
- insert_line(filename, line_number, content): Insert a line
- search_in_files(pattern): Regex search across files
- list_files(): List all tex files
- validate_latex(filename): Check syntax (braces, environments)
- detect_encoding(filename): Check file encoding
- fix_encoding(filename): Convert to UTF-8
- compile_latex(main_file): Compile and get error log
- create_backup(filename): Backup before risky changes
- fix_complete(summary): Call when compilation succeeds

WORKFLOW - Follow this process for each error:

STEP 1: PARSE THE ERROR
From the compilation log, extract:
- Error type (e.g., "Too many }'s", "Undefined control sequence")
- File name (from lines like "(./filename.tex")
- Line number (from "l.XXX")

STEP 2: READ THE ERROR LOCATION
Use read_lines(filename, line-10, line+10) to see context around the error.
For encoding errors (garbled text like "锟斤拷"), use detect_encoding first.

STEP 3: ANALYZE AND FIX
Based on what you see:

For "Too many }'s":
- Look for standalone } on its own line
- Check for } before \end{document} that shouldn't be there
- Use delete_line to remove the extra }

For "Missing }":
- Find unclosed { or \begin{} without \end{}
- Add the missing } or \end{}

For "Undefined control sequence":
- Check if command name is corrupted
- Fix the command name or add missing package

For encoding issues (garbled Chinese):
- Use fix_encoding(filename) to convert to UTF-8

STEP 4: APPLY THE FIX
Use the most precise tool:
- delete_line for removing a line
- replace_line for changing a line
- write_file only when multiple changes needed

STEP 5: VERIFY
Compile again. If more errors, repeat from Step 1.

RULES:
1. Always read before fixing - don't guess
2. Use replace_line/delete_line instead of write_file when possible
3. Never delete translated content
4. Fix one error at a time
5. First error is usually the root cause

When compilation succeeds, call fix_complete.`
}

func (f *LaTeXAgentFixer) buildInitialUserMessage(mainTexFile, compileLog string) string {
	var sb strings.Builder
	sb.WriteString("Please fix the LaTeX compilation errors in this document.\n\n")
	sb.WriteString(fmt.Sprintf("Main file: %s\n\n", mainTexFile))
	sb.WriteString("Compilation log (relevant portion):\n")
	sb.WriteString("```\n")
	sb.WriteString(extractRelevantLogPortion(compileLog, 4000))
	sb.WriteString("\n```\n\n")
	sb.WriteString("Start by listing the files and reading the main tex file to understand the document structure.")
	return sb.String()
}

// LLMResponse represents the response from the LLM
type LLMResponse struct {
	Content   string
	ToolCalls []AgentToolCall
}

func (f *LaTeXAgentFixer) callLLM(ctx context.Context, messages []map[string]interface{}) (*LLMResponse, error) {
	reqBody := map[string]interface{}{
		"model":      f.model,
		"messages":   messages,
		"tools":      f.getTools(),
		"max_tokens": 8192,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("API returned no choices")
	}

	result := &LLMResponse{
		Content: apiResp.Choices[0].Message.Content,
	}

	for _, tc := range apiResp.Choices[0].Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, AgentToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return result, nil
}
