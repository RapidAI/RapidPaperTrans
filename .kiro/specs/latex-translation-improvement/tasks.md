# LaTeX 翻译改进任务列表

## 任务概述
实现 LaTeX 源文件预处理功能，在解压后、编译和翻译之前自动修复常见问题。

---

## 任务列表

### 阶段 0: 源文件预处理（优先级最高）

- [x] 0.1 创建预处理函数
  - [x] 0.1.1 在 `internal/compiler/fixer.go` 中添加 `PreprocessTexFiles(dir string) error` 函数
  - [x] 0.1.2 函数扫描指定目录下所有 .tex 文件（包括子目录）
  - [x] 0.1.3 对每个 .tex 文件读取内容并应用 `QuickFix()`
  - [x] 0.1.4 如果有修复，将修复后的内容写回文件
  - [x] 0.1.5 记录日志：哪些文件被修复，修复了什么

- [x] 0.2 集成预处理到主流程
  - [x] 0.2.1 在 `app.go` 的 `ProcessSource()` 函数中，在 `ExtractZip()` 调用后添加预处理调用
  - [x] 0.2.2 预处理应在"查找主 tex 文件"步骤之前完成
  - [x] 0.2.3 预处理应在"编译原始文档"步骤之前完成
  - [x] 0.2.4 更新状态消息显示预处理进度

- [x] 0.3 添加单元测试
  - [x] 0.3.1 测试 `PreprocessTexFiles` 能正确扫描目录
  - [x] 0.3.2 测试 `PreprocessTexFiles` 能修复 bibliography 顺序问题
  - [x] 0.3.3 测试 `PreprocessTexFiles` 不会破坏正常的 tex 文件
  - [x] 0.3.4 测试 `PreprocessTexFiles` 能处理子目录中的 tex 文件

---

### 阶段 1: 翻译质量改进（已部分实现）

- [x] 1.1 改进翻译提示词
- [x] 1.2 实现 QuickFix 规则修复

---

### 阶段 2: 编译驱动的修复流程（已实现）

- [x] 2.1 编译错误解析器
- [x] 2.2 分层修复策略 (Rule -> LLM -> Agent)
- [x] 2.3 修复循环控制

---

## 实现说明

### PreprocessTexFiles 函数设计

```go
// PreprocessTexFiles 预处理目录中的所有 tex 文件
// 在解压后、编译前调用，修复常见的 LaTeX 源文件问题
func PreprocessTexFiles(dir string) error {
    // 1. 遍历目录找到所有 .tex 文件
    // 2. 对每个文件：
    //    a. 读取内容
    //    b. 调用 QuickFix()
    //    c. 如果有修复，写回文件
    //    d. 记录日志
    // 3. 返回错误（如果有）
}
```

### 集成位置

在 `app.go` 的 `ProcessSource()` 函数中，大约在以下位置添加预处理调用：

```go
// Step 2: Download/extract source code based on type
// ... ExtractZip 调用 ...

// Step 2.5: Preprocess tex files (NEW)
a.updateStatus(types.PhaseExtracting, 22, "预处理 LaTeX 源文件...")
if err := compiler.PreprocessTexFiles(sourceInfo.ExtractDir); err != nil {
    logger.Warn("preprocessing failed", logger.Err(err))
    // 不中断流程，继续执行
}

// Step 3: Find main tex file
// ...
```
