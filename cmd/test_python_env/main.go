// Command test_python_env tests the automatic Python environment setup.
// It downloads uv, creates a virtual environment, and installs pymupdf.
package main

import (
	"fmt"
	"os"

	"latex-translator/internal/python"
)

func main() {
	fmt.Println("=== 测试 Python 环境自动安装 ===")
	fmt.Println()

	// Progress callback
	progress := func(msg string) {
		fmt.Printf("  %s\n", msg)
	}

	// Ensure global environment is set up
	fmt.Println("1. 设置 Python 环境...")
	if err := python.EnsureGlobalEnv(progress); err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	// Get environment info
	env, err := python.GetGlobalEnv()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("2. 环境信息:")
	fmt.Printf("   Base Dir:    %s\n", env.BaseDir)
	fmt.Printf("   Tools Dir:   %s\n", env.ToolsDir)
	fmt.Printf("   Venv Dir:    %s\n", env.VenvDir)
	fmt.Printf("   UV Path:     %s\n", env.UvPath)
	fmt.Printf("   Python Path: %s\n", env.PythonPath)
	fmt.Printf("   Is Ready:    %v\n", env.IsReady())
	fmt.Println()

	// Test running a simple Python script
	fmt.Println("3. 测试 Python 执行...")
	output, err := python.RunScript("-c", "import sys; print(f'Python {sys.version}')")
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   %s", output)
	fmt.Println()

	// Test pymupdf import
	fmt.Println("4. 测试 PyMuPDF 导入...")
	output, err = python.RunScript("-c", "import fitz; print(f'PyMuPDF version: {fitz.version}')")
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   %s", output)
	fmt.Println()

	fmt.Println("=== 测试完成 ===")
}
