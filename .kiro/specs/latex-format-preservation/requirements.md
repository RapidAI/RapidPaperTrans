# Requirements Document

## Introduction

本功能旨在解决LLM在翻译LaTeX内容时修改格式的问题。当前系统在翻译arXiv论文时，LLM经常会：
- 删除或修改LaTeX命令
- 合并应该分开的行
- 丢失大括号或环境标签
- 修改表格结构
- 改变注释格式

这些问题导致翻译后的LaTeX文档无法编译，需要大量人工修复。本功能将通过多层次的保护机制来最大程度地保持LaTeX格式的完整性。

## Glossary

- **Format_Protector**: 格式保护器，负责在翻译前识别和保护LaTeX结构
- **Placeholder_System**: 占位符系统，用于将LaTeX命令替换为安全的占位符
- **Structure_Validator**: 结构验证器，验证翻译后的LaTeX结构是否完整
- **Format_Restorer**: 格式恢复器，负责在翻译后恢复原始LaTeX格式
- **Chunk_Translator**: 分块翻译器，将内容分成小块进行翻译
- **Environment**: LaTeX环境，如 `\begin{...}` 和 `\end{...}` 包围的内容
- **Brace_Balance**: 大括号平衡，指开括号和闭括号数量相等

## Requirements

### Requirement 1: 增强的LaTeX命令保护

**User Story:** As a translator, I want LaTeX commands to be protected during translation, so that the document structure remains intact after translation.

#### Acceptance Criteria

1. WHEN the Format_Protector receives LaTeX content, THE Format_Protector SHALL identify all LaTeX commands including `\begin`, `\end`, `\section`, `\cite`, `\ref`, `\label`, and custom commands
2. WHEN a LaTeX command is identified, THE Placeholder_System SHALL replace it with a unique placeholder in the format `<<<LATEX_CMD_N>>>`
3. WHEN mathematical content is detected (inline `$...$` or display `\[...\]`), THE Format_Protector SHALL protect the entire mathematical expression as a single unit
4. WHEN a table environment is detected, THE Format_Protector SHALL protect the entire table structure including `\multirow`, `\multicolumn`, and cell separators
5. IF a placeholder is lost during translation, THEN THE Format_Restorer SHALL attempt to recover it by analyzing the context

### Requirement 2: 行结构保护

**User Story:** As a translator, I want line structure to be preserved during translation, so that LaTeX environments remain properly formatted.

#### Acceptance Criteria

1. WHEN translating content, THE Chunk_Translator SHALL preserve the line count between original and translated content
2. WHEN a `\begin{...}` command is detected, THE Format_Protector SHALL ensure it remains on its own line
3. WHEN a `\end{...}` command is detected, THE Format_Protector SHALL ensure it remains on its own line
4. WHEN multiple `\item` commands exist, THE Format_Protector SHALL ensure each `\item` remains on a separate line
5. IF the LLM merges lines during translation, THEN THE Format_Restorer SHALL split them back to match the original structure

### Requirement 3: 环境匹配验证

**User Story:** As a translator, I want environment tags to be validated after translation, so that all `\begin` tags have matching `\end` tags.

#### Acceptance Criteria

1. WHEN translation is complete, THE Structure_Validator SHALL count all `\begin{env}` and `\end{env}` pairs
2. WHEN a mismatch is detected between begin and end counts, THE Structure_Validator SHALL report the specific environment name and count difference
3. IF an `\end{...}` tag is missing, THEN THE Format_Restorer SHALL insert it at the appropriate position based on the original document structure
4. IF an extra `\begin{...}` tag is detected, THEN THE Structure_Validator SHALL log a warning and attempt to identify the source of the error
5. WHEN validating nested environments, THE Structure_Validator SHALL verify proper nesting order

### Requirement 4: 大括号平衡验证

**User Story:** As a translator, I want brace balance to be maintained after translation, so that LaTeX commands with arguments remain valid.

#### Acceptance Criteria

1. WHEN translation is complete, THE Structure_Validator SHALL count opening and closing braces
2. WHEN a brace imbalance is detected, THE Structure_Validator SHALL identify the approximate location of the imbalance
3. IF closing braces are missing in `\multirow` or `\multicolumn` commands, THEN THE Format_Restorer SHALL add the missing braces
4. WHEN nested braces exist, THE Structure_Validator SHALL verify each nesting level is properly closed
5. IF the brace count differs from the original, THEN THE Format_Restorer SHALL attempt to restore the original balance

### Requirement 5: 注释格式保护

**User Story:** As a translator, I want LaTeX comments to be preserved correctly, so that commented code remains commented after translation.

#### Acceptance Criteria

1. WHEN a line starts with `%`, THE Format_Protector SHALL mark it as a comment line
2. WHEN translating comment content, THE Chunk_Translator SHALL translate the text after `%` while preserving the `%` symbol
3. IF the LLM removes the `%` symbol from a comment line, THEN THE Format_Restorer SHALL restore it
4. WHEN a `\begin{...}` or `\end{...}` is inside a comment, THE Format_Protector SHALL ensure it remains commented
5. IF the LLM uncomments a commented environment tag, THEN THE Format_Restorer SHALL re-comment it based on the original

### Requirement 6: 分块翻译优化

**User Story:** As a translator, I want content to be split into optimal chunks, so that translation quality is maximized while preserving structure.

#### Acceptance Criteria

1. WHEN splitting content into chunks, THE Chunk_Translator SHALL respect environment boundaries
2. WHEN a chunk boundary falls within an environment, THE Chunk_Translator SHALL extend the chunk to include the complete environment
3. WHEN a table is detected, THE Chunk_Translator SHALL keep the entire table in a single chunk if possible
4. IF a chunk exceeds the maximum size, THEN THE Chunk_Translator SHALL split at the nearest safe boundary (paragraph break or section)
5. WHEN reassembling chunks, THE Chunk_Translator SHALL verify no content is duplicated or lost

### Requirement 7: 翻译后验证

**User Story:** As a translator, I want translated content to be validated, so that format errors are detected before compilation.

#### Acceptance Criteria

1. WHEN translation is complete, THE Structure_Validator SHALL compare the structure of original and translated content
2. WHEN the translated content has significantly different length ratio (< 0.3 or > 3.0), THE Structure_Validator SHALL flag it as potentially problematic
3. WHEN required LaTeX patterns (like `\documentclass`) are missing in translation, THE Structure_Validator SHALL report an error
4. IF validation fails, THEN THE Structure_Validator SHALL provide specific error messages indicating what went wrong
5. WHEN Chinese character ratio is too low (< 5% for substantial content), THE Structure_Validator SHALL warn that translation may have failed

### Requirement 8: Prompt工程优化

**User Story:** As a translator, I want the LLM prompt to clearly instruct format preservation, so that the LLM is less likely to modify LaTeX structure.

#### Acceptance Criteria

1. THE Chunk_Translator SHALL include explicit instructions in the system prompt about preserving LaTeX commands
2. THE Chunk_Translator SHALL include examples of correct format preservation in the prompt
3. WHEN sending content to LLM, THE Chunk_Translator SHALL include a reminder about line structure preservation
4. THE Chunk_Translator SHALL instruct the LLM to output the same number of lines as input
5. IF the LLM response contains format violations, THEN THE Chunk_Translator SHALL log the violation type for analysis

### Requirement 9: 格式恢复机制

**User Story:** As a translator, I want automatic format recovery, so that common LLM format errors are fixed without manual intervention.

#### Acceptance Criteria

1. WHEN a translated chunk is received, THE Format_Restorer SHALL apply post-processing fixes
2. WHEN `\end{env}` and `\item` are on the same line, THE Format_Restorer SHALL split them onto separate lines
3. WHEN `\caption{...}` is followed by `\begin{tabular}` on the same line, THE Format_Restorer SHALL insert a line break
4. WHEN table row separators (`\\`) are followed by `\midrule` on the same line, THE Format_Restorer SHALL insert a line break
5. WHEN comparing with original content, THE Format_Restorer SHALL use the original as reference to fix structural issues
