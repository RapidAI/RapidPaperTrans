// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"regexp"
	"strings"
)

// StructureFixer compares translated LaTeX with original to fix structural issues.
type StructureFixer struct {
	original   string
	translated string
}

// NewStructureFixer creates a new StructureFixer.
func NewStructureFixer(original, translated string) *StructureFixer {
	return &StructureFixer{
		original:   original,
		translated: translated,
	}
}

// StructureInfo holds structural information about a LaTeX document.
type StructureInfo struct {
	Environments []EnvInfo   // All environments (begin/end pairs)
	Commands     []CmdInfo   // Commands with arguments
	BraceCount   int         // Total brace balance
	Tables       []TableInfo // Table structures
}

// EnvInfo holds information about a LaTeX environment.
type EnvInfo struct {
	Name       string
	StartLine  int
	EndLine    int
	HasEnd     bool
	Content    string
	Parameters string // e.g., [h] for \begin{table}[h]
}

// CmdInfo holds information about a LaTeX command.
type CmdInfo struct {
	Name      string
	Line      int
	Arguments []string
	BraceCount int // Number of opening braces in this command
}

// TableInfo holds information about a table structure.
type TableInfo struct {
	Line        int
	HasResizebox bool
	ResizeboxClosed bool
	TabularClosed bool
}

// FixResult contains the result of structure fixing.
type StructureFixResult struct {
	Fixed       bool
	Content     string
	Fixes       []string // Description of fixes applied
	Errors      []string // Errors that couldn't be fixed
}

// Fix compares structures and fixes issues in translated content.
func (sf *StructureFixer) Fix() *StructureFixResult {
	result := &StructureFixResult{
		Content: sf.translated,
		Fixes:   []string{},
		Errors:  []string{},
	}

	// Step 1: Extract structure from original
	origStructure := sf.extractStructure(sf.original)
	
	// Step 2: Extract structure from translated
	transStructure := sf.extractStructure(sf.translated)
	
	// Step 3: Compare and fix environments
	result.Content = sf.fixEnvironments(result.Content, origStructure, transStructure, result)
	
	// Step 4: Fix brace balance issues
	result.Content = sf.fixBraceBalance(result.Content, origStructure, transStructure, result)
	
	// Step 5: Fix table structures (resizebox closing braces)
	result.Content = sf.fixTableStructures(result.Content, origStructure, transStructure, result)
	
	// Step 6: Fix caption brace issues
	result.Content = sf.fixCaptionBraces(result.Content, sf.original, result)
	
	result.Fixed = len(result.Fixes) > 0
	return result
}

// extractStructure extracts structural information from LaTeX content.
func (sf *StructureFixer) extractStructure(content string) *StructureInfo {
	info := &StructureInfo{
		Environments: []EnvInfo{},
		Commands:     []CmdInfo{},
		Tables:       []TableInfo{},
	}
	
	lines := strings.Split(content, "\n")
	
	// Track environments
	envStack := []EnvInfo{}
	beginEnvRe := regexp.MustCompile(`\\begin\{([^}]+)\}(\[[^\]]*\])?`)
	endEnvRe := regexp.MustCompile(`\\end\{([^}]+)\}`)
	
	// Track resizebox
	resizeboxRe := regexp.MustCompile(`\\resizebox\{[^}]*\}\{[^}]*\}\{`)
	
	for i, line := range lines {
		lineNum := i + 1
		
		// Find begin environments
		beginMatches := beginEnvRe.FindAllStringSubmatch(line, -1)
		for _, match := range beginMatches {
			env := EnvInfo{
				Name:      match[1],
				StartLine: lineNum,
				HasEnd:    false,
			}
			if len(match) > 2 {
				env.Parameters = match[2]
			}
			envStack = append(envStack, env)
		}
		
		// Find end environments
		endMatches := endEnvRe.FindAllStringSubmatch(line, -1)
		for _, match := range endMatches {
			envName := match[1]
			// Find matching begin in stack
			for j := len(envStack) - 1; j >= 0; j-- {
				if envStack[j].Name == envName && !envStack[j].HasEnd {
					envStack[j].HasEnd = true
					envStack[j].EndLine = lineNum
					info.Environments = append(info.Environments, envStack[j])
					break
				}
			}
		}
		
		// Track tables with resizebox
		if strings.Contains(line, "\\begin{table") {
			tableInfo := TableInfo{
				Line:        lineNum,
				HasResizebox: false,
			}
			// Check if this table has resizebox
			for j := i; j < len(lines) && j < i+10; j++ {
				if resizeboxRe.MatchString(lines[j]) {
					tableInfo.HasResizebox = true
					break
				}
				if strings.Contains(lines[j], "\\end{table") {
					break
				}
			}
			info.Tables = append(info.Tables, tableInfo)
		}
		
		// Count braces
		info.BraceCount += strings.Count(line, "{") - strings.Count(line, "}")
	}
	
	// Add unclosed environments
	for _, env := range envStack {
		if !env.HasEnd {
			info.Environments = append(info.Environments, env)
		}
	}
	
	return info
}

// fixEnvironments fixes environment mismatches.
func (sf *StructureFixer) fixEnvironments(content string, orig, trans *StructureInfo, result *StructureFixResult) string {
	// Find environments in original that are properly closed
	origEnvs := make(map[string]int) // env name -> count
	for _, env := range orig.Environments {
		if env.HasEnd {
			origEnvs[env.Name]++
		}
	}
	
	// Find environments in translated
	transEnvs := make(map[string]int)
	transUnclosed := []EnvInfo{}
	for _, env := range trans.Environments {
		if env.HasEnd {
			transEnvs[env.Name]++
		} else {
			transUnclosed = append(transUnclosed, env)
		}
	}
	
	// Fix unclosed environments that should be closed
	for _, env := range transUnclosed {
		if origEnvs[env.Name] > 0 {
			// This environment should be closed - add \end{envname}
			endTag := "\\end{" + env.Name + "}"
			if !strings.Contains(content, endTag) || transEnvs[env.Name] < origEnvs[env.Name] {
				// Find a good place to insert the end tag
				content = sf.insertEndEnvironment(content, env)
				result.Fixes = append(result.Fixes, "Added missing "+endTag)
			}
		}
	}
	
	return content
}

// insertEndEnvironment inserts an end environment tag at an appropriate location.
func (sf *StructureFixer) insertEndEnvironment(content string, env EnvInfo) string {
	lines := strings.Split(content, "\n")
	endTag := "\\end{" + env.Name + "}"
	
	// Strategy: Find the next environment start or end of file
	insertLine := len(lines) - 1
	
	for i := env.StartLine; i < len(lines); i++ {
		line := lines[i]
		// If we find another \begin or \end{document}, insert before it
		if strings.Contains(line, "\\begin{") && i > env.StartLine {
			insertLine = i - 1
			break
		}
		if strings.Contains(line, "\\end{document}") {
			insertLine = i - 1
			break
		}
	}
	
	// Insert the end tag
	if insertLine >= 0 && insertLine < len(lines) {
		lines[insertLine] = lines[insertLine] + "\n" + endTag
	}
	
	return strings.Join(lines, "\n")
}

// fixBraceBalance fixes brace balance issues by comparing with original.
func (sf *StructureFixer) fixBraceBalance(content string, orig, trans *StructureInfo, result *StructureFixResult) string {
	// If original has balanced braces but translated doesn't
	if orig.BraceCount == 0 && trans.BraceCount != 0 {
		if trans.BraceCount > 0 {
			// Too many opening braces - remove trailing ones
			content = sf.removeTrailingBraces(content, trans.BraceCount)
			result.Fixes = append(result.Fixes, "Removed extra closing braces")
		} else {
			// Too many closing braces - this is handled by other rules
		}
	}
	
	return content
}

// removeTrailingBraces removes extra closing braces from the end of content.
func (sf *StructureFixer) removeTrailingBraces(content string, count int) string {
	// Pattern: multiple } at end of file
	trailingBraceRe := regexp.MustCompile(`\}+\s*$`)
	
	content = trailingBraceRe.ReplaceAllStringFunc(content, func(match string) string {
		braceCount := strings.Count(match, "}")
		if braceCount > count {
			return strings.Repeat("}", braceCount-count) + strings.TrimLeft(match, "}")
		}
		return ""
	})
	
	return content
}

// fixTableStructures fixes table structure issues like missing resizebox closing braces.
func (sf *StructureFixer) fixTableStructures(content string, orig, trans *StructureInfo, result *StructureFixResult) string {
	lines := strings.Split(content, "\n")
	
	// Find all resizebox patterns and ensure they're properly closed
	resizeboxRe := regexp.MustCompile(`\\resizebox\{[^}]*\}\{[^}]*\}\{`)
	endTabularRe := regexp.MustCompile(`\\end\{tabular\}`)
	
	inResizebox := false
	_ = inResizebox // May be used for future enhancements
	
	for i, line := range lines {
		if resizeboxRe.MatchString(line) {
			inResizebox = true
		}
		
		if inResizebox && endTabularRe.MatchString(line) {
			// Check if this line has the closing brace for resizebox
			// Pattern: \end{tabular}} (with closing brace) vs \end{tabular} (without)
			if !strings.Contains(line, "\\end{tabular}}") {
				// Need to add closing brace
				lines[i] = strings.Replace(line, "\\end{tabular}", "\\end{tabular}}", 1)
				result.Fixes = append(result.Fixes, "Added missing } for resizebox at line "+string(rune(i+1)))
			}
			inResizebox = false
		}
	}
	
	return strings.Join(lines, "\n")
}

// fixCaptionBraces fixes brace issues in captions by comparing with original.
func (sf *StructureFixer) fixCaptionBraces(content, original string, result *StructureFixResult) string {
	// Extract captions from original
	origCaptions := sf.extractCaptions(original)
	transCaptions := sf.extractCaptions(content)
	
	// Compare brace counts in captions
	for i, transCap := range transCaptions {
		if i < len(origCaptions) {
			origBraces := sf.countBraces(origCaptions[i])
			transBraces := sf.countBraces(transCap)
			
			if origBraces != transBraces {
				// Fix the caption
				fixedCaption := sf.fixCaptionBraceBalance(transCap, origCaptions[i])
				if fixedCaption != transCap {
					content = strings.Replace(content, transCap, fixedCaption, 1)
					result.Fixes = append(result.Fixes, "Fixed brace balance in caption")
				}
			}
		}
	}
	
	return content
}

// extractCaptions extracts all \caption{...} from content.
func (sf *StructureFixer) extractCaptions(content string) []string {
	captions := []string{}
	captionRe := regexp.MustCompile(`\\caption\{`)
	
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if captionRe.MatchString(line) {
			// Extract the full caption including nested braces
			caption := sf.extractBracedContent(line, "\\caption")
			if caption != "" {
				captions = append(captions, caption)
			}
		}
	}
	
	return captions
}

// extractBracedContent extracts content within braces after a command.
func (sf *StructureFixer) extractBracedContent(line, cmd string) string {
	idx := strings.Index(line, cmd)
	if idx == -1 {
		return ""
	}
	
	start := idx + len(cmd)
	// Find opening brace
	for start < len(line) && line[start] != '{' {
		start++
	}
	if start >= len(line) {
		return ""
	}
	
	// Count braces to find matching close
	braceCount := 0
	end := start
	for end < len(line) {
		if line[end] == '{' {
			braceCount++
		} else if line[end] == '}' {
			braceCount--
			if braceCount == 0 {
				return line[idx : end+1]
			}
		}
		end++
	}
	
	// Unclosed - return what we have
	return line[idx:]
}

// countBraces counts the brace balance in a string.
func (sf *StructureFixer) countBraces(s string) int {
	return strings.Count(s, "{") - strings.Count(s, "}")
}

// fixCaptionBraceBalance fixes brace balance in a caption based on original.
func (sf *StructureFixer) fixCaptionBraceBalance(translated, original string) string {
	// Common issue: \textbf{\textit{n}=64 instead of \textbf{\textit{n}}=64
	// Strategy: Find patterns like \textbf{\textit{X}= and fix to \textbf{\textit{X}}=
	
	// Pattern: \textbf{\textit{...}= (missing closing brace)
	pattern := regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+\})([^}])`)
	fixed := pattern.ReplaceAllString(translated, "${1}}${2}")
	
	// Also fix: \textbf{\textit{n}= to \textbf{\textit{n}}=
	pattern2 := regexp.MustCompile(`(\\textbf\{\\textit\{[^}]+)\}([=,;:])`)
	fixed = pattern2.ReplaceAllString(fixed, "${1}}}${2}")
	
	return fixed
}
