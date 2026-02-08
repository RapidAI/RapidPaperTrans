// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"regexp"
	"strings"

	"latex-translator/internal/logger"
)

// fixResizeboxClosingBrace fixes missing closing brace for \resizebox in tables.
// This is a common issue in arXiv source files where the closing brace is missing.
// Pattern: \resizebox{\columnwidth}{!}{ ... \end{tabular} should have } after \end{tabular}
func fixResizeboxClosingBrace(content string) (string, bool) {
	if !strings.Contains(content, "\\resizebox") {
		return content, false
	}

	lines := strings.Split(content, "\n")
	fixed := false
	inResizebox := false
	braceDepth := 0

	for i, line := range lines {
		if strings.Contains(line, "\\resizebox") {
			inResizebox = true
			// Count braces on this line
			braceDepth = strings.Count(line, "{") - strings.Count(line, "}")
		} else if inResizebox {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		}

		// Check if we're at \end{tabular} inside a resizebox
		if inResizebox && strings.Contains(line, "\\end{tabular}") {
			// If the line doesn't already have the closing brace for resizebox
			if !strings.Contains(line, "\\end{tabular}}") && braceDepth > 0 {
				// Add the missing closing brace
				lines[i] = strings.Replace(line, "\\end{tabular}", "\\end{tabular}}", 1)
				fixed = true
				braceDepth--
				logger.Debug("rule-based fix: added missing } for resizebox after \\end{tabular}",
					logger.Int("line", i+1))
			}
			inResizebox = false
		}

		// Reset if we hit \end{table}
		if strings.Contains(line, "\\end{table") {
			inResizebox = false
			braceDepth = 0
		}
	}

	if fixed {
		return strings.Join(lines, "\n"), true
	}
	return content, false
}

// fixOrphanedListEnvironments fixes orphaned \begin{itemize/enumerate/description}
// that are followed by commented-out content including the \end tag.
// This is a common issue in arXiv source files where authors comment out list content
// but forget to comment out the \begin tag.
// Pattern: \begin{itemize} followed by % comments including % \end{itemize}
func fixOrphanedListEnvironments(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	fixed := false

	listEnvs := []string{"itemize", "enumerate", "description"}

	for _, env := range listEnvs {
		beginTag := "\\begin{" + env + "}"
		endTag := "\\end{" + env + "}"
		commentedEndTag := "% " + endTag
		commentedEndTag2 := "%\t" + endTag
		commentedEndTag3 := "%" + endTag

		for i := 0; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])

			// Check if this line has \begin{env}
			if strings.Contains(line, beginTag) {
				// Look ahead to see if the content is commented out
				// and if there's a commented \end{env}
				allCommented := true
				commentedEndLine := -1

				for j := i + 1; j < len(lines) && j < i+20; j++ { // Look at next 20 lines max
					nextLine := strings.TrimSpace(lines[j])

					// Skip empty lines
					if nextLine == "" {
						continue
					}

					// Check for commented end tag
					if strings.Contains(nextLine, commentedEndTag) ||
						strings.Contains(nextLine, commentedEndTag2) ||
						strings.Contains(nextLine, commentedEndTag3) {
						commentedEndLine = j
						break
					}

					// Check for uncommented end tag (normal case, no fix needed)
					if strings.Contains(nextLine, endTag) && !strings.HasPrefix(nextLine, "%") {
						allCommented = false
						break
					}

					// Check if line is a comment
					if !strings.HasPrefix(nextLine, "%") {
						// Found non-comment content before end tag
						allCommented = false
						break
					}
				}

				// If we found a pattern where \begin is followed by comments
				// and there's a commented \end, comment out the \begin too
				if allCommented && commentedEndLine > i {
					// Comment out the \begin line
					lines[i] = "% " + lines[i]
					fixed = true
					logger.Debug("rule-based fix: commented out orphaned \\begin",
						logger.String("env", env),
						logger.Int("line", i+1))
				}
			}
		}
	}

	if fixed {
		return strings.Join(lines, "\n"), true
	}
	return content, false
}

// fixDuplicateGraphicsPath fixes duplicate image paths when \graphicspath is set.
// This is a common issue in PDF-to-LaTeX converted documents where:
// - \graphicspath{ {./images/} } is set in the preamble
// - \includegraphics{images/file.jpg} uses the full path including "images/"
// This causes LaTeX to look for ./images/images/file.jpg which doesn't exist.
//
// The fix removes the redundant path prefix from \includegraphics commands.
// For example:
// - \graphicspath{ {./images/} } + \includegraphics{images/foo.jpg}
//   -> \includegraphics{foo.jpg}
// - \graphicspath{ {./figures/} } + \includegraphics{figures/bar.png}
//   -> \includegraphics{bar.png}
func fixDuplicateGraphicsPath(content string) (string, bool) {
	// First, extract the graphics path(s) from \graphicspath command
	// Pattern: \graphicspath{ {./path1/} {./path2/} ... }
	graphicsPathPattern := regexp.MustCompile(`\\graphicspath\s*\{\s*(\{[^}]+\}\s*)+\}`)
	match := graphicsPathPattern.FindString(content)
	if match == "" {
		return content, false
	}

	// Extract individual paths from the match
	// Pattern: {./path/} or {path/} - capture the path name
	// Examples: {./images/} -> images, {figures/} -> figures, {images} -> images
	// Handle various formats: {./images/}, {images/}, {images}, {./images}
	// The key is to not match the outer braces of \graphicspath{ ... }
	pathPattern := regexp.MustCompile(`\{(\.?/?[^{}/]+/?)\}`)
	pathMatches := pathPattern.FindAllStringSubmatch(match, -1)
	if len(pathMatches) == 0 {
		return content, false
	}

	// Collect all graphics paths (normalized)
	var graphicsPaths []string
	for _, pm := range pathMatches {
		if len(pm) > 1 {
			path := pm[1]
			// Clean up the path: remove leading ./ and trailing /
			path = strings.TrimPrefix(path, "./")
			path = strings.TrimPrefix(path, "/")
			path = strings.TrimSuffix(path, "/")
			if path != "" {
				graphicsPaths = append(graphicsPaths, path)
			}
		}
	}

	if len(graphicsPaths) == 0 {
		return content, false
	}

	result := content
	fixed := false

	// For each graphics path, find and fix \includegraphics commands that duplicate the path
	for _, gpath := range graphicsPaths {
		// Pattern: \includegraphics[...]{path/file} or \includegraphics{path/file}
		// We need to handle both cases with and without options
		
		// Pattern with options: \includegraphics[...]{path/...}
		patternWithOpts := regexp.MustCompile(
			`(\\includegraphics\s*\[[^\]]*\]\s*\{)` + regexp.QuoteMeta(gpath) + `/([^}]+})`)
		if patternWithOpts.MatchString(result) {
			result = patternWithOpts.ReplaceAllString(result, "$1$2")
			fixed = true
			logger.Debug("rule-based fix: removed duplicate graphics path from \\includegraphics with options",
				logger.String("path", gpath))
		}

		// Pattern without options: \includegraphics{path/...}
		patternNoOpts := regexp.MustCompile(
			`(\\includegraphics\s*\{)` + regexp.QuoteMeta(gpath) + `/([^}]+})`)
		if patternNoOpts.MatchString(result) {
			result = patternNoOpts.ReplaceAllString(result, "$1$2")
			fixed = true
			logger.Debug("rule-based fix: removed duplicate graphics path from \\includegraphics",
				logger.String("path", gpath))
		}
	}

	return result, fixed
}


// fixDocumentEndIssues fixes various issues at the end of the document:
// 1. Extra braces before \end{document} like }}} \end{document}
// 2. Incomplete \end{document (missing closing brace)
// 3. Duplicate \end{document}
// This is a common issue in arXiv source files with corrupted endings.
func fixDocumentEndIssues(content string) (string, bool) {
	fixed := false

	// Pattern 1: Fix incomplete \end{document (missing closing brace)
	// \end{document followed by newline or end of file without }
	incompleteEndDocPattern := regexp.MustCompile(`\\end\{document([^}])`)
	if incompleteEndDocPattern.MatchString(content) {
		content = incompleteEndDocPattern.ReplaceAllString(content, `\end{document}$1`)
		fixed = true
		logger.Debug("rule-based fix: fixed incomplete \\end{document")
	}

	// Pattern 2: Remove duplicate \end{document}
	// Keep only the first valid \end{document}
	duplicateEndDocPattern := regexp.MustCompile(`(\\end\{document\})\s*(\\end\{document\})`)
	for duplicateEndDocPattern.MatchString(content) {
		content = duplicateEndDocPattern.ReplaceAllString(content, "$1")
		fixed = true
		logger.Debug("rule-based fix: removed duplicate \\end{document}")
	}

	// Pattern 3: Remove extra braces immediately before \end{document}
	// }}} \end{document} -> \end{document}
	// But be careful not to remove braces that are part of valid structures
	extraBracesBeforeEndDocPattern := regexp.MustCompile(`\n\s*(\}{2,})\s*\n*(\\end\{document\})`)
	if extraBracesBeforeEndDocPattern.MatchString(content) {
		content = extraBracesBeforeEndDocPattern.ReplaceAllString(content, "\n$2")
		fixed = true
		logger.Debug("rule-based fix: removed extra braces before \\end{document}")
	}

	// Pattern 4: Fix standalone }}} on a line before \end{document}
	// This handles cases where }}} is on its own line
	standaloneBracesPattern := regexp.MustCompile(`\n\s*\}{2,}\s*\n`)
	if standaloneBracesPattern.MatchString(content) {
		// Only fix if it's near the end of the document (within last 200 chars before \end{document})
		endDocIdx := strings.LastIndex(content, `\end{document}`)
		if endDocIdx > 0 {
			// Check if there are standalone braces in the last portion
			lastPortion := content[max(0, endDocIdx-200):endDocIdx]
			if standaloneBracesPattern.MatchString(lastPortion) {
				// Replace in the last portion only
				fixedPortion := standaloneBracesPattern.ReplaceAllString(lastPortion, "\n")
				content = content[:max(0, endDocIdx-200)] + fixedPortion + content[endDocIdx:]
				fixed = true
				logger.Debug("rule-based fix: removed standalone braces before \\end{document}")
			}
		}
	}

	// Pattern 5: Remove single standalone } on a line before \end{document}
	// This handles cases where LLM adds a single extra } before \end{document}
	// Pattern: \n}\n...\end{document} -> \n\end{document}
	// Allow multiple newlines/whitespace between } and \end{document}
	// 
	// A standalone } on its own line right before \end{document} is almost always wrong.
	// Valid closing braces are typically on the same line as the content they close,
	// or part of \end{...} commands. A lone } followed by blank lines and \end{document}
	// is a common LLM translation artifact.
	singleBraceBeforeEndDocPattern := regexp.MustCompile(`\n\s*\}\s*\n+\s*(\\end\{document\})`)
	if singleBraceBeforeEndDocPattern.MatchString(content) {
		// Remove the standalone brace - it's almost certainly an LLM artifact
		content = singleBraceBeforeEndDocPattern.ReplaceAllString(content, "\n$1")
		fixed = true
		logger.Debug("rule-based fix: removed single extra brace before \\end{document}")
	}

	return content, fixed
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}


// fixNewreptheoremDefinition fixes a common bug in arXiv papers where the
// \newreptheorem definition is missing closing braces.
// Pattern: \newcommand{\newreptheorem}[2]{%
//          \newenvironment{rep#1}[1]{...}%
//          {\end{rep@theorem}
//          \makeatother
// Should be:
//          {\end{rep@theorem}}}
//          \makeatother
func fixNewreptheoremDefinition(content string) (string, bool) {
	// Pattern: {\end{rep@theorem} followed by newline and \makeatother
	// without the closing }}}
	pattern := regexp.MustCompile(`(\{\\end\{rep@theorem\})(\s*\n\\makeatother)`)
	if pattern.MatchString(content) {
		// Check if it's already fixed (has }}} before \makeatother)
		alreadyFixed := regexp.MustCompile(`\{\\end\{rep@theorem\}\}\}\}\s*\n\\makeatother`)
		if alreadyFixed.MatchString(content) {
			return content, false
		}
		
		content = pattern.ReplaceAllString(content, "$1}}}$2")
		logger.Debug("rule-based fix: fixed \\newreptheorem definition missing closing braces")
		return content, true
	}
	return content, false
}


// fixNestedTabularExtraBrace fixes extra closing brace after nested \end{tabular}
// This is a common issue in arXiv papers where authors have nested tabular environments
// inside table cells and accidentally add an extra } after \end{tabular}
// Pattern: \end{tabular}} & or \end{tabular}} \\ should be \end{tabular} & or \end{tabular} \\
func fixNestedTabularExtraBrace(content string) (string, bool) {
	// Pattern: \end{tabular}} followed by & or \\ or whitespace+&/\\
	// This indicates a nested tabular inside a table cell with an extra brace
	pattern := regexp.MustCompile(`(\\end\{tabular\})\}(\s*[&\\])`)
	
	if pattern.MatchString(content) {
		content = pattern.ReplaceAllString(content, "$1$2")
		logger.Debug("rule-based fix: removed extra } after nested \\end{tabular}")
		return content, true
	}
	
	return content, false
}
