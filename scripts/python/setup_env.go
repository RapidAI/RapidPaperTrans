// Package python provides utilities for managing a local Python environment
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PythonEnv manages a local Python virtual environment
type PythonEnv struct {
	BaseDir    string // Project root directory
	VenvDir    string // .venv directory path
	UvPath     string // Path to uv executable
	PythonPath string // Path to python in venv
}

// NewPythonEnv creates a new PythonEnv for the given project directory
func NewPythonEnv(baseDir string) *PythonEnv {
	venvDir := filepath.Join(baseDir, ".venv")
	
	env := &PythonEnv{
		BaseDir: baseDir,
		VenvDir: venvDir,
	}
	
	// Set platform-specific paths
	if runtime.GOOS == "windows" {
		env.UvPath = filepath.Join(baseDir, ".tools", "uv.exe")
		env.PythonPath = filepath.Join(venvDir, "Scripts", "python.exe")
	} else {
		env.UvPath = filepath.Join(baseDir, ".tools", "uv")
		env.PythonPath = filepath.Join(venvDir, "bin", "python")
	}
	
	return env
}

// EnsureSetup ensures uv is installed and venv is created
func (e *PythonEnv) EnsureSetup() error {
	// 1. Ensure uv is available
	if err := e.ensureUv(); err != nil {
		return fmt.Errorf("failed to setup uv: %w", err)
	}
	
	// 2. Ensure venv exists
	if err := e.ensureVenv(); err != nil {
		return fmt.Errorf("failed to setup venv: %w", err)
	}
	
	return nil
}

// ensureUv downloads uv if not present
func (e *PythonEnv) ensureUv() error {
	// Check if uv already exists
	if _, err := os.Stat(e.UvPath); err == nil {
		fmt.Println("✓ uv already installed")
		return nil
	}
	
	fmt.Println("Downloading uv...")
	
	// Create .tools directory
	toolsDir := filepath.Dir(e.UvPath)
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		return err
	}
	
	// Download uv binary
	var url string
	if runtime.GOOS == "windows" {
		url = "https://github.com/astral-sh/uv/releases/latest/download/uv-x86_64-pc-windows-msvc.zip"
	} else if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			url = "https://github.com/astral-sh/uv/releases/latest/download/uv-aarch64-apple-darwin.tar.gz"
		} else {
			url = "https://github.com/astral-sh/uv/releases/latest/download/uv-x86_64-apple-darwin.tar.gz"
		}
	} else {
		url = "https://github.com/astral-sh/uv/releases/latest/download/uv-x86_64-unknown-linux-gnu.tar.gz"
	}
	
	// Download to temp file
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download uv: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download uv: HTTP %d", resp.StatusCode)
	}
	
	// Save and extract
	archivePath := filepath.Join(toolsDir, "uv-archive")
	if runtime.GOOS == "windows" {
		archivePath += ".zip"
	} else {
		archivePath += ".tar.gz"
	}
	
	outFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	
	_, err = io.Copy(outFile, resp.Body)
	outFile.Close()
	if err != nil {
		return err
	}
	
	// Extract archive
	if runtime.GOOS == "windows" {
		// Use PowerShell to extract zip
		cmd := exec.Command("powershell", "-Command",
			fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", archivePath, toolsDir))
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to extract: %s: %w", string(out), err)
		}
		// Move uv.exe to correct location
		extractedUv := filepath.Join(toolsDir, "uv.exe")
		if _, err := os.Stat(extractedUv); err != nil {
			// Try to find it in subdirectory
			matches, _ := filepath.Glob(filepath.Join(toolsDir, "*", "uv.exe"))
			if len(matches) > 0 {
				os.Rename(matches[0], e.UvPath)
			}
		}
	} else {
		cmd := exec.Command("tar", "-xzf", archivePath, "-C", toolsDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to extract: %s: %w", string(out), err)
		}
	}
	
	// Cleanup archive
	os.Remove(archivePath)
	
	// Make executable on Unix
	if runtime.GOOS != "windows" {
		os.Chmod(e.UvPath, 0755)
	}
	
	fmt.Println("✓ uv installed")
	return nil
}

// ensureVenv creates the virtual environment if it doesn't exist
func (e *PythonEnv) ensureVenv() error {
	// Check if venv already exists
	if _, err := os.Stat(e.PythonPath); err == nil {
		fmt.Println("✓ Python venv already exists")
		return nil
	}
	
	fmt.Println("Creating Python virtual environment...")
	
	// Create venv using uv
	cmd := exec.Command(e.UvPath, "venv", e.VenvDir, "--python", "3.11")
	cmd.Dir = e.BaseDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create venv: %w", err)
	}
	
	fmt.Println("✓ Python venv created")
	return nil
}

// InstallPackages installs the required Python packages
func (e *PythonEnv) InstallPackages(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	
	fmt.Printf("Installing packages: %s\n", strings.Join(packages, ", "))
	
	args := append([]string{"pip", "install", "--python", e.PythonPath}, packages...)
	cmd := exec.Command(e.UvPath, args...)
	cmd.Dir = e.BaseDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install packages: %w", err)
	}
	
	fmt.Println("✓ Packages installed")
	return nil
}

// RunScript runs a Python script in the virtual environment
func (e *PythonEnv) RunScript(scriptPath string, args ...string) error {
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command(e.PythonPath, cmdArgs...)
	cmd.Dir = e.BaseDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	
	return cmd.Run()
}

// GetPythonPath returns the path to the Python executable
func (e *PythonEnv) GetPythonPath() string {
	return e.PythonPath
}

func main() {
	// Get current directory
	baseDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	env := NewPythonEnv(baseDir)
	
	// Setup environment
	if err := env.EnsureSetup(); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up Python environment: %v\n", err)
		os.Exit(1)
	}
	
	// Install required packages for LaTeX translation
	packages := []string{
		"openai",      // For API calls
		"tiktoken",    // For token counting
	}
	
	if err := env.InstallPackages(packages); err != nil {
		fmt.Fprintf(os.Stderr, "Error installing packages: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("\n✓ Python environment ready!")
	fmt.Printf("  Python: %s\n", env.PythonPath)
}
