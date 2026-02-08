// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"latex-translator/internal/logger"
)

// StructureType 结构类型枚举
type StructureType int

const (
	StructureCommand     StructureType = iota // \command{...}
	StructureEnvironment                      // \begin{...}...\end{...}
	StructureMathInline                       // $...$
	StructureMathDisplay                      // \[...\] 或 $$...$$
	StructureTable                            // 表格环境
	StructureComment                          // % 注释
)

// String returns the string representation of StructureType
func (st StructureType) String() string {
	switch st {
	case StructureCommand:
		return "Command"
	case StructureEnvironment:
		return "Environment"
	case StructureMathInline:
		return "MathInline"
	case StructureMathDisplay:
		return "MathDisplay"
	case StructureTable:
		return "Table"
	case StructureComment:
		return "Comment"
	default:
		return "Unknown"
	}
}

// LaTeXStructure 表示一个LaTeX结构
type LaTeXStructure struct {
	Type       StructureType // 结构类型：命令、环境、数学等
	Name       string        // 结构名称
	Start      int           // 起始位置
	End        int           // 结束位置
	Content    string        // 原始内容
	IsNested   bool          // 是否嵌套
	ParentType string        // 父结构类型
}

// IdentifyStructures 识别LaTeX内容中的所有结构
// 支持识别命令、环境、数学、表格、注释等结构类型
func IdentifyStructures(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 1. 识别注释 (% 开头的行)
	structures = append(structures, identifyComments(content)...)

	// 2. 识别数学环境 (需要在命令之前识别，避免冲突)
	structures = append(structures, identifyMathEnvironments(content)...)

	// 3. 识别环境 (\begin{...}...\end{...})
	structures = append(structures, identifyEnvironments(content)...)

	// 4. 识别表格环境 (特殊处理)
	structures = append(structures, identifyTables(content)...)

	// 5. 识别命令 (\command{...})
	structures = append(structures, identifyCommands(content)...)

	// 按起始位置排序
	sort.Slice(structures, func(i, j int) bool {
		return structures[i].Start < structures[j].Start
	})

	// 标记嵌套结构
	markNestedStructures(structures)

	logger.Debug("identified LaTeX structures",
		logger.Int("totalStructures", len(structures)),
		logger.Int("contentLength", len(content)))

	return structures
}

// identifyComments 识别注释行
func identifyComments(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	lines := strings.Split(content, "\n")
	pos := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%") {
			// 找到注释在原始内容中的位置
			commentStart := strings.Index(line, "%")
			if commentStart >= 0 {
				structures = append(structures, LaTeXStructure{
					Type:    StructureComment,
					Name:    "comment",
					Start:   pos + commentStart,
					End:     pos + len(line),
					Content: line[commentStart:],
				})
			}
		} else {
			// 检查行内注释 (非转义的 %)
			inlineCommentPos := findUnescapedPercentInLine(line)
			if inlineCommentPos >= 0 {
				structures = append(structures, LaTeXStructure{
					Type:    StructureComment,
					Name:    "inline_comment",
					Start:   pos + inlineCommentPos,
					End:     pos + len(line),
					Content: line[inlineCommentPos:],
				})
			}
		}
		pos += len(line) + 1 // +1 for newline
	}

	return structures
}

// findUnescapedPercentInLine 找到第一个非转义的 % 符号位置
// 注意：postprocessor.go 中已有 findUnescapedPercent 函数，这里使用不同名称避免冲突
func findUnescapedPercentInLine(line string) int {
	i := 0
	for i < len(line) {
		idx := strings.Index(line[i:], "%")
		if idx == -1 {
			return -1
		}

		pos := i + idx

		// 检查是否被转义
		if pos > 0 && line[pos-1] == '\\' {
			// 计算反斜杠数量
			backslashCount := 0
			for j := pos - 1; j >= 0 && line[j] == '\\'; j-- {
				backslashCount++
			}
			if backslashCount%2 == 1 {
				// 奇数个反斜杠表示 % 被转义
				i = pos + 1
				continue
			}
		}

		return pos
	}

	return -1
}

// identifyMathEnvironments 识别数学环境
func identifyMathEnvironments(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 1. 识别 $$...$$ (display math)
	structures = append(structures, identifyDoubleDollarMath(content)...)

	// 2. 识别 $...$ (inline math) - 需要排除 $$
	structures = append(structures, identifySingleDollarMath(content)...)

	// 3. 识别 \[...\] (display math)
	structures = append(structures, identifyBracketMath(content)...)

	// 4. 识别 \(...\) (inline math)
	structures = append(structures, identifyParenMath(content)...)

	// 5. 识别数学环境 (equation, align, gather 等)
	structures = append(structures, identifyMathEnvs(content)...)

	return structures
}

// identifyDoubleDollarMath 识别 $$...$$ 数学环境
func identifyDoubleDollarMath(content string) []LaTeXStructure {
	var structures []LaTeXStructure
	re := regexp.MustCompile(`\$\$[\s\S]*?\$\$`)

	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		structures = append(structures, LaTeXStructure{
			Type:    StructureMathDisplay,
			Name:    "$$",
			Start:   match[0],
			End:     match[1],
			Content: content[match[0]:match[1]],
		})
	}

	return structures
}

// identifySingleDollarMath 识别 $...$ 数学环境 (排除 $$)
func identifySingleDollarMath(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 使用状态机来正确匹配 $...$ 而不是 $$...$$
	i := 0
	for i < len(content) {
		// 跳过 $$
		if i+1 < len(content) && content[i] == '$' && content[i+1] == '$' {
			// 找到 $$ 的结束
			endIdx := strings.Index(content[i+2:], "$$")
			if endIdx >= 0 {
				i = i + 2 + endIdx + 2
			} else {
				i += 2
			}
			continue
		}

		// 检查单个 $
		if content[i] == '$' {
			// 检查是否被转义
			if i > 0 && content[i-1] == '\\' {
				i++
				continue
			}

			// 找到匹配的结束 $
			start := i
			i++
			for i < len(content) {
				if content[i] == '$' {
					// 检查是否是 $$ 或被转义
					if i+1 < len(content) && content[i+1] == '$' {
						i += 2
						continue
					}
					if i > 0 && content[i-1] == '\\' {
						i++
						continue
					}

					// 找到匹配的 $
					structures = append(structures, LaTeXStructure{
						Type:    StructureMathInline,
						Name:    "$",
						Start:   start,
						End:     i + 1,
						Content: content[start : i+1],
					})
					i++
					break
				}
				i++
			}
			continue
		}

		i++
	}

	return structures
}

// identifyBracketMath 识别 \[...\] 数学环境
func identifyBracketMath(content string) []LaTeXStructure {
	var structures []LaTeXStructure
	re := regexp.MustCompile(`\\\[[\s\S]*?\\\]`)

	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		structures = append(structures, LaTeXStructure{
			Type:    StructureMathDisplay,
			Name:    "\\[\\]",
			Start:   match[0],
			End:     match[1],
			Content: content[match[0]:match[1]],
		})
	}

	return structures
}

// identifyParenMath 识别 \(...\) 数学环境
func identifyParenMath(content string) []LaTeXStructure {
	var structures []LaTeXStructure
	re := regexp.MustCompile(`\\\([\s\S]*?\\\)`)

	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		structures = append(structures, LaTeXStructure{
			Type:    StructureMathInline,
			Name:    "\\(\\)",
			Start:   match[0],
			End:     match[1],
			Content: content[match[0]:match[1]],
		})
	}

	return structures
}

// identifyMathEnvs 识别数学环境 (equation, align, gather 等)
func identifyMathEnvs(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	mathEnvs := []string{
		"equation", "equation*",
		"align", "align*",
		"gather", "gather*",
		"multline", "multline*",
		"eqnarray", "eqnarray*",
		"displaymath",
		"math",
		"flalign", "flalign*",
		"alignat", "alignat*",
	}

	for _, env := range mathEnvs {
		structures = append(structures, findEnvironment(content, env, StructureMathDisplay)...)
	}

	return structures
}

// identifyEnvironments 识别通用环境
func identifyEnvironments(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 常见的LaTeX环境
	commonEnvs := []string{
		"document", "abstract", "figure", "figure*",
		"itemize", "enumerate", "description",
		"quote", "quotation", "verse",
		"center", "flushleft", "flushright",
		"minipage", "parbox",
		"verbatim", "verbatim*",
		"lstlisting",
		"theorem", "lemma", "proof", "definition", "corollary", "proposition",
		"example", "remark", "note",
		"appendix", "bibliography",
		"tikzpicture",
		"algorithm", "algorithmic",
		"frame", // beamer
		"block", "alertblock", "exampleblock", // beamer
		"columns", "column", // beamer
		"thebibliography",
		"filecontents", "filecontents*",
	}

	for _, env := range commonEnvs {
		structures = append(structures, findEnvironment(content, env, StructureEnvironment)...)
	}

	// 动态查找其他 \begin{...} 环境
	structures = append(structures, findAllBeginEndEnvironments(content)...)

	return structures
}

// identifyTables 识别表格环境
func identifyTables(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	tableEnvs := []string{
		"table", "table*",
		"tabular", "tabular*",
		"tabularx", "tabulary",
		"longtable", "longtable*",
		"supertabular",
		"array",
		"matrix", "pmatrix", "bmatrix", "vmatrix", "Vmatrix", "Bmatrix",
		"smallmatrix",
	}

	for _, env := range tableEnvs {
		structures = append(structures, findEnvironment(content, env, StructureTable)...)
	}

	return structures
}

// findEnvironment 查找特定环境
func findEnvironment(content string, envName string, structType StructureType) []LaTeXStructure {
	var structures []LaTeXStructure

	// 转义环境名中的特殊字符
	escapedEnv := regexp.QuoteMeta(envName)

	// 匹配 \begin{env}...\end{env}
	// 使用非贪婪匹配，但需要处理嵌套
	beginPattern := `\\begin\{` + escapedEnv + `\}`
	endPattern := `\\end\{` + escapedEnv + `\}`

	beginRe := regexp.MustCompile(beginPattern)
	endRe := regexp.MustCompile(endPattern)

	beginMatches := beginRe.FindAllStringIndex(content, -1)
	endMatches := endRe.FindAllStringIndex(content, -1)

	// 使用栈来匹配嵌套环境
	type envMatch struct {
		start int
		end   int
	}

	var stack []int
	var matched []envMatch

	// 合并并排序所有匹配
	type matchInfo struct {
		pos     int
		isBegin bool
		end     int
	}

	var allMatches []matchInfo
	for _, m := range beginMatches {
		allMatches = append(allMatches, matchInfo{pos: m[0], isBegin: true, end: m[1]})
	}
	for _, m := range endMatches {
		allMatches = append(allMatches, matchInfo{pos: m[0], isBegin: false, end: m[1]})
	}

	sort.Slice(allMatches, func(i, j int) bool {
		return allMatches[i].pos < allMatches[j].pos
	})

	for _, m := range allMatches {
		if m.isBegin {
			stack = append(stack, m.pos)
		} else if len(stack) > 0 {
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			matched = append(matched, envMatch{start: start, end: m.end})
		}
	}

	for _, m := range matched {
		structures = append(structures, LaTeXStructure{
			Type:    structType,
			Name:    envName,
			Start:   m.start,
			End:     m.end,
			Content: content[m.start:m.end],
		})
	}

	return structures
}

// findAllBeginEndEnvironments 动态查找所有 \begin{...}\end{...} 环境
func findAllBeginEndEnvironments(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 找到所有 \begin{...} 并提取环境名
	beginRe := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	matches := beginRe.FindAllStringSubmatch(content, -1)

	// 收集所有唯一的环境名
	envNames := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			envNames[match[1]] = true
		}
	}

	// 对每个环境名查找完整的环境
	for envName := range envNames {
		// 跳过已经在其他函数中处理的环境
		if isKnownEnvironment(envName) {
			continue
		}
		structures = append(structures, findEnvironment(content, envName, StructureEnvironment)...)
	}

	return structures
}

// isKnownEnvironment 检查是否是已知的环境类型
func isKnownEnvironment(envName string) bool {
	knownEnvs := map[string]bool{
		// 数学环境
		"equation": true, "equation*": true,
		"align": true, "align*": true,
		"gather": true, "gather*": true,
		"multline": true, "multline*": true,
		"eqnarray": true, "eqnarray*": true,
		"displaymath": true, "math": true,
		"flalign": true, "flalign*": true,
		"alignat": true, "alignat*": true,
		// 表格环境
		"table": true, "table*": true,
		"tabular": true, "tabular*": true,
		"tabularx": true, "tabulary": true,
		"longtable": true, "longtable*": true,
		"supertabular": true, "array": true,
		"matrix": true, "pmatrix": true, "bmatrix": true,
		"vmatrix": true, "Vmatrix": true, "Bmatrix": true,
		"smallmatrix": true,
		// 通用环境
		"document": true, "abstract": true,
		"figure": true, "figure*": true,
		"itemize": true, "enumerate": true, "description": true,
		"quote": true, "quotation": true, "verse": true,
		"center": true, "flushleft": true, "flushright": true,
		"minipage": true, "parbox": true,
		"verbatim": true, "verbatim*": true,
		"lstlisting": true,
		"theorem": true, "lemma": true, "proof": true,
		"definition": true, "corollary": true, "proposition": true,
		"example": true, "remark": true, "note": true,
		"appendix": true, "bibliography": true,
		"tikzpicture": true,
		"algorithm": true, "algorithmic": true,
		"frame": true, "block": true, "alertblock": true, "exampleblock": true,
		"columns": true, "column": true,
		"thebibliography": true,
		"filecontents": true, "filecontents*": true,
	}

	return knownEnvs[envName]
}

// identifyCommands 识别LaTeX命令
func identifyCommands(content string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 常见的需要保护的命令
	commands := []string{
		// 文档结构命令
		"documentclass", "usepackage", "input", "include", "includegraphics",
		"section", "subsection", "subsubsection", "paragraph", "subparagraph",
		"chapter", "part",
		// 引用命令
		"cite", "citep", "citet", "citealp", "citeauthor", "citeyear",
		"ref", "eqref", "pageref", "autoref", "nameref",
		"label", "caption", "footnote", "footnotemark", "footnotetext",
		// 格式命令
		"textbf", "textit", "texttt", "textrm", "textsf", "textsc",
		"emph", "underline", "overline",
		"tiny", "scriptsize", "footnotesize", "small", "normalsize",
		"large", "Large", "LARGE", "huge", "Huge",
		// 数学命令
		"frac", "sqrt", "sum", "prod", "int", "oint",
		"lim", "limsup", "liminf", "max", "min", "sup", "inf",
		"sin", "cos", "tan", "cot", "sec", "csc",
		"log", "ln", "exp",
		"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta",
		"iota", "kappa", "lambda", "mu", "nu", "xi", "pi", "rho",
		"sigma", "tau", "upsilon", "phi", "chi", "psi", "omega",
		// 表格命令
		"multirow", "multicolumn", "hline", "cline",
		"toprule", "midrule", "bottomrule",
		// 其他常用命令
		"newcommand", "renewcommand", "newenvironment", "renewenvironment",
		"def", "let", "setlength", "addtolength",
		"vspace", "hspace", "vfill", "hfill",
		"newline", "linebreak", "pagebreak", "newpage", "clearpage",
		"item",
		"title", "author", "date", "maketitle",
		"tableofcontents", "listoffigures", "listoftables",
		"appendix", "bibliography", "bibliographystyle",
		"url", "href",
	}

	for _, cmd := range commands {
		structures = append(structures, findCommand(content, cmd)...)
	}

	return structures
}

// findCommand 查找特定命令
func findCommand(content string, cmdName string) []LaTeXStructure {
	var structures []LaTeXStructure

	// 转义命令名中的特殊字符
	escapedCmd := regexp.QuoteMeta(cmdName)

	// 匹配 \command 或 \command{...} 或 \command[...]{...}
	// 简化版本：只匹配命令名和可选的第一个参数
	pattern := `\\` + escapedCmd + `(?:\[[^\]]*\])?(?:\{[^}]*\})?`
	re := regexp.MustCompile(pattern)

	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		structures = append(structures, LaTeXStructure{
			Type:    StructureCommand,
			Name:    cmdName,
			Start:   match[0],
			End:     match[1],
			Content: content[match[0]:match[1]],
		})
	}

	return structures
}

// markNestedStructures 标记嵌套结构
func markNestedStructures(structures []LaTeXStructure) {
	for i := range structures {
		for j := range structures {
			if i == j {
				continue
			}
			// 检查 structures[i] 是否在 structures[j] 内部
			if structures[i].Start > structures[j].Start && structures[i].End < structures[j].End {
				structures[i].IsNested = true
				structures[i].ParentType = structures[j].Type.String()
				break
			}
		}
	}
}

// GetStructuresByType 按类型获取结构
func GetStructuresByType(structures []LaTeXStructure, structType StructureType) []LaTeXStructure {
	var result []LaTeXStructure
	for _, s := range structures {
		if s.Type == structType {
			result = append(result, s)
		}
	}
	return result
}

// GetMathStructures 获取所有数学结构
func GetMathStructures(structures []LaTeXStructure) []LaTeXStructure {
	var result []LaTeXStructure
	for _, s := range structures {
		if s.Type == StructureMathInline || s.Type == StructureMathDisplay {
			result = append(result, s)
		}
	}
	return result
}

// GetTableStructures 获取所有表格结构
func GetTableStructures(structures []LaTeXStructure) []LaTeXStructure {
	return GetStructuresByType(structures, StructureTable)
}

// GetCommentStructures 获取所有注释结构
func GetCommentStructures(structures []LaTeXStructure) []LaTeXStructure {
	return GetStructuresByType(structures, StructureComment)
}

// PreprocessOptions contains options for preprocessing LaTeX content.
type PreprocessOptions struct {
	// RemoveComments removes all LaTeX comments (lines starting with %)
	RemoveComments bool
	// PreserveImportantComments keeps comments that contain important markers
	// like TODO, FIXME, NOTE, or are part of special comment blocks
	PreserveImportantComments bool
	// RemoveEmptyLines removes consecutive empty lines (keeps at most one)
	RemoveEmptyLines bool
}

// DefaultPreprocessOptions returns the default preprocessing options.
func DefaultPreprocessOptions() PreprocessOptions {
	return PreprocessOptions{
		RemoveComments:            true,
		PreserveImportantComments: false,
		RemoveEmptyLines:          true,
	}
}

// PreprocessLaTeX preprocesses LaTeX content before translation.
// This removes comments and cleans up the content to reduce LLM processing
// and avoid comment-related formatting issues.
func PreprocessLaTeX(content string, opts PreprocessOptions) string {
	if !opts.RemoveComments && !opts.RemoveEmptyLines {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	lastWasEmpty := false

	for _, line := range lines {
		processedLine := line

		if opts.RemoveComments {
			processedLine = removeLineComment(line, opts.PreserveImportantComments)
		}

		// Handle empty lines
		trimmed := strings.TrimSpace(processedLine)
		if trimmed == "" {
			if opts.RemoveEmptyLines {
				if !lastWasEmpty {
					result = append(result, "")
					lastWasEmpty = true
				}
				// Skip consecutive empty lines
				continue
			}
		} else {
			lastWasEmpty = false
		}

		result = append(result, processedLine)
	}

	processed := strings.Join(result, "\n")
	
	logger.Debug("preprocessed LaTeX content",
		logger.Int("originalLength", len(content)),
		logger.Int("processedLength", len(processed)),
		logger.Int("originalLines", len(lines)),
		logger.Int("processedLines", len(result)))

	return processed
}

// removeLineComment removes the comment portion from a line.
// It handles several cases:
// 1. Full line comments (line starts with %)
// 2. Inline comments (text followed by % comment)
// 3. Escaped percent signs (\%)
func removeLineComment(line string, preserveImportant bool) string {
	// Check if this is a full line comment
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "%") {
		// Check if it's an important comment to preserve
		if preserveImportant && isImportantComment(trimmed) {
			return line
		}
		// Remove full line comment
		return ""
	}

	// Handle inline comments
	// We need to find % that is not escaped (\%)
	result := removeInlineComment(line)
	return result
}

// removeInlineComment removes inline comments from a line.
// It preserves escaped percent signs (\%).
func removeInlineComment(line string) string {
	// Find the first unescaped % character
	i := 0
	for i < len(line) {
		idx := strings.Index(line[i:], "%")
		if idx == -1 {
			// No % found
			return line
		}
		
		pos := i + idx
		
		// Check if it's escaped
		if pos > 0 && line[pos-1] == '\\' {
			// Check if the backslash itself is escaped
			backslashCount := 0
			for j := pos - 1; j >= 0 && line[j] == '\\'; j-- {
				backslashCount++
			}
			if backslashCount%2 == 1 {
				// Odd number of backslashes means % is escaped
				i = pos + 1
				continue
			}
		}
		
		// Found unescaped %, remove everything after it
		return strings.TrimRight(line[:pos], " \t")
	}
	
	return line
}

// isImportantComment checks if a comment contains important markers.
func isImportantComment(comment string) bool {
	upper := strings.ToUpper(comment)
	importantMarkers := []string{"TODO", "FIXME", "NOTE", "XXX", "HACK", "BUG", "WARNING"}
	
	for _, marker := range importantMarkers {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	
	return false
}

// PreprocessAllFiles preprocesses all LaTeX files in a map.
// This is useful for preprocessing multiple files before translation.
func PreprocessAllFiles(files map[string]string, opts PreprocessOptions) map[string]string {
	result := make(map[string]string, len(files))
	
	for path, content := range files {
		result[path] = PreprocessLaTeX(content, opts)
	}
	
	return result
}

// RemoveCommentsOnly is a convenience function that only removes comments.
func RemoveCommentsOnly(content string) string {
	return PreprocessLaTeX(content, PreprocessOptions{
		RemoveComments:            true,
		PreserveImportantComments: false,
		RemoveEmptyLines:          false,
	})
}

// CleanupContent is a convenience function that removes comments and empty lines.
func CleanupContent(content string) string {
	return PreprocessLaTeX(content, DefaultPreprocessOptions())
}


// MathPlaceholder represents a math environment placeholder
type MathPlaceholder struct {
	Placeholder string // The placeholder string (e.g., <<<LATEX_MATH_0>>>)
	Original    string // The original math content
	Type        string // Type of math: "inline", "display", "environment"
	Start       int    // Start position in original content
	End         int    // End position in original content
}

// ProtectMathEnvironments protects all mathematical content in LaTeX text.
// It replaces math environments with unique placeholders in the format <<<LATEX_MATH_N>>>.
//
// Supported formats:
// - Inline math: $...$ and \(...\)
// - Display math: $$...$$ and \[...\]
// - Math environments: equation, align, gather, multline, eqnarray, etc.
//
// Returns the protected content and a map of placeholders to original content.
//
// Validates: Requirements 1.3
func ProtectMathEnvironments(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	
	// Collect all math regions with their positions
	var mathRegions []MathPlaceholder
	
	// 1. Find math environments first (highest priority - they can contain other patterns)
	mathRegions = append(mathRegions, findMathEnvironmentRegions(content)...)
	
	// 2. Find display math $$...$$ (before inline $...$)
	mathRegions = append(mathRegions, findDoubleDollarRegions(content)...)
	
	// 3. Find bracket math \[...\]
	mathRegions = append(mathRegions, findBracketMathRegions(content)...)
	
	// 4. Find parenthesis math \(...\)
	mathRegions = append(mathRegions, findParenMathRegions(content)...)
	
	// 5. Find inline math $...$ (lowest priority)
	mathRegions = append(mathRegions, findSingleDollarRegions(content)...)
	
	// Remove overlapping regions (keep the first/larger one)
	mathRegions = removeOverlappingMathRegions(mathRegions)
	
	// Sort by position (descending) for safe replacement
	sort.Slice(mathRegions, func(i, j int) bool {
		return mathRegions[i].Start > mathRegions[j].Start
	})
	
	// Replace math content with placeholders
	result := content
	for i, region := range mathRegions {
		placeholder := fmt.Sprintf("<<<LATEX_MATH_%d>>>", len(mathRegions)-1-i)
		placeholders[placeholder] = region.Original
		result = result[:region.Start] + placeholder + result[region.End:]
	}
	
	logger.Debug("protected math environments",
		logger.Int("totalRegions", len(mathRegions)),
		logger.Int("originalLength", len(content)),
		logger.Int("protectedLength", len(result)))
	
	return result, placeholders
}

// findMathEnvironmentRegions finds all named math environments
func findMathEnvironmentRegions(content string) []MathPlaceholder {
	var regions []MathPlaceholder
	
	// List of math environment names
	mathEnvs := []string{
		"equation", "equation*",
		"align", "align*",
		"alignat", "alignat*",
		"gather", "gather*",
		"multline", "multline*",
		"flalign", "flalign*",
		"eqnarray", "eqnarray*",
		"math", "displaymath",
		"split", "subequations",
		"cases",
		// Matrix environments
		"matrix", "pmatrix", "bmatrix", "vmatrix", "Vmatrix", "Bmatrix",
		"smallmatrix",
	}
	
	for _, envName := range mathEnvs {
		regions = append(regions, findEnvironmentRegions(content, envName, "environment")...)
	}
	
	return regions
}

// findEnvironmentRegions finds all occurrences of a specific environment
func findEnvironmentRegions(content string, envName string, mathType string) []MathPlaceholder {
	var regions []MathPlaceholder
	
	beginTag := fmt.Sprintf("\\begin{%s}", envName)
	
	startIdx := 0
	for {
		// Find the begin tag
		beginPos := strings.Index(content[startIdx:], beginTag)
		if beginPos == -1 {
			break
		}
		beginPos += startIdx
		
		// Find the matching end tag (handle nested environments)
		endPos := findMathEnvironmentEnd(content, beginPos, envName)
		if endPos == -1 {
			startIdx = beginPos + len(beginTag)
			continue
		}
		
		regions = append(regions, MathPlaceholder{
			Original: content[beginPos:endPos],
			Type:     mathType,
			Start:    beginPos,
			End:      endPos,
		})
		
		startIdx = endPos
	}
	
	return regions
}

// findMathEnvironmentEnd finds the matching \end{envName} for a \begin{envName}
func findMathEnvironmentEnd(content string, startPos int, envName string) int {
	beginTag := fmt.Sprintf("\\begin{%s}", envName)
	endTag := fmt.Sprintf("\\end{%s}", envName)
	
	depth := 1
	searchPos := startPos + len(beginTag)
	
	for depth > 0 && searchPos < len(content) {
		nextBegin := strings.Index(content[searchPos:], beginTag)
		nextEnd := strings.Index(content[searchPos:], endTag)
		
		if nextEnd == -1 {
			return -1
		}
		
		if nextBegin != -1 && nextBegin < nextEnd {
			depth++
			searchPos += nextBegin + len(beginTag)
		} else {
			depth--
			if depth == 0 {
				return searchPos + nextEnd + len(endTag)
			}
			searchPos += nextEnd + len(endTag)
		}
	}
	
	return -1
}

// findDoubleDollarRegions finds all $$...$$ display math regions
func findDoubleDollarRegions(content string) []MathPlaceholder {
	var regions []MathPlaceholder
	
	// Use a more robust pattern that handles multiline content
	re := regexp.MustCompile(`\$\$[\s\S]*?\$\$`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, MathPlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "display",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findBracketMathRegions finds all \[...\] display math regions
func findBracketMathRegions(content string) []MathPlaceholder {
	var regions []MathPlaceholder
	
	re := regexp.MustCompile(`\\\[[\s\S]*?\\\]`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, MathPlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "display",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findParenMathRegions finds all \(...\) inline math regions
func findParenMathRegions(content string) []MathPlaceholder {
	var regions []MathPlaceholder
	
	re := regexp.MustCompile(`\\\([\s\S]*?\\\)`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, MathPlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "inline",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findSingleDollarRegions finds all $...$ inline math regions
// This is more complex because we need to:
// 1. Avoid matching $$...$$ (already handled)
// 2. Handle escaped dollar signs \$
// 3. Handle nested content properly
func findSingleDollarRegions(content string) []MathPlaceholder {
	var regions []MathPlaceholder
	
	i := 0
	for i < len(content) {
		// Skip escaped dollar signs
		if i > 0 && content[i-1] == '\\' && content[i] == '$' {
			i++
			continue
		}
		
		// Skip $$ (display math)
		if i+1 < len(content) && content[i] == '$' && content[i+1] == '$' {
			// Find the end of $$...$$
			endIdx := strings.Index(content[i+2:], "$$")
			if endIdx >= 0 {
				i = i + 2 + endIdx + 2
			} else {
				i += 2
			}
			continue
		}
		
		// Found a single $
		if content[i] == '$' {
			start := i
			i++
			
			// Find the matching closing $
			for i < len(content) {
				// Skip escaped dollar signs
				if content[i] == '\\' && i+1 < len(content) && content[i+1] == '$' {
					i += 2
					continue
				}
				
				// Skip $$ inside (shouldn't happen in valid LaTeX, but be safe)
				if i+1 < len(content) && content[i] == '$' && content[i+1] == '$' {
					i += 2
					continue
				}
				
				// Found closing $
				if content[i] == '$' {
					// Make sure it's not empty ($$ would be display math)
					if i > start+1 {
						regions = append(regions, MathPlaceholder{
							Original: content[start : i+1],
							Type:     "inline",
							Start:    start,
							End:      i + 1,
						})
					}
					i++
					break
				}
				
				i++
			}
			continue
		}
		
		i++
	}
	
	return regions
}

// removeOverlappingMathRegions removes overlapping regions, keeping the first one found
// (which is typically the larger/more specific one due to processing order)
func removeOverlappingMathRegions(regions []MathPlaceholder) []MathPlaceholder {
	if len(regions) <= 1 {
		return regions
	}
	
	// Sort by start position, then by length (descending)
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Start == regions[j].Start {
			return regions[i].End-regions[i].Start > regions[j].End-regions[j].Start
		}
		return regions[i].Start < regions[j].Start
	})
	
	var result []MathPlaceholder
	lastEnd := -1
	
	for _, region := range regions {
		// Skip if this region overlaps with the previous one
		if region.Start < lastEnd {
			continue
		}
		result = append(result, region)
		lastEnd = region.End
	}
	
	return result
}

// RestoreMathEnvironments restores the original math content from placeholders.
// It takes the translated content and the placeholder map from ProtectMathEnvironments.
//
// Validates: Requirements 1.3
func RestoreMathEnvironments(content string, placeholders map[string]string) string {
	result := content
	for placeholder, original := range placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

// ProtectAllMath is a convenience function that protects all math environments
// and returns both the protected content and a combined placeholder map.
// This function is designed to be used with ProtectLaTeXCommands for comprehensive protection.
//
// Validates: Requirements 1.3
func ProtectAllMath(content string) (string, map[string]string) {
	return ProtectMathEnvironments(content)
}

// GetMathPlaceholderCount returns the number of math placeholders in the content
func GetMathPlaceholderCount(content string) int {
	re := regexp.MustCompile(`<<<LATEX_MATH_\d+>>>`)
	return len(re.FindAllString(content, -1))
}

// ValidateMathPlaceholders checks if all math placeholders are present in the content
// Returns a list of missing placeholders
func ValidateMathPlaceholders(content string, placeholders map[string]string) []string {
	var missing []string
	for placeholder := range placeholders {
		if !strings.Contains(content, placeholder) {
			missing = append(missing, placeholder)
		}
	}
	return missing
}

// TablePlaceholder represents a table structure placeholder
type TablePlaceholder struct {
	Placeholder string // The placeholder string (e.g., <<<LATEX_TABLE_0>>>)
	Original    string // The original table content
	Type        string // Type of table: "environment", "command", "separator"
	Start       int    // Start position in original content
	End         int    // End position in original content
}

// ProtectTableStructures protects all table structures in LaTeX text.
// It replaces table environments and commands with unique placeholders in the format <<<LATEX_TABLE_N>>>.
//
// Supported table environments:
// - tabular, tabular*, tabularx, tabulary
// - table, table*
// - longtable, longtable*
// - array
//
// Supported table commands:
// - \multirow, \multicolumn
// - \cline, \hline
// - \toprule, \midrule, \bottomrule
//
// Also protects:
// - Cell separators: &
// - Row terminators: \\
//
// Returns the protected content and a map of placeholders to original content.
//
// Validates: Requirements 1.4
func ProtectTableStructures(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	
	// Collect all table regions with their positions
	var tableRegions []TablePlaceholder
	
	// 1. Find table environments first (highest priority - they contain other elements)
	tableRegions = append(tableRegions, findTableEnvironmentRegions(content)...)
	
	// Remove overlapping regions (keep the larger/outer ones)
	tableRegions = removeOverlappingTableRegions(tableRegions)
	
	// Sort by position (descending) for safe replacement
	sort.Slice(tableRegions, func(i, j int) bool {
		return tableRegions[i].Start > tableRegions[j].Start
	})
	
	// Replace table content with placeholders
	result := content
	for i, region := range tableRegions {
		placeholder := fmt.Sprintf("<<<LATEX_TABLE_%d>>>", len(tableRegions)-1-i)
		placeholders[placeholder] = region.Original
		result = result[:region.Start] + placeholder + result[region.End:]
	}
	
	logger.Debug("protected table structures",
		logger.Int("totalRegions", len(tableRegions)),
		logger.Int("originalLength", len(content)),
		logger.Int("protectedLength", len(result)))
	
	return result, placeholders
}

// findTableEnvironmentRegions finds all table environment regions
func findTableEnvironmentRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// List of table environment names
	tableEnvs := []string{
		"table", "table*",
		"tabular", "tabular*",
		"tabularx", "tabulary",
		"longtable", "longtable*",
		"array",
		"supertabular", "supertabular*",
		"xtabular", "xtabular*",
		"tabu", "longtabu",
	}
	
	for _, envName := range tableEnvs {
		regions = append(regions, findTableEnvRegions(content, envName)...)
	}
	
	return regions
}

// findTableEnvRegions finds all occurrences of a specific table environment
func findTableEnvRegions(content string, envName string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	beginTag := fmt.Sprintf("\\begin{%s}", envName)
	
	startIdx := 0
	for {
		// Find the begin tag
		beginPos := strings.Index(content[startIdx:], beginTag)
		if beginPos == -1 {
			break
		}
		beginPos += startIdx
		
		// Find the matching end tag (handle nested environments)
		endPos := findTableEnvironmentEnd(content, beginPos, envName)
		if endPos == -1 {
			startIdx = beginPos + len(beginTag)
			continue
		}
		
		regions = append(regions, TablePlaceholder{
			Original: content[beginPos:endPos],
			Type:     "environment",
			Start:    beginPos,
			End:      endPos,
		})
		
		startIdx = endPos
	}
	
	return regions
}

// findTableEnvironmentEnd finds the matching \end{envName} for a \begin{envName}
func findTableEnvironmentEnd(content string, startPos int, envName string) int {
	beginTag := fmt.Sprintf("\\begin{%s}", envName)
	endTag := fmt.Sprintf("\\end{%s}", envName)
	
	depth := 1
	searchPos := startPos + len(beginTag)
	
	for depth > 0 && searchPos < len(content) {
		nextBegin := strings.Index(content[searchPos:], beginTag)
		nextEnd := strings.Index(content[searchPos:], endTag)
		
		if nextEnd == -1 {
			return -1
		}
		
		if nextBegin != -1 && nextBegin < nextEnd {
			depth++
			searchPos += nextBegin + len(beginTag)
		} else {
			depth--
			if depth == 0 {
				return searchPos + nextEnd + len(endTag)
			}
			searchPos += nextEnd + len(endTag)
		}
	}
	
	return -1
}

// removeOverlappingTableRegions removes overlapping regions, keeping the larger/outer one
func removeOverlappingTableRegions(regions []TablePlaceholder) []TablePlaceholder {
	if len(regions) <= 1 {
		return regions
	}
	
	// Sort by start position, then by length (descending) to prefer larger regions
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Start == regions[j].Start {
			return regions[i].End-regions[i].Start > regions[j].End-regions[j].Start
		}
		return regions[i].Start < regions[j].Start
	})
	
	var result []TablePlaceholder
	lastEnd := -1
	
	for _, region := range regions {
		// Skip if this region overlaps with the previous one
		if region.Start < lastEnd {
			continue
		}
		result = append(result, region)
		lastEnd = region.End
	}
	
	return result
}

// RestoreTableStructures restores the original table content from placeholders.
// It takes the translated content and the placeholder map from ProtectTableStructures.
//
// Validates: Requirements 1.4
func RestoreTableStructures(content string, placeholders map[string]string) string {
	result := content
	for placeholder, original := range placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

// ProtectAllTables is a convenience function that protects all table structures
// and returns both the protected content and a combined placeholder map.
// This function is designed to be used with ProtectLaTeXCommands for comprehensive protection.
//
// Validates: Requirements 1.4
func ProtectAllTables(content string) (string, map[string]string) {
	return ProtectTableStructures(content)
}

// GetTablePlaceholderCount returns the number of table placeholders in the content
func GetTablePlaceholderCount(content string) int {
	re := regexp.MustCompile(`<<<LATEX_TABLE_\d+>>>`)
	return len(re.FindAllString(content, -1))
}

// ValidateTablePlaceholders checks if all table placeholders are present in the content
// Returns a list of missing placeholders
func ValidateTablePlaceholders(content string, placeholders map[string]string) []string {
	var missing []string
	for placeholder := range placeholders {
		if !strings.Contains(content, placeholder) {
			missing = append(missing, placeholder)
		}
	}
	return missing
}

// ProtectTableCommands protects individual table commands within content.
// This is useful when you want to protect table commands without protecting entire environments.
// Commands protected: \multirow, \multicolumn, \cline, \hline, \toprule, \midrule, \bottomrule
//
// Validates: Requirements 1.4
func ProtectTableCommands(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	
	var regions []TablePlaceholder
	
	// Find table commands
	regions = append(regions, findMultirowRegions(content)...)
	regions = append(regions, findMulticolumnRegions(content)...)
	regions = append(regions, findClineRegions(content)...)
	regions = append(regions, findHlineRegions(content)...)
	regions = append(regions, findBooktabsRegions(content)...)
	
	// Remove overlapping regions
	regions = removeOverlappingTableRegions(regions)
	
	// Sort by position (descending) for safe replacement
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Start > regions[j].Start
	})
	
	// Replace with placeholders
	result := content
	for i, region := range regions {
		placeholder := fmt.Sprintf("<<<LATEX_TBLCMD_%d>>>", len(regions)-1-i)
		placeholders[placeholder] = region.Original
		result = result[:region.Start] + placeholder + result[region.End:]
	}
	
	return result, placeholders
}

// findMultirowRegions finds all \multirow{...}{...}{...} commands
func findMultirowRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// \multirow{nrows}[bigstruts]{width}[fixup]{text}
	// Simplified pattern: \multirow{...}{...}{...} with optional arguments
	re := regexp.MustCompile(`\\multirow\s*(?:\[[^\]]*\])?\s*\{[^}]*\}\s*(?:\[[^\]]*\])?\s*\{[^}]*\}\s*(?:\[[^\]]*\])?\s*\{[^}]*\}`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, TablePlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "command",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findMulticolumnRegions finds all \multicolumn{...}{...}{...} commands
func findMulticolumnRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// \multicolumn{ncols}{cols}{text}
	re := regexp.MustCompile(`\\multicolumn\s*\{[^}]*\}\s*\{[^}]*\}\s*\{[^}]*\}`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, TablePlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "command",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findClineRegions finds all \cline{...} commands
func findClineRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// \cline{i-j}
	re := regexp.MustCompile(`\\cline\s*\{[^}]*\}`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, TablePlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "command",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findHlineRegions finds all \hline commands
func findHlineRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// \hline (no arguments)
	re := regexp.MustCompile(`\\hline\b`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, TablePlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "command",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// findBooktabsRegions finds all booktabs commands (\toprule, \midrule, \bottomrule)
func findBooktabsRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// \toprule, \midrule, \bottomrule with optional arguments
	re := regexp.MustCompile(`\\(toprule|midrule|bottomrule)(?:\s*\[[^\]]*\])?`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		regions = append(regions, TablePlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "command",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// RestoreTableCommands restores the original table commands from placeholders.
//
// Validates: Requirements 1.4
func RestoreTableCommands(content string, placeholders map[string]string) string {
	result := content
	for placeholder, original := range placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

// ProtectCellSeparators protects cell separators (&) and row terminators (\\) in table content.
// This should be used carefully as & and \\ have meaning outside tables too.
// It's recommended to use this only on content that is known to be inside a table.
//
// Validates: Requirements 1.4
func ProtectCellSeparators(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	
	var regions []TablePlaceholder
	
	// Find cell separators (&) - but not \& (escaped)
	regions = append(regions, findCellSeparatorRegions(content)...)
	
	// Find row terminators (\\) - but not \\\ (escaped) or \\\\ (double line break)
	regions = append(regions, findRowTerminatorRegions(content)...)
	
	// Remove overlapping regions
	regions = removeOverlappingTableRegions(regions)
	
	// Sort by position (descending) for safe replacement
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Start > regions[j].Start
	})
	
	// Replace with placeholders
	result := content
	for i, region := range regions {
		placeholder := fmt.Sprintf("<<<LATEX_SEP_%d>>>", len(regions)-1-i)
		placeholders[placeholder] = region.Original
		result = result[:region.Start] + placeholder + result[region.End:]
	}
	
	return result, placeholders
}

// findCellSeparatorRegions finds all unescaped & characters
func findCellSeparatorRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	for i := 0; i < len(content); i++ {
		if content[i] == '&' {
			// Check if it's escaped (\&)
			if i > 0 && content[i-1] == '\\' {
				// Count backslashes to handle \\&
				backslashCount := 0
				for j := i - 1; j >= 0 && content[j] == '\\'; j-- {
					backslashCount++
				}
				if backslashCount%2 == 1 {
					// Odd number of backslashes means & is escaped
					continue
				}
			}
			
			regions = append(regions, TablePlaceholder{
				Original: "&",
				Type:     "separator",
				Start:    i,
				End:      i + 1,
			})
		}
	}
	
	return regions
}

// findRowTerminatorRegions finds all \\ row terminators
func findRowTerminatorRegions(content string) []TablePlaceholder {
	var regions []TablePlaceholder
	
	// Match \\ optionally followed by [length] for spacing
	re := regexp.MustCompile(`\\\\(?:\s*\[[^\]]*\])?`)
	
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		// Check if this is actually a row terminator and not part of a longer sequence
		// Skip if preceded by another backslash (would be \\\\ which is different)
		if match[0] > 0 && content[match[0]-1] == '\\' {
			continue
		}
		
		regions = append(regions, TablePlaceholder{
			Original: content[match[0]:match[1]],
			Type:     "separator",
			Start:    match[0],
			End:      match[1],
		})
	}
	
	return regions
}

// RestoreCellSeparators restores the original cell separators from placeholders.
//
// Validates: Requirements 1.4
func RestoreCellSeparators(content string, placeholders map[string]string) string {
	result := content
	for placeholder, original := range placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

// ProtectCompleteTable provides comprehensive table protection by:
// 1. Protecting entire table environments
// 2. Within non-protected areas, protecting table commands
// 3. Within non-protected areas, protecting cell separators
//
// This is the recommended function for complete table protection.
//
// Validates: Requirements 1.4
func ProtectCompleteTable(content string) (string, map[string]string) {
	allPlaceholders := make(map[string]string)
	
	// Step 1: Protect table environments (highest priority)
	result, envPlaceholders := ProtectTableStructures(content)
	for k, v := range envPlaceholders {
		allPlaceholders[k] = v
	}
	
	// Step 2: Protect table commands in remaining content
	result, cmdPlaceholders := ProtectTableCommands(result)
	for k, v := range cmdPlaceholders {
		allPlaceholders[k] = v
	}
	
	// Note: We don't protect cell separators by default as they may appear
	// in non-table contexts. Use ProtectCellSeparators explicitly if needed.
	
	logger.Debug("protected complete table structures",
		logger.Int("envPlaceholders", len(envPlaceholders)),
		logger.Int("cmdPlaceholders", len(cmdPlaceholders)),
		logger.Int("totalPlaceholders", len(allPlaceholders)))
	
	return result, allPlaceholders
}

// RestoreCompleteTable restores all table-related placeholders.
//
// Validates: Requirements 1.4
func RestoreCompleteTable(content string, placeholders map[string]string) string {
	result := content
	for placeholder, original := range placeholders {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

// ProtectAuthorBlock protects the \author{} block from translation.
// Author names should not be translated as they are proper nouns.
// This function finds \author{...} blocks and replaces them with placeholders.
//
// Validates: Requirements 1.4 (preserve LaTeX structure)
func ProtectAuthorBlock(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	result := content
	
	// Find \author{ and extract the complete block with balanced braces
	authorPattern := regexp.MustCompile(`\\author\s*\{`)
	
	matches := authorPattern.FindAllStringIndex(result, -1)
	if len(matches) == 0 {
		return content, placeholders
	}
	
	// Process matches in reverse order to maintain correct positions
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		startPos := match[0]
		braceStart := match[1] - 1 // Position of the opening {
		
		// Find the matching closing brace
		braceCount := 1
		endPos := braceStart + 1
		for endPos < len(result) && braceCount > 0 {
			if result[endPos] == '{' {
				braceCount++
			} else if result[endPos] == '}' {
				braceCount--
			}
			endPos++
		}
		
		if braceCount != 0 {
			// Unbalanced braces, skip this match
			continue
		}
		
		// Extract the complete \author{...} block
		authorBlock := result[startPos:endPos]
		placeholder := fmt.Sprintf("<<<LATEX_AUTHOR_%d>>>", i)
		placeholders[placeholder] = authorBlock
		
		// Replace with placeholder
		result = result[:startPos] + placeholder + result[endPos:]
		
		logger.Debug("protected author block",
			logger.Int("index", i),
			logger.Int("length", len(authorBlock)))
	}
	
	logger.Debug("protected author blocks",
		logger.Int("count", len(placeholders)))
	
	return result, placeholders
}

// ProtectTitleBlock protects the \title{} block from translation.
// While titles may sometimes need translation, they often contain
// technical terms or proper nouns that should be preserved.
// This function finds \title{...} blocks and replaces them with placeholders.
//
// Validates: Requirements 1.4 (preserve LaTeX structure)
func ProtectTitleBlock(content string) (string, map[string]string) {
	placeholders := make(map[string]string)
	result := content
	
	// Find \title{ and extract the complete block with balanced braces
	titlePattern := regexp.MustCompile(`\\title\s*\{`)
	
	matches := titlePattern.FindAllStringIndex(result, -1)
	if len(matches) == 0 {
		return content, placeholders
	}
	
	// Process matches in reverse order to maintain correct positions
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		startPos := match[0]
		braceStart := match[1] - 1 // Position of the opening {
		
		// Find the matching closing brace
		braceCount := 1
		endPos := braceStart + 1
		for endPos < len(result) && braceCount > 0 {
			if result[endPos] == '{' {
				braceCount++
			} else if result[endPos] == '}' {
				braceCount--
			}
			endPos++
		}
		
		if braceCount != 0 {
			// Unbalanced braces, skip this match
			continue
		}
		
		// Extract the complete \title{...} block
		titleBlock := result[startPos:endPos]
		placeholder := fmt.Sprintf("<<<LATEX_TITLE_%d>>>", i)
		placeholders[placeholder] = titleBlock
		
		// Replace with placeholder
		result = result[:startPos] + placeholder + result[endPos:]
		
		logger.Debug("protected title block",
			logger.Int("index", i),
			logger.Int("length", len(titleBlock)))
	}
	
	logger.Debug("protected title blocks",
		logger.Int("count", len(placeholders)))
	
	return result, placeholders
}
