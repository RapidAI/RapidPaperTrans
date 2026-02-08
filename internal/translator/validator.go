// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"latex-translator/internal/logger"
)

// EnvironmentValidation 环境验证结果
type EnvironmentValidation struct {
	IsValid      bool                   // 验证是否通过
	Environments map[string]EnvCount    // 环境名 -> 计数
	Mismatches   []EnvMismatch          // 不匹配的环境
	NestingValid bool                   // 嵌套是否正确
	NestingError string                 // 嵌套错误描述
}

// EnvCount 环境计数
type EnvCount struct {
	BeginCount int // \begin{env} 的数量
	EndCount   int // \end{env} 的数量
}

// EnvMismatch 环境不匹配信息
type EnvMismatch struct {
	EnvName    string // 环境名称
	Expected   int    // 期望的数量 (begin count)
	Actual     int    // 实际的数量 (end count)
	Difference int    // 差异 (begin - end)
}

// envPosition 记录环境标签的位置信息
type envPosition struct {
	envName  string // 环境名称
	isBegin  bool   // 是否是 \begin
	position int    // 在内容中的位置
	line     int    // 行号
}

// ValidateEnvironments 验证LaTeX内容中的环境匹配
// 实现 Requirements 3.1, 3.2, 3.5
func ValidateEnvironments(content string) *EnvironmentValidation {
	result := &EnvironmentValidation{
		IsValid:      true,
		Environments: make(map[string]EnvCount),
		Mismatches:   []EnvMismatch{},
		NestingValid: true,
	}

	// 统计所有环境的 begin 和 end 数量
	countEnvironmentsForValidation(content, result)

	// 检查每个环境的匹配情况
	checkEnvironmentMismatches(result)

	// 验证嵌套顺序
	validateNestingOrder(content, result)

	return result
}

// countEnvironmentsForValidation 统计内容中所有环境的 begin 和 end 数量
// 这是增强版的环境计数函数，支持嵌套检测，实现 Requirement 3.1
func countEnvironmentsForValidation(content string, result *EnvironmentValidation) {
	// 匹配 \begin{envname} 和 \end{envname}
	beginRegex := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endRegex := regexp.MustCompile(`\\end\{([^}]+)\}`)

	// 统计 \begin{...}
	beginMatches := beginRegex.FindAllStringSubmatch(content, -1)
	for _, match := range beginMatches {
		if len(match) > 1 {
			envName := match[1]
			count := result.Environments[envName]
			count.BeginCount++
			result.Environments[envName] = count
		}
	}

	// 统计 \end{...}
	endMatches := endRegex.FindAllStringSubmatch(content, -1)
	for _, match := range endMatches {
		if len(match) > 1 {
			envName := match[1]
			count := result.Environments[envName]
			count.EndCount++
			result.Environments[envName] = count
		}
	}

	logger.Debug("environment counts",
		logger.Int("totalEnvironments", len(result.Environments)))
}

// checkEnvironmentMismatches 检查环境不匹配情况
// 实现 Requirement 3.2
func checkEnvironmentMismatches(result *EnvironmentValidation) {
	for envName, count := range result.Environments {
		if count.BeginCount != count.EndCount {
			mismatch := EnvMismatch{
				EnvName:    envName,
				Expected:   count.BeginCount,
				Actual:     count.EndCount,
				Difference: count.BeginCount - count.EndCount,
			}
			result.Mismatches = append(result.Mismatches, mismatch)
			result.IsValid = false

			logger.Warn("environment mismatch detected",
				logger.String("envName", envName),
				logger.Int("beginCount", count.BeginCount),
				logger.Int("endCount", count.EndCount),
				logger.Int("difference", mismatch.Difference))
		}
	}

	// 按环境名排序，确保输出稳定
	sort.Slice(result.Mismatches, func(i, j int) bool {
		return result.Mismatches[i].EnvName < result.Mismatches[j].EnvName
	})
}

// validateNestingOrder 验证环境的嵌套顺序是否正确
// 实现 Requirement 3.5
func validateNestingOrder(content string, result *EnvironmentValidation) {
	positions := extractEnvPositions(content)

	// 使用栈来验证嵌套顺序
	stack := []envPosition{}

	for _, pos := range positions {
		if pos.isBegin {
			// 遇到 \begin，压入栈
			stack = append(stack, pos)
		} else {
			// 遇到 \end，检查栈顶是否匹配
			if len(stack) == 0 {
				// 栈为空但遇到 \end，说明有多余的 \end
				result.NestingValid = false
				result.NestingError = fmt.Sprintf(
					"unexpected \\end{%s} at line %d without matching \\begin",
					pos.envName, pos.line)
				result.IsValid = false
				logger.Warn("nesting error: unexpected end",
					logger.String("envName", pos.envName),
					logger.Int("line", pos.line))
				return
			}

			top := stack[len(stack)-1]
			if top.envName != pos.envName {
				// 栈顶环境名与当前 \end 不匹配，嵌套错误
				result.NestingValid = false
				result.NestingError = fmt.Sprintf(
					"nesting error: expected \\end{%s} but found \\end{%s} at line %d (\\begin{%s} was at line %d)",
					top.envName, pos.envName, pos.line, top.envName, top.line)
				result.IsValid = false
				logger.Warn("nesting error: mismatched end",
					logger.String("expected", top.envName),
					logger.String("found", pos.envName),
					logger.Int("endLine", pos.line),
					logger.Int("beginLine", top.line))
				return
			}

			// 匹配成功，弹出栈顶
			stack = stack[:len(stack)-1]
		}
	}

	// 检查栈是否为空
	if len(stack) > 0 {
		// 还有未关闭的环境
		unclosed := stack[len(stack)-1]
		result.NestingValid = false
		result.NestingError = fmt.Sprintf(
			"unclosed environment: \\begin{%s} at line %d has no matching \\end",
			unclosed.envName, unclosed.line)
		result.IsValid = false
		logger.Warn("nesting error: unclosed environment",
			logger.String("envName", unclosed.envName),
			logger.Int("line", unclosed.line))
	}
}

// extractEnvPositions 提取所有环境标签的位置信息
func extractEnvPositions(content string) []envPosition {
	positions := []envPosition{}

	// 匹配 \begin{envname} 和 \end{envname}，记录位置
	envRegex := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)
	matches := envRegex.FindAllStringSubmatchIndex(content, -1)

	for _, match := range matches {
		if len(match) >= 6 {
			fullStart := match[0]
			typeStart := match[2]
			typeEnd := match[3]
			nameStart := match[4]
			nameEnd := match[5]

			envType := content[typeStart:typeEnd]
			envName := content[nameStart:nameEnd]

			// 计算行号
			line := strings.Count(content[:fullStart], "\n") + 1

			positions = append(positions, envPosition{
				envName:  envName,
				isBegin:  envType == "begin",
				position: fullStart,
				line:     line,
			})
		}
	}

	return positions
}

// FormatEnvironmentValidation 格式化环境验证结果为可读字符串
func FormatEnvironmentValidation(result *EnvironmentValidation) string {
	if result.IsValid {
		return "环境验证通过: 所有 \\begin 和 \\end 标签匹配正确"
	}

	var sb strings.Builder
	sb.WriteString("环境验证失败:\n")

	if len(result.Mismatches) > 0 {
		sb.WriteString("  不匹配的环境:\n")
		for _, m := range result.Mismatches {
			if m.Difference > 0 {
				sb.WriteString(fmt.Sprintf("    - %s: 缺少 %d 个 \\end{%s}\n",
					m.EnvName, m.Difference, m.EnvName))
			} else {
				sb.WriteString(fmt.Sprintf("    - %s: 多余 %d 个 \\end{%s}\n",
					m.EnvName, -m.Difference, m.EnvName))
			}
		}
	}

	if !result.NestingValid && result.NestingError != "" {
		sb.WriteString(fmt.Sprintf("  嵌套错误: %s\n", result.NestingError))
	}

	return sb.String()
}

// TranslationValidationResult contains the result of translation validation
type TranslationValidationResult struct {
	IsValid          bool     `json:"is_valid"`
	Errors           []string `json:"errors,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	OriginalLength   int      `json:"original_length"`
	TranslatedLength int      `json:"translated_length"`
	ChineseCharCount int      `json:"chinese_char_count"`
	LengthRatio      float64  `json:"length_ratio"`
}

// TranslationValidator validates translation results to detect anomalies
type TranslationValidator struct {
	// MinLengthRatio is the minimum acceptable ratio of translated/original length
	MinLengthRatio float64
	// MaxLengthRatio is the maximum acceptable ratio of translated/original length
	MaxLengthRatio float64
	// MinChineseRatio is the minimum ratio of Chinese characters in translated content
	MinChineseRatio float64
	// RequiredPatterns are patterns that must be preserved in translation
	RequiredPatterns []string
	// ForbiddenPatterns are patterns that indicate a bad translation (e.g., template text)
	ForbiddenPatterns []string
}

// NewTranslationValidator creates a new validator with default settings
func NewTranslationValidator() *TranslationValidator {
	return &TranslationValidator{
		MinLengthRatio:  0.3,  // Translated should be at least 30% of original
		MaxLengthRatio:  3.0,  // Translated should be at most 300% of original
		MinChineseRatio: 0.05, // At least 5% of content should be Chinese
		RequiredPatterns: []string{
			`\\documentclass`,
		},
		ForbiddenPatterns: []string{
			// Common template/placeholder text that indicates failed translation (English)
			`This document contains the translated content`,
			`Please add your paper content here`,
			`Add related work section here`,
			`Add methodology section here`,
			`Add experiments section here`,
			`Add conclusion section here`,
			`NeurIPS 2025 Paper Translation`, // Generic title replacement
			`Anonymous Authors`,               // Generic author replacement
			// Chinese placeholder text that indicates failed translation
			`此处应放置`,           // "This is where you should put..."
			`此处应填写`,           // "This is where you should fill in..."
			`请在此处添加`,         // "Please add here..."
			`翻译内容`,             // "translated content" (placeholder)
			`论文翻译`,             // Generic "paper translation" title
			`本文档是`,             // "This document is..." (template intro)
			`的翻译版本`,           // "...translated version" (template intro)
		},
	}
}

// ValidateTranslation checks if the translation result is valid
func (v *TranslationValidator) ValidateTranslation(original, translated string) *TranslationValidationResult {
	result := &TranslationValidationResult{
		IsValid:          true,
		OriginalLength:   len(original),
		TranslatedLength: len(translated),
	}

	// Calculate length ratio
	if result.OriginalLength > 0 {
		result.LengthRatio = float64(result.TranslatedLength) / float64(result.OriginalLength)
	}

	// Count Chinese characters
	result.ChineseCharCount = countChineseCharacters(translated)

	// Check for empty translation
	if strings.TrimSpace(translated) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "翻译结果为空")
		return result
	}

	// Check length ratio
	if result.LengthRatio < v.MinLengthRatio {
		result.IsValid = false
		result.Errors = append(result.Errors, 
			"翻译结果过短，可能丢失了大量内容")
		logger.Warn("translation too short",
			logger.Float64("lengthRatio", result.LengthRatio),
			logger.Float64("minRatio", v.MinLengthRatio))
	}

	if result.LengthRatio > v.MaxLengthRatio {
		result.Warnings = append(result.Warnings,
			"翻译结果异常长，可能包含重复内容")
		logger.Warn("translation unusually long",
			logger.Float64("lengthRatio", result.LengthRatio),
			logger.Float64("maxRatio", v.MaxLengthRatio))
	}

	// Check Chinese character ratio
	chineseRatio := float64(result.ChineseCharCount) / float64(result.TranslatedLength)
	
	// For files with mostly configuration (< 10000 chars), be more lenient
	minChineseRatio := v.MinChineseRatio
	if result.TranslatedLength < 10000 {
		minChineseRatio = 0.02 // Only require 2% for configuration-heavy files
		logger.Debug("using lenient validation for configuration file",
			logger.Int("length", result.TranslatedLength),
			logger.Float64("minRatio", minChineseRatio))
	}
	
	if chineseRatio < minChineseRatio && result.TranslatedLength > 1000 {
		result.IsValid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("翻译结果中中文字符过少，可能翻译失败 (中文比例: %.2f%%, 需要至少 %.2f%%)", 
				chineseRatio*100, minChineseRatio*100))
		result.Errors = append(result.Errors,
			"可能的原因:")
		result.Errors = append(result.Errors,
			"  1. OpenAI API 密钥未配置或无效")
		result.Errors = append(result.Errors,
			"  2. API 调用失败但未正确报错")
		result.Errors = append(result.Errors,
			"  3. 翻译模型返回了英文而不是中文")
		result.Errors = append(result.Errors,
			"  4. 文件主要包含 LaTeX 配置代码，实际文本内容很少")
		result.Errors = append(result.Errors,
			"请检查:")
		result.Errors = append(result.Errors,
			"  - 配置文件中的 openai_api_key 是否正确")
		result.Errors = append(result.Errors,
			"  - 环境变量 OPENAI_API_KEY 是否设置")
		result.Errors = append(result.Errors,
			"  - API 密钥是否有效且有足够的配额")
		result.Errors = append(result.Errors,
			"  - 查看翻译后的文件，确认是否有中文内容")
		logger.Warn("too few Chinese characters in translation",
			logger.Float64("chineseRatio", chineseRatio),
			logger.Int("chineseCount", result.ChineseCharCount))
	}

	// Check required patterns
	for _, pattern := range v.RequiredPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(original) && !re.MatchString(translated) {
			result.IsValid = false
			result.Errors = append(result.Errors,
				"翻译结果丢失了必要的 LaTeX 结构: "+pattern)
			logger.Warn("required pattern missing in translation",
				logger.String("pattern", pattern))
		}
	}

	// Check forbidden patterns (template text)
	for _, pattern := range v.ForbiddenPatterns {
		if strings.Contains(translated, pattern) {
			result.IsValid = false
			result.Errors = append(result.Errors,
				"翻译结果包含模板占位符文本，表明翻译失败")
			logger.Warn("forbidden pattern found in translation",
				logger.String("pattern", pattern))
			break // One forbidden pattern is enough to fail
		}
	}

	// Check if original document structure is preserved
	if !v.checkStructurePreserved(original, translated) {
		result.Warnings = append(result.Warnings,
			"翻译结果的文档结构可能与原文不一致")
	}

	return result
}

// checkStructurePreserved checks if key document structure is preserved
func (v *TranslationValidator) checkStructurePreserved(original, translated string) bool {
	// Check if documentclass is preserved (allowing for ctex addition)
	origDocClass := extractDocumentClass(original)
	transDocClass := extractDocumentClass(translated)
	
	if origDocClass != "" && transDocClass != "" {
		// Allow ctex to be added, but base class should be similar
		// e.g., "article" should remain "article", not become something else
		if !strings.Contains(transDocClass, origDocClass) && 
		   !strings.Contains(origDocClass, transDocClass) {
			// Check if it's just article -> article with ctex
			if origDocClass != "article" || transDocClass != "article" {
				logger.Warn("document class changed",
					logger.String("original", origDocClass),
					logger.String("translated", transDocClass))
				return false
			}
		}
	}

	// Check if major sections are preserved
	origSections := countSections(original)
	transSections := countSections(translated)
	
	// Allow some variation, but not too much
	if origSections > 0 && transSections < origSections/2 {
		logger.Warn("many sections lost in translation",
			logger.Int("original", origSections),
			logger.Int("translated", transSections))
		return false
	}

	return true
}

// ValidateChunkTranslation validates a single chunk translation
func (v *TranslationValidator) ValidateChunkTranslation(originalChunk, translatedChunk string) *TranslationValidationResult {
	result := &TranslationValidationResult{
		IsValid:          true,
		OriginalLength:   len(originalChunk),
		TranslatedLength: len(translatedChunk),
	}

	// Calculate length ratio
	if result.OriginalLength > 0 {
		result.LengthRatio = float64(result.TranslatedLength) / float64(result.OriginalLength)
	}

	// Count Chinese characters
	result.ChineseCharCount = countChineseCharacters(translatedChunk)

	// Check for empty translation
	if strings.TrimSpace(translatedChunk) == "" && strings.TrimSpace(originalChunk) != "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "分块翻译结果为空")
		return result
	}

	// Check length ratio (more lenient for chunks)
	if result.LengthRatio < 0.1 {
		result.IsValid = false
		result.Errors = append(result.Errors, "分块翻译结果过短")
	}

	// Check for forbidden patterns in chunks
	for _, pattern := range v.ForbiddenPatterns {
		if strings.Contains(translatedChunk, pattern) {
			result.IsValid = false
			result.Errors = append(result.Errors,
				"分块翻译包含模板文本: "+pattern)
			break
		}
	}

	// For chunks with substantial text, check Chinese ratio
	if result.OriginalLength > 500 {
		chineseRatio := float64(result.ChineseCharCount) / float64(result.TranslatedLength)
		if chineseRatio < 0.02 {
			result.Warnings = append(result.Warnings,
				"分块翻译中中文字符较少")
		}
	}

	return result
}

// countChineseCharacters counts the number of Chinese characters in a string
func countChineseCharacters(s string) int {
	count := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			count++
		}
	}
	return count
}

// extractDocumentClass extracts the document class from LaTeX content
func extractDocumentClass(content string) string {
	re := regexp.MustCompile(`\\documentclass(?:\[[^\]]*\])?\{([^}]+)\}`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// countSections counts the number of section commands in LaTeX content
func countSections(content string) int {
	re := regexp.MustCompile(`\\(?:section|subsection|subsubsection|chapter)\s*[\[{]`)
	return len(re.FindAllString(content, -1))
}

// BraceValidation 大括号验证结果
// 实现 Requirements 4.1, 4.2, 4.4
type BraceValidation struct {
	IsBalanced     bool  // 大括号是否平衡
	OpenCount      int   // 开括号 { 的数量
	CloseCount     int   // 闭括号 } 的数量
	Difference     int   // 差异 (OpenCount - CloseCount)
	ErrorLocations []int // 可能的错误位置（字符位置）
	ErrorLines     []int // 可能的错误行号
	MaxNestDepth   int   // 最大嵌套深度
}

// bracePosition 记录大括号的位置信息
type bracePosition struct {
	position int  // 字符位置
	line     int  // 行号
	isOpen   bool // 是否是开括号
}

// ValidateBraces 验证LaTeX内容中的大括号平衡
// 实现 Requirements 4.1, 4.2, 4.4
func ValidateBraces(content string) *BraceValidation {
	result := &BraceValidation{
		IsBalanced:     true,
		ErrorLocations: []int{},
		ErrorLines:     []int{},
	}

	// 统计大括号数量
	countBracesForValidation(content, result)

	// 验证嵌套并定位错误
	validateBraceNesting(content, result)

	return result
}

// countBracesForValidation 统计内容中的大括号数量（用于验证）
// 实现 Requirement 4.1
func countBracesForValidation(content string, result *BraceValidation) {
	inComment := false
	lineStart := true

	for i, r := range content {
		// 检测注释行
		if lineStart && r == '%' {
			inComment = true
		}
		if r == '\n' {
			inComment = false
			lineStart = true
			continue
		}
		if r != ' ' && r != '\t' {
			lineStart = false
		}

		// 跳过注释中的大括号
		if inComment {
			continue
		}

		// 检查是否是转义的大括号 \{ 或 \}
		if i > 0 && content[i-1] == '\\' {
			continue
		}

		if r == '{' {
			result.OpenCount++
		} else if r == '}' {
			result.CloseCount++
		}
	}

	result.Difference = result.OpenCount - result.CloseCount
	result.IsBalanced = result.Difference == 0

	logger.Debug("brace counts",
		logger.Int("openCount", result.OpenCount),
		logger.Int("closeCount", result.CloseCount),
		logger.Int("difference", result.Difference))
}

// validateBraceNesting 验证大括号嵌套并定位错误
// 实现 Requirements 4.2, 4.4
func validateBraceNesting(content string, result *BraceValidation) {
	// 使用栈来跟踪大括号嵌套
	stack := []bracePosition{}
	currentLine := 1
	inComment := false
	lineStart := true
	maxDepth := 0

	for i, r := range content {
		// 检测注释行
		if lineStart && r == '%' {
			inComment = true
		}
		if r == '\n' {
			currentLine++
			inComment = false
			lineStart = true
			continue
		}
		if r != ' ' && r != '\t' {
			lineStart = false
		}

		// 跳过注释中的大括号
		if inComment {
			continue
		}

		// 检查是否是转义的大括号 \{ 或 \}
		if i > 0 && content[i-1] == '\\' {
			continue
		}

		if r == '{' {
			stack = append(stack, bracePosition{
				position: i,
				line:     currentLine,
				isOpen:   true,
			})
			// 更新最大嵌套深度
			if len(stack) > maxDepth {
				maxDepth = len(stack)
			}
		} else if r == '}' {
			if len(stack) == 0 {
				// 栈为空但遇到闭括号，说明有多余的闭括号
				result.IsBalanced = false
				result.ErrorLocations = append(result.ErrorLocations, i)
				result.ErrorLines = append(result.ErrorLines, currentLine)
				logger.Warn("unmatched closing brace",
					logger.Int("position", i),
					logger.Int("line", currentLine))
			} else {
				// 弹出栈顶
				stack = stack[:len(stack)-1]
			}
		}
	}

	result.MaxNestDepth = maxDepth

	// 检查栈是否为空（是否有未关闭的开括号）
	if len(stack) > 0 {
		result.IsBalanced = false
		// 记录所有未关闭的开括号位置
		for _, pos := range stack {
			result.ErrorLocations = append(result.ErrorLocations, pos.position)
			result.ErrorLines = append(result.ErrorLines, pos.line)
			logger.Warn("unclosed opening brace",
				logger.Int("position", pos.position),
				logger.Int("line", pos.line))
		}
	}
}

// FormatBraceValidation 格式化大括号验证结果为可读字符串
func FormatBraceValidation(result *BraceValidation) string {
	if result.IsBalanced {
		return fmt.Sprintf("大括号验证通过: 共 %d 对大括号，最大嵌套深度 %d",
			result.OpenCount, result.MaxNestDepth)
	}

	var sb strings.Builder
	sb.WriteString("大括号验证失败:\n")
	sb.WriteString(fmt.Sprintf("  开括号数量: %d\n", result.OpenCount))
	sb.WriteString(fmt.Sprintf("  闭括号数量: %d\n", result.CloseCount))
	sb.WriteString(fmt.Sprintf("  差异: %d\n", result.Difference))
	sb.WriteString(fmt.Sprintf("  最大嵌套深度: %d\n", result.MaxNestDepth))

	if len(result.ErrorLines) > 0 {
		sb.WriteString("  可能的错误位置:\n")
		for i, line := range result.ErrorLines {
			if i < len(result.ErrorLocations) {
				sb.WriteString(fmt.Sprintf("    - 第 %d 行 (字符位置 %d)\n",
					line, result.ErrorLocations[i]))
			}
		}
	}

	return sb.String()
}

// FormatValidationErrors formats validation errors for display
func FormatValidationErrors(result *TranslationValidationResult) string {
	if result.IsValid {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("翻译验证失败:\n")
	
	for _, err := range result.Errors {
		sb.WriteString("  - ")
		sb.WriteString(err)
		sb.WriteString("\n")
	}
	
	if len(result.Warnings) > 0 {
		sb.WriteString("警告:\n")
		for _, warn := range result.Warnings {
			sb.WriteString("  - ")
			sb.WriteString(warn)
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf("统计: 原文长度=%d, 译文长度=%d, 中文字符=%d, 长度比=%.2f\n",
		result.OriginalLength, result.TranslatedLength, 
		result.ChineseCharCount, result.LengthRatio))

	return sb.String()
}


// CommentValidation 注释验证结果
// 实现 Requirements 5.1, 5.4
type CommentValidation struct {
	IsValid                  bool                    // 验证是否通过
	OriginalCommentLines     []int                   // 原始内容中的注释行号
	TranslatedCommentLines   []int                   // 翻译后内容中的注释行号
	RemovedCommentLines      []CommentLineChange     // 被移除注释符号的行
	UncommentedEnvTags       []UncommentedEnvTag     // 被取消注释的环境标签
	TotalOriginalComments    int                     // 原始注释行总数
	TotalTranslatedComments  int                     // 翻译后注释行总数
}

// CommentLineChange 记录注释行变化
type CommentLineChange struct {
	OriginalLine   int    // 原始行号
	OriginalText   string // 原始行内容
	TranslatedLine int    // 对应的翻译行号（如果能匹配）
	TranslatedText string // 翻译后的行内容
}

// UncommentedEnvTag 记录被取消注释的环境标签
type UncommentedEnvTag struct {
	OriginalLine   int    // 原始行号
	EnvTag         string // 环境标签 (如 \begin{figure} 或 \end{table})
	EnvName        string // 环境名称 (如 figure, table)
	IsBegin        bool   // 是否是 \begin 标签
	TranslatedLine int    // 翻译后对应的行号
}

// commentLineInfo 内部使用的注释行信息
type commentLineInfo struct {
	lineNumber int
	content    string
	envTags    []envTagInComment // 该行中的环境标签
}

// envTagInComment 注释中的环境标签
type envTagInComment struct {
	tag     string // 完整标签，如 \begin{figure}
	envName string // 环境名，如 figure
	isBegin bool   // 是否是 \begin
}

// ValidateComments 验证注释格式
// 比较原始内容和翻译后内容，检测注释变化
// 实现 Requirements 5.1, 5.4
func ValidateComments(original, translated string) *CommentValidation {
	result := &CommentValidation{
		IsValid:                true,
		OriginalCommentLines:   []int{},
		TranslatedCommentLines: []int{},
		RemovedCommentLines:    []CommentLineChange{},
		UncommentedEnvTags:     []UncommentedEnvTag{},
	}

	// 提取原始内容中的注释行信息
	originalComments := extractCommentLines(original)
	result.OriginalCommentLines = getLineNumbers(originalComments)
	result.TotalOriginalComments = len(originalComments)

	// 提取翻译后内容中的注释行信息
	translatedComments := extractCommentLines(translated)
	result.TranslatedCommentLines = getLineNumbers(translatedComments)
	result.TotalTranslatedComments = len(translatedComments)

	// 检测被移除注释符号的行
	detectRemovedComments(original, translated, originalComments, result)

	// 检测被取消注释的环境标签
	detectUncommentedEnvTags(original, translated, originalComments, result)

	// 如果有任何问题，标记为无效
	if len(result.RemovedCommentLines) > 0 || len(result.UncommentedEnvTags) > 0 {
		result.IsValid = false
	}

	logger.Debug("comment validation complete",
		logger.Int("originalComments", result.TotalOriginalComments),
		logger.Int("translatedComments", result.TotalTranslatedComments),
		logger.Int("removedComments", len(result.RemovedCommentLines)),
		logger.Int("uncommentedEnvTags", len(result.UncommentedEnvTags)))

	return result
}

// extractCommentLines 提取内容中的所有注释行
// 实现 Requirement 5.1: 识别以 % 开头的注释行
func extractCommentLines(content string) []commentLineInfo {
	comments := []commentLineInfo{}
	lines := strings.Split(content, "\n")

	// 用于匹配环境标签的正则表达式
	envTagRegex := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)

	for i, line := range lines {
		trimmedLine := strings.TrimLeft(line, " \t")
		
		// 检查是否是注释行（以 % 开头）
		if strings.HasPrefix(trimmedLine, "%") {
			info := commentLineInfo{
				lineNumber: i + 1, // 行号从1开始
				content:    line,
				envTags:    []envTagInComment{},
			}

			// 查找该注释行中的环境标签
			matches := envTagRegex.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 3 {
					info.envTags = append(info.envTags, envTagInComment{
						tag:     match[0],
						envName: match[2],
						isBegin: match[1] == "begin",
					})
				}
			}

			comments = append(comments, info)
		}
	}

	return comments
}

// getLineNumbers 从注释行信息中提取行号列表
func getLineNumbers(comments []commentLineInfo) []int {
	lineNumbers := make([]int, len(comments))
	for i, c := range comments {
		lineNumbers[i] = c.lineNumber
	}
	return lineNumbers
}

// detectRemovedComments 检测被移除注释符号的行
// 通过比较原始和翻译后的内容，找出原本是注释但翻译后不再是注释的行
func detectRemovedComments(original, translated string, originalComments []commentLineInfo, result *CommentValidation) {
	originalLines := strings.Split(original, "\n")
	translatedLines := strings.Split(translated, "\n")

	// 创建翻译后注释行的集合（用于快速查找）
	translatedCommentSet := make(map[int]bool)
	for _, c := range extractCommentLines(translated) {
		translatedCommentSet[c.lineNumber] = true
	}

	// 对于每个原始注释行，检查对应的翻译行是否仍然是注释
	for _, origComment := range originalComments {
		// 尝试找到对应的翻译行
		// 策略1: 如果行数相同，直接比较对应行
		if origComment.lineNumber <= len(translatedLines) {
			translatedLine := translatedLines[origComment.lineNumber-1]
			trimmedTranslated := strings.TrimLeft(translatedLine, " \t")

			// 如果原始行是注释，但翻译后不是注释
			if !strings.HasPrefix(trimmedTranslated, "%") {
				// 检查内容是否相似（可能只是移除了 % 符号）
				origContent := strings.TrimLeft(origComment.content, " \t%")
				transContent := strings.TrimLeft(translatedLine, " \t")

				// 如果内容相似或翻译行非空，记录为被移除的注释
				if transContent != "" && (isSimilarContent(origContent, transContent) || 
					containsSimilarEnvTags(origComment.content, translatedLine)) {
					result.RemovedCommentLines = append(result.RemovedCommentLines, CommentLineChange{
						OriginalLine:   origComment.lineNumber,
						OriginalText:   origComment.content,
						TranslatedLine: origComment.lineNumber,
						TranslatedText: translatedLine,
					})

					logger.Warn("comment symbol removed",
						logger.Int("line", origComment.lineNumber),
						logger.String("original", truncateForDisplay(origComment.content, 50)),
						logger.String("translated", truncateForDisplay(translatedLine, 50)))
				}
			}
		}
	}

	// 策略2: 基于内容匹配（用于行数不同的情况）
	if len(originalLines) != len(translatedLines) {
		detectRemovedCommentsByContent(originalComments, translatedLines, result)
	}
}

// detectRemovedCommentsByContent 通过内容匹配检测被移除的注释
func detectRemovedCommentsByContent(originalComments []commentLineInfo, translatedLines []string, result *CommentValidation) {
	// 已经在 detectRemovedComments 中处理过的行
	processedOrigLines := make(map[int]bool)
	for _, change := range result.RemovedCommentLines {
		processedOrigLines[change.OriginalLine] = true
	}

	for _, origComment := range originalComments {
		if processedOrigLines[origComment.lineNumber] {
			continue
		}

		// 提取原始注释的内容（去掉 % 符号）
		origContent := strings.TrimLeft(origComment.content, " \t%")
		if origContent == "" {
			continue
		}

		// 在翻译后的内容中查找相似的非注释行
		for transLineNum, transLine := range translatedLines {
			trimmedTrans := strings.TrimLeft(transLine, " \t")
			
			// 跳过注释行
			if strings.HasPrefix(trimmedTrans, "%") {
				continue
			}

			// 检查是否包含相似的环境标签
			if containsSimilarEnvTags(origComment.content, transLine) {
				result.RemovedCommentLines = append(result.RemovedCommentLines, CommentLineChange{
					OriginalLine:   origComment.lineNumber,
					OriginalText:   origComment.content,
					TranslatedLine: transLineNum + 1,
					TranslatedText: transLine,
				})
				break
			}
		}
	}
}

// detectUncommentedEnvTags 检测被取消注释的环境标签
// 实现 Requirement 5.4: 确保注释中的 \begin{...} 或 \end{...} 保持注释状态
func detectUncommentedEnvTags(original, translated string, originalComments []commentLineInfo, result *CommentValidation) {
	translatedLines := strings.Split(translated, "\n")

	// 用于匹配环境标签的正则表达式
	envTagRegex := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)

	// 收集原始内容中被注释的环境标签
	commentedEnvTags := make(map[string][]commentLineInfo) // envTag -> 出现的注释行
	for _, comment := range originalComments {
		for _, tag := range comment.envTags {
			commentedEnvTags[tag.tag] = append(commentedEnvTags[tag.tag], comment)
		}
	}

	// 在翻译后的内容中查找这些环境标签
	for transLineNum, transLine := range translatedLines {
		trimmedTrans := strings.TrimLeft(transLine, " \t")
		
		// 如果这行是注释，跳过
		if strings.HasPrefix(trimmedTrans, "%") {
			continue
		}

		// 查找该行中的环境标签
		matches := envTagRegex.FindAllStringSubmatch(transLine, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				tag := match[0]
				envName := match[2]
				isBegin := match[1] == "begin"

				// 检查这个标签在原始内容中是否被注释
				if origComments, exists := commentedEnvTags[tag]; exists {
					// 找到了一个原本被注释但现在没有被注释的环境标签
					for _, origComment := range origComments {
						result.UncommentedEnvTags = append(result.UncommentedEnvTags, UncommentedEnvTag{
							OriginalLine:   origComment.lineNumber,
							EnvTag:         tag,
							EnvName:        envName,
							IsBegin:        isBegin,
							TranslatedLine: transLineNum + 1,
						})

						logger.Warn("commented env tag was uncommented",
							logger.String("tag", tag),
							logger.Int("originalLine", origComment.lineNumber),
							logger.Int("translatedLine", transLineNum+1))
					}
					// 从映射中移除，避免重复报告
					delete(commentedEnvTags, tag)
				}
			}
		}
	}
}

// isSimilarContent 检查两个字符串内容是否相似
func isSimilarContent(s1, s2 string) bool {
	// 简单的相似度检查：去除空白后比较
	clean1 := strings.ReplaceAll(strings.TrimSpace(s1), " ", "")
	clean2 := strings.ReplaceAll(strings.TrimSpace(s2), " ", "")

	if clean1 == "" || clean2 == "" {
		return false
	}

	// 如果一个是另一个的子串，认为相似
	if strings.Contains(clean1, clean2) || strings.Contains(clean2, clean1) {
		return true
	}

	// 计算公共前缀长度
	minLen := len(clean1)
	if len(clean2) < minLen {
		minLen = len(clean2)
	}

	commonPrefix := 0
	for i := 0; i < minLen; i++ {
		if clean1[i] == clean2[i] {
			commonPrefix++
		} else {
			break
		}
	}

	// 如果公共前缀超过较短字符串的50%，认为相似
	return float64(commonPrefix)/float64(minLen) > 0.5
}

// containsSimilarEnvTags 检查两行是否包含相似的环境标签
func containsSimilarEnvTags(line1, line2 string) bool {
	envTagRegex := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)

	tags1 := envTagRegex.FindAllString(line1, -1)
	tags2 := envTagRegex.FindAllString(line2, -1)

	if len(tags1) == 0 || len(tags2) == 0 {
		return false
	}

	// 检查是否有相同的环境标签
	tagSet := make(map[string]bool)
	for _, tag := range tags1 {
		tagSet[tag] = true
	}

	for _, tag := range tags2 {
		if tagSet[tag] {
			return true
		}
	}

	return false
}

// truncateForDisplay 截断字符串到指定长度（用于显示）
func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FormatCommentValidation 格式化注释验证结果为可读字符串
func FormatCommentValidation(result *CommentValidation) string {
	if result.IsValid {
		return fmt.Sprintf("注释验证通过: 原始 %d 行注释，翻译后 %d 行注释",
			result.TotalOriginalComments, result.TotalTranslatedComments)
	}

	var sb strings.Builder
	sb.WriteString("注释验证失败:\n")
	sb.WriteString(fmt.Sprintf("  原始注释行数: %d\n", result.TotalOriginalComments))
	sb.WriteString(fmt.Sprintf("  翻译后注释行数: %d\n", result.TotalTranslatedComments))

	if len(result.RemovedCommentLines) > 0 {
		sb.WriteString("  被移除注释符号的行:\n")
		for _, change := range result.RemovedCommentLines {
			sb.WriteString(fmt.Sprintf("    - 第 %d 行: %s\n",
				change.OriginalLine, truncateForDisplay(change.OriginalText, 60)))
			sb.WriteString(fmt.Sprintf("      翻译后第 %d 行: %s\n",
				change.TranslatedLine, truncateForDisplay(change.TranslatedText, 60)))
		}
	}

	if len(result.UncommentedEnvTags) > 0 {
		sb.WriteString("  被取消注释的环境标签:\n")
		for _, tag := range result.UncommentedEnvTags {
			sb.WriteString(fmt.Sprintf("    - %s (原第 %d 行 -> 翻译后第 %d 行)\n",
				tag.EnvTag, tag.OriginalLine, tag.TranslatedLine))
		}
	}

	return sb.String()
}

// StructureComparison 结构比较结果
// 实现 Requirement 7.1
type StructureComparison struct {
	IsMatch            bool                    // 结构是否匹配
	OriginalStructure  *DocumentStructure      // 原始文档结构
	TranslatedStructure *DocumentStructure     // 翻译后文档结构
	Differences        []StructureDifference   // 结构差异列表
	Summary            string                  // 差异摘要
}

// DocumentStructure 文档结构信息
type DocumentStructure struct {
	DocumentClass    string            // 文档类型
	Packages         []string          // 使用的包
	Sections         map[string]int    // 章节命令计数 (section, subsection, etc.)
	Environments     map[string]int    // 环境计数
	Commands         map[string]int    // 关键命令计数 (cite, ref, label, etc.)
	TotalLines       int               // 总行数
	NonEmptyLines    int               // 非空行数
}

// StructureDifference 结构差异信息
type StructureDifference struct {
	Type           string // 差异类型: "section", "environment", "command", "package", "documentclass"
	Name           string // 元素名称
	OriginalCount  int    // 原始数量
	TranslatedCount int   // 翻译后数量
	Difference     int    // 差异 (translated - original)
	Severity       string // 严重程度: "error", "warning", "info"
}

// CompareStructure 比较原始和翻译后的文档结构
// 实现 Requirement 7.1 和 Property 21
func CompareStructure(original, translated string) *StructureComparison {
	result := &StructureComparison{
		IsMatch:     true,
		Differences: []StructureDifference{},
	}

	// 提取原始文档结构
	result.OriginalStructure = extractDocumentStructure(original)
	
	// 提取翻译后文档结构
	result.TranslatedStructure = extractDocumentStructure(translated)

	// 比较文档类型
	compareDocumentClass(result)

	// 比较包
	comparePackages(result)

	// 比较章节命令
	compareSections(result)

	// 比较环境
	compareEnvironments(result)

	// 比较关键命令
	compareCommands(result)

	// 生成摘要
	result.Summary = generateComparisonSummary(result)

	logger.Debug("structure comparison complete",
		logger.Bool("isMatch", result.IsMatch),
		logger.Int("differenceCount", len(result.Differences)))

	return result
}

// extractDocumentStructure 提取文档结构信息
func extractDocumentStructure(content string) *DocumentStructure {
	structure := &DocumentStructure{
		Packages:     []string{},
		Sections:     make(map[string]int),
		Environments: make(map[string]int),
		Commands:     make(map[string]int),
	}

	lines := strings.Split(content, "\n")
	structure.TotalLines = len(lines)

	// 统计非空行
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			structure.NonEmptyLines++
		}
	}

	// 提取文档类型
	structure.DocumentClass = extractDocumentClass(content)

	// 提取使用的包
	structure.Packages = extractPackages(content)

	// 统计章节命令
	countSectionCommands(content, structure)

	// 统计环境
	countEnvironmentsForStructure(content, structure)

	// 统计关键命令
	countKeyCommands(content, structure)

	return structure
}

// extractPackages 提取使用的包
func extractPackages(content string) []string {
	packages := []string{}
	packageRegex := regexp.MustCompile(`\\usepackage(?:\[[^\]]*\])?\{([^}]+)\}`)
	
	matches := packageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			// 处理多个包在一个 usepackage 中的情况
			pkgList := strings.Split(match[1], ",")
			for _, pkg := range pkgList {
				pkg = strings.TrimSpace(pkg)
				if pkg != "" {
					packages = append(packages, pkg)
				}
			}
		}
	}

	// 去重并排序
	packages = uniqueStrings(packages)
	sort.Strings(packages)

	return packages
}

// countSectionCommands 统计章节命令
func countSectionCommands(content string, structure *DocumentStructure) {
	sectionTypes := []string{
		"part",
		"chapter",
		"section",
		"subsection",
		"subsubsection",
		"paragraph",
		"subparagraph",
	}

	for _, secType := range sectionTypes {
		// 匹配 \section{...} 或 \section*{...} 或 \section[...]{...}
		pattern := fmt.Sprintf(`\\%s\*?(?:\[[^\]]*\])?\s*\{`, secType)
		re := regexp.MustCompile(pattern)
		count := len(re.FindAllString(content, -1))
		if count > 0 {
			structure.Sections[secType] = count
		}
	}
}

// countEnvironmentsForStructure 统计环境（用于结构比较）
func countEnvironmentsForStructure(content string, structure *DocumentStructure) {
	// 匹配 \begin{envname}
	beginRegex := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	
	matches := beginRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			envName := match[1]
			structure.Environments[envName]++
		}
	}
}

// countKeyCommands 统计关键命令
func countKeyCommands(content string, structure *DocumentStructure) {
	// 关键命令列表
	keyCommands := []string{
		"cite",
		"citep",
		"citet",
		"ref",
		"eqref",
		"pageref",
		"label",
		"caption",
		"footnote",
		"includegraphics",
		"input",
		"include",
		"bibliography",
		"bibliographystyle",
		"title",
		"author",
		"date",
		"maketitle",
		"tableofcontents",
		"listoffigures",
		"listoftables",
	}

	for _, cmd := range keyCommands {
		// 匹配 \command{...} 或 \command[...]{...} 或 \command*{...}
		pattern := fmt.Sprintf(`\\%s\*?(?:\[[^\]]*\])?(?:\{|$|\s)`, cmd)
		re := regexp.MustCompile(pattern)
		count := len(re.FindAllString(content, -1))
		if count > 0 {
			structure.Commands[cmd] = count
		}
	}
}

// compareDocumentClass 比较文档类型
func compareDocumentClass(result *StructureComparison) {
	orig := result.OriginalStructure.DocumentClass
	trans := result.TranslatedStructure.DocumentClass

	if orig != trans {
		// 允许 ctex 相关的变化（如 article -> ctexart）
		if !isCompatibleDocumentClass(orig, trans) {
			result.IsMatch = false
			result.Differences = append(result.Differences, StructureDifference{
				Type:            "documentclass",
				Name:            fmt.Sprintf("%s -> %s", orig, trans),
				OriginalCount:   1,
				TranslatedCount: 1,
				Difference:      0,
				Severity:        "warning",
			})

			logger.Warn("document class changed",
				logger.String("original", orig),
				logger.String("translated", trans))
		}
	}
}

// isCompatibleDocumentClass 检查文档类型是否兼容
func isCompatibleDocumentClass(orig, trans string) bool {
	// 允许的兼容变化
	compatiblePairs := map[string][]string{
		"article":  {"article", "ctexart"},
		"report":   {"report", "ctexrep"},
		"book":     {"book", "ctexbook"},
		"ctexart":  {"article", "ctexart"},
		"ctexrep":  {"report", "ctexrep"},
		"ctexbook": {"book", "ctexbook"},
	}

	if compatible, exists := compatiblePairs[orig]; exists {
		for _, c := range compatible {
			if c == trans {
				return true
			}
		}
	}

	return orig == trans
}

// comparePackages 比较使用的包
func comparePackages(result *StructureComparison) {
	origPkgs := result.OriginalStructure.Packages
	transPkgs := result.TranslatedStructure.Packages

	// 创建包集合
	origSet := make(map[string]bool)
	for _, pkg := range origPkgs {
		origSet[pkg] = true
	}

	transSet := make(map[string]bool)
	for _, pkg := range transPkgs {
		transSet[pkg] = true
	}

	// 检查丢失的包
	for _, pkg := range origPkgs {
		if !transSet[pkg] {
			result.Differences = append(result.Differences, StructureDifference{
				Type:            "package",
				Name:            pkg,
				OriginalCount:   1,
				TranslatedCount: 0,
				Difference:      -1,
				Severity:        "warning",
			})

			logger.Warn("package missing in translation",
				logger.String("package", pkg))
		}
	}

	// 检查新增的包（通常是允许的，如 ctex）
	for _, pkg := range transPkgs {
		if !origSet[pkg] {
			// 某些包是翻译时添加的，不算错误
			severity := "info"
			if !isExpectedAddedPackage(pkg) {
				severity = "warning"
			}

			result.Differences = append(result.Differences, StructureDifference{
				Type:            "package",
				Name:            pkg,
				OriginalCount:   0,
				TranslatedCount: 1,
				Difference:      1,
				Severity:        severity,
			})
		}
	}
}

// isExpectedAddedPackage 检查是否是翻译时预期添加的包
func isExpectedAddedPackage(pkg string) bool {
	expectedPackages := []string{
		"ctex",
		"xeCJK",
		"fontspec",
		"CJKutf8",
	}

	for _, expected := range expectedPackages {
		if pkg == expected {
			return true
		}
	}
	return false
}

// compareSections 比较章节命令
func compareSections(result *StructureComparison) {
	origSections := result.OriginalStructure.Sections
	transSections := result.TranslatedStructure.Sections

	// 获取所有章节类型
	allSectionTypes := make(map[string]bool)
	for secType := range origSections {
		allSectionTypes[secType] = true
	}
	for secType := range transSections {
		allSectionTypes[secType] = true
	}

	// 比较每种章节类型
	for secType := range allSectionTypes {
		origCount := origSections[secType]
		transCount := transSections[secType]

		if origCount != transCount {
			diff := transCount - origCount
			severity := "warning"
			if diff < 0 && origCount > 0 {
				// 丢失章节是更严重的问题
				severity = "error"
				result.IsMatch = false
			}

			result.Differences = append(result.Differences, StructureDifference{
				Type:            "section",
				Name:            secType,
				OriginalCount:   origCount,
				TranslatedCount: transCount,
				Difference:      diff,
				Severity:        severity,
			})

			logger.Warn("section count mismatch",
				logger.String("sectionType", secType),
				logger.Int("original", origCount),
				logger.Int("translated", transCount))
		}
	}
}

// compareEnvironments 比较环境
func compareEnvironments(result *StructureComparison) {
	origEnvs := result.OriginalStructure.Environments
	transEnvs := result.TranslatedStructure.Environments

	// 获取所有环境类型
	allEnvTypes := make(map[string]bool)
	for envType := range origEnvs {
		allEnvTypes[envType] = true
	}
	for envType := range transEnvs {
		allEnvTypes[envType] = true
	}

	// 比较每种环境类型
	for envType := range allEnvTypes {
		origCount := origEnvs[envType]
		transCount := transEnvs[envType]

		if origCount != transCount {
			diff := transCount - origCount
			severity := "warning"
			
			// 关键环境的丢失是错误
			if diff < 0 && isCriticalEnvironment(envType) {
				severity = "error"
				result.IsMatch = false
			}

			result.Differences = append(result.Differences, StructureDifference{
				Type:            "environment",
				Name:            envType,
				OriginalCount:   origCount,
				TranslatedCount: transCount,
				Difference:      diff,
				Severity:        severity,
			})

			logger.Warn("environment count mismatch",
				logger.String("envType", envType),
				logger.Int("original", origCount),
				logger.Int("translated", transCount))
		}
	}
}

// isCriticalEnvironment 检查是否是关键环境
func isCriticalEnvironment(envType string) bool {
	criticalEnvs := []string{
		"document",
		"figure",
		"table",
		"equation",
		"align",
		"theorem",
		"lemma",
		"proof",
		"abstract",
		"itemize",
		"enumerate",
		"description",
	}

	for _, critical := range criticalEnvs {
		if envType == critical {
			return true
		}
	}
	return false
}

// compareCommands 比较关键命令
func compareCommands(result *StructureComparison) {
	origCmds := result.OriginalStructure.Commands
	transCmds := result.TranslatedStructure.Commands

	// 获取所有命令类型
	allCmdTypes := make(map[string]bool)
	for cmdType := range origCmds {
		allCmdTypes[cmdType] = true
	}
	for cmdType := range transCmds {
		allCmdTypes[cmdType] = true
	}

	// 比较每种命令类型
	for cmdType := range allCmdTypes {
		origCount := origCmds[cmdType]
		transCount := transCmds[cmdType]

		if origCount != transCount {
			diff := transCount - origCount
			severity := "warning"
			
			// 引用命令的丢失是更严重的问题
			if diff < 0 && isCriticalCommand(cmdType) {
				severity = "error"
				result.IsMatch = false
			}

			result.Differences = append(result.Differences, StructureDifference{
				Type:            "command",
				Name:            cmdType,
				OriginalCount:   origCount,
				TranslatedCount: transCount,
				Difference:      diff,
				Severity:        severity,
			})

			logger.Warn("command count mismatch",
				logger.String("cmdType", cmdType),
				logger.Int("original", origCount),
				logger.Int("translated", transCount))
		}
	}
}

// isCriticalCommand 检查是否是关键命令
func isCriticalCommand(cmdType string) bool {
	criticalCmds := []string{
		"cite",
		"citep",
		"citet",
		"ref",
		"eqref",
		"label",
		"caption",
		"includegraphics",
	}

	for _, critical := range criticalCmds {
		if cmdType == critical {
			return true
		}
	}
	return false
}

// generateComparisonSummary 生成比较摘要
func generateComparisonSummary(result *StructureComparison) string {
	if result.IsMatch && len(result.Differences) == 0 {
		return "文档结构完全匹配"
	}

	var sb strings.Builder

	// 统计各类差异
	errorCount := 0
	warningCount := 0
	infoCount := 0

	for _, diff := range result.Differences {
		switch diff.Severity {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "info":
			infoCount++
		}
	}

	if result.IsMatch {
		sb.WriteString("文档结构基本匹配")
	} else {
		sb.WriteString("文档结构存在差异")
	}

	sb.WriteString(fmt.Sprintf(" (错误: %d, 警告: %d, 信息: %d)", errorCount, warningCount, infoCount))

	return sb.String()
}

// uniqueStrings 去除字符串切片中的重复项
func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// FormatStructureComparison 格式化结构比较结果为可读字符串
func FormatStructureComparison(result *StructureComparison) string {
	var sb strings.Builder

	sb.WriteString("=== 文档结构比较 ===\n")
	sb.WriteString(fmt.Sprintf("结果: %s\n\n", result.Summary))

	// 原始文档结构
	sb.WriteString("原始文档:\n")
	sb.WriteString(formatDocumentStructure(result.OriginalStructure))
	sb.WriteString("\n")

	// 翻译后文档结构
	sb.WriteString("翻译后文档:\n")
	sb.WriteString(formatDocumentStructure(result.TranslatedStructure))
	sb.WriteString("\n")

	// 差异详情
	if len(result.Differences) > 0 {
		sb.WriteString("差异详情:\n")
		
		// 按严重程度分组显示
		errors := []StructureDifference{}
		warnings := []StructureDifference{}
		infos := []StructureDifference{}

		for _, diff := range result.Differences {
			switch diff.Severity {
			case "error":
				errors = append(errors, diff)
			case "warning":
				warnings = append(warnings, diff)
			case "info":
				infos = append(infos, diff)
			}
		}

		if len(errors) > 0 {
			sb.WriteString("  [错误]\n")
			for _, diff := range errors {
				sb.WriteString(formatDifference(diff))
			}
		}

		if len(warnings) > 0 {
			sb.WriteString("  [警告]\n")
			for _, diff := range warnings {
				sb.WriteString(formatDifference(diff))
			}
		}

		if len(infos) > 0 {
			sb.WriteString("  [信息]\n")
			for _, diff := range infos {
				sb.WriteString(formatDifference(diff))
			}
		}
	}

	return sb.String()
}

// formatDocumentStructure 格式化文档结构信息
func formatDocumentStructure(structure *DocumentStructure) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("  文档类型: %s\n", structure.DocumentClass))
	sb.WriteString(fmt.Sprintf("  总行数: %d (非空: %d)\n", structure.TotalLines, structure.NonEmptyLines))

	if len(structure.Packages) > 0 {
		sb.WriteString(fmt.Sprintf("  包数量: %d\n", len(structure.Packages)))
	}

	if len(structure.Sections) > 0 {
		sb.WriteString("  章节: ")
		parts := []string{}
		for secType, count := range structure.Sections {
			parts = append(parts, fmt.Sprintf("%s(%d)", secType, count))
		}
		sort.Strings(parts)
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString("\n")
	}

	if len(structure.Environments) > 0 {
		sb.WriteString(fmt.Sprintf("  环境数量: %d 种\n", len(structure.Environments)))
	}

	if len(structure.Commands) > 0 {
		sb.WriteString(fmt.Sprintf("  关键命令: %d 种\n", len(structure.Commands)))
	}

	return sb.String()
}

// formatDifference 格式化单个差异
func formatDifference(diff StructureDifference) string {
	var action string
	if diff.Difference > 0 {
		action = fmt.Sprintf("增加了 %d 个", diff.Difference)
	} else if diff.Difference < 0 {
		action = fmt.Sprintf("减少了 %d 个", -diff.Difference)
	} else {
		action = "已更改"
	}

	return fmt.Sprintf("    - %s [%s]: %s (原: %d, 译: %d)\n",
		diff.Type, diff.Name, action, diff.OriginalCount, diff.TranslatedCount)
}
