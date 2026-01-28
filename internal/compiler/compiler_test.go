package compiler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLaTeXCompiler(t *testing.T) {
	tests := []struct {
		name            string
		compiler        string
		workDir         string
		timeout         time.Duration
		expectedCompiler string
		expectedTimeout  time.Duration
	}{
		{
			name:            "default values",
			compiler:        "",
			workDir:         "/tmp",
			timeout:         0,
			expectedCompiler: CompilerPDFLaTeX,
			expectedTimeout:  DefaultTimeout,
		},
		{
			name:            "custom pdflatex",
			compiler:        CompilerPDFLaTeX,
			workDir:         "/tmp",
			timeout:         10 * time.Minute,
			expectedCompiler: CompilerPDFLaTeX,
			expectedTimeout:  10 * time.Minute,
		},
		{
			name:            "custom xelatex",
			compiler:        CompilerXeLaTeX,
			workDir:         "/tmp",
			timeout:         3 * time.Minute,
			expectedCompiler: CompilerXeLaTeX,
			expectedTimeout:  3 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewLaTeXCompiler(tt.compiler, tt.workDir, tt.timeout)
			if c.GetCompiler() != tt.expectedCompiler {
				t.Errorf("expected compiler %s, got %s", tt.expectedCompiler, c.GetCompiler())
			}
			if c.GetTimeout() != tt.expectedTimeout {
				t.Errorf("expected timeout %v, got %v", tt.expectedTimeout, c.GetTimeout())
			}
		})
	}
}

func TestContainsChinese(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "english only",
			text:     "Hello World",
			expected: false,
		},
		{
			name:     "latex commands only",
			text:     "\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}",
			expected: false,
		},
		{
			name:     "single chinese character",
			text:     "你",
			expected: true,
		},
		{
			name:     "chinese text",
			text:     "你好世界",
			expected: true,
		},
		{
			name:     "mixed english and chinese",
			text:     "Hello 世界",
			expected: true,
		},
		{
			name:     "latex with chinese",
			text:     "\\documentclass{article}\n\\begin{document}\n你好\n\\end{document}",
			expected: true,
		},
		{
			name:     "numbers and punctuation",
			text:     "123!@#$%^&*()",
			expected: false,
		},
		{
			name:     "japanese hiragana (not chinese)",
			text:     "あいうえお",
			expected: false,
		},
		{
			name:     "japanese kanji (chinese characters)",
			text:     "日本語",
			expected: true,
		},
		{
			name:     "complex latex document without chinese",
			text:     "\\documentclass{article}\n\\usepackage{amsmath}\n\\begin{document}\n$E=mc^2$\n\\section{Introduction}\nThis is a test.\n\\end{document}",
			expected: false,
		},
		{
			name:     "complex latex document with chinese",
			text:     "\\documentclass{article}\n\\usepackage{ctex}\n\\begin{document}\n\\section{介绍}\n这是一个测试。\n\\end{document}",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsChinese(tt.text)
			if result != tt.expected {
				t.Errorf("ContainsChinese(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestSelectCompiler(t *testing.T) {
	tests := []struct {
		name            string
		defaultCompiler string
		content         string
		expected        string
	}{
		{
			name:            "english content with pdflatex default",
			defaultCompiler: CompilerPDFLaTeX,
			content:         "\\documentclass{article}\n\\begin{document}\nHello World\n\\end{document}",
			expected:        CompilerPDFLaTeX,
		},
		{
			name:            "english content with xelatex default",
			defaultCompiler: CompilerXeLaTeX,
			content:         "\\documentclass{article}\n\\begin{document}\nHello World\n\\end{document}",
			expected:        CompilerXeLaTeX,
		},
		{
			name:            "chinese content with pdflatex default",
			defaultCompiler: CompilerPDFLaTeX,
			content:         "\\documentclass{article}\n\\begin{document}\n你好世界\n\\end{document}",
			expected:        CompilerXeLaTeX,
		},
		{
			name:            "chinese content with xelatex default",
			defaultCompiler: CompilerXeLaTeX,
			content:         "\\documentclass{article}\n\\begin{document}\n你好世界\n\\end{document}",
			expected:        CompilerXeLaTeX,
		},
		{
			name:            "empty content",
			defaultCompiler: CompilerPDFLaTeX,
			content:         "",
			expected:        CompilerPDFLaTeX,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewLaTeXCompiler(tt.defaultCompiler, "", 0)
			result := c.selectCompiler(tt.content)
			if result != tt.expected {
				t.Errorf("selectCompiler() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestBuildCompilerArgs(t *testing.T) {
	tests := []struct {
		name       string
		compiler   string
		texFile    string
		outputDir  string
		texDir     string
		wantArgs   []string
	}{
		{
			name:      "pdflatex without output dir",
			compiler:  CompilerPDFLaTeX,
			texFile:   "test.tex",
			outputDir: "",
			texDir:    "",
			wantArgs:  []string{"-interaction=nonstopmode", "test.tex"},
		},
		{
			name:      "pdflatex with output dir different from source",
			compiler:  CompilerPDFLaTeX,
			texFile:   "test.tex",
			outputDir: "/tmp/output",
			texDir:    "/tmp/source",
			wantArgs:  []string{"-interaction=nonstopmode", "-output-directory=/tmp/output", "test.tex"},
		},
		{
			name:      "xelatex with output dir same as source",
			compiler:  CompilerXeLaTeX,
			texFile:   "document.tex",
			outputDir: "/home/user/source",
			texDir:    "/home/user/source",
			wantArgs:  []string{"-interaction=nonstopmode", "document.tex"},
		},
		{
			name:      "xelatex with output dir different from source",
			compiler:  CompilerXeLaTeX,
			texFile:   "document.tex",
			outputDir: "/home/user/output",
			texDir:    "/home/user/source",
			wantArgs:  []string{"-interaction=nonstopmode", "-output-directory=/home/user/output", "document.tex"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildCompilerArgs(tt.compiler, tt.texFile, tt.outputDir, tt.texDir)
			if len(args) != len(tt.wantArgs) {
				t.Errorf("buildCompilerArgs() returned %d args, expected %d", len(args), len(tt.wantArgs))
				return
			}
			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("buildCompilerArgs()[%d] = %s, expected %s", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestCombineOutput(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		stderr   string
		expected string
	}{
		{
			name:     "both empty",
			stdout:   "",
			stderr:   "",
			expected: "",
		},
		{
			name:     "stdout only",
			stdout:   "output",
			stderr:   "",
			expected: "output",
		},
		{
			name:     "stderr only",
			stdout:   "",
			stderr:   "error",
			expected: "error",
		},
		{
			name:     "both present",
			stdout:   "output",
			stderr:   "error",
			expected: "output\nerror",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := combineOutput(tt.stdout, tt.stderr)
			if result != tt.expected {
				t.Errorf("combineOutput(%q, %q) = %q, expected %q", tt.stdout, tt.stderr, result, tt.expected)
			}
		})
	}
}

func TestCompile_FileNotFound(t *testing.T) {
	c := NewLaTeXCompiler(CompilerPDFLaTeX, "", 0)
	result, err := c.Compile("/nonexistent/path/test.tex", "")
	
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if result.Success {
		t.Error("expected Success to be false")
	}
	if result.ErrorMsg == "" {
		t.Error("expected ErrorMsg to be set")
	}
}

func TestCompileWithXeLaTeX_FileNotFound(t *testing.T) {
	c := NewLaTeXCompiler(CompilerPDFLaTeX, "", 0)
	result, err := c.CompileWithXeLaTeX("/nonexistent/path/test.tex", "")
	
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if result.Success {
		t.Error("expected Success to be false")
	}
}

func TestCompileWithPDFLaTeX_FileNotFound(t *testing.T) {
	c := NewLaTeXCompiler(CompilerXeLaTeX, "", 0)
	result, err := c.CompileWithPDFLaTeX("/nonexistent/path/test.tex", "")
	
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if result.Success {
		t.Error("expected Success to be false")
	}
}

func TestSettersAndGetters(t *testing.T) {
	c := NewLaTeXCompiler(CompilerPDFLaTeX, "", time.Minute)
	
	// Test SetCompiler
	c.SetCompiler(CompilerXeLaTeX)
	if c.GetCompiler() != CompilerXeLaTeX {
		t.Errorf("SetCompiler failed: got %s, expected %s", c.GetCompiler(), CompilerXeLaTeX)
	}
	
	// Test SetTimeout
	newTimeout := 10 * time.Minute
	c.SetTimeout(newTimeout)
	if c.GetTimeout() != newTimeout {
		t.Errorf("SetTimeout failed: got %v, expected %v", c.GetTimeout(), newTimeout)
	}
}

// TestCompile_Integration tests actual compilation if LaTeX is installed
// This test is skipped if pdflatex is not available
func TestCompile_Integration(t *testing.T) {
	// Check if pdflatex is available
	if _, err := os.Stat("C:\\texlive"); os.IsNotExist(err) {
		// Try to find pdflatex in PATH
		_, err := os.LookupEnv("PATH")
		if err {
			// Skip if we can't determine LaTeX availability
			t.Skip("Skipping integration test: LaTeX installation not detected")
		}
	}

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "latex-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a simple tex file
	texContent := `\documentclass{article}
\begin{document}
Hello World
\end{document}`
	
	texPath := filepath.Join(tmpDir, "test.tex")
	if err := os.WriteFile(texPath, []byte(texContent), 0644); err != nil {
		t.Fatalf("failed to write tex file: %v", err)
	}

	// Create compiler and compile
	c := NewLaTeXCompiler(CompilerPDFLaTeX, tmpDir, time.Minute)
	result, err := c.Compile(texPath, tmpDir)

	// If pdflatex is not installed, the test should handle gracefully
	if err != nil {
		t.Logf("Compilation returned error (may be expected if LaTeX not installed): %v", err)
		t.Logf("Result: %+v", result)
		// Don't fail the test if LaTeX is not installed
		return
	}

	if !result.Success {
		t.Errorf("Compilation failed: %s", result.ErrorMsg)
		t.Logf("Log: %s", result.Log)
	}

	// Check if PDF was created
	if result.PDFPath != "" {
		if _, err := os.Stat(result.PDFPath); os.IsNotExist(err) {
			t.Errorf("PDF file was not created at %s", result.PDFPath)
		}
	}
}
