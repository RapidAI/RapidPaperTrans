# 需求文档

## 简介

LaTeX 翻译器是一个桌面应用程序，用于将英文 LaTeX 文档翻译成中文并自动编译生成 PDF。该工具采用 Go 语言和 Wails 框架开发，提供双栏预览界面，支持从 arXiv 下载源码或导入本地 zip 文件。

## 术语表

- **LaTeX_Translator**: 主应用程序系统
- **PDF_Viewer**: PDF 预览组件
- **Translation_Engine**: 使用 OpenAI API 的翻译引擎
- **LaTeX_Compiler**: LaTeX 编译器封装组件（pdflatex/xelatex）
- **Source_Downloader**: arXiv 源码下载组件
- **Syntax_Validator**: LaTeX 语法检测和修正组件

## 需求

### 需求 1：源码获取

**用户故事：** 作为用户，我希望能够通过多种方式导入 LaTeX 源码，以便灵活地处理不同来源的文档。

#### 验收标准

1. WHEN 用户提供 arXiv LaTeX 源码 URL THEN LaTeX_Translator SHALL 自动下载并解压源码到临时目录
2. WHEN 用户提供 arXiv ID（如 2301.00001）THEN LaTeX_Translator SHALL 构建下载链接并获取源码
3. WHEN 用户提供本地 zip 文件路径 THEN LaTeX_Translator SHALL 解压该文件并加载 LaTeX 源码
4. IF 下载或解压失败 THEN LaTeX_Translator SHALL 显示明确的错误信息并提示用户重试
5. WHEN 源码成功加载 THEN LaTeX_Translator SHALL 自动识别主 tex 文件

### 需求 2：LaTeX 编译

**用户故事：** 作为用户，我希望系统能够自动编译 LaTeX 文档生成 PDF，以便我可以预览原始和翻译后的文档。

#### 验收标准

1. WHEN 源码加载完成 THEN LaTeX_Compiler SHALL 自动编译原始英文文档生成 PDF
2. WHEN 翻译完成 THEN LaTeX_Compiler SHALL 自动编译中文文档生成 PDF
3. THE LaTeX_Compiler SHALL 支持 pdflatex 和 xelatex 两种编译器
4. IF 编译失败 THEN LaTeX_Compiler SHALL 返回详细的错误日志
5. WHEN 编译中文文档时 THEN LaTeX_Compiler SHALL 使用 xelatex 以支持中文字体

### 需求 3：翻译功能

**用户故事：** 作为用户，我希望系统能够智能翻译 LaTeX 文档内容，同时保持 LaTeX 语法的完整性。

#### 验收标准

1. WHEN 用户触发翻译 THEN Translation_Engine SHALL 调用 OpenAI API 将英文内容翻译为中文
2. WHILE 翻译进行中 THEN Translation_Engine SHALL 保留所有 LaTeX 命令和数学公式不变
3. WHEN 翻译完成 THEN Syntax_Validator SHALL 检测翻译后的 LaTeX 语法正确性
4. IF 检测到语法错误 THEN Syntax_Validator SHALL 调用 OpenAI API 自动修正语法
5. WHEN 翻译和修正完成 THEN LaTeX_Translator SHALL 保存翻译后的 tex 文件

### 需求 4：用户界面

**用户故事：** 作为用户，我希望能够在双栏界面中同时预览原始和翻译后的 PDF，以便对比查看翻译效果。

#### 验收标准

1. THE PDF_Viewer SHALL 以双栏布局显示，左侧为原始英文 PDF，右侧为翻译后的中文 PDF
2. WHEN 原始 PDF 编译完成 THEN PDF_Viewer SHALL 在左侧 iframe 中显示该 PDF
3. WHEN 中文 PDF 编译完成 THEN PDF_Viewer SHALL 在右侧显示该 PDF
4. WHEN 用户滚动任一侧 PDF THEN PDF_Viewer SHALL 同步滚动另一侧（可选功能）
5. THE LaTeX_Translator SHALL 显示当前处理状态（下载中、翻译中、编译中等）

### 需求 5：命令行支持

**用户故事：** 作为用户，我希望能够通过命令行参数启动应用并指定输入源，以便实现自动化工作流。

#### 验收标准

1. WHEN 用户通过命令行提供 arXiv URL 参数 THEN LaTeX_Translator SHALL 启动并自动开始处理
2. WHEN 用户通过命令行提供 arXiv ID 参数 THEN LaTeX_Translator SHALL 启动并自动开始处理
3. WHEN 用户通过命令行提供本地 zip 路径参数 THEN LaTeX_Translator SHALL 启动并自动开始处理
4. IF 命令行参数无效 THEN LaTeX_Translator SHALL 显示帮助信息并退出

### 需求 6：配置管理

**用户故事：** 作为用户，我希望能够配置 OpenAI API 密钥和其他设置，以便系统能够正常工作。

#### 验收标准

1. THE LaTeX_Translator SHALL 支持通过配置文件设置 OpenAI API 密钥
2. THE LaTeX_Translator SHALL 支持通过环境变量设置 OpenAI API 密钥
3. WHEN API 密钥未配置 THEN LaTeX_Translator SHALL 提示用户配置密钥
4. THE LaTeX_Translator SHALL 允许用户选择默认的 LaTeX 编译器（pdflatex 或 xelatex）

### 需求 7：错误处理

**用户故事：** 作为用户，我希望系统能够优雅地处理各种错误情况，以便我能够了解问题并采取相应措施。

#### 验收标准

1. IF 网络连接失败 THEN LaTeX_Translator SHALL 显示网络错误提示并允许重试
2. IF OpenAI API 调用失败 THEN Translation_Engine SHALL 显示 API 错误详情并允许重试
3. IF LaTeX 编译失败 THEN LaTeX_Compiler SHALL 显示编译错误日志
4. WHEN 发生任何错误 THEN LaTeX_Translator SHALL 记录错误日志以便调试
