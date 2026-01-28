# 实现计划: LaTeX 翻译器

## 概述

本计划将 LaTeX 翻译器的设计分解为可执行的编码任务。采用增量开发方式，每个任务都建立在前一个任务的基础上，确保代码始终可运行。

## 任务

- [x] 1. 初始化项目结构和核心类型
  - [x] 1.1 使用 Wails CLI 初始化项目
    - 运行 `wails init -n latex-translator -t vanilla`
    - 配置 Go module
    - _Requirements: 项目基础设施_
  
  - [x] 1.2 创建核心数据类型和枚举
    - 创建 `internal/types/types.go`
    - 定义 Config, SourceInfo, SourceType, ProcessResult, Status, ProcessPhase
    - 定义 AppError, ErrorCode 错误类型
    - _Requirements: 数据模型_

- [x] 2. 实现配置管理模块
  - [x] 2.1 实现 ConfigManager
    - 创建 `internal/config/config.go`
    - 实现 Load(), Save(), GetAPIKey(), SetAPIKey() 方法
    - 支持从配置文件和环境变量读取
    - _Requirements: 6.1, 6.2_
  
  - [ ]* 2.2 编写 ConfigManager 属性测试
    - **Property 6: 配置加载往返**
    - **Validates: Requirements 6.1**

- [x] 3. 实现输入解析模块
  - [x] 3.1 实现输入类型识别
    - 创建 `internal/parser/input_parser.go`
    - 实现 ParseInput() 函数识别 URL、ArxivID、LocalZip 类型
    - 实现 arXiv ID 格式验证
    - _Requirements: 5.1, 5.2, 5.3, 5.4_
  
  - [ ]* 3.2 编写输入解析属性测试
    - **Property 5: 输入类型识别**
    - **Validates: Requirements 5.1, 5.2, 5.3**

- [x] 4. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 5. 实现源码下载模块
  - [x] 5.1 实现 SourceDownloader 基础结构
    - 创建 `internal/downloader/downloader.go`
    - 实现 HTTP 客户端配置
    - 实现 BuildArxivURL() 函数
    - _Requirements: 1.2_
  
  - [ ]* 5.2 编写 arXiv URL 构建属性测试
    - **Property 1: arXiv ID 到 URL 转换正确性**
    - **Validates: Requirements 1.2**
  
  - [x] 5.3 实现下载功能
    - 实现 DownloadFromURL() 方法
    - 实现 DownloadByID() 方法
    - 添加重试逻辑和错误处理
    - _Requirements: 1.1, 1.2, 1.4_
  
  - [x] 5.4 实现 Zip 解压功能
    - 实现 ExtractZip() 方法
    - 处理嵌套目录结构
    - _Requirements: 1.3_
  
  - [ ]* 5.5 编写 Zip 解压属性测试
    - **Property 2: Zip 解压完整性（往返属性）**
    - **Validates: Requirements 1.3**
  
  - [x] 5.6 实现主 tex 文件识别
    - 实现 FindMainTexFile() 方法
    - 搜索包含 \documentclass 的文件
    - _Requirements: 1.5_
  
  - [ ]* 5.7 编写主 tex 文件识别属性测试
    - **Property 3: 主 tex 文件识别**
    - **Validates: Requirements 1.5**

- [x] 6. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 7. 实现 LaTeX 编译模块
  - [x] 7.1 实现 LaTeXCompiler
    - 创建 `internal/compiler/compiler.go`
    - 实现 Compile() 方法调用 pdflatex
    - 实现 CompileWithXeLaTeX() 方法
    - 实现编译器自动选择逻辑（检测中文字符）
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_
  
  - [ ]* 7.2 编写编译器选择属性测试
    - **Property 7: 中文文档编译器选择**
    - **Validates: Requirements 2.5**

- [x] 8. 实现翻译模块
  - [x] 8.1 实现 TranslationEngine
    - 创建 `internal/translator/translator.go`
    - 实现 OpenAI API 客户端初始化
    - 实现 TranslateTeX() 方法
    - 实现文本分块处理（处理大文档）
    - _Requirements: 3.1, 3.2_
  
  - [x] 8.2 实现 LaTeX 命令保护逻辑
    - 实现 extractLaTeXCommands() 提取 LaTeX 命令
    - 实现翻译时保护 LaTeX 命令的逻辑
    - _Requirements: 3.2_
  
  - [ ]* 8.3 编写 LaTeX 命令保留属性测试
    - **Property 4: LaTeX 命令保留（翻译不变量）**
    - **Validates: Requirements 3.2**
  
  - [x] 8.4 实现 SyntaxValidator
    - 创建 `internal/validator/validator.go`
    - 实现 Validate() 方法检测语法错误
    - 实现 Fix() 方法调用 OpenAI 修正语法
    - _Requirements: 3.3, 3.4, 3.5_

- [x] 9. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 10. 实现 App 主控制器
  - [x] 10.1 实现 App 结构和初始化
    - 修改 `app.go`
    - 集成所有模块（ConfigManager, SourceDownloader, TranslationEngine, LaTeXCompiler, SyntaxValidator）
    - 实现 startup() 和 shutdown() 生命周期方法
    - _Requirements: 整体集成_
  
  - [x] 10.2 实现 ProcessSource 主流程
    - 实现完整的处理流程：解析输入 → 下载/解压 → 编译原始 → 翻译 → 验证修正 → 编译中文
    - 实现状态更新回调
    - _Requirements: 1.1-1.5, 2.1-2.5, 3.1-3.5_
  
  - [x] 10.3 实现状态管理和取消功能
    - 实现 GetStatus() 方法
    - 实现 CancelProcess() 方法
    - 使用 context 实现取消传播
    - _Requirements: 4.5_

- [x] 11. 实现前端界面
  - [x] 11.1 创建双栏 PDF 预览布局
    - 修改 `frontend/index.html`
    - 创建左右两栏 iframe 容器
    - 添加状态栏区域
    - _Requirements: 4.1, 4.2, 4.3_
  
  - [x] 11.2 实现前端交互逻辑
    - 创建 `frontend/src/main.js`
    - 实现输入表单（URL/ID/文件选择）
    - 实现与后端的 Wails 绑定调用
    - 实现状态显示更新
    - _Requirements: 4.5_
  
  - [x] 11.3 实现 PDF 加载和显示
    - 实现 PDF 文件加载到 iframe
    - 实现加载状态指示
    - _Requirements: 4.2, 4.3_

- [x] 12. 实现命令行支持
  - [x] 12.1 添加命令行参数解析
    - 修改 `main.go`
    - 使用 flag 包解析命令行参数
    - 支持 --url, --id, --file 参数
    - 实现帮助信息显示
    - _Requirements: 5.1, 5.2, 5.3, 5.4_

- [x] 13. 实现错误处理和日志
  - [x] 13.1 实现日志模块
    - 创建 `internal/logger/logger.go`
    - 实现 Logger 接口
    - 配置日志文件输出
    - _Requirements: 7.4_
  
  - [x] 13.2 集成错误处理
    - 在所有模块中添加错误日志记录
    - 实现用户友好的错误消息转换
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

- [x] 14. 最终检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 15. 集成测试和完善
  - [x] 15.1 编写集成测试
    - 测试完整的处理流程（使用模拟数据）
    - 测试错误场景处理
    - _Requirements: 全部_
  
  - [x] 15.2 添加 README 和使用说明
    - 创建 README.md
    - 说明安装依赖（LaTeX 编译器）
    - 说明配置 OpenAI API 密钥
    - 说明使用方法
    - _Requirements: 文档_

## 备注

- 标记 `*` 的任务为可选任务，可跳过以加快 MVP 开发
- 每个任务都引用了具体的需求以便追溯
- 检查点确保增量验证
- 属性测试验证通用正确性属性
- 单元测试验证特定示例和边界情况
