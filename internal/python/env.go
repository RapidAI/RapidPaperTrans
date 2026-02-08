// Package python provides utilities for managing a local Python environment using uv.
// It automatically downloads uv and creates an isolated virtual environment,
// ensuring the application doesn't affect or get affected by the system Python.
package python

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// AppDataDirName is the directory name used in user's home directory
const AppDataDirName = ".RapidPaperTrans"

// Env manages a local Python virtual environment using uv.
// It provides automatic setup of uv and venv, package installation,
// and script execution capabilities.
type Env struct {
	BaseDir    string // Base directory for .tools and .venv
	ToolsDir   string // .tools directory path (contains uv)
	VenvDir    string // .venv directory path
	UvPath     string // Path to uv executable
	PythonPath string // Path to python in venv

	mu       sync.Mutex
	setupDone bool
}

// Config holds configuration for creating a new Env
type Config struct {
	BaseDir string // Base directory, defaults to user home directory
}

// getDefaultBaseDir returns the default base directory for Python environment.
// It uses ~/.RapidPaperTrans to avoid permission issues with program directory.
func getDefaultBaseDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, AppDataDirName), nil
}

// New creates a new Env for managing Python environment.
// If baseDir is empty, it uses ~/.RapidPaperTrans directory.
func New(cfg Config) (*Env, error) {
	baseDir := cfg.BaseDir
	if baseDir == "" {
		// Use user home directory to avoid permission issues
		var err error
		baseDir, err = getDefaultBaseDir()
		if err != nil {
			return nil, err
		}
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	toolsDir := filepath.Join(baseDir, ".tools")
	venvDir := filepath.Join(baseDir, ".venv")

	env := &Env{
		BaseDir:  baseDir,
		ToolsDir: toolsDir,
		VenvDir:  venvDir,
	}

	// Set platform-specific paths
	if runtime.GOOS == "windows" {
		env.UvPath = filepath.Join(toolsDir, "uv.exe")
		env.PythonPath = filepath.Join(venvDir, "Scripts", "python.exe")
	} else {
		env.UvPath = filepath.Join(toolsDir, "uv")
		env.PythonPath = filepath.Join(venvDir, "bin", "python")
	}

	return env, nil
}

// EnsureSetup ensures uv is installed and venv is created.
// This method is idempotent and thread-safe.
func (e *Env) EnsureSetup(progressFn func(message string)) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.setupDone {
		return nil
	}

	progress := func(msg string) {
		if progressFn != nil {
			progressFn(msg)
		}
	}

	// 1. Ensure uv is available
	progress("检查 uv 工具...")
	if err := e.ensureUv(progress); err != nil {
		return fmt.Errorf("failed to setup uv: %w", err)
	}

	// 2. Ensure venv exists
	progress("检查 Python 虚拟环境...")
	if err := e.ensureVenv(progress); err != nil {
		return fmt.Errorf("failed to setup venv: %w", err)
	}

	e.setupDone = true
	progress("Python 环境准备完成")
	return nil
}


// ensureUv downloads uv if not present
func (e *Env) ensureUv(progress func(string)) error {
	// Check if uv already exists and is executable
	if e.isUvInstalled() {
		progress("✓ uv 已安装")
		return nil
	}

	progress("正在下载 uv...")

	// Create .tools directory
	if err := os.MkdirAll(e.ToolsDir, 0755); err != nil {
		return fmt.Errorf("failed to create tools directory: %w", err)
	}

	// Determine download URL based on platform
	url := e.getUvDownloadURL()
	if url == "" {
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Download uv archive
	archivePath, err := e.downloadFile(url, e.ToolsDir)
	if err != nil {
		return fmt.Errorf("failed to download uv: %w", err)
	}
	defer os.Remove(archivePath)

	// Extract archive
	progress("正在解压 uv...")
	if err := e.extractUv(archivePath); err != nil {
		return fmt.Errorf("failed to extract uv: %w", err)
	}

	// Verify installation
	if !e.isUvInstalled() {
		return fmt.Errorf("uv installation verification failed")
	}

	progress("✓ uv 安装完成")
	return nil
}

// isUvInstalled checks if uv is installed and executable
func (e *Env) isUvInstalled() bool {
	if _, err := os.Stat(e.UvPath); err != nil {
		return false
	}

	// Try to run uv --version to verify it works
	cmd := exec.Command(e.UvPath, "--version")
	hideWindow(cmd)
	return cmd.Run() == nil
}

// getUvDownloadURL returns the download URL for uv based on the current platform
func (e *Env) getUvDownloadURL() string {
	// uv release URLs from https://github.com/astral-sh/uv/releases
	baseURL := "https://github.com/astral-sh/uv/releases/latest/download"

	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return baseURL + "/uv-x86_64-pc-windows-msvc.zip"
		}
		if runtime.GOARCH == "arm64" {
			return baseURL + "/uv-aarch64-pc-windows-msvc.zip"
		}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return baseURL + "/uv-aarch64-apple-darwin.tar.gz"
		}
		if runtime.GOARCH == "amd64" {
			return baseURL + "/uv-x86_64-apple-darwin.tar.gz"
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			return baseURL + "/uv-x86_64-unknown-linux-gnu.tar.gz"
		}
		if runtime.GOARCH == "arm64" {
			return baseURL + "/uv-aarch64-unknown-linux-gnu.tar.gz"
		}
	}
	return ""
}

// downloadFile downloads a file from URL to the specified directory
func (e *Env) downloadFile(url, destDir string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Determine filename from URL
	filename := filepath.Base(url)
	destPath := filepath.Join(destDir, filename)

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Copy content
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(destPath)
		return "", err
	}

	return destPath, nil
}

// extractUv extracts the uv archive based on file type
func (e *Env) extractUv(archivePath string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return e.extractZip(archivePath)
	}
	return e.extractTarGz(archivePath)
}

// extractZip extracts a zip archive
func (e *Env) extractZip(archivePath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	uvName := "uv"
	if runtime.GOOS == "windows" {
		uvName = "uv.exe"
	}

	for _, f := range r.File {
		// Look for uv executable
		if filepath.Base(f.Name) == uvName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.OpenFile(e.UvPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}

	return fmt.Errorf("uv executable not found in archive")
}

// extractTarGz extracts a tar.gz archive using system tar command
func (e *Env) extractTarGz(archivePath string) error {
	// Use system tar command
	cmd := exec.Command("tar", "-xzf", archivePath, "-C", e.ToolsDir, "--strip-components=1")
	hideWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extraction failed: %s: %w", string(output), err)
	}

	// Make executable
	return os.Chmod(e.UvPath, 0755)
}


// ensureVenv creates the virtual environment if it doesn't exist
func (e *Env) ensureVenv(progress func(string)) error {
	// Check if venv already exists and is valid
	if e.isVenvValid() {
		progress("✓ Python 虚拟环境已存在")
		return nil
	}

	progress("正在创建 Python 虚拟环境...")

	// Remove existing broken venv if any
	if _, err := os.Stat(e.VenvDir); err == nil {
		os.RemoveAll(e.VenvDir)
	}

	// Create venv using uv
	// uv will automatically download Python if needed
	cmd := exec.Command(e.UvPath, "venv", e.VenvDir, "--python", "3.11")
	hideWindow(cmd)
	cmd.Dir = e.BaseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create venv: %s: %w", string(output), err)
	}

	// Verify venv was created
	if !e.isVenvValid() {
		return fmt.Errorf("venv creation verification failed")
	}

	progress("✓ Python 虚拟环境创建完成")
	return nil
}

// isVenvValid checks if the venv exists and has a valid Python
func (e *Env) isVenvValid() bool {
	if _, err := os.Stat(e.PythonPath); err != nil {
		return false
	}

	// Try to run python --version to verify it works
	cmd := exec.Command(e.PythonPath, "--version")
	hideWindow(cmd)
	return cmd.Run() == nil
}

// InstallPackages installs Python packages using uv pip
func (e *Env) InstallPackages(packages []string, progress func(string)) error {
	if len(packages) == 0 {
		return nil
	}

	// Ensure environment is set up first
	if err := e.EnsureSetup(progress); err != nil {
		return err
	}

	if progress != nil {
		progress(fmt.Sprintf("正在安装 Python 包: %s", strings.Join(packages, ", ")))
	}

	// Use uv pip install with --python flag to target our venv
	args := append([]string{"pip", "install", "--python", e.PythonPath}, packages...)
	cmd := exec.Command(e.UvPath, args...)
	hideWindow(cmd)
	cmd.Dir = e.BaseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install packages: %s: %w", string(output), err)
	}

	if progress != nil {
		progress("✓ Python 包安装完成")
	}
	return nil
}

// EnsurePackages ensures the specified packages are installed.
// It checks if packages are already installed before installing.
func (e *Env) EnsurePackages(packages []string, progress func(string)) error {
	if len(packages) == 0 {
		return nil
	}

	// Ensure environment is set up first
	if err := e.EnsureSetup(progress); err != nil {
		return err
	}

	// Check which packages need to be installed
	var missing []string
	for _, pkg := range packages {
		if !e.isPackageInstalled(pkg) {
			missing = append(missing, pkg)
		}
	}

	if len(missing) == 0 {
		if progress != nil {
			progress("✓ 所有 Python 包已安装")
		}
		return nil
	}

	return e.InstallPackages(missing, progress)
}

// isPackageInstalled checks if a package is installed in the venv
func (e *Env) isPackageInstalled(pkg string) bool {
	// Extract package name (remove version specifiers)
	pkgName := pkg
	for _, sep := range []string{">=", "<=", "==", ">", "<", "~="} {
		if idx := strings.Index(pkg, sep); idx > 0 {
			pkgName = pkg[:idx]
			break
		}
	}

	// Try to import the package
	cmd := exec.Command(e.PythonPath, "-c", fmt.Sprintf("import %s", pkgName))
	hideWindow(cmd)
	return cmd.Run() == nil
}

// RunScript runs a Python script in the virtual environment
func (e *Env) RunScript(scriptPath string, args ...string) (string, error) {
	// Ensure environment is set up first
	if err := e.EnsureSetup(nil); err != nil {
		return "", err
	}

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command(e.PythonPath, cmdArgs...)
	hideWindow(cmd)
	cmd.Dir = e.BaseDir

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunScriptWithStdin runs a Python script with stdin input
func (e *Env) RunScriptWithStdin(scriptPath string, stdin io.Reader, args ...string) (string, error) {
	// Ensure environment is set up first
	if err := e.EnsureSetup(nil); err != nil {
		return "", err
	}

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command(e.PythonPath, cmdArgs...)
	hideWindow(cmd)
	cmd.Dir = e.BaseDir
	cmd.Stdin = stdin

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetPythonPath returns the path to the Python executable in the venv
func (e *Env) GetPythonPath() string {
	return e.PythonPath
}

// GetUvPath returns the path to the uv executable
func (e *Env) GetUvPath() string {
	return e.UvPath
}

// IsReady returns true if the environment is fully set up
func (e *Env) IsReady() bool {
	return e.isUvInstalled() && e.isVenvValid()
}

// Cleanup removes the virtual environment (but keeps uv)
func (e *Env) Cleanup() error {
	if e.VenvDir != "" {
		return os.RemoveAll(e.VenvDir)
	}
	return nil
}

// CleanupAll removes both uv and the virtual environment
func (e *Env) CleanupAll() error {
	var errs []error
	if e.VenvDir != "" {
		if err := os.RemoveAll(e.VenvDir); err != nil {
			errs = append(errs, err)
		}
	}
	if e.ToolsDir != "" {
		if err := os.RemoveAll(e.ToolsDir); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}
