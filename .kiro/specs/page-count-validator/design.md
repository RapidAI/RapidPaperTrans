# 页数验证与自动修复功能设计

## 1. 架构概述

```
┌─────────────────────────────────────────────────────────────┐
│                     Compiler Integration                     │
│  (internal/compiler/compiler.go)                            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              PageCountValidator                              │
│  (internal/validator/page_count_validator.go)               │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Compare    │  │   Diagnose   │  │     Fix      │     │
│  │    Pages     │─▶│   Problems   │─▶│   Problems   │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ PDF Parser   │    │  Diagnostic  │    │    Fixer     │
│              │    │    Rules     │    │  Strategies  │
└──────────────┘    └──────────────┘    └──────────────┘
```

## 2. 核心组件设计

### 2.1 PageCountValidator

主验证器，协调整个诊断和修复流程。

```go
type PageCountValidator struct {
    pdfParser    *pdf.PDFParser
    diagnoser    *Diagnoser
    fixer        *Fixer
    backupMgr    *editor.BackupManager
    logger       *logger.Logger
}

type ValidationResult struct {
    OriginalPages   int
    TranslatedPages int
    PageDifference  int
    Problems        []Problem
    FixSuggestions  []FixSuggestion
    FixApplied      bool
    FixResults      []FixResult
}

func (v *PageCountValidator) Validate(
    originalPDF string,
    translatedPDF string,
    texFile string,
) (*ValidationResult, error)

func (v *PageCountValidator) AutoFix(
    texFile string,
    problems []Problem,
) ([]FixResult, error)
```

### 2.2 Diagnoser

诊断器，检测各种可能导致页数缺失的问题。

```go
type Diagnoser struct {
    rules []DiagnosticRule
}

type Problem struct {
    Type        ProblemType
    Severity    Severity
    Location    Location
    Description string
    Details     map[string]interface{}
}

type ProblemType string

const (
    ProblemUnreferencedFile    ProblemType = "unreferenced_file"
    ProblemCommentedInput      ProblemType = "commented_input"
    ProblemExtraBraces         ProblemType = "extra_braces"
    ProblemUnbalancedEnv       ProblemType = "unbalanced_environment"
    ProblemHideOption          ProblemType = "hide_option"
    ProblemConditionalFalse    ProblemType = "conditional_false"
    ProblemAppendixAfterEnd    ProblemType = "appendix_after_end"
    ProblemMissingAppendix     ProblemType = "missing_appendix"
    ProblemLargeCommentBlock   ProblemType = "large_comment_block"
)

type Severity string

const (
    SeverityCritical Severity = "critical"
    SeverityHigh     Severity = "high"
    SeverityMedium   Severity = "medium"
    SeverityLow      Severity = "low"
)

type Location struct {
    File      string
    Line      int
    Column    int
    Context   string
}

func (d *Diagnoser) Diagnose(texFile string, workDir string) ([]Problem, error)
```

### 2.3 DiagnosticRule

诊断规则接口，每个规则检测一种特定问题。

```go
type DiagnosticRule interface {
    Name() string
    Check(ctx *DiagnosticContext) ([]Problem, error)
}

type DiagnosticContext struct {
    MainFile    string
    WorkDir     string
    Content     string
    Lines       []string
    AllFiles    []string
}

// 具体规则实现
type UnreferencedFileRule struct{}
type CommentedInputRule struct{}
type ExtraBracesRule struct{}
type UnbalancedEnvRule struct{}
type HideOptionRule struct{}
type ConditionalFalseRule struct{}
type AppendixPositionRule struct{}
type LargeCommentBlockRule struct{}
```

### 2.4 Fixer

修复器，应用修复策略解决问题。

```go
type Fixer struct {
    strategies map[ProblemType]FixStrategy
    backupMgr  *editor.BackupManager
}

type FixStrategy interface {
    Name() string
    CanFix(problem Problem) bool
    Fix(problem Problem, ctx *FixContext) (*FixResult, error)
}

type FixContext struct {
    File      string
    Content   string
    Lines     []string
    WorkDir   string
    DryRun    bool
}

type FixResult struct {
    Problem     Problem
    Strategy    string
    Success     bool
    Changes     []Change
    Error       error
    BackupFile  string
}

type Change struct {
    Type        ChangeType
    Location    Location
    OldContent  string
    NewContent  string
    Description string
}

type ChangeType string

const (
    ChangeTypeUncomment     ChangeType = "uncomment"
    ChangeTypeRemove        ChangeType = "remove"
    ChangeTypeReplace       ChangeType = "replace"
    ChangeTypeInsert        ChangeType = "insert"
    ChangeTypeMove          ChangeType = "move"
)

// 具体策略实现
type UncommentInputStrategy struct{}
type RemoveHideOptionStrategy struct{}
type FixExtraBracesStrategy struct{}
type ChangeConditionalStrategy struct{}
type MoveAppendixStrategy struct{}
type AddAppendixStrategy struct{}
```

## 3. 诊断规则详细设计

### 3.1 UnreferencedFileRule

检测未被引用的 .tex 文件。

**算法：**
1. 扫描工作目录下所有 .tex 文件
2. 解析主文件中的所有 \input 和 \include 命令
3. 比较找出未被引用的文件
4. 排除特殊文件（如 main.tex, macro.tex）

**输出：**
- 问题类型：ProblemUnreferencedFile
- 严重性：Medium
- 详细信息：文件名、大小、行数

### 3.2 CommentedInputRule

检测被注释的 \input 命令。

**算法：**
1. 逐行扫描主文件
2. 查找以 % 开头且包含 \input 或 \include 的行
3. 提取文件名并检查文件是否存在
4. 如果文件存在，标记为问题

**输出：**
- 问题类型：ProblemCommentedInput
- 严重性：High
- 详细信息：行号、文件名、是否存在

### 3.3 ExtraBracesRule

检测多余的大括号。

**算法：**
1. 逐行追踪大括号平衡
2. 记录每行的平衡变化
3. 检测特定模式（如 }}} 或 }}}}）
4. 检查环境结束后的多余大括号

**输出：**
- 问题类型：ProblemExtraBraces
- 严重性：Critical
- 详细信息：行号、多余的数量、上下文

### 3.4 UnbalancedEnvRule

检测不平衡的环境。

**算法：**
1. 解析所有 \begin{} 和 \end{} 命令
2. 使用栈匹配环境
3. 检测未闭合或多余的 \end{}
4. 记录不匹配的位置

**输出：**
- 问题类型：ProblemUnbalancedEnv
- 严重性：Critical
- 详细信息：环境名、位置、缺失或多余

### 3.5 HideOptionRule

检测隐藏内容的文档类选项。

**算法：**
1. 查找 \documentclass 命令
2. 解析选项列表
3. 检查已知的隐藏选项（hidesupplement, hideappendix 等）

**输出：**
- 问题类型：ProblemHideOption
- 严重性：High
- 详细信息：选项名、行号

### 3.6 ConditionalFalseRule

检测 \iffalse 条件编译。

**算法：**
1. 查找所有 \iffalse 命令
2. 检查是否被注释
3. 估算被跳过的内容量

**输出：**
- 问题类型：ProblemConditionalFalse
- 严重性：Medium
- 详细信息：行号、内容量估算

### 3.7 AppendixPositionRule

检查附录位置。

**算法：**
1. 查找 \appendix 命令位置
2. 查找 \end{document} 位置
3. 检查附录是否在文档结束之后
4. 检查附录区域的内容量

**输出：**
- 问题类型：ProblemAppendixAfterEnd 或 ProblemMissingAppendix
- 严重性：Critical
- 详细信息：附录行号、文档结束行号、内容量

### 3.8 LargeCommentBlockRule

检测大型注释块。

**算法：**
1. 识别连续的注释行（以 % 开头）
2. 统计注释块的大小
3. 如果超过阈值（如 10 行），标记为问题

**输出：**
- 问题类型：ProblemLargeCommentBlock
- 严重性：Low
- 详细信息：起始行、结束行、行数

## 4. 修复策略详细设计

### 4.1 UncommentInputStrategy

取消注释 \input 命令。

**算法：**
1. 定位到问题行
2. 移除行首的 %
3. 保持原始缩进
4. 验证文件存在

**前置条件：**
- 引用的文件必须存在

**后置条件：**
- 文件被正确引用
- 缩进保持不变

### 4.2 RemoveHideOptionStrategy

移除隐藏选项。

**算法：**
1. 定位 \documentclass 行
2. 解析选项列表
3. 移除隐藏选项
4. 处理逗号分隔符

**前置条件：**
- 选项存在于文档类声明中

**后置条件：**
- 选项被移除
- 其他选项保持不变

### 4.3 FixExtraBracesStrategy

修复多余的大括号。

**算法：**
1. 定位到问题行
2. 分析大括号的上下文
3. 移除多余的闭合大括号
4. 验证修复后的平衡

**前置条件：**
- 能够确定哪些大括号是多余的

**后置条件：**
- 大括号平衡
- 语法正确

### 4.4 ChangeConditionalStrategy

将 \iffalse 改为 \iftrue。

**算法：**
1. 定位 \iffalse 命令
2. 替换为 \iftrue
3. 保持其他内容不变

**前置条件：**
- \iffalse 未被注释

**后置条件：**
- 条件内容会被编译

### 4.5 MoveAppendixStrategy

移动附录到正确位置。

**算法：**
1. 提取附录内容（从 \appendix 到 \end{document}）
2. 在 \end{document} 之前插入
3. 移除原位置的附录
4. 调整空行

**前置条件：**
- 附录在 \end{document} 之后

**后置条件：**
- 附录在文档结束之前
- 格式正确

### 4.6 AddAppendixStrategy

添加缺失的附录引用。

**算法：**
1. 查找附录文件（appendix.tex, supplement.tex 等）
2. 在合适位置插入 \appendix 命令
3. 添加 \input 命令
4. 调整位置（在参考文献后、\end{document} 前）

**前置条件：**
- 附录文件存在
- 没有 \appendix 命令

**后置条件：**
- 附录被正确引用
- 位置合适

## 5. 集成设计

### 5.1 编译器集成

在 `internal/compiler/compiler.go` 中集成：

```go
func (c *LaTeXCompiler) Compile(req *types.CompileRequest) *types.CompileResult {
    // ... 现有编译逻辑 ...
    
    if result.Error == nil && req.ValidatePageCount {
        // 运行页数验证
        validator := validator.NewPageCountValidator(c.workDir)
        validationResult, err := validator.Validate(
            req.OriginalPDF,
            result.PDFPath,
            req.TexFile,
        )
        
        if err == nil && validationResult.PageDifference > 0 {
            // 页数不匹配，记录到结果
            result.PageCountIssue = validationResult
            
            // 如果启用自动修复
            if req.AutoFixPageCount {
                fixResults, err := validator.AutoFix(
                    req.TexFile,
                    validationResult.Problems,
                )
                
                if err == nil && len(fixResults) > 0 {
                    // 重新编译
                    result = c.Compile(req)
                    result.FixResults = fixResults
                }
            }
        }
    }
    
    return result
}
```

### 5.2 前端集成

在 `app.go` 中添加方法：

```go
func (a *App) ValidatePageCount(
    originalPDF string,
    translatedPDF string,
    texFile string,
) (*ValidationResult, error) {
    validator := validator.NewPageCountValidator(a.workDir)
    return validator.Validate(originalPDF, translatedPDF, texFile)
}

func (a *App) AutoFixPageCount(
    texFile string,
    problems []Problem,
) ([]FixResult, error) {
    validator := validator.NewPageCountValidator(a.workDir)
    return validator.AutoFix(texFile, problems)
}
```

在前端 `main.js` 中：

```javascript
async function validatePageCount() {
    const result = await ValidatePageCount(
        originalPDF,
        translatedPDF,
        texFile
    );
    
    if (result.PageDifference > 0) {
        showPageCountWarning(result);
        
        if (confirm('发现页数不匹配，是否自动修复？')) {
            const fixResults = await AutoFixPageCount(
                texFile,
                result.Problems
            );
            showFixResults(fixResults);
        }
    }
}
```

### 5.3 命令行工具

创建独立的命令行工具：

```go
// cmd/validate_pages/main.go
func main() {
    texFile := flag.String("tex", "", "LaTeX file path")
    originalPDF := flag.String("original", "", "Original PDF path")
    translatedPDF := flag.String("translated", "", "Translated PDF path")
    autoFix := flag.Bool("fix", false, "Auto fix problems")
    flag.Parse()
    
    validator := validator.NewPageCountValidator(filepath.Dir(*texFile))
    result, err := validator.Validate(*originalPDF, *translatedPDF, *texFile)
    
    // 输出诊断报告
    printReport(result)
    
    if *autoFix && len(result.Problems) > 0 {
        fixResults, err := validator.AutoFix(*texFile, result.Problems)
        printFixResults(fixResults)
    }
}
```

## 6. 配置设计

```go
type PageCountValidatorConfig struct {
    // 是否启用自动验证
    Enabled bool
    
    // 是否自动修复
    AutoFix bool
    
    // 页数差异阈值（小于此值不报警）
    PageDifferenceThreshold int
    
    // 大型注释块阈值（行数）
    LargeCommentBlockThreshold int
    
    // 启用的诊断规则
    EnabledRules []string
    
    // 启用的修复策略
    EnabledStrategies []string
    
    // 备份配置
    BackupConfig BackupConfig
}
```

## 7. 错误处理

```go
var (
    ErrPDFNotFound      = errors.New("PDF file not found")
    ErrTexNotFound      = errors.New("TeX file not found")
    ErrCannotReadPDF    = errors.New("cannot read PDF")
    ErrCannotParseTex   = errors.New("cannot parse TeX file")
    ErrFixFailed        = errors.New("fix failed")
    ErrBackupFailed     = errors.New("backup failed")
    ErrNoProblemsFound  = errors.New("no problems found")
)
```

## 8. 测试策略

### 8.1 单元测试

每个诊断规则和修复策略都需要单元测试：

```go
func TestUnreferencedFileRule(t *testing.T) {
    // 测试用例
}

func TestCommentedInputRule(t *testing.T) {
    // 测试用例
}

func TestExtraBracesRule(t *testing.T) {
    // 测试用例
}

// ... 其他规则和策略的测试
```

### 8.2 集成测试

测试完整的诊断-修复流程：

```go
func TestPageCountValidation(t *testing.T) {
    // 准备测试文件
    // 运行验证
    // 检查结果
}

func TestAutoFix(t *testing.T) {
    // 准备有问题的文件
    // 运行自动修复
    // 验证修复结果
    // 检查备份文件
}
```

### 8.3 端到端测试

使用真实的 arXiv 论文：

```go
func TestRealPaper_2501_17161(t *testing.T) {
    // 使用 arXiv 2501.17161 测试
    // 验证能够检测和修复问题
}
```

## 9. 性能优化

### 9.1 缓存策略
- 缓存文件内容，避免重复读取
- 缓存 PDF 页数信息
- 缓存诊断结果

### 9.2 并行处理
- 并行运行多个诊断规则
- 并行处理多个文件

### 9.3 增量处理
- 只处理修改过的文件
- 记录上次验证结果

## 10. 可扩展性

### 10.1 插件机制

支持外部插件添加新的诊断规则和修复策略：

```go
type Plugin interface {
    Name() string
    Version() string
    Rules() []DiagnosticRule
    Strategies() []FixStrategy
}

func (v *PageCountValidator) RegisterPlugin(plugin Plugin) error
```

### 10.2 自定义规则

支持用户定义自定义规则：

```go
type CustomRule struct {
    name    string
    pattern *regexp.Regexp
    check   func(*DiagnosticContext) ([]Problem, error)
}
```

## 11. 监控和日志

```go
type ValidationMetrics struct {
    TotalValidations    int
    ProblemsDetected    int
    ProblemsFixed       int
    FixSuccessRate      float64
    AverageFixTime      time.Duration
}

func (v *PageCountValidator) GetMetrics() *ValidationMetrics
```

## 12. 文档生成

自动生成诊断报告：

```go
type ReportGenerator interface {
    Generate(result *ValidationResult) (string, error)
}

type TextReportGenerator struct{}
type HTMLReportGenerator struct{}
type JSONReportGenerator struct{}
type MarkdownReportGenerator struct{}
```
