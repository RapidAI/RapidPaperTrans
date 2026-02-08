// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"fmt"
	"regexp"
	"strings"

	"latex-translator/internal/logger"
)

// PostprocessChunk performs post-processing on a translated chunk by comparing
// it with the original English content. This fixes common LLM translation issues.
//
// Parameters:
//   - translated: the translated content from LLM
//   - original: the original English content (for reference)
//
// Returns the fixed translated content.
func PostprocessChunk(translated, original string) string {
	if translated == "" {
		return translated
	}

	result := translated

	// 1. Restore line structure using enhanced function (Requirements 2.1-2.5)
	// This ensures \begin{...}, \end{...}, and \item are on separate lines
	// and attempts to match the original line structure
	result = RestoreLineStructure(result, original)

	// 2. Restore comment formatting (Requirements 5.2, 5.3, 5.5)
	// This restores % symbols and re-comments environment tags
	result = RestoreComments(result, original)

	// 3. Fix mismatched comment markers on begin/end pairs
	// This must be done BEFORE environment matching to ensure proper counting
	result = fixMismatchedCommentedEnvironments(result, original)

	// 4. Restore environment tags (Requirements 3.3, 3.4)
	// This auto-inserts missing \end{...} tags and maintains proper nesting order
	result = RestoreEnvironments(result, original)

	// 5. Fix misplaced environment tags (e.g., \end{sidewaystable} inside \scalebox)
	result = fixMisplacedEnvironmentTags(result, original)

	// 6. Fix brace balance issues (including multirow/multicolumn)
	result = fixBraceBalance(result, original)

	// 7. Fix tabular structure issues (column specs on wrong line)
	result = fixTabularStructure(result)

	// 8. Clean up extra whitespace
	result = cleanupWhitespace(result)

	if result != translated {
		logger.Debug("postprocessor made changes to chunk",
			logger.Int("originalLen", len(translated)),
			logger.Int("fixedLen", len(result)))
	}

	return result
}

// fixLineStructure fixes common line structure issues in translated content.
// This handles cases where LLM merges lines that should be separate.
func fixLineStructure(content string) string {
	result := content

	// Pattern: text followed by \begin{env} on same line
	// These environments should always start on their own line
	floatEnvs := []string{
		"figure", "figure*", "table", "table*",
		"wrapfigure", "wraptable", "sidewaystable", "sidewaysfigure",
		"equation", "equation*", "align", "align*",
		"algorithm", "algorithmic", "lstlisting",
	}

	for _, env := range floatEnvs {
		// Fix: text\begin{env} -> text\n\begin{env}
		pattern := regexp.MustCompile(`([^\n\s])\s*(\\begin\{` + regexp.QuoteMeta(env) + `\})`)
		result = pattern.ReplaceAllString(result, "$1\n$2")

		// Fix: \end{env}text -> \end{env}\ntext
		pattern = regexp.MustCompile(`(\\end\{` + regexp.QuoteMeta(env) + `\})\s*([^\n\s\\])`)
		result = pattern.ReplaceAllString(result, "$1\n$2")
	}

	// Fix: \end{env}\begin{env} -> \end{env}\n\begin{env}
	endBeginPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\begin\{[^}]+\})`)
	result = endBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Fix: \end{env}\section -> \end{env}\n\n\section
	endSectionPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\b)`)
	result = endSectionPattern.ReplaceAllString(result, "$1\n\n$2")

	// Fix: closing brace followed by \section/\subsection/etc on same line
	// This handles cases like: \revised{...}\subsection{...} -> \revised{...}\n\n\subsection{...}
	// Common pattern when LLM merges lines after commands like \revised{}, \textbf{}, etc.
	braceSectionPattern := regexp.MustCompile(`(\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\b)`)
	result = braceSectionPattern.ReplaceAllString(result, "$1\n\n$2")

	// Fix: closing brace followed by \begin{env} on same line (for non-float environments)
	// This handles cases like: \revised{...}\begin{itemize} -> \revised{...}\n\begin{itemize}
	braceBeginPattern := regexp.MustCompile(`(\})\s*(\\begin\{(?:itemize|enumerate|description|quote|quotation|center|flushleft|flushright)\})`)
	result = braceBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Fix: multiple \item on same line
	multiItemPattern := regexp.MustCompile(`(\\item\s+[^\n]+)(\\item\b)`)
	result = multiItemPattern.ReplaceAllString(result, "$1\n$2")

	// ============================================================
	// Table-specific fixes
	// ============================================================

	// Fix: \caption{...}\footnotesize -> \caption{...}\n\footnotesize
	// This is a common issue where LLM merges caption with table formatting commands
	captionFootnotePattern := regexp.MustCompile(`(\\caption\{[^}]*\})\s*(\\(?:footnotesize|small|tiny|scriptsize|normalsize|large|Large|LARGE|huge|Huge)\b)`)
	result = captionFootnotePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \caption{...}\setlength -> \caption{...}\n\setlength
	captionSetlengthPattern := regexp.MustCompile(`(\\caption\{[^}]*\})\s*(\\setlength)`)
	result = captionSetlengthPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \caption{...}\begin{tabular} -> \caption{...}\n\begin{tabular}
	captionTabularPattern := regexp.MustCompile(`(\\caption\{[^}]*\})\s*(\\begin\{(?:tabular|tabularx|longtable|array)\})`)
	result = captionTabularPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \begin{tabular}{...}\toprule -> \begin{tabular}{...}\n\toprule
	tabularToprulePattern := regexp.MustCompile(`(\\begin\{(?:tabular|tabularx|longtable|array)\}(?:\{[^}]*\}|\[[^\]]*\])*)\s*(\\(?:toprule|midrule|bottomrule|hline)\b)`)
	result = tabularToprulePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \toprule...\midrule on same line -> separate lines
	rulePattern := regexp.MustCompile(`(\\(?:toprule|midrule|bottomrule|hline))\s*(\\(?:toprule|midrule|bottomrule|hline)\b)`)
	result = rulePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: table row ending with \\ followed by \midrule on same line
	rowMidrulePattern := regexp.MustCompile(`(\\\\)\s*(\\(?:midrule|bottomrule|hline)\b)`)
	result = rowMidrulePattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \midrule followed by table content on same line
	midruleContentPattern := regexp.MustCompile(`(\\(?:midrule|toprule|hline))\s*(&|\w)`)
	result = midruleContentPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix: \end{tabular}\end{table} -> \end{tabular}\n\end{table}
	endTabularTablePattern := regexp.MustCompile(`(\\end\{(?:tabular|tabularx|longtable|array)\})\s*(\\end\{(?:table|table\*)\})`)
	result = endTabularTablePattern.ReplaceAllString(result, "$1\n$2")

	// Fix: closing brace followed by \begin{tabular} on same line
	braceTabularPattern := regexp.MustCompile(`(\})\s*(\\begin\{(?:tabular|tabularx|longtable|array)\})`)
	result = braceTabularPattern.ReplaceAllString(result, "$1\n    $2")

	// Fix incomplete tabular column specs using a function-based approach
	// This handles cases like: \begin{tabular}{@{}l cccc@{}\n\toprule
	// where the column spec is missing the closing }
	result = FixIncompleteTabularColumnSpec(result)

	return result
}

// FixIncompleteTabularColumnSpec fixes tabular column specs that are missing the closing brace.
// This handles cases like: \begin{tabular}{@{}l cccc@{}\n\toprule
// where the column spec should be {@{}l cccc@{}} but is missing the final }
func FixIncompleteTabularColumnSpec(content string) string {
	// Find all \begin{tabular} (and variants) patterns
	tabularPattern := regexp.MustCompile(`\\begin\{(tabular|tabularx|longtable|array)\*?\}\{`)
	
	result := content
	offset := 0
	
	matches := tabularPattern.FindAllStringIndex(content, -1)
	
	for _, match := range matches {
		startIdx := match[1] + offset // Position right after the opening { of column spec
		
		// Find the column spec by counting braces, but STOP at newline
		// The column spec should be on the same line as \begin{tabular}
		braceCount := 1 // We're starting after the opening {
		
		for i := startIdx; i < len(result); i++ {
			c := result[i]
			if c == '\n' {
				// Stop at newline - column spec should not span multiple lines
				break
			}
			if c == '{' {
				braceCount++
			} else if c == '}' {
				braceCount--
				if braceCount == 0 {
					break // Column spec is properly closed
				}
			}
		}
		
		// If braceCount > 0 after reaching newline, the column spec is incomplete
		if braceCount > 0 {
			// Find where the column spec should end (before \toprule, \midrule, \hline, or newline)
			remaining := result[startIdx:]
			
			// Look for the pattern where column spec ends with @{} followed by newline and \toprule/\midrule
			// Pattern: ...@{}\n    \toprule or ...@{} \n\toprule
			endPattern := regexp.MustCompile(`(@\{\})\s*\n\s*(\\(?:toprule|midrule|bottomrule|hline)\b)`)
			if endMatch := endPattern.FindStringSubmatchIndex(remaining); endMatch != nil {
				// Insert the missing } after @{}
				insertPos := startIdx + endMatch[3] // Position after @{}
				result = result[:insertPos] + "}" + result[insertPos:]
				offset++
				logger.Info("fixed incomplete tabular column spec",
					logger.Int("position", insertPos))
			}
		}
	}
	
	return result
}

// fixEnvironmentsByReference fixes environment issues by comparing with original.
// This detects mismatched begin/end tags and fixes them.
func fixEnvironmentsByReference(translated, original string) string {
	if original == "" {
		return translated
	}

	// Count environments in original
	origEnvCounts := countEnvironments(original)
	transEnvCounts := countEnvironments(translated)

	result := translated

	// Check for extra \begin tags (LLM added environments that don't exist in original)
	for env, transCount := range transEnvCounts {
		origCount := origEnvCounts[env]
		if transCount.begins > origCount.begins {
			// Extra begin tags - this might be a case where LLM removed comment from \begin
			// We can't easily fix this without more context, but log it
			logger.Warn("translated content has extra \\begin tags",
				logger.String("env", env),
				logger.Int("original", origCount.begins),
				logger.Int("translated", transCount.begins))
		}
	}

	// Check for missing \end tags - use enhanced nesting-aware insertion
	result = insertMissingEndTagsWithNesting(result, original)

	return result
}

// insertMissingEndTagsWithNesting inserts missing \end tags while maintaining proper nesting order.
// This implements Requirements 3.3 and 3.4:
// - 3.3: Missing \end{...} tags must be auto-inserted
// - 3.4: Environment tag restoration must maintain correct nesting order
func insertMissingEndTagsWithNesting(translated, original string) string {
	result := translated
	
	// First, count environments to see what's missing
	transEnvCounts := countEnvironments(result)
	
	// Collect all environments that need end tags
	missingEnds := make(map[string]int)
	for env, count := range transEnvCounts {
		if count.begins > count.ends {
			missingEnds[env] = count.begins - count.ends
		}
	}
	
	if len(missingEnds) == 0 {
		return result
	}
	
	// Parse environment positions to understand nesting structure
	envPositions := parseEnvironmentPositions(result)
	
	// Use a stack to track open environments
	stack := []envPositionInfo{}
	
	for _, pos := range envPositions {
		if pos.isBegin {
			stack = append(stack, pos)
		} else {
			// Found an \end tag - pop matching \begin from stack
			if len(stack) > 0 {
				// Find the matching \begin in the stack (may not be at top due to nesting errors)
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].envName == pos.envName {
						// Remove this element from stack
						stack = append(stack[:i], stack[i+1:]...)
						break
					}
				}
			}
		}
	}
	
	// Stack now contains all unclosed environments in order (outermost first)
	// We need to insert their \end tags in reverse order (innermost first)
	// to maintain proper nesting
	
	// Build the end tags string in proper nesting order (innermost first)
	var endTagsBuilder strings.Builder
	for i := len(stack) - 1; i >= 0; i-- {
		unclosed := stack[i]
		endTagsBuilder.WriteString("\n\\end{")
		endTagsBuilder.WriteString(unclosed.envName)
		endTagsBuilder.WriteString("}")
		logger.Debug("will insert missing \\end tag for unclosed environment",
			logger.String("env", unclosed.envName))
	}
	
	// Append all end tags at the end of content
	if endTagsBuilder.Len() > 0 {
		result = result + endTagsBuilder.String() + "\n"
		logger.Info("inserted missing \\end tags",
			logger.Int("count", len(stack)))
	}
	
	return result
}

// envPositionInfo holds information about an environment tag position
type envPositionInfo struct {
	envName  string
	isBegin  bool
	position int
	line     int
}

// envInsertion represents a pending \end tag insertion
type envInsertion struct {
	position int
	envName  string
	endTag   string
}

// parseEnvironmentPositions extracts all environment tag positions from content
func parseEnvironmentPositions(content string) []envPositionInfo {
	positions := []envPositionInfo{}
	
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
			
			// Calculate line number
			line := strings.Count(content[:fullStart], "\n") + 1
			
			positions = append(positions, envPositionInfo{
				envName:  envName,
				isBegin:  envType == "begin",
				position: fullStart,
				line:     line,
			})
		}
	}
	
	return positions
}

// findBestEndTagPosition finds the best position to insert a missing \end tag
// by analyzing the original content structure
func findBestEndTagPosition(translated string, unclosed envPositionInfo, original string) int {
	// For safety, always insert at the end of content
	// This ensures we don't break existing LaTeX tags
	// The insertions will be in reverse nesting order (innermost first)
	// so they will be properly nested when all are inserted
	return len(translated)
}

// findOriginalEndTagPosition finds the position of \end{envName} in original content
// that corresponds to the last unclosed \begin{envName}
func findOriginalEndTagPosition(original, envName string) int {
	endPattern := regexp.MustCompile(`\\end\{` + regexp.QuoteMeta(envName) + `\}`)
	matches := endPattern.FindAllStringIndex(original, -1)
	
	if len(matches) == 0 {
		return -1
	}
	
	// Return the position of the last \end tag
	return matches[len(matches)-1][0]
}

// mapPositionToTranslated attempts to map a position from original to translated content
// by looking at surrounding context
func mapPositionToTranslated(original, translated string, origPos int) int {
	// Get context around the original position
	contextStart := origPos - 50
	if contextStart < 0 {
		contextStart = 0
	}
	contextEnd := origPos + 50
	if contextEnd > len(original) {
		contextEnd = len(original)
	}
	
	// Look for LaTeX commands near this position that might be preserved
	contextBefore := original[contextStart:origPos]
	
	// Find the last LaTeX command before the position
	cmdPattern := regexp.MustCompile(`\\[a-zA-Z]+(?:\{[^}]*\})?`)
	cmdMatches := cmdPattern.FindAllStringIndex(contextBefore, -1)
	
	if len(cmdMatches) > 0 {
		lastCmd := contextBefore[cmdMatches[len(cmdMatches)-1][0]:cmdMatches[len(cmdMatches)-1][1]]
		
		// Find this command in translated content
		transIdx := strings.LastIndex(translated, lastCmd)
		if transIdx >= 0 {
			// Return position after this command
			return transIdx + len(lastCmd)
		}
	}
	
	// Fallback: use proportional mapping
	ratio := float64(len(translated)) / float64(len(original))
	return int(float64(origPos) * ratio)
}

// applyEnvInsertions applies all pending \end tag insertions to the content
// Insertions are applied in reverse order to preserve positions
func applyEnvInsertions(content string, insertions []envInsertion) string {
	if len(insertions) == 0 {
		return content
	}
	
	// Sort insertions by position in descending order
	sortedInsertions := make([]envInsertion, len(insertions))
	copy(sortedInsertions, insertions)
	
	// Simple bubble sort for small number of insertions
	for i := 0; i < len(sortedInsertions)-1; i++ {
		for j := 0; j < len(sortedInsertions)-i-1; j++ {
			if sortedInsertions[j].position < sortedInsertions[j+1].position {
				sortedInsertions[j], sortedInsertions[j+1] = sortedInsertions[j+1], sortedInsertions[j]
			}
		}
	}
	
	result := content
	for _, ins := range sortedInsertions {
		if ins.position > len(result) {
			ins.position = len(result)
		}
		
		// Insert the \end tag with proper newlines
		insertText := "\n" + ins.endTag + "\n"
		result = result[:ins.position] + insertText + result[ins.position:]
		
		logger.Info("inserted missing \\end tag",
			logger.String("env", ins.envName),
			logger.Int("position", ins.position))
	}
	
	return result
}

// envCount holds begin and end counts for an environment.
type envCount struct {
	begins int
	ends   int
}

// countEnvironments counts \begin and \end tags for each environment.
func countEnvironments(content string) map[string]envCount {
	counts := make(map[string]envCount)

	beginRe := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endRe := regexp.MustCompile(`\\end\{([^}]+)\}`)

	for _, match := range beginRe.FindAllStringSubmatch(content, -1) {
		env := match[1]
		c := counts[env]
		c.begins++
		counts[env] = c
	}

	for _, match := range endRe.FindAllStringSubmatch(content, -1) {
		env := match[1]
		c := counts[env]
		c.ends++
		counts[env] = c
	}

	return counts
}

// insertMissingEndTag inserts a missing \end tag at an appropriate position.
func insertMissingEndTag(content, env, endTag string) string {
	// Strategy: find the last \begin{env} and insert \end{env} before the next
	// \begin or \end of a different environment, or at the end of content

	beginPattern := regexp.MustCompile(`\\begin\{` + regexp.QuoteMeta(env) + `\}`)
	matches := beginPattern.FindAllStringIndex(content, -1)

	if len(matches) == 0 {
		return content
	}

	// Start searching from after the last \begin{env}
	lastBeginEnd := matches[len(matches)-1][1]

	// Find the next \begin or \end (of any environment)
	nextEnvPattern := regexp.MustCompile(`\\(?:begin|end)\{[^}]+\}`)
	remaining := content[lastBeginEnd:]
	nextMatch := nextEnvPattern.FindStringIndex(remaining)

	var insertPos int
	if nextMatch != nil {
		insertPos = lastBeginEnd + nextMatch[0]
	} else {
		// Insert at end of content
		insertPos = len(content)
	}

	// Insert the end tag
	return content[:insertPos] + "\n" + endTag + "\n" + content[insertPos:]
}

// fixMisplacedEnvironmentTags fixes cases where \end{...} tags are placed incorrectly.
// For example, LLM might put \end{sidewaystable} right after \scalebox{0.82}{ instead of
// at the end of the table content.
func fixMisplacedEnvironmentTags(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated

	// Pattern: \scalebox{...}{\n\end{sidewaystable}\n\begin{tabular}
	// Should be: \scalebox{...}{\n\begin{tabular}
	// The \end{sidewaystable} should be moved to after the table content

	// Fix pattern: \scalebox{...}{\n\end{env}\n\begin{tabular} -> \scalebox{...}{\n\begin{tabular}
	// This removes the misplaced \end{env} that appears right after \scalebox
	misplacedEndPattern := regexp.MustCompile(`(\\scalebox\{[^}]+\}\{)\s*\n\\end\{(sidewaystable|table\*?|figure\*?)\}\s*\n(\\begin\{tabular\})`)
	result = misplacedEndPattern.ReplaceAllString(result, "$1\n$3")

	// Also fix: }}\end{env} at wrong position
	// Pattern: closing braces followed immediately by \end{env} where it shouldn't be
	wrongEndPattern := regexp.MustCompile(`(\}\})\s*(\\end\{(?:sidewaystable|table\*?)\})\s*(\\begin\{tabular\})`)
	result = wrongEndPattern.ReplaceAllString(result, "$1\n$3")

	return result
}

// fixBraceBalance fixes brace balance issues by comparing with original.
func fixBraceBalance(translated, original string) string {
	if original == "" {
		return translated
	}

	// First, fix multirow/multicolumn missing braces
	result := fixMultirowMulticolumnBraces(translated, original)

	// Count braces in original (excluding those in comments)
	origOpen, origClose := countBraces(original)
	transOpen, transClose := countBraces(result)

	// If translated has more closing braces than original, remove extras at end
	origBalance := origOpen - origClose
	transBalance := transOpen - transClose

	if transBalance < origBalance {
		// Too many closing braces
		extraClose := origBalance - transBalance
		result = removeTrailingBraces(result, extraClose)
	} else if transBalance > origBalance {
		// Too few closing braces (or too many opening)
		// This is harder to fix automatically
		logger.Debug("translated content has brace imbalance",
			logger.Int("originalBalance", origBalance),
			logger.Int("translatedBalance", transBalance))
	}

	return result
}

// fixMultirowMulticolumnBraces fixes missing closing braces in \multirow and \multicolumn commands.
// LLM often drops the closing brace after \textbf{...} inside these commands.
// Example: \multirow{3}{*}{\textbf{Method} -> \multirow{3}{*}{\textbf{Method}}
// Example: \multicolumn{3}{|c}{\textbf{GSM8K} -> \multicolumn{3}{|c}{\textbf{GSM8K}}
func fixMultirowMulticolumnBraces(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated
	fixCount := 0

	// The core issue: LLM drops closing braces from patterns like:
	// \multirow{3}{*}{\textbf{Method}} -> \multirow{3}{*}{\textbf{Method}
	// \multicolumn{3}{|c}{\textbf{GSM8K}} -> \multicolumn{3}{|c}{\textbf{GSM8K}
	//
	// After \textbf{text}, we should have TWO closing braces: one for \textbf{} and one for \multirow/\multicolumn
	// But LLM drops BOTH braces, leaving only the text without any closing brace.
	//
	// Detection: \multirow{...}{...}{\textbf{text followed by & or \\ (no closing braces!)

	// Check if there are any multirow/multicolumn patterns to fix
	hasMultirow := strings.Contains(result, "\\multirow")
	hasMulticol := strings.Contains(result, "\\multicolumn")
	
	if hasMultirow || hasMulticol {
		logger.Debug("checking for multirow/multicolumn brace issues",
			logger.Bool("hasMultirow", hasMultirow),
			logger.Bool("hasMulticol", hasMulticol))
	}

	// ============================================================
	// PATTERN GROUP 1: Missing BOTH closing braces (}} needed)
	// ============================================================
	
	// Pattern: \multirow{N}{spec}{\textbf{text followed by space and & (missing BOTH closing braces)
	// Input:  \multirow{3}{*}{\textbf{Method} &
	// Output: \multirow{3}{*}{\textbf{Method}}} &
	multirowBeforeAmpSpace := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}&]+)\s+(&)`)
	if multirowBeforeAmpSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing both braces before space+&")
	}
	result = multirowBeforeAmpSpace.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multirow{N}{spec}{\textbf{text}& (NO space before &, missing one })
	// Input:  \multirow{3}{*}{\textbf{Method}&
	// Output: \multirow{3}{*}{\textbf{Method}}}&
	multirowBeforeAmpNoSpace := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})(&)`)
	if multirowBeforeAmpNoSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing one brace before & (no space)")
	}
	result = multirowBeforeAmpNoSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text followed by space and &
	multicolBeforeAmpSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}&]+)\s+(&)`)
	if multicolBeforeAmpSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing both braces before space+&")
	}
	result = multicolBeforeAmpSpace.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text}& (NO space before &, missing one })
	multicolBeforeAmpNoSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})(&)`)
	if multicolBeforeAmpNoSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before & (no space)")
	}
	result = multicolBeforeAmpNoSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multirow{N}{spec}{\textbf{text followed by \\
	multirowBeforeBackslash := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}\\]+)\s*(\\\\)`)
	if multirowBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing braces before \\\\")
	}
	result = multirowBeforeBackslash.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text followed by \\
	multicolBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}\\]+)\s*(\\\\)`)
	if multicolBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing braces before \\\\")
	}
	result = multicolBeforeBackslash.ReplaceAllString(result, "$1}} $2")

	// ============================================================
	// PATTERN GROUP 1b: \textit{} patterns (same as \textbf{} but for italic)
	// ============================================================
	
	// Pattern: \multicolumn{N}{spec}{\textit{text} & (space before &, missing BOTH braces)
	multicolTextitBeforeAmpSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}&]+)\s+(&)`)
	if multicolTextitBeforeAmpSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing both braces before space+&")
	}
	result = multicolTextitBeforeAmpSpace.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textit{text}& (NO space before &)
	multicolTextitBeforeAmpNoSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}]+\})(&)`)
	if multicolTextitBeforeAmpNoSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing one brace before & (no space)")
	}
	result = multicolTextitBeforeAmpNoSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{\textit{text followed by \\ (missing BOTH braces)
	multicolTextitBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}\\]+)\s*(\\\\)`)
	if multicolTextitBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing braces before \\\\")
	}
	result = multicolTextitBeforeBackslash.ReplaceAllString(result, "$1}} $2")

	// ============================================================
	// PATTERN GROUP 2: Nested commands like \textbf{\textsc{...}}
	// ============================================================
	
	// Pattern: \multicolumn{N}{spec}{\textbf{\textsc{text}} - nested commands (missing one closing brace)
	// Example: \multicolumn{12}{c}{\textbf{\textsc{Math}}} -> \multicolumn{12}{c}{\textbf{\textsc{Math}}}}
	multicolNestedBeforeAmp := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{\\textsc\{[^}]+\}\})\s*(&)`)
	if multicolNestedBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn nested missing brace before &")
	}
	result = multicolNestedBeforeAmp.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{\textsc{text}} followed by \\
	multicolNestedBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{\\textsc\{[^}]+\}\})\s*(\\\\)`)
	if multicolNestedBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn nested missing brace before \\\\")
	}
	result = multicolNestedBeforeBackslash.ReplaceAllString(result, "$1} $2")

	// ============================================================
	// PATTERN GROUP 3: Missing ONE closing brace (} needed)
	// ============================================================
	
	// Pattern: \multirow{N}{spec}{\textbf{text} followed by & (has one }, missing one)
	multirowOneBeforeAmp := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}\s*(&)`)
	if multirowOneBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing one brace before &")
	}
	result = multirowOneBeforeAmp.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text} followed by &
	multicolOneBeforeAmp := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}\s*(&)`)
	if multicolOneBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before &")
	}
	result = multicolOneBeforeAmp.ReplaceAllString(result, "$1}} $2")

	// Pattern: end of line - \multirow{...}{\textbf{text}\n (missing one })
	multirowEndOfLine := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}(\s*\n)`)
	if multirowEndOfLine.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing brace at end of line")
	}
	result = multirowEndOfLine.ReplaceAllString(result, "$1}}$2")

	// Pattern: end of line - \multicolumn{...}{\textbf{text}\n
	multicolEndOfLine := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}(\s*\n)`)
	if multicolEndOfLine.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing brace at end of line")
	}
	result = multicolEndOfLine.ReplaceAllString(result, "$1}}$2")

	// ============================================================
	// PATTERN GROUP 4: Missing ONE closing brace before \\ (table row end)
	// Pattern: \multicolumn{N}{spec}{\textbf{text}} \\ (has } for textbf, missing } for multicolumn)
	// This is common in table headers like: \multicolumn{2}{|c}{\textbf{Overall} \\
	// Should be: \multicolumn{2}{|c}{\textbf{Overall}} \\
	// ============================================================

	// Pattern: \multicolumn{N}{spec}{\textbf{text} followed by space and \\ (missing one } for multicolumn)
	multicolBeforeBackslashWithSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})\s+(\\\\)`)
	if multicolBeforeBackslashWithSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before space+\\\\")
	}
	result = multicolBeforeBackslashWithSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multirow{N}{spec}{\textbf{text} followed by space and \\ (missing one } for multirow)
	multirowBeforeBackslashWithSpace := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})\s+(\\\\)`)
	if multirowBeforeBackslashWithSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing one brace before space+\\\\")
	}
	result = multirowBeforeBackslashWithSpace.ReplaceAllString(result, "$1} $2")

	// ============================================================
	// PATTERN GROUP 5: Plain text content (no \textbf or \textit)
	// Pattern: \multicolumn{N}{spec}{text} & or \\ where text has no formatting
	// ============================================================

	// Pattern: \multicolumn{N}{spec}{plain text followed by & (missing closing brace)
	// Example: \multicolumn{2}{c}{Method & -> \multicolumn{2}{c}{Method} &
	multicolPlainBeforeAmp := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{[^}\\&]+)\s+(&)`)
	if multicolPlainBeforeAmp.MatchString(result) {
		// Only fix if the content doesn't already have a closing brace
		fixCount++
		logger.Debug("fixing multicolumn plain text missing brace before &")
	}
	result = multicolPlainBeforeAmp.ReplaceAllString(result, "$1} $2")

	// Pattern: \multirow{N}{spec}{plain text followed by & (missing closing brace)
	multirowPlainBeforeAmp := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{[^}\\&]+)\s+(&)`)
	if multirowPlainBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow plain text missing brace before &")
	}
	result = multirowPlainBeforeAmp.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{plain text followed by \\ (missing closing brace)
	multicolPlainBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{[^}\\]+)\s*(\\\\)`)
	if multicolPlainBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn plain text missing brace before \\\\")
	}
	result = multicolPlainBeforeBackslash.ReplaceAllString(result, "$1} $2")

	// Pattern: \multirow{N}{spec}{plain text followed by \\ (missing closing brace)
	multirowPlainBeforeBackslash := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{[^}\\]+)\s*(\\\\)`)
	if multirowPlainBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow plain text missing brace before \\\\")
	}
	result = multirowPlainBeforeBackslash.ReplaceAllString(result, "$1} $2")

	// ============================================================
	// PATTERN GROUP 6: \cmidrule and \cline patterns
	// Pattern: \multicolumn followed by \cmidrule or \cline
	// ============================================================

	// Pattern: \multicolumn{N}{spec}{\textbf{text} followed by \cmidrule
	multicolBeforeCmidrule := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\s*(\\cmidrule)`)
	if multicolBeforeCmidrule.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing braces before \\cmidrule")
	}
	result = multicolBeforeCmidrule.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text}} followed by \cmidrule (missing one brace)
	multicolOneBraceCmidrule := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})\s*(\\cmidrule)`)
	if multicolOneBraceCmidrule.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before \\cmidrule")
	}
	result = multicolOneBraceCmidrule.ReplaceAllString(result, "$1} $2")

	if fixCount > 0 {
		logger.Info("fixed multirow/multicolumn braces", logger.Int("fixCount", fixCount))
	}

	return result
}

// FixMultirowMulticolumnBracesGlobal is the exported version of fixMultirowMulticolumnBraces
// that can be called on the final assembled content. It doesn't require the original content.
func FixMultirowMulticolumnBracesGlobal(content string) string {
	if content == "" {
		return content
	}

	result := content
	fixCount := 0

	// Check if there are any multirow/multicolumn patterns to fix
	hasMultirow := strings.Contains(result, "\\multirow")
	hasMulticol := strings.Contains(result, "\\multicolumn")
	
	if !hasMultirow && !hasMulticol {
		return content
	}

	logger.Debug("FixMultirowMulticolumnBracesGlobal: checking for brace issues",
		logger.Bool("hasMultirow", hasMultirow),
		logger.Bool("hasMulticol", hasMulticol))

	// ============================================================
	// PATTERN GROUP 1: Missing BOTH closing braces (}} needed)
	// These patterns match when \textbf{text has NO closing braces at all
	// ============================================================
	
	// Pattern: \multirow{N}{spec}{\textbf{text} & (space before &, missing BOTH braces)
	// Input:  \multirow{3}{*}{\textbf{Method} &
	// Output: \multirow{3}{*}{\textbf{Method}}} &
	multirowBeforeAmpSpace := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}&]+)\s+(&)`)
	if multirowBeforeAmpSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing both braces before space+&")
	}
	result = multirowBeforeAmpSpace.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text} & (space before &, missing BOTH braces)
	multicolBeforeAmpSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}&]+)\s+(&)`)
	if multicolBeforeAmpSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing both braces before space+&")
	}
	result = multicolBeforeAmpSpace.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multirow{N}{spec}{\textbf{text}& (NO space before &)
	// After the above fix, this handles remaining cases
	multirowBeforeAmpNoSpace := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})(&)`)
	if multirowBeforeAmpNoSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing one brace before & (no space)")
	}
	result = multirowBeforeAmpNoSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text}& (NO space before &)
	multicolBeforeAmpNoSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})(&)`)
	if multicolBeforeAmpNoSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before & (no space)")
	}
	result = multicolBeforeAmpNoSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multirow{N}{spec}{\textbf{text followed by \\ (missing BOTH braces)
	multirowBeforeBackslash := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}\\]+)\s*(\\\\)`)
	if multirowBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing braces before \\\\")
	}
	result = multirowBeforeBackslash.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text followed by \\ (missing BOTH braces)
	multicolBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}\\]+)\s*(\\\\)`)
	if multicolBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing braces before \\\\")
	}
	result = multicolBeforeBackslash.ReplaceAllString(result, "$1}} $2")

	// ============================================================
	// PATTERN GROUP 2: Nested commands like \textbf{\textsc{...}}
	// ============================================================
	
	// Pattern: \multicolumn{N}{spec}{\textbf{\textsc{text}}} & (missing one closing brace for multicolumn)
	multicolNestedBeforeAmp := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{\\textsc\{[^}]+\}\})\s*(&)`)
	if multicolNestedBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn nested missing brace before &")
	}
	result = multicolNestedBeforeAmp.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{\textsc{text}}} \\ (missing one closing brace)
	multicolNestedBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{\\textsc\{[^}]+\}\})\s*(\\\\)`)
	if multicolNestedBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn nested missing brace before \\\\")
	}
	result = multicolNestedBeforeBackslash.ReplaceAllString(result, "$1} $2")

	// ============================================================
	// PATTERN GROUP 3: Missing ONE closing brace (} needed)
	// These patterns match when \textbf{text} has one } but multirow/multicolumn is missing its }
	// ============================================================
	
	// Pattern: \multirow{N}{spec}{\textbf{text} & (has one }, missing one for multirow)
	multirowOneBeforeAmp := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}\s*(&)`)
	if multirowOneBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing one brace before &")
	}
	result = multirowOneBeforeAmp.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textbf{text} & (has one }, missing one for multicolumn)
	multicolOneBeforeAmp := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}\s*(&)`)
	if multicolOneBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before &")
	}
	result = multicolOneBeforeAmp.ReplaceAllString(result, "$1}} $2")

	// Pattern: end of line - \multirow{...}{\textbf{text}\n (missing one })
	multirowEndOfLine := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}(\s*\n)`)
	if multirowEndOfLine.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing brace at end of line")
	}
	result = multirowEndOfLine.ReplaceAllString(result, "$1}}$2")

	// Pattern: end of line - \multicolumn{...}{\textbf{text}\n (missing one })
	multicolEndOfLine := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+)\}(\s*\n)`)
	if multicolEndOfLine.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing brace at end of line")
	}
	result = multicolEndOfLine.ReplaceAllString(result, "$1}}$2")

	// ============================================================
	// PATTERN GROUP 4: Missing ONE closing brace before \\ (table row end)
	// Pattern: \multicolumn{N}{spec}{\textbf{text}} \\ (has } for textbf, missing } for multicolumn)
	// This is common in table headers like: \multicolumn{2}{|c}{\textbf{Overall} \\
	// Should be: \multicolumn{2}{|c}{\textbf{Overall}} \\
	// ============================================================
	
	// Pattern: \multicolumn{N}{spec}{\textbf{text} followed by space and \\ (missing one } for multicolumn)
	multicolBeforeBackslashWithSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})\s+(\\\\)`)
	if multicolBeforeBackslashWithSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn missing one brace before space+\\\\")
	}
	result = multicolBeforeBackslashWithSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multirow{N}{spec}{\textbf{text} followed by space and \\ (missing one } for multirow)
	multirowBeforeBackslashWithSpace := regexp.MustCompile(`(\\multirow\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})\s+(\\\\)`)
	if multirowBeforeBackslashWithSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multirow missing one brace before space+\\\\")
	}
	result = multirowBeforeBackslashWithSpace.ReplaceAllString(result, "$1} $2")

	// ============================================================
	// PATTERN GROUP 5: \textit{} patterns (same as \textbf{} but for italic)
	// ============================================================
	
	// Pattern: \multicolumn{N}{spec}{\textit{text} & (space before &, missing BOTH braces)
	// Input:  \multicolumn{2}{l}{\textit{Hardware} &
	// Output: \multicolumn{2}{l}{\textit{Hardware}}} &
	multicolTextitBeforeAmpSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}&]+)\s+(&)`)
	if multicolTextitBeforeAmpSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing both braces before space+&")
	}
	result = multicolTextitBeforeAmpSpace.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textit{text}& (NO space before &)
	multicolTextitBeforeAmpNoSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}]+\})(&)`)
	if multicolTextitBeforeAmpNoSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing one brace before & (no space)")
	}
	result = multicolTextitBeforeAmpNoSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: \multicolumn{N}{spec}{\textit{text followed by \\ (missing BOTH braces)
	multicolTextitBeforeBackslash := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}\\]+)\s*(\\\\)`)
	if multicolTextitBeforeBackslash.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing braces before \\\\")
	}
	result = multicolTextitBeforeBackslash.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textit{text} & (has one }, missing one for multicolumn)
	multicolTextitOneBeforeAmp := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}]+)\}\s*(&)`)
	if multicolTextitOneBeforeAmp.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing one brace before &")
	}
	result = multicolTextitOneBeforeAmp.ReplaceAllString(result, "$1}} $2")

	// Pattern: \multicolumn{N}{spec}{\textit{text} followed by space and \\ (missing one } for multicolumn)
	multicolTextitBeforeBackslashWithSpace := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}]+\})\s+(\\\\)`)
	if multicolTextitBeforeBackslashWithSpace.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing one brace before space+\\\\")
	}
	result = multicolTextitBeforeBackslashWithSpace.ReplaceAllString(result, "$1} $2")

	// Pattern: end of line - \multicolumn{...}{\textit{text}\n (missing one })
	multicolTextitEndOfLine := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textit\{[^}]+)\}(\s*\n)`)
	if multicolTextitEndOfLine.MatchString(result) {
		fixCount++
		logger.Debug("fixing multicolumn textit missing brace at end of line")
	}
	result = multicolTextitEndOfLine.ReplaceAllString(result, "$1}}$2")

	if fixCount > 0 {
		logger.Info("FixMultirowMulticolumnBracesGlobal: fixed braces", logger.Int("fixCount", fixCount))
	}

	return result
}

// countBraces counts opening and closing braces in content.
// It ignores braces in comments and escaped braces.
func countBraces(content string) (open, close int) {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		// Remove comment portion
		commentIdx := findUnescapedPercent(line)
		if commentIdx >= 0 {
			line = line[:commentIdx]
		}

		// Count braces, ignoring escaped ones
		for i := 0; i < len(line); i++ {
			if line[i] == '{' {
				if i == 0 || line[i-1] != '\\' {
					open++
				}
			} else if line[i] == '}' {
				if i == 0 || line[i-1] != '\\' {
					close++
				}
			}
		}
	}

	return
}

// findUnescapedPercent finds the position of the first unescaped % in a line.
func findUnescapedPercent(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] == '%' {
			// Check if escaped
			backslashes := 0
			for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// removeTrailingBraces removes extra closing braces from the end of content.
func removeTrailingBraces(content string, count int) string {
	result := strings.TrimRight(content, " \t\n")

	for i := 0; i < count && len(result) > 0; i++ {
		if result[len(result)-1] == '}' {
			result = result[:len(result)-1]
			result = strings.TrimRight(result, " \t\n")
		} else {
			break
		}
	}

	return result + "\n"
}

// fixTabularStructure fixes broken tabular structures where column specifications
// are incorrectly placed on separate lines from \begin{tabular}.
// This commonly happens when LLM adds newlines in the middle of tabular definitions.
func fixTabularStructure(content string) string {
	result := content

	// Fix: \begin{tabular}\n{...} -> \begin{tabular}{...}
	// Pattern matches \begin{tabular} followed by newline and column spec
	tabularNewlinePattern := regexp.MustCompile(`(\\begin\{tabular\})\s*\n\s*(\{[^}]*\})`)
	result = tabularNewlinePattern.ReplaceAllString(result, "$1$2")

	// Fix: \begin{tabular}[pos]\n{...} -> \begin{tabular}[pos]{...}
	tabularPosNewlinePattern := regexp.MustCompile(`(\\begin\{tabular\}\[[^\]]*\])\s*\n\s*(\{[^}]*\})`)
	result = tabularPosNewlinePattern.ReplaceAllString(result, "$1$2")

	// Fix: \end{tabular}}\n}\n\n& -> \end{tabular}} &
	// This fixes broken multirow with nested tabular
	endTabularExtraBrace := regexp.MustCompile(`(\\end\{tabular\}\})\s*\n\}\s*\n\s*&`)
	result = endTabularExtraBrace.ReplaceAllString(result, "$1 &")

	// Fix: \multirow{N}{*}{\n\begin{tabular} -> \multirow{N}{*}{\begin{tabular}
	multirowNewlinePattern := regexp.MustCompile(`(\\multirow\{[0-9]+\}\{\*\}\{)\s*\n\s*(\\begin\{tabular\})`)
	result = multirowNewlinePattern.ReplaceAllString(result, "$1$2")

	// Fix: \end{tabular}}\n} followed by content -> \end{tabular}} followed by content
	// Remove extra closing brace that appears after nested tabular in multirow
	endTabularExtraClose := regexp.MustCompile(`(\\end\{tabular\}\})\s*\n\}\s*\n`)
	result = endTabularExtraClose.ReplaceAllString(result, "$1 ")

	if result != content {
		logger.Debug("fixed tabular structure issues")
	}

	return result
}

// cleanupWhitespace cleans up extra whitespace in content.
func cleanupWhitespace(content string) string {
	// Remove trailing whitespace from each line
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	result := strings.Join(lines, "\n")

	// Remove more than 2 consecutive empty lines
	multiEmptyPattern := regexp.MustCompile(`\n{4,}`)
	result = multiEmptyPattern.ReplaceAllString(result, "\n\n\n")

	return result
}

// PostprocessOptions contains options for post-processing.
type PostprocessOptions struct {
	// FixLineStructure fixes line structure issues
	FixLineStructure bool
	// FixEnvironments fixes environment matching issues
	FixEnvironments bool
	// FixBraces fixes brace balance issues
	FixBraces bool
	// CleanWhitespace cleans up extra whitespace
	CleanWhitespace bool
}

// DefaultPostprocessOptions returns the default post-processing options.
func DefaultPostprocessOptions() PostprocessOptions {
	return PostprocessOptions{
		FixLineStructure: true,
		FixEnvironments:  true,
		FixBraces:        true,
		CleanWhitespace:  true,
	}
}

// PostprocessChunkWithOptions performs post-processing with custom options.
func PostprocessChunkWithOptions(translated, original string, opts PostprocessOptions) string {
	if translated == "" {
		return translated
	}

	result := translated

	if opts.FixLineStructure {
		// Use the enhanced RestoreLineStructure function (Requirements 2.1-2.5)
		result = RestoreLineStructure(result, original)
	}

	if opts.FixEnvironments {
		result = fixEnvironmentsByReference(result, original)
	}

	if opts.FixBraces {
		result = fixBraceBalance(result, original)
	}

	if opts.CleanWhitespace {
		result = cleanupWhitespace(result)
	}

	return result
}


// LineByLineFixResult contains the result of line-by-line fixing.
type LineByLineFixResult struct {
	Fixed       string
	FixCount    int
	FixDetails  []string
}

// FixByLineComparison performs line-by-line comparison between original and translated
// content to fix LaTeX structure issues. This is the most accurate fix method because
// it compares each line individually.
//
// The function:
// 1. Splits both original and translated into lines
// 2. For each line pair, extracts LaTeX commands from original
// 3. Ensures translated line has the same LaTeX structure
// 4. Fixes common issues like wrongly added comments, missing commands, etc.
func FixByLineComparison(translated, original string) *LineByLineFixResult {
	if original == "" || translated == "" {
		return &LineByLineFixResult{Fixed: translated}
	}

	origLines := strings.Split(original, "\n")
	transLines := strings.Split(translated, "\n")

	result := &LineByLineFixResult{
		FixDetails: []string{},
	}

	// If line counts differ significantly, we can't do line-by-line comparison
	// Fall back to other methods
	// Use a stricter threshold (10% instead of 25%) to avoid misaligned fixes
	if abs(len(origLines)-len(transLines)) > len(origLines)/10 {
		logger.Warn("line count differs too much for line-by-line fix",
			logger.Int("original", len(origLines)),
			logger.Int("translated", len(transLines)),
			logger.Int("threshold", len(origLines)/10))
		result.Fixed = translated
		return result
	}

	fixedLines := make([]string, len(transLines))
	copy(fixedLines, transLines)

	// Process each line pair
	minLines := len(origLines)
	if len(transLines) < minLines {
		minLines = len(transLines)
	}

	for i := 0; i < minLines; i++ {
		origLine := origLines[i]
		transLine := transLines[i]

		fixed, wasFixed, detail := fixLineByReference(transLine, origLine, i+1)
		if wasFixed {
			fixedLines[i] = fixed
			result.FixCount++
			if detail != "" {
				result.FixDetails = append(result.FixDetails, detail)
			}
		}
	}

	result.Fixed = strings.Join(fixedLines, "\n")
	
	if result.FixCount > 0 {
		logger.Info("line-by-line fix applied",
			logger.Int("fixCount", result.FixCount))
	}

	return result
}

// fixLineByReference fixes a single translated line by comparing with original.
// Returns the fixed line, whether it was fixed, and a description of the fix.
func fixLineByReference(transLine, origLine string, lineNum int) (string, bool, string) {
	// Skip empty lines
	if strings.TrimSpace(origLine) == "" {
		return transLine, false, ""
	}

	fixed := transLine
	wasFixed := false
	var details []string

	// 1. Check if original line is a comment but translated is not
	origIsComment := strings.HasPrefix(strings.TrimSpace(origLine), "%")
	transIsComment := strings.HasPrefix(strings.TrimSpace(transLine), "%")

	// Helper function to get content without comment marker
	getContentWithoutComment := func(line string) string {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "% ") {
			return strings.TrimSpace(trimmed[2:])
		} else if strings.HasPrefix(trimmed, "%") {
			return strings.TrimSpace(trimmed[1:])
		}
		return trimmed
	}

	// Helper function to check if two lines have similar content (ignoring comment markers)
	// This prevents incorrect comment fixes when lines are misaligned
	linesAreSimilar := func(line1, line2 string) bool {
		content1 := getContentWithoutComment(line1)
		content2 := getContentWithoutComment(line2)
		
		// If either is empty, they're not similar
		if content1 == "" || content2 == "" {
			return false
		}
		
		// Check if they share the same LaTeX command at the start WITH its argument
		// e.g., both start with \usepackage{icml2026}, \section{Introduction}, etc.
		// This is more strict than just checking the command name
		cmdWithArgPattern := regexp.MustCompile(`^\\[a-zA-Z]+(\[[^\]]*\])?\{[^}]*\}`)
		cmdArg1 := cmdWithArgPattern.FindString(content1)
		cmdArg2 := cmdWithArgPattern.FindString(content2)
		
		if cmdArg1 != "" && cmdArg2 != "" {
			// Both have LaTeX commands with arguments - they should match exactly
			return cmdArg1 == cmdArg2
		}
		
		// Check if they share the same LaTeX command (without argument)
		cmdPattern := regexp.MustCompile(`^\\[a-zA-Z]+`)
		cmd1 := cmdPattern.FindString(content1)
		cmd2 := cmdPattern.FindString(content2)
		
		if cmd1 != "" && cmd2 != "" {
			// Both have LaTeX commands
			if cmd1 != cmd2 {
				return false // Different commands
			}
			// Same command - check if the rest of the line is similar
			// This handles cases like \usepackage{icml2026} vs \usepackage{hyperref}
			rest1 := strings.TrimPrefix(content1, cmd1)
			rest2 := strings.TrimPrefix(content2, cmd2)
			// Check first 15 characters of the rest
			checkLen := 15
			if len(rest1) < checkLen {
				checkLen = len(rest1)
			}
			if len(rest2) < checkLen {
				checkLen = len(rest2)
			}
			if checkLen > 0 {
				return rest1[:checkLen] == rest2[:checkLen]
			}
			return true // Both have same command with no arguments
		}
		
		// For non-command lines, check if they share significant common prefix
		minLen := len(content1)
		if len(content2) < minLen {
			minLen = len(content2)
		}
		if minLen < 10 {
			// Too short to compare meaningfully
			return true // Allow fix for short lines
		}
		
		// Check first 20 characters (or less if shorter)
		checkLen := 20
		if minLen < checkLen {
			checkLen = minLen
		}
		return content1[:checkLen] == content2[:checkLen]
	}

	if origIsComment && !transIsComment {
		// Original is commented, translated should be too
		// But only if the content is similar (to avoid misaligned fixes)
		if linesAreSimilar(origLine, transLine) {
			fixed = "% " + strings.TrimLeft(fixed, " \t")
			wasFixed = true
			details = append(details, fmt.Sprintf("L%d: added missing comment marker", lineNum))
		}
	} else if !origIsComment && transIsComment {
		// Original is NOT commented, but translated is - remove wrong comment
		// But only if the content is similar (to avoid misaligned fixes)
		if !linesAreSimilar(origLine, transLine) {
			// Lines are not similar - skip this fix to avoid incorrect uncommenting
			logger.Debug("skipping uncomment due to content mismatch",
				logger.Int("lineNum", lineNum),
				logger.String("origLine", origLine[:min(50, len(origLine))]),
				logger.String("transLine", transLine[:min(50, len(transLine))]))
		} else {
			// EXCEPTION: Do NOT uncomment \bibliography commands - they should stay commented
			// in the preamble to avoid putting thebibliography in the wrong place
			trimmed := strings.TrimSpace(transLine)
			if strings.Contains(trimmed, `\bibliography{`) {
				// Skip uncommenting \bibliography commands - they were intentionally commented
				// by fixWronglyUncommentedBibliography and should stay that way
				logger.Debug("skipping uncomment for \\bibliography command",
					logger.Int("lineNum", lineNum),
					logger.String("line", trimmed))
			} else {
				if strings.HasPrefix(trimmed, "% ") {
					fixed = strings.TrimSpace(trimmed[2:])
				} else if strings.HasPrefix(trimmed, "%") {
					fixed = strings.TrimSpace(trimmed[1:])
				}
				// Preserve original indentation
				indent := getLeadingWhitespace(origLine)
				fixed = indent + fixed
				wasFixed = true
				details = append(details, fmt.Sprintf("L%d: removed wrong comment marker", lineNum))
			}
		}
	}

	// 2. Check for LaTeX commands that should be preserved
	origCmds := extractLaTeXStructure(origLine)
	transCmds := extractLaTeXStructure(fixed)

	// Check for missing \begin or \end
	for _, cmd := range origCmds {
		if (strings.HasPrefix(cmd, "\\begin{") || strings.HasPrefix(cmd, "\\end{")) &&
			!containsCommand(transCmds, cmd) {
			// Missing environment command - this is serious
			// Try to add it back
			if strings.HasPrefix(cmd, "\\begin{") {
				// Check if it was wrongly commented
				if strings.Contains(fixed, "% "+cmd) || strings.Contains(fixed, "%"+cmd) {
					fixed = strings.Replace(fixed, "% "+cmd, cmd, 1)
					fixed = strings.Replace(fixed, "%"+cmd, cmd, 1)
					wasFixed = true
					details = append(details, fmt.Sprintf("L%d: uncommented %s", lineNum, cmd))
				}
			} else if strings.HasPrefix(cmd, "\\end{") {
				if strings.Contains(fixed, "% "+cmd) || strings.Contains(fixed, "%"+cmd) {
					fixed = strings.Replace(fixed, "% "+cmd, cmd, 1)
					fixed = strings.Replace(fixed, "%"+cmd, cmd, 1)
					wasFixed = true
					details = append(details, fmt.Sprintf("L%d: uncommented %s", lineNum, cmd))
				}
			}
		}
	}

	// 3. Check for extra closing braces
	origBraceBalance := countLineBraces(origLine)
	transBraceBalance := countLineBraces(fixed)

	if transBraceBalance < origBraceBalance {
		// Too many closing braces in translated
		extra := origBraceBalance - transBraceBalance
		for i := 0; i < extra; i++ {
			// Remove trailing }
			trimmed := strings.TrimRight(fixed, " \t")
			if strings.HasSuffix(trimmed, "}}") {
				// Check if it's a pattern like \textit{...}}
				fixed = trimmed[:len(trimmed)-1]
				wasFixed = true
				details = append(details, fmt.Sprintf("L%d: removed extra closing brace", lineNum))
			}
		}
	}

	// 4. Check for wrongly added \end{document}
	if strings.Contains(fixed, "\\end{document}") && !strings.Contains(origLine, "\\end{document}") {
		fixed = strings.ReplaceAll(fixed, "\\end{document}", "")
		fixed = strings.TrimSpace(fixed)
		// If line becomes empty after removing \end{document}, keep it empty
		// Do NOT restore original line - that would cause content duplication
		wasFixed = true
		details = append(details, fmt.Sprintf("L%d: removed wrongly added \\end{document}", lineNum))
	}

	detail := ""
	if len(details) > 0 {
		detail = strings.Join(details, "; ")
	}

	return fixed, wasFixed, detail
}

// extractLaTeXStructure extracts LaTeX structural commands from a line.
// This includes \begin{}, \end{}, \section{}, etc.
func extractLaTeXStructure(line string) []string {
	var cmds []string

	// Match \begin{...} and \end{...}
	envPattern := regexp.MustCompile(`\\(?:begin|end)\{[^}]+\}`)
	for _, match := range envPattern.FindAllString(line, -1) {
		cmds = append(cmds, match)
	}

	// Match sectioning commands
	sectionPattern := regexp.MustCompile(`\\(?:section|subsection|subsubsection|chapter|paragraph)\*?\{`)
	for _, match := range sectionPattern.FindAllString(line, -1) {
		cmds = append(cmds, match)
	}

	// Match \item
	if strings.Contains(line, "\\item") {
		cmds = append(cmds, "\\item")
	}

	return cmds
}

// containsCommand checks if a command list contains a specific command.
func containsCommand(cmds []string, cmd string) bool {
	for _, c := range cmds {
		if c == cmd {
			return true
		}
	}
	return false
}

// countLineBraces counts brace balance in a single line (open - close).
func countLineBraces(line string) int {
	// Remove comment portion
	commentIdx := findUnescapedPercent(line)
	if commentIdx >= 0 {
		line = line[:commentIdx]
	}

	balance := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '{' && (i == 0 || line[i-1] != '\\') {
			balance++
		} else if line[i] == '}' && (i == 0 || line[i-1] != '\\') {
			balance--
		}
	}
	return balance
}

// getLeadingWhitespace returns the leading whitespace of a line.
func getLeadingWhitespace(line string) string {
	for i, c := range line {
		if c != ' ' && c != '\t' {
			return line[:i]
		}
	}
	return line
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// FixTableEnvironmentLineStructure fixes line structure issues around table environments.
// Common issues:
// 1. \end{table*}\n}text -> \end{table*}\n}\n\ntext (} should be on its own line, then blank line)
// 2. \end{table*}\setlength{...} -> \end{table*}\n\n\setlength{...} (commands should be on separate lines)
// 3. }\textbf{...} after \end{table*} -> }\n\n\textbf{...} (text should start on new paragraph)
func FixTableEnvironmentLineStructure(content string) string {
	if content == "" {
		return content
	}

	result := content
	fixCount := 0

	// Pattern 1: \end{table*}\n}text or \end{table*}\n}\text
	// The } closes \scalebox{}, and text should be on a new paragraph
	// Match: \end{table*}\n}followed by non-whitespace (except another })
	pattern1 := regexp.MustCompile(`(\\end\{table\*?\}\s*\n\})([^\s\n}])`)
	if pattern1.MatchString(result) {
		fixCount++
		logger.Debug("fixing table environment: } followed by text")
	}
	result = pattern1.ReplaceAllString(result, "$1\n\n$2")

	// Pattern 2: \end{table*}\setlength or \end{table*}\renewcommand etc.
	// These should be on separate lines with a blank line between
	pattern2 := regexp.MustCompile(`(\\end\{table\*?\})(\\(?:setlength|renewcommand|begin|newcommand|def)\{)`)
	if pattern2.MatchString(result) {
		fixCount++
		logger.Debug("fixing table environment: \\end{table*} followed by command")
	}
	result = pattern2.ReplaceAllString(result, "$1\n\n$2")

	// Pattern 3: }\textbf{ or }\textit{ or }text after a line that ends with }
	// This often happens when LLM merges the closing brace of \scalebox with following text
	// Match lines that start with }\ followed by a command or text
	pattern3 := regexp.MustCompile(`(\n\})(\\(?:textbf|textit|textsc|emph|section|subsection|paragraph)\{)`)
	if pattern3.MatchString(result) {
		fixCount++
		logger.Debug("fixing: } followed by formatting command")
	}
	result = pattern3.ReplaceAllString(result, "$1\n\n$2")

	// Pattern 4: }text (} at start of line followed directly by text, not a command)
	// This is the case like: }Comparison... where } should be separate
	// Use Unicode range for Chinese characters: \x{4e00}-\x{9fff}
	pattern4 := regexp.MustCompile(`(\n\})([A-Za-z\p{Han}])`)
	if pattern4.MatchString(result) {
		fixCount++
		logger.Debug("fixing: } followed by text")
	}
	result = pattern4.ReplaceAllString(result, "$1\n\n$2")

	// Pattern 5: \end{tabular}}\n}text - the extra } after \end{tabular}} should stay,
	// but the } on next line followed by text needs fixing
	// This is already handled by pattern 1 and 4

	// Pattern 6: Fix cases where \end{table*} is immediately followed by text on same line
	// (without the intermediate } line)
	pattern6 := regexp.MustCompile(`(\\end\{table\*?\})([A-Za-z\p{Han}\\])`)
	if pattern6.MatchString(result) {
		fixCount++
		logger.Debug("fixing: \\end{table*} followed by text on same line")
	}
	result = pattern6.ReplaceAllString(result, "$1\n\n$2")

	if fixCount > 0 {
		logger.Info("FixTableEnvironmentLineStructure: fixed line structure issues",
			logger.Int("fixCount", fixCount))
	}

	return result
}

// FixScaleboxClosingBrace fixes the closing brace of \scalebox that gets merged with following content.
// In LaTeX tables with \scalebox{0.69}{...}, the closing } should be on its own line after \end{table*}.
// LLM often merges this } with the following paragraph text.
func FixScaleboxClosingBrace(content string) string {
	if content == "" {
		return content
	}

	result := content
	fixCount := 0

	// The pattern we're looking for:
	// \end{table*}
	// }Some text here
	//
	// Should become:
	// \end{table*}
	// }
	//
	// Some text here

	// Split into lines and process
	lines := strings.Split(result, "\n")
	var newLines []string
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		
		// Check if this line starts with } followed by content
		if strings.HasPrefix(trimmed, "}") && len(trimmed) > 1 {
			// Check what follows the }
			rest := strings.TrimSpace(trimmed[1:])
			
			// If it's followed by text or a command (not another } or empty)
			if rest != "" && rest[0] != '}' {
				// Check if previous line contains \end{table*} or \end{tabular}
				if i > 0 {
					prevLine := strings.TrimSpace(lines[i-1])
					if strings.Contains(prevLine, "\\end{table") || 
					   strings.Contains(prevLine, "\\end{tabular}") ||
					   strings.Contains(prevLine, "\\label{") {
						// Split this line: } on its own, then blank line, then rest
						newLines = append(newLines, "}")
						newLines = append(newLines, "")
						newLines = append(newLines, rest)
						fixCount++
						logger.Debug("fixed scalebox closing brace",
							logger.Int("lineNum", i+1),
							logger.String("rest", rest[:min(50, len(rest))]))
						continue
					}
				}
			}
		}
		
		newLines = append(newLines, line)
	}

	if fixCount > 0 {
		result = strings.Join(newLines, "\n")
		logger.Info("FixScaleboxClosingBrace: fixed closing braces",
			logger.Int("fixCount", fixCount))
	}

	return result
}



// FixEndTabularBraces fixes cases where \end{tabular}} has the closing brace merged on the same line.
// In LaTeX tables with \scalebox{...}{\begin{tabular}...\end{tabular}}, the structure should be:
//   \end{tabular}
//   }
// But LLM often merges them to:
//   \end{tabular}}
// This causes brace mismatch errors.
//
// NOTE: This function should NOT split \end{tabular}} when it's inside a \resizebox{} because
// in that case the }} is correct (one for tabular, one for resizebox).
func FixEndTabularBraces(content string) string {
	if content == "" {
		return content
	}

	// If content contains \resizebox, don't split \end{tabular}}
	// because in resizebox context, the }} is correct
	if strings.Contains(content, "\\resizebox") {
		return content
	}

	result := content
	fixCount := 0

	// Pattern: \end{tabular}} at end of line (not followed by more content on same line)
	// Should become:
	// \end{tabular}
	// }
	pattern1 := regexp.MustCompile(`(\\end\{tabular\})\}(\s*\n)`)
	if pattern1.MatchString(result) {
		fixCount++
		logger.Debug("fixing: \\end{tabular}} -> \\end{tabular}\\n}")
	}
	result = pattern1.ReplaceAllString(result, "$1\n}$2")

	// Pattern: \end{tabular*}} at end of line
	pattern2 := regexp.MustCompile(`(\\end\{tabular\*\})\}(\s*\n)`)
	if pattern2.MatchString(result) {
		fixCount++
		logger.Debug("fixing: \\end{tabular*}} -> \\end{tabular*}\\n}")
	}
	result = pattern2.ReplaceAllString(result, "$1\n}$2")

	if fixCount > 0 {
		logger.Info("FixEndTabularBraces: fixed closing braces",
			logger.Int("fixCount", fixCount))
	}

	return result
}

// FixSidewaystableStructure fixes cases where \end{sidewaystable} is placed in the wrong position.
// LLM sometimes moves \end{sidewaystable} to right after \scalebox{...}{ instead of after the table content.
// 
// Wrong structure (LLM output):
//   \begin{sidewaystable}[]
//   \caption{...}
//   \scalebox{0.82}{
//   \end{sidewaystable}      <- WRONG: should not be here
//   \begin{tabular}{...}
//   ...
//   \end{tabular}
//   }
//   
// Correct structure:
//   \begin{sidewaystable}[]
//   \caption{...}
//   \scalebox{0.82}{
//   \begin{tabular}{...}
//   ...
//   \end{tabular}
//   }
//   \end{sidewaystable}
func FixSidewaystableStructure(content string) string {
	if content == "" {
		return content
	}

	// Check if there's a sidewaystable environment
	if !strings.Contains(content, "\\begin{sidewaystable}") {
		return content
	}

	// First, count begin and end tags to see if they're balanced
	beginCount := strings.Count(content, "\\begin{sidewaystable}")
	endCount := strings.Count(content, "\\end{sidewaystable}")
	
	// If already balanced, don't add more
	if beginCount == endCount {
		logger.Debug("FixSidewaystableStructure: sidewaystable already balanced",
			logger.Int("count", beginCount))
		return content
	}

	result := content
	fixCount := 0

	// Pattern: \scalebox{...}{\n\end{sidewaystable}\n\begin{tabular}
	// This is the wrong structure where \end{sidewaystable} is misplaced
	// We need to:
	// 1. Remove the misplaced \end{sidewaystable}
	// 2. Find where the table content ends (after \end{tabular}\n})
	// 3. Add \end{sidewaystable} there

	lines := strings.Split(result, "\n")
	var newLines []string
	
	inSidewaystable := false
	misplacedEndFound := false
	scaleboxDepth := 0
	needToAddEnd := false
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		
		// Track sidewaystable environment
		if strings.Contains(trimmed, "\\begin{sidewaystable}") {
			inSidewaystable = true
		}
		
		// Track scalebox depth
		if strings.Contains(trimmed, "\\scalebox{") {
			scaleboxDepth++
		}
		
		// Check for misplaced \end{sidewaystable} right after \scalebox{
		// Pattern: previous line has \scalebox{...}{ and current line is \end{sidewaystable}
		if inSidewaystable && trimmed == "\\end{sidewaystable}" && scaleboxDepth > 0 {
			// Check if next line starts with \begin{tabular}
			if i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextTrimmed, "\\begin{tabular}") {
					// This is the misplaced \end{sidewaystable} - skip it
					misplacedEndFound = true
					needToAddEnd = true
					fixCount++
					logger.Debug("found misplaced \\end{sidewaystable}",
						logger.Int("lineNum", i+1))
					continue // Skip this line
				}
			}
		}
		
		// Track when we exit scalebox (closing brace on its own line after \end{tabular})
		if needToAddEnd && scaleboxDepth > 0 {
			// Check if this line is just "}" after \end{tabular}
			if trimmed == "}" {
				// Check if previous line has \end{tabular}
				if i > 0 {
					prevTrimmed := strings.TrimSpace(lines[i-1])
					if strings.Contains(prevTrimmed, "\\end{tabular}") {
						// Add the line, then add \end{sidewaystable}
						newLines = append(newLines, line)
						newLines = append(newLines, "\\end{sidewaystable}")
						needToAddEnd = false
						inSidewaystable = false
						scaleboxDepth--
						logger.Debug("added \\end{sidewaystable} after scalebox closing brace",
							logger.Int("lineNum", i+1))
						continue
					}
				}
			}
		}
		
		// Normal line - add it
		newLines = append(newLines, line)
		
		// Track scalebox closing
		if trimmed == "}" && scaleboxDepth > 0 {
			scaleboxDepth--
		}
		
		// Track proper \end{sidewaystable}
		if strings.Contains(trimmed, "\\end{sidewaystable}") && !misplacedEndFound {
			inSidewaystable = false
		}
	}

	if fixCount > 0 {
		result = strings.Join(newLines, "\n")
		logger.Info("FixSidewaystableStructure: fixed sidewaystable structure",
			logger.Int("fixCount", fixCount))
	}

	return result
}

// FixExtraClosingBraceAfterTable fixes cases where there's an extra } after \end{table*}.
// This happens when LLM incorrectly adds a closing brace that doesn't match any opening brace.
//
// Wrong structure:
//   \end{table*}
//   }              <- Extra brace that shouldn't be here
//   
//   \textbf{...}
//
// This function removes the extra } if it's not matched by a corresponding opening brace.
func FixExtraClosingBraceAfterTable(content string) string {
	if content == "" {
		return content
	}

	result := content
	fixCount := 0

	// Pattern: \end{table*}\n}\n\n (} on its own line after \end{table*}, followed by blank line)
	// This is often an extra brace that shouldn't be there
	
	// First, let's check if the braces are balanced in table environments
	// We look for patterns like:
	// \setlength{...}
	// \begin{table*}
	// ...
	// \end{table*}
	// }              <- This } might be extra if there's no matching {
	
	lines := strings.Split(result, "\n")
	var newLines []string
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		
		// Check if this line is just "}" and previous line has \end{table*}
		if trimmed == "}" && i > 0 {
			prevTrimmed := strings.TrimSpace(lines[i-1])
			if strings.Contains(prevTrimmed, "\\end{table*}") || strings.Contains(prevTrimmed, "\\end{table}") {
				// Check if there's a matching opening brace
				// Look backwards for \scalebox{ or similar
				hasMatchingOpen := false
				braceCount := 1 // We have one }
				
				for j := i - 1; j >= 0; j-- {
					checkLine := lines[j]
					// Count braces in this line
					for _, c := range checkLine {
						if c == '{' {
							braceCount--
						} else if c == '}' {
							braceCount++
						}
					}
					// If we find \begin{table*}, stop looking
					if strings.Contains(checkLine, "\\begin{table*}") || strings.Contains(checkLine, "\\begin{table}") {
						break
					}
					// If we find \setlength{ or \scalebox{ before the table, that's the matching open
					if strings.Contains(checkLine, "\\setlength{") || strings.Contains(checkLine, "\\scalebox{") {
						hasMatchingOpen = true
						break
					}
				}
				
				// If braceCount is still positive, this } is extra
				if braceCount > 0 && !hasMatchingOpen {
					// Skip this extra }
					fixCount++
					logger.Debug("removing extra } after \\end{table*}",
						logger.Int("lineNum", i+1))
					continue
				}
			}
		}
		
		newLines = append(newLines, line)
	}

	if fixCount > 0 {
		result = strings.Join(newLines, "\n")
		logger.Info("FixExtraClosingBraceAfterTable: removed extra closing braces",
			logger.Int("fixCount", fixCount))
	}

	return result
}

// FixBraceMergedWithComment fixes cases where an opening brace { is incorrectly merged
// with the end of a comment line.
//
// Wrong structure (LLM output):
//   % some comment{
//   \setlength{\tabcolsep}{2.5pt}
//
// Correct structure:
//   % some comment
//   
//   {
//   \setlength{\tabcolsep}{2.5pt}
//
// This happens when LLM merges lines during translation.
func FixBraceMergedWithComment(content string) string {
	if content == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	var newLines []string
	fixCount := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check if this is a comment line ending with {
		// Pattern: % ... {
		if strings.HasPrefix(trimmed, "%") && strings.HasSuffix(trimmed, "{") {
			// Check if next line starts with \setlength, \renewcommand, \begin, etc.
			if i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextTrimmed, "\\setlength") ||
					strings.HasPrefix(nextTrimmed, "\\renewcommand") ||
					strings.HasPrefix(nextTrimmed, "\\begin{table") ||
					strings.HasPrefix(nextTrimmed, "\\begin{figure") ||
					strings.HasPrefix(nextTrimmed, "\\centering") ||
					strings.HasPrefix(nextTrimmed, "\\caption") {
					// Remove the trailing { from comment and add it as a new line
					fixedLine := strings.TrimSuffix(trimmed, "{")
					newLines = append(newLines, fixedLine)
					newLines = append(newLines, "")
					newLines = append(newLines, "{")
					fixCount++
					logger.Debug("fixed brace merged with comment",
						logger.Int("lineNum", i+1))
					continue
				}
			}
		}

		// Also check for { merged at the end of a text line before \setlength
		// Pattern: some text{
		// Next line: \setlength{...}
		if !strings.HasPrefix(trimmed, "%") && strings.HasSuffix(trimmed, "{") && !strings.HasSuffix(trimmed, "{{") {
			if i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextTrimmed, "\\setlength") ||
					strings.HasPrefix(nextTrimmed, "\\renewcommand") {
					// Check if this looks like a standalone { that was merged
					// The { should be on its own line if the next line is \setlength
					// But we need to be careful not to break valid LaTeX like \textbf{text{
					
					// Only fix if the line doesn't contain common LaTeX commands that use {
					if !strings.Contains(trimmed, "\\textbf{") &&
						!strings.Contains(trimmed, "\\textit{") &&
						!strings.Contains(trimmed, "\\emph{") &&
						!strings.Contains(trimmed, "\\cite{") &&
						!strings.Contains(trimmed, "\\ref{") &&
						!strings.Contains(trimmed, "\\label{") {
						// Remove the trailing { and add it as a new line
						fixedLine := strings.TrimSuffix(trimmed, "{")
						newLines = append(newLines, fixedLine)
						newLines = append(newLines, "")
						newLines = append(newLines, "{")
						fixCount++
						logger.Debug("fixed brace merged with text line",
							logger.Int("lineNum", i+1))
						continue
					}
				}
			}
		}

		newLines = append(newLines, line)
	}

	if fixCount > 0 {
		logger.Info("FixBraceMergedWithComment: fixed merged braces",
			logger.Int("fixCount", fixCount))
		return strings.Join(newLines, "\n")
	}

	return content
}

// CleanCCSMathItalics fixes CCS (ACM Computing Classification System) metadata
// that has been incorrectly rendered with mathematical italic Unicode characters.
//
// This happens when XML tags like <conceptid>, <conceptdesc>, etc. are placed
// in math mode during translation, causing each letter to be rendered as a
// mathematical italic character (e.g., 𝑐𝑜𝑛𝑐𝑒𝑝𝑡𝑖𝑑 instead of conceptid).
//
// The function converts these mathematical italic characters back to regular ASCII
// and removes the malformed CCS XML content entirely, as it's metadata that
// shouldn't appear in the rendered PDF.
func CleanCCSMathItalics(content string) string {
	if content == "" {
		return content
	}

	// Check if content contains any mathematical italic characters first
	// This is a quick check to avoid unnecessary processing
	hasMathItalic := false
	for _, r := range content {
		if (r >= 0x1D434 && r <= 0x1D467) || r == 0x210E { // Math italic ranges
			hasMathItalic = true
			break
		}
	}

	// If no math italic characters, just do a quick check for CCS patterns
	if !hasMathItalic {
		// Only process if we see CCS-related patterns
		if !strings.Contains(content, "<ccs") && !strings.Contains(content, "<concept") &&
			!strings.Contains(content, "</ccs") && !strings.Contains(content, "</concept") {
			return content
		}
	}

	// Mathematical italic Unicode ranges:
	// 𝑎-𝑧 (U+1D44E to U+1D467) - lowercase italic
	// 𝐴-𝑍 (U+1D434 to U+1D44D) - uppercase italic

	// Map of mathematical italic characters to ASCII
	mathItalicToASCII := map[rune]rune{
		// Lowercase mathematical italic (U+1D44E - U+1D467)
		'𝑎': 'a', '𝑏': 'b', '𝑐': 'c', '𝑑': 'd', '𝑒': 'e',
		'𝑓': 'f', '𝑔': 'g', 'ℎ': 'h', '𝑖': 'i', '𝑗': 'j',
		'𝑘': 'k', '𝑙': 'l', '𝑚': 'm', '𝑛': 'n', '𝑜': 'o',
		'𝑝': 'p', '𝑞': 'q', '𝑟': 'r', '𝑠': 's', '𝑡': 't',
		'𝑢': 'u', '𝑣': 'v', '𝑤': 'w', '𝑥': 'x', '𝑦': 'y',
		'𝑧': 'z',
		// Uppercase mathematical italic (U+1D434 - U+1D44D)
		'𝐴': 'A', '𝐵': 'B', '𝐶': 'C', '𝐷': 'D', '𝐸': 'E',
		'𝐹': 'F', '𝐺': 'G', '𝐻': 'H', '𝐼': 'I', '𝐽': 'J',
		'𝐾': 'K', '𝐿': 'L', '𝑀': 'M', '𝑁': 'N', '𝑂': 'O',
		'𝑃': 'P', '𝑄': 'Q', '𝑅': 'R', '𝑆': 'S', '𝑇': 'T',
		'𝑈': 'U', '𝑉': 'V', '𝑊': 'W', '𝑋': 'X', '𝑌': 'Y',
		'𝑍': 'Z',
	}

	// Convert mathematical italic characters to ASCII
	var converted strings.Builder
	converted.Grow(len(content))
	for _, r := range content {
		if ascii, ok := mathItalicToASCII[r]; ok {
			converted.WriteRune(ascii)
		} else {
			converted.WriteRune(r)
		}
	}
	result := converted.String()

	// Now remove malformed CCS XML content
	// Only match complete CCS XML tags - be very specific to avoid false positives
	fixCount := 0

	// Pattern for complete CCS blocks (most specific)
	ccsBlockPattern := regexp.MustCompile(`<\s*ccs2012\s*>[\s\S]*?<\s*/\s*ccs2012\s*>`)
	if ccsBlockPattern.MatchString(result) {
		result = ccsBlockPattern.ReplaceAllString(result, "")
		fixCount++
	}

	// Pattern for individual CCS tags (only exact matches with < and >)
	ccsTagPatterns := []string{
		`<\s*/?\s*conceptid\s*>`,
		`<\s*/?\s*conceptdesc\s*>`,
		`<\s*/?\s*conceptsignificance\s*>`,
		`<\s*/?\s*ccs2012\s*>`,
	}

	for _, pattern := range ccsTagPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if re.MatchString(result) {
			result = re.ReplaceAllString(result, "")
			fixCount++
		}
	}

	// Remove CCS concept IDs (very specific pattern: exactly two 8+ digit numbers separated by dot)
	// Only match if surrounded by whitespace or at line boundaries
	ccsNumberPattern := regexp.MustCompile(`(?:^|[\s<])(\d{8,}\.\d{8,})(?:[\s>]|$)`)
	if ccsNumberPattern.MatchString(result) {
		result = ccsNumberPattern.ReplaceAllString(result, " ")
		fixCount++
	}

	if fixCount > 0 {
		// Clean up multiple spaces left after removal (but preserve newlines)
		multiSpacePattern := regexp.MustCompile(`[^\S\n]{2,}`)
		result = multiSpacePattern.ReplaceAllString(result, " ")

		// Clean up lines that are now empty or just whitespace
		emptyLinePattern := regexp.MustCompile(`\n[ \t]*\n[ \t]*\n`)
		result = emptyLinePattern.ReplaceAllString(result, "\n\n")

		logger.Info("CleanCCSMathItalics: cleaned CCS metadata",
			logger.Int("fixCount", fixCount))
	}

	return result
}


// fixMismatchedCommentedEnvironments fixes cases where \begin{env} is commented
// but the corresponding \end{env} is not (or vice versa).
//
// This commonly happens when LLM translation incorrectly adds or removes comment
// markers from only one part of an environment pair.
//
// Example problem:
//   % \begin{CCSXML}    <- commented
//   <content>
//   \end{CCSXML}        <- NOT commented (causes LaTeX error!)
//
// This function ensures that if \begin{env} is commented, \end{env} is also commented,
// and vice versa. It uses the original content as reference to determine the correct state.
func fixMismatchedCommentedEnvironments(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated

	// Find all environment names used in the document
	envPattern := regexp.MustCompile(`\\(?:begin|end)\{([^}]+)\}`)
	envNames := make(map[string]bool)
	for _, match := range envPattern.FindAllStringSubmatch(original, -1) {
		envNames[match[1]] = true
	}
	for _, match := range envPattern.FindAllStringSubmatch(translated, -1) {
		envNames[match[1]] = true
	}

	// For each environment, check if begin/end comment status matches
	for envName := range envNames {
		result = fixSingleEnvironmentComments(result, original, envName)
	}

	return result
}

// fixSingleEnvironmentComments fixes comment mismatches for a single environment type.
// The primary goal is to ensure consistency: if \begin{env} is commented, \end{env} must also be commented.
func fixSingleEnvironmentComments(translated, original, envName string) string {
	// Patterns to find commented and uncommented begin/end
	beginTag := `\begin{` + envName + `}`
	endTag := `\end{` + envName + `}`

	// Check if tags exist at all in translated
	transHasBegin := strings.Contains(translated, beginTag)
	transHasEnd := strings.Contains(translated, endTag)

	if !transHasBegin && !transHasEnd {
		return translated
	}

	// Check translated comment status
	transBeginCommented := isTagCommented(translated, beginTag)
	transEndCommented := isTagCommented(translated, endTag)

	result := translated

	// PRIORITY 1: Ensure consistency between begin and end
	// If begin is commented but end is not, comment the end
	if transBeginCommented && !transEndCommented && transHasEnd {
		result = commentTag(result, endTag)
		logger.Debug("fixed mismatched comment: commented \\end to match \\begin",
			logger.String("env", envName))
		return result
	}

	// If end is commented but begin is not, uncomment the end (to match begin)
	if !transBeginCommented && transEndCommented && transHasBegin {
		result = uncommentTag(result, endTag)
		logger.Debug("fixed mismatched comment: uncommented \\end to match \\begin",
			logger.String("env", envName))
		return result
	}

	// PRIORITY 2: Try to match original if both are consistent in translated
	// Only do this if we have original content to compare
	if original == "" {
		return result
	}

	origBeginCommented := isTagCommented(original, beginTag)
	origEndCommented := isTagCommented(original, endTag)

	// If original has both uncommented, and translated has both commented, uncomment both
	if !origBeginCommented && !origEndCommented && transBeginCommented && transEndCommented {
		result = uncommentTag(result, beginTag)
		result = uncommentTag(result, endTag)
		logger.Debug("fixed wrongly commented environment pair",
			logger.String("env", envName))
	}

	// If original has both commented, and translated has both uncommented, comment both
	if origBeginCommented && origEndCommented && !transBeginCommented && !transEndCommented {
		result = commentTag(result, beginTag)
		result = commentTag(result, endTag)
		logger.Debug("fixed wrongly uncommented environment pair",
			logger.String("env", envName))
	}

	return result
}

// isTagCommented checks if a tag appears only in commented form in the content.
// Returns true if all occurrences of the tag are preceded by % on the same line.
func isTagCommented(content, tag string) bool {
	if !strings.Contains(content, tag) {
		return false
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, tag) {
			// Check if this line has the tag after a % (commented)
			idx := strings.Index(line, tag)
			percentIdx := strings.Index(line, "%")
			
			// If there's no % before the tag, it's uncommented
			if percentIdx == -1 || percentIdx > idx {
				return false
			}
		}
	}
	
	// All occurrences are commented
	return true
}

// isLastOccurrenceCommented checks if the last occurrence of a tag is commented.
// Returns true if the tag is preceded by % on the same line.
func isLastOccurrenceCommented(content, tag string) bool {
	return isTagCommented(content, tag)
}

// commentTag adds a comment marker to the first uncommented occurrence of a tag.
func commentTag(content, tag string) string {
	lines := strings.Split(content, "\n")
	
	for i, line := range lines {
		if strings.Contains(line, tag) {
			idx := strings.Index(line, tag)
			percentIdx := strings.Index(line, "%")
			
			// If there's no % before the tag, comment this line
			if percentIdx == -1 || percentIdx > idx {
				// Find leading whitespace
				leadingSpace := ""
				for _, ch := range line {
					if ch == ' ' || ch == '\t' {
						leadingSpace += string(ch)
					} else {
						break
					}
				}
				// Replace the tag with commented version
				lines[i] = strings.Replace(line, tag, "% "+tag, 1)
				// If the tag was at the start (after whitespace), adjust
				if strings.HasPrefix(strings.TrimSpace(line), tag[1:]) { // tag starts with \
					lines[i] = leadingSpace + "% " + strings.TrimSpace(line)
				}
				break
			}
		}
	}
	
	return strings.Join(lines, "\n")
}

// uncommentTag removes the comment marker from the first commented occurrence of a tag.
func uncommentTag(content, tag string) string {
	lines := strings.Split(content, "\n")
	
	for i, line := range lines {
		if strings.Contains(line, tag) {
			idx := strings.Index(line, tag)
			percentIdx := strings.Index(line, "%")
			
			// If there's a % before the tag, uncomment this line
			if percentIdx != -1 && percentIdx < idx {
				// Remove the % and any space after it
				beforePercent := line[:percentIdx]
				afterPercent := line[percentIdx+1:]
				// Remove leading space after %
				afterPercent = strings.TrimPrefix(afterPercent, " ")
				lines[i] = beforePercent + afterPercent
				break
			}
		}
	}
	
	return strings.Join(lines, "\n")
}


// RestoreEnvironments restores environment tags in translated content by comparing with original.
// This function implements Requirements 3.3 and 3.4:
// - 3.3: Missing \end{...} tags must be auto-inserted
// - 3.4: Environment tag restoration must maintain correct nesting order
//
// The function performs the following steps:
// 1. Validate environment matching in translated content
// 2. Detect missing \end{...} tags by comparing with original
// 3. Auto-insert missing end tags at appropriate positions
// 4. Ensure proper nesting order is maintained
//
// Parameters:
//   - translated: the translated content that may have missing environment tags
//   - original: the original content used as reference for correct structure
//
// Returns the fixed content with restored environment tags.
func RestoreEnvironments(translated, original string) string {
	if translated == "" {
		return translated
	}

	result := translated

	// Step 1: Validate current environment state
	validation := ValidateEnvironments(result)
	
	if validation.IsValid {
		// All environments are already balanced
		logger.Debug("RestoreEnvironments: environments already balanced")
		return result
	}

	// Step 2: Log the issues found
	for _, mismatch := range validation.Mismatches {
		if mismatch.Difference > 0 {
			logger.Info("RestoreEnvironments: missing \\end tags detected",
				logger.String("env", mismatch.EnvName),
				logger.Int("missing", mismatch.Difference))
		} else {
			logger.Info("RestoreEnvironments: extra \\end tags detected",
				logger.String("env", mismatch.EnvName),
				logger.Int("extra", -mismatch.Difference))
		}
	}

	if !validation.NestingValid {
		logger.Warn("RestoreEnvironments: nesting error detected",
			logger.String("error", validation.NestingError))
	}

	// Step 3: Fix environment issues using the enhanced nesting-aware approach
	// This handles both missing end tags and maintains proper nesting order
	result = insertMissingEndTagsWithNesting(result, original)

	// Step 4: Validate again to confirm fixes
	finalValidation := ValidateEnvironments(result)
	if !finalValidation.IsValid {
		logger.Warn("RestoreEnvironments: some issues remain after restoration",
			logger.Int("remainingMismatches", len(finalValidation.Mismatches)))
	} else {
		logger.Info("RestoreEnvironments: all environment issues fixed")
	}

	return result
}

// fixEnvironmentNestingOrder fixes nesting order issues in the content.
// This handles cases where environments are improperly nested (e.g., overlapping).
func fixEnvironmentNestingOrder(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated

	// Parse environment positions
	positions := parseEnvironmentPositions(result)
	
	// Use a stack to track open environments
	stack := []envPositionInfo{}
	fixes := []nestingFix{}
	
	for _, pos := range positions {
		if pos.isBegin {
			stack = append(stack, pos)
		} else {
			// Found an \end tag
			if len(stack) == 0 {
				// Extra \end without matching \begin - mark for potential removal
				logger.Warn("fixEnvironmentNestingOrder: extra \\end tag",
					logger.String("env", pos.envName),
					logger.Int("line", pos.line))
				continue
			}
			
			top := stack[len(stack)-1]
			if top.envName == pos.envName {
				// Proper match
				stack = stack[:len(stack)-1]
			} else {
				// Nesting error - find the matching \begin in the stack
				matchIdx := -1
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].envName == pos.envName {
						matchIdx = i
						break
					}
				}
				
				if matchIdx >= 0 {
					// Need to close all environments between matchIdx and top of stack
					for i := len(stack) - 1; i > matchIdx; i-- {
						fixes = append(fixes, nestingFix{
							insertBefore: pos.position,
							envName:      stack[i].envName,
							fixType:      "insert_end",
						})
					}
					// Remove closed environments from stack
					stack = stack[:matchIdx]
				} else {
					// No matching \begin found - this \end is orphaned
					logger.Warn("fixEnvironmentNestingOrder: orphaned \\end tag",
						logger.String("env", pos.envName),
						logger.Int("line", pos.line))
				}
			}
		}
	}
	
	// Apply fixes in reverse order
	result = applyNestingFixes(result, fixes)
	
	return result
}

// nestingFix represents a fix needed for nesting order
type nestingFix struct {
	insertBefore int
	envName      string
	fixType      string // "insert_end" or "remove_end"
}

// applyNestingFixes applies nesting fixes to the content
func applyNestingFixes(content string, fixes []nestingFix) string {
	if len(fixes) == 0 {
		return content
	}
	
	// Sort fixes by position in descending order
	sortedFixes := make([]nestingFix, len(fixes))
	copy(sortedFixes, fixes)
	
	for i := 0; i < len(sortedFixes)-1; i++ {
		for j := 0; j < len(sortedFixes)-i-1; j++ {
			if sortedFixes[j].insertBefore < sortedFixes[j+1].insertBefore {
				sortedFixes[j], sortedFixes[j+1] = sortedFixes[j+1], sortedFixes[j]
			}
		}
	}
	
	result := content
	for _, fix := range sortedFixes {
		if fix.fixType == "insert_end" {
			pos := fix.insertBefore
			if pos > len(result) {
				pos = len(result)
			}
			
			endTag := "\\end{" + fix.envName + "}"
			result = result[:pos] + "\n" + endTag + "\n" + result[pos:]
			
			logger.Info("fixEnvironmentNestingOrder: inserted \\end tag",
				logger.String("env", fix.envName),
				logger.Int("position", pos))
		}
	}
	
	return result
}

// RestoreEnvironmentsResult contains detailed information about environment restoration
type RestoreEnvironmentsResult struct {
	Content           string        // The restored content
	OriginalValid     bool          // Whether original had valid environments
	TranslatedValid   bool          // Whether translated had valid environments before fix
	RestoredValid     bool          // Whether restored content has valid environments
	InsertedEndTags   []string      // List of \end tags that were inserted
	FixedNestingOrder bool          // Whether nesting order was fixed
	Errors            []string      // Any errors encountered during restoration
}

// RestoreEnvironmentsWithDetails performs environment restoration and returns detailed results
func RestoreEnvironmentsWithDetails(translated, original string) *RestoreEnvironmentsResult {
	result := &RestoreEnvironmentsResult{
		InsertedEndTags: []string{},
		Errors:          []string{},
	}

	if translated == "" {
		result.Content = translated
		result.RestoredValid = true
		return result
	}

	// Validate original
	if original != "" {
		origValidation := ValidateEnvironments(original)
		result.OriginalValid = origValidation.IsValid
	} else {
		result.OriginalValid = true
	}

	// Validate translated before fix
	transValidation := ValidateEnvironments(translated)
	result.TranslatedValid = transValidation.IsValid

	if result.TranslatedValid {
		result.Content = translated
		result.RestoredValid = true
		return result
	}

	// Track which end tags need to be inserted
	for _, mismatch := range transValidation.Mismatches {
		if mismatch.Difference > 0 {
			for i := 0; i < mismatch.Difference; i++ {
				result.InsertedEndTags = append(result.InsertedEndTags, "\\end{"+mismatch.EnvName+"}")
			}
		}
	}

	// Check if nesting order needs fixing
	result.FixedNestingOrder = !transValidation.NestingValid

	// Perform restoration
	content := translated
	content = fixEnvironmentsByReference(content, original)
	content = fixEnvironmentNestingOrder(content, original)

	result.Content = content

	// Validate result
	finalValidation := ValidateEnvironments(content)
	result.RestoredValid = finalValidation.IsValid

	if !result.RestoredValid {
		for _, mismatch := range finalValidation.Mismatches {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("remaining mismatch: %s (diff=%d)", mismatch.EnvName, mismatch.Difference))
		}
		if !finalValidation.NestingValid {
			result.Errors = append(result.Errors, "nesting still invalid: "+finalValidation.NestingError)
		}
	}

	logger.Info("RestoreEnvironmentsWithDetails completed",
		logger.Bool("originalValid", result.OriginalValid),
		logger.Bool("translatedValid", result.TranslatedValid),
		logger.Bool("restoredValid", result.RestoredValid),
		logger.Int("insertedEndTags", len(result.InsertedEndTags)),
		logger.Bool("fixedNesting", result.FixedNestingOrder))

	return result
}


// RestoreLineStructure restores the line structure of translated content to match the original.
// This function addresses Requirements 2.1, 2.2, 2.3, 2.4, 2.5:
// - 2.1: Preserve line count between original and translated content
// - 2.2: Ensure \begin{...} commands remain on their own line
// - 2.3: Ensure \end{...} commands remain on their own line
// - 2.4: Ensure each \item remains on a separate line
// - 2.5: Split merged lines back to match original structure
//
// The function performs the following steps:
// 1. Compare line counts between original and translated
// 2. Identify and fix merged lines (where multiple commands are on one line)
// 3. Ensure structural commands (\begin, \end, \item) are on separate lines
// 4. Attempt to match the original line structure as closely as possible
func RestoreLineStructure(translated, original string) string {
	if translated == "" {
		return translated
	}

	result := translated

	// Step 1: Apply basic line structure fixes first
	result = fixLineStructure(result)

	// Step 2: Ensure \begin{...}, \end{...}, and \item are on separate lines
	result = ensureCommandsOnSeparateLines(result)

	// Step 3: If we have original content, try to match its structure
	if original != "" {
		result = matchOriginalLineStructure(result, original)
	}

	// Step 4: Final cleanup - ensure no multiple structural commands on same line
	result = splitMergedStructuralCommands(result)

	if result != translated {
		logger.Debug("RestoreLineStructure made changes",
			logger.Int("originalLen", len(translated)),
			logger.Int("restoredLen", len(result)))
	}

	return result
}

// ensureCommandsOnSeparateLines ensures that \begin{...}, \end{...}, and \item
// commands are each on their own line.
func ensureCommandsOnSeparateLines(content string) string {
	result := content

	// Pattern: text\begin{env} -> text\n\begin{env}
	// Match any non-whitespace character followed by \begin{
	beginPattern := regexp.MustCompile(`([^\n\s])\s*(\\begin\{[^}]+\})`)
	result = beginPattern.ReplaceAllString(result, "$1\n$2")

	// Pattern: \begin{env}text -> \begin{env}\ntext
	// Match \begin{env} followed by non-whitespace (except [ for optional args)
	beginFollowedPattern := regexp.MustCompile(`(\\begin\{[^}]+\}(?:\[[^\]]*\])?)\s*([^\n\s\[\\])`)
	result = beginFollowedPattern.ReplaceAllString(result, "$1\n$2")

	// Pattern: text\end{env} -> text\n\end{env}
	endPattern := regexp.MustCompile(`([^\n\s])\s*(\\end\{[^}]+\})`)
	result = endPattern.ReplaceAllString(result, "$1\n$2")

	// Pattern: \end{env}text -> \end{env}\ntext
	// Match \end{env} followed by non-whitespace (except another \)
	endFollowedPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*([^\n\s\\])`)
	result = endFollowedPattern.ReplaceAllString(result, "$1\n$2")

	// Pattern: text\item -> text\n\item
	// But be careful not to break \item that's already at line start
	itemPattern := regexp.MustCompile(`([^\n\s])\s*(\\item\b)`)
	result = itemPattern.ReplaceAllString(result, "$1\n$2")

	// Pattern: \item text\item -> \item text\n\item
	// Multiple \item on same line
	multiItemPattern := regexp.MustCompile(`(\\item\s+[^\n]*[^\s])\s+(\\item\b)`)
	// Apply multiple times to handle chains of \item
	for i := 0; i < 5; i++ {
		newResult := multiItemPattern.ReplaceAllString(result, "$1\n$2")
		if newResult == result {
			break
		}
		result = newResult
	}

	return result
}

// matchOriginalLineStructure attempts to match the translated content's line structure
// to the original content's structure.
func matchOriginalLineStructure(translated, original string) string {
	origLines := strings.Split(original, "\n")
	transLines := strings.Split(translated, "\n")

	// If line counts are very different, we can't reliably match
	// Use a tolerance of 20% difference
	lineDiff := abs(len(origLines) - len(transLines))
	tolerance := len(origLines) / 5 // 20%
	if tolerance < 5 {
		tolerance = 5 // Minimum tolerance of 5 lines
	}

	if lineDiff > tolerance {
		logger.Debug("line count difference too large for structure matching",
			logger.Int("original", len(origLines)),
			logger.Int("translated", len(transLines)),
			logger.Int("tolerance", tolerance))
		return translated
	}

	result := translated

	// Analyze original structure to find key structural lines
	origStructure := analyzeLineStructure(original)
	transStructure := analyzeLineStructure(translated)

	// Try to align structural elements
	result = alignStructuralElements(result, origStructure, transStructure)

	return result
}

// LineStructureInfo contains information about structural elements in content
type LineStructureInfo struct {
	BeginLines    []int    // Line numbers containing \begin{...}
	EndLines      []int    // Line numbers containing \end{...}
	ItemLines     []int    // Line numbers containing \item
	SectionLines  []int    // Line numbers containing \section, \subsection, etc.
	TotalLines    int      // Total number of lines
	EmptyLines    []int    // Line numbers that are empty
}

// analyzeLineStructure analyzes the line structure of content
func analyzeLineStructure(content string) *LineStructureInfo {
	lines := strings.Split(content, "\n")
	info := &LineStructureInfo{
		TotalLines: len(lines),
	}

	beginRe := regexp.MustCompile(`\\begin\{[^}]+\}`)
	endRe := regexp.MustCompile(`\\end\{[^}]+\}`)
	itemRe := regexp.MustCompile(`\\item\b`)
	sectionRe := regexp.MustCompile(`\\(?:section|subsection|subsubsection|chapter|paragraph)\*?\{`)

	for i, line := range lines {
		lineNum := i + 1 // 1-indexed

		if strings.TrimSpace(line) == "" {
			info.EmptyLines = append(info.EmptyLines, lineNum)
		}

		if beginRe.MatchString(line) {
			info.BeginLines = append(info.BeginLines, lineNum)
		}

		if endRe.MatchString(line) {
			info.EndLines = append(info.EndLines, lineNum)
		}

		if itemRe.MatchString(line) {
			info.ItemLines = append(info.ItemLines, lineNum)
		}

		if sectionRe.MatchString(line) {
			info.SectionLines = append(info.SectionLines, lineNum)
		}
	}

	return info
}

// alignStructuralElements attempts to align structural elements between translated
// and original content
func alignStructuralElements(translated string, origInfo, transInfo *LineStructureInfo) string {
	// If the translated content has fewer structural elements on separate lines,
	// we may need to split some lines

	result := translated

	// Check if we have merged \begin commands
	if len(transInfo.BeginLines) < len(origInfo.BeginLines) {
		// Some \begin commands might be merged - try to split them
		result = splitMergedBeginCommands(result)
	}

	// Check if we have merged \end commands
	if len(transInfo.EndLines) < len(origInfo.EndLines) {
		// Some \end commands might be merged - try to split them
		result = splitMergedEndCommands(result)
	}

	// Check if we have merged \item commands
	if len(transInfo.ItemLines) < len(origInfo.ItemLines) {
		// Some \item commands might be merged - try to split them
		result = splitMergedItemCommands(result)
	}

	return result
}

// splitMergedBeginCommands splits lines that have multiple \begin commands
func splitMergedBeginCommands(content string) string {
	// Pattern: \begin{env1}...\begin{env2} on same line
	pattern := regexp.MustCompile(`(\\begin\{[^}]+\}(?:\[[^\]]*\])?[^\n]*)(\\begin\{[^}]+\})`)
	
	result := content
	for i := 0; i < 10; i++ { // Limit iterations to prevent infinite loop
		newResult := pattern.ReplaceAllString(result, "$1\n$2")
		if newResult == result {
			break
		}
		result = newResult
	}
	
	return result
}

// splitMergedEndCommands splits lines that have multiple \end commands
func splitMergedEndCommands(content string) string {
	// Pattern: \end{env1}...\end{env2} on same line
	pattern := regexp.MustCompile(`(\\end\{[^}]+\}[^\n]*)(\\end\{[^}]+\})`)
	
	result := content
	for i := 0; i < 10; i++ {
		newResult := pattern.ReplaceAllString(result, "$1\n$2")
		if newResult == result {
			break
		}
		result = newResult
	}
	
	return result
}

// splitMergedItemCommands splits lines that have multiple \item commands
func splitMergedItemCommands(content string) string {
	// Pattern: \item ... \item on same line
	pattern := regexp.MustCompile(`(\\item\s+[^\n]*[^\s])\s+(\\item\b)`)
	
	result := content
	for i := 0; i < 10; i++ {
		newResult := pattern.ReplaceAllString(result, "$1\n$2")
		if newResult == result {
			break
		}
		result = newResult
	}
	
	return result
}

// splitMergedStructuralCommands performs a final pass to split any remaining
// merged structural commands
func splitMergedStructuralCommands(content string) string {
	result := content

	// Split \end{env}\begin{env} patterns
	endBeginPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\begin\{[^}]+\})`)
	result = endBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Split \end{env}\section patterns
	endSectionPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\(?:section|subsection|subsubsection|paragraph|chapter)\*?\{)`)
	result = endSectionPattern.ReplaceAllString(result, "$1\n\n$2")

	// Split }\begin{env} patterns (closing brace followed by begin)
	braceBeginPattern := regexp.MustCompile(`(\})\s*(\\begin\{(?:itemize|enumerate|description|quote|quotation|center|flushleft|flushright|figure|table|equation|align)\*?\})`)
	result = braceBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Split \item\begin{env} patterns
	itemBeginPattern := regexp.MustCompile(`(\\item[^\n]*[^\s])\s*(\\begin\{[^}]+\})`)
	result = itemBeginPattern.ReplaceAllString(result, "$1\n$2")

	// Split \end{env}\item patterns
	endItemPattern := regexp.MustCompile(`(\\end\{[^}]+\})\s*(\\item\b)`)
	result = endItemPattern.ReplaceAllString(result, "$1\n$2")

	return result
}

// RestoreLineStructureResult contains the result of line structure restoration
type RestoreLineStructureResult struct {
	Content          string   // The restored content
	OriginalLines    int      // Number of lines in original
	RestoredLines    int      // Number of lines after restoration
	FixesApplied     []string // List of fixes that were applied
	LineDifference   int      // Difference in line count (restored - original)
}

// RestoreLineStructureWithDetails performs line structure restoration and returns
// detailed information about the changes made.
func RestoreLineStructureWithDetails(translated, original string) *RestoreLineStructureResult {
	result := &RestoreLineStructureResult{
		OriginalLines: len(strings.Split(original, "\n")),
	}

	if translated == "" {
		result.Content = translated
		result.RestoredLines = 0
		return result
	}

	content := translated
	initialLines := len(strings.Split(content, "\n"))

	// Step 1: Apply basic line structure fixes
	content = fixLineStructure(content)
	if content != translated {
		result.FixesApplied = append(result.FixesApplied, "basic line structure fixes")
	}

	// Step 2: Ensure commands on separate lines
	beforeEnsure := content
	content = ensureCommandsOnSeparateLines(content)
	if content != beforeEnsure {
		result.FixesApplied = append(result.FixesApplied, "ensured commands on separate lines")
	}

	// Step 3: Match original structure if available
	if original != "" {
		beforeMatch := content
		content = matchOriginalLineStructure(content, original)
		if content != beforeMatch {
			result.FixesApplied = append(result.FixesApplied, "matched original line structure")
		}
	}

	// Step 4: Split merged structural commands
	beforeSplit := content
	content = splitMergedStructuralCommands(content)
	if content != beforeSplit {
		result.FixesApplied = append(result.FixesApplied, "split merged structural commands")
	}

	result.Content = content
	result.RestoredLines = len(strings.Split(content, "\n"))
	result.LineDifference = result.RestoredLines - result.OriginalLines

	if len(result.FixesApplied) > 0 {
		logger.Info("RestoreLineStructureWithDetails completed",
			logger.Int("originalLines", result.OriginalLines),
			logger.Int("initialLines", initialLines),
			logger.Int("restoredLines", result.RestoredLines),
			logger.Int("lineDifference", result.LineDifference),
			logger.Int("fixCount", len(result.FixesApplied)))
	}

	return result
}


// RestoreBraces restores brace balance in translated content by comparing with original.
// This function implements Requirements 4.3 and 4.5:
// - 4.3: IF closing braces are missing in \multirow or \multicolumn commands,
//        THEN THE Format_Restorer SHALL add the missing braces
// - 4.5: IF the brace count differs from the original,
//        THEN THE Format_Restorer SHALL attempt to restore the original balance
//
// The function performs the following steps:
// 1. Validate brace balance in translated content
// 2. Compare brace counts with original content
// 3. Fix multirow/multicolumn specific brace issues
// 4. Apply general brace balance restoration
// 5. Ensure proper nesting structure is maintained
//
// Parameters:
//   - translated: the translated content that may have brace imbalance
//   - original: the original content used as reference for correct brace count
//
// Returns the fixed content with restored brace balance.
func RestoreBraces(translated, original string) string {
	if translated == "" {
		return translated
	}

	result := translated

	// Step 1: Validate current brace state
	validation := ValidateBraces(result)

	if validation.IsBalanced {
		// Braces are already balanced, but still check multirow/multicolumn
		// as they may have structural issues even with balanced braces
		result = fixMultirowMulticolumnBraces(result, original)
		logger.Debug("RestoreBraces: braces already balanced, checked multirow/multicolumn")
		return result
	}

	logger.Info("RestoreBraces: brace imbalance detected",
		logger.Int("openCount", validation.OpenCount),
		logger.Int("closeCount", validation.CloseCount),
		logger.Int("difference", validation.Difference))

	// Step 2: Fix multirow/multicolumn specific issues first
	// These are the most common brace issues in LaTeX tables
	result = fixMultirowMulticolumnBraces(result, original)

	// Step 3: Fix general brace balance issues
	result = fixGeneralBraceBalance(result, original)

	// Step 4: Apply context-aware brace restoration
	result = restoreBracesContextAware(result, original)

	// Step 5: Final validation and cleanup
	finalValidation := ValidateBraces(result)
	if !finalValidation.IsBalanced {
		// If still unbalanced, try simple brace addition/removal
		result = fixRemainingBraceImbalance(result, original)
	}

	logger.Info("RestoreBraces: completed",
		logger.Bool("wasBalanced", validation.IsBalanced),
		logger.Bool("nowBalanced", ValidateBraces(result).IsBalanced))

	return result
}

// fixGeneralBraceBalance fixes general brace balance issues by comparing with original.
// This handles cases beyond multirow/multicolumn, such as:
// - Missing closing braces in \textbf{}, \textit{}, etc.
// - Extra closing braces added by LLM
// - Braces dropped from nested commands
func fixGeneralBraceBalance(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated
	fixCount := 0

	// ============================================================
	// PATTERN GROUP 1: Common formatting commands with missing braces
	// ============================================================

	// Pattern: \textbf{text followed by space and punctuation (missing closing brace)
	// Example: \textbf{Important note. -> \textbf{Important note}.
	textbfMissing := regexp.MustCompile(`(\\textbf\{[^}]+)(\s*[.,:;!?])(\s)`)
	if textbfMissing.MatchString(result) {
		fixCount++
		logger.Debug("fixing textbf missing closing brace before punctuation")
	}
	result = textbfMissing.ReplaceAllString(result, "$1}$2$3")

	// Pattern: \textit{text followed by space and punctuation
	textitMissing := regexp.MustCompile(`(\\textit\{[^}]+)(\s*[.,:;!?])(\s)`)
	if textitMissing.MatchString(result) {
		fixCount++
		logger.Debug("fixing textit missing closing brace before punctuation")
	}
	result = textitMissing.ReplaceAllString(result, "$1}$2$3")

	// Pattern: \emph{text followed by space and punctuation
	emphMissing := regexp.MustCompile(`(\\emph\{[^}]+)(\s*[.,:;!?])(\s)`)
	if emphMissing.MatchString(result) {
		fixCount++
		logger.Debug("fixing emph missing closing brace before punctuation")
	}
	result = emphMissing.ReplaceAllString(result, "$1}$2$3")

	// ============================================================
	// PATTERN GROUP 2: Commands at end of line missing closing brace
	// ============================================================

	// Pattern: \textbf{text at end of line (missing closing brace)
	textbfEndLine := regexp.MustCompile(`(\\textbf\{[^}\n]+)(\n)`)
	if textbfEndLine.MatchString(result) {
		// Only fix if the line doesn't already have balanced braces
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			if strings.Contains(line, "\\textbf{") {
				balance := countLineBraces(line)
				if balance > 0 {
					// More opening than closing braces
					lines[i] = line + strings.Repeat("}", balance)
					fixCount++
					logger.Debug("fixing textbf missing closing brace at end of line",
						logger.Int("line", i+1))
				}
			}
		}
		result = strings.Join(lines, "\n")
	}

	// ============================================================
	// PATTERN GROUP 3: Nested commands with missing braces
	// ============================================================

	// Pattern: \textbf{\textsc{text} missing outer closing brace
	// Example: \textbf{\textsc{Title} -> \textbf{\textsc{Title}}
	nestedMissing := regexp.MustCompile(`(\\textbf\{\\textsc\{[^}]+\})(\s*[&\\])`)
	if nestedMissing.MatchString(result) {
		fixCount++
		logger.Debug("fixing nested textbf/textsc missing closing brace")
	}
	result = nestedMissing.ReplaceAllString(result, "$1}$2")

	// Pattern: \textit{\textbf{text} missing outer closing brace
	nestedItBf := regexp.MustCompile(`(\\textit\{\\textbf\{[^}]+\})(\s*[&\\])`)
	if nestedItBf.MatchString(result) {
		fixCount++
		logger.Debug("fixing nested textit/textbf missing closing brace")
	}
	result = nestedItBf.ReplaceAllString(result, "$1}$2")

	// ============================================================
	// PATTERN GROUP 4: \scalebox and \resizebox with missing braces
	// ============================================================

	// Pattern: \scalebox{factor}{ content without closing braces
	// This is common when LLM drops the closing braces of scalebox
	scaleboxMissing := regexp.MustCompile(`(\\scalebox\{[^}]+\}\{[^}]+)(\\end\{)`)
	if scaleboxMissing.MatchString(result) {
		fixCount++
		logger.Debug("fixing scalebox missing closing brace before \\end")
	}
	result = scaleboxMissing.ReplaceAllString(result, "$1}\n$2")

	// Pattern: \resizebox{width}{height}{ content without closing braces
	resizeboxMissing := regexp.MustCompile(`(\\resizebox\{[^}]+\}\{[^}]+\}\{[^}]+)(\\end\{)`)
	if resizeboxMissing.MatchString(result) {
		fixCount++
		logger.Debug("fixing resizebox missing closing brace before \\end")
	}
	result = resizeboxMissing.ReplaceAllString(result, "$1}\n$2")

	if fixCount > 0 {
		logger.Info("fixGeneralBraceBalance: fixed braces",
			logger.Int("fixCount", fixCount))
	}

	return result
}

// restoreBracesContextAware performs context-aware brace restoration by analyzing
// the structure of both original and translated content.
func restoreBracesContextAware(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated

	// Get brace counts from both contents
	origOpen, origClose := countBraces(original)
	transOpen, transClose := countBraces(result)

	origBalance := origOpen - origClose
	transBalance := transOpen - transClose

	// If balances match, no need for context-aware fixes
	if origBalance == transBalance {
		return result
	}

	logger.Debug("restoreBracesContextAware: balance mismatch",
		logger.Int("origBalance", origBalance),
		logger.Int("transBalance", transBalance))

	// Analyze line-by-line to find where braces are missing
	origLines := strings.Split(original, "\n")
	transLines := strings.Split(result, "\n")

	// Only proceed if line counts are similar (within 20%)
	if abs(len(origLines)-len(transLines)) > len(origLines)/5 {
		logger.Debug("restoreBracesContextAware: line count too different, skipping")
		return result
	}

	// Compare brace balance line by line
	minLines := len(origLines)
	if len(transLines) < minLines {
		minLines = len(transLines)
	}

	fixedLines := make([]string, len(transLines))
	copy(fixedLines, transLines)
	fixCount := 0

	for i := 0; i < minLines; i++ {
		origLineBalance := countLineBraces(origLines[i])
		transLineBalance := countLineBraces(transLines[i])

		if origLineBalance != transLineBalance {
			diff := origLineBalance - transLineBalance
			if diff > 0 {
				// Original has more opening braces (or fewer closing)
				// Translated might have extra closing braces - remove them
				fixedLines[i] = removeExtraClosingBraces(transLines[i], diff)
				if fixedLines[i] != transLines[i] {
					fixCount++
				}
			} else if diff < 0 {
				// Original has fewer opening braces (or more closing)
				// Translated might be missing closing braces - add them
				fixedLines[i] = addMissingClosingBraces(transLines[i], -diff)
				if fixedLines[i] != transLines[i] {
					fixCount++
				}
			}
		}
	}

	if fixCount > 0 {
		result = strings.Join(fixedLines, "\n")
		logger.Info("restoreBracesContextAware: fixed lines",
			logger.Int("fixCount", fixCount))
	}

	return result
}

// removeExtraClosingBraces removes extra closing braces from a line.
// It removes braces from the end of the line first.
func removeExtraClosingBraces(line string, count int) string {
	result := line
	removed := 0

	// Remove from end of line first
	for removed < count {
		trimmed := strings.TrimRight(result, " \t")
		if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '}' {
			// Check if it's not an escaped brace
			if len(trimmed) < 2 || trimmed[len(trimmed)-2] != '\\' {
				result = trimmed[:len(trimmed)-1]
				removed++
				continue
			}
		}
		break
	}

	return result
}

// addMissingClosingBraces adds missing closing braces to a line.
// It adds braces at the end of the line.
func addMissingClosingBraces(line string, count int) string {
	// Don't add braces to empty lines or comment lines
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%") {
		return line
	}

	return line + strings.Repeat("}", count)
}

// fixRemainingBraceImbalance performs a final pass to fix any remaining brace imbalance.
// This is a last resort when other methods haven't fully balanced the braces.
func fixRemainingBraceImbalance(translated, original string) string {
	if original == "" {
		return translated
	}

	result := translated

	// Get final brace counts
	origOpen, origClose := countBraces(original)
	transOpen, transClose := countBraces(result)

	origBalance := origOpen - origClose
	transBalance := transOpen - transClose

	if origBalance == transBalance {
		return result
	}

	diff := transBalance - origBalance

	if diff > 0 {
		// Too many opening braces (or too few closing)
		// Add closing braces at the end
		result = strings.TrimRight(result, "\n") + strings.Repeat("}", diff) + "\n"
		logger.Info("fixRemainingBraceImbalance: added closing braces",
			logger.Int("count", diff))
	} else if diff < 0 {
		// Too many closing braces (or too few opening)
		// Remove closing braces from the end
		result = removeTrailingBraces(result, -diff)
		logger.Info("fixRemainingBraceImbalance: removed closing braces",
			logger.Int("count", -diff))
	}

	return result
}

// RestoreBracesResult contains detailed information about brace restoration
type RestoreBracesResult struct {
	Content           string   // The restored content
	OriginalBalance   int      // Brace balance in original (open - close)
	TranslatedBalance int      // Brace balance in translated before fix
	RestoredBalance   int      // Brace balance after restoration
	FixesApplied      []string // List of fixes that were applied
	WasBalanced       bool     // Whether braces were already balanced
	NowBalanced       bool     // Whether braces are balanced after restoration
}

// RestoreBracesWithDetails performs brace restoration and returns detailed information.
func RestoreBracesWithDetails(translated, original string) *RestoreBracesResult {
	result := &RestoreBracesResult{
		FixesApplied: []string{},
	}

	if translated == "" {
		result.Content = translated
		result.WasBalanced = true
		result.NowBalanced = true
		return result
	}

	// Calculate original balance
	origOpen, origClose := countBraces(original)
	result.OriginalBalance = origOpen - origClose

	// Calculate translated balance before fix
	transOpen, transClose := countBraces(translated)
	result.TranslatedBalance = transOpen - transClose

	// Check if already balanced
	validation := ValidateBraces(translated)
	result.WasBalanced = validation.IsBalanced

	content := translated

	// Step 1: Fix multirow/multicolumn
	beforeMulti := content
	content = fixMultirowMulticolumnBraces(content, original)
	if content != beforeMulti {
		result.FixesApplied = append(result.FixesApplied, "multirow/multicolumn brace fixes")
	}

	// Step 2: Fix general brace balance
	beforeGeneral := content
	content = fixGeneralBraceBalance(content, original)
	if content != beforeGeneral {
		result.FixesApplied = append(result.FixesApplied, "general brace balance fixes")
	}

	// Step 3: Context-aware restoration
	beforeContext := content
	content = restoreBracesContextAware(content, original)
	if content != beforeContext {
		result.FixesApplied = append(result.FixesApplied, "context-aware brace restoration")
	}

	// Step 4: Final imbalance fix
	beforeFinal := content
	content = fixRemainingBraceImbalance(content, original)
	if content != beforeFinal {
		result.FixesApplied = append(result.FixesApplied, "final brace imbalance fix")
	}

	result.Content = content

	// Calculate final balance
	finalOpen, finalClose := countBraces(content)
	result.RestoredBalance = finalOpen - finalClose

	// Check if now balanced
	finalValidation := ValidateBraces(content)
	result.NowBalanced = finalValidation.IsBalanced

	if len(result.FixesApplied) > 0 {
		logger.Info("RestoreBracesWithDetails completed",
			logger.Int("originalBalance", result.OriginalBalance),
			logger.Int("translatedBalance", result.TranslatedBalance),
			logger.Int("restoredBalance", result.RestoredBalance),
			logger.Bool("wasBalanced", result.WasBalanced),
			logger.Bool("nowBalanced", result.NowBalanced),
			logger.Int("fixCount", len(result.FixesApplied)))
	}

	return result
}

// =============================================================================
// Comment Restoration Functions
// =============================================================================
// These functions implement Requirements 5.2, 5.3, 5.5 for comment format preservation.

// RestoreComments restores comment formatting in translated content by comparing with original.
// This function implements Requirements 5.2, 5.3, and 5.5:
// - 5.2: Preserve the % symbol when translating comment content
// - 5.3: Restore % symbol if LLM removes it from a comment line
// - 5.5: Re-comment environment tags that were commented in original but uncommented in translated
//
// The function performs the following steps:
// 1. Compare original and translated content line by line
// 2. Identify lines that were comments in original but not in translated
// 3. Restore the % symbol to lines that should be comments
// 4. Re-comment environment tags (\begin{...}, \end{...}) that were commented in original
//
// Parameters:
//   - translated: the translated content that may have lost comment markers
//   - original: the original content used as reference for comment structure
//
// Returns the fixed content with restored comment markers.
func RestoreComments(translated, original string) string {
	if translated == "" || original == "" {
		return translated
	}

	result := translated

	// Step 1: Restore removed % symbols on comment lines
	result = restoreRemovedCommentSymbols(result, original)

	// Step 2: Re-comment environment tags that were commented in original
	result = restoreCommentedEnvTags(result, original)

	// Step 3: Ensure comment consistency (if \begin is commented, \end should be too)
	result = ensureCommentConsistency(result, original)

	if result != translated {
		logger.Info("RestoreComments made changes",
			logger.Int("originalLen", len(translated)),
			logger.Int("fixedLen", len(result)))
	}

	return result
}

// restoreRemovedCommentSymbols restores % symbols that were removed by LLM.
// This implements Requirement 5.3: IF the LLM removes the % symbol from a comment line,
// THEN THE Format_Restorer SHALL restore it.
func restoreRemovedCommentSymbols(translated, original string) string {
	origLines := strings.Split(original, "\n")
	transLines := strings.Split(translated, "\n")

	// If line counts differ significantly, we can't do reliable line-by-line comparison
	if abs(len(origLines)-len(transLines)) > len(origLines)/5 {
		return translated
	}

	fixedLines := make([]string, len(transLines))
	copy(fixedLines, transLines)
	fixCount := 0

	minLines := len(origLines)
	if len(transLines) < minLines {
		minLines = len(transLines)
	}

	for i := 0; i < minLines; i++ {
		origLine := origLines[i]
		transLine := transLines[i]

		origTrimmed := strings.TrimLeft(origLine, " \t")
		transTrimmed := strings.TrimLeft(transLine, " \t")

		origIsComment := strings.HasPrefix(origTrimmed, "%")
		transIsComment := strings.HasPrefix(transTrimmed, "%")

		if origIsComment && !transIsComment {
			indent := getLeadingWhitespace(origLine)
			fixedLines[i] = indent + "% " + strings.TrimLeft(transLine, " \t")
			fixCount++
		}
	}

	if fixCount > 0 {
		logger.Info("restored removed comment symbols", logger.Int("count", fixCount))
	}

	return strings.Join(fixedLines, "\n")
}

// restoreCommentedEnvTags re-comments environment tags that were commented in original.
// This implements Requirement 5.5.
func restoreCommentedEnvTags(translated, original string) string {
	envTagRegex := regexp.MustCompile(`\\(begin|end)\{([^}]+)\}`)
	origLines := strings.Split(original, "\n")
	
	// Find commented env tags in original
	commentedTags := make(map[string]bool)
	for _, line := range origLines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "%") {
			matches := envTagRegex.FindAllString(line, -1)
			for _, tag := range matches {
				commentedTags[tag] = true
			}
		}
	}
	
	if len(commentedTags) == 0 {
		return translated
	}

	result := translated
	transLines := strings.Split(result, "\n")
	fixCount := 0

	for i, line := range transLines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "%") {
			continue // Already commented
		}
		
		for tag := range commentedTags {
			if strings.Contains(line, tag) {
				indent := getLeadingWhitespace(line)
				transLines[i] = indent + "% " + strings.TrimLeft(line, " \t")
				fixCount++
				break
			}
		}
	}

	if fixCount > 0 {
		logger.Info("re-commented environment tags", logger.Int("count", fixCount))
		return strings.Join(transLines, "\n")
	}

	return translated
}

// ensureCommentConsistency ensures that if \begin{env} is commented, \end{env} is also commented.
func ensureCommentConsistency(translated, original string) string {
	envPattern := regexp.MustCompile(`\\(?:begin|end)\{([^}]+)\}`)
	envNames := make(map[string]bool)
	
	for _, match := range envPattern.FindAllStringSubmatch(original, -1) {
		envNames[match[1]] = true
	}

	result := translated

	for envName := range envNames {
		beginTag := `\begin{` + envName + `}`
		endTag := `\end{` + envName + `}`

		if !strings.Contains(result, beginTag) || !strings.Contains(result, endTag) {
			continue
		}

		beginCommented := isTagCommented(result, beginTag)
		endCommented := isTagCommented(result, endTag)

		if beginCommented && !endCommented {
			result = commentTagInContent(result, endTag)
		}
		if endCommented && !beginCommented {
			result = commentTagInContent(result, beginTag)
		}
	}

	return result
}

// commentTagInContent adds a comment marker to the first uncommented occurrence of a tag.
func commentTagInContent(content, tag string) string {
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if !strings.Contains(line, tag) {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "%") {
			continue
		}
		indent := getLeadingWhitespace(line)
		lines[i] = indent + "% " + strings.TrimLeft(line, " \t")
		break
	}

	return strings.Join(lines, "\n")
}

