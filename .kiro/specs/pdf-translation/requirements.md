# 需求文档

## 简介

PDF 翻译功能是 LaTeX 翻译器的扩展模块，用于直接将英文 PDF 文档翻译成中文 PDF。该功能使用大语言模型进行翻译，采用批量合并翻译策略以提升翻译速度。用户界面采用双栏布局，左侧显示原始 PDF 文档，右侧显示翻译后的 PDF 文档。

## 术语表

- **PDF_Translator**: PDF 翻译主模块
- **PDF_Parser**: PDF 文本提取组件
- **Batch_Translator**: 批量合并翻译引擎
- **PDF_Generator**: 翻译后 PDF 生成组件
- **Translation_Cache**: 翻译缓存组件
- **Text_Block**: PDF 中的文本块单元

## 需求

### 需求 1：PDF 文件导入

**用户故事：** 作为用户，我希望能够导入本地 PDF 文件进行翻译，以便处理各种来源的英文 PDF 文档。

#### 验收标准

1. WHEN 用户选择本地 PDF 文件 THEN PDF_Translator SHALL 加载该文件并显示在左侧预览区域
2. WHEN 用户拖拽 PDF 文件到应用窗口 THEN PDF_Translator SHALL 自动识别并加载该文件
3. IF PDF 文件无法读取或格式无效 THEN PDF_Translator SHALL 显示明确的错误信息
4. WHEN PDF 文件加载成功 THEN PDF_Translator SHALL 显示文件基本信息（页数、文件大小）

### 需求 2：PDF 文本提取

**用户故事：** 作为用户，我希望系统能够准确提取 PDF 中的文本内容，以便进行翻译处理。

#### 验收标准

1. WHEN PDF 文件加载完成 THEN PDF_Parser SHALL 提取所有可识别的文本内容
2. WHILE 提取文本时 THEN PDF_Parser SHALL 保留文本的位置信息和格式属性
3. WHEN 提取完成 THEN PDF_Parser SHALL 将文本按逻辑块（段落、标题等）进行分组
4. IF PDF 包含扫描图像而非可选文本 THEN PDF_Parser SHALL 提示用户该文件不支持直接翻译
5. WHEN 提取文本时 THEN PDF_Parser SHALL 识别并保留数学公式和特殊符号

### 需求 3：批量合并翻译

**用户故事：** 作为用户，我希望系统能够高效地翻译大量文本，以便快速获得翻译结果。

#### 验收标准

1. WHEN 用户触发翻译 THEN Batch_Translator SHALL 将多个文本块合并为批次进行翻译
2. WHILE 翻译进行中 THEN Batch_Translator SHALL 根据 LLM 上下文窗口大小动态调整批次大小
3. WHEN 单个批次翻译完成 THEN Batch_Translator SHALL 正确拆分结果并映射回原始文本块
4. IF 批次翻译失败 THEN Batch_Translator SHALL 自动降级为单块翻译模式重试
5. WHEN 翻译进行中 THEN PDF_Translator SHALL 显示翻译进度（已完成块数/总块数）
6. WHILE 翻译进行中 THEN Batch_Translator SHALL 支持并发处理多个批次以提升速度

### 需求 4：翻译后 PDF 生成

**用户故事：** 作为用户，我希望系统能够生成保持原始布局的中文 PDF，以便对比阅读。

#### 验收标准

1. WHEN 翻译完成 THEN PDF_Generator SHALL 生成包含中文翻译的新 PDF 文件
2. WHILE 生成 PDF 时 THEN PDF_Generator SHALL 尽可能保持原始文档的布局和格式
3. WHEN 中文文本超出原始文本框 THEN PDF_Generator SHALL 自动调整字体大小或换行
4. WHEN PDF 生成完成 THEN PDF_Translator SHALL 在右侧预览区域显示翻译后的 PDF
5. IF PDF 生成失败 THEN PDF_Generator SHALL 返回详细的错误信息

### 需求 5：用户界面

**用户故事：** 作为用户，我希望能够在双栏界面中同时预览原始和翻译后的 PDF，以便对比查看翻译效果。

#### 验收标准

1. THE PDF_Translator SHALL 提供 PDF 翻译模式入口，与现有 LaTeX 翻译模式并存
2. WHEN 进入 PDF 翻译模式 THEN PDF_Translator SHALL 以双栏布局显示，左侧为原始 PDF，右侧为翻译后 PDF
3. WHEN 用户滚动任一侧 PDF THEN PDF_Translator SHALL 同步滚动另一侧到对应位置
4. WHEN 用户点击左侧 PDF 中的文本 THEN PDF_Translator SHALL 高亮显示右侧对应的翻译文本
5. THE PDF_Translator SHALL 显示当前处理状态（加载中、提取中、翻译中、生成中等）
6. WHEN 翻译完成 THEN PDF_Translator SHALL 提供下载翻译后 PDF 的功能

### 需求 6：翻译缓存

**用户故事：** 作为用户，我希望系统能够缓存翻译结果，以便避免重复翻译相同内容。

#### 验收标准

1. WHEN 文本块翻译完成 THEN Translation_Cache SHALL 缓存该翻译结果
2. WHEN 遇到已缓存的文本块 THEN Batch_Translator SHALL 直接使用缓存结果而非重新翻译
3. THE Translation_Cache SHALL 使用文本内容的哈希值作为缓存键
4. WHEN 应用退出时 THEN Translation_Cache SHALL 持久化缓存到本地存储
5. WHEN 应用启动时 THEN Translation_Cache SHALL 加载之前的缓存数据

### 需求 7：错误处理

**用户故事：** 作为用户，我希望系统能够优雅地处理各种错误情况，以便我能够了解问题并采取相应措施。

#### 验收标准

1. IF PDF 文件损坏或加密 THEN PDF_Translator SHALL 显示明确的错误提示
2. IF 文本提取失败 THEN PDF_Parser SHALL 返回部分结果并标记失败的页面
3. IF LLM API 调用失败 THEN Batch_Translator SHALL 自动重试并在多次失败后提示用户
4. IF PDF 生成失败 THEN PDF_Generator SHALL 提供错误详情并允许重试
5. WHEN 发生任何错误 THEN PDF_Translator SHALL 记录错误日志以便调试
6. IF 翻译过程中断 THEN PDF_Translator SHALL 保存已完成的翻译进度以便恢复

