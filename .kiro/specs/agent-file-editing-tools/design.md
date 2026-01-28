# Agent 文件编辑工具设计

## 1. 架构概述

```
┌─────────────────────────────────────────────────────────┐
│                    Agent Interface                       │
│  (通过命令行工具或直接 API 调用)                          │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│              File Editor Orchestrator                    │
│  - 协调各个编辑操作                                       │
│  - 管理备份和回滚                                         │
│  - 提供统一的错误处理                                     │
└─────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│ Line Editor  │   │   Encoding   │   │  Validator   │
│              │   │   Handler    │   │              │
│ - ReadLines  │   │ - Detect     │   │ - LaTeX      │
│ - Replace    │   │ - Convert    │   │ - Encoding   │
│ - Insert     │   │ - RemoveBOM  │   │ - Integrity  │
│ - Delete     │   │              │   │              │
└──────────────┘   └──────────────┘   └──────────────┘
        │                   │                   │
        └───────────────────┼───────────────────┘
                            ▼
                   ┌──────────────┐
                   │ Backup Mgr   │
                   │              │
                   │ - Create     │
                   │ - Restore    │
                   │ - Cleanup    │
                   └──────────────┘
```

## 2. 核心组件设计

### 2.1 Line Editor

```go
package editor

import (
    "bufio"
    "io"
    "os"
)

// LineEditor 提供行级文件编辑功能
type LineEditor struct {
    backupMgr *BackupManager
}

// ReadLines 读取指定行范围
// start: 起始行号（1-based）
// end: 结束行号（-1 表示到文件末尾）
func (e *LineEditor) ReadLines(path string, start, end int) ([]string, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    var lines []string
    scanner := bufio.NewScanner(file)
    lineNum := 1
    
    for scanner.Scan() {
        if lineNum >= start && (end == -1 || lineNum <= end) {
            lines = append(lines, scanner.Text())
        }
        if end != -1 && lineNum > end {
            break
        }
        lineNum++
    }
    
    return lines, scanner.Err()
}

// ReplaceLine 替换指定行
func (e *LineEditor) ReplaceLine(path string, lineNum int, newContent string) error {
    // 1. 创建备份
    backup, err := e.backupMgr.CreateBackup(path)
    if err != nil {
        return err
    }
    
    // 2. 读取所有行
    lines, err := e.readAllLines(path)
    if err != nil {
        return err
    }
    
    // 3. 替换指定行
    if lineNum < 1 || lineNum > len(lines) {
        return fmt.Errorf("line number %d out of range", lineNum)
    }
    lines[lineNum-1] = newContent
    
    // 4. 写回文件
    if err := e.writeAllLines(path, lines); err != nil {
        // 失败时恢复备份
        e.backupMgr.Restore(backup)
        return err
    }
    
    return nil
}

// InsertLine 在指定位置插入新行
func (e *LineEditor) InsertLine(path string, lineNum int, content string) error {
    backup, err := e.backupMgr.CreateBackup(path)
    if err != nil {
        return err
    }
    
    lines, err := e.readAllLines(path)
    if err != nil {
        return err
    }
    
    // 插入新行
    if lineNum < 1 || lineNum > len(lines)+1 {
        return fmt.Errorf("line number %d out of range", lineNum)
    }
    
    newLines := make([]string, 0, len(lines)+1)
    newLines = append(newLines, lines[:lineNum-1]...)
    newLines = append(newLines, content)
    newLines = append(newLines, lines[lineNum-1:]...)
    
    if err := e.writeAllLines(path, newLines); err != nil {
        e.backupMgr.Restore(backup)
        return err
    }
    
    return nil
}

// DeleteLine 删除指定行
func (e *LineEditor) DeleteLine(path string, lineNum int) error {
    backup, err := e.backupMgr.CreateBackup(path)
    if err != nil {
        return err
    }
    
    lines, err := e.readAllLines(path)
    if err != nil {
        return err
    }
    
    if lineNum < 1 || lineNum > len(lines) {
        return fmt.Errorf("line number %d out of range", lineNum)
    }
    
    newLines := append(lines[:lineNum-1], lines[lineNum:]...)
    
    if err := e.writeAllLines(path, newLines); err != nil {
        e.backupMgr.Restore(backup)
        return err
    }
    
    return nil
}

// ReplaceLines 替换行范围
func (e *LineEditor) ReplaceLines(path string, start, end int, newContent []string) error {
    backup, err := e.backupMgr.CreateBackup(path)
    if err != nil {
        return err
    }
    
    lines, err := e.readAllLines(path)
    if err != nil {
        return err
    }
    
    if start < 1 || end > len(lines) || start > end {
        return fmt.Errorf("invalid line range: %d-%d", start, end)
    }
    
    newLines := make([]string, 0, len(lines)-（end-start+1)+len(newContent))
    newLines = append(newLines, lines[:start-1]...)
    newLines = append(newLines, newContent...)
    newLines = append(newLines, lines[end:]...)
    
    if err := e.writeAllLines(path, newLines); err != nil {
        e.backupMgr.Restore(backup)
        return err
    }
    
    return nil
}
```

### 2.2 Encoding Handler

```go
package editor

import (
    "bytes"
    "io/ioutil"
    "unicode/utf8"
    
    "golang.org/x/text/encoding"
    "golang.org/x/text/encoding/simplifiedchinese"
    "golang.org/x/text/encoding/unicode"
)

type EncodingHandler struct {
    backupMgr *BackupManager
}

// DetectEncoding 检测文件编码
func (h *EncodingHandler) DetectEncoding(path string) (string, error) {
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return "", err
    }
    
    // 检查 BOM
    if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
        return "UTF-8-BOM", nil
    }
    if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
        return "UTF-16LE", nil
    }
    if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
        return "UTF-16BE", nil
    }
    
    // 检查是否为有效的 UTF-8
    if utf8.Valid(data) {
        return "UTF-8", nil
    }
    
    // 尝试 GBK
    if h.isValidGBK(data) {
        return "GBK", nil
    }
    
    return "UNKNOWN", nil
}

// ConvertEncoding 转换文件编码
func (h *EncodingHandler) ConvertEncoding(path string, from, to string) error {
    backup, err := h.backupMgr.CreateBackup(path)
    if err != nil {
        return err
    }
    
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return err
    }
    
    // 解码
    var decoded []byte
    switch from {
    case "GBK":
        decoder := simplifiedchinese.GBK.NewDecoder()
        decoded, err = decoder.Bytes(data)
    case "UTF-8-BOM":
        decoded = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
    default:
        decoded = data
    }
    
    if err != nil {
        h.backupMgr.Restore(backup)
        return err
    }
    
    // 编码
    var encoded []byte
    switch to {
    case "UTF-8":
        encoded = decoded
    case "UTF-8-BOM":
        encoded = append([]byte{0xEF, 0xBB, 0xBF}, decoded...)
    case "GBK":
        encoder := simplifiedchinese.GBK.NewEncoder()
        encoded, err = encoder.Bytes(decoded)
    }
    
    if err != nil {
        h.backupMgr.Restore(backup)
        return err
    }
    
    // 写回文件
    if err := ioutil.WriteFile(path, encoded, 0644); err != nil {
        h.backupMgr.Restore(backup)
        return err
    }
    
    return nil
}

// RemoveBOM 移除 BOM 标记
func (h *EncodingHandler) RemoveBOM(path string) error {
    backup, err := h.backupMgr.CreateBackup(path)
    if err != nil {
        return err
    }
    
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return err
    }
    
    // 移除 UTF-8 BOM
    data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
    
    if err := ioutil.WriteFile(path, data, 0644); err != nil {
        h.backupMgr.Restore(backup)
        return err
    }
    
    return nil
}

// EnsureUTF8 确保文件是 UTF-8 编码（无 BOM）
func (h *EncodingHandler) EnsureUTF8(path string) error {
    encoding, err := h.DetectEncoding(path)
    if err != nil {
        return err
    }
    
    if encoding == "UTF-8" {
        return nil // 已经是 UTF-8
    }
    
    return h.ConvertEncoding(path, encoding, "UTF-8")
}
```

### 2.3 LaTeX Validator

```go
package editor

import (
    "regexp"
    "strings"
)

type ValidationResult struct {
    Valid    bool
    Errors   []ValidationError
    Warnings []ValidationWarning
}

type ValidationError struct {
    Line    int
    Column  int
    Message string
    Type    string // "syntax", "encoding", "structure"
}

type LaTeXValidator struct{}

// ValidateLaTeX 验证 LaTeX 文件
func (v *LaTeXValidator) ValidateLaTeX(path string) (*ValidationResult, error) {
    content, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    result := &ValidationResult{Valid: true}
    
    // 检查括号匹配
    if err := v.checkBraceBalance(string(content), result); err != nil {
        result.Valid = false
    }
    
    // 检查环境闭合
    if err := v.checkEnvironments(string(content), result); err != nil {
        result.Valid = false
    }
    
    // 检查常见错误
    v.checkCommonErrors(string(content), result)
    
    return result, nil
}

// checkBraceBalance 检查括号平衡
func (v *LaTeXValidator) checkBraceBalance(content string, result *ValidationResult) error {
    openBrace := strings.Count(content, "{")
    closeBrace := strings.Count(content, "}")
    
    if openBrace != closeBrace {
        result.Errors = append(result.Errors, ValidationError{
            Message: fmt.Sprintf("Unbalanced braces: %d open, %d close", openBrace, closeBrace),
            Type:    "syntax",
        })
        return fmt.Errorf("unbalanced braces")
    }
    
    return nil
}

// checkEnvironments 检查环境闭合
func (v *LaTeXValidator) checkEnvironments(content string, result *ValidationResult) error {
    envPattern := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)
    matches := envPattern.FindAllStringSubmatch(content, -1)
    
    envStack := make([]string, 0)
    
    for _, match := range matches {
        cmd := match[1]  // "begin" or "end"
        env := match[2]  // environment name
        
        if cmd == "begin" {
            envStack = append(envStack, env)
        } else if cmd == "end" {
            if len(envStack) == 0 {
                result.Errors = append(result.Errors, ValidationError{
                    Message: fmt.Sprintf("Unexpected \\end{%s} without matching \\begin", env),
                    Type:    "structure",
                })
                return fmt.Errorf("unmatched end")
            }
            
            last := envStack[len(envStack)-1]
            if last != env {
                result.Errors = append(result.Errors, ValidationError{
                    Message: fmt.Sprintf("Mismatched environment: \\begin{%s} ... \\end{%s}", last, env),
                    Type:    "structure",
                })
                return fmt.Errorf("mismatched environment")
            }
            
            envStack = envStack[:len(envStack)-1]
        }
    }
    
    if len(envStack) > 0 {
        result.Errors = append(result.Errors, ValidationError{
            Message: fmt.Sprintf("Unclosed environments: %v", envStack),
            Type:    "structure",
        })
        return fmt.Errorf("unclosed environments")
    }
    
    return nil
}
```

### 2.4 Backup Manager

```go
package editor

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "time"
)

type BackupManager struct {
    backupDir string
}

// CreateBackup 创建文件备份
func (m *BackupManager) CreateBackup(path string) (string, error) {
    // 生成备份文件名
    timestamp := time.Now().Format("20060102_150405")
    basename := filepath.Base(path)
    backupName := fmt.Sprintf("%s.backup_%s", basename, timestamp)
    backupPath := filepath.Join(m.backupDir, backupName)
    
    // 确保备份目录存在
    if err := os.MkdirAll(m.backupDir, 0755); err != nil {
        return "", err
    }
    
    // 复制文件
    src, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer src.Close()
    
    dst, err := os.Create(backupPath)
    if err != nil {
        return "", err
    }
    defer dst.Close()
    
    if _, err := io.Copy(dst, src); err != nil {
        return "", err
    }
    
    return backupPath, nil
}

// Restore 恢复备份
func (m *BackupManager) Restore(backupPath string) error {
    // 从备份文件名推断原始文件路径
    // 这需要在创建备份时记录映射关系
    // 简化实现：假设调用者提供原始路径
    return fmt.Errorf("not implemented")
}
```

## 3. 命令行工具设计

```bash
# 行编辑
latex-edit lines read <file> <start> <end>
latex-edit lines replace <file> <line> <content>
latex-edit lines insert <file> <line> <content>
latex-edit lines delete <file> <line>

# 编码处理
latex-edit encoding detect <file>
latex-edit encoding convert <file> --from=GBK --to=UTF-8
latex-edit encoding remove-bom <file>
latex-edit encoding ensure-utf8 <file>

# 验证
latex-edit validate <file>
latex-edit validate --check=syntax,encoding,structure <file>

# 备份管理
latex-edit backup create <file>
latex-edit backup restore <backup-file>
latex-edit backup list <file>
latex-edit backup cleanup <file>

# 组合操作
latex-edit fix <file> --auto  # 自动检测并修复常见问题
```

## 4. 集成到现有系统

### 4.1 在 Compiler 中使用

```go
// internal/compiler/compiler.go

func (c *LaTeXCompiler) Compile(texPath string, outputDir string) (*types.CompileResult, error) {
    // 1. 编码检查和修复
    encodingHandler := editor.NewEncodingHandler(backupMgr)
    if err := encodingHandler.EnsureUTF8(texPath); err != nil {
        logger.Warn("failed to ensure UTF-8 encoding", logger.Err(err))
    }
    
    // 2. 语法验证
    validator := editor.NewLaTeXValidator()
    validationResult, err := validator.ValidateLaTeX(texPath)
    if err != nil || !validationResult.Valid {
        // 尝试自动修复
        if err := c.autoFix(texPath, validationResult); err != nil {
            return nil, err
        }
    }
    
    // 3. 继续编译...
}
```

### 4.2 在 Validator 中使用

```go
// internal/validator/validator.go

func (v *Validator) ValidateAndFix(texPath string) error {
    lineEditor := editor.NewLineEditor(backupMgr)
    validator := editor.NewLaTeXValidator()
    
    // 验证
    result, err := validator.ValidateLaTeX(texPath)
    if err != nil {
        return err
    }
    
    // 修复错误
    for _, err := range result.Errors {
        if err.Type == "structure" {
            // 使用 lineEditor 修复
            if err := v.fixStructureError(lineEditor, texPath, err); err != nil {
                return err
            }
        }
    }
    
    return nil
}
```

## 5. 错误处理策略

### 5.1 分层错误处理

```
Level 1: 自动修复（无需用户干预）
  - 编码转换
  - BOM 移除
  - 简单的括号匹配

Level 2: 半自动修复（需要确认）
  - 环境闭合
  - 复杂的语法错误

Level 3: 手动修复（提供建议）
  - 语义错误
  - 复杂的结构问题
```

### 5.2 回滚机制

```go
type EditSession struct {
    backups []string
    operations []Operation
}

func (s *EditSession) Commit() error {
    // 所有操作成功，清理备份
    for _, backup := range s.backups {
        os.Remove(backup)
    }
    return nil
}

func (s *EditSession) Rollback() error {
    // 恢复所有备份
    for _, backup := range s.backups {
        if err := restoreBackup(backup); err != nil {
            return err
        }
    }
    return nil
}
```

## 6. 性能优化

### 6.1 大文件处理

```go
// 使用流式处理避免内存问题
func (e *LineEditor) ReplaceLineStreaming(path string, lineNum int, newContent string) error {
    tmpFile, err := ioutil.TempFile("", "edit-*")
    if err != nil {
        return err
    }
    defer os.Remove(tmpFile.Name())
    
    src, err := os.Open(path)
    if err != nil {
        return err
    }
    defer src.Close()
    
    scanner := bufio.NewScanner(src)
    writer := bufio.NewWriter(tmpFile)
    
    currentLine := 1
    for scanner.Scan() {
        if currentLine == lineNum {
            writer.WriteString(newContent + "\n")
        } else {
            writer.WriteString(scanner.Text() + "\n")
        }
        currentLine++
    }
    
    writer.Flush()
    tmpFile.Close()
    
    // 原子性替换
    return os.Rename(tmpFile.Name(), path)
}
```

### 6.2 批量操作优化

```go
// 批量编辑使用单次文件读写
func (e *LineEditor) BatchEdit(path string, edits []Edit) error {
    lines, err := e.readAllLines(path)
    if err != nil {
        return err
    }
    
    // 按行号排序编辑操作
    sort.Slice(edits, func(i, j int) bool {
        return edits[i].LineNum < edits[j].LineNum
    })
    
    // 应用所有编辑
    for _, edit := range edits {
        lines = edit.Apply(lines)
    }
    
    return e.writeAllLines(path, lines)
}
```

## 7. 测试策略

### 7.1 单元测试

```go
func TestLineEditor_ReplaceLine(t *testing.T) {
    tests := []struct {
        name        string
        input       []string
        lineNum     int
        newContent  string
        expected    []string
        expectError bool
    }{
        {
            name:       "replace middle line",
            input:      []string{"line1", "line2", "line3"},
            lineNum:    2,
            newContent: "new line2",
            expected:   []string{"line1", "new line2", "line3"},
        },
        // 更多测试用例...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 测试实现...
        })
    }
}
```

### 7.2 集成测试

```go
func TestFullEditWorkflow(t *testing.T) {
    // 1. 创建测试文件
    // 2. 执行编辑操作
    // 3. 验证结果
    // 4. 测试回滚
}
```

## 8. 文档和示例

### 8.1 API 文档

为每个公共函数提供详细的 godoc 注释。

### 8.2 使用示例

```go
// 示例：修复 LaTeX 文件的编码和语法问题
func FixLaTeXFile(path string) error {
    backupMgr := editor.NewBackupManager(".backups")
    encodingHandler := editor.NewEncodingHandler(backupMgr)
    validator := editor.NewLaTeXValidator()
    lineEditor := editor.NewLineEditor(backupMgr)
    
    // 1. 确保 UTF-8 编码
    if err := encodingHandler.EnsureUTF8(path); err != nil {
        return err
    }
    
    // 2. 验证语法
    result, err := validator.ValidateLaTeX(path)
    if err != nil {
        return err
    }
    
    // 3. 修复错误
    if !result.Valid {
        for _, err := range result.Errors {
            // 根据错误类型应用不同的修复策略
            if err.Type == "structure" {
                // 使用 lineEditor 修复
            }
        }
    }
    
    return nil
}
```

## 9. 未来扩展

- 支持更多编码格式
- 可视化编辑界面
- 智能修复建议（基于 AI）
- 版本控制集成
- 协作编辑支持
