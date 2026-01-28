# 实现计划：PDF 翻译功能

## 概述

本计划将 PDF 翻译功能集成到现有的 LaTeX 翻译器项目中。采用增量开发方式，每个任务都建立在前一个任务的基础上，确保代码始终可运行。

## 任务

- [x] 1. 创建 PDF 翻译模块基础结构
  - [x] 1.1 创建 `internal/pdf` 目录和基础类型定义
    - 创建 `types.go` 定义 PDFInfo, TextBlock, TranslatedBlock, PDFStatus 等数据结构
    - 创建 PDFPhase 和 PDFErrorCode 枚举
    - _Requirements: 2.2, 2.3, 3.5, 5.5_
  
  - [ ]* 1.2 编写数据结构属性测试
    - **Property 2: 文本块数据完整性**
    - **Property 7: 状态有效性**
    - **Validates: Requirements 1.4, 2.2, 2.3, 3.5, 5.5**

- [x] 2. 实现翻译缓存组件
  - [x] 2.1 创建 `internal/pdf/cache.go` 实现 TranslationCache
    - 实现 ComputeHash 使用 SHA256 计算文本哈希
    - 实现 Get/Set 方法进行缓存读写
    - 实现 Load/Save 方法进行持久化
    - 实现 FilterCached 方法过滤已缓存文本块
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_
  
  - [ ]* 2.2 编写缓存属性测试
    - **Property 10: 缓存往返**
    - **Property 11: 缓存过滤正确性**
    - **Property 12: 哈希一致性**
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4**

- [x] 3. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 4. 实现 PDF 解析器
  - [x] 4.1 添加 pdfcpu 依赖并创建 `internal/pdf/parser.go`
    - 运行 `go get github.com/pdfcpu/pdfcpu`
    - 实现 GetPDFInfo 获取 PDF 基本信息（页数、文件大小）
    - 实现 IsTextPDF 检查 PDF 是否包含可提取文本
    - _Requirements: 1.1, 1.3, 1.4, 2.4_
  
  - [x] 4.2 实现 ExtractText 文本提取功能
    - 提取文本内容及位置信息（X, Y, Width, Height）
    - 识别文本块类型（paragraph, heading 等）
    - 保留数学公式和特殊符号
    - _Requirements: 2.1, 2.2, 2.3, 2.5_
  
  - [ ]* 4.3 编写 PDF 解析器属性测试
    - **Property 1: PDF 加载有效性**
    - **Property 3: 文本提取非空性**
    - **Property 4: 特殊字符保留**
    - **Validates: Requirements 1.1, 1.3, 2.1, 2.5**

- [x] 5. 实现批量翻译器
  - [x] 5.1 创建 `internal/pdf/batch_translator.go`
    - 实现 MergeBatches 将文本块合并为批次
    - 根据上下文窗口大小动态调整批次
    - 实现批次分隔符和结果解析逻辑
    - _Requirements: 3.1, 3.2_
  
  - [x] 5.2 实现 TranslateBatch 批量翻译功能
    - 调用 OpenAI API 进行批量翻译
    - 实现结果拆分和映射回原始文本块
    - 支持并发处理多个批次
    - _Requirements: 3.3, 3.6_
  
  - [x] 5.3 实现错误处理和重试逻辑
    - 实现 TranslateWithRetry 带重试的翻译
    - 批次失败时降级为单块翻译
    - _Requirements: 3.4, 7.3_
  
  - [ ]* 5.4 编写批量翻译器属性测试
    - **Property 5: 批次大小约束**
    - **Property 6: 翻译结果映射**
    - **Validates: Requirements 3.1, 3.2, 3.3**

- [x] 6. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 7. 实现 PDF 生成器
  - [x] 7.1 创建 `internal/pdf/generator.go`
    - 实现 GeneratePDF 生成翻译后的 PDF
    - 在原始 PDF 基础上覆盖翻译文本
    - 配置中文字体支持
    - _Requirements: 4.1, 4.2_
  
  - [x] 7.2 实现自适应布局
    - 实现 AdjustTextSize 自动调整字体大小
    - 处理中文文本超出原始文本框的情况
    - _Requirements: 4.3_
  
  - [ ]* 7.3 编写 PDF 生成器属性测试
    - **Property 8: PDF 生成有效性**
    - **Property 9: 布局保持**
    - **Validates: Requirements 4.1, 4.2, 4.3**

- [x] 8. 实现 PDF 翻译控制器
  - [x] 8.1 创建 `internal/pdf/translator.go` 主控制器
    - 实现 LoadPDF 加载 PDF 并提取文本
    - 实现 TranslatePDF 执行完整翻译流程
    - 实现 GetStatus 获取处理状态
    - 实现 CancelTranslation 取消翻译
    - _Requirements: 1.1, 3.5, 5.5_
  
  - [x] 8.2 集成缓存和进度保存
    - 翻译前检查缓存
    - 翻译后更新缓存
    - 支持中断后恢复进度
    - _Requirements: 6.1, 6.2, 7.6_

- [x] 9. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 10. 集成到主应用
  - [x] 10.1 修改 `app.go` 添加 PDF 翻译方法
    - 添加 LoadPDF, TranslatePDF, GetPDFStatus 等 Wails 绑定方法
    - 添加 PDF 文件选择对话框支持
    - _Requirements: 1.1, 5.1_
  
  - [x] 10.2 添加 PDF 翻译相关配置
    - 在 Config 中添加 PDF 翻译相关设置
    - 支持配置批次大小、并发数等参数
    - _Requirements: 3.2, 3.6_

- [x] 11. 实现前端界面
  - [x] 11.1 添加模式切换 UI
    - 在 `frontend/index.html` 添加 LaTeX/PDF 模式切换按钮
    - 添加 PDF 翻译模式的输入区域
    - _Requirements: 5.1_
  
  - [x] 11.2 实现 PDF 翻译交互逻辑
    - 在 `frontend/src/main.js` 添加 PDF 翻译相关函数
    - 实现文件选择和拖拽上传
    - 实现翻译进度显示
    - _Requirements: 1.1, 1.2, 3.5, 5.5_
  
  - [x] 11.3 实现双栏 PDF 预览
    - 左侧显示原始 PDF，右侧显示翻译后 PDF
    - 实现同步滚动功能
    - 添加下载翻译后 PDF 按钮
    - _Requirements: 5.2, 5.3, 5.6_

- [x] 12. 最终检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

## 备注

- 标记为 `*` 的任务为可选任务，可跳过以加快 MVP 开发
- 每个任务都引用了具体的需求以便追溯
- 检查点确保增量验证
- 属性测试验证通用正确性属性
- 单元测试验证特定示例和边界情况
