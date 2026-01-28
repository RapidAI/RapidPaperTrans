// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"regexp"
	"strings"
)

// ReferenceFixer uses original LaTeX as reference to fix translation errors.
type ReferenceFixer struct {
	originalFiles   map[string]string // filename -> content
	translatedFiles map[string]string // filename -> content
}

// NewReferenceFixer creates a new ReferenceFixer.
func NewReferenceFixer() *ReferenceFixer {
	return &ReferenceFixer{
		originalFiles:   make(map[string]string),
		translatedFiles: make(map[string]string),
	}
}

// SetOriginal sets the original file content.
func (rf *ReferenceFixer) SetOriginal(filename, content string) {
	rf.originalFiles[filename] = content
}

// SetTranslated sets the translated file content.
func (rf *ReferenceFixer) SetTranslated(filename, content string) {
	rf.translatedFiles[filename] = content
}

// ReferenceFixResult contains the result of reference-based fixing.
type ReferenceFixResult struct {
	Fixed       bool
	FixedFiles  map[string]string
	Fixes       []FixDescription
	Errors      []string
}

// FixDescription describes a single fix.
type FixDescription struct {
	File        string
	Line        int
	Original    string
	Translated  string
	Fixed       string
	Description string
}

// Fix performs reference-based fixing on all translated files.
func (rf *ReferenceFixer) Fix() *ReferenceFixResult {
	result := &ReferenceFixResult{
		FixedFiles: make(map[string]string),
		Fixes:      []FixDescription{},
		Errors:     []string{},
	}

	for filename, translated := range rf.translatedFiles {
		original, hasOriginal := rf.originalFiles[filename]
		if !hasOriginal {
			result.FixedFiles[filename] = translated
			continue
		}

		fixed, fixes := rf.fixFile(filename, original, translated)
		result.FixedFiles[filename] = fixed
		result.Fixes = append(result.Fixes, fixes...)
	}

	result.Fixed = len(result.Fixes) > 0
	return result
}

// fixFile fixes a single file by comparing with original.
func (rf *ReferenceFixer) fixFile(filename, original, translated string) (string, []FixDescription) {
	fixes := []FixDescription{}
	content := translated

	// 1. Fix environment structure
	content, envFixes := rf.fixEnvironmentStructure(filename, original, content)
	fixes = append(fixes, envFixes...)

	// 2. Fix table structures (resizebox, tabular)
	content, tableFixes := rf.fixTableStructure(filename, original, content)
	fixes = append(fixes, tableFixes...)

	// 3. Fix caption brace issues
	content, captionFixes := rf.fixCaptionIssues(filename, original, content)
	fixes = append(fixes, captionFixes...)

	// 4. Fix trailing garbage
	content, trailingFixes := rf.fixTrailingGarbage(filename, original, content)
	fixes = append(fixes, trailingFixes...)

	// 5. Fix command argument braces
	content, cmdFixes := rf.fixCommandBraces(filename, original, content)
	fixes = append(fixes, cmdFixes...)

	return content, fixes
}

// fixEnvironmentStructure ensures environments match between original and translated.
func (rf *ReferenceFixer) fixEnvironmentStructure(filename, original, translated string) (string, []FixDescription) {
	fixes := []FixDescription{}
	
	// Extract environments from both
	origEnvs := rf.extractEnvironments(original)
	transEnvs := rf.extractEnvironments(translated)
	
	// Check for missing \end tags
	for envName, origCount := range origEnvs {
		transCount := transEnvs[envName]
		if transCount.ends < origCount.ends {
			// Missing end tags
			missing := origCount.ends - transCount.ends
			for i := 0; i < missing; i++ {
				// Find where to insert
				translated = rf.insertMissingEnd(translated, envName)
				fixes = append(fixes, FixDescription{
					File:        filename,
					Description: "Added missing \\end{" + envName + "}",
				})
			}
		}
	}
	
	return translated, fixes
}

type envCount struct {
	begins int
	ends   int
}

// extractEnvironments counts begin/end for each environment type.
func (rf *ReferenceFixer) extractEnvironments(content string) map[string]envCount {
	result := make(map[string]envCount)
	
	beginRe := regexp.MustCompile(`\\begin\{([^}]+)\}`)
	endRe := regexp.MustCompile(`\\end\{([^}]+)\}`)
	
	for _, match := range beginRe.FindAllStringSubmatch(content, -1) {
		env := result[match[1]]
		env.begins++
		result[match[1]] = env
	}
	
	for _, match := range endRe.FindAllStringSubmatch(content, -1) {
		env := result[match[1]]
		env.ends++
		result[match[1]] = env
	}
	
	return result
}

// insertMissingEnd inserts a missing \end tag at an appropriate location.
func (rf *ReferenceFixer) insertMissingEnd(content, envName string) string {
	// Find the last \begin{envName} without matching \end
	lines := strings.Split(content, "\n")
	beginRe := regexp.MustCompile(`\\begin\{` + regexp.QuoteMeta(envName) + `\}`)
	endRe := regexp.MustCompile(`\\end\{` + regexp.QuoteMeta(envName) + `\}`)
	
	beginCount := 0
	endCount := 0
	lastUnmatchedBegin := -1
	
	for i, line := range lines {
		if beginRe.MatchString(line) {
			beginCount++
			lastUnmatchedBegin = i
		}
		if endRe.MatchString(line) {
			endCount++
		}
	}
	
	if beginCount > endCount && lastUnmatchedBegin >= 0 {
		// Find appropriate place to insert end
		insertAt := len(lines) - 1
		for i := lastUnmatchedBegin + 1; i < len(lines); i++ {
			if strings.Contains(lines[i], "\\end{document}") {
				insertAt = i
				break
			}
			if strings.Contains(lines[i], "\\begin{") && i > lastUnmatchedBegin+1 {
				insertAt = i
				break
			}
		}
		
		endTag := "\\end{" + envName + "}"
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:insertAt]...)
		newLines = append(newLines, endTag)
		newLines = append(newLines, lines[insertAt:]...)
		return strings.Join(newLines, "\n")
	}
	
	return content
}

// fixTableStructure fixes table-related issues like resizebox closing braces.
func (rf *ReferenceFixer) fixTableStructure(filename, original, translated string) (string, []FixDescription) {
	fixes := []FixDescription{}
	
	// Find all tables in original and check their structure
	origTables := rf.extractTableStructures(original)
	transTables := rf.extractTableStructures(translated)
	
	lines := strings.Split(translated, "\n")
	modified := false
	
	for i, transTable := range transTables {
		if i < len(origTables) {
			origTable := origTables[i]
			
			// Check if original has resizebox but translated is missing closing brace
			if origTable.hasResizebox && transTable.hasResizebox {
				if origTable.resizeboxClosed && !transTable.resizeboxClosed {
					// Find the \end{tabular} line and add }
					for j, line := range lines {
						if strings.Contains(line, "\\end{tabular}") && !strings.Contains(line, "\\end{tabular}}") {
							// Check if this is the right table (approximate by line number)
							if j >= transTable.startLine-1 && j <= transTable.startLine+50 {
								lines[j] = strings.Replace(line, "\\end{tabular}", "\\end{tabular}}", 1)
								modified = true
								fixes = append(fixes, FixDescription{
									File:        filename,
									Line:        j + 1,
									Description: "Added missing } for resizebox",
								})
								break
							}
						}
					}
				}
			}
		}
	}
	
	if modified {
		translated = strings.Join(lines, "\n")
	}
	
	return translated, fixes
}

type tableStructure struct {
	startLine       int
	hasResizebox    bool
	resizeboxClosed bool
}

// extractTableStructures extracts table structure information.
func (rf *ReferenceFixer) extractTableStructures(content string) []tableStructure {
	tables := []tableStructure{}
	lines := strings.Split(content, "\n")
	
	inTable := false
	currentTable := tableStructure{}
	braceCount := 0
	
	for i, line := range lines {
		if strings.Contains(line, "\\begin{table") {
			inTable = true
			currentTable = tableStructure{startLine: i + 1}
			braceCount = 0
		}
		
		if inTable {
			if strings.Contains(line, "\\resizebox") {
				currentTable.hasResizebox = true
				// Count opening braces from resizebox
				braceCount += strings.Count(line, "{") - strings.Count(line, "}")
			} else {
				braceCount += strings.Count(line, "{") - strings.Count(line, "}")
			}
			
			if strings.Contains(line, "\\end{tabular}") {
				// Check if resizebox is properly closed
				if currentTable.hasResizebox {
					// After \end{tabular}, braceCount should be back to table level
					if strings.Contains(line, "\\end{tabular}}") {
						currentTable.resizeboxClosed = true
					}
				}
			}
			
			if strings.Contains(line, "\\end{table") {
				tables = append(tables, currentTable)
				inTable = false
			}
		}
	}
	
	return tables
}

// fixCaptionIssues fixes brace issues in captions.
func (rf *ReferenceFixer) fixCaptionIssues(filename, original, translated string) (string, []FixDescription) {
	fixes := []FixDescription{}
	
	// Extract captions from both
	origCaptions := rf.extractCaptionLines(original)
	transCaptions := rf.extractCaptionLines(translated)
	
	lines := strings.Split(translated, "\n")
	
	for i, transCap := range transCaptions {
		if i < len(origCaptions) {
			origCap := origCaptions[i]
			
			// Compare brace structure
			origBraces := rf.analyzeBraceStructure(origCap.content)
			transBraces := rf.analyzeBraceStructure(transCap.content)
			
			if origBraces.balance == 0 && transBraces.balance != 0 {
				// Fix the caption
				fixedLine := rf.fixCaptionLine(transCap.content, origCap.content)
				if fixedLine != transCap.content {
					lines[transCap.lineNum-1] = fixedLine
					fixes = append(fixes, FixDescription{
						File:        filename,
						Line:        transCap.lineNum,
						Original:    origCap.content,
						Translated:  transCap.content,
						Fixed:       fixedLine,
						Description: "Fixed brace balance in caption",
					})
				}
			}
		}
	}
	
	return strings.Join(lines, "\n"), fixes
}

type captionLine struct {
	lineNum int
	content string
}

// extractCaptionLines extracts lines containing \caption.
func (rf *ReferenceFixer) extractCaptionLines(content string) []captionLine {
	captions := []captionLine{}
	lines := strings.Split(content, "\n")
	
	for i, line := range lines {
		if strings.Contains(line, "\\caption{") {
			captions = append(captions, captionLine{
				lineNum: i + 1,
				content: line,
			})
		}
	}
	
	return captions
}

type braceAnalysis struct {
	balance int
	opens   int
	closes  int
}

// analyzeBraceStructure analyzes brace structure in a line.
func (rf *ReferenceFixer) analyzeBraceStructure(line string) braceAnalysis {
	opens := strings.Count(line, "{")
	closes := strings.Count(line, "}")
	return braceAnalysis{
		balance: opens - closes,
		opens:   opens,
		closes:  closes,
	}
}

// fixCaptionLine fixes brace issues in a caption line.
func (rf *ReferenceFixer) fixCaptionLine(translated, original string) string {
	// Common pattern: \textbf{\textit{n}=64 should be \textbf{\textit{n}}=64
	// Find patterns where a closing brace is missing before = or other chars
	
	patterns := []struct {
		re   *regexp.Regexp
		repl string
	}{
		// \textbf{\textit{X}= -> \textbf{\textit{X}}=
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+)\}([=,;:，；：])`), "${1}}}${2}"},
		// \textbf{\textit{X}） -> \textbf{\textit{X}}）
		{regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+)\}([）\)])`), "${1}}}${2}"},
	}
	
	result := translated
	for _, p := range patterns {
		result = p.re.ReplaceAllString(result, p.repl)
	}
	
	return result
}

// fixTrailingGarbage removes trailing garbage that doesn't exist in original.
func (rf *ReferenceFixer) fixTrailingGarbage(filename, original, translated string) (string, []FixDescription) {
	fixes := []FixDescription{}
	
	// Check for trailing braces
	origTrailing := rf.getTrailingChars(original, 20)
	transTrailing := rf.getTrailingChars(translated, 20)
	
	// If original doesn't end with multiple braces but translated does
	origBraceCount := strings.Count(origTrailing, "}")
	transBraceCount := strings.Count(transTrailing, "}")
	
	if transBraceCount > origBraceCount+2 {
		// Remove extra trailing braces
		extra := transBraceCount - origBraceCount
		translated = rf.removeTrailingBracesN(translated, extra)
		fixes = append(fixes, FixDescription{
			File:        filename,
			Description: "Removed " + string(rune(extra+'0')) + " extra trailing braces",
		})
	}
	
	return translated, fixes
}

// getTrailingChars gets the last n characters of content.
func (rf *ReferenceFixer) getTrailingChars(content string, n int) string {
	content = strings.TrimSpace(content)
	if len(content) <= n {
		return content
	}
	return content[len(content)-n:]
}

// removeTrailingBracesN removes n trailing braces.
func (rf *ReferenceFixer) removeTrailingBracesN(content string, n int) string {
	content = strings.TrimSpace(content)
	removed := 0
	
	for removed < n && len(content) > 0 {
		if content[len(content)-1] == '}' {
			content = content[:len(content)-1]
			removed++
		} else if content[len(content)-1] == '\n' || content[len(content)-1] == ' ' {
			content = content[:len(content)-1]
		} else {
			break
		}
	}
	
	return content + "\n"
}

// fixCommandBraces fixes brace issues in commands by comparing with original.
func (rf *ReferenceFixer) fixCommandBraces(filename, original, translated string) (string, []FixDescription) {
	fixes := []FixDescription{}
	
	// Find commands with potential brace issues
	// Pattern: commands like \textbf, \textit, \emph that might have unbalanced braces
	
	commands := []string{"textbf", "textit", "emph", "underline", "section", "subsection"}
	
	for _, cmd := range commands {
		origCount := rf.countCommandBraces(original, cmd)
		transCount := rf.countCommandBraces(translated, cmd)
		
		if origCount.balance == 0 && transCount.balance != 0 {
			// Try to fix
			translated = rf.fixCommandBraceBalance(translated, cmd)
			fixes = append(fixes, FixDescription{
				File:        filename,
				Description: "Fixed brace balance in \\" + cmd + " commands",
			})
		}
	}
	
	return translated, fixes
}

// countCommandBraces counts brace balance for a specific command.
func (rf *ReferenceFixer) countCommandBraces(content, cmd string) braceAnalysis {
	pattern := regexp.MustCompile(`\\` + cmd + `\{`)
	matches := pattern.FindAllStringIndex(content, -1)
	
	totalBalance := 0
	for _, match := range matches {
		// Count braces from this command
		balance := 0
		for i := match[0]; i < len(content); i++ {
			if content[i] == '{' {
				balance++
			} else if content[i] == '}' {
				balance--
				if balance == 0 {
					break
				}
			}
		}
		totalBalance += balance
	}
	
	return braceAnalysis{balance: totalBalance}
}

// fixCommandBraceBalance attempts to fix brace balance for a command.
func (rf *ReferenceFixer) fixCommandBraceBalance(content, cmd string) string {
	// This is a heuristic fix - add missing closing braces
	pattern := regexp.MustCompile(`(\\` + cmd + `\{[^}]*)\}([^}])`)
	
	// Try to find and fix patterns where closing brace is missing
	return pattern.ReplaceAllString(content, "${1}}}${2}")
}

// FixWithReference is a convenience function to fix translated content using original as reference.
func FixWithReference(originalFiles, translatedFiles map[string]string) *ReferenceFixResult {
	fixer := NewReferenceFixer()
	
	for name, content := range originalFiles {
		fixer.SetOriginal(name, content)
	}
	
	for name, content := range translatedFiles {
		fixer.SetTranslated(name, content)
	}
	
	return fixer.Fix()
}
