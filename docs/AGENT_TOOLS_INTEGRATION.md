# Agent 编辑工具集成完成

## 概述

已成功将编辑工具集成到 Agent 中，Agent 现在可以通过函数调用的方式使用这些工具来修复 LaTeX 文件。

## 集成的工具

### 文件读取工具
- **read_file**: 读取完整文件内容
- **read_lines**: 读取指定行范围（对大文件更高效）

### 文件编辑工具
- **write_file**: 写入完整文件内容（用于小文件或完全重写）
- **replace_line**: 替换单行（推荐用于精确修复）
- **insert_line**: 在指定位置插入新行
- **delete_line**: 删除指定行

### 编码工具
- **detect_encoding**: 检测文件编码（UTF-8, GBK, UTF-8-BOM 等）
- **fix_encoding**: 将文件转换为 UTF-8（用于修复乱码）

### 验证工具
- **validate_latex**: 检查 LaTeX 语法（括号、环境、常见错误）

### 备份工具
- **create_backup**: 在进行危险更改前创建备份

### 其他工具
- **compile_latex**: 编译并测试更改
- **list_files**: 列出所有文件以了解结构
- **search_in_files**: 在所有文件中搜索模式
- **fix_complete**: 修复完成时调用

## Agent 工作流程

Agent 现在遵循以下系统化调试策略：

### 1. 理解结构
- 列出所有文件以查看文档组织
- 读取主文件以了解文档流程
- 识别包含/输入的文件

### 2. 分析根本原因
- 仔细阅读编译日志
- 查找第一个错误（后续错误通常是级联的）
- 识别有问题的文件和行号
- 使用 read_lines 检查错误位置及其上下文

### 3. 首先检查编码问题
- 如果在错误中看到乱码中文（例如 "鎮ㄧ殑", "锟斤拷"）
- 使用 detect_encoding 检查文件编码
- 使用 fix_encoding 转换为 UTF-8
- 这通常可以一次性解决多个级联错误

### 4. 识别模式
- 这是结构错误（环境不匹配）吗？
- 这是语法错误（括号多余/缺失）吗？
- 这是包冲突吗？
- 这是翻译产物吗？

### 5. 系统化修复
- 使用 replace_line 进行最小化、针对性的更改
- 修复根本原因，而不是症状
- 保留所有内容和翻译
- 每次重要更改后进行测试

### 6. 验证和迭代
- 编译以验证修复
- 如果出现新错误，重复该过程
- 编译成功时调用 fix_complete

## 常见错误模式

### A. 编码错误（首先检查！）
- 乱码中文字符："鎮ㄧ殑", "锟斤拷", "�"
- UTF-8 BOM 导致问题
- GBK/GB2312 编码而不是 UTF-8
- **解决方案**: 使用 detect_encoding + fix_encoding

### B. 结构错误
- \begin{} 和 \end{} 环境不匹配
- 未闭合的环境（特别是 figure*, table*, tikzpicture）
- \end{} 命令后的额外闭合括号（例如 \end{figure*}}}）
- 跨多个文件的嵌套环境问题

### C. 语法错误
- 括号多余或缺失：{, }
- 翻译导致的 LaTeX 命令损坏（例如 \引用 而不是 \cite）
- 命令参数格式错误

### D. 包冲突
- ctex/xeCJK 兼容性问题
- 包加载顺序问题
- 缺少包依赖

### E. 翻译产物
- 命令名称中的中文字符
- 数学模式分隔符损坏
- 内容中的 JSON 产物

## 工具使用最佳实践

1. **对大文件使用 read_lines 而不是 read_file**
2. **对单行修复使用 replace_line 而不是 write_file**
3. **看到乱码文本时使用 detect_encoding**
4. **编译前使用 validate_latex 检查语法**
5. **进行危险更改前使用 create_backup**

## 示例工作流程

```
1. list_files → 了解结构
2. read_file(main.tex) → 查看文档流程
3. 分析错误日志 → 识别第一个错误
4. 如果有乱码文本: detect_encoding(file) → fix_encoding(file)
5. read_lines(file, start, end) → 查看错误上下文
6. validate_latex(file) → 检查语法
7. replace_line(file, line_num, fixed_content) → 应用针对性修复
8. compile_latex → 测试修复
9. 如果需要则重复
10. fix_complete → 成功时
```

## 关键规则

1. **永远不要删除内容** - 只修复语法错误
2. **始终保留中文翻译**
3. **进行最小化更改** - 尽可能使用 replace_line 而不是 write_file
4. **频繁测试** - 每次修复后编译
5. **系统化思考** - 修复前先理解
6. **读取实际错误位置** - 不要猜测
7. **修复根本原因** - 不要只是修补症状
8. **首先检查编码** - 许多错误是由编码问题引起的

## 技术实现

### 代码位置
- **Agent 实现**: `latex-translator/internal/compiler/agent_fixer.go`
- **编辑器工具**: `latex-translator/internal/editor/`
  - `line_editor.go` - 行级编辑
  - `encoding_handler.go` - 编码处理
  - `latex_validator.go` - LaTeX 验证
  - `backup_manager.go` - 备份管理

### 工具初始化
在 `NewLaTeXAgentFixer` 中初始化所有编辑器工具：
```go
backupMgr := editor.NewBackupManager("")
lineEditor := editor.NewLineEditor(backupMgr)
encodingHandler := editor.NewEncodingHandler(backupMgr)
validator := editor.NewLaTeXValidator()
```

### 工具执行
在 `executeTool` 中处理所有工具调用，每个工具都有对应的实现函数：
- `toolReadLines`
- `toolReplaceLine`
- `toolInsertLine`
- `toolDeleteLine`
- `toolDetectEncoding`
- `toolFixEncoding`
- `toolValidateLaTeX`
- `toolCreateBackup`

## 测试

运行测试以验证集成：
```bash
cd latex-translator
go build -o test_agent_tools.exe ./cmd/test_agent_tools
./test_agent_tools.exe
```

测试验证：
- ✓ LineEditor 功能
- ✓ EncodingHandler 功能
- ✓ LaTeXValidator 功能
- ✓ Agent 工具集成

## 下一步

Agent 现在已经具备了修复 arXiv 2501.17161 问题所需的所有工具：
1. 检测编码问题（detect_encoding）
2. 修复编码（fix_encoding）
3. 验证 LaTeX 语法（validate_latex）
4. 进行精确的行级编辑（replace_line）
5. 编译和测试（compile_latex）

Agent 将能够：
- 自动检测并修复编码问题（乱码中文）
- 使用 xelatex 编译中文文档
- 进行精确的语法修复
- 验证修复是否成功

## 状态

✅ **完成**: Agent 编辑工具集成
- 所有工具已实现并集成到 Agent
- 系统提示已更新以教导 Agent 如何使用工具
- 测试通过，验证集成成功

🎯 **准备就绪**: Agent 现在可以使用这些工具来修复 LaTeX 编译错误
