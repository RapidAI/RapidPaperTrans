# LaTeX Editor Tools

这个包提供了一套完整的 LaTeX 文件编辑工具，实现了**编辑-修复-验证**工作流程。

## 功能特性

### 1. 行级编辑器 (Line Editor)
精确的行级文件编辑功能：
- 读取指定行范围
- 替换单行或多行
- 插入新行
- 删除行
- 搜索包含特定文本的行

### 2. 编码处理器 (Encoding Handler)
自动检测和转换文件编码：
- 检测编码（UTF-8, UTF-8-BOM, GBK, UTF-16LE, UTF-16BE）
- 转换编码
- 移除 BOM 标记
- 确保文件为 UTF-8 编码
- 修复编码问题

### 3. LaTeX 验证器 (LaTeX Validator)
验证 LaTeX 文件的正确性：
- 检查括号匹配（`{}`, `[]`, `()`）
- 检查环境闭合（`\begin{...}` 和 `\end{...}`）
- 检测常见错误和拼写错误
- 检测编码问题
- 生成详细的验证报告

### 4. 备份管理器 (Backup Manager)
安全的文件备份和恢复：
- 自动创建带时间戳的备份
- 恢复备份
- 列出所有备份
- 清理旧备份

### 5. 修复工作流 (Fix Workflow)
完整的编辑-修复-验证工作流程：
- 自动修复常见问题
- 编码问题修复
- 验证修复结果
- 失败时自动回滚

## 使用示例

### 基本用法

```go
package main

import (
    "fmt"
    "latex-translator/internal/editor"
)

func main() {
    // 创建修复工作流
    workflow := editor.NewFixWorkflow(".backups")
    
    // 自动修复文件
    result, err := workflow.AutoFix("main.tex")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    // 打印结果
    fmt.Println(editor.FormatFixResult(result))
}
```

### 行级编辑

```go
backupMgr := editor.NewBackupManager(".backups")
lineEditor := editor.NewLineEditor(backupMgr)

// 读取第 10-20 行
lines, err := lineEditor.ReadLines("main.tex", 10, 20)

// 替换第 15 行
err = lineEditor.ReplaceLine("main.tex", 15, "\\section{新章节}")

// 插入新行
err = lineEditor.InsertLine("main.tex", 16, "这是新插入的内容")

// 删除行
err = lineEditor.DeleteLine("main.tex", 17)
```

### 编码处理

```go
backupMgr := editor.NewBackupManager(".backups")
encodingHandler := editor.NewEncodingHandler(backupMgr)

// 检测编码
encoding, err := encodingHandler.DetectEncoding("main.tex")
fmt.Printf("Encoding: %s\n", encoding)

// 确保文件为 UTF-8
err = encodingHandler.EnsureUTF8("main.tex")

// 转换编码
err = encodingHandler.ConvertEncoding("main.tex", "GBK", "UTF-8")

// 移除 BOM
err = encodingHandler.RemoveBOM("main.tex")
```

### LaTeX 验证

```go
validator := editor.NewLaTeXValidator()

// 验证文件
result, err := validator.ValidateLaTeX("main.tex")
if !result.Valid {
    for _, err := range result.Errors {
        fmt.Printf("Error at line %d: %s\n", err.Line, err.Message)
    }
}

// 生成验证报告
report, err := validator.ValidateAndReport("main.tex")
fmt.Println(report)
```

### 安全编辑

```go
workflow := editor.NewFixWorkflow(".backups")

// 使用安全编辑模式（自动备份和验证）
err := workflow.SafeEdit("main.tex", func() error {
    // 执行编辑操作
    lineEditor := workflow.GetLineEditor()
    return lineEditor.ReplaceLine("main.tex", 10, "新内容")
})
```

## 命令行工具

### latex-edit

完整的命令行工具，提供所有编辑功能：

```bash
# 编码检测
latex-edit encoding detect main.tex

# 确保 UTF-8 编码
latex-edit encoding ensure-utf8 main.tex

# 验证文件
latex-edit validate main.tex

# 自动修复
latex-edit fix --auto main.tex

# 读取行
latex-edit lines read main.tex 10 20

# 替换行
latex-edit lines replace main.tex 15 "新内容"

# 创建备份
latex-edit backup create main.tex

# 列出备份
latex-edit backup list main.tex
```

### fix_2501_encoding

专门用于修复 arXiv 2501.17161 编码问题的工具：

```bash
# 使用默认路径
go run cmd/fix_2501_encoding/main.go

# 指定自定义路径
go run cmd/fix_2501_encoding/main.go "C:\path\to\latex\dir"
```

这个工具会：
1. 修复所有 .tex 文件的编码问题
2. 验证 main.tex
3. 检查中文字符是否可以被正确检测
4. 提供下一步操作建议

## 工作流程

### 编辑-修复-验证模式

```
1. 创建备份
   ↓
2. 检测并修复编码问题
   ↓
3. 验证 LaTeX 语法
   ↓
4. 应用自动修复
   ↓
5. 再次验证
   ↓
6. 成功 → 完成
   失败 → 恢复备份
```

### 使用工作流

```go
workflow := editor.NewFixWorkflow(".backups")

// 方式 1: 完全自动修复
result, err := workflow.AutoFix("main.tex")

// 方式 2: 仅修复编码
result, err := workflow.FixEncodingOnly("main.tex")

// 方式 3: 验证后修复
result, err := workflow.ValidateAndFix("main.tex")

// 方式 4: 批量修复
files := []string{"main.tex", "chapter1.tex", "chapter2.tex"}
results := workflow.BatchFix(files)
```

## 解决 arXiv 2501.17161 问题

这个工具集专门设计用于解决 arXiv 2501.17161 翻译后只显示 4 页的问题。

### 问题原因

1. **编码问题**: 翻译后的文件包含中文，但编码不正确（UTF-8 BOM 或其他编码）
2. **中文检测失败**: 由于编码问题，`ContainsChinese()` 无法检测到中文字符
3. **编译器选择错误**: 程序使用 pdflatex 而不是 xelatex，导致 Unicode 错误

### 解决方案

```bash
# 运行修复工具
go run cmd/fix_2501_encoding/main.go

# 工具会自动：
# 1. 检测所有 .tex 文件的编码
# 2. 转换为 UTF-8（无 BOM）
# 3. 验证文件语法
# 4. 检查中文字符是否可检测

# 然后编译
cd C:\Users\ma139\latex-translator-results\2501.17161\latex
xelatex -interaction=nonstopmode main.tex
```

### 预期结果

- ✓ 所有文件转换为 UTF-8（无 BOM）
- ✓ 中文字符可以被正确检测
- ✓ 编译器自动选择 xelatex
- ✓ 生成完整的 21 页 PDF

## API 文档

### LineEditor

```go
type LineEditor struct {
    backupMgr *BackupManager
}

func NewLineEditor(backupMgr *BackupManager) *LineEditor
func (e *LineEditor) ReadLines(path string, start, end int) ([]string, error)
func (e *LineEditor) ReplaceLine(path string, lineNum int, newContent string) error
func (e *LineEditor) InsertLine(path string, lineNum int, content string) error
func (e *LineEditor) DeleteLine(path string, lineNum int) error
func (e *LineEditor) ReplaceLines(path string, start, end int, newContent []string) error
func (e *LineEditor) CountLines(path string) (int, error)
func (e *LineEditor) SearchLines(path string, searchText string) ([]int, error)
```

### EncodingHandler

```go
type EncodingHandler struct {
    backupMgr *BackupManager
}

func NewEncodingHandler(backupMgr *BackupManager) *EncodingHandler
func (h *EncodingHandler) DetectEncoding(path string) (string, error)
func (h *EncodingHandler) ConvertEncoding(path string, from, to string) error
func (h *EncodingHandler) RemoveBOM(path string) error
func (h *EncodingHandler) EnsureUTF8(path string) error
func (h *EncodingHandler) ReadFileWithEncoding(path string) (string, error)
func (h *EncodingHandler) WriteFileWithEncoding(path string, content string, encoding string) error
func (h *EncodingHandler) FixEncodingIssues(path string) error
func (h *EncodingHandler) GetEncodingInfo(path string) (*EncodingInfo, error)
```

### LaTeXValidator

```go
type LaTeXValidator struct{}

func NewLaTeXValidator() *LaTeXValidator
func (v *LaTeXValidator) ValidateLaTeX(path string) (*ValidationResult, error)
func (v *LaTeXValidator) ValidateAndReport(path string) (string, error)
func (v *LaTeXValidator) QuickCheck(path string) (bool, error)
func (v *LaTeXValidator) GetErrorSummary(result *ValidationResult) string
```

### FixWorkflow

```go
type FixWorkflow struct {
    lineEditor      *LineEditor
    encodingHandler *EncodingHandler
    validator       *LaTeXValidator
    backupMgr       *BackupManager
}

func NewFixWorkflow(backupDir string) *FixWorkflow
func (w *FixWorkflow) AutoFix(path string) (*FixResult, error)
func (w *FixWorkflow) ValidateAndFix(path string) (*FixResult, error)
func (w *FixWorkflow) FixEncodingOnly(path string) (*FixResult, error)
func (w *FixWorkflow) SafeEdit(path string, editFunc SafeEditFunc) error
func (w *FixWorkflow) BatchFix(paths []string) map[string]*FixResult
```

## 测试

运行测试：

```bash
# 运行所有测试
go test ./internal/editor/...

# 运行特定测试
go test ./internal/editor/ -run TestLineEditor_ReadLines

# 运行测试并显示详细输出
go test -v ./internal/editor/...
```

## 性能考虑

- **小文件（< 1MB）**: 使用内存操作，速度快
- **大文件（> 1MB）**: 使用流式处理，避免内存问题
- **批量操作**: 支持并发处理多个文件

## 安全性

- **自动备份**: 所有编辑操作前自动创建备份
- **原子操作**: 编辑失败时自动回滚
- **验证机制**: 编辑后自动验证，确保文件完整性
- **权限保持**: 保持原文件的权限设置

## 限制

- 仅支持文本文件（不支持二进制文件）
- 编码检测基于启发式算法，可能不是 100% 准确
- 大文件（> 100MB）可能需要较长处理时间

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License
