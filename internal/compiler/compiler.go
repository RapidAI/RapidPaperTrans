// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode"

	"latex-translator/internal/editor"
	"latex-translator/internal/logger"
	"latex-translator/internal/types"
	"latex-translator/internal/validator"
)

const (
	// CompilerPDFLaTeX is the pdflatex compiler
	CompilerPDFLaTeX = "pdflatex"
	// CompilerXeLaTeX is the xelatex compiler
	CompilerXeLaTeX = "xelatex"
	// CompilerLuaLaTeX is the lualatex compiler
	CompilerLuaLaTeX = "lualatex"
)

// DefaultTimeout is the default compilation timeout
const DefaultTimeout = 5 * time.Minute

// LaTeXCompiler is responsible for compiling LaTeX documents
type LaTeXCompiler struct {
	compiler string        // "pdflatex" or "xelatex"
	workDir  string        // working directory
	timeout  time.Duration // compilation timeout
}

// NewLaTeXCompiler creates a new LaTeXCompiler instance
func NewLaTeXCompiler(compiler string, workDir string, timeout time.Duration) *LaTeXCompiler {
	if compiler == "" {
		compiler = CompilerPDFLaTeX
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &LaTeXCompiler{
		compiler: compiler,
		workDir:  workDir,
		timeout:  timeout,
	}
}

// Compile compiles a tex file to PDF using the default compiler.
// It automatically selects xelatex if the document contains Chinese characters.
// It also applies QuickFix to fix common LaTeX issues before compilation.
func (c *LaTeXCompiler) Compile(texPath string, outputDir string) (*types.CompileResult, error) {
	logger.Info("compiling tex file", logger.String("texPath", texPath), logger.String("outputDir", outputDir))

	// Step 1: Fix encoding issues automatically
	texDir := filepath.Dir(texPath)
	if err := c.autoFixEncoding(texPath, texDir); err != nil {
		logger.Warn("encoding auto-fix failed", logger.Err(err))
		// Continue anyway - encoding might be fine
	}

	// Step 2: Apply TikZ and syntax fixes to all tex files
	// NOTE: Disabled for now as it can cause issues with some documents
	// if err := c.autoFixSyntax(texDir); err != nil {
	// 	logger.Warn("syntax auto-fix failed", logger.Err(err))
	// 	// Continue anyway
	// }

	// Read the tex file to check for Chinese characters
	content, err := os.ReadFile(texPath)
	if err != nil {
		logger.Error("failed to read tex file", err, logger.String("texPath", texPath))
		return &types.CompileResult{
			Success:  false,
			ErrorMsg: fmt.Sprintf("failed to read tex file: %v", err),
		}, types.NewAppError(types.ErrFileNotFound, "failed to read tex file", err)
	}

	// Apply QuickFix to fix common LaTeX issues (like bibliography order)
	// NOTE: QuickFix is designed for translated documents and can break original documents
	// Only apply it to files that look like translated documents (contain "translated_" in name)
	texBaseName := filepath.Base(texPath)
	if strings.HasPrefix(texBaseName, "translated_") {
		fixedContent, wasFixed := QuickFix(string(content))
		if wasFixed {
			logger.Info("applied QuickFix before compilation", logger.String("texPath", texPath))
			if err := os.WriteFile(texPath, []byte(fixedContent), 0644); err != nil {
				logger.Warn("failed to write QuickFix changes", logger.Err(err))
			}
			content = []byte(fixedContent)
		}
	}

	// Auto-select compiler based on content (including input files)
	compiler := c.selectCompilerWithInputFiles(texPath)
	logger.Debug("selected compiler", logger.String("compiler", compiler))

	// If using xelatex, ensure xeCJK is available
	if compiler == CompilerXeLaTeX {
		c.ensureChineseSupport(texPath, string(content))
	}

	return c.compileWithCompiler(texPath, outputDir, compiler)
}

// CompileWithXeLaTeX compiles a tex file using xelatex (for Chinese documents)
func (c *LaTeXCompiler) CompileWithXeLaTeX(texPath string, outputDir string) (*types.CompileResult, error) {
	logger.Info("compiling with XeLaTeX", logger.String("texPath", texPath))

	// Read the tex file
	content, err := os.ReadFile(texPath)
	if err != nil {
		logger.Error("failed to read tex file", err, logger.String("texPath", texPath))
		return &types.CompileResult{
			Success:  false,
			ErrorMsg: fmt.Sprintf("failed to read tex file: %v", err),
		}, types.NewAppError(types.ErrFileNotFound, "failed to read tex file", err)
	}

	// Apply QuickFix to fix common LaTeX issues (like breakurl compatibility)
	fixedContent, wasFixed := QuickFix(string(content))
	if wasFixed {
		logger.Info("applied QuickFix before XeLaTeX compilation", logger.String("texPath", texPath))
		if err := os.WriteFile(texPath, []byte(fixedContent), 0644); err != nil {
			logger.Warn("failed to write QuickFix changes", logger.Err(err))
		}
	}

	return c.compileWithCompiler(texPath, outputDir, CompilerXeLaTeX)
}

// CompileWithPDFLaTeX compiles a tex file using pdflatex
func (c *LaTeXCompiler) CompileWithPDFLaTeX(texPath string, outputDir string) (*types.CompileResult, error) {
	logger.Info("compiling with PDFLaTeX", logger.String("texPath", texPath))
	return c.compileWithCompiler(texPath, outputDir, CompilerPDFLaTeX)
}

// CompileWithLuaLaTeX compiles a tex file using lualatex
// LuaLaTeX has better UTF-8 support than XeLaTeX, especially for Chinese characters on Windows
func (c *LaTeXCompiler) CompileWithLuaLaTeX(texPath string, outputDir string) (*types.CompileResult, error) {
	logger.Info("compiling with LuaLaTeX", logger.String("texPath", texPath))
	
	// Apply QuickFix before compilation
	content, err := os.ReadFile(texPath)
	if err == nil {
		fixedContent, fixed := QuickFix(string(content))
		if fixed {
			logger.Info("applied QuickFix before LuaLaTeX compilation", logger.String("texPath", texPath))
			if err := os.WriteFile(texPath, []byte(fixedContent), 0644); err != nil {
				logger.Warn("failed to write QuickFix changes", logger.Err(err))
			}
		}
	}
	
	return c.compileWithCompiler(texPath, outputDir, CompilerLuaLaTeX)
}

// selectCompiler selects the appropriate compiler based on content.
// If the content contains Chinese characters, xelatex is selected.
// It also checks all \input and \include files for Chinese content.
// Otherwise, the default compiler is used.
func (c *LaTeXCompiler) selectCompiler(content string) string {
	if ContainsChinese(content) {
		return CompilerXeLaTeX
	}
	return c.compiler
}

// selectCompilerWithInputFiles selects the appropriate compiler by checking
// the main file and all referenced input files for Chinese content.
// NOTE: We now only use xelatex if the document explicitly requires it (uses fontspec, xeCJK, or ctex).
// Simply having Chinese characters is not enough, as pdflatex with CJKutf8 can handle many cases.
func (c *LaTeXCompiler) selectCompilerWithInputFiles(mainTexPath string) string {
	// Read main file
	content, err := os.ReadFile(mainTexPath)
	if err != nil {
		return c.compiler
	}

	contentStr := string(content)

	// Check if document explicitly requires xelatex
	// These packages require xelatex/lualatex
	xelatexPackages := []string{
		"\\usepackage{fontspec}",
		"\\usepackage{xeCJK}",
		"\\usepackage{ctex}",
		"\\usepackage[ctex]",
		"\\RequirePackage{fontspec}",
		"\\RequirePackage{xeCJK}",
		"\\RequirePackage{ctex}",
	}

	for _, pkg := range xelatexPackages {
		if strings.Contains(contentStr, pkg) {
			logger.Debug("xelatex-specific package detected, using xelatex", logger.String("package", pkg))
			return CompilerXeLaTeX
		}
	}

	// Also check document class files (.cls) in the same directory
	texDir := filepath.Dir(mainTexPath)
	clsFiles, _ := filepath.Glob(filepath.Join(texDir, "*.cls"))
	for _, clsFile := range clsFiles {
		if clsContent, err := os.ReadFile(clsFile); err == nil {
			clsStr := string(clsContent)
			for _, pkg := range xelatexPackages {
				if strings.Contains(clsStr, pkg) {
					logger.Debug("xelatex-specific package detected in class file, using xelatex",
						logger.String("classFile", clsFile),
						logger.String("package", pkg))
					return CompilerXeLaTeX
				}
			}
		}
	}

	// Default to the configured compiler (usually pdflatex)
	return c.compiler
}

// ContainsChinese checks if the given text contains Chinese characters.
// This is used to determine whether to use xelatex for compilation.
func ContainsChinese(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// compileWithCompiler performs the actual compilation using the specified compiler
func (c *LaTeXCompiler) compileWithCompiler(texPath string, outputDir string, compiler string) (*types.CompileResult, error) {
	logger.Debug("starting compilation", logger.String("compiler", compiler), logger.String("texPath", texPath))

	// Validate tex file exists
	if _, err := os.Stat(texPath); os.IsNotExist(err) {
		logger.Error("tex file not found", err, logger.String("texPath", texPath))
		return &types.CompileResult{
			Success:  false,
			ErrorMsg: fmt.Sprintf("tex file not found: %s", texPath),
		}, types.NewAppError(types.ErrFileNotFound, "tex file not found", err)
	}

	// Try compilation first without fixes
	result, err := c.tryCompile(texPath, outputDir, compiler)
	if err == nil && result.Success {
		return result, nil
	}

	// If compilation failed, try to fix and recompile
	logger.Info("initial compilation failed, attempting automatic fixes", logger.String("texPath", texPath))
	
	// Use SimpleFixer from validator package
	texDir := filepath.Dir(texPath)
	fixer := validator.NewSimpleFixer(texDir)
	fixResult, fixErr := fixer.SmartFix(texPath)
	
	if fixErr != nil {
		logger.Warn("auto-fix failed", logger.Err(fixErr))
		// Return original compilation result
		return result, err
	}

	if fixResult != nil && fixResult.Fixed {
		logger.Info("applied automatic fixes, retrying compilation",
			logger.Int("fixCount", len(fixResult.FixesApplied)))

		// Retry compilation with fixed file
		retryResult, retryErr := c.tryCompile(texPath, outputDir, compiler)
		if retryErr == nil && retryResult.Success {
			logger.Info("compilation succeeded after automatic fixes")
			// Add fix information to the log
			retryResult.Log = fmt.Sprintf("=== Automatic Fixes Applied ===\n%s\n\n%s",
				strings.Join(fixResult.FixesApplied, "\n"), retryResult.Log)
			
			// Clean up backup on success
			if fixResult.BackupPath != "" {
				os.Remove(fixResult.BackupPath)
				logger.Debug("cleaned up backup file", logger.String("path", fixResult.BackupPath))
			}
			
			return retryResult, nil
		}

		logger.Warn("compilation still failed after fixes, restoring backup")
		// Restore backup if retry failed
		if fixResult.BackupPath != "" {
			if restoreErr := fixer.RestoreBackup(texPath); restoreErr != nil {
				logger.Error("failed to restore backup", restoreErr)
			}
		}
	}

	// Return original compilation result
	return result, err
}

// tryCompile attempts to compile without any modifications
func (c *LaTeXCompiler) tryCompile(texPath string, outputDir string, compiler string) (*types.CompileResult, error) {

	// Create output directory if it doesn't exist
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			logger.Error("failed to create output directory", err, logger.String("outputDir", outputDir))
			return &types.CompileResult{
				Success:  false,
				ErrorMsg: fmt.Sprintf("failed to create output directory: %v", err),
			}, types.NewAppError(types.ErrInternal, "failed to create output directory", err)
		}
	}

	// Get absolute paths
	absTexPath, err := filepath.Abs(texPath)
	if err != nil {
		logger.Error("failed to get absolute path", err, logger.String("texPath", texPath))
		return &types.CompileResult{
			Success:  false,
			ErrorMsg: fmt.Sprintf("failed to get absolute path: %v", err),
		}, types.NewAppError(types.ErrInternal, "failed to get absolute path", err)
	}

	// Determine working directory (directory containing the tex file)
	texDir := filepath.Dir(absTexPath)
	texFileName := filepath.Base(absTexPath)
	texBaseName := strings.TrimSuffix(texFileName, filepath.Ext(texFileName))

	// Determine output directory
	var absOutputDir string
	if outputDir != "" {
		absOutputDir, err = filepath.Abs(outputDir)
		if err != nil {
			logger.Error("failed to get absolute output path", err, logger.String("outputDir", outputDir))
			return &types.CompileResult{
				Success:  false,
				ErrorMsg: fmt.Sprintf("failed to get absolute output path: %v", err),
			}, types.NewAppError(types.ErrInternal, "failed to get absolute output path", err)
		}
	} else {
		absOutputDir = texDir
	}

	// Create an empty .aux file in source directory if it doesn't exist
	// This is needed because xelatex with -output-directory tries to read .aux from source dir at \end{document}
	// Some packages like lastpage require this file to exist
	if absOutputDir != texDir {
		auxPath := filepath.Join(texDir, texBaseName+".aux")
		if _, err := os.Stat(auxPath); os.IsNotExist(err) {
			if err := os.WriteFile(auxPath, []byte("\\relax \n"), 0644); err != nil {
				logger.Warn("failed to create empty .aux file", logger.Err(err))
			} else {
				logger.Debug("created empty .aux file in source directory")
			}
		}
	}

	var allLogs []string

	// Check for missing .bib files and inline .bbl if available
	// This handles cases where arXiv source only includes .bbl but tex references .bib via \bibliography{}
	var skipBibtex bool
	texContent, err := os.ReadFile(absTexPath)
	if err == nil {
		inlinedContent, wasInlined := fixMissingBibFile(string(texContent), texDir, texBaseName)
		if wasInlined {
			if err := os.WriteFile(absTexPath, []byte(inlinedContent), 0644); err != nil {
				logger.Warn("failed to write inlined bibliography", logger.Err(err))
			} else {
				logger.Info("inlined .bbl file due to missing .bib file",
					logger.String("texPath", absTexPath))
				skipBibtex = true
			}
		}
	}

	// For translated documents (files starting with "translated_"), inline the bibliography
	// This ensures that bibliography references work correctly even if LaTeX stops early due to errors
	if !skipBibtex && strings.HasPrefix(texBaseName, "translated_") {
		originalBaseName := strings.TrimPrefix(texBaseName, "translated_")
		originalBblPath := filepath.Join(texDir, originalBaseName+".bbl")

		// Check if original .bbl exists
		if originalBblInfo, err := os.Stat(originalBblPath); err == nil && originalBblInfo.Size() > 0 {
			// Read the .bbl file content
			if bblContent, err := os.ReadFile(originalBblPath); err == nil {
				// Read the tex file content
				if texContent, err := os.ReadFile(absTexPath); err == nil {
					// Check if the file already has thebibliography environment
					// If so, don't inline again to avoid duplicates
					if strings.Contains(string(texContent), `\begin{thebibliography}`) {
						logger.Debug("skipping bibliography inlining - file already has thebibliography environment",
							logger.String("texPath", absTexPath))
					} else {
						// Inline the bibliography: replace \bibliography{...} with the .bbl content
						inlinedContent, wasInlined := inlineBibliography(string(texContent), string(bblContent))
						if wasInlined {
							// Write back the modified tex file
							if err := os.WriteFile(absTexPath, []byte(inlinedContent), 0644); err != nil {
								logger.Warn("failed to write inlined bibliography", logger.Err(err))
							} else {
								logger.Info("inlined bibliography into translated document",
									logger.String("texPath", absTexPath),
									logger.Int64("bblSize", originalBblInfo.Size()))
								// Skip bibtex since we've inlined the bibliography
								skipBibtex = true
							}
						}
					}
				}
			}
		}
	}

	// First pass: compile to generate .aux file
	logger.Debug("first compilation pass")
	log1, err := c.runCompiler(compiler, texFileName, texDir, absOutputDir)
	allLogs = append(allLogs, "=== First Pass ===", log1)
	if err != nil {
		// Continue even if first pass has errors - we still want to try bibtex
		logger.Warn("first pass had errors, continuing", logger.Err(err))
	}

	// Copy .aux file from output directory to source directory
	// This is needed because xelatex with -output-directory tries to read .aux from source dir at \end{document}
	if absOutputDir != texDir {
		auxSrc := filepath.Join(absOutputDir, texBaseName+".aux")
		auxDst := filepath.Join(texDir, texBaseName+".aux")
		if auxContent, err := os.ReadFile(auxSrc); err == nil {
			if err := os.WriteFile(auxDst, auxContent, 0644); err != nil {
				logger.Warn("failed to copy .aux file to source directory", logger.Err(err))
			} else {
				logger.Debug("copied .aux file to source directory")
			}
		}
	}

	// Check if there's a .bib file or bibliography commands
	auxPath := filepath.Join(absOutputDir, texBaseName+".aux")
	needsBibtex := c.checkNeedsBibtex(auxPath, texDir)

	if needsBibtex && !skipBibtex {
		// Run bibtex
		logger.Debug("running bibtex")
		bibtexLog, bibtexErr := c.runBibtex(texBaseName, texDir, absOutputDir)
		allLogs = append(allLogs, "=== BibTeX ===", bibtexLog)
		if bibtexErr != nil {
			logger.Warn("bibtex had errors", logger.Err(bibtexErr))
		}

		// Copy .bbl file from output directory to source directory
		// This is needed because LaTeX runs in texDir but bibtex outputs to absOutputDir
		bblSrc := filepath.Join(absOutputDir, texBaseName+".bbl")
		bblDst := filepath.Join(texDir, texBaseName+".bbl")
		if bblContent, err := os.ReadFile(bblSrc); err == nil {
			if err := os.WriteFile(bblDst, bblContent, 0644); err != nil {
				logger.Warn("failed to copy .bbl file", logger.Err(err))
			} else {
				logger.Debug("copied .bbl file to source directory")
			}
		}

		// Second pass: resolve citations
		logger.Debug("second compilation pass")
		log2, _ := c.runCompiler(compiler, texFileName, texDir, absOutputDir)
		allLogs = append(allLogs, "=== Second Pass ===", log2)
		// Copy .aux file after second pass
		copyAuxFile(absOutputDir, texDir, texBaseName)

		// Third pass: resolve cross-references
		logger.Debug("third compilation pass")
		log3, _ := c.runCompiler(compiler, texFileName, texDir, absOutputDir)
		allLogs = append(allLogs, "=== Third Pass ===", log3)
		// Copy .aux file after third pass
		copyAuxFile(absOutputDir, texDir, texBaseName)
	} else if skipBibtex {
		// We have a pre-existing .bbl file (from original document), run additional passes to resolve citations
		logger.Debug("using pre-existing .bbl file, running additional compilation passes")

		// Second pass: resolve citations
		logger.Debug("second compilation pass")
		log2, _ := c.runCompiler(compiler, texFileName, texDir, absOutputDir)
		allLogs = append(allLogs, "=== Second Pass ===", log2)
		// Copy .aux file after second pass
		copyAuxFile(absOutputDir, texDir, texBaseName)

		// Third pass: resolve cross-references
		logger.Debug("third compilation pass")
		log3, _ := c.runCompiler(compiler, texFileName, texDir, absOutputDir)
		allLogs = append(allLogs, "=== Third Pass ===", log3)
		// Copy .aux file after third pass
		copyAuxFile(absOutputDir, texDir, texBaseName)
	} else {
		// Just run a second pass for cross-references
		logger.Debug("second compilation pass (no bibtex needed)")
		log2, _ := c.runCompiler(compiler, texFileName, texDir, absOutputDir)
		allLogs = append(allLogs, "=== Second Pass ===", log2)
		// Copy .aux file after second pass
		copyAuxFile(absOutputDir, texDir, texBaseName)
	}

	// Combine all logs
	combinedLog := strings.Join(allLogs, "\n")

	// Determine PDF path
	pdfName := texBaseName + ".pdf"
	pdfPath := filepath.Join(absOutputDir, pdfName)

	// Verify PDF was created
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		logger.Error("PDF file was not generated", nil, logger.String("expectedPath", pdfPath))
		return &types.CompileResult{
			Success:  false,
			Log:      combinedLog,
			ErrorMsg: "PDF file was not generated",
		}, types.NewAppError(types.ErrCompile, "PDF file was not generated", nil)
	}

	logger.Info("compilation completed successfully", logger.String("pdfPath", pdfPath))
	return &types.CompileResult{
		Success: true,
		PDFPath: pdfPath,
		Log:     combinedLog,
	}, nil
}

// runCompiler executes a single compilation pass
func (c *LaTeXCompiler) runCompiler(compiler string, texFileName string, texDir string, outputDir string) (string, error) {
	args := buildCompilerArgs(compiler, texFileName, outputDir, texDir)

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, compiler, args...)
	cmd.Dir = texDir

	// Set TEXINPUTS to include the source directory so LaTeX can find .sty, .cls, etc.
	// This is needed when using -output-directory because LaTeX may not find files
	// in the source directory otherwise
	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}
	// The trailing path separator means "also search default paths"
	texInputs := fmt.Sprintf("TEXINPUTS=.%s%s%s", pathSep, texDir, pathSep)
	cmd.Env = append(os.Environ(), texInputs)

	// Hide console window on Windows
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	log := combineOutput(stdout.String(), stderr.String())

	if ctx.Err() == context.DeadlineExceeded {
		return log, types.NewAppError(types.ErrCompile, "compilation timed out", ctx.Err())
	}

	return log, err
}

// runBibtex executes bibtex to process bibliography
func (c *LaTeXCompiler) runBibtex(baseName string, texDir string, outputDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// bibtex needs to run in the output directory where .aux file is
	// This is important because TeX Live's security settings (openout_any = p)
	// prevent bibtex from writing to directories other than the current one
	workDir := outputDir
	if outputDir == "" {
		workDir = texDir
	}

	cmd := exec.CommandContext(ctx, "bibtex", baseName)
	cmd.Dir = workDir

	// Set BIBINPUTS to include the source directory for .bib files
	// Set BSTINPUTS to include the source directory for .bst files
	// Use semicolon on Windows, colon on Unix
	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}
	// Include both texDir and outputDir in BIBINPUTS/BSTINPUTS so bibtex can find .bib and .bst files
	// The trailing path separator means "also search default paths"
	bibInputs := fmt.Sprintf("BIBINPUTS=%s%s%s%s", texDir, pathSep, outputDir, pathSep)
	bstInputs := fmt.Sprintf("BSTINPUTS=%s%s%s%s", texDir, pathSep, outputDir, pathSep)
	cmd.Env = append(os.Environ(), bibInputs, bstInputs)

	// Hide console window on Windows
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	log := combineOutput(stdout.String(), stderr.String())

	return log, err
}

// copyAuxFile copies the .aux file from output directory to source directory
// This is needed because xelatex with -output-directory tries to read .aux from source dir at \end{document}
func copyAuxFile(outputDir, texDir, baseName string) {
	if outputDir == texDir {
		return
	}
	auxSrc := filepath.Join(outputDir, baseName+".aux")
	auxDst := filepath.Join(texDir, baseName+".aux")
	if auxContent, err := os.ReadFile(auxSrc); err == nil {
		if err := os.WriteFile(auxDst, auxContent, 0644); err != nil {
			logger.Warn("failed to copy .aux file to source directory", logger.Err(err))
		}
	}
}

// checkNeedsBibtex checks if bibtex needs to be run by looking for \citation commands in .aux file
// or .bib files in the source directory
func (c *LaTeXCompiler) checkNeedsBibtex(auxPath string, texDir string) bool {
	// Check if .aux file contains \citation or \bibdata commands
	if auxContent, err := os.ReadFile(auxPath); err == nil {
		auxStr := string(auxContent)
		if strings.Contains(auxStr, "\\citation{") || strings.Contains(auxStr, "\\bibdata{") {
			logger.Debug("found citation/bibdata in aux file, bibtex needed")
			return true
		}
	}

	// Check if there are .bib files in the source directory
	files, err := os.ReadDir(texDir)
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".bib") {
				logger.Debug("found .bib file, bibtex needed", logger.String("file", f.Name()))
				return true
			}
		}
	}

	return false
}

// buildCompilerArgs builds the command line arguments for the LaTeX compiler
func buildCompilerArgs(compiler string, texFileName string, outputDir string, texDir string) []string {
	args := []string{
		"-interaction=nonstopmode", // Don't stop on errors, continue processing
		// Note: We intentionally don't use -halt-on-error here because some documents
		// can still generate valid PDFs despite having non-fatal errors (e.g., undefined
		// control sequences in hyperref/url packages that don't affect the output).
	}

	// Add output directory if specified and different from source directory
	// When outputDir equals texDir, we don't use -output-directory to avoid issues
	// with complex LaTeX documents that may not handle this parameter well
	if outputDir != "" && outputDir != texDir {
		args = append(args, fmt.Sprintf("-output-directory=%s", outputDir))
	}

	// Add the tex file
	args = append(args, texFileName)

	return args
}

// combineOutput combines stdout and stderr into a single log string
func combineOutput(stdout, stderr string) string {
	var parts []string
	if stdout != "" {
		parts = append(parts, stdout)
	}
	if stderr != "" {
		parts = append(parts, stderr)
	}
	return strings.Join(parts, "\n")
}

// GetCompiler returns the default compiler
func (c *LaTeXCompiler) GetCompiler() string {
	return c.compiler
}

// GetTimeout returns the compilation timeout
func (c *LaTeXCompiler) GetTimeout() time.Duration {
	return c.timeout
}

// SetCompiler sets the default compiler
func (c *LaTeXCompiler) SetCompiler(compiler string) {
	c.compiler = compiler
}

// SetTimeout sets the compilation timeout
func (c *LaTeXCompiler) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// inlineBibliography replaces \bibliography{...} command with the actual .bbl content.
// This is useful for translated documents where LaTeX might stop early due to errors,
// preventing the \bibliography command from being processed.
// It also fixes compatibility issues with xelatex by redefining problematic commands.
// Returns the modified content and a boolean indicating if the replacement was made.
func inlineBibliography(texContent string, bblContent string) (string, bool) {
	// Check if the file already has thebibliography environment
	// If so, don't inline again to avoid duplicates
	if strings.Contains(texContent, `\begin{thebibliography}`) {
		logger.Debug("skipping inlineBibliography - file already has thebibliography environment")
		return texContent, false
	}

	// Pattern to match \bibliography{...} command (only non-commented lines)
	// This matches \bibliography{refs} or \bibliography{refs,other} etc.
	// Use multiline mode to ensure we only match lines that don't start with %
	bibPattern := regexp.MustCompile(`(?m)^([^%\n]*)\\bibliography\{[^}]+\}[^\n]*`)

	if !bibPattern.MatchString(texContent) {
		return texContent, false
	}

	// Check if the \bibliography command is in the preamble (before \begin{document})
	// If so, skip inlining to avoid putting thebibliography in the preamble
	beginDocIdx := strings.Index(texContent, `\begin{document}`)
	if beginDocIdx != -1 {
		match := bibPattern.FindStringIndex(texContent)
		if match != nil && match[0] < beginDocIdx {
			logger.Debug("skipping inlineBibliography - \\bibliography is in preamble",
				logger.Int("bibIdx", match[0]),
				logger.Int("beginDocIdx", beginDocIdx))
			return texContent, false
		}
	}

	// Fix xelatex compatibility issues in the .bbl content
	// The breakurl package's \burl command uses \pdf@box which is not available in xelatex
	// We need to redefine \burl to use a simpler implementation
	fixedBblContent := fixBblForXelatex(bblContent)

	// Replace \bibliography{...} with the fixed .bbl content
	// The .bbl file contains the \begin{thebibliography}...\end{thebibliography} environment
	// Preserve any content before \bibliography on the same line (captured in $1)
	result := bibPattern.ReplaceAllString(texContent, "${1}"+fixedBblContent)

	logger.Debug("inlined bibliography content",
		logger.Int("bblLength", len(fixedBblContent)))

	return result, true
}

// fixBblForXelatex fixes compatibility issues in .bbl content for xelatex.
// The main issue is that the breakurl package's \burl command uses \pdf@box
// which is not available in xelatex. We redefine \burl to use a simpler implementation.
func fixBblForXelatex(bblContent string) string {
	// Check if the .bbl file defines \burl
	if !strings.Contains(bblContent, `\burl`) {
		return bblContent
	}

	// Replace the \burl definition with a xelatex-compatible version
	// Original: \ifx \burl  \undefined \def \burl#1{\textsf{#1}}\fi
	// We need to ensure \burl is defined in a way that works with xelatex
	
	// First, check if there's an existing \burl definition
	burlDefPattern := regexp.MustCompile(`\\ifx\s*\\burl\s*\\undefined\s*\\def\s*\\burl#1\{[^}]*\}\\fi`)
	
	if burlDefPattern.MatchString(bblContent) {
		// Replace with a simpler definition that works with xelatex
		// We use \url if available, otherwise fall back to \textsf
		newBurlDef := `\ifx \burl \undefined
  \ifx \url \undefined
    \def \burl#1{\textsf{#1}}
  \else
    \def \burl#1{\url{#1}}
  \fi
\fi`
		bblContent = burlDefPattern.ReplaceAllString(bblContent, newBurlDef)
		logger.Debug("fixed \\burl definition for xelatex compatibility")
	}

	return bblContent
}

// fixMissingBibFile checks if the tex file references a .bib file via \bibliography{} command
// but the .bib file doesn't exist. If a .bbl file exists (common in arXiv submissions),
// it inlines the .bbl content to replace the \bibliography{} command.
// This handles cases where arXiv source only includes pre-compiled .bbl but tex references .bib.
// Returns the modified content and a boolean indicating if any fix was applied.
func fixMissingBibFile(texContent string, texDir string, texBaseName string) (string, bool) {
	// Check if the file already has thebibliography environment
	// If so, don't inline again to avoid duplicates
	if strings.Contains(texContent, `\begin{thebibliography}`) {
		logger.Debug("skipping fixMissingBibFile - file already has thebibliography environment")
		return texContent, false
	}

	// Pattern to match \bibliography{...} command and extract the bib file name(s)
	// Use multiline mode to match only non-commented lines
	// The pattern matches \bibliography{...} only if it's not preceded by % on the same line
	bibPattern := regexp.MustCompile(`(?m)^[^%\n]*\\bibliography\{([^}]+)\}`)
	matches := bibPattern.FindStringSubmatch(texContent)
	
	if len(matches) < 2 {
		// No uncommented \bibliography command found
		return texContent, false
	}
	
	// Check if the \bibliography command is in the preamble (before \begin{document})
	// If so, skip inlining to avoid putting thebibliography in the preamble
	beginDocIdx := strings.Index(texContent, `\begin{document}`)
	if beginDocIdx != -1 {
		bibIdx := strings.Index(texContent, matches[0])
		if bibIdx != -1 && bibIdx < beginDocIdx {
			logger.Debug("skipping fixMissingBibFile - \\bibliography is in preamble",
				logger.Int("bibIdx", bibIdx),
				logger.Int("beginDocIdx", beginDocIdx))
			return texContent, false
		}
	}
	
	// Extract bib file names (can be comma-separated)
	bibNames := strings.Split(matches[1], ",")
	allBibFilesExist := true
	
	for _, bibName := range bibNames {
		bibName = strings.TrimSpace(bibName)
		if bibName == "" {
			continue
		}
		
		// Check if .bib file exists
		bibPath := filepath.Join(texDir, bibName)
		if !strings.HasSuffix(bibPath, ".bib") {
			bibPath += ".bib"
		}
		
		if _, err := os.Stat(bibPath); os.IsNotExist(err) {
			allBibFilesExist = false
			logger.Debug("bib file not found", logger.String("bibPath", bibPath))
			break
		}
	}
	
	if allBibFilesExist {
		// All .bib files exist, no need to fix
		return texContent, false
	}
	
	// .bib file(s) missing, look for .bbl file
	// Try multiple possible .bbl file names:
	// 1. Same name as the tex file (most common)
	// 2. Same name as the first bib file referenced
	possibleBblNames := []string{
		texBaseName + ".bbl",
	}
	
	// Add bib file names as possible bbl names
	for _, bibName := range bibNames {
		bibName = strings.TrimSpace(bibName)
		if bibName != "" {
			possibleBblNames = append(possibleBblNames, bibName+".bbl")
		}
	}
	
	// Also scan directory for any .bbl file
	files, err := os.ReadDir(texDir)
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".bbl") {
				possibleBblNames = append(possibleBblNames, f.Name())
			}
		}
	}
	
	// Try to find and read a .bbl file
	var bblContent string
	var bblFound bool
	for _, bblName := range possibleBblNames {
		bblPath := filepath.Join(texDir, bblName)
		if content, err := os.ReadFile(bblPath); err == nil && len(content) > 0 {
			// Verify it's a valid .bbl file (should contain thebibliography environment)
			if strings.Contains(string(content), "\\begin{thebibliography}") {
				bblContent = string(content)
				bblFound = true
				logger.Info("found .bbl file to inline",
					logger.String("bblPath", bblPath),
					logger.Int("size", len(content)))
				break
			}
		}
	}
	
	if !bblFound {
		logger.Warn("no valid .bbl file found to replace missing .bib file",
			logger.String("texDir", texDir))
		return texContent, false
	}
	
	// Inline the .bbl content using the existing inlineBibliography function
	return inlineBibliography(texContent, bblContent)
}


// autoFixEncoding automatically fixes encoding issues in all .tex files in the directory
// This is integrated into the compilation process to ensure Chinese characters are properly detected
func (c *LaTeXCompiler) autoFixEncoding(mainTexPath string, texDir string) error {
	logger.Info("auto-fixing encoding issues", logger.String("texDir", texDir))

	// Create backup directory
	backupDir := filepath.Join(texDir, ".encoding_backups")
	workflow := editor.NewFixWorkflow(backupDir)
	encodingHandler := workflow.GetEncodingHandler()

	// Find all .tex files in the directory
	texFiles, err := findTexFilesInDir(texDir)
	if err != nil {
		return fmt.Errorf("failed to find tex files: %w", err)
	}

	fixedCount := 0
	for _, texFile := range texFiles {
		// Detect encoding
		encoding, err := encodingHandler.DetectEncoding(texFile)
		if err != nil {
			logger.Warn("failed to detect encoding", logger.Err(err), logger.String("file", texFile))
			continue
		}

		// Skip if already UTF-8 without BOM
		if encoding == "UTF-8" {
			continue
		}

		// Fix encoding
		logger.Debug("fixing encoding", logger.String("file", texFile), logger.String("encoding", encoding))
		if err := encodingHandler.EnsureUTF8(texFile); err != nil {
			logger.Warn("failed to fix encoding", logger.Err(err), logger.String("file", texFile))
			continue
		}

		fixedCount++
		logger.Info("fixed encoding", logger.String("file", texFile), logger.String("from", encoding))
	}

	if fixedCount > 0 {
		logger.Info("encoding auto-fix completed", logger.Int("fixedCount", fixedCount))
	}

	return nil
}

// findTexFilesInDir finds all .tex files in a directory (non-recursive)
func findTexFilesInDir(dir string) ([]string, error) {
	var texFiles []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check subdirectories like "Tex/"
			subDir := filepath.Join(dir, entry.Name())
			subEntries, err := os.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if !subEntry.IsDir() && filepath.Ext(subEntry.Name()) == ".tex" {
					texFiles = append(texFiles, filepath.Join(subDir, subEntry.Name()))
				}
			}
		} else if filepath.Ext(entry.Name()) == ".tex" {
			texFiles = append(texFiles, filepath.Join(dir, entry.Name()))
		}
	}

	return texFiles, nil
}

// autoFixSyntax automatically fixes TikZ and other syntax issues in all .tex files
func (c *LaTeXCompiler) autoFixSyntax(texDir string) error {
	logger.Info("auto-fixing syntax issues", logger.String("texDir", texDir))

	fixer := validator.NewSimpleFixer(texDir)

	// Find all .tex files
	texFiles, err := findTexFilesInDir(texDir)
	if err != nil {
		return fmt.Errorf("failed to find tex files: %w", err)
	}

	fixedCount := 0
	for _, texFile := range texFiles {
		// Try to fix the file
		result, err := fixer.SmartFix(texFile)
		if err != nil {
			logger.Warn("failed to fix file", logger.Err(err), logger.String("file", texFile))
			continue
		}

		if result != nil && result.Fixed {
			fixedCount++
			logger.Info("fixed syntax issues", 
				logger.String("file", texFile),
				logger.Int("fixCount", len(result.FixesApplied)))
		}
	}

	if fixedCount > 0 {
		logger.Info("syntax auto-fix completed", logger.Int("fixedFiles", fixedCount))
	}

	return nil
}

// ensureChineseSupport ensures the document has proper Chinese support for xelatex
// NOTE: This function is now disabled because automatically adding xeCJK can conflict
// with existing document configurations. Documents that need Chinese support should
// include it themselves.
func (c *LaTeXCompiler) ensureChineseSupport(texPath string, content string) {
	// Disabled: automatically adding xeCJK can cause compilation failures
	// when it conflicts with existing packages or document classes.
	// The document should include its own Chinese support if needed.
	logger.Debug("ensureChineseSupport is disabled to avoid conflicts")
	return
}
