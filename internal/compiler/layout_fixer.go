// Package compiler provides LaTeX compilation functionality.
package compiler

import (
	"regexp"
	"strings"

	"latex-translator/internal/logger"
)

// FixTableLayout applies comprehensive table layout fixes to prevent overflow and overlap.
// This includes:
// - Reducing column spacing (tabcolsep)
// - Applying smart scaling based on column count
// - Converting wide tables to table* environment
// - Improving float placement parameters
func FixTableLayout(content string) (string, bool) {
	anyFixed := false
	lines := strings.Split(content, "\n")

	// Track state
	inTable := false
	inTableStar := false
	tableStartLine := -1
	hasResizebox := false
	currentTableLines := []string{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Detect entering table or table* environment
		if strings.Contains(line, `\begin{table*}`) {
			inTableStar = true
			inTable = false
			tableStartLine = i
			hasResizebox = false
			currentTableLines = []string{line}
			// Improve float placement
			if strings.Contains(line, `\begin{table*}[!t]`) || strings.Contains(line, `\begin{table*}[t]`) {
				lines[i] = strings.Replace(line, `\begin{table*}[!t]`, `\begin{table*}[!htbp]`, 1)
				lines[i] = strings.Replace(lines[i], `\begin{table*}[t]`, `\begin{table*}[!htbp]`, 1)
				anyFixed = true
			}
			continue
		}

		if strings.Contains(line, `\begin{table}`) && !strings.Contains(line, `\begin{table*}`) {
			inTable = true
			inTableStar = false
			tableStartLine = i
			hasResizebox = false
			currentTableLines = []string{line}
			// Improve float placement
			if strings.Contains(line, `\begin{table}[!t]`) || strings.Contains(line, `\begin{table}[t]`) || strings.Contains(line, `\begin{table}[!h]`) {
				lines[i] = strings.Replace(line, `\begin{table}[!t]`, `\begin{table}[!htbp]`, 1)
				lines[i] = strings.Replace(lines[i], `\begin{table}[t]`, `\begin{table}[!htbp]`, 1)
				lines[i] = strings.Replace(lines[i], `\begin{table}[!h]`, `\begin{table}[!htbp]`, 1)
				anyFixed = true
			}
			continue
		}

		// Inside table environment
		if inTable || inTableStar {
			currentTableLines = append(currentTableLines, line)

			// Ensure tabcolsep is set
			if strings.Contains(line, `\centering`) && i > tableStartLine {
				// Check if tabcolsep already exists
				hasTabcolsep := false
				for j := tableStartLine; j < i+3 && j < len(lines); j++ {
					if strings.Contains(lines[j], `\tabcolsep`) {
						hasTabcolsep = true
						break
					}
				}

				if !hasTabcolsep {
					indent := ""
					for _, ch := range line {
						if ch == ' ' || ch == '\t' {
							indent += string(ch)
						} else {
							break
						}
					}
					lines[i] = indent + `\setlength{\tabcolsep}{1.5pt}` + "\n" + line
					anyFixed = true
				}
			}

			// Detect \begin{tabular} and apply scaling
			if strings.Contains(line, `\begin{tabular}`) {
				// Extract column definition
				tabularPattern := regexp.MustCompile(`\\begin\{tabular\}\{([^}]+)\}`)
				matches := tabularPattern.FindStringSubmatch(line)

				if len(matches) > 1 {
					colDef := matches[1]
					// Count columns
					colCount := 0
					for _, ch := range colDef {
						if ch == 'c' || ch == 'l' || ch == 'r' || ch == 'p' {
							colCount++
						}
					}

					// Determine if scaling is needed
					needsScalebox := false
					needsResizebox := false

					if inTableStar {
						if colCount >= 12 {
							needsScalebox = true
						} else if colCount >= 8 {
							needsResizebox = true
						}
					} else {
						if colCount >= 8 {
							needsScalebox = true
						} else if colCount >= 6 {
							needsResizebox = true
						}
					}

					// Apply scaling if needed and not already present
					if (needsScalebox || needsResizebox) && !strings.Contains(line, `\scalebox`) && !strings.Contains(line, `\resizebox`) {
						indent := ""
						for _, ch := range line {
							if ch == ' ' || ch == '\t' {
								indent += string(ch)
							} else {
								break
							}
						}

						if needsScalebox {
							scale := "0.75"
							if inTableStar && colCount >= 14 {
								scale = "0.65"
							} else if inTableStar && colCount >= 12 {
								scale = "0.7"
							} else if colCount >= 10 {
								scale = "0.7"
							}
							lines[i] = indent + `\scalebox{` + scale + `}{%` + "\n" + line
							hasResizebox = true
							anyFixed = true
						} else if needsResizebox {
							width := `\textwidth`
							if inTable {
								width = `\columnwidth`
							}
							lines[i] = indent + `\resizebox{` + width + `}{!}{%` + "\n" + line
							hasResizebox = true
							anyFixed = true
						}
					}
				}
			}

			// Close scaling at \end{tabular}
			if strings.Contains(line, `\end{tabular}`) && hasResizebox {
				indent := ""
				for _, ch := range line {
					if ch == ' ' || ch == '\t' {
						indent += string(ch)
					} else {
						break
					}
				}
				lines[i] = line + "\n" + indent + `}% end scaling`
				hasResizebox = false
				anyFixed = true
			}

			// Detect leaving table environment
			if strings.Contains(line, `\end{table*}`) {
				inTableStar = false
				tableStartLine = -1
				currentTableLines = nil
			}
			if strings.Contains(line, `\end{table}`) && !strings.Contains(line, `\end{table*}`) {
				inTable = false
				tableStartLine = -1
				currentTableLines = nil
			}
		}
	}

	if anyFixed {
		logger.Debug("layout fix: applied table layout fixes")
	}

	return strings.Join(lines, "\n"), anyFixed
}

// FixFigureLayout applies figure layout fixes to prevent overflow and overlap.
// This includes:
// - Fixing figure*/figure environment mismatches
// - Adjusting figure widths
// - Improving float placement parameters
func FixFigureLayout(content string) (string, bool) {
	anyFixed := false

	// Fix 1: Ensure figure* uses appropriate width (0.85\textwidth)
	// Fix 2: Ensure figure uses appropriate width (0.9\columnwidth)
	lines := strings.Split(content, "\n")
	inFigureStar := false
	inFigure := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Detect figure* environment
		if strings.Contains(line, `\begin{figure*}`) {
			inFigureStar = true
			inFigure = false
			// Improve float placement
			if strings.Contains(line, `\begin{figure*}[!t]`) || strings.Contains(line, `\begin{figure*}[t]`) {
				lines[i] = strings.Replace(line, `\begin{figure*}[!t]`, `\begin{figure*}[!htbp]`, 1)
				lines[i] = strings.Replace(lines[i], `\begin{figure*}[t]`, `\begin{figure*}[!htbp]`, 1)
				anyFixed = true
			}
		}

		// Detect figure environment (not figure*)
		if strings.Contains(line, `\begin{figure}`) && !strings.Contains(line, `\begin{figure*}`) {
			inFigure = true
			inFigureStar = false
			// Improve float placement
			if strings.Contains(line, `\begin{figure}[!t]`) || strings.Contains(line, `\begin{figure}[t]`) || strings.Contains(line, `\begin{figure}[!h]`) {
				lines[i] = strings.Replace(line, `\begin{figure}[!t]`, `\begin{figure}[!htbp]`, 1)
				lines[i] = strings.Replace(lines[i], `\begin{figure}[t]`, `\begin{figure}[!htbp]`, 1)
				lines[i] = strings.Replace(lines[i], `\begin{figure}[!h]`, `\begin{figure}[!htbp]`, 1)
				anyFixed = true
			}
		}

		// Adjust includegraphics width in figure*
		if inFigureStar && strings.Contains(line, `\includegraphics`) {
			// Pattern: \includegraphics[width=X\textwidth]{...}
			widthPattern := regexp.MustCompile(`(\\includegraphics\[width=)([0-9.]+)(\\textwidth\])`)
			if widthPattern.MatchString(line) {
				// Replace with 0.85\textwidth for figure*
				lines[i] = widthPattern.ReplaceAllString(line, `${1}0.85${3}`)
				anyFixed = true
			}
		}

		// Adjust includegraphics width in figure
		if inFigure && strings.Contains(line, `\includegraphics`) {
			// Pattern: \includegraphics[width=X\columnwidth]{...}
			widthPattern := regexp.MustCompile(`(\\includegraphics\[width=)([0-9.]+)(\\columnwidth\])`)
			if widthPattern.MatchString(line) {
				// Replace with 0.9\columnwidth for figure
				lines[i] = widthPattern.ReplaceAllString(line, `${1}0.9${3}`)
				anyFixed = true
			}
		}

		// Fix mismatched end tags
		if inFigureStar && strings.Contains(line, `\end{figure}`) && !strings.Contains(line, `\end{figure*}`) {
			lines[i] = strings.Replace(line, `\end{figure}`, `\end{figure*}`, 1)
			anyFixed = true
			logger.Debug("layout fix: fixed figure*/figure end tag mismatch")
		}

		if inFigure && strings.Contains(line, `\end{figure*}`) {
			lines[i] = strings.Replace(line, `\end{figure*}`, `\end{figure}`, 1)
			anyFixed = true
			logger.Debug("layout fix: fixed figure/figure* end tag mismatch")
		}

		// Detect leaving figure environments
		if strings.Contains(line, `\end{figure*}`) {
			inFigureStar = false
		}
		if strings.Contains(line, `\end{figure}`) && !strings.Contains(line, `\end{figure*}`) {
			inFigure = false
		}
	}

	if anyFixed {
		logger.Debug("layout fix: applied figure layout fixes")
	}

	return strings.Join(lines, "\n"), anyFixed
}

// ConvertWideTableToTableStar converts extremely wide tables (14+ columns) from table to table*
// This prevents overlap with surrounding text by making the table span both columns
func ConvertWideTableToTableStar(content string) (string, bool) {
	anyFixed := false
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Look for \begin{table} (not table*)
		if strings.Contains(line, `\begin{table}`) && !strings.Contains(line, `\begin{table*}`) {
			// Find the corresponding \begin{tabular} to count columns
			tableStartIdx := i
			tabularIdx := -1
			endTableIdx := -1

			// Search forward for \begin{tabular} and \end{table}
			for j := i + 1; j < len(lines) && j < i+50; j++ {
				if strings.Contains(lines[j], `\begin{tabular}`) && tabularIdx == -1 {
					tabularIdx = j
				}
				if strings.Contains(lines[j], `\end{table}`) && !strings.Contains(lines[j], `\end{table*}`) {
					endTableIdx = j
					break
				}
			}

			if tabularIdx > 0 && endTableIdx > 0 {
				// Extract column definition
				tabularPattern := regexp.MustCompile(`\\begin\{tabular\}\{([^}]+)\}`)
				matches := tabularPattern.FindStringSubmatch(lines[tabularIdx])

				if len(matches) > 1 {
					colDef := matches[1]
					// Count columns
					colCount := 0
					for _, ch := range colDef {
						if ch == 'c' || ch == 'l' || ch == 'r' || ch == 'p' {
							colCount++
						}
					}

					// If 14+ columns, convert to table*
					if colCount >= 14 {
						// Convert \begin{table} to \begin{table*}[!t]
						lines[tableStartIdx] = strings.Replace(lines[tableStartIdx], `\begin{table}[!htbp]`, `\begin{table*}[!t]`, 1)
						lines[tableStartIdx] = strings.Replace(lines[tableStartIdx], `\begin{table}[!t]`, `\begin{table*}[!t]`, 1)
						lines[tableStartIdx] = strings.Replace(lines[tableStartIdx], `\begin{table}`, `\begin{table*}[!t]`, 1)

						// Convert \end{table} to \end{table*}
						lines[endTableIdx] = strings.Replace(lines[endTableIdx], `\end{table}`, `\end{table*}`, 1)

						anyFixed = true
						logger.Debug("layout fix: converted wide table to table*",
							logger.Int("columns", colCount),
							logger.Int("line", tableStartIdx+1))

						// Skip to end of this table
						i = endTableIdx
					}
				}
			}
		}
	}

	if anyFixed {
		logger.Debug("layout fix: converted wide tables to table*")
	}

	return strings.Join(lines, "\n"), anyFixed
}
