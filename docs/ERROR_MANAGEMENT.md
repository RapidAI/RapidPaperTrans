# 错误文档管理功能

## 概述

错误文档管理功能用于记录和管理论文翻译过程中出现的错误，并提供便捷的重试机制。当翻译过程在任何阶段失败时，系统会自动记录错误信息，用户可以通过界面查看错误详情并一键重试。

## 功能特性

### 1. 自动错误记录

系统会自动记录以下阶段的错误：

- **下载阶段** (download): arXiv 论文下载失败
- **解压阶段** (extract): 源文件解压失败
- **原始文档编译** (original_compile): 原始 LaTeX 文档编译失败
- **翻译阶段** (translation): 文本翻译失败
- **翻译后编译** (translated_compile): 翻译后的文档编译失败
- **PDF生成** (pdf_generation): 双语对照 PDF 生成失败

### 2. 错误信息记录

每条错误记录包含：

- **ID**: 论文的唯一标识符（通常是 arXiv ID）
- **标题**: 论文标题
- **输入**: 原始输入（URL/ID/路径）
- **出错阶段**: 具体在哪个阶段出错
- **错误消息**: 详细的错误信息
- **时间戳**: 错误发生的时间
- **重试次数**: 已经重试的次数
- **最后重试时间**: 上次重试的时间

### 3. 一键重试

- 点击"重试"按钮可以重新开始完整的翻译流程
- 系统会自动记录重试次数
- **重试成功后，错误记录会自动移除**
- 重试失败后，错误信息会被更新

### 4. 错误管理

- 查看所有错误记录
- 清除单个错误记录
- 批量清除所有错误记录
- **导出错误列表到文本文件** ✅

## 使用方法

### 查看错误列表

1. 点击主界面头部的 "⚠️ 错误" 按钮
2. 打开错误管理模态框
3. 查看所有出错的论文列表

### 重试翻译

1. 在错误列表中找到要重试的论文
2. 点击该项的 "🔄 重试" 按钮
3. 系统会自动关闭错误管理窗口并开始重新翻译
4. 翻译成功后，该错误记录会自动移除

### 清除错误记录

**清除单个记录：**
1. 点击错误项的 "🗑️ 清除" 按钮
2. 确认删除操作

**清除所有记录：**
1. 点击模态框底部的 "清除全部" 按钮
2. 确认删除操作

### 导出错误列表

**导出到文本文件：**
1. 点击模态框底部的 "📤 导出列表" 按钮
2. 选择保存位置和文件名
3. 系统会生成包含所有错误详情和 arXiv ID 列表的文本文件

详细说明请参考：[错误导出功能指南](ERROR_EXPORT_GUIDE.md)

## 技术实现

### 后端实现

#### 错误管理器 (ErrorManager)

位置：`internal/errors/manager.go`

主要方法：

```go
// 创建错误管理器
func NewErrorManager(baseDir string) (*ErrorManager, error)

// 记录错误
func (em *ErrorManager) RecordError(id, title, input string, stage ErrorStage, errorMsg string) error

// 增加重试次数
func (em *ErrorManager) IncrementRetry(id string) error

// 移除错误记录
func (em *ErrorManager) RemoveError(id string) error

// 列出所有错误
func (em *ErrorManager) ListErrors() []*ErrorRecord

// 清除所有错误
func (em *ErrorManager) ClearAll() error
```

#### App 集成

位置：`app.go`

新增方法：

```go
// 列出所有错误记录
func (a *App) ListErrors() ([]*errors.ErrorRecord, error)

// 从错误记录重试翻译
func (a *App) RetryFromError(id string) (*types.ProcessResult, error)

// 清除特定错误记录
func (a *App) ClearError(id string) error

// 清除所有错误记录
func (a *App) ClearAllErrors() error

// 导出错误列表到文本文件
func (a *App) ExportErrorsToFile() (string, error)
```

### 前端实现

#### 错误管理模块

位置：`frontend/src/errors.js`

主要功能：

- 错误列表显示
- 错误项渲染
- 重试操作
- 清除操作
- 导出操作
- 时间格式化
- 阶段名称显示

#### UI 组件

位置：`frontend/index.html`

新增元素：

- 错误管理按钮 (`btn-errors`)
- 错误管理模态框 (`errors-modal`)
- 错误列表容器 (`errors-list`)
- 清除全部按钮 (`btn-clear-all-errors`)
- 导出列表按钮 (`btn-export-errors`)

### 数据存储

错误记录存储在：

```
~/.latex-translator/errors/errors.json
```

数据格式：

```json
[
  {
    "id": "2501.17161",
    "title": "Example Paper Title",
    "input": "https://arxiv.org/abs/2501.17161",
    "stage": "download",
    "error_msg": "Failed to download: connection timeout",
    "timestamp": "2026-01-28T10:30:00Z",
    "can_retry": true,
    "retry_count": 1,
    "last_retry": "2026-01-28T11:00:00Z"
  }
]
```

## 工作流程

### 错误记录流程

```
翻译开始
    ↓
某阶段出错
    ↓
记录错误信息
    ↓
保存到 errors.json
    ↓
显示错误状态
```

### 重试流程

```
用户点击重试
    ↓
增加重试计数
    ↓
重新开始完整翻译
    ↓
成功 → 自动移除错误记录 ✓
    ↓
失败 → 更新错误记录（保留在列表中）
```

## 最佳实践

### 1. 定期检查错误列表

建议定期查看错误管理界面，了解哪些论文翻译失败，分析失败原因。

### 2. 合理重试

- 对于网络相关错误（下载失败），可以稍后重试
- 对于编译错误，可能需要先检查 LaTeX 环境
- 对于翻译错误，可能需要检查 API 配置

### 3. 清理无用记录

对于确定无法修复的错误，可以清除记录以保持列表整洁。

### 4. 分析错误模式

如果某类错误频繁出现，可能需要：
- 检查网络连接
- 更新 LaTeX 环境
- 调整 API 配置
- 报告 bug

## 故障排除

### 错误记录未显示

1. 检查错误管理器是否正确初始化
2. 查看日志文件确认错误是否被记录
3. 检查 `~/.latex-translator/errors/errors.json` 文件

### 重试失败

1. 查看错误消息了解失败原因
2. 检查相关配置（API key、LaTeX 环境等）
3. 尝试手动输入 arXiv ID 重新翻译

### 错误记录无法清除

1. 检查文件权限
2. 确认 `errors.json` 文件未被其他程序占用
3. 尝试重启应用

## 未来改进

- [ ] 错误统计和分析
- [ ] 错误分类和过滤
- [ ] 批量重试功能
- [x] 错误导出功能 ✅ (已实现 - 见 [导出功能指南](ERROR_EXPORT_GUIDE.md))
- [ ] 错误通知提醒
- [ ] 自动重试策略
