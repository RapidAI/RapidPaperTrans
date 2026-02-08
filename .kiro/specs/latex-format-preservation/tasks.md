# Implementation Plan: LaTeX Format Preservation

## Overview

本实现计划将LaTeX格式保护功能分解为可执行的编码任务。实现采用增量方式，每个任务都建立在前一个任务的基础上，确保代码始终处于可工作状态。

## Tasks

- [x] 1. 增强Format_Protector组件
  - [x] 1.1 扩展LaTeX结构识别功能
    - 在 `internal/translator/preprocessor.go` 中添加 `IdentifyStructures` 函数
    - 支持识别命令、环境、数学、表格、注释等结构类型
    - 添加 `LaTeXStructure` 和 `StructureType` 类型定义
    - _Requirements: 1.1, 1.3, 1.4_
  
  - [x]* 1.2 编写结构识别的属性测试
    - **Property 2: Math Environment Protection**
    - **Property 3: Table Structure Protection**
    - **Validates: Requirements 1.3, 1.4**
  
  - [x] 1.3 增强数学环境保护
    - 修改 `ProtectLaTeXCommands` 函数以完整保护数学环境
    - 支持 `$...$`, `$$...$$`, `\[...\]`, `\(...\)` 格式
    - 支持 `equation`, `align`, `gather` 等环境
    - _Requirements: 1.3_
  
  - [x] 1.4 增强表格结构保护
    - 添加 `ProtectTableStructures` 函数
    - 保护 `\multirow`, `\multicolumn`, `&`, `\\` 等表格元素
    - 保护 `tabular`, `table`, `longtable` 等环境
    - _Requirements: 1.4_

- [x] 2. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 3. 增强Placeholder_System组件
  - [x] 3.1 改进占位符生成和管理
    - 在 `internal/translator/translator.go` 中添加 `PlaceholderSystem` 结构
    - 实现 `GeneratePlaceholder`, `StorePlaceholder`, `RestorePlaceholder` 方法
    - 添加占位符验证功能 `ValidatePlaceholders`
    - _Requirements: 1.2, 1.5_
  
  - [x]* 3.2 编写占位符往返属性测试
    - **Property 1: Placeholder Round-Trip**
    - **Validates: Requirements 1.2**
  
  - [x] 3.3 实现占位符丢失恢复机制
    - 增强 `recoverMissingPlaceholders` 函数
    - 基于上下文分析尝试恢复丢失的占位符
    - 添加恢复日志记录
    - _Requirements: 1.5_

- [x] 4. 增强Structure_Validator组件
  - [x] 4.1 实现环境匹配验证
    - 在 `internal/translator/validator.go` 中添加 `ValidateEnvironments` 函数
    - 实现 `countEnvironments` 增强版，支持嵌套检测
    - 添加 `EnvironmentValidation` 和 `EnvMismatch` 类型
    - _Requirements: 3.1, 3.2, 3.5_
  
  - [x]* 4.2 编写环境验证属性测试
    - **Property 6: Environment Counting Accuracy**
    - **Property 7: Environment Mismatch Detection**
    - **Property 9: Nested Environment Validation**
    - **Validates: Requirements 3.1, 3.2, 3.5**
  
  - [x] 4.3 实现大括号平衡验证
    - 添加 `ValidateBraces` 函数
    - 实现嵌套大括号计数
    - 添加错误位置定位功能
    - _Requirements: 4.1, 4.2, 4.4_
  
  - [x]* 4.4 编写大括号验证属性测试
    - **Property 10: Brace Balance Validation**
    - **Property 11: Brace Imbalance Location**
    - **Validates: Requirements 4.1, 4.2, 4.4**
  
  - [x] 4.5 实现注释格式验证
    - 添加 `ValidateComments` 函数
    - 检测注释行的 `%` 符号是否保留
    - 检测被注释的环境标签是否保持注释状态
    - _Requirements: 5.1, 5.4_

- [x] 5. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 6. 增强Format_Restorer组件
  - [x] 6.1 增强行结构恢复
    - 在 `internal/translator/postprocessor.go` 中增强 `fixLineStructure` 函数
    - 添加基于原始内容的行结构比较
    - 实现 `RestoreLineStructure` 函数
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_
  
  - [x]* 6.2 编写行结构恢复属性测试
    - **Property 4: Line Count Preservation**
    - **Property 5: Line Structure Preservation**
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5**
  
  - [x] 6.3 增强环境标签恢复
    - 实现 `RestoreEnvironments` 函数
    - 增强 `fixEnvironmentsByReference` 函数
    - 实现缺失 `\end{...}` 标签的自动插入
    - _Requirements: 3.3, 3.4_
  
  - [x]* 6.4 编写环境恢复属性测试
    - **Property 8: Missing End Tag Restoration**
    - **Validates: Requirements 3.3**
  
  - [x] 6.5 增强大括号恢复
    - 实现 `RestoreBraces` 函数
    - 增强 `fixMultirowMulticolumnBraces` 函数
    - 添加通用大括号平衡恢复
    - _Requirements: 4.3, 4.5_
  
  - [x]* 6.6 编写大括号恢复属性测试
    - **Property 12: Multirow/Multicolumn Brace Fix**
    - **Property 13: Brace Restoration**
    - **Validates: Requirements 4.3, 4.5**
  
  - [x] 6.7 实现注释恢复
    - 添加 `RestoreComments` 函数
    - 恢复被删除的 `%` 符号
    - 重新注释被取消注释的环境标签
    - _Requirements: 5.2, 5.3, 5.5_
  
  - [x]* 6.8 编写注释恢复属性测试
    - **Property 14: Comment Line Protection**
    - **Property 15: Comment Symbol Preservation**
    - **Property 16: Comment Restoration**
    - **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5**

- [x] 7. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 8. 增强Chunk_Translator组件
  - [x] 8.1 实现智能分块
    - 在 `internal/translator/translator.go` 中增强 `splitIntoChunks` 函数
    - 添加环境边界检测
    - 实现安全边界查找
    - _Requirements: 6.1, 6.2, 6.4_
  
  - [x]* 8.2 编写分块属性测试
    - **Property 17: Chunk Boundary Respect**
    - **Property 19: Safe Boundary Splitting**
    - **Validates: Requirements 6.1, 6.2, 6.4**
  
  - [x] 8.3 实现表格分块优化
    - 添加表格检测和完整性保护
    - 确保表格不被分割
    - _Requirements: 6.3_
  
  - [x]* 8.4 编写表格分块属性测试
    - **Property 18: Table Chunking**
    - **Validates: Requirements 6.3**
  
  - [x] 8.5 实现分块重组验证
    - 添加重组后的内容完整性验证
    - 检测重复或丢失的内容
    - _Requirements: 6.5_
  
  - [x]* 8.6 编写分块重组属性测试
    - **Property 20: Chunk Reassembly Round-Trip**
    - **Validates: Requirements 6.5**

- [x] 9. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 10. 优化Prompt工程
  - [x] 10.1 增强系统提示词
    - 修改 `buildSystemPromptWithProtection` 函数
    - 添加更明确的格式保护指令
    - 添加行结构保护指令
    - _Requirements: 8.1, 8.3, 8.4_
  
  - [x] 10.2 添加示例到提示词
    - 在提示词中添加正确格式保护的示例
    - 添加常见错误和正确处理方式的对比
    - _Requirements: 8.2_
  
  - [x] 10.3 实现格式违规日志
    - 添加翻译结果的格式违规检测
    - 记录违规类型用于分析
    - _Requirements: 8.5_

- [x] 11. 实现完整验证流程
  - [x] 11.1 实现结构比较
    - 添加 `CompareStructure` 函数
    - 比较原始和翻译后的文档结构
    - _Requirements: 7.1_
  
  - [x]* 11.2 编写结构比较属性测试
    - **Property 21: Structure Comparison**
    - **Validates: Requirements 7.1**
  
  - [x] 11.3 增强长度比验证
    - 增强 `ValidateTranslation` 函数中的长度比检查
    - 添加更详细的错误信息
    - _Requirements: 7.2_
  
  - [x]* 11.4 编写长度比验证属性测试
    - **Property 22: Length Ratio Validation**
    - **Validates: Requirements 7.2**
  
  - [x] 11.5 增强模式验证
    - 添加必需模式的验证
    - 添加中文比例验证
    - _Requirements: 7.3, 7.5_
  
  - [x]* 11.6 编写模式验证属性测试
    - **Property 23: Required Pattern Validation**
    - **Property 24: Chinese Ratio Validation**
    - **Validates: Requirements 7.3, 7.5**

- [x] 12. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 13. 集成和优化
  - [x] 13.1 集成所有组件
    - 在 `TranslateTeXWithProgress` 函数中集成所有新组件
    - 确保流水线正确执行
    - _Requirements: 9.1, 9.5_
  
  - [x]* 13.2 编写集成属性测试
    - **Property 25: Post-Processing Application**
    - **Property 27: Reference-Based Fixing**
    - **Validates: Requirements 9.1, 9.5**
  
  - [x] 13.3 实现行分隔修复集成
    - 集成所有行分隔修复规则
    - 确保 `\end{env}` + `\item`, `\caption` + `\begin{tabular}`, `\\` + `\midrule` 等情况被正确处理
    - _Requirements: 9.2, 9.3, 9.4_
  
  - [x]* 13.4 编写行分隔修复属性测试
    - **Property 26: Line Break Insertion**
    - **Validates: Requirements 9.2, 9.3, 9.4**
  
  - [x] 13.5 添加配置支持
    - 实现 `FormatPreservationConfig` 结构
    - 添加配置加载和默认值
    - _Requirements: All_

- [x] 14. Final Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

## Notes

- 标记为 `*` 的任务是可选的属性测试任务，可以跳过以加快MVP开发
- 每个任务都引用了具体的需求以确保可追溯性
- Checkpoint任务用于确保增量开发的稳定性
- 属性测试验证通用正确性属性，应使用 `gopter` 或 `testing/quick` 库

## Summary of Remaining Work

### Required Tasks (Non-Optional)
✅ **All required tasks are complete!**

All core functionality has been implemented:
- Format_Protector: LaTeX structure identification and protection
- Placeholder_System: Placeholder generation, storage, and recovery
- Structure_Validator: Environment, brace, and comment validation
- Format_Restorer: Line structure, environment, brace, and comment restoration
- Chunk_Translator: Smart chunking with environment boundary respect
- Prompt Engineering: Enhanced system prompts with format preservation instructions
- Configuration: FormatPreservationConfig with validation and persistence

### Optional Property-Based Tests
✅ **All optional property-based tests are complete!**

All 27 property tests have been implemented in `internal/translator/property_test.go`:
- Properties 1-3: Placeholder and structure protection
- Properties 4-5: Line structure preservation
- Properties 6-9: Environment validation
- Properties 10-13: Brace validation and restoration
- Properties 14-16: Comment handling
- Properties 17-20: Chunking
- Properties 21-24: Validation
- Properties 25-27: Integration
