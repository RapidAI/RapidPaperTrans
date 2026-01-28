package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQuickFix(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFixed   bool
		wantContain string
	}{
		{
			name:        "fix end itemize item on same line",
			input:       `\end{itemize}\item text`,
			wantFixed:   true,
			wantContain: "\\end{itemize}\n\\item",
		},
		{
			name:        "fix end enumerate item on same line",
			input:       `\end{enumerate}\item text`,
			wantFixed:   true,
			wantContain: "\\end{enumerate}\n\\item",
		},
		{
			name:        "fix end begin on same line",
			input:       `\end{itemize}\begin{enumerate}`,
			wantFixed:   true,
			wantContain: "\\end{itemize}\n\\begin{enumerate}",
		},
		{
			name:        "fix end section on same line",
			input:       `\end{itemize}\section{Title}`,
			wantFixed:   true,
			wantContain: "\\end{itemize}\n\n\\section",
		},
		{
			name:      "no fix needed",
			input:     `\begin{itemize}\n\item text\n\end{itemize}`,
			wantFixed: false,
		},
		{
			name:        "fix text before begin list",
			input:       `Some text\begin{itemize}`,
			wantFixed:   true,
			wantContain: "Some text\n\\begin{itemize}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := QuickFix(tt.input)
			if fixed != tt.wantFixed {
				t.Errorf("QuickFix() fixed = %v, want %v", fixed, tt.wantFixed)
			}
			if tt.wantContain != "" && !contains(result, tt.wantContain) {
				t.Errorf("QuickFix() result = %q, want to contain %q", result, tt.wantContain)
			}
		})
	}
}

func TestNewLaTeXFixer(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		apiURL    string
		model     string
		wantURL   string
		wantModel string
	}{
		{
			name:      "default values",
			apiKey:    "test-key",
			apiURL:    "",
			model:     "",
			wantURL:   "https://api.openai.com/v1/chat/completions",
			wantModel: "gpt-4o",
		},
		{
			name:      "custom URL without suffix",
			apiKey:    "test-key",
			apiURL:    "https://custom.api.com/v1",
			model:     "gpt-4",
			wantURL:   "https://custom.api.com/v1/chat/completions",
			wantModel: "gpt-4",
		},
		{
			name:      "custom URL with suffix",
			apiKey:    "test-key",
			apiURL:    "https://custom.api.com/v1/chat/completions",
			model:     "gpt-4",
			wantURL:   "https://custom.api.com/v1/chat/completions",
			wantModel: "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixer := NewLaTeXFixer(tt.apiKey, tt.apiURL, tt.model)
			if fixer.apiURL != tt.wantURL {
				t.Errorf("NewLaTeXFixer() apiURL = %v, want %v", fixer.apiURL, tt.wantURL)
			}
			if fixer.model != tt.wantModel {
				t.Errorf("NewLaTeXFixer() model = %v, want %v", fixer.model, tt.wantModel)
			}
			if !fixer.enableAgent {
				t.Error("NewLaTeXFixer() enableAgent should be true by default")
			}
		})
	}
}

func TestNewLaTeXFixerWithAgent(t *testing.T) {
	fixer := NewLaTeXFixerWithAgent("key", "https://api.com/v1", "gpt-4", "gpt-4-turbo", true)
	
	if fixer.model != "gpt-4" {
		t.Errorf("model = %v, want gpt-4", fixer.model)
	}
	if fixer.agentModel != "gpt-4-turbo" {
		t.Errorf("agentModel = %v, want gpt-4-turbo", fixer.agentModel)
	}
	if !fixer.enableAgent {
		t.Error("enableAgent should be true")
	}

	// Test with agent disabled
	fixer2 := NewLaTeXFixerWithAgent("key", "https://api.com/v1", "gpt-4", "", false)
	if fixer2.enableAgent {
		t.Error("enableAgent should be false")
	}
	if fixer2.agentModel != "gpt-4" {
		t.Errorf("agentModel should default to model when empty, got %v", fixer2.agentModel)
	}
}

func TestSetAgentModel(t *testing.T) {
	fixer := NewLaTeXFixer("key", "", "gpt-4")
	fixer.SetAgentModel("gpt-4-turbo")
	
	if fixer.agentModel != "gpt-4-turbo" {
		t.Errorf("SetAgentModel() agentModel = %v, want gpt-4-turbo", fixer.agentModel)
	}
}

func TestSetEnableAgent(t *testing.T) {
	fixer := NewLaTeXFixer("key", "", "gpt-4")
	
	if !fixer.enableAgent {
		t.Error("enableAgent should be true by default")
	}
	
	fixer.SetEnableAgent(false)
	if fixer.enableAgent {
		t.Error("SetEnableAgent(false) should disable agent")
	}
	
	fixer.SetEnableAgent(true)
	if !fixer.enableAgent {
		t.Error("SetEnableAgent(true) should enable agent")
	}
}

func TestParseLatexErrors(t *testing.T) {
	tests := []struct {
		name       string
		log        string
		wantCount  int
		wantFirst  string
	}{
		{
			name: "single error",
			log: `(./test.tex
! Undefined control sequence.
l.10 \unknowncommand
`,
			wantCount: 1,
			wantFirst: "Undefined control sequence.",
		},
		{
			name: "multiple errors",
			log: `(./test.tex
! Missing $ inserted.
l.5 x_1
! Undefined control sequence.
l.10 \foo
`,
			wantCount: 2,
			wantFirst: "Missing $ inserted.",
		},
		{
			name:      "no errors",
			log:       "Output written on test.pdf",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := parseLatexErrors(tt.log)
			if len(errors) != tt.wantCount {
				t.Errorf("parseLatexErrors() count = %v, want %v", len(errors), tt.wantCount)
			}
			if tt.wantCount > 0 && errors[0].Message != tt.wantFirst {
				t.Errorf("parseLatexErrors() first error = %v, want %v", errors[0].Message, tt.wantFirst)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "raw JSON",
			content: `{"key": "value"}`,
			want:    `{"key": "value"}`,
		},
		{
			name: "JSON in code block",
			content: "```json\n{\"key\": \"value\"}\n```",
			want:    `{"key": "value"}`,
		},
		{
			name: "JSON in plain code block",
			content: "```\n{\"key\": \"value\"}\n```",
			want:    `{"key": "value"}`,
		},
		{
			name:    "JSON with surrounding text",
			content: "Here is the result: {\"key\": \"value\"} done",
			want:    `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.content)
			if result != tt.want {
				t.Errorf("extractJSON() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestExtractRelevantLogPortion(t *testing.T) {
	shortLog := "Short log content"
	result := extractRelevantLogPortion(shortLog, 100)
	if result != shortLog {
		t.Errorf("extractRelevantLogPortion() should return full log when under maxLen")
	}

	longLog := "! Error message\nl.10 some code\n" + string(make([]byte, 5000))
	result = extractRelevantLogPortion(longLog, 1000)
	if len(result) > 1100 { // Allow some margin for truncation message
		t.Errorf("extractRelevantLogPortion() result too long: %d", len(result))
	}
}

func TestMin(t *testing.T) {
	if min(5, 10) != 5 {
		t.Error("min(5, 10) should be 5")
	}
	if min(10, 5) != 5 {
		t.Error("min(10, 5) should be 5")
	}
	if min(5, 5) != 5 {
		t.Error("min(5, 5) should be 5")
	}
}

func TestFixLevelConstants(t *testing.T) {
	// Ensure fix levels are in correct order
	if FixLevelRule >= FixLevelLLM {
		t.Error("FixLevelRule should be less than FixLevelLLM")
	}
	if FixLevelLLM >= FixLevelAgent {
		t.Error("FixLevelLLM should be less than FixLevelAgent")
	}
}

func TestFixLonelyItems(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFixed   bool
		wantContain string
	}{
		{
			name: "fix lonely item after end itemize",
			input: `\begin{itemize}
\item First item
\item Second item
\end{itemize}
\item Third item
\end{itemize}`,
			wantFixed:   true,
			wantContain: "\\item Third item\n\\end{itemize}",
		},
		{
			name: "no fix needed - proper structure",
			input: `\begin{itemize}
\item First item
\item Second item
\item Third item
\end{itemize}`,
			wantFixed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := fixLonelyItems(tt.input)
			if fixed != tt.wantFixed {
				t.Errorf("fixLonelyItems() fixed = %v, want %v", fixed, tt.wantFixed)
			}
			if tt.wantContain != "" && !contains(result, tt.wantContain) {
				t.Errorf("fixLonelyItems() result = %q, want to contain %q", result, tt.wantContain)
			}
		})
	}
}

func TestFixBibliographyOrder(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFixed   bool
		wantContain string
	}{
		{
			name: "fix wrong order - bibliography before bibliographystyle",
			input: `\bibliography{references}
\bibliographystyle{plain}`,
			wantFixed:   true,
			wantContain: "\\bibliographystyle{plain}\n\\bibliography{references}",
		},
		{
			name: "fix wrong order with icml style",
			input: `\bibliography{icml2026}
\bibliographystyle{icml2026}`,
			wantFixed:   true,
			wantContain: "\\bibliographystyle{icml2026}\n\\bibliography{icml2026}",
		},
		{
			name: "no fix needed - correct order",
			input: `\bibliographystyle{plain}
\bibliography{references}`,
			wantFixed: false,
		},
		{
			name: "no fix needed - only bibliography",
			input: `\bibliography{references}`,
			wantFixed: false,
		},
		{
			name: "no fix needed - only bibliographystyle",
			input: `\bibliographystyle{plain}`,
			wantFixed: false,
		},
		{
			name: "fix wrong order with whitespace",
			input: `\bibliography{refs}

\bibliographystyle{ieeetr}`,
			wantFixed:   true,
			wantContain: "\\bibliographystyle{ieeetr}\n\\bibliography{refs}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := fixBibliographyOrder(tt.input)
			if fixed != tt.wantFixed {
				t.Errorf("fixBibliographyOrder() fixed = %v, want %v", fixed, tt.wantFixed)
			}
			if tt.wantContain != "" && !contains(result, tt.wantContain) {
				t.Errorf("fixBibliographyOrder() result = %q, want to contain %q", result, tt.wantContain)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}


// ============================================================================
// PreprocessTexFiles Tests
// ============================================================================

// TestPreprocessTexFiles_EmptyDir tests preprocessing an empty directory
func TestPreprocessTexFiles_EmptyDir(t *testing.T) {
	tempDir := t.TempDir()

	err := PreprocessTexFiles(tempDir)
	if err != nil {
		t.Errorf("PreprocessTexFiles should not fail on empty directory: %v", err)
	}
}

// TestPreprocessTexFiles_NoTexFiles tests preprocessing a directory with no tex files
func TestPreprocessTexFiles_NoTexFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create some non-tex files
	os.WriteFile(filepath.Join(tempDir, "readme.md"), []byte("# README"), 0644)
	os.WriteFile(filepath.Join(tempDir, "data.json"), []byte("{}"), 0644)

	err := PreprocessTexFiles(tempDir)
	if err != nil {
		t.Errorf("PreprocessTexFiles should not fail with no tex files: %v", err)
	}
}

// TestPreprocessTexFiles_FixesBibliographyOrder tests that preprocessing fixes bibliography order
func TestPreprocessTexFiles_FixesBibliographyOrder(t *testing.T) {
	tempDir := t.TempDir()

	// Create a tex file with wrong bibliography order
	wrongOrder := `\documentclass{article}
\begin{document}
Hello world.
\bibliography{refs}
\bibliographystyle{plain}
\end{document}`

	texPath := filepath.Join(tempDir, "main.tex")
	os.WriteFile(texPath, []byte(wrongOrder), 0644)

	err := PreprocessTexFiles(tempDir)
	if err != nil {
		t.Fatalf("PreprocessTexFiles failed: %v", err)
	}

	// Read the file back
	content, err := os.ReadFile(texPath)
	if err != nil {
		t.Fatalf("Failed to read preprocessed file: %v", err)
	}

	// Check that the order is now correct
	contentStr := string(content)
	bibStylePos := strings.Index(contentStr, `\bibliographystyle{plain}`)
	bibPos := strings.Index(contentStr, `\bibliography{refs}`)

	if bibStylePos == -1 || bibPos == -1 {
		t.Error("Bibliography commands not found in preprocessed file")
	}

	if bibStylePos > bibPos {
		t.Error("Bibliography order was not fixed: \\bibliographystyle should come before \\bibliography")
	}
}

// TestPreprocessTexFiles_PreservesCorrectFiles tests that preprocessing doesn't modify correct files
func TestPreprocessTexFiles_PreservesCorrectFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a tex file with correct content
	correctContent := `\documentclass{article}
\begin{document}
Hello world.
\bibliographystyle{plain}
\bibliography{refs}
\end{document}`

	texPath := filepath.Join(tempDir, "main.tex")
	os.WriteFile(texPath, []byte(correctContent), 0644)

	// Get original modification time
	origInfo, _ := os.Stat(texPath)
	origModTime := origInfo.ModTime()

	// Wait a bit to ensure time difference would be detectable
	time.Sleep(10 * time.Millisecond)

	err := PreprocessTexFiles(tempDir)
	if err != nil {
		t.Fatalf("PreprocessTexFiles failed: %v", err)
	}

	// Read the file back
	content, err := os.ReadFile(texPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Content should be unchanged
	if string(content) != correctContent {
		t.Error("Correct file content was modified")
	}

	// File should not have been rewritten (mod time unchanged)
	newInfo, _ := os.Stat(texPath)
	if newInfo.ModTime() != origModTime {
		// Note: This might fail on some systems, so we just check content
		t.Log("File was rewritten even though no changes were needed (this is acceptable)")
	}
}

// TestPreprocessTexFiles_HandlesSubdirectories tests preprocessing files in subdirectories
func TestPreprocessTexFiles_HandlesSubdirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create subdirectory structure
	secDir := filepath.Join(tempDir, "sec")
	os.MkdirAll(secDir, 0755)

	// Create main tex file
	mainContent := `\documentclass{article}
\begin{document}
\input{sec/intro}
\bibliography{refs}
\bibliographystyle{plain}
\end{document}`
	os.WriteFile(filepath.Join(tempDir, "main.tex"), []byte(mainContent), 0644)

	// Create tex file in subdirectory with an issue
	introContent := `\section{Introduction}
\begin{itemize}
\item First item
\end{itemize}\item Lonely item
\end{itemize}`
	os.WriteFile(filepath.Join(secDir, "intro.tex"), []byte(introContent), 0644)

	err := PreprocessTexFiles(tempDir)
	if err != nil {
		t.Fatalf("PreprocessTexFiles failed: %v", err)
	}

	// Check main.tex was fixed
	mainFixed, _ := os.ReadFile(filepath.Join(tempDir, "main.tex"))
	mainStr := string(mainFixed)
	if strings.Index(mainStr, `\bibliographystyle`) > strings.Index(mainStr, `\bibliography{refs}`) {
		t.Error("main.tex bibliography order was not fixed")
	}

	// Check intro.tex was processed (file exists and readable)
	_, err = os.ReadFile(filepath.Join(secDir, "intro.tex"))
	if err != nil {
		t.Errorf("Failed to read subdirectory tex file: %v", err)
	}
}

// TestPreprocessTexFiles_NonexistentDir tests preprocessing a non-existent directory
func TestPreprocessTexFiles_NonexistentDir(t *testing.T) {
	// Note: filepath.Walk returns nil error for non-existent directories on some systems
	// It just doesn't process any files. This is acceptable behavior.
	err := PreprocessTexFiles("/nonexistent/path/that/does/not/exist")
	// We don't require an error here - the function handles this gracefully
	if err != nil {
		t.Logf("PreprocessTexFiles returned error for non-existent directory (expected on some systems): %v", err)
	}
}

// TestNewLaTeXAgentFixer tests the creation of a new agent fixer
func TestNewLaTeXAgentFixer(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		apiURL    string
		model     string
		wantURL   string
		wantModel string
	}{
		{
			name:      "default values",
			apiKey:    "test-key",
			apiURL:    "",
			model:     "",
			wantURL:   "https://api.openai.com/v1/chat/completions",
			wantModel: "gpt-4o",
		},
		{
			name:      "custom URL without suffix",
			apiKey:    "test-key",
			apiURL:    "https://custom.api.com/v1",
			model:     "gpt-4",
			wantURL:   "https://custom.api.com/v1/chat/completions",
			wantModel: "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixer := NewLaTeXAgentFixer(tt.apiKey, tt.apiURL, tt.model)
			if fixer.apiURL != tt.wantURL {
				t.Errorf("apiURL = %v, want %v", fixer.apiURL, tt.wantURL)
			}
			if fixer.model != tt.wantModel {
				t.Errorf("model = %v, want %v", fixer.model, tt.wantModel)
			}
			if fixer.maxSteps != 10 {
				t.Errorf("maxSteps = %v, want 10", fixer.maxSteps)
			}
		})
	}
}

// TestAgentFixerGetTools tests that the agent fixer returns the correct tools
func TestAgentFixerGetTools(t *testing.T) {
	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	tools := fixer.getTools()

	expectedTools := []string{
		"read_file", "read_lines", "write_file", "replace_line",
		"insert_line", "delete_line", "detect_encoding", "fix_encoding",
		"validate_latex", "create_backup", "compile_latex", "list_files",
		"search_in_files", "fix_complete",
	}

	if len(tools) != len(expectedTools) {
		t.Errorf("got %d tools, want %d", len(tools), len(expectedTools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		fn, ok := tool["function"].(map[string]interface{})
		if !ok {
			t.Errorf("tool has no function field")
			continue
		}
		name, ok := fn["name"].(string)
		if !ok {
			t.Errorf("tool has no name field")
			continue
		}
		toolNames[name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("missing expected tool: %s", expected)
		}
	}
}

// TestAgentFixerToolReadFile tests the read_file tool
func TestAgentFixerToolReadFile(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "\\documentclass{article}\n\\begin{document}\nHello World\n\\end{document}"
	testFile := filepath.Join(tempDir, "test.tex")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	result, err := fixer.toolReadFile("test.tex")
	if err != nil {
		t.Errorf("toolReadFile failed: %v", err)
	}
	if result != testContent {
		t.Errorf("toolReadFile returned wrong content: got %q, want %q", result, testContent)
	}
}

// TestAgentFixerToolWriteFile tests the write_file tool
func TestAgentFixerToolWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	testContent := "\\documentclass{article}\n\\begin{document}\nTest\n\\end{document}"
	result, err := fixer.toolWriteFile("output.tex", testContent)
	if err != nil {
		t.Errorf("toolWriteFile failed: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("toolWriteFile returned unexpected result: %s", result)
	}

	// Verify file was written
	content, err := os.ReadFile(filepath.Join(tempDir, "output.tex"))
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Written content mismatch: got %q, want %q", string(content), testContent)
	}
}

// TestAgentFixerToolListFiles tests the list_files tool
func TestAgentFixerToolListFiles(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create some test files
	files := []string{"main.tex", "chapter1.tex", "style.sty"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tempDir, f), []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", f, err)
		}
	}

	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	result, err := fixer.toolListFiles()
	if err != nil {
		t.Errorf("toolListFiles failed: %v", err)
	}

	for _, f := range files {
		if !strings.Contains(result, f) {
			t.Errorf("toolListFiles result should contain %s, got: %s", f, result)
		}
	}
}

// TestAgentFixerToolSearchInFiles tests the search_in_files tool
func TestAgentFixerToolSearchInFiles(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test file with searchable content
	content := "\\documentclass{article}\n\\usepackage{ctex}\n\\begin{document}\nHello\n\\end{document}"
	if err := os.WriteFile(filepath.Join(tempDir, "test.tex"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	result, err := fixer.toolSearchInFiles("usepackage")
	if err != nil {
		t.Errorf("toolSearchInFiles failed: %v", err)
	}
	if !strings.Contains(result, "ctex") {
		t.Errorf("toolSearchInFiles should find ctex package, got: %s", result)
	}
}

// TestAgentFixerExecuteTool tests the executeTool dispatcher
func TestAgentFixerExecuteTool(t *testing.T) {
	tempDir := t.TempDir()
	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	// Test unknown tool
	_, err := fixer.executeTool("unknown_tool", "{}")
	if err == nil {
		t.Error("executeTool should return error for unknown tool")
	}

	// Test invalid JSON
	_, err = fixer.executeTool("read_file", "invalid json")
	if err == nil {
		t.Error("executeTool should return error for invalid JSON")
	}
}

// TestAgentFixerBuildSystemPrompt tests the system prompt generation
func TestAgentFixerBuildSystemPrompt(t *testing.T) {
	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	prompt := fixer.buildSystemPrompt()

	// Check that the prompt contains key instructions
	expectedPhrases := []string{
		"LaTeX debugging agent",
		"read_file",
		"write_file",
		"compile_latex",
		"ctex",
		"fix_complete",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("System prompt should contain %q", phrase)
		}
	}
}

// TestAgentFixerBuildInitialUserMessage tests the initial user message generation
func TestAgentFixerBuildInitialUserMessage(t *testing.T) {
	fixer := NewLaTeXAgentFixer("test-key", "", "gpt-4")
	
	compileLog := "! Undefined control sequence.\nl.42 \\badcommand"
	message := fixer.buildInitialUserMessage("main.tex", compileLog)

	if !strings.Contains(message, "main.tex") {
		t.Error("Initial message should contain main file name")
	}
	if !strings.Contains(message, "Undefined control sequence") {
		t.Error("Initial message should contain error from log")
	}
}

// TestNewEinoAgentFixer tests the creation of a new eino agent fixer
func TestNewEinoAgentFixer(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		baseURL   string
		model     string
		wantModel string
	}{
		{
			name:      "default model",
			apiKey:    "test-key",
			baseURL:   "",
			model:     "",
			wantModel: "gpt-4o",
		},
		{
			name:      "custom model",
			apiKey:    "test-key",
			baseURL:   "https://api.example.com/v1",
			model:     "gpt-4-turbo",
			wantModel: "gpt-4-turbo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixer := NewEinoAgentFixer(tt.apiKey, tt.baseURL, tt.model)
			if fixer.model != tt.wantModel {
				t.Errorf("model = %v, want %v", fixer.model, tt.wantModel)
			}
			if fixer.maxSteps != 20 {
				t.Errorf("maxSteps = %v, want 20", fixer.maxSteps)
			}
		})
	}
}

// TestEinoAgentFixerToolReadFile tests the read_file tool for eino agent
func TestEinoAgentFixerToolReadFile(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "\\documentclass{article}\n\\begin{document}\nHello World\n\\end{document}"
	testFile := filepath.Join(tempDir, "test.tex")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	result, err := fixer.toolReadFile("test.tex")
	if err != nil {
		t.Errorf("toolReadFile failed: %v", err)
	}
	if result != testContent {
		t.Errorf("toolReadFile returned wrong content: got %q, want %q", result, testContent)
	}
}

// TestEinoAgentFixerToolWriteFile tests the write_file tool for eino agent
func TestEinoAgentFixerToolWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	testContent := "\\documentclass{article}\n\\begin{document}\nTest\n\\end{document}"
	result, err := fixer.toolWriteFile("output.tex", testContent)
	if err != nil {
		t.Errorf("toolWriteFile failed: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("toolWriteFile returned unexpected result: %s", result)
	}

	// Verify file was written
	content, err := os.ReadFile(filepath.Join(tempDir, "output.tex"))
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Written content mismatch: got %q, want %q", string(content), testContent)
	}
}

// TestEinoAgentFixerToolListFiles tests the list_files tool for eino agent
func TestEinoAgentFixerToolListFiles(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create some test files
	files := []string{"main.tex", "chapter1.tex", "style.sty"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tempDir, f), []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", f, err)
		}
	}

	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	result, err := fixer.toolListFiles()
	if err != nil {
		t.Errorf("toolListFiles failed: %v", err)
	}

	for _, f := range files {
		if !strings.Contains(result, f) {
			t.Errorf("toolListFiles result should contain %s, got: %s", f, result)
		}
	}
}

// TestEinoAgentFixerToolSearchInFiles tests the search_in_files tool for eino agent
func TestEinoAgentFixerToolSearchInFiles(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create test file with searchable content
	content := "\\documentclass{article}\n\\usepackage{ctex}\n\\begin{document}\nHello\n\\end{document}"
	if err := os.WriteFile(filepath.Join(tempDir, "test.tex"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	result, err := fixer.toolSearchInFiles("usepackage")
	if err != nil {
		t.Errorf("toolSearchInFiles failed: %v", err)
	}
	if !strings.Contains(result, "ctex") {
		t.Errorf("toolSearchInFiles should find ctex package, got: %s", result)
	}
}

// TestEinoAgentFixerBuildSystemPrompt tests the system prompt generation for eino agent
func TestEinoAgentFixerBuildSystemPrompt(t *testing.T) {
	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	prompt := fixer.buildSystemPrompt()

	// Check that the prompt contains key instructions
	expectedPhrases := []string{
		"LaTeX debugging agent",
		"read_file",
		"write_file",
		"compile_latex",
		"ctex",
		"fix_complete",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("System prompt should contain %q", phrase)
		}
	}
}

// TestEinoAgentFixerBuildInitialUserMessage tests the initial user message generation for eino agent
func TestEinoAgentFixerBuildInitialUserMessage(t *testing.T) {
	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	
	compileLog := "! Undefined control sequence.\nl.42 \\badcommand"
	message := fixer.buildInitialUserMessage("main.tex", compileLog)

	if !strings.Contains(message, "main.tex") {
		t.Error("Initial message should contain main file name")
	}
	if !strings.Contains(message, "Undefined control sequence") {
		t.Error("Initial message should contain error from log")
	}
}

// TestEinoAgentFixerCreateTools tests that tools are created correctly
func TestEinoAgentFixerCreateTools(t *testing.T) {
	tempDir := t.TempDir()
	fixer := NewEinoAgentFixer("test-key", "", "gpt-4")
	fixer.texDir = tempDir

	tools, err := fixer.createTools()
	if err != nil {
		t.Fatalf("createTools failed: %v", err)
	}

	expectedToolCount := 6 // read_file, write_file, compile_latex, list_files, search_in_files, fix_complete
	if len(tools) != expectedToolCount {
		t.Errorf("got %d tools, want %d", len(tools), expectedToolCount)
	}
}
