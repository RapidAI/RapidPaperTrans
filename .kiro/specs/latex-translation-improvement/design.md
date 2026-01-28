# LaTeX 翻译改进设计文档

## 1. 整体架构

### 1.1 改进后的翻译流程

```
┌─────────────────────────────────────────────────────────────────┐
│                      翻译与修复流程                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐  │
│  │ 1. 预处理 │───▶│ 2. 分块  │───▶│ 3. 翻译  │───▶│ 4. 合并  │  │
│  │ 提取结构  │    │ 智能切分 │    │ 严格模式 │    │ 重组文档 │  │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘  │
│                                                        │        │
│                                                        ▼        │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐  │
│  │ 8. 完成  │◀───│ 7. 验证  │◀───│ 6. 修复  │◀───│ 5. 编译  │  │
│  │ 输出PDF  │    │ 再次编译 │    │ 增量修复 │    │ 检测错误 │  │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘  │
│                        │                │                       │
│                        │                │                       │
│                        └────────────────┘                       │
│                         循环直到成功或达到最大次数                │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 核心组件

1. **DocumentPreprocessor**: 预处理文档，提取结构信息
2. **SmartChunker**: 智能分块器，按 LaTeX 结构切分
3. **StrictTranslator**: 严格模式翻译器，改进的提示词
4. **CompileErrorParser**: 编译错误解析器
5. **IncrementalFixer**: 增量修复器，最小化上下文

---

## 2. 详细设计

### 2.1 改进的翻译提示词

```go
// 新的系统提示词 - 更严格的指令
const StrictTranslationSystemPrompt = `你是一个专业的 LaTeX 文档翻译器。你的任务是将英文 LaTeX 文档翻译成中文。

【绝对禁止】
1. 禁止添加任何解释、说明或注释
2. 禁止使用 markdown 代码块（如 ` + "```" + `latex 或 ` + "```" + `）
3. 禁止修改任何 LaTeX 命令、宏或环境
4. 禁止翻译数学公式内的任何内容
5. 禁止删除或添加任何 LaTeX 命令
6. 禁止改变文档结构

【必须遵守】
1. 只翻译自然语言文本（英文→中文）
2. 保持所有 LaTeX 命令完全不变，包括：
   - \section, \subsection, \chapter 等结构命令
   - \begin{...}, \end{...} 环境
   - \ref, \cite, \label 等引用命令
   - \textbf, \textit, \emph 等格式命令
   - 所有数学环境：$...$, $$...$$, \[...\], \(...\)
   - equation, align, figure, table 等环境
3. 保持原文的换行和空行
4. 使用中文标点符号（。，、；：""''）
5. 直接输出翻译后的 LaTeX 内容，不要有任何前缀或后缀

【输出格式】
直接输出翻译后的 LaTeX 代码，第一个字符就是 LaTeX 内容的开始。`
```

### 2.2 智能分块策略

```go
// ChunkingStrategy 定义分块策略
type ChunkingStrategy struct {
    MaxChunkSize     int      // 最大分块大小（字符数）
    OverlapSize      int      // 重叠大小，用于保持上下文
    PreserveEnvs     []string // 需要保持完整的环境列表
}

// 默认配置
var DefaultChunkingStrategy = ChunkingStrategy{
    MaxChunkSize: 3000,  // 约 750 tokens
    OverlapSize:  200,   // 重叠 200 字符
    PreserveEnvs: []string{
        "equation", "align", "figure", "table", 
        "theorem", "proof", "lemma", "definition",
        "itemize", "enumerate", "description",
    },
}

// SmartChunker 智能分块器
type SmartChunker struct {
    strategy ChunkingStrategy
}

// Chunk 将文档分块
func (c *SmartChunker) Chunk(content string) []DocumentChunk {
    // 1. 解析文档结构
    // 2. 识别所有环境边界
    // 3. 在安全位置切分（章节边界、段落边界）
    // 4. 确保每个分块不超过最大大小
    // 5. 添加必要的上下文信息
}
```

### 2.3 编译错误解析器

```go
// CompileError 表示一个编译错误
type CompileError struct {
    Line       int    // 错误行号
    Column     int    // 错误列号（如果有）
    Type       string // 错误类型
    Message    string // 错误消息
    Context    string // 错误上下文（前后几行）
    Severity   string // 严重程度: "error", "warning"
    SourceFile string // 源文件名
}

// CompileErrorParser 编译错误解析器
type CompileErrorParser struct{}

// Parse 解析编译日志
func (p *CompileErrorParser) Parse(log string) []CompileError {
    // 常见的 LaTeX 错误模式：
    // 1. "! Undefined control sequence." - 未定义命令
    // 2. "! Missing $ inserted." - 缺少数学模式
    // 3. "! Extra }, or forgotten $." - 括号不匹配
    // 4. "! LaTeX Error: Environment xxx undefined." - 未定义环境
    // 5. "! Package xxx Error:" - 包错误
    // 6. "l.123" - 错误行号
}

// 错误模式正则表达式
var errorPatterns = []struct {
    Pattern *regexp.Regexp
    Type    string
}{
    {regexp.MustCompile(`^! (.+)$`), "latex_error"},
    {regexp.MustCompile(`^l\.(\d+)`), "line_number"},
    {regexp.MustCompile(`^! LaTeX Error: (.+)$`), "latex_error"},
    {regexp.MustCompile(`^! Package (\w+) Error: (.+)$`), "package_error"},
    {regexp.MustCompile(`Undefined control sequence`), "undefined_command"},
    {regexp.MustCompile(`Missing \$ inserted`), "missing_math_mode"},
    {regexp.MustCompile(`Extra \}, or forgotten \$`), "bracket_mismatch"},
}
```

### 2.4 增量修复器

```go
// IncrementalFixer 增量修复器
type IncrementalFixer struct {
    apiKey        string
    model         string
    apiURL        string
    contextLines  int  // 错误上下文行数，默认 10
    maxAttempts   int  // 最大修复尝试次数，默认 5
}

// FixRequest 修复请求
type FixRequest struct {
    ErrorLine     int      // 错误行号
    ErrorMessage  string   // 错误消息
    CodeSnippet   string   // 代码片段（错误行及上下文）
    StartLine     int      // 代码片段起始行号
    EndLine       int      // 代码片段结束行号
    DocumentInfo  string   // 文档信息（使用的包等）
}

// Fix 修复单个错误
func (f *IncrementalFixer) Fix(content string, err CompileError) (string, error) {
    // 1. 提取错误周围的代码片段
    snippet := f.extractSnippet(content, err.Line, f.contextLines)
    
    // 2. 构建修复请求
    request := FixRequest{
        ErrorLine:    err.Line,
        ErrorMessage: err.Message,
        CodeSnippet:  snippet.Code,
        StartLine:    snippet.StartLine,
        EndLine:      snippet.EndLine,
        DocumentInfo: f.extractDocumentInfo(content),
    }
    
    // 3. 调用 LLM 修复
    fixedSnippet, err := f.callLLM(request)
    if err != nil {
        return "", err
    }
    
    // 4. 替换原文档中的对应部分
    return f.replaceSnippet(content, snippet, fixedSnippet), nil
}

// 修复提示词
const FixSystemPrompt = `你是一个 LaTeX 语法修复专家。你的任务是修复 LaTeX 编译错误。

【输入】
- 错误消息：编译器报告的错误
- 代码片段：包含错误的代码及其上下文
- 行号信息：错误在原文档中的位置

【输出要求】
1. 只输出修复后的代码片段
2. 保持代码片段的行数不变（除非必须添加/删除行）
3. 只修改必要的部分，不要改动其他内容
4. 不要添加任何解释或注释

【常见修复】
- 未定义命令：检查拼写或添加必要的包
- 括号不匹配：添加缺失的括号
- 缺少数学模式：添加 $ 或 \( \)
- 环境不匹配：修正 \begin 和 \end 的环境名`
```

### 2.5 修复循环控制

```go
// FixLoop 修复循环
type FixLoop struct {
    compiler    *LaTeXCompiler
    fixer       *IncrementalFixer
    parser      *CompileErrorParser
    maxAttempts int
}

// Run 运行修复循环
func (l *FixLoop) Run(texPath string, outputDir string) (*types.CompileResult, error) {
    content, _ := os.ReadFile(texPath)
    
    for attempt := 1; attempt <= l.maxAttempts; attempt++ {
        // 1. 尝试编译
        result, _ := l.compiler.Compile(texPath, outputDir)
        
        if result.Success {
            return result, nil
        }
        
        // 2. 解析错误
        errors := l.parser.Parse(result.Log)
        
        if len(errors) == 0 {
            // 无法解析错误，返回失败
            return result, fmt.Errorf("compilation failed but no errors parsed")
        }
        
        // 3. 按优先级排序错误
        sortedErrors := l.prioritizeErrors(errors)
        
        // 4. 修复第一个（最重要的）错误
        fixedContent, err := l.fixer.Fix(string(content), sortedErrors[0])
        if err != nil {
            return nil, err
        }
        
        // 5. 保存修复后的内容
        content = []byte(fixedContent)
        os.WriteFile(texPath, content, 0644)
        
        // 6. 记录日志
        logger.Info("fix attempt completed", 
            logger.Int("attempt", attempt),
            logger.Int("errorsRemaining", len(errors)))
    }
    
    return nil, fmt.Errorf("max fix attempts reached")
}

// prioritizeErrors 按优先级排序错误
func (l *FixLoop) prioritizeErrors(errors []CompileError) []CompileError {
    // 排序规则：
    // 1. 致命错误优先
    // 2. 行号小的优先
    // 3. 相同类型的错误分组
}
```

---

## 3. 配置扩展

### 3.1 新增配置项

```go
// Config 扩展配置
type Config struct {
    // ... 现有配置 ...
    
    // 翻译配置
    TranslationChunkSize int `json:"translation_chunk_size"` // 翻译分块大小，默认 3000
    
    // 修复配置
    MaxFixAttempts    int `json:"max_fix_attempts"`    // 最大修复尝试次数，默认 5
    FixContextLines   int `json:"fix_context_lines"`   // 修复上下文行数，默认 10
    
    // 上下文配置
    ContextWindow     int `json:"context_window"`      // LLM 上下文窗口大小，默认 8192
}
```

---

## 4. 错误处理策略

### 4.1 常见错误及修复策略

| 错误类型 | 错误消息示例 | 修复策略 |
|---------|-------------|---------|
| 未定义命令 | `Undefined control sequence \xxx` | 检查拼写，或建议添加包 |
| 括号不匹配 | `Extra }, or forgotten $` | 分析括号配对，添加缺失的括号 |
| 缺少数学模式 | `Missing $ inserted` | 在数学符号周围添加 $ |
| 环境不匹配 | `Environment xxx undefined` | 检查环境名拼写，或添加必要的包 |
| 编码问题 | `Package inputenc Error` | 建议使用 xelatex 或修改编码声明 |

### 4.2 修复失败处理

1. **记录详细日志**：保存每次修复尝试的输入输出
2. **保存中间结果**：保存每次修复后的文档版本
3. **提供诊断信息**：向用户展示无法修复的错误列表
4. **建议手动修复**：对于复杂错误，提供修复建议

---

## 5. 性能优化

### 5.1 减少 API 调用

1. **批量处理相邻错误**：如果多个错误在相邻行，合并为一个修复请求
2. **缓存文档信息**：文档结构信息只提取一次
3. **跳过重复错误**：相同类型的错误只修复一次

### 5.2 上下文优化

1. **动态调整上下文大小**：根据错误复杂度调整上下文行数
2. **压缩文档信息**：只包含必要的包声明和文档类型
3. **分批修复**：如果错误太多，分批处理

---

## 6. 实现优先级

### Phase 1: 核心改进
1. 改进翻译提示词（严格模式）
2. 实现编译错误解析器
3. 实现基本的增量修复器

### Phase 2: 智能分块
1. 实现智能分块器
2. 添加环境边界检测
3. 优化分块策略

### Phase 3: 高级功能
1. 实现修复循环控制
2. 添加错误优先级排序
3. 优化上下文使用

---

## 7. 测试策略

### 7.1 单元测试
- 测试错误解析器对各种错误格式的解析
- 测试分块器的边界检测
- 测试修复器的代码替换逻辑

### 7.2 集成测试
- 测试完整的翻译-编译-修复流程
- 测试各种类型的 LaTeX 文档
- 测试错误恢复机制

### 7.3 性能测试
- 测试大型文档的处理时间
- 测试 API 调用次数
- 测试内存使用
