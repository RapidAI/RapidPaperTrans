# LaTeX 自动修复功能

## 功能概述

为了提高编译成功率，我们实现了一个自动修复系统，可以在编译失败后自动修复常见的 LaTeX 语法错误，然后重新尝试编译。

## 工作流程

```
1. 尝试编译 LaTeX 文件
   ↓
2. 编译失败？
   ↓ 是
3. 自动检测并修复常见错误
   ↓
4. 创建备份文件
   ↓
5. 应用修复
   ↓
6. 重新编译
   ↓
7. 成功？
   ↓ 是
8. 返回成功结果（包含修复信息）
   ↓ 否
9. 恢复备份，返回原始错误
```

## 支持的自动修复

### 1. 移除 \end{document} 后的内容

**问题**: 文档结束标记后有额外的非注释内容

**修复**: 删除 `\end{document}` 之后的所有非注释内容

**示例**:
```latex
% 修复前
\end{document}
}}
extra content

% 修复后
\end{document}
```

### 2. 修复常见拼写错误

**支持的修复**:
- `\begn{` → `\begin{`
- `\ened{` → `\end{`
- `\docmentclass` → `\documentclass`

**示例**:
```latex
% 修复前
\begn{document}
Hello
\ened{document}

% 修复后
\begin{document}
Hello
\end{document}
```

### 3. 移除重复的 \end{document}

**问题**: 文件中有多个 `\end{document}` 命令

**修复**: 只保留最后一个 `\end{document}`

**示例**:
```latex
% 修复前
\begin{document}
Content
\end{document}
More content
\end{document}

% 修复后
\begin{document}
Content
More content
\end{document}
```

### 4. 规范化行结束符

**问题**: 混合使用 Windows (`\r\n`) 和 Unix (`\n`) 行结束符

**修复**: 统一转换为 Unix 格式 (`\n`)

## 使用方式

### 方式 1: 自动集成（推荐）

编译器会自动尝试修复，无需额外配置：

```go
compiler := compiler.NewLaTeXCompiler(compiler.CompilerXeLaTeX, workDir, 5*time.Minute)
result, err := compiler.Compile(texPath, outputDir)

if err == nil && result.Success {
    // 检查是否应用了自动修复
    if strings.Contains(result.Log, "Automatic Fixes Applied") {
        fmt.Println("编译成功（应用了自动修复）")
    }
}
```

### 方式 2: 独立使用修复器

可以在编译前手动修复文件：

```go
// 创建修复器
fixer := validator.NewSimpleFixer(workDir)

// 智能修复（先验证，再修复）
fixResult, err := fixer.SmartFix(texPath)
if err != nil {
    return err
}

if fixResult.Fixed {
    fmt.Printf("应用了 %d 个修复:\n", len(fixResult.FixesApplied))
    for _, fix := range fixResult.FixesApplied {
        fmt.Printf("  - %s\n", fix)
    }
}
```

### 方式 3: 批量修复目录中的所有文件

```go
fixer := validator.NewSimpleFixer(workDir)
results, err := fixer.FixAllTexFiles(extractDir)

for file, result := range results {
    fmt.Printf("修复了 %s:\n", file)
    for _, fix := range result.FixesApplied {
        fmt.Printf("  - %s\n", fix)
    }
}
```

## 安全机制

### 1. 自动备份

每次修复前都会创建备份文件：
- 备份文件名: `原文件名.autofix.backup`
- 如果修复后编译仍然失败，会自动恢复备份

### 2. 非侵入性

- 只修复明确可以安全修复的问题
- 对于复杂的错误（如环境不匹配），不会尝试自动修复
- 修复失败时不会影响原始文件

### 3. 详细日志

修复过程会记录详细日志：
```
=== Automatic Fixes Applied ===
Removed content after \end{document}
Fixed typo: \begn{ -> \begin{ (2 occurrences)

=== First Pass ===
...
```

## 测试

### 运行自动修复测试

```bash
# 测试简单修复器和编译器集成
go run ./cmd/test_autofix/main.go

# 运行单元测试
go test ./internal/validator -v -run TestSimpleFixer
```

### 测试输出示例

```
=== 测试简单修复器 ===

1. 创建了有问题的文件:
   - 在 \end{document} 后有额外的 }}
   - 在 \end{document} 后有额外内容

2. 验证文件...
   验证结果: ✗ Validation failed: 3 error(s), 0 warning(s)

3. 尝试自动修复...
   ✓ 应用了 1 个修复:
     - Removed content after \end{document}
   ✓ 备份文件: test_output_autofix\broken.tex.backup

4. 重新验证修复后的文件...
   验证结果: ✓ LaTeX source validation passed with no issues
   ✓ 文件现在是有效的!
```

## API 参考

### SimpleFixer

```go
type SimpleFixer struct {
    workDir string
}

// 创建修复器
func NewSimpleFixer(workDir string) *SimpleFixer

// 智能修复（先验证，再修复）
func (f *SimpleFixer) SmartFix(filePath string) (*FixResult, error)

// 尝试修复文件
func (f *SimpleFixer) TryFixFile(filePath string, validationResult *ValidationResult) (*FixResult, error)

// 批量修复目录中的所有 .tex 文件
func (f *SimpleFixer) FixAllTexFiles(dir string) (map[string]*FixResult, error)

// 使用正则表达式修复
func (f *SimpleFixer) FixWithRegex(filePath string, pattern string, replacement string, description string) (*FixResult, error)

// 恢复备份
func (f *SimpleFixer) RestoreBackup(filePath string) error

// 清理备份文件
func (f *SimpleFixer) CleanupBackup(filePath string) error
```

### FixResult

```go
type FixResult struct {
    Fixed        bool     // 是否应用了修复
    FixesApplied []string // 应用的修复列表
    BackupPath   string   // 备份文件路径
}
```

## 与验证器的集成

自动修复器与验证器紧密集成：

```go
// 1. 验证文件
validator := validator.NewLaTeXValidator(workDir)
validationResult, _ := validator.ValidateMainFile(texPath)

// 2. 如果有问题，尝试修复
if !validationResult.Valid {
    fixer := validator.NewSimpleFixer(workDir)
    fixResult, _ := fixer.TryFixFile(texPath, validationResult)
    
    // 3. 重新验证
    if fixResult.Fixed {
        newResult, _ := validator.ValidateMainFile(texPath)
        if newResult.Valid {
            fmt.Println("修复成功！")
        }
    }
}
```

## 未来改进

### 计划中的修复功能

1. **智能括号修复**
   - 自动添加缺失的闭合括号
   - 修复括号不匹配问题

2. **环境修复**
   - 自动添加缺失的 `\end{environment}`
   - 修复环境名称不匹配

3. **包依赖修复**
   - 检测缺失的包
   - 自动添加 `\usepackage{}`

4. **AI 辅助修复**
   - 使用 AI 模型分析复杂错误
   - 提供智能修复建议

### 配置选项（未来）

```go
type FixerConfig struct {
    EnableAutoFix     bool     // 是否启用自动修复
    MaxFixAttempts    int      // 最大修复尝试次数
    EnabledFixes      []string // 启用的修复类型
    CreateBackup      bool     // 是否创建备份
    RestoreOnFailure  bool     // 失败时是否恢复
}
```

## 性能考虑

- **快速**: 修复操作通常在毫秒级完成
- **轻量**: 只读取和修改必要的文件
- **并行**: 可以并行修复多个文件
- **缓存**: 验证结果可以缓存，避免重复验证

## 最佳实践

1. **先验证，再修复**: 使用 `SmartFix()` 而不是直接 `TryFixFile()`
2. **保留备份**: 不要立即删除备份文件，以便需要时恢复
3. **记录修复**: 在日志中记录所有应用的修复
4. **测试修复**: 修复后重新验证和编译
5. **用户通知**: 告知用户应用了哪些修复

## 故障排除

### 问题: 修复后仍然编译失败

**解决方案**:
1. 检查编译日志，查看具体错误
2. 手动检查修复后的文件
3. 使用验证器查看剩余问题
4. 考虑使用 AI 辅助修复（未来功能）

### 问题: 备份文件未创建

**解决方案**:
1. 检查文件权限
2. 确保有足够的磁盘空间
3. 查看日志中的警告信息

### 问题: 修复破坏了文件

**解决方案**:
1. 使用 `RestoreBackup()` 恢复原始文件
2. 报告问题，帮助改进修复逻辑
3. 手动修复文件

## 总结

自动修复功能显著提高了 LaTeX 编译的成功率，特别是对于从 arXiv 下载的源文件。通过自动修复常见错误，用户可以更快地获得可用的 PDF，而无需手动修复语法问题。

该功能设计为安全、非侵入性的，并且与现有的验证和编译流程无缝集成。
